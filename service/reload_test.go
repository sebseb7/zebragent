package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
)

func TestAgentReloadAndReconnect(t *testing.T) {
	origCfg, _ := LoadConfig(false)
	t.Cleanup(func() {
		if origCfg != nil {
			_ = SaveApiUrl(origCfg.ApiUrl)
			_ = SaveZplPrinters(origCfg.ZplPrinters)
		}
	})

	// 1. Initial config
	initialPrinters := []string{"192.168.1.50"}
	cfg := &Config{
		ApiUrl:      "http://initial-api.example.com",
		PrinterIp:   "192.168.1.50",
		ZplPrinters: initialPrinters,
		LocalAddr:   "127.0.0.1:0",
	}

	a := &Agent{
		cfg:             cfg,
		configNotifier:  make(chan struct{}, 1),
		printerNotifier: make(chan struct{}, 1),
		printersInfo:    make(map[string]PrinterInfo),
		http:            &http.Client{Timeout: 5 * time.Second},
	}

	if a.getApiUrl() != "http://initial-api.example.com" {
		t.Fatalf("expected initial ApiUrl, got %s", a.getApiUrl())
	}
	if !slices.Equal(a.getPrinters(), initialPrinters) {
		t.Fatalf("expected initial printers, got %v", a.getPrinters())
	}

	// 2. Save new configuration via SaveApiUrl and SaveZplPrinters
	newApiUrl := "http://updated-quixpos.example.com"
	newPrinters := []string{"192.168.1.100", "192.168.1.101"}
	if err := SaveApiUrl(newApiUrl); err != nil {
		t.Fatalf("SaveApiUrl failed: %v", err)
	}
	if err := SaveZplPrinters(newPrinters); err != nil {
		t.Fatalf("SaveZplPrinters failed: %v", err)
	}

	// 3. Trigger reload
	if err := a.ReloadFromRegistry(); err != nil {
		t.Fatalf("ReloadFromRegistry failed: %v", err)
	}

	// 4. Verify updated ApiUrl and printers
	if a.getApiUrl() != newApiUrl {
		t.Errorf("expected ApiUrl %q, got %q", newApiUrl, a.getApiUrl())
	}
	if !slices.Equal(a.getPrinters(), newPrinters) {
		t.Errorf("expected printers %v, got %v", newPrinters, a.getPrinters())
	}

	// 5. Verify notifications were fired
	select {
	case <-a.configNotifier:
		// success: backend stream reconnect was triggered
	case <-time.After(500 * time.Millisecond):
		t.Errorf("expected configNotifier to be signaled on ApiUrl change")
	}

	select {
	case <-a.printerNotifier:
		// success: printer reconnect was triggered
	case <-time.After(500 * time.Millisecond):
		t.Errorf("expected printerNotifier to be signaled on printer change")
	}
}

func TestMonitorServerReloadEndpoint(t *testing.T) {
	origCfg, _ := LoadConfig(false)
	t.Cleanup(func() {
		if origCfg != nil {
			_ = SaveApiUrl(origCfg.ApiUrl)
			_ = SaveZplPrinters(origCfg.ZplPrinters)
		}
	})

	newApi := "http://reload-endpoint-test.example.com"
	newPrinters := []string{"10.0.0.42"}
	_ = SaveApiUrl(newApi)
	_ = SaveZplPrinters(newPrinters)

	a := &Agent{
		cfg: &Config{
			ApiUrl:      "http://before-reload.example.com",
			PrinterIp:   "10.0.0.1",
			ZplPrinters: []string{"10.0.0.1"},
			LocalAddr:   "127.0.0.1:0",
		},
		configNotifier:  make(chan struct{}, 1),
		printerNotifier: make(chan struct{}, 1),
		printersInfo:    make(map[string]PrinterInfo),
		http:            &http.Client{Timeout: 5 * time.Second},
	}
	m := NewMonitorServer("127.0.0.1:0", a)

	req := httptest.NewRequest("POST", "/reload", nil)
	w := httptest.NewRecorder()
	m.handleReload(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200 from /reload, got %d", resp.StatusCode)
	}

	var res struct {
		Success     bool     `json:"success"`
		ApiUrl      string   `json:"apiUrl"`
		PrinterIp   string   `json:"printerIp"`
		ZplPrinters []string `json:"zplPrinters"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode /reload response: %v", err)
	}
	if !res.Success {
		t.Errorf("expected success=true")
	}
	if res.ApiUrl != newApi {
		t.Errorf("expected ApiUrl %q, got %q", newApi, res.ApiUrl)
	}
	if !slices.Equal(res.ZplPrinters, newPrinters) {
		t.Errorf("expected printers %v, got %v", newPrinters, res.ZplPrinters)
	}
}
