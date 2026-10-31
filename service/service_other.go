//go:build !windows

package main

import (
	"log"
	"strings"
)

func platformMain() {
	pin := strings.TrimSpace(flagPairingPin)
	if pin != "" {
		if sendPinToLocalService(pin) {
			return
		}
	}
	if err := consoleRun(); err != nil {
		log.Fatal(err)
	}
}
