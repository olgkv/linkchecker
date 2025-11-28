package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type fakeServer struct {
	listenCalled   int32
	shutdownCalled int32
	shutdownErr    error
}

func (f *fakeServer) ListenAndServe() error {
	atomic.AddInt32(&f.listenCalled, 1)
	// имитируем обычную работу сервера до отмены контекста
	time.Sleep(50 * time.Millisecond)
	return errors.New("server closed")
}

func (f *fakeServer) Shutdown(ctx context.Context) error {
	atomic.AddInt32(&f.shutdownCalled, 1)
	return f.shutdownErr
}

func TestRunHTTPServer_ShutdownCalledOnContextCancel(t *testing.T) {
	f := &fakeServer{}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// даём немного поработать ListenAndServe, затем отменяем контекст
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	runHTTPServer(ctx, f)

	if atomic.LoadInt32(&f.shutdownCalled) == 0 {
		t.Fatalf("expected Shutdown to be called")
	}
	if atomic.LoadInt32(&f.listenCalled) == 0 {
		t.Fatalf("expected ListenAndServe to be called")
	}
}
