package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// rateLimiter is a simple per-IP fixed-window limiter used to keep the /test
// endpoint from being abused as a UDP-reflector trigger (§4).
type rateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string]*window
	now    func() time.Time
}

type window struct {
	start time.Time
	count int
}

func newRateLimiter(limit int, w time.Duration) *rateLimiter {
	return &rateLimiter{
		limit:  limit,
		window: w,
		hits:   make(map[string]*window),
		now:    time.Now,
	}
}

// allow reports whether a request from key may proceed, counting it if so.
func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := rl.now()
	wnd, ok := rl.hits[key]
	if !ok || now.Sub(wnd.start) >= rl.window {
		rl.hits[key] = &window{start: now, count: 1}
		// opportunistic cleanup of stale entries
		if len(rl.hits) > 1024 {
			for k, v := range rl.hits {
				if now.Sub(v.start) >= rl.window {
					delete(rl.hits, k)
				}
			}
		}
		return true
	}
	if wnd.count >= rl.limit {
		return false
	}
	wnd.count++
	return true
}

// middleware limits requests by client IP.
func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(clientIP(r)) {
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded; slow down")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP returns the request's source IP from the transport peer address
// only. We deliberately ignore client-supplied forwarding headers (and do not
// enable RealIP) so a caller cannot forge its address to pick a fresh
// rate-limit bucket. See the note in api.go routes().
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
