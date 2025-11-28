package service

import "testing"

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		ok   bool
	}{
		{"empty", "", false},
		{"plain domain", "example.com", true},
		{"subdomain", "sub.example.com", true},
		{"with slash", "example.com/path", false},
		{"with query", "example.com?x=1", false},
		{"with fragment", "example.com#hash", false},
		{"with port", "example.com:8080", false},
		{"with scheme", "http://example.com", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := validateURL(tc.url)
			if got != tc.ok {
				t.Fatalf("validateURL(%q) = %v, want %v", tc.url, got, tc.ok)
			}
		})
	}
}
