package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"net"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

// EmailStatus represents the result of an SMTP check
type EmailStatus string

const (
	StatusValid      EmailStatus = "VALID"
	StatusInvalid    EmailStatus = "INVALID"
	StatusGreylisted EmailStatus = "GREYLISTED"
	StatusCatchAll   EmailStatus = "CATCH_ALL"
	StatusUnknown    EmailStatus = "UNKNOWN"
)

// SMTPResult contains the result of an SMTP check
type SMTPResult struct {
	Status       EmailStatus
	SMTPCode     int
	BounceReason string
	IsRetryable  bool // True if this should be retried (450, 451, 421)
}

// ProxyConfig contains SOCKS5 proxy configuration
type ProxyConfig struct {
	Address  string // host:port
	Username string
	Password string
}

// CheckEmail performs SMTP validation on an email address with enterprise features:
// 1. SOCKS5 proxy support with authentication (fail-safe, no fallback)
// 2. Catch-all detection via random probe
// 3. Greylisting detection (returns IsRetryable flag)
// 4. Proper SMTP identity using WORKER_HOSTNAME
func CheckEmail(ctx context.Context, email string, isDevMode bool, proxyConfig *ProxyConfig, workerHostname string) (*SMTPResult, error) {
	// Extract domain from email
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return &SMTPResult{
			Status:      StatusInvalid,
			SMTPCode:    550,
			BounceReason: "Invalid email format",
			IsRetryable: false,
		}, nil
	}

	domain := parts[1]

	var mailServer string
	var port string

	if isDevMode {
		// DEV MODE: Skip DNS lookup, use MailHog directly
		mailServer = "localhost"
		port = "1025"
	} else {
		// ============================================================
		// CRITICAL: MX LOOKUP VALIDATION (The "0% Match" Bug Fix)
		// ============================================================
		// PRODUCTION MODE: Look up MX records
		// CRITICAL: If MX lookup fails or returns empty, mark as INVALID immediately
		// Do NOT proceed to SMTP checks
		mxRecords, err := net.LookupMX(domain)
		
		// Check for lookup errors
		if err != nil {
			log.Printf("❌ MX lookup failed for domain %s: %v", domain, err)
			return &SMTPResult{
				Status:       StatusInvalid,
				SMTPCode:     550,
				BounceReason: fmt.Sprintf("MX lookup failed: %v", err),
				IsRetryable:  false,
			}, nil
		}

		// Check for empty MX records list
		if len(mxRecords) == 0 {
			log.Printf("❌ No MX records found for domain %s", domain)
			return &SMTPResult{
				Status:       StatusInvalid,
				SMTPCode:     550,
				BounceReason: "No MX records found",
				IsRetryable:  false,
			}, nil
		}

		// Validate MX record hostname is not empty
		if mxRecords[0].Host == "" || strings.TrimSpace(mxRecords[0].Host) == "" {
			log.Printf("❌ Invalid MX record (empty hostname) for domain %s", domain)
			return &SMTPResult{
				Status:       StatusInvalid,
				SMTPCode:     550,
				BounceReason: "Invalid MX record (empty hostname)",
				IsRetryable:  false,
			}, nil
		}

		// Use the first MX record
		mailServer = strings.TrimSuffix(mxRecords[0].Host, ".")
		port = "25"
		
		// Final validation: mailServer must not be empty after trimming
		if mailServer == "" {
			log.Printf("❌ Invalid MX record (empty after trim) for domain %s", domain)
			return &SMTPResult{
				Status:       StatusInvalid,
				SMTPCode:     550,
				BounceReason: "Invalid MX record (empty hostname after processing)",
				IsRetryable:  false,
			}, nil
		}
	}

	// ============================================================
	// FEATURE 3: CATCH-ALL DETECTION (Random Probe)
	// ============================================================
	// Before validating the actual email, probe with a random address
	// to detect catch-all domains
	if !isDevMode {
		probeResult := checkCatchAll(mailServer, port, domain, proxyConfig, workerHostname)
		if probeResult.IsCatchAll {
			// Domain is catch-all - mark original email as CATCH_ALL immediately
			return &SMTPResult{
				Status:       StatusCatchAll,
				SMTPCode:     250, // Catch-all accepts all addresses
				BounceReason: "Catch-all domain detected via probe",
				IsRetryable: false,
			}, nil
		}
		// If probe returned 550, domain is normal - proceed with real validation
	}

	// ============================================================
	// FEATURE 2: SOCKS5 PROXY SUPPORT (Fail-Safe, No Fallback)
	// ============================================================
	// Connect to mail server (with SOCKS5 proxy if configured)
	conn, err := connectWithProxy(ctx, mailServer, port, proxyConfig, isDevMode)
	if err != nil {
		// FAIL-SAFE: If proxy connection fails, mark as UNKNOWN (do not fallback)
		log.Printf("❌ Proxy connection failed for %s: %v", domain, err)
		return &SMTPResult{
			Status:      StatusUnknown,
			SMTPCode:    0,
			BounceReason: fmt.Sprintf("Connection failed: %v", err),
			IsRetryable: false,
		}, nil
	}
	defer conn.Close()

	// Log successful proxy connection
	if proxyConfig != nil && proxyConfig.Address != "" && !isDevMode {
		log.Printf("⚡ Connected via Proxy to [%s]", domain)
	}

	// Set read/write timeout
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	// Read initial greeting
	buffer := make([]byte, 512)
	n, err := conn.Read(buffer)
	if err != nil {
		return &SMTPResult{
			Status:      StatusUnknown,
			SMTPCode:    0,
			BounceReason: fmt.Sprintf("Failed to read greeting: %v", err),
			IsRetryable: false,
		}, nil
	}

	response := string(buffer[:n])
	code := parseSMTPCode(response)
	if code != 220 {
		return &SMTPResult{
			Status:      StatusUnknown,
			SMTPCode:    code,
			BounceReason: fmt.Sprintf("Server greeting error: %s", response),
			IsRetryable: false,
		}, nil
	}

	// ============================================================
	// FEATURE 3: PROPER SMTP IDENTITY (WORKER_HOSTNAME)
	// ============================================================
	// Send HELO with proper worker hostname (never localhost/127.0.0.1)
	heloCmd := fmt.Sprintf("HELO %s\r\n", workerHostname)
	_, err = conn.Write([]byte(heloCmd))
	if err != nil {
		return &SMTPResult{
			Status:      StatusUnknown,
			SMTPCode:    0,
			BounceReason: fmt.Sprintf("Failed to send HELO: %v", err),
			IsRetryable: false,
		}, nil
	}

	n, err = conn.Read(buffer)
	if err != nil {
		return &SMTPResult{
			Status:      StatusUnknown,
			SMTPCode:    0,
			BounceReason: fmt.Sprintf("Failed to read HELO response: %v", err),
			IsRetryable: false,
		}, nil
	}

	response = string(buffer[:n])
	code = parseSMTPCode(response)
	if code != 250 {
		return &SMTPResult{
			Status:      StatusUnknown,
			SMTPCode:    code,
			BounceReason: fmt.Sprintf("HELO error: %s", response),
			IsRetryable: false,
		}, nil
	}

	// Send MAIL FROM
	_, err = conn.Write([]byte("MAIL FROM:<check@yourdomain.com>\r\n"))
	if err != nil {
		return &SMTPResult{
			Status:      StatusUnknown,
			SMTPCode:    0,
			BounceReason: fmt.Sprintf("Failed to send MAIL FROM: %v", err),
			IsRetryable: false,
		}, nil
	}

	n, err = conn.Read(buffer)
	if err != nil {
		return &SMTPResult{
			Status:      StatusUnknown,
			SMTPCode:    0,
			BounceReason: fmt.Sprintf("Failed to read MAIL FROM response: %v", err),
			IsRetryable: false,
		}, nil
	}

	response = string(buffer[:n])
	code = parseSMTPCode(response)
	if code != 250 {
		return &SMTPResult{
			Status:      StatusUnknown,
			SMTPCode:    code,
			BounceReason: fmt.Sprintf("MAIL FROM error: %s", response),
			IsRetryable: false,
		}, nil
	}

	// Send RCPT TO (this is the critical check)
	rcptCmd := fmt.Sprintf("RCPT TO:<%s>\r\n", email)
	_, err = conn.Write([]byte(rcptCmd))
	if err != nil {
		return &SMTPResult{
			Status:      StatusUnknown,
			SMTPCode:    0,
			BounceReason: fmt.Sprintf("Failed to send RCPT TO: %v", err),
			IsRetryable: false,
		}, nil
	}

	n, err = conn.Read(buffer)
	if err != nil {
		return &SMTPResult{
			Status:      StatusUnknown,
			SMTPCode:    0,
			BounceReason: fmt.Sprintf("Failed to read RCPT TO response: %v", err),
			IsRetryable: false,
		}, nil
	}

	response = string(buffer[:n])
	code = parseSMTPCode(response)
	bounceReason := strings.TrimSpace(response[4:]) // Remove the code and space

	// ============================================================
	// FEATURE 2: GREYLISTING DETECTION
	// ============================================================
	// Parse response code and determine if retryable (450, 451, 421)
	var status EmailStatus
	var isRetryable bool

	switch {
	case code == 250:
		// Valid recipient
		status = StatusValid
		isRetryable = false
	case code == 251 || code == 252:
		// Valid but forwarded or catch-all
		status = StatusCatchAll
		isRetryable = false
	case code == 450 || code == 451 || code == 421:
		// Greylisted or temporary failure - RETRYABLE
		status = StatusGreylisted
		isRetryable = true // Mark for retry queue
	case code == 550 || code == 551 || code == 553:
		// Invalid (permanent failure)
		status = StatusInvalid
		isRetryable = false
	default:
		// Unknown response
		status = StatusUnknown
		isRetryable = false
	}

	// Send QUIT (do not send DATA)
	conn.Write([]byte("QUIT\r\n"))
	conn.Read(buffer) // Read QUIT response (ignore errors)

	return &SMTPResult{
		Status:       status,
		SMTPCode:     code,
		BounceReason: bounceReason,
		IsRetryable:  isRetryable,
	}, nil
}

// ============================================================
// FEATURE 2: SOCKS5 PROXY CONNECTION (With Authentication)
// ============================================================
// connectWithProxy establishes a TCP connection through SOCKS5 proxy
// FAIL-SAFE: If proxy fails, returns error (no fallback to direct connection)
func connectWithProxy(ctx context.Context, mailServer, port string, proxyConfig *ProxyConfig, isDevMode bool) (net.Conn, error) {
	targetAddr := net.JoinHostPort(mailServer, port)

	// DEV MODE: Always use direct connection (MailHog doesn't need proxy)
	if isDevMode {
		return net.DialTimeout("tcp", targetAddr, 5*time.Second)
	}

	// PRODUCTION MODE: MUST use proxy if configured (fail-safe)
	if proxyConfig != nil && proxyConfig.Address != "" {
		// Create SOCKS5 dialer with authentication
		var auth *proxy.Auth
		if proxyConfig.Username != "" && proxyConfig.Password != "" {
			auth = &proxy.Auth{
				User:     proxyConfig.Username,
				Password: proxyConfig.Password,
			}
		}

		dialer, err := proxy.SOCKS5("tcp", proxyConfig.Address, auth, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("failed to create SOCKS5 dialer: %v", err)
		}

		// Connect through proxy with timeout
		type result struct {
			conn net.Conn
			err  error
		}
		resultChan := make(chan result, 1)
		go func() {
			conn, err := dialer.Dial("tcp", targetAddr)
			resultChan <- result{conn: conn, err: err}
		}()

		select {
		case res := <-resultChan:
			if res.err != nil {
				// FAIL-SAFE: Log error and return (no fallback)
				log.Printf("❌ SOCKS5 proxy connection failed to %s: %v", targetAddr, res.err)
				return nil, fmt.Errorf("SOCKS5 proxy connection failed: %v", res.err)
			}
			return res.conn, nil
		case <-ctx.Done():
			return nil, fmt.Errorf("SOCKS5 proxy connection cancelled: %v", ctx.Err())
		case <-time.After(5 * time.Second):
			return nil, fmt.Errorf("SOCKS5 proxy connection timeout")
		}
	}

	// If no proxy configured in production, this is an error (safety requirement)
	// In production, we should always use proxy to protect IP
	if !isDevMode {
		return nil, fmt.Errorf("SOCKS5_PROXY not configured in production mode (safety requirement)")
	}

	// Fallback only for dev mode
	return net.DialTimeout("tcp", targetAddr, 5*time.Second)
}

// ============================================================
// FEATURE 3: CATCH-ALL DETECTION (Random Probe)
// ============================================================
// ProbeResult contains the result of a catch-all probe
type ProbeResult struct {
	IsCatchAll bool
	SMTPCode   int
}

// checkCatchAll performs a probe check with a random email address
// to detect if the domain is a catch-all
func checkCatchAll(mailServer, port, domain string, proxyConfig *ProxyConfig, workerHostname string) ProbeResult {
	// Generate a random, impossible email address for this domain
	// Format: randomstring@domain.com (e.g., d8s7f6g8s7df@example.com)
	randomString := generateRandomString(15)
	probeEmail := fmt.Sprintf("%s@%s", randomString, domain)

	// Connect to mail server
	ctx := context.Background()
	conn, err := connectWithProxy(ctx, mailServer, port, proxyConfig, false)
	if err != nil {
		// Connection failed - can't determine catch-all, assume normal
		return ProbeResult{IsCatchAll: false, SMTPCode: 0}
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(10 * time.Second))

	// Perform minimal SMTP handshake for probe
	buffer := make([]byte, 512)

	// Read greeting
	conn.Read(buffer)

	// Send HELO with proper hostname
	heloCmd := fmt.Sprintf("HELO %s\r\n", workerHostname)
	conn.Write([]byte(heloCmd))
	conn.Read(buffer)

	// Send MAIL FROM
	conn.Write([]byte("MAIL FROM:<check@yourdomain.com>\r\n"))
	conn.Read(buffer)

	// Send RCPT TO with random probe email
	rcptCmd := fmt.Sprintf("RCPT TO:<%s>\r\n", probeEmail)
	conn.Write([]byte(rcptCmd))

	n, err := conn.Read(buffer)
	if err != nil {
		return ProbeResult{IsCatchAll: false, SMTPCode: 0}
	}

	response := string(buffer[:n])
	code := parseSMTPCode(response)

	// Send QUIT
	conn.Write([]byte("QUIT\r\n"))
	conn.Read(buffer)

	// Decision Tree:
	// - If probe returns 250 OK: Domain is CATCH-ALL (accepts random address)
	// - If probe returns 550: Domain is normal (rejects random address)
	if code == 250 || code == 251 || code == 252 {
		return ProbeResult{IsCatchAll: true, SMTPCode: code}
	}

	return ProbeResult{IsCatchAll: false, SMTPCode: code}
}

// generateRandomString creates a random alphanumeric string of specified length
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[num.Int64()]
	}
	return string(b)
}

// parseSMTPCode extracts the 3-digit SMTP code from a response
func parseSMTPCode(response string) int {
	if len(response) < 3 {
		return 0
	}

	var code int
	fmt.Sscanf(response[:3], "%d", &code)
	return code
}
