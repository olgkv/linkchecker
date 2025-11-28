package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"webserver/internal/httpapi"
	"webserver/internal/service"
	"webserver/internal/storage"
)

type httpServer interface {
	ListenAndServe() error
	Shutdown(ctx context.Context) error
}

func runHTTPServer(ctx context.Context, srv httpServer) {
	go func() {
		log.Println("server listening on :8080")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Println("server shutdown error:", err)
	}
}

func main() {
	st := storage.NewFileStorage("tasks.json")
	if err := st.Load(); err != nil {
		log.Fatal("load storage:", err)
	}

	svc := service.New(st, &http.Client{Timeout: 5 * time.Second})
	h := httpapi.NewHandler(svc)

	mux := http.NewServeMux()
	mux.Handle("/links", loggingMiddleware(http.HandlerFunc(h.Links)))
	mux.Handle("/report", loggingMiddleware(http.HandlerFunc(h.Report)))

	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runHTTPServer(ctx, srv)

	total, completed := st.Stats()
	log.Printf("INFO shutdown summary: total_tasks=%d completed_tasks=%d", total, completed)
}

// loggingMiddleware логирует каждый HTTP-запрос в формате:
// timestamp, level, method, path, links_num, latency, status_code.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		lw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(lw, r)

		latency := time.Since(start)
		linksNum := r.Context().Value("links_num")
		if linksNum == nil {
			linksNum = 0
		}

		log.Printf("INFO method=%s path=%s links_num=%v latency_ms=%d status=%d", r.Method, r.URL.Path, linksNum, latency.Milliseconds(), lw.statusCode)
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lw *loggingResponseWriter) WriteHeader(code int) {
	lw.statusCode = code
	lw.ResponseWriter.WriteHeader(code)
}
