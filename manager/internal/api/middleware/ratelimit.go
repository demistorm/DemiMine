package middleware

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

type RateLimiter struct {
	attempts  map[string]*Attempt
	skipPaths map[string]bool
	mu        sync.Mutex
	limit     int
	window    time.Duration
}

type Attempt struct {
	Count     int
	FirstSeen time.Time
}

func NewRateLimiter(limit int, window time.Duration, skipPaths []string) *RateLimiter {
	rl := &RateLimiter{
		attempts:  make(map[string]*Attempt),
		limit:     limit,
		window:    window,
		skipPaths: make(map[string]bool),
	}
	for _, p := range skipPaths {
		rl.skipPaths[p] = true
	}

	go rl.cleanup()

	return rl
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, attempt := range rl.attempts {
			if now.Sub(attempt.FirstSeen) > rl.window {
				delete(rl.attempts, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rl.skipPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}

		ip := getIP(r)

		rl.mu.Lock()
		attempt, exists := rl.attempts[ip]
		now := time.Now()

		if !exists || now.Sub(attempt.FirstSeen) > rl.window {
			rl.attempts[ip] = &Attempt{Count: 1, FirstSeen: now}
		} else {
			attempt.Count++
			if attempt.Count > rl.limit {
				rl.mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]string{
					"error": "too many requests",
				})
				return
			}
		}
		rl.mu.Unlock()

		next.ServeHTTP(w, r)
	})
}

func getIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.SplitN(forwarded, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	return r.RemoteAddr
}
