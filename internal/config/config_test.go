package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("HTTP_TIMEOUT", "10s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Port != "9090" {
		t.Fatalf("expected port 9090, got %q", cfg.Port)
	}

	if cfg.HTTPTimeout.String() != "10s" {
		t.Fatalf("expected HTTP timeout 10s, got %s", cfg.HTTPTimeout)
	}
}
