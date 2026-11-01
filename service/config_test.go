package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

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

func TestFileConfigPath(t *testing.T) {
	// 1. In-memory override
	setConfigPathOverride("/tmp/test-override.json")
	defer setConfigPathOverride("")
	if got := getPlatformConfigPath(); got != "/tmp/test-override.json" {
		t.Fatalf("expected /tmp/test-override.json, got %q", got)
	}

	// 2. Environment variable override
	setConfigPathOverride("")
	t.Setenv("ZEBRA_CONFIG_PATH", "/custom/zebra/config.json")
	if got := getPlatformConfigPath(); got != "/custom/zebra/config.json" {
		t.Fatalf("expected /custom/zebra/config.json, got %q", got)
	}

	// 3. Default path uses XDG path ~/.config/zebragent/config.json
	t.Setenv("ZEBRA_CONFIG_PATH", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		got := getPlatformConfigPath()
		expectedXDG := filepath.Join(home, ".config", "zebragent", "config.json")
		expectedLegacyZebra := filepath.Join(home, ".zebraagent")
		expectedLegacyWhatever := filepath.Join(home, ".whatever")
		if got != expectedXDG && got != expectedLegacyZebra && got != expectedLegacyWhatever {
			t.Fatalf("expected valid config path in home dir, got %q", got)
		}
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("UserHomeDir not available")
	}

	if got := expandHome("~"); got != home {
		t.Errorf("expandHome(~) = %q, expected %q", got, home)
	}
	expected := filepath.Join(home, ".config", "zebragent", "config.json")
	if got := expandHome("~/.config/zebragent/config.json"); got != expected {
		t.Errorf("expandHome(~/.config/zebragent/config.json) = %q, expected %q", got, expected)
	}
	if got := expandHome("/absolute/path"); got != "/absolute/path" {
		t.Errorf("expandHome(/absolute/path) = %q, expected /absolute/path", got)
	}
}

func TestFileConfigReadWriteLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	// Initially non-existent file returns empty config
	c0 := readFileConfig(cfgPath)
	if c0.Token != "" || c0.ApiUrl != "" {
		t.Fatalf("expected empty config from non-existent file, got %+v", c0)
	}

	// Save Token
	if err := saveFileConfigFieldWithPath(cfgPath, "Token", "token-secret-12345678"); err != nil {
		t.Fatalf("save Token failed: %v", err)
	}

	// Save ApiUrl (should be normalized)
	if err := saveFileConfigFieldWithPath(cfgPath, "ApiUrl", "http://myapi.example.com/api/agent/"); err != nil {
		t.Fatalf("save ApiUrl failed: %v", err)
	}

	// Save Name
	if err := saveFileConfigFieldWithPath(cfgPath, "Name", "Warehouse-Printer-1"); err != nil {
		t.Fatalf("save Name failed: %v", err)
	}

	// Save ZplPrinters (should also set PrinterIp to first printer)
	printers := []string{"192.168.1.101", "192.168.1.102"}
	if err := saveFileConfigFieldWithPath(cfgPath, "ZplPrinters", printers); err != nil {
		t.Fatalf("save ZplPrinters failed: %v", err)
	}

	// Save Port (should update Port and LocalAddr)
	if err := saveFileConfigFieldWithPath(cfgPath, "Port", 41785); err != nil {
		t.Fatalf("save Port failed: %v", err)
	}

	// Read back and verify all fields
	cfg := readFileConfig(cfgPath)
	if cfg.Token != "token-secret-12345678" {
		t.Errorf("expected Token 'token-secret-12345678', got %q", cfg.Token)
	}
	if cfg.ApiUrl != "http://myapi.example.com" {
		t.Errorf("expected ApiUrl 'http://myapi.example.com', got %q", cfg.ApiUrl)
	}
	if cfg.AgentName != "Warehouse-Printer-1" {
		t.Errorf("expected AgentName 'Warehouse-Printer-1', got %q", cfg.AgentName)
	}
	if !slices.Equal(cfg.ZplPrinters, printers) {
		t.Errorf("expected ZplPrinters %v, got %v", printers, cfg.ZplPrinters)
	}
	if cfg.PrinterIp != "192.168.1.101" {
		t.Errorf("expected PrinterIp '192.168.1.101', got %q", cfg.PrinterIp)
	}
	if cfg.LocalAddr != "127.0.0.1:41785" {
		t.Errorf("expected LocalAddr '127.0.0.1:41785', got %q", cfg.LocalAddr)
	}

	// Test idempotent write: writing same value succeeds without error
	if err := saveFileConfigFieldWithPath(cfgPath, "Token", "token-secret-12345678"); err != nil {
		t.Fatalf("idempotent save failed: %v", err)
	}

	// Test clearing token on unpair
	if err := saveFileConfigFieldWithPath(cfgPath, "Token", ""); err != nil {
		t.Fatalf("clear token failed: %v", err)
	}
	cfgUnpaired := readFileConfig(cfgPath)
	if cfgUnpaired.Token != "" {
		t.Errorf("expected empty token after unpair, got %q", cfgUnpaired.Token)
	}
	// ApiUrl and printers should still remain
	if cfgUnpaired.ApiUrl != "http://myapi.example.com" {
		t.Errorf("expected ApiUrl to persist after unpair, got %q", cfgUnpaired.ApiUrl)
	}
}

func TestParseFileConfigVariants(t *testing.T) {
	// JSON with various casing and formats
	jsonPayload := `{
		"Token": "abc123456789",
		"name": "My Agent",
		"apiUrl": "https://agent.example.com",
		"zpl_printers": "10.0.0.1, 10.0.0.2",
		"port": "42000",
		"events_path": "/events",
		"state_path": "/state",
		"pair_path": "/pair",
		"client_serial": "SER123",
		"client_model": "ZT410"
	}`

	cfg, err := parseFileConfig("test.json", []byte(jsonPayload))
	if err != nil {
		t.Fatalf("parseFileConfig failed: %v", err)
	}

	if cfg.Token != "abc123456789" {
		t.Errorf("Token mismatch: got %q", cfg.Token)
	}
	if cfg.AgentName != "My Agent" {
		t.Errorf("AgentName mismatch: got %q", cfg.AgentName)
	}
	if cfg.ApiUrl != "https://agent.example.com" {
		t.Errorf("ApiUrl mismatch: got %q", cfg.ApiUrl)
	}
	expectedPrinters := []string{"10.0.0.1", "10.0.0.2"}
	if !slices.Equal(cfg.ZplPrinters, expectedPrinters) {
		t.Errorf("ZplPrinters mismatch: got %v", cfg.ZplPrinters)
	}
	if cfg.LocalAddr != "127.0.0.1:42000" {
		t.Errorf("LocalAddr mismatch: got %q", cfg.LocalAddr)
	}
	if cfg.EventsPath != "/events" || cfg.StatePath != "/state" || cfg.PairPath != "/pair" {
		t.Errorf("Paths mismatch: %+v", cfg)
	}
	if cfg.ClientSerial != "SER123" || cfg.ClientModel != "ZT410" {
		t.Errorf("Client metadata mismatch: %+v", cfg)
	}
}
