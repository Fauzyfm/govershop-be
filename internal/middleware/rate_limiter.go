package middleware

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// visitor tracks the request count and window for a single IP
type visitor struct {
	tokens    int
	lastReset time.Time
}

// RateLimiter implements a per-IP token bucket rate limiter
type RateLimiter struct {
	mu       sync.RWMutex
	visitors map[string]*visitor
	limit    int           // max requests per window
	window   time.Duration // time window
	stopCh   chan struct{}
}

// NewRateLimiter creates a new rate limiter with the given limit per window.
// It starts a background goroutine to clean up stale visitors every 5 minutes.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		limit:    limit,
		window:   window,
		stopCh:   make(chan struct{}),
	}

	// Background cleanup of stale entries to prevent memory leak
	go rl.cleanup()

	return rl
}

// cleanup removes visitors that haven't been seen for more than 2x the window
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			staleThreshold := time.Now().Add(-2 * rl.window)
			count := 0
			for ip, v := range rl.visitors {
				if v.lastReset.Before(staleThreshold) {
					delete(rl.visitors, ip)
					count++
				}
			}
			if count > 0 {
				log.Printf("[RateLimit] Cleaned up %d stale entries", count)
			}
			rl.mu.Unlock()
		case <-rl.stopCh:
			return
		}
	}
}

// getVisitor retrieves or creates a visitor for the given IP
func (rl *RateLimiter) getVisitor(ip string) *visitor {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	if !exists {
		v = &visitor{
			tokens:    rl.limit,
			lastReset: time.Now(),
		}
		rl.visitors[ip] = v
		return v
	}

	// Reset tokens if window has passed
	if time.Since(v.lastReset) > rl.window {
		v.tokens = rl.limit
		v.lastReset = time.Now()
	}

	return v
}

// allow checks if the IP is allowed to make a request
func (rl *RateLimiter) allow(ip string) bool {
	v := rl.getVisitor(ip)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	if v.tokens > 0 {
		v.tokens--
		return true
	}

	return false
}

// getClientIP extracts the real client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (first IP is the client)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can contain multiple IPs: client, proxy1, proxy2
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-Ip"); xri != "" {
		return xri
	}

	// Fallback to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// Limit wraps an http.HandlerFunc with rate limiting
func (rl *RateLimiter) Limit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r)

		if !rl.allow(ip) {
			w.Header().Set("Retry-After", "60")
			w.Header().Set("X-RateLimit-Limit", intToStr(rl.limit))
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusTooManyRequests)

			resp := map[string]interface{}{
				"success": false,
				"error":   "Terlalu banyak permintaan. Silakan coba lagi nanti.",
				"code":    "RATE_LIMIT_EXCEEDED",
			}
			json.NewEncoder(w).Encode(resp)

			log.Printf("[RateLimit] IP %s blocked on %s %s (limit: %d/%s)",
				ip, r.Method, r.URL.Path, rl.limit, rl.window)
			return
		}

		next.ServeHTTP(w, r)
	}
}

// LimitHandler wraps an http.Handler with rate limiting (for middleware chain)
func (rl *RateLimiter) LimitHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r)

		if !rl.allow(ip) {
			w.Header().Set("Retry-After", "60")
			w.Header().Set("X-RateLimit-Limit", intToStr(rl.limit))
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusTooManyRequests)

			resp := map[string]interface{}{
				"success": false,
				"error":   "Terlalu banyak permintaan. Silakan coba lagi nanti.",
				"code":    "RATE_LIMIT_EXCEEDED",
			}
			json.NewEncoder(w).Encode(resp)

			log.Printf("[RateLimit] IP %s blocked on %s %s (limit: %d/%s)",
				ip, r.Method, r.URL.Path, rl.limit, rl.window)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// intToStr converts an int to string without importing strconv
func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	negative := false
	if n < 0 {
		negative = true
		n = -n
	}
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	if negative {
		result = "-" + result
	}
	return result
}
