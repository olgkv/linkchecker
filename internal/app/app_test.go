package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimitMiddleware_PerIP(t *testing.T) {
    limiter := newIPRateLimiter(1, 1, time.Minute)
    var hits int
    inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        hits++
        w.WriteHeader(http.StatusOK)
    })
    h := rateLimitMiddleware(limiter, inner)

    req := httptest.NewRequest(http.MethodGet, "/links", nil)
    req.RemoteAddr = "1.1.1.1:1234"

    rec := httptest.NewRecorder()
    h.ServeHTTP(rec, req)
    if rec.Code != http.StatusOK {
        t.Fatalf("expected first request allowed, got %d", rec.Code)
    }

    rec2 := httptest.NewRecorder()
    h.ServeHTTP(rec2, req)
    if rec2.Code != http.StatusTooManyRequests {
        t.Fatalf("expected second request blocked, got %d", rec2.Code)
    }
    if hits != 1 {
        t.Fatalf("expected handler invoked once, got %d", hits)
    }
}

func TestRateLimitMiddleware_DifferentIPs(t *testing.T) {
    limiter := newIPRateLimiter(1, 1, time.Minute)
    inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    })
    h := rateLimitMiddleware(limiter, inner)

    req1 := httptest.NewRequest(http.MethodGet, "/links", nil)
    req1.RemoteAddr = "2.2.2.2:1000"
    rec1 := httptest.NewRecorder()
    h.ServeHTTP(rec1, req1)
    if rec1.Code != http.StatusOK {
        t.Fatalf("expected first IP allowed, got %d", rec1.Code)
    }

    req2 := httptest.NewRequest(http.MethodGet, "/links", nil)
    req2.RemoteAddr = "3.3.3.3:2000"
    rec2 := httptest.NewRecorder()
    h.ServeHTTP(rec2, req2)
    if rec2.Code != http.StatusOK {
        t.Fatalf("expected second IP allowed, got %d", rec2.Code)
    }
}

func TestClientIPExtraction(t *testing.T) {
    req := httptest.NewRequest(http.MethodGet, "/", nil)
    req.Header.Set("X-Forwarded-For", "10.0.0.1, 192.168.0.1")
    if ip := clientIP(req); ip != "10.0.0.1" {
        t.Fatalf("expected X-Forwarded-For ip, got %s", ip)
    }

    req.Header.Del("X-Forwarded-For")
    req.Header.Set("X-Real-IP", "172.16.0.5")
    if ip := clientIP(req); ip != "172.16.0.5" {
        t.Fatalf("expected X-Real-IP, got %s", ip)
    }

    req.Header.Del("X-Real-IP")
    req.RemoteAddr = "4.4.4.4:8080"
    if ip := clientIP(req); ip != "4.4.4.4" {
        t.Fatalf("expected RemoteAddr host, got %s", ip)
    }
}
