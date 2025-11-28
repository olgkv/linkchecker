package app

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"webserver/internal/config"
	"webserver/internal/httpapi"
	"webserver/internal/service"
	"webserver/internal/storage"

	"golang.org/x/time/rate"
)

// NewServer wires application dependencies and returns configured HTTP server,
// service instance, and a stats function for graceful shutdown logging.
func NewServer(cfg *config.Config) (*http.Server, *service.Service, func() (int, int), error) {
	repo := storage.NewJSONRepository(cfg.TasksFile)
	st := storage.NewFileStorage(repo)
	if err := st.Load(); err != nil {
		return nil, nil, nil, fmt.Errorf("load storage: %w", err)
	}

	client := &http.Client{Timeout: cfg.HTTPTimeout}
	svc := service.New(st, client, cfg.MaxWorkers, cfg.HTTPTimeout, cfg.ReportWorkers)
	h := httpapi.NewHandler(svc, cfg.MaxLinks)

	var ipLimiter *ipRateLimiter
	if cfg.RateLimitRPS > 0 && cfg.RateLimitBurst > 0 {
		ipLimiter = newIPRateLimiter(rate.Limit(cfg.RateLimitRPS), cfg.RateLimitBurst, 10*time.Minute)
	}

	mux := http.NewServeMux()
	mux.Handle("/links", rateLimitMiddleware(ipLimiter, loggingMiddleware(http.HandlerFunc(h.Links))))
	mux.Handle("/report", rateLimitMiddleware(ipLimiter, loggingMiddleware(http.HandlerFunc(h.Report))))

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	statsFn := func() (int, int) {
		return st.Stats()
	}

	return srv, svc, statsFn, nil
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		lw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(lw, r)
		if v := r.Context().Value(httpapi.LinksNumContextKey); v != nil {
			if id, ok := v.(int); ok {
				lw.linksNum = id
			}
		}

		latency := time.Since(start)
		slog.Info("request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"links_num", lw.linksNum,
			"latency_ms", latency.Milliseconds(),
			"status", lw.statusCode,
		)
	})
}


type ipRateLimiter struct {
	mu      sync.Mutex
	limit   rate.Limit
	burst   int
	ttl     time.Duration
	clients map[string]*ipLimiterEntry
}

type ipLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newIPRateLimiter(limit rate.Limit, burst int, ttl time.Duration) *ipRateLimiter {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &ipRateLimiter{
		limit:   limit,
		burst:   burst,
		ttl:     ttl,
		clients: make(map[string]*ipLimiterEntry),
	}
}

func (l *ipRateLimiter) allow(ip string) bool {
	if ip == "" {
		ip = "unknown"
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	if entry, ok := l.clients[ip]; ok {
		if now.Sub(entry.lastSeen) > l.ttl {
			delete(l.clients, ip)
		} else {
			entry.lastSeen = now
			return entry.limiter.Allow()
		}
	}

	limiter := rate.NewLimiter(l.limit, l.burst)
	l.clients[ip] = &ipLimiterEntry{limiter: limiter, lastSeen: now}

	for key, entry := range l.clients {
		if now.Sub(entry.lastSeen) > l.ttl {
			delete(l.clients, key)
		}
	}

	return limiter.Allow()
}

func rateLimitMiddleware(limiter *ipRateLimiter, next http.Handler) http.Handler {
	if limiter == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !limiter.allow(ip) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.Split(fwd, ",")
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	if rip := strings.TrimSpace(r.Header.Get("X-Real-IP")); rip != "" {
		return rip
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
	linksNum   int
}

func (lw *loggingResponseWriter) WriteHeader(code int) {
	lw.statusCode = code
	lw.ResponseWriter.WriteHeader(code)
}
