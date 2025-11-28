package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"webserver/internal/config"
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
		log.Println("server listening on", getAddr(srv))
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

func getAddr(srv httpServer) string {
	if hs, ok := srv.(*http.Server); ok {
		return hs.Addr
	}
	return ""
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("load config:", err)
	}

	st := storage.NewFileStorage(cfg.TasksFile)
	if err := st.Load(); err != nil {
		log.Fatal("load storage:", err)
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
