//go:build windows

package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows/svc"
)

// Service name the installer must register (binPath = this exe, no arguments).
const svcName = "ZebraAgent"

func platformMain() {
	isSvc, err := svc.IsWindowsService()
	if err != nil {
		log.Fatal(err)
	}
	if !isSvc {
		// Non-service mode
		pin := strings.TrimSpace(flagPairingPin)
		if pin != "" {
			// If background service is running, deliver PIN to it and exit
			if sendPinToLocalService(pin) {
				return
			}
		}
		// Otherwise run in the foreground console (pairs inline and continues running)
		if err := consoleRun(); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Service mode: strictly HKLM
	dir := filepath.Join(os.Getenv("ProgramData"), "ZebraAgent")
	_ = os.MkdirAll(dir, 0o755)
	if f, err := os.OpenFile(filepath.Join(dir, "agent.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		log.SetOutput(f)
	}
	if err := svc.Run(svcName, &handler{}); err != nil {
		log.Fatalf("service failed: %v", err)
	}
}

type handler struct{}

func (h *handler) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- run(ctx, true) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-done:
			// e.g. missing config: exit non-zero so SCM recovery actions apply.
			if err != nil {
				log.Printf("%v", err)
				return false, 1
			}
			return false, 0
		case c := <-req:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case <-done:
				case <-time.After(10 * time.Second):
				}
				return false, 0
			}
		}
	}
}
