//go:build !windows

package main

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestPlatformConfig_Other(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "config.json")

	setConfigPathOverride(testPath)
	defer setConfigPathOverride("")

	// Initially empty
	cfg := readPlatformConfig()
	if cfg.Token != "" || cfg.ApiUrl != "" {
		t.Fatalf("expected empty config, got %+v", cfg)
	}

	// Save fields using platform functions
	if err := savePlatformToken("test-tok-999"); err != nil {
		t.Fatalf("savePlatformToken failed: %v", err)
	}
	if err := savePlatformName("PlatformAgent"); err != nil {
		t.Fatalf("savePlatformName failed: %v", err)
	}
	if err := savePlatformApiUrl("http://platform.example.com"); err != nil {
		t.Fatalf("savePlatformApiUrl failed: %v", err)
	}
	if err := savePlatformPrinterIp("192.168.1.50"); err != nil {
		t.Fatalf("savePlatformPrinterIp failed: %v", err)
	}
	printers := []string{"192.168.1.50", "192.168.1.51"}
	if err := savePlatformZplPrinters(printers); err != nil {
		t.Fatalf("savePlatformZplPrinters failed: %v", err)
	}
	if err := savePlatformPort(41790); err != nil {
		t.Fatalf("savePlatformPort failed: %v", err)
	}

	// Read back
	cfg = readPlatformConfig()
	if cfg.Token != "test-tok-999" {
		t.Errorf("Token mismatch: got %q", cfg.Token)
	}
	if cfg.AgentName != "PlatformAgent" {
		t.Errorf("AgentName mismatch: got %q", cfg.AgentName)
	}
	if cfg.ApiUrl != "http://platform.example.com" {
		t.Errorf("ApiUrl mismatch: got %q", cfg.ApiUrl)
	}
	if cfg.PrinterIp != "192.168.1.50" {
		t.Errorf("PrinterIp mismatch: got %q", cfg.PrinterIp)
	}
	if !slices.Equal(cfg.ZplPrinters, printers) {
		t.Errorf("ZplPrinters mismatch: got %v", cfg.ZplPrinters)
	}
	if cfg.LocalAddr != "127.0.0.1:41790" {
		t.Errorf("LocalAddr mismatch: got %q", cfg.LocalAddr)
	}
}

