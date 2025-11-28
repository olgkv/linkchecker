package app

import (
	"fmt"
	"log/slog"
	"net/http"
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
	svc := service.New(st, client, cfg.MaxWorkers, cfg.HTTPTimeout)
	h := httpapi.NewHandler(svc, cfg.MaxLinks)

	var limiter *rate.Limiter
	if cfg.RateLimitRPS > 0 && cfg.RateLimitBurst > 0 {
		limiter = rate.NewLimiter(rate.Limit(cfg.RateLimitRPS), cfg.RateLimitBurst)
	}

	mux := http.NewServeMux()
	mux.Handle("/links", rateLimitMiddleware(limiter, loggingMiddleware(http.HandlerFunc(h.Links))))
	mux.Handle("/report", rateLimitMiddleware(limiter, loggingMiddleware(http.HandlerFunc(h.Report))))

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

func rateLimitMiddleware(limiter *rate.Limiter, next http.Handler) http.Handler {
	if limiter == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
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
