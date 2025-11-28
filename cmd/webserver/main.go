package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"webserver/internal/httpapi"
	"webserver/internal/service"
	"webserver/internal/storage"
)

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

	go func() {
		log.Println("server listening on :8080")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Println("server shutdown error:", err)
	}
}
