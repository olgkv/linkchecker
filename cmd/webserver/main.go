package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
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

type serviceWaiter interface {
	Wait()
}

func runHTTPServer(ctx context.Context, srv httpServer, svc serviceWaiter) {
	go func() {
		slog.Info("server listening", "addr", getAddr(srv))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server exited", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "err", err)
	}

	if svc != nil {
		svc.Wait()
	}
}

func getAddr(srv httpServer) string {
	if hs, ok := srv.(*http.Server); ok {
		return hs.Addr
	}
	return ""
}

func main() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{AddSource: true})
	slog.SetDefault(slog.New(handler))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	srv, svc, statsFn, err := app.NewServer(cfg)
	if err != nil {
		slog.Error("init server", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runHTTPServer(ctx, srv, svc)

	total, completed := statsFn()
	slog.Info("shutdown summary", "total_tasks", total, "completed_tasks", completed)
}
