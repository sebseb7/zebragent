package main

import "testing"

func TestEndpoint(t *testing.T) {
	tests := []struct {
		apiUrl   string
		subpath  string
		expected string
	}{
		{"https://api.example.com", "/pair", "https://api.example.com/api/agent/pair"},
		{"https://api.example.com", "/events", "https://api.example.com/api/agent/events"},
		{"https://api.example.com", "/state", "https://api.example.com/api/agent/state"},
		{"https://api.example.com/", "/api/agent/pair", "https://api.example.com/api/agent/pair"},
		{"https://api.example.com/api/agent", "/pair", "https://api.example.com/api/agent/pair"},
		{"api.example.com", "/pair", "https://api.example.com/api/agent/pair"},
		{"localhost:3000", "/events", "http://localhost:3000/api/agent/events"},
	}

	for _, tc := range tests {
		cfg := &Config{ApiUrl: tc.apiUrl}
		got := cfg.Endpoint(tc.subpath)
		if got != tc.expected {
			t.Errorf("Endpoint(%q, %q) = %q, expected %q", tc.apiUrl, tc.subpath, got, tc.expected)
		}
	}
}

