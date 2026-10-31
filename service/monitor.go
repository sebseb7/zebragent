package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// MonitorMessage is an event streamed to the Tauri monitor over local SSE.
type MonitorMessage struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Data      any    `json:"data"`
}

// MonitorServer provides a local HTTP and SSE API for the Tauri v2 monitor.
type MonitorServer struct {
	addr        string
	agent       *Agent
	subscribers sync.Map // chan MonitorMessage -> struct{}
}

func NewMonitorServer(addr string, agent *Agent) *MonitorServer {
	return &MonitorServer{
		addr:  addr,
		agent: agent,
	}
}

// Broadcast sends an event to all connected Tauri monitor clients.
func (m *MonitorServer) Broadcast(msgType string, data any) {
	msg := MonitorMessage{
		Type:      msgType,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Data:      data,
	}
	m.subscribers.Range(func(key, _ any) bool {
		ch := key.(chan MonitorMessage)
		select {
		case ch <- msg:
		default:
			// Client buffer full; skip to avoid blocking
		}
		return true
	})
}

// Start launches the local HTTP listener and shuts down when ctx is cancelled.
func (m *MonitorServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/events", m.handleSSE)
	mux.HandleFunc("/pair", m.handlePair)
	mux.HandleFunc("/unpair", m.handleUnpair)
	mux.HandleFunc("/config", m.handleConfig)
	mux.HandleFunc("/state", m.handleState)
	mux.HandleFunc("/print", m.handlePrint)

	// Wrap mux with CORS support for Tauri WebView
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, agentversion")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		mux.ServeHTTP(w, r)
	})

	targetAddr := m.addr
	if targetAddr == "" {
		targetAddr = "127.0.0.1:41782"
	}

	listener, err := net.Listen("tcp", targetAddr)
	if err != nil {
		logf("Local monitor could not bind to %s (%v); trying alternative ports...", targetAddr, err)
		host, _, _ := net.SplitHostPort(targetAddr)
		if host == "" {
			host = "127.0.0.1"
		}

		var altListener net.Listener
		for p := 41783; p <= 41799; p++ {
			altAddr := fmt.Sprintf("%s:%d", host, p)
			if l, altErr := net.Listen("tcp", altAddr); altErr == nil {
				altListener = l
				logf("Local monitor bound to alternative port %d", p)
				break
			}
		}
		if altListener == nil {
			l, osErr := net.Listen("tcp", host+":0")
			if osErr != nil {
				return fmt.Errorf("local monitor listen failed: %w", osErr)
			}
			altListener = l
		}
		listener = altListener
	}

	tcpAddr := listener.Addr().(*net.TCPAddr)
	boundPort := tcpAddr.Port
	boundAddr := fmt.Sprintf("127.0.0.1:%d", boundPort)
	m.addr = boundAddr
	if m.agent != nil {
		m.agent.mu.Lock()
		m.agent.cfg.LocalAddr = boundAddr
		m.agent.mu.Unlock()
	}

	// Always update the port in HKCU on launch to ensure controller connects
	_ = SavePort(boundPort)

	server := &http.Server{
		Addr:        m.addr,
		Handler:     handler,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}

	logf("Local monitor listening on http://%s", m.addr)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	err = server.Serve(listener)
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// handleSSE streams events to Tauri monitor over text/event-stream.
func (m *MonitorServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan MonitorMessage, 32)
	m.subscribers.Store(ch, struct{}{})
	defer m.subscribers.Delete(ch)

	// Push current agent state snapshot upon connecting
	snapshot := m.agent.GetSnapshot()
	initMsg, _ := json.Marshal(MonitorMessage{
		Type:      "snapshot",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Data:      snapshot,
	})
	fmt.Fprintf(w, "data: %s\n\n", initMsg)
	flusher.Flush()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case msg := <-ch:
			data, err := json.Marshal(msg)
			if err == nil {
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.Type, data)
				flusher.Flush()
			}
		}
	}
}

// handleConfig receives dynamic configuration updates (e.g. apiUrl, printerIp) from Tauri.
func (m *MonitorServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	apiUrl := strings.TrimSpace(r.URL.Query().Get("apiurl"))
	printerIp := strings.TrimSpace(r.URL.Query().Get("printerip"))

	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 2048))
		var req struct {
			ApiUrl    string `json:"apiurl"`
			PrinterIp string `json:"printerip"`
		}
		if err := json.Unmarshal(body, &req); err == nil {
			if req.ApiUrl != "" {
				apiUrl = req.ApiUrl
			}
			if req.PrinterIp != "" {
				printerIp = req.PrinterIp
			}
		}
	}

	if apiUrl != "" {
		_ = m.agent.SetApiUrl(apiUrl)
	}
	if printerIp != "" {
		_ = m.agent.SetPrinterIp(printerIp)
	}

	snap := m.agent.GetSnapshot()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success":   true,
		"apiUrl":    snap.ApiUrl,
		"printerIp": snap.PrinterIP,
	})
}

// handlePair receives pairing requests from Tauri UI or CLI.
func (m *MonitorServer) handlePair(w http.ResponseWriter, r *http.Request) {
	pin := strings.TrimSpace(r.URL.Query().Get("pairingpin"))
	apiUrl := strings.TrimSpace(r.URL.Query().Get("apiurl"))

	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 2048))
		var req struct {
			PairingPin string `json:"pairingpin"`
			ApiUrl     string `json:"apiurl"`
		}
		if err := json.Unmarshal(body, &req); err == nil {
			if req.PairingPin != "" {
				pin = req.PairingPin
			}
			if req.ApiUrl != "" {
				apiUrl = req.ApiUrl
			}
		}
	}

	if apiUrl != "" {
		_ = m.agent.SetApiUrl(apiUrl)
	}

	pin = strings.TrimSpace(pin)
	if pin == "" {
		http.Error(w, `{"error":"missing pairingpin"}`, http.StatusBadRequest)
		return
	}

	logf("Received pairingpin request: %s", pin)
	resp, err := m.agent.Pair(r.Context(), pin)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success":   true,
		"agentId":   resp.ID,
		"name":      resp.Name,
		"companyId": resp.CompanyID,
	})
}

// handleState returns current agent state in JSON.
func (m *MonitorServer) handleState(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(m.agent.GetSnapshot())
}

// handleUnpair clears stored token and returns to unpaired state.
func (m *MonitorServer) handleUnpair(w http.ResponseWriter, r *http.Request) {
	m.agent.Unpair()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": true, "paired": false})
}

// handlePrint allows submitting a test print from Tauri.
func (m *MonitorServer) handlePrint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req struct {
		ZPL string `json:"zpl"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.ZPL == "" {
		http.Error(w, `{"error":"missing zpl"}`, http.StatusBadRequest)
		return
	}

	id := fmt.Sprintf("test-%d", time.Now().Unix())
	m.agent.EnqueueJob(Job{ID: id, ZPL: req.ZPL})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": true, "jobId": id})
}
