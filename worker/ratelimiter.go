package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"golang.org/x/time/rate"
)

// RateLimiterManager manages global and per-domain rate limiters
type RateLimiterManager struct {
	globalLimiter *rate.Limiter
	domainLimiters map[string]*rate.Limiter
	mu            sync.RWMutex
}

// NewRateLimiterManager creates a new rate limiter manager with safety limits
func NewRateLimiterManager() *RateLimiterManager {
	// Global limit: 10 checks/second
	globalLimiter := rate.NewLimiter(10, 10) // 10 per second, burst of 10

	// Domain-specific limits
	domainLimiters := make(map[string]*rate.Limiter)
	
	// Gmail domains: 2 checks/second
	domainLimiters["gmail.com"] = rate.NewLimiter(2, 2)
	domainLimiters["googlemail.com"] = rate.NewLimiter(2, 2)
	
	// Outlook domains: 1 check/second
	domainLimiters["outlook.com"] = rate.NewLimiter(1, 1)
	domainLimiters["hotmail.com"] = rate.NewLimiter(1, 1)
	domainLimiters["live.com"] = rate.NewLimiter(1, 1)
	
	// Yahoo: 1 check/second
	domainLimiters["yahoo.com"] = rate.NewLimiter(1, 1)
	
	// Default: 5 checks/second (for other domains)
	// This will be created on-demand

	return &RateLimiterManager{
		globalLimiter:  globalLimiter,
		domainLimiters: domainLimiters,
	}
}

// Wait waits for both global and domain-specific rate limits
// Returns an error if context is cancelled
func (rlm *RateLimiterManager) Wait(ctx context.Context, domain string) error {
	// Normalize domain to lowercase
	domain = strings.ToLower(domain)
	
	// Wait for global limiter first
	if err := rlm.globalLimiter.Wait(ctx); err != nil {
		return err
	}
	
	// Get or create domain limiter
	rlm.mu.RLock()
	limiter, exists := rlm.domainLimiters[domain]
	rlm.mu.RUnlock()
	
	if !exists {
		// Create default limiter (5 checks/second)
		rlm.mu.Lock()
		// Double-check after acquiring write lock
		if limiter, exists = rlm.domainLimiters[domain]; !exists {
			limiter = rate.NewLimiter(5, 5) // 5 per second, burst of 5
			rlm.domainLimiters[domain] = limiter
		}
		rlm.mu.Unlock()
	}
	
	// Wait for domain limiter
	if err := limiter.Wait(ctx); err != nil {
		return err
	}
	
	// Log rate limit wait for sensitive domains
	if domain == "gmail.com" || domain == "googlemail.com" || 
	   domain == "outlook.com" || domain == "hotmail.com" || 
	   domain == "live.com" || domain == "yahoo.com" {
		log.Printf("⏳ Rate Limit Wait for [%s]", domain)
	}
	
	return nil
}

// GetDomainRate returns the current rate limit for a domain (for logging)
func (rlm *RateLimiterManager) GetDomainRate(domain string) string {
	domain = strings.ToLower(domain)
	
	rlm.mu.RLock()
	defer rlm.mu.RUnlock()
	
	if limiter, exists := rlm.domainLimiters[domain]; exists {
		limit := limiter.Limit()
		return fmt.Sprintf("%.1f/sec", float64(limit))
	}
	return "5.0/sec (default)"
}
