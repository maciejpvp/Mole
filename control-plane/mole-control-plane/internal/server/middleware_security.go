package server

import (
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/cors"
	"golang.org/x/time/rate"
)

// IPNetOrIP represents either a single IP address or an IP net subnet.
type IPNetOrIP struct {
	ip    net.IP
	ipNet *net.IPNet
}

func parseIPOrCIDR(entry string) (IPNetOrIP, bool) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return IPNetOrIP{}, false
	}

	if _, ipNet, err := net.ParseCIDR(entry); err == nil {
		return IPNetOrIP{ipNet: ipNet}, true
	}

	if ip := net.ParseIP(entry); ip != nil {
		return IPNetOrIP{ip: ip}, true
	}

	return IPNetOrIP{}, false
}

func (rule IPNetOrIP) Matches(ip net.IP) bool {
	if rule.ipNet != nil {
		return rule.ipNet.Contains(ip)
	}
	if rule.ip != nil {
		return rule.ip.Equal(ip)
	}
	return false
}

// GetClientIP extracts client IP address from request checking X-Forwarded-For, X-Real-IP, or RemoteAddr.
func GetClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		for _, ip := range ips {
			trimmed := strings.TrimSpace(ip)
			if parsed := net.ParseIP(trimmed); parsed != nil {
				return trimmed
			}
		}
	}

	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		if parsed := net.ParseIP(xri); parsed != nil {
			return xri
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// IPBlocker manages IP blacklisting and whitelisting.
type IPBlocker struct {
	blocked []IPNetOrIP
	allowed []IPNetOrIP
}

// NewIPBlocker constructs IPBlocker from comma-separated string rules.
func NewIPBlocker(blockedStr, allowedStr string) *IPBlocker {
	blocker := &IPBlocker{}

	if blockedStr != "" {
		for _, raw := range strings.Split(blockedStr, ",") {
			if rule, ok := parseIPOrCIDR(raw); ok {
				blocker.blocked = append(blocker.blocked, rule)
			}
		}
	}

	if allowedStr != "" {
		for _, raw := range strings.Split(allowedStr, ",") {
			if rule, ok := parseIPOrCIDR(raw); ok {
				blocker.allowed = append(blocker.allowed, rule)
			}
		}
	}

	return blocker
}

func (b *IPBlocker) IsBlocked(clientIPStr string) bool {
	ip := net.ParseIP(clientIPStr)
	if ip == nil {
		return false
	}

	// Check whitelist first if provided
	if len(b.allowed) > 0 {
		allowed := false
		for _, rule := range b.allowed {
			if rule.Matches(ip) {
				allowed = true
				break
			}
		}
		if !allowed {
			return true
		}
	}

	// Check blacklist
	for _, rule := range b.blocked {
		if rule.Matches(ip) {
			return true
		}
	}

	return false
}

// Handler middleware to enforce IP blockage.
func (b *IPBlocker) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := GetClientIP(r)
		if b.IsBlocked(clientIP) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "access denied"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// IPRateLimiter tracks per-IP rate limiters.
type IPRateLimiter struct {
	mu       sync.Mutex
	clients  map[string]*clientLimiter
	r        rate.Limit
	b        int
	stopChan chan struct{}
}

// NewIPRateLimiter initializes an in-memory rate limiter with periodic cleanup.
func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	limiter := &IPRateLimiter{
		clients:  make(map[string]*clientLimiter),
		r:        r,
		b:        b,
		stopChan: make(chan struct{}),
	}

	// Periodically cleanup idle limiters (older than 5 minutes)
	go func() {
		ticker := time.NewTicker(3 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				limiter.mu.Lock()
				for ip, cl := range limiter.clients {
					if time.Since(cl.lastSeen) > 5*time.Minute {
						delete(limiter.clients, ip)
					}
				}
				limiter.mu.Unlock()
			case <-limiter.stopChan:
				return
			}
		}
	}()

	return limiter
}

func (lim *IPRateLimiter) getLimiter(ip string) *rate.Limiter {
	lim.mu.Lock()
	defer lim.mu.Unlock()

	cl, exists := lim.clients[ip]
	if !exists {
		l := rate.NewLimiter(lim.r, lim.b)
		cl = &clientLimiter{limiter: l, lastSeen: time.Now()}
		lim.clients[ip] = cl
	} else {
		cl.lastSeen = time.Now()
	}

	return cl.limiter
}

// Handler middleware to enforce rate limiting per IP.
func (lim *IPRateLimiter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := GetClientIP(r)
		l := lim.getLimiter(ip)

		if !l.Allow() {
			w.Header().Set("Retry-After", "60")
			w.Header().Set("X-RateLimit-Limit", strconv.FormatFloat(float64(lim.r), 'f', -1, 64))
			w.Header().Set("X-RateLimit-Remaining", "0")
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// SecurityHeadersMiddleware adds standard security headers to all responses.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		next.ServeHTTP(w, r)
	})
}

// BuildCORSConfig generates production-safe CORS middleware options.
func BuildCORSConfig() cors.Options {
	rawOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
	var allowedOrigins []string

	if rawOrigins != "" {
		for _, o := range strings.Split(rawOrigins, ",") {
			trimmed := strings.TrimSpace(o)
			if trimmed != "" {
				allowedOrigins = append(allowedOrigins, trimmed)
			}
		}
	}

	// Fallback for local development if not configured
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"http://localhost:3000", "http://127.0.0.1:3000"}
	}

	return cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link", "X-RateLimit-Limit", "X-RateLimit-Remaining", "Retry-After"},
		AllowCredentials: true,
		MaxAge:           300,
	}
}

// MaxRequestBodySizeMiddleware limits maximum allowed HTTP request body size.
func MaxRequestBodySizeMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GetEnvInt parses integer env var with fallback default.
func GetEnvInt(key string, defaultValue int) int {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultValue
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return defaultValue
	}
	return val
}

// GetEnvInt64 parses int64 env var with fallback default.
func GetEnvInt64(key string, defaultValue int64) int64 {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultValue
	}
	val, err := strconv.ParseInt(valStr, 10, 64)
	if err != nil {
		return defaultValue
	}
	return val
}

// GetEnvFloat parses float env var with fallback default.
func GetEnvFloat(key string, defaultValue float64) float64 {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultValue
	}
	val, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return defaultValue
	}
	return val
}
