package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// ErrUnauthorized indicates the backend returned HTTP 401.
type ErrUnauthorized struct {
	StatusCode int
	Body       string
}

func (e ErrUnauthorized) Error() string {
	return fmt.Sprintf("HTTP %d Unauthorized: %s", e.StatusCode, e.Body)
}

// Event is one SSE message.
type Event struct {
	ID    string
	Event string
	Data  string
}

var (
	// HeartbeatInterval is the frequency at which the backend emits ": heartbeat\n\n".
	HeartbeatInterval = 15 * time.Second
	// HeartbeatTimeout is the maximum duration to wait for a heartbeat or data frame before reconnecting.
	// 35s allows missing 1 heartbeat + network jitter before triggering reconnect.
	HeartbeatTimeout = 35 * time.Second
)

// StreamEvents connects to the SSE endpoint and reconnects with backoff until
// ctx is cancelled. Each received event is passed to handle.
func StreamEvents(ctx context.Context, cfg *Config, onConnChange func(bool), onUnauthorized func(), handle func(Event)) {
	lastID := ""
	backoff := time.Second
	everConnected := false

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}

	for ctx.Err() == nil {
		reqUrl := cfg.Endpoint(cfg.EventsPath)
		if everConnected {
			logf("SSE reconnecting to %s...", reqUrl)
		} else {
			logf("SSE connecting to %s...", reqUrl)
		}

		start := time.Now()
		connectedOnce := false
		err := readStream(ctx, client, cfg, &lastID, func(c bool) {
			if c {
				connectedOnce = true
				everConnected = true
				backoff = time.Second
			}
			if onConnChange != nil {
				onConnChange(c)
			}
		}, handle)

		if onConnChange != nil {
			onConnChange(false)
		}
		if ctx.Err() != nil {
			return
		}

		var unauth ErrUnauthorized
		if errors.As(err, &unauth) {
			logf("SSE authentication failed (%v). Token invalid or revoked.", err)
			if onUnauthorized != nil {
				onUnauthorized()
			}
			return
		}

		if connectedOnce && time.Since(start) > HeartbeatTimeout {
			backoff = time.Second // was a healthy connection
		}

		if !connectedOnce && !everConnected {
			logf("SSE initial connection failed: %v (retrying in %s)", err, backoff)
		} else {
			logf("SSE disconnected: %v (reconnecting in %s)", err, backoff)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}
}

func readStream(ctx context.Context, client *http.Client, cfg *Config, lastID *string, onConnected func(bool), handle func(Event)) error {
	reqUrl := cfg.Endpoint(cfg.EventsPath)
	parsedUrl, err := url.Parse(reqUrl)
	if err != nil {
		return err
	}
	q := parsedUrl.Query()
	q.Set("token", cfg.Token)
	q.Set("agentversion", AgentVersion)
	parsedUrl.RawQuery = q.Encode()

	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()

	req, err := http.NewRequestWithContext(streamCtx, "GET", parsedUrl.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("agentversion", AgentVersion)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	if *lastID != "" {
		req.Header.Set("Last-Event-ID", *lastID)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return ErrUnauthorized{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(b))}
	}

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	logf("SSE connected to %s", reqUrl)
	if onConnected != nil {
		onConnected(true)
	}

	// Watchdog: If no heartbeat or data packet arrives within HeartbeatTimeout,
	// close the stream and cancel context to trigger reconnection.
	var heartbeatTimedOut atomic.Bool
	timer := time.AfterFunc(HeartbeatTimeout, func() {
		heartbeatTimedOut.Store(true)
		logf("SSE heartbeat timeout: no heartbeat received for %v; issuing reconnect...", HeartbeatTimeout)
		cancelStream()
		_ = resp.Body.Close()
	})
	defer timer.Stop()

	r := bufio.NewReaderSize(resp.Body, 64*1024)
	var ev Event
	var data []string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if heartbeatTimedOut.Load() {
				return fmt.Errorf("heartbeat timeout (no heartbeat for %v)", HeartbeatTimeout)
			}
			return err
		}

		// Reset heartbeat watchdog on every received line (including ": heartbeat" comment frames)
		timer.Reset(HeartbeatTimeout)

		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "": // dispatch on blank line
			if len(data) > 0 || ev.Event != "" {
				ev.Data = strings.Join(data, "\n")
				if ev.ID != "" {
					*lastID = ev.ID
				}
				handle(ev)
			}
			ev, data = Event{}, nil
		case strings.HasPrefix(line, ":"):
			// SSE comment / keep-alive (e.g. ": heartbeat")
			// Silently ignored by EventSource specification, keeping connection active
		default:
			field, val, _ := strings.Cut(line, ":")
			val = strings.TrimPrefix(val, " ")
			switch field {
			case "data":
				data = append(data, val)
			case "id":
				ev.ID = val
			case "event":
				ev.Event = val
			}
		}
	}
}