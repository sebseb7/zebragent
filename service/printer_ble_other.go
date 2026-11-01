//go:build !windows

package main

import (
	"errors"
	"net"
	"strings"
)

// IsBluetoothTarget checks whether a target string represents a Bluetooth LE printer.
func IsBluetoothTarget(target string) bool {
	clean := strings.TrimSpace(strings.ToLower(target))
	if clean == "" {
		return false
	}
	if clean == "ble" || clean == "bluetooth" || clean == "auto" {
		return true
	}
	return strings.HasPrefix(clean, "ble://") || strings.HasPrefix(clean, "ble:") ||
		strings.HasPrefix(clean, "bt://") || strings.HasPrefix(clean, "bt:")
}

// IsValidPrinterTarget checks if target is a valid IPv4/IPv6 address or Bluetooth target.
func IsValidPrinterTarget(target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	if net.ParseIP(target) != nil {
		return true
	}
	return IsBluetoothTarget(target)
}

func queryBLEPrinterInfoLocked(target string) PrinterInfo {
	return PrinterInfo{
		IP:     target,
		Type:   "zpl",
		Status: "offline",
		Errors: PrinterErrors{
			Unreachable: true,
		},
	}
}

func sendZPLBLELocked(target, zpl string) error {
	return errors.New("direct BLE GATT is not supported on this platform")
}

func queryStatusBLELocked(target string) (HostStatus, error) {
	return HostStatus{}, errors.New("direct BLE GATT is not supported on this platform")
}

func disconnectBLE() {}


