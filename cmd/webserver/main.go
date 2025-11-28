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
	mux.Handle("/links", http.HandlerFunc(h.Links))
	mux.Handle("/report", http.HandlerFunc(h.Report))

	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runHTTPServer(ctx, srv)
}
