package server

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// visitor tracks a per-IP rate limiter and when it was last seen.
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter enforces per-IP request rate limits using a token-bucket algorithm.
type RateLimiter struct {
	mu       sync.RWMutex
	visitors map[string]*visitor
	rps      rate.Limit
	burst    int
}

// NewRateLimiter creates a RateLimiter that allows requestsPerMin requests per
// minute for each unique IP. Stale entries are cleaned up every minute.
func NewRateLimiter(requestsPerMin int) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rps:      rate.Limit(float64(requestsPerMin) / 60.0),
		burst:    requestsPerMin, // allow short bursts up to the per-minute cap
	}
	go rl.cleanupLoop()
	return rl
}

// Allow reports whether a request from ip is permitted.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	v, ok := rl.visitors[ip]
	if !ok {
		v = &visitor{limiter: rate.NewLimiter(rl.rps, rl.burst)}
		rl.visitors[ip] = v
	}
	v.lastSeen = time.Now()
	rl.mu.Unlock()
	return v.limiter.Allow()
}

// Middleware wraps an http.Handler and rejects requests that exceed the rate limit.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if !rl.Allow(ip) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// cleanupLoop removes visitors that have not been seen for 10 minutes.
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.lastSeen) > 10*time.Minute {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}
