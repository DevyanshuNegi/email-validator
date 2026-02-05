package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

// EmailJob represents a job from Redis queue
type EmailJob struct {
	JobID string `json:"jobId"`
	Email string `json:"email"`
}

const (
	workerCount        = 50
	redisQueue         = "email_queue"
	retryQueue         = "email_retry_queue" // Redis ZSET for greylisting retries
	retryDelay         = 900                 // 15 minutes in seconds
	retryCheckInterval = 30 * time.Second    // Check retry queue every 30 seconds
)

var (
	isDevMode      bool
	proxyConfig    *ProxyConfig
	workerHostname string
	rateLimiter    *RateLimiterManager
)

var (
	redisClient *redis.Client
	db          *sql.DB
)

func main() {
	fmt.Println("🚀 Starting Email Validator Worker (Enterprise Edition - Production Safe)...")

	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("⚠️  No .env file found, using defaults: %v", err)
	}

	// Check if we're in dev mode
	isDevMode = os.Getenv("IS_DEV") == "true"
	if isDevMode {
		fmt.Println("🔧 Running in DEV MODE - Using MailHog (localhost:1025)")
	} else {
		fmt.Println("🌐 Running in PRODUCTION MODE - Using real SMTP servers")
	}

	// ============================================================
	// FEATURE 1: RATE LIMITER INITIALIZATION (STRICT GLOBAL LIMIT)
	// ============================================================
	rateLimiter = NewRateLimiterManager()
	fmt.Println("🛡️  Rate Limiter initialized (Global: 2/sec TOTAL, Domain-specific limits active)")

	// ============================================================
	// FEATURE 2: SOCKS5 PROXY CONFIGURATION (With Authentication)
	// ============================================================
	socks5ProxyAddr := os.Getenv("SOCKS5_PROXY")
	if socks5ProxyAddr != "" {
		proxyConfig = &ProxyConfig{
			Address:  socks5ProxyAddr,
			Username: os.Getenv("PROXY_USER"),
			Password: os.Getenv("PROXY_PASS"),
		}
		fmt.Printf("🔌 SOCKS5 Proxy configured: %s", socks5ProxyAddr)
		if proxyConfig.Username != "" {
			fmt.Printf(" (Authenticated: %s)", proxyConfig.Username)
		}
		fmt.Println()
	} else if !isDevMode {
		log.Printf("⚠️  WARNING: SOCKS5_PROXY not set in production mode - IP protection disabled")
	}

	// ============================================================
	// FEATURE 3: WORKER HOSTNAME CONFIGURATION
	// ============================================================
	workerHostname = os.Getenv("WORKER_HOSTNAME")
	if workerHostname == "" {
		// Fallback: try to get hostname from system
		hostname, err := os.Hostname()
		if err != nil || hostname == "" || hostname == "localhost" || strings.HasPrefix(hostname, "127.0.0.1") {
			log.Fatalf("❌ WORKER_HOSTNAME must be set in production (e.g., worker1.devyanshu.me)")
		}
		workerHostname = hostname
	}

	// Safety check: Never use localhost or 127.0.0.1
	if workerHostname == "localhost" || workerHostname == "127.0.0.1" || strings.HasPrefix(workerHostname, "127.") {
		if !isDevMode {
			log.Fatalf("❌ WORKER_HOSTNAME cannot be localhost/127.0.0.1 in production mode")
		}
		// In dev mode, allow localhost but warn
		log.Printf("⚠️  Using localhost as WORKER_HOSTNAME (dev mode only)")
	}

	fmt.Printf("🆔 Worker Hostname: %s\n", workerHostname)

	// Get Redis configuration from env or use defaults
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	redisPassword := os.Getenv("REDIS_PASSWORD")
	redisDB := 0
	if dbStr := os.Getenv("REDIS_DB"); dbStr != "" {
		fmt.Sscanf(dbStr, "%d", &redisDB)
	}

	// Connect to Redis
	redisClient = redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       redisDB,
	})

	ctx := context.Background()
	_, err := redisClient.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("❌ Failed to connect to Redis: %v", err)
	}
	fmt.Println("✅ Connected to Redis")

	// Connect to PostgreSQL
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5433/emailvalidator?sslmode=disable"
	}
	db, err = sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("❌ Failed to connect to PostgreSQL: %v", err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		log.Fatalf("❌ Failed to ping PostgreSQL: %v", err)
	}
	fmt.Println("✅ Connected to PostgreSQL")

	// Create worker pool
	jobChan := make(chan EmailJob, workerCount*2) // Buffer for jobs

	// Start workers
	for i := 0; i < workerCount; i++ {
		go worker(i+1, jobChan, ctx)
	}

	fmt.Printf("✅ Started %d workers\n", workerCount)
	fmt.Println("📬 Listening for emails in queue:", redisQueue)

	// ============================================================
	// FEATURE 2: START RETRY MONITOR GOROUTINE
	// ============================================================
	// Start the retry monitor in a separate goroutine
	go RetryMonitor(ctx)
	fmt.Println("🔄 Retry Monitor started (checking every 30 seconds)")

	// Main loop: BRPOP from Redis queue
	for {
		// ============================================================
		// CRITICAL: GLOBAL RATE LIMIT ENFORCEMENT (Safety Valve)
		// ============================================================
		// BEFORE picking up ANY job, wait for global rate limiter
		// This ensures we NEVER process more than 2 emails/second TOTAL
		if err := rateLimiter.globalLimiter.Wait(ctx); err != nil {
			log.Printf("⚠️  Global rate limit wait cancelled: %v", err)
			continue
		}

		// BRPOP with 5 second timeout
		result, err := redisClient.BRPop(ctx, 5*time.Second, redisQueue).Result()
		if err != nil {
			if err == redis.Nil {
				// Timeout - no jobs available, continue
				continue
			}
			log.Printf("⚠️  Error reading from Redis: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		// Parse the job
		if len(result) < 2 {
			log.Printf("⚠️  Invalid queue result: %v", result)
			continue
		}

		var job EmailJob
		err = json.Unmarshal([]byte(result[1]), &job)
		if err != nil {
			log.Printf("⚠️  Failed to parse job JSON: %v", err)
			continue
		}

		// Send job to worker pool
		select {
		case jobChan <- job:
			// Job sent successfully
		default:
			log.Printf("⚠️  Worker pool full, dropping job: %s", job.Email)
		}
	}
}

// ============================================================
// FEATURE 2: RETRY MONITOR (ZSET Pattern)
// ============================================================
// RetryMonitor runs in a separate goroutine and monitors the retry queue
// It checks every 30 seconds for emails that are ready to be retried
func RetryMonitor(ctx context.Context) {
	ticker := time.NewTicker(retryCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Get current timestamp
			now := time.Now().Unix()

			// Get all items from ZSET with score <= now (ready to retry)
			// ZRANGEBYSCORE email_retry_queue -inf now
			items, err := redisClient.ZRangeByScore(ctx, retryQueue, &redis.ZRangeBy{
				Min: "-inf",
				Max: fmt.Sprintf("%d", now),
			}).Result()

			if err != nil {
				log.Printf("⚠️  Error reading retry queue: %v", err)
				continue
			}

			if len(items) == 0 {
				// No items ready for retry
				continue
			}

			log.Printf("🔄 Found %d email(s) ready for retry", len(items))

			// Process each item
			for _, itemJSON := range items {
				// Parse the job
				var job EmailJob
				err := json.Unmarshal([]byte(itemJSON), &job)
				if err != nil {
					log.Printf("⚠️  Failed to parse retry job JSON: %v", err)
					// Remove invalid item from ZSET
					redisClient.ZRem(ctx, retryQueue, itemJSON)
					continue
				}

				// Remove from retry queue (atomic operation)
				removed, err := redisClient.ZRem(ctx, retryQueue, itemJSON).Result()
				if err != nil || removed == 0 {
					log.Printf("⚠️  Failed to remove item from retry queue: %v", err)
					continue
				}

				// Push back to main queue for re-processing
				jobJSON, _ := json.Marshal(job)
				err = redisClient.LPush(ctx, redisQueue, string(jobJSON)).Err()
				if err != nil {
					log.Printf("⚠️  Failed to push retry job to queue: %v", err)
					// Re-add to retry queue if push failed
					redisClient.ZAdd(ctx, retryQueue, redis.Z{
						Score:  float64(now + retryDelay),
						Member: itemJSON,
					})
					continue
				}

				log.Printf("🔄 Retrying email: %s (Job: %s)", job.Email, job.JobID)
			}

		case <-ctx.Done():
			return
		}
	}
}

// worker processes email validation jobs
func worker(id int, jobChan <-chan EmailJob, ctx context.Context) {
	for job := range jobChan {
		processEmail(id, job, ctx)
	}
}

// processEmail performs SMTP check and updates database
// Now includes rate limiting and proper proxy/hostname handling
func processEmail(workerID int, job EmailJob, ctx context.Context) {
	fmt.Printf("[Worker %d] 🔍 Checking: %s\n", workerID, job.Email)

	// ============================================================
	// CRITICAL: EMAIL SYNTAX VALIDATION (RFC 5322 Compliant)
	// ============================================================
	// Validate email syntax BEFORE any processing
	if !isValidEmailSyntax(job.Email) {
		log.Printf("[Worker %d] ❌ Invalid email syntax: %s", workerID, job.Email)
		updateEmailStatus(job.JobID, job.Email, "INVALID", 550, "Invalid email syntax")
		return
	}

	// Extract domain for rate limiting
	parts := strings.Split(job.Email, "@")
	if len(parts) != 2 {
		log.Printf("[Worker %d] ❌ Invalid email format: %s", workerID, job.Email)
		updateEmailStatus(job.JobID, job.Email, "INVALID", 550, "Invalid email format")
		return
	}
	domain := strings.ToLower(parts[1])

	// ============================================================
	// FEATURE 1: DOMAIN-SPECIFIC RATE LIMITING (The Governor)
	// ============================================================
	// Note: Global rate limit is already enforced in main loop
	// This is for domain-specific limits only
	if err := rateLimiter.Wait(ctx, domain); err != nil {
		log.Printf("[Worker %d] ❌ Rate limit wait cancelled: %v", workerID, err)
		return
	}

	// Perform SMTP check (with proxy config and worker hostname)
	result, err := CheckEmail(ctx, job.Email, isDevMode, proxyConfig, workerHostname)
	if err != nil {
		log.Printf("[Worker %d] ❌ SMTP check error for %s: %v", workerID, job.Email, err)
		updateEmailStatus(job.JobID, job.Email, "UNKNOWN", 0, err.Error())
		return
	}

	// ============================================================
	// FEATURE 2: GREYLISTING RETRY LOGIC
	// ============================================================
	// If the result is retryable (450, 451, 421), add to retry queue instead of updating DB
	if result.IsRetryable {
		log.Printf("[Worker %d] ⏳ Greylisted: %s (Code: %d) - Adding to retry queue", workerID, job.Email, result.SMTPCode)

		// Calculate retry timestamp (15 minutes from now)
		retryTime := time.Now().Unix() + retryDelay

		// Serialize job for ZSET
		jobJSON, err := json.Marshal(job)
		if err != nil {
			log.Printf("[Worker %d] ❌ Failed to serialize job for retry queue: %v", workerID, err)
			// Fallback: update DB with greylisted status
			updateEmailStatus(job.JobID, job.Email, string(result.Status), result.SMTPCode, result.BounceReason)
			return
		}

		// Add to Redis ZSET with score = retry timestamp
		err = redisClient.ZAdd(ctx, retryQueue, redis.Z{
			Score:  float64(retryTime),
			Member: string(jobJSON),
		}).Err()

		if err != nil {
			log.Printf("[Worker %d] ❌ Failed to add to retry queue: %v", workerID, err)
			// Fallback: update DB with greylisted status
			updateEmailStatus(job.JobID, job.Email, string(result.Status), result.SMTPCode, result.BounceReason)
			return
		}

		fmt.Printf("[Worker %d] ⏳ Queued for retry at %s: %s\n", workerID, time.Unix(retryTime, 0).Format(time.RFC3339), job.Email)
		return
	}

	// Not retryable - update database immediately
	err = updateEmailStatus(job.JobID, job.Email, string(result.Status), result.SMTPCode, result.BounceReason)
	if err != nil {
		log.Printf("[Worker %d] ❌ Database update error for %s: %v", workerID, job.Email, err)
		return
	}

	// Print result with emoji
	emoji := getStatusEmoji(result.Status)
	fmt.Printf("[Worker %d] %s %s: %s (Code: %d)\n", workerID, emoji, result.Status, job.Email, result.SMTPCode)
}

// updateEmailStatus updates the EmailCheck record in PostgreSQL
func updateEmailStatus(jobID, email, status string, smtpCode int, bounceReason string) error {
	query := `
		UPDATE "EmailCheck" 
		SET status = $1, 
		    "smtpCode" = $2, 
		    "bounceReason" = $3
		WHERE "jobId" = $4 AND email = $5
	`

	_, err := db.Exec(query, status, smtpCode, bounceReason, jobID, email)
	return err
}

// getStatusEmoji returns an emoji for the status
func getStatusEmoji(status EmailStatus) string {
	switch status {
	case StatusValid:
		return "✅"
	case StatusInvalid:
		return "❌"
	case StatusGreylisted:
		return "⏳"
	case StatusCatchAll:
		return "📬"
	case StatusUnknown:
		return "❓"
	default:
		return "❓"
	}
}

// ============================================================
// CRITICAL: STRICT EMAIL SYNTAX VALIDATION (RFC 5322 Compliant)
// ============================================================
// isValidEmailSyntax validates email syntax using strict RFC 5322 compliant regex
// Catches: double @, missing TLD, invalid characters, double dots, etc.
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)

func isValidEmailSyntax(email string) bool {
	// Basic length check
	if len(email) < 3 || len(email) > 254 {
		return false
	}

	// Check for double @
	if strings.Count(email, "@") != 1 {
		return false
	}

	// Split into local and domain parts
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}

	localPart := parts[0]
	domainPart := parts[1]

	// Validate local part (before @)
	if len(localPart) == 0 || len(localPart) > 64 {
		return false
	}

	// Check for double dots in local part
	if strings.Contains(localPart, "..") {
		return false
	}

	// Check for leading/trailing dots in local part
	if strings.HasPrefix(localPart, ".") || strings.HasSuffix(localPart, ".") {
		return false
	}

	// Validate domain part (after @)
	if len(domainPart) == 0 || len(domainPart) > 253 {
		return false
	}

	// Check for double dots in domain part
	if strings.Contains(domainPart, "..") {
		return false
	}

	// Check for leading/trailing dots in domain part
	if strings.HasPrefix(domainPart, ".") || strings.HasSuffix(domainPart, ".") {
		return false
	}

	// Must have at least one dot in domain (TLD required)
	if !strings.Contains(domainPart, ".") {
		return false
	}

	// Extract TLD (last part after last dot)
	domainParts := strings.Split(domainPart, ".")
	tld := domainParts[len(domainParts)-1]

	// TLD must be at least 2 characters and only letters
	if len(tld) < 2 {
		return false
	}

	// Use regex for final validation (RFC 5322 compliant)
	return emailRegex.MatchString(email)
}
