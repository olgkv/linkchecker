package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"webserver/internal/app"
	"webserver/internal/config"
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

	srv, statsFn, err := app.NewServer(cfg)
	if err != nil {
		log.Fatal("init server:", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runHTTPServer(ctx, srv)

	total, completed := statsFn()
	slog.Info("shutdown summary", "total_tasks", total, "completed_tasks", completed)
}
