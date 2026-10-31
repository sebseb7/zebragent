package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSSEHeartbeatKeepalive(t *testing.T) {
	origTimeout := HeartbeatTimeout
	origInterval := HeartbeatInterval
	HeartbeatTimeout = 200 * time.Millisecond
	HeartbeatInterval = 50 * time.Millisecond
	defer func() {
		HeartbeatTimeout = origTimeout
		HeartbeatInterval = origInterval
	}()

	var heartbeatCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		// Send initial connected event
		fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
		flusher.Flush()

		// Send heartbeat every 50ms for 350ms (well over the 200ms timeout)
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; i < 7; i++ {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				fmt.Fprintf(w, ": heartbeat\n\n")
				flusher.Flush()
				heartbeatCount.Add(1)
			}
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()

	cfg := &Config{
		ApiUrl:     server.URL,
		Token:      "test-token",
		EventsPath: "/events",
	}

	var connChanges []bool
	StreamEvents(ctx, cfg, func(c bool) {
		connChanges = append(connChanges, c)
	}, nil, func(ev Event) {})

	if heartbeatCount.Load() < 4 {
		t.Errorf("expected at least 4 heartbeats sent and processed, got %d", heartbeatCount.Load())
	}
}

func TestSSEHeartbeatTimeoutAndReconnect(t *testing.T) {
	origTimeout := HeartbeatTimeout
	HeartbeatTimeout = 100 * time.Millisecond
	defer func() {
		HeartbeatTimeout = origTimeout
	}()

	var connectAttempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connectAttempts.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
			f.Flush()
		}
		// Stalls and sends NO heartbeat, triggering timeout after 100ms
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1400*time.Millisecond)
	defer cancel()

	cfg := &Config{
		ApiUrl:     server.URL,
		Token:      "test-token",
		EventsPath: "/events",
	}

	StreamEvents(ctx, cfg, nil, nil, func(ev Event) {})

	// In 350ms with 100ms timeout and 1s reconnect backoff, at least 1 timeout and subsequent attempt occurred
	if connectAttempts.Load() < 2 {
		t.Errorf("expected at least 2 connect attempts due to heartbeat timeout reconnect, got %d", connectAttempts.Load())
	}
}

func TestSSEReconnectLoopAtStart(t *testing.T) {
	// Find a free port but don't start server yet
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()

	var connectedSuccess atomic.Bool
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()

	cfg := &Config{
		ApiUrl:     "http://" + addr,
		Token:      "test-token",
		EventsPath: "/events",
	}

	// Start server after 500ms delay to simulate server starting up after agent
	go func() {
		time.Sleep(500 * time.Millisecond)
		mux := http.NewServeMux()
		mux.HandleFunc("/api/agent/events", func(w http.ResponseWriter, r *http.Request) {
			connectedSuccess.Store(true)
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			if f, ok := w.(http.Flusher); ok {
				fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
				f.Flush()
			}
			<-r.Context().Done()
		})
		s := &http.Server{Handler: mux}
		l, lErr := net.Listen("tcp", addr)
		if lErr == nil {
			go s.Serve(l)
			<-ctx.Done()
			_ = s.Close()
		}
	}()

	StreamEvents(ctx, cfg, nil, nil, func(ev Event) {})

	if !connectedSuccess.Load() {
		t.Errorf("expected agent to reconnect at start once server became available")
	}
}
