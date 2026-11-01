package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	flagPairingPin   string
	flagUnpair       bool
	flagApiUrl       string
	flagPrinterIp    string
	flagToken        string
	flagLocalAddr    string
	flagClientSerial string
	flagClientModel  string
)

func initFlags() {
	flag.StringVar(&flagPairingPin, "pairingpin", "", "6-digit pairing PIN")
	flag.BoolVar(&flagUnpair, "unpair", false, "Clear stored token and enter pairing state")
	flag.StringVar(&flagApiUrl, "apiurl", "", "Backend API URL (e.g. https://api.example.com)")
	flag.StringVar(&flagPrinterIp, "printerip", "", "Printer IP address (TCP 9100)")
	flag.StringVar(&flagToken, "token", "", "Persistent agent token")
	flag.StringVar(&flagLocalAddr, "localaddr", "", "Local monitor address (default 127.0.0.1:41782)")
	flag.StringVar(&flagClientSerial, "clientserial", "", "Client serial override")
	flag.StringVar(&flagClientModel, "clientmodel", "", "Client model override")
}

func logf(format string, a ...any) { log.Printf(format, a...) }

type Job struct {
	ID             string
	BackendJobID   int
	AgentPrinterID int
	ZPL            string
}

type PairResponse struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	CompanyID    int    `json:"companyId"`
	Token        string `json:"token"`
	PairedAt     string `json:"pairedAt"`
	ClientSerial string `json:"clientserial"`
	ClientModel  string `json:"clientmodel"`
	AgentVersion string `json:"agentversion"`
}

type JobReport struct {
	ID        string    `json:"id"`
	State     string    `json:"state"`
	Detail    string    `json:"detail,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type AgentSnapshot struct {
	AgentVersion     string        `json:"agentVersion"`
	ApiUrl           string        `json:"apiUrl"`
	Paired           bool          `json:"paired"`
	AgentID          int           `json:"agentId,omitempty"`
	AgentName        string        `json:"agentName,omitempty"`
	BackendConnected bool          `json:"backendConnected"`
	PrinterIP        string        `json:"printerIp"`
	PrinterSerial    string        `json:"printerSerial,omitempty"`
	PrinterModel     string        `json:"printerModel,omitempty"`
	PrinterType      string        `json:"printerType"`
	PrinterReady     bool          `json:"printerReady"`
	PrinterStatus    string        `json:"printerStatus"`
	LabelLength      *int          `json:"labelLength"`
	Errors           PrinterErrors `json:"errors"`
	Fault            string        `json:"fault,omitempty"`
	LastJob          *JobReport    `json:"lastJob,omitempty"`
}

type Agent struct {
	cfg     *Config
	http    *http.Client
	monitor *MonitorServer

	mu               sync.Mutex
	stateMu          sync.Mutex
	jobs             chan Job
	seq              int
	paired           bool
	agentID          int
	agentName        string
	backendConnected bool
	lastJob          *JobReport
	lastFault        string
	paperStatus      string
	printerReady     bool
	lastPrinterInfo  PrinterInfo
	lastPostedBody   string
	lastPostedTime   time.Time

	activeJobsMu  sync.Mutex
	activeJobs    map[int]bool
	cancelledJobs map[int]time.Time
	completedJobs map[int]time.Time

	pairNotifier   chan struct{}
	configNotifier chan struct{}
}

func main() {
	log.SetFlags(log.LstdFlags)
	initFlags()
	flag.Parse()

	platformMain()
}

// sendPinToLocalService checks if the agent service is already running locally and delivers the PIN.
// Returns true if a background service processed it.
func sendPinToLocalService(pin string) bool {
	pin = strings.TrimSpace(pin)
	if pin == "" {
		return false
	}

	cfg, _ := LoadConfig(false)
	localAddr := "127.0.0.1:41782"
	if cfg != nil && cfg.LocalAddr != "" {
		localAddr = cfg.LocalAddr
	}

	payload, _ := json.Marshal(map[string]string{"pairingpin": pin, "apiurl": flagApiUrl})
	req, err := http.NewRequest("POST", "http://"+localAddr+"/pair", bytes.NewReader(payload))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false // Local service is not running
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 200 {
		fmt.Printf("Successfully paired via local running agent service: %s\n", strings.TrimSpace(string(b)))
		return true
	}
	fmt.Printf("Local running service returned error: %s\n", strings.TrimSpace(string(b)))
	return true
}

// consoleRun runs the agent in non-service mode until Ctrl+C.
func consoleRun() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return run(ctx, false)
}

// run is the agent body; it returns when ctx is cancelled.
func run(ctx context.Context, isService bool) error {
	cfg, err := LoadConfig(isService)
	if err != nil {
		return fmt.Errorf("config error: %w", err)
	}

	// In non-service mode, persist ApiUrl and PrinterIp if provided via flag
	if !isService && flagApiUrl != "" {
		_ = SaveApiUrl(cfg.ApiUrl)
	}
	if !isService && flagPrinterIp != "" {
		_ = SavePrinterIp(cfg.PrinterIp)
	}

	// Handle explicit -unpair request
	if !isService && flagUnpair {
		_ = SaveToken("")
		_ = SaveName("")
		cfg.Token = ""
		logf("Token cleared via -unpair flag.")
	}

	a := &Agent{
		cfg:            cfg,
		http:           &http.Client{Timeout: 15 * time.Second},
		jobs:           make(chan Job, 256),
		pairNotifier:   make(chan struct{}, 1),
		configNotifier: make(chan struct{}, 1),
		paired:         cfg.Token != "",
		agentName:      cfg.AgentName,
		paperStatus:    "",
		printerReady:   false,
		activeJobs:     make(map[int]bool),
		cancelledJobs:  make(map[int]time.Time),
		completedJobs:  make(map[int]time.Time),
	}

	// Start local HTTP / SSE monitor server for Tauri v2 & CLI
	a.monitor = NewMonitorServer(cfg.LocalAddr, a)
	go func() {
		if err := a.monitor.Start(ctx); err != nil && ctx.Err() == nil {
			logf("Local monitor server error: %v", err)
		}
	}()

	// Start background printer poller (^HH / ~HQES)
	go a.printerPoller(ctx)

	// If pairingpin was provided via CLI, attempt immediate pairing (re-pair)
	initialPin := strings.TrimSpace(flagPairingPin)
	if initialPin != "" && a.getApiUrl() != "" {
		logf("Pairing with provided pairingpin: %s...", initialPin)
		if res, err := a.Pair(ctx, initialPin); err == nil {
			logf("Direct pairing successful: Registered as %q (ID %d). Token saved to registry.", res.Name, res.ID)
		} else {
			logf("Pairing failed: %v", err)
		}
	}

	for ctx.Err() == nil {
		// 1. Wait for ApiUrl if not set
		for a.getApiUrl() == "" {
			logf("ZebraAgent started. ApiUrl not set. Waiting for ApiUrl via local monitor (http://%s) POST /config or /pair...", a.getLocalAddr())
			select {
			case <-ctx.Done():
				return nil
			case <-a.configNotifier:
				logf("ApiUrl configured: %s", a.getApiUrl())
			}
		}

		// 2. Wait for pairing PIN if not paired
		if !a.isPaired() {
			logf("ZebraAgent waiting for pairing PIN via local monitor (http://%s) or CLI...", a.getLocalAddr())
			select {
			case <-ctx.Done():
				return nil
			case <-a.configNotifier:
				continue
			case <-a.pairNotifier:
				logf("ZebraAgent paired successfully!")
			}
		}

		// Drain any stale notification signals before starting the active stream
		select {
		case <-a.configNotifier:
		default:
		}
		select {
		case <-a.pairNotifier:
		default:
		}

		// 3. Paired and ApiUrl set: take a thread-safe snapshot of Config, start worker, and stream events
		cfgSnapshot := a.getConfigSnapshot()
		workerCtx, cancelWorker := context.WithCancel(ctx)

		// Watch for runtime config/pair/unpair changes and restart the stream immediately
		go func() {
			select {
			case <-workerCtx.Done():
			case <-a.configNotifier:
				logf("Configuration updated; restarting event stream...")
				cancelWorker()
			case <-a.pairNotifier:
				logf("Pairing state updated; restarting event stream...")
				cancelWorker()
			}
		}()

		go a.worker(workerCtx)

		StreamEvents(workerCtx, &cfgSnapshot,
			func(connected bool) {
				a.mu.Lock()
				a.backendConnected = connected
				a.mu.Unlock()
				a.monitor.Broadcast("connection", map[string]any{"connected": connected})
			},
			func() {
				a.Unpair()
			},
			func(ev Event) {
				a.handle(workerCtx, ev)
			},
		)

		cancelWorker()
		if !a.isPaired() || a.getApiUrl() == "" {
			continue
		}
		if ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
			break
		case <-time.After(2 * time.Second):
		}
	}

	logf("shutting down")
	return nil
}

func (a *Agent) getConfigSnapshot() Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return *a.cfg
}

func (a *Agent) getLocalAddr() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.LocalAddr
}

func (a *Agent) isPaired() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.paired && a.cfg.Token != ""
}

// Unpair clears stored token, wakes any active stream, and returns to unpaired state.
func (a *Agent) Unpair() {
	a.mu.Lock()
	a.paired = false
	a.cfg.Token = ""
	a.agentID = 0
	a.agentName = ""
	a.backendConnected = false
	a.mu.Unlock()

	_ = SaveToken("")
	_ = SaveName("")

	// Wake stream watcher if currently connected
	select {
	case a.configNotifier <- struct{}{}:
	default:
	}

	if a.monitor != nil {
		a.monitor.Broadcast("unpaired", map[string]any{"reason": "unauthorized"})
	}
	logf("Agent unpaired (token cleared). Waiting for pairing PIN...")
}

func (a *Agent) getApiUrl() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.ApiUrl
}

// SetApiUrl sets and persists the backend API URL (domain only).
func (a *Agent) SetApiUrl(rawUrl string) error {
	clean := CleanApiUrl(rawUrl)
	if clean == "" {
		return errors.New("empty apiUrl")
	}

	a.mu.Lock()
	changed := a.cfg.ApiUrl != clean
	a.cfg.ApiUrl = clean
	a.mu.Unlock()

	_ = SaveApiUrl(clean)

	if changed {
		select {
		case a.configNotifier <- struct{}{}:
		default:
		}
	}

	if a.monitor != nil {
		a.monitor.Broadcast("config", map[string]any{"apiUrl": clean})
	}
	logf("Updated ApiUrl: %s", clean)
	return nil
}

func (a *Agent) getPrinterIp() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.PrinterIp
}

// SetPrinterIp sets and persists the printer IP address.
func (a *Agent) SetPrinterIp(ip string) error {
	clean := strings.TrimSpace(ip)
	if clean != "" && !IsValidPrinterTarget(clean) {
		return errors.New("invalid printer IP or Bluetooth address")
	}

	a.mu.Lock()
	a.cfg.PrinterIp = clean
	a.mu.Unlock()

	_ = SavePrinterIp(clean)

	if a.monitor != nil {
		a.monitor.Broadcast("config", map[string]any{"printerIp": clean})
	}
	logf("Updated PrinterIp: %s", clean)
	return nil
}

// Pair submits the PIN to /api/agent/pair, persists the token, and wakes the agent.
func (a *Agent) Pair(ctx context.Context, pin string) (*PairResponse, error) {
	cfgSnap := a.getConfigSnapshot()
	if cfgSnap.ApiUrl == "" {
		return nil, errors.New("cannot pair: ApiUrl is not configured")
	}

	reqUrl := cfgSnap.Endpoint(cfgSnap.PairPath)
	reqBody := map[string]string{
		"pairingpin":   pin,
		"clientserial": cfgSnap.ClientSerial,
		"clientmodel":  cfgSnap.ClientModel,
		"clienttype":   "zpl",
		"agentversion": AgentVersion,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST", reqUrl, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := a.http
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pairing request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pairing failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}

	var pairResp PairResponse
	if err := json.Unmarshal(respBytes, &pairResp); err != nil {
		return nil, fmt.Errorf("invalid pairing response: %w", err)
	}
	if pairResp.Token == "" {
		return nil, fmt.Errorf("pairing response did not contain token")
	}

	// Persist the token to HKLM
	if err := SaveToken(pairResp.Token); err != nil {
		logf("warning: failed to save token to registry: %v", err)
	}
	if pairResp.Name != "" {
		_ = SaveName(pairResp.Name)
	}

	a.mu.Lock()
	a.cfg.Token = pairResp.Token
	a.paired = true
	a.agentID = pairResp.ID
	a.agentName = pairResp.Name
	a.mu.Unlock()

	// Notify waiting run loop or active stream watcher
	select {
	case a.pairNotifier <- struct{}{}:
	default:
	}

	// Broadcast pairing success to local Tauri monitor
	if a.monitor != nil {
		a.monitor.Broadcast("paired", map[string]any{
			"agentId": pairResp.ID,
			"name":    pairResp.Name,
		})
	}

	logf("Successfully paired: Agent %q (ID: %d)", pairResp.Name, pairResp.ID)
	return &pairResp, nil
}

// GetSnapshot returns the current state of the agent for Tauri monitor.
func (a *Agent) GetSnapshot() AgentSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()

	st := a.lastPrinterInfo.Status
	if st == "" {
		st = "unpolled"
	}

	return AgentSnapshot{
		AgentVersion:     AgentVersion,
		ApiUrl:           a.cfg.ApiUrl,
		Paired:           a.paired && a.cfg.Token != "",
		AgentID:          a.agentID,
		AgentName:        a.agentName,
		BackendConnected: a.backendConnected,
		PrinterIP:        a.cfg.PrinterIp,
		PrinterSerial:    a.lastPrinterInfo.Serial,
		PrinterModel:     a.lastPrinterInfo.Model,
		PrinterType:      "zpl",
		PrinterReady:     a.printerReady,
		PrinterStatus:    st,
		LabelLength:      a.lastPrinterInfo.LabelLength,
		Errors:           a.lastPrinterInfo.Errors,
		Fault:            a.lastFault,
		LastJob:          a.lastJob,
	}
}

// EnqueueJob puts a job into the queue.
func (a *Agent) EnqueueJob(j Job) {
	a.reportState(j.ID, "queued", "")
	select {
	case a.jobs <- j:
	default:
		a.reportState(j.ID, "error", "agent queue full")
	}
}

// handle parses incoming SSE events from the backend.
func (a *Agent) handle(ctx context.Context, ev Event) {
	if ev.Event == "connected" {
		logf("Backend SSE event: connected, %s", ev.Data)

		var connData struct {
			AgentID int    `json:"agentId"`
			Name    string `json:"name"`
		}
		if err := json.Unmarshal([]byte(ev.Data), &connData); err == nil {
			a.mu.Lock()
			if connData.AgentID != 0 {
				a.agentID = connData.AgentID
			}
			if connData.Name != "" {
				a.agentName = connData.Name
				a.cfg.AgentName = connData.Name
			}
			nameToSave := a.agentName
			a.mu.Unlock()

			if nameToSave != "" {
				_ = SaveName(nameToSave)
			}
		}

		go a.sendPrinterStateUpstream(ctx)
		go a.drainBacklog(ctx)
		return
	}

	// Handle "cancel" event with model, serial, and type parameters -> send "~JP" to printer
	if model, serial, pType, isCancel := parseCancelEvent(ev); isCancel {
		logf("Backend SSE event: cancel (model=%q serial=%q type=%q)", model, serial, pType)
		go a.handleCancel(model, serial, pType)
		return
	}

	// Handle real-time job notifications ("job", "printerjob", or payload containing action)
	if jobEv, ok := parseJobEvent(ev); ok {
		id, _ := jobEv.JobID.Int64()
		jobID := int(id)
		if strings.EqualFold(jobEv.Action, "cancelled") {
			logf("Backend SSE event: job cancelled (jobId=%d agentprinterId=%d)", jobID, jobEv.AgentPrinterID)
			a.cancelJob(jobID)
			return
		}
		if strings.EqualFold(jobEv.Action, "created") {
			logf("Backend SSE event: job created (jobId=%d agentprinterId=%d type=%q)", jobID, jobEv.AgentPrinterID, jobEv.Type)
			a.handleJobCreated(ctx, jobID, jobEv.AgentPrinterID, "")
			return
		}
	}

	trimmed := strings.TrimSpace(ev.Data)
	if trimmed == "" {
		return
	}

	// Parse "print <zpl>"
	cmd, arg, _ := strings.Cut(trimmed, " ")
	if strings.EqualFold(cmd, "print") {
		a.mu.Lock()
		id := ev.ID
		if id == "" {
			a.seq++
			id = fmt.Sprintf("local-%d-%d", time.Now().Unix(), a.seq)
		}
		a.mu.Unlock()
		a.EnqueueJob(Job{ID: id, ZPL: arg})
		return
	}

	logf("unknown message: %.80q", ev.Data)
}

// JobEvent represents the payload of a real-time job event from backend SSE.
type JobEvent struct {
	Action         string      `json:"action"`
	JobID          json.Number `json:"jobId"`
	AgentPrinterID int         `json:"agentprinterId"`
	Type           string      `json:"type"`
	State          string      `json:"state"`
}

// parseJobEvent parses job notification events (event "job", "printerjob", or payload containing action).
func parseJobEvent(ev Event) (JobEvent, bool) {
	isJobEvent := strings.EqualFold(ev.Event, "job") || strings.EqualFold(ev.Event, "printerjob")
	trimmed := strings.TrimSpace(ev.Data)

	if !isJobEvent && !strings.Contains(trimmed, `"action"`) {
		return JobEvent{}, false
	}

	var je JobEvent
	dec := json.NewDecoder(strings.NewReader(trimmed))
	if err := dec.Decode(&je); err == nil && je.Action != "" && je.JobID != "" {
		return je, true
	}
	return JobEvent{}, false
}

// parseCancelEvent checks if the SSE event represents a cancel request
// and extracts model, serial, and type parameters.
func parseCancelEvent(ev Event) (model, serial, printType string, isCancel bool) {
	isCancelEvent := strings.EqualFold(ev.Event, "cancel")
	trimmed := strings.TrimSpace(ev.Data)

	if !isCancelEvent && !strings.HasPrefix(strings.ToLower(trimmed), "cancel") {
		return "", "", "", false
	}

	payload := trimmed
	if strings.HasPrefix(strings.ToLower(payload), "cancel") {
		payload = strings.TrimSpace(payload[6:])
	}

	if payload == "" {
		return "", "", "", true
	}

	// Try JSON payload: e.g. {"model":"ZD410-200dpi","serial":"50J203002218","type":"zpl"}
	var p struct {
		Model         string `json:"model"`
		Serial        string `json:"serial"`
		Type          string `json:"type"`
		ClientModel   string `json:"clientmodel"`
		ClientSerial  string `json:"clientserial"`
		ClientType    string `json:"clienttype"`
		PrinterModel  string `json:"printerModel"`
		PrinterSerial string `json:"printerSerial"`
		PrinterType   string `json:"printerType"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err == nil {
		m := p.Model
		if m == "" {
			m = p.ClientModel
		}
		if m == "" {
			m = p.PrinterModel
		}
		s := p.Serial
		if s == "" {
			s = p.ClientSerial
		}
		if s == "" {
			s = p.PrinterSerial
		}
		t := p.Type
		if t == "" {
			t = p.ClientType
		}
		if t == "" {
			t = p.PrinterType
		}
		return m, s, t, true
	}

	// Try URL query format: e.g. model=ZD410-200dpi&serial=50J203002218&type=zpl
	if q, err := url.ParseQuery(payload); err == nil && (q.Has("model") || q.Has("serial") || q.Has("type")) {
		return q.Get("model"), q.Get("serial"), q.Get("type"), true
	}

	// Try space-separated tokens: e.g. "ZD410-200dpi 50J203002218 zpl"
	parts := strings.Fields(payload)
	if len(parts) >= 3 {
		return parts[0], parts[1], parts[2], true
	} else if len(parts) == 2 {
		return parts[0], parts[1], "", true
	} else if len(parts) == 1 {
		return "", parts[0], "", true
	}

	return "", "", "", true
}

// handleCancel verifies model/serial/type match and sends "~JP" to the printer.
func (a *Agent) handleCancel(reqModel, reqSerial, reqType string) {
	ip := a.getPrinterIp()
	if ip == "" {
		logf("Cancel ignored: no printer IP configured")
		return
	}

	a.mu.Lock()
	currentModel := a.lastPrinterInfo.Model
	currentSerial := a.lastPrinterInfo.Serial
	a.mu.Unlock()

	// If unpolled, query the printer to verify
	if currentModel == "" && currentSerial == "" {
		info := QueryPrinterInfo(ip)
		a.mu.Lock()
		a.lastPrinterInfo = info
		currentModel = info.Model
		currentSerial = info.Serial
		a.mu.Unlock()
	}

	// If type parameter is provided, it must match "zpl"
	if reqType != "" && !strings.EqualFold(reqType, "zpl") {
		logf("Cancel ignored: type mismatch (event requested %q, printer has \"zpl\")", reqType)
		return
	}

	// If serial parameter is provided, it must match our printer
	if reqSerial != "" && currentSerial != "" && !strings.EqualFold(reqSerial, currentSerial) {
		logf("Cancel ignored: serial mismatch (event requested %q, printer has %q)", reqSerial, currentSerial)
		return
	}

	// If model parameter is provided, it must match our printer
	if reqModel != "" && currentModel != "" && !strings.EqualFold(reqModel, currentModel) {
		logf("Cancel ignored: model mismatch (event requested %q, printer has %q)", reqModel, currentModel)
		return
	}

	logf("Executing cancel (model=%q serial=%q type=\"zpl\"): sending ~JP to printer %s", reqModel, reqSerial, ip)
	if err := sendZPL(ip, "~JP"); err != nil {
		logf("Failed to send ~JP to printer %s: %v", ip, err)
		return
	}
	logf("Successfully sent ~JP to printer %s", ip)

	if a.monitor != nil {
		a.monitor.Broadcast("cancel", map[string]any{
			"model":  reqModel,
			"serial": reqSerial,
			"type":   "zpl",
			"status": "cancelled",
		})
	}
}

func (a *Agent) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-a.jobs:
			if j.BackendJobID > 0 {
				a.runBackendJob(ctx, j)
			} else {
				a.runJob(ctx, j)
			}
		}
	}
}

// runJob sends the ZPL then polls the printer (~HS) until idle, reporting state changes.
func (a *Agent) runJob(ctx context.Context, j Job) {
	ip := a.getPrinterIp()
	if ip == "" {
		a.reportState(j.ID, "error", "no printer IP configured")
		return
	}
	if err := sendZPL(ip, j.ZPL); err != nil {
		a.reportState(j.ID, "error", "send failed: "+err.Error())
		return
	}
	a.reportState(j.ID, "sent", "")

	last := ""
	failures := 0
	deadline := time.Now().Add(5 * time.Minute)
	sawBusy := false
	idleSince := time.Time{}
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
		}
		st, err := QueryStatus(ip)
		if err != nil {
			if failures++; failures >= 5 {
				a.reportState(j.ID, "error", "status poll failed: "+err.Error())
				return
			}
			continue
		}
		failures = 0

		a.mu.Lock()
		if st.PaperOut {
			a.paperStatus = "out"
		} else {
			a.paperStatus = "ok"
		}
		a.lastFault = st.Fault()
		a.printerReady = true
		a.mu.Unlock()

		if f := st.Fault(); f != "" {
			if last != "fault:"+f {
				a.reportState(j.ID, "fault", f)
				last = "fault:" + f
			}
			continue
		}
		if st.Busy() {
			sawBusy = true
			idleSince = time.Time{}
			if last != "printing" {
				a.reportState(j.ID, "printing", fmt.Sprintf("formats=%d labelsRemaining=%d", st.FormatsInBuffer, st.LabelsRemaining))
				last = "printing"
			}
			continue
		}
		// Idle. Require idle for 2s (or having seen busy) to avoid racing the printer's parse.
		if idleSince.IsZero() {
			idleSince = time.Now()
		}
		if sawBusy || time.Since(idleSince) >= 2*time.Second {
			a.reportState(j.ID, "done", "")
			return
		}
	}
	a.reportState(j.ID, "error", "timeout waiting for printer")
}

// runBackendJob executes a Dashback backend print job following the lifecycle guide.
func (a *Agent) runBackendJob(ctx context.Context, j Job) {
	jobID := j.BackendJobID

	// 1. Check if cancelled before execution
	if a.isJobCancelled(jobID) {
		logf("Job %d was cancelled before execution, discarding.", jobID)
		a.finishJob(jobID)
		return
	}

	// 2. Fetch payload if not provided
	zpl := j.ZPL
	if zpl == "" {
		fetched, err := a.fetchJobPayload(ctx, jobID)
		if err != nil {
			logf("Failed to fetch payload for job %d: %v", jobID, err)
			a.patchJobFailed(ctx, jobID)
			a.reportState(j.ID, "error", "fetch failed: "+err.Error())
			a.finishJob(jobID)
			return
		}
		zpl = fetched
	}

	// 3. Check again if cancelled during fetch
	if a.isJobCancelled(jobID) {
		logf("Job %d was cancelled during fetch, discarding.", jobID)
		a.finishJob(jobID)
		return
	}

	// 4. Mark processing
	a.markJobProcessing(ctx, jobID)
	a.reportState(j.ID, "processing", "")

	// 5. Write raw ZPL directly to target printer
	ip := a.getPrinterIp()
	if ip == "" {
		logf("Failed to execute job %d: no printer IP configured", jobID)
		a.patchJobFailed(ctx, jobID)
		a.reportState(j.ID, "error", "no printer IP configured")
		a.finishJob(jobID)
		return
	}

	logf("Writing print job %d (%d bytes) to printer %s...", jobID, len(zpl), ip)
	if err := sendZPL(ip, zpl); err != nil {
		logf("Failed to execute job %d: %v", jobID, err)
		a.patchJobFailed(ctx, jobID)
		a.reportState(j.ID, "error", "send failed: "+err.Error())
		a.finishJob(jobID)
		return
	}
	// 6. Mark completed
	a.markJobCompleted(ctx, jobID)
	a.reportState(j.ID, "completed", "")
	logf("Job %d successfully completed.", jobID)
	a.finishJob(jobID)

	select {
	case <-ctx.Done():
	case <-time.After(300 * time.Millisecond):
	}
}

func (a *Agent) isJobCancelled(jobID int) bool {
	a.activeJobsMu.Lock()
	_, cancelled := a.cancelledJobs[jobID]
	a.activeJobsMu.Unlock()
	return cancelled
}

func (a *Agent) handleJobCreated(ctx context.Context, jobID, agentPrinterID int, data string) {
	if jobID <= 0 {
		return
	}

	a.activeJobsMu.Lock()
	if a.activeJobs[jobID] {
		a.activeJobsMu.Unlock()
		return
	}
	if _, cancelled := a.cancelledJobs[jobID]; cancelled {
		a.activeJobsMu.Unlock()
		logf("Job %d was cancelled, skipping creation.", jobID)
		return
	}
	if _, completed := a.completedJobs[jobID]; completed {
		a.activeJobsMu.Unlock()
		return
	}
	a.activeJobs[jobID] = true
	a.activeJobsMu.Unlock()

	j := Job{
		ID:             strconv.Itoa(jobID),
		BackendJobID:   jobID,
		AgentPrinterID: agentPrinterID,
		ZPL:            data,
	}

	select {
	case a.jobs <- j:
	default:
		logf("Agent job queue full, cannot enqueue job %d", jobID)
		a.patchJobFailed(ctx, jobID)
		a.finishJob(jobID)
	}
}

func (a *Agent) cancelJob(jobID int) {
	now := time.Now()
	a.activeJobsMu.Lock()
	a.cancelledJobs[jobID] = now
	delete(a.activeJobs, jobID)
	for id, t := range a.cancelledJobs {
		if now.Sub(t) > 10*time.Minute {
			delete(a.cancelledJobs, id)
		}
	}
	a.activeJobsMu.Unlock()

	logf("Job %d cancelled by user.", jobID)
	if a.monitor != nil {
		a.monitor.Broadcast("job", map[string]any{
			"id":    strconv.Itoa(jobID),
			"state": "cancelled",
		})
	}
}

func (a *Agent) finishJob(jobID int) {
	now := time.Now()
	a.activeJobsMu.Lock()
	delete(a.activeJobs, jobID)
	a.completedJobs[jobID] = now
	for id, t := range a.completedJobs {
		if now.Sub(t) > 10*time.Minute {
			delete(a.completedJobs, id)
		}
	}
	for id, t := range a.cancelledJobs {
		if now.Sub(t) > 10*time.Minute {
			delete(a.cancelledJobs, id)
		}
	}
	a.activeJobsMu.Unlock()
}

func (a *Agent) fetchJobPayload(ctx context.Context, jobID int) (string, error) {
	cfgSnap := a.getConfigSnapshot()
	if cfgSnap.Token == "" {
		return "", errors.New("no auth token")
	}

	reqUrl := cfgSnap.Endpoint(fmt.Sprintf("jobs/%d", jobID))
	req, err := http.NewRequestWithContext(ctx, "GET", reqUrl, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+cfgSnap.Token)
	req.Header.Set("agentversion", AgentVersion)

	resp, err := a.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var res struct {
		Job struct {
			Data string `json:"data"`
		} `json:"job"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	return res.Job.Data, nil
}

func (a *Agent) markJobProcessing(ctx context.Context, jobID int) {
	cfgSnap := a.getConfigSnapshot()
	if cfgSnap.Token == "" {
		return
	}
	reqUrl := cfgSnap.Endpoint(fmt.Sprintf("jobs/%d/processing", jobID))
	req, err := http.NewRequestWithContext(ctx, "POST", reqUrl, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+cfgSnap.Token)
	req.Header.Set("agentversion", AgentVersion)

	resp, err := a.http.Do(req)
	if err != nil {
		logf("markJobProcessing %d error: %v", jobID, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		logf("markJobProcessing %d HTTP %d: %s", jobID, resp.StatusCode, strings.TrimSpace(string(b)))
	}
}

func (a *Agent) markJobCompleted(ctx context.Context, jobID int) {
	cfgSnap := a.getConfigSnapshot()
	if cfgSnap.Token == "" {
		return
	}
	reqUrl := cfgSnap.Endpoint(fmt.Sprintf("jobs/%d/completed", jobID))
	req, err := http.NewRequestWithContext(ctx, "POST", reqUrl, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+cfgSnap.Token)
	req.Header.Set("agentversion", AgentVersion)

	resp, err := a.http.Do(req)
	if err != nil {
		logf("markJobCompleted %d error: %v", jobID, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		logf("markJobCompleted %d HTTP %d: %s", jobID, resp.StatusCode, strings.TrimSpace(string(b)))
	}
}

func (a *Agent) patchJobFailed(ctx context.Context, jobID int) {
	cfgSnap := a.getConfigSnapshot()
	if cfgSnap.Token == "" {
		return
	}
	reqUrl := cfgSnap.Endpoint(fmt.Sprintf("jobs/%d", jobID))
	bodyBytes, _ := json.Marshal(map[string]string{"state": "failed"})
	req, err := http.NewRequestWithContext(ctx, "PATCH", reqUrl, bytes.NewReader(bodyBytes))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+cfgSnap.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("agentversion", AgentVersion)

	resp, err := a.http.Do(req)
	if err != nil {
		logf("patchJobFailed %d error: %v", jobID, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		logf("patchJobFailed %d HTTP %d: %s", jobID, resp.StatusCode, strings.TrimSpace(string(b)))
	}
}

func (a *Agent) drainBacklog(ctx context.Context) {
	cfgSnap := a.getConfigSnapshot()
	if cfgSnap.Token == "" {
		return
	}

	reqUrl := cfgSnap.Endpoint("jobs?state=pending")
	req, err := http.NewRequestWithContext(ctx, "GET", reqUrl, nil)
	if err != nil {
		logf("drainBacklog request error: %v", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+cfgSnap.Token)
	req.Header.Set("agentversion", AgentVersion)

	resp, err := a.http.Do(req)
	if err != nil {
		logf("drainBacklog HTTP error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		logf("drainBacklog HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
		return
	}

	var res struct {
		Jobs []struct {
			ID             int    `json:"id"`
			AgentPrinterID int    `json:"agentprinterId"`
			Type           string `json:"type"`
			State          string `json:"state"`
			Data           string `json:"data"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		logf("drainBacklog decode error: %v", err)
		return
	}

	if len(res.Jobs) > 0 {
		logf("Processing %d pending jobs from backlog...", len(res.Jobs))
		for _, j := range res.Jobs {
			a.handleJobCreated(ctx, j.ID, j.AgentPrinterID, j.Data)
		}
	}
}

// printerPoller continuously queries ^HH and ~HQES from the printer.
// It reports state upstream on initial discovery and whenever printer status changes.
func (a *Agent) printerPoller(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var (
		lastInfo      PrinterInfo
		hasPolledOnce bool
	)

	// Initial poll immediately
	a.checkAndReportPrinter(ctx, &lastInfo, &hasPolledOnce)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.checkAndReportPrinter(ctx, &lastInfo, &hasPolledOnce)
		}
	}
}

func equalIntPtr(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func (a *Agent) isBackendConnected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.backendConnected
}

func (a *Agent) getToken() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.Token
}

func (a *Agent) checkAndReportPrinter(ctx context.Context, last *PrinterInfo, hasPolledOnce *bool) {
	ip := a.getPrinterIp()
	if ip == "" {
		return
	}

	// Skip background poll tick if the printer socket is currently busy printing or cancelling
	info, ok := TryQueryPrinterInfo(ip)
	if !ok {
		return
	}

	a.mu.Lock()
	a.lastPrinterInfo = info
	a.printerReady = info.Ready
	if info.Errors.HeadOpen {
		a.lastFault = "head open"
	} else if info.Errors.Pause {
		a.lastFault = "paused"
	} else if info.Errors.Other {
		a.lastFault = "printer error"
	} else if info.Errors.Unreachable {
		a.lastFault = "unreachable"
	} else {
		a.lastFault = ""
	}
	a.mu.Unlock()

	// Notify local Tauri monitor over SSE
	if a.monitor != nil {
		a.monitor.Broadcast("printer", info)
	}

	isInitialPoll := !*hasPolledOnce
	changed := isInitialPoll ||
		last.IP != info.IP ||
		last.Ready != info.Ready ||
		last.Status != info.Status ||
		!equalIntPtr(last.LabelLength, info.LabelLength) ||
		last.Errors != info.Errors

	if changed {
		*hasPolledOnce = true
		*last = info

		labelLenStr := "null"
		if info.LabelLength != nil {
			labelLenStr = fmt.Sprintf("%d", *info.LabelLength)
		}
		logf("Printer [%s] status: %s (labelLength=%s headOpen=%v pause=%v other=%v unreachable=%v)",
			info.IP, info.Status, labelLenStr,
			info.Errors.HeadOpen, info.Errors.Pause, info.Errors.Other, info.Errors.Unreachable,
		)

		// On initial poll, state is reported via sendPrinterStateUpstream strictly after SSE connects.
		// Only report from background poller when an actual status change occurs.
		if !isInitialPoll && a.isPaired() && a.isBackendConnected() {
			go a.postStatePayload(ctx, "", "", "", false)
		}
	}
}

// sendPrinterStateUpstream sends the latest printer state to /api/agent/state after SSE connection.
func (a *Agent) sendPrinterStateUpstream(ctx context.Context) {
	ip := a.getPrinterIp()
	if ip == "" {
		return
	}

	a.mu.Lock()
	hasInfo := a.lastPrinterInfo.IP != ""
	a.mu.Unlock()

	// If printer has not been polled yet, poll it synchronously now
	if !hasInfo {
		info := QueryPrinterInfo(ip)
		a.mu.Lock()
		a.lastPrinterInfo = info
		a.printerReady = info.Ready
		a.mu.Unlock()
	}

	logf("Sending printer state upstream after SSE connection...")
	a.postStatePayload(ctx, "", "", "", true)
}

// reportPrinterState queries printer status and publishes state to /api/agent/state.
func (a *Agent) reportPrinterState(ctx context.Context) {
	ip := a.getPrinterIp()
	if ip == "" {
		return
	}
	info := QueryPrinterInfo(ip)
	a.mu.Lock()
	a.lastPrinterInfo = info
	a.printerReady = info.Ready
	a.mu.Unlock()

	a.postStatePayload(ctx, "", "", "", true)
}

// reportState updates local state, notifies local monitor SSE, and POSTs to /api/agent/state.
func (a *Agent) reportState(jobID, jobState, detail string) {
	logf("job %s: %s %s", jobID, jobState, detail)

	a.mu.Lock()
	a.lastJob = &JobReport{
		ID:        jobID,
		State:     jobState,
		Detail:    detail,
		UpdatedAt: time.Now().UTC(),
	}
	a.mu.Unlock()

	// Notify Tauri monitor over local SSE
	if a.monitor != nil {
		a.monitor.Broadcast("job", map[string]any{
			"jobId":  jobID,
			"state":  jobState,
			"detail": detail,
		})
	}

	// POST to upstream /api/agent/state (serialized by stateMu inside postStatePayload)
	go a.postStatePayload(context.Background(), jobID, jobState, detail, true)
}

// postStatePayload POSTs state to /api/agent/state according to the integration guide.
func (a *Agent) postStatePayload(ctx context.Context, jobID, jobState, detail string, force bool) {
	// Serialize outbound state POSTs so job state transitions cannot arrive out of order
	a.stateMu.Lock()
	defer a.stateMu.Unlock()

	a.mu.Lock()
	cfgSnap := *a.cfg
	backendConnected := a.backendConnected
	fault := a.lastFault
	info := a.lastPrinterInfo
	var jobCopy *JobReport
	if a.lastJob != nil {
		jc := *a.lastJob
		jobCopy = &jc
	}
	a.mu.Unlock()

	if cfgSnap.Token == "" || !backendConnected {
		return
	}

	printerObj := map[string]any{
		"serial":      info.Serial,
		"model":       info.Model,
		"type":        "zpl",
		"labelLength": info.LabelLength,
		"errors":      info.Errors,
	}

	stateMap := map[string]any{
		"printers": []any{printerObj},
	}
	if fault != "" {
		stateMap["fault"] = fault
	}
	if jobID != "" {
		stateMap["job"] = map[string]string{
			"id":     jobID,
			"state":  jobState,
			"detail": detail,
		}
	} else if jobCopy != nil {
		stateMap["job"] = map[string]string{
			"id":     jobCopy.ID,
			"state":  jobCopy.State,
			"detail": jobCopy.Detail,
		}
	}

	bodyObj := map[string]any{
		"state": stateMap,
	}

	body, err := json.Marshal(bodyObj)
	if err != nil {
		return
	}

	bodyStr := string(body)
	a.mu.Lock()
	if !force && bodyStr == a.lastPostedBody && time.Since(a.lastPostedTime) < 3*time.Second {
		a.mu.Unlock()
		return
	}
	a.lastPostedBody = bodyStr
	a.lastPostedTime = time.Now()
	a.mu.Unlock()

	reqUrl := cfgSnap.Endpoint(cfgSnap.StatePath) + "?agentversion=" + AgentVersion
	req, err := http.NewRequestWithContext(ctx, "POST", reqUrl, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+cfgSnap.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("agentversion", AgentVersion)

	resp, err := a.http.Do(req)
	if err != nil {
		logf("postState failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		logf("postState HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	} else {
		labelLenStr := "null"
		if info.LabelLength != nil {
			labelLenStr = fmt.Sprintf("%d", *info.LabelLength)
		}
		logf("Posted printer state upstream: HTTP %d (model=%q serial=%q type=%q labelLength=%s)", resp.StatusCode, info.Model, info.Serial, "zpl", labelLenStr)
	}
}