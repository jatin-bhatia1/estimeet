package api

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jatin-bhatia1/estimeet/backend/internal/domain"
	"github.com/jatin-bhatia1/estimeet/backend/internal/service"
)

type ctxKey int

const sessionKey ctxKey = iota

// sessionFrom returns the authenticated session attached by requireSession.
func sessionFrom(ctx context.Context) (service.Session, bool) {
	sess, ok := ctx.Value(sessionKey).(service.Session)
	return sess, ok
}

// bearerToken extracts the token from the Authorization header, or from the
// WebSocket subprotocol list (browsers cannot set headers on a WebSocket
// handshake, and putting the token in the query string would leak it into logs).
func bearerToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if after, ok := strings.CutPrefix(h, "Bearer "); ok {
			return strings.TrimSpace(after)
		}
	}
	for _, proto := range strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",") {
		if after, ok := strings.CutPrefix(strings.TrimSpace(proto), "bearer."); ok {
			return after
		}
	}
	return ""
}

// requireSession authenticates the caller and checks they belong to the room in the URL.
func (s *server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, err := s.svc.Authenticate(r.Context(), bearerToken(r))
		if err != nil {
			writeError(w, r, err)
			return
		}
		if code := chi.URLParam(r, "code"); code != "" {
			if !strings.EqualFold(sess.Room.Code, domain.NormalizeCode(code)) {
				writeError(w, r, domain.ErrForbidden)
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey, sess)))
	})
}

// securityHeaders sets conservative defaults for an API that also serves the SPA.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

// rateLimiter is a small per-IP token bucket protecting the unauthenticated
// endpoints (room creation and joining) from abuse.
type rateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	capacity float64
	refill   float64 // tokens per second
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(capacity float64, perMinute float64) *rateLimiter {
	rl := &rateLimiter{
		buckets:  make(map[string]*bucket),
		capacity: capacity,
		refill:   perMinute / 60,
	}
	go rl.janitor()
	return rl
}

func (rl *rateLimiter) janitor() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-30 * time.Minute)
		rl.mu.Lock()
		for k, b := range rl.buckets {
			if b.last.Before(cutoff) {
				delete(rl.buckets, k)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *rateLimiter) allow(key string) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok {
		rl.buckets[key] = &bucket{tokens: rl.capacity - 1, last: now}
		return true
	}
	b.tokens = min(rl.capacity, b.tokens+now.Sub(b.last).Seconds()*rl.refill)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "30")
			writeJSON(w, http.StatusTooManyRequests, errorBody{Error: "too many requests, please slow down"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	// RemoteAddr is the only value we can trust unless a proxy is explicitly
	// configured, so it is used as-is.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
