package app

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"webserver/internal/config"
	"webserver/internal/httpapi"
	"webserver/internal/service"
	"webserver/internal/storage"
)

// NewServer wires application dependencies and returns configured HTTP server and
// a stats function for graceful shutdown logging.
func NewServer(cfg *config.Config) (*http.Server, func() (int, int), error) {
	repo := storage.NewJSONRepository(cfg.TasksFile)
	st := storage.NewFileStorage(repo)
	if err := st.Load(); err != nil {
		return nil, nil, fmt.Errorf("load storage: %w", err)
	}

	client := &http.Client{Timeout: cfg.HTTPTimeout}
	svc := service.New(st, client, cfg.MaxWorkers, cfg.HTTPTimeout)
	h := httpapi.NewHandler(svc, cfg.MaxLinks)

	mux := http.NewServeMux()
	mux.Handle("/links", loggingMiddleware(http.HandlerFunc(h.Links)))
	mux.Handle("/report", loggingMiddleware(http.HandlerFunc(h.Report)))

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	statsFn := func() (int, int) {
		return st.Stats()
	}

	return srv, statsFn, nil
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		lw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(lw, r)

		latency := time.Since(start)
		log.Printf("INFO method=%s path=%s links_num=%d latency_ms=%d status=%d", r.Method, r.URL.Path, lw.linksNum, latency.Milliseconds(), lw.statusCode)
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

func (lw *loggingResponseWriter) SetLinksNum(id int) {
	lw.linksNum = id
}
