package main

import (
	"testing"
)

func TestIsBluetoothTarget(t *testing.T) {
	tests := []struct {
		target   string
		expected bool
	}{
		{"ble://34:14:B5:B3:89:4A", true},
		{"ble:34:14:B5:B3:89:4A", true},
		{"bt://34:14:B5:B3:89:4A", true},
		{"34:14:B5:B3:89:4A", true},
		{"34-14-B5-B3-89-4A", true},
		{"3414b5b3894a", true},
		{"ble", true},
		{"bluetooth", true},
		{"auto", true},
		{"192.168.1.50", false},
		{"10.0.0.1", false},
		{"", false},
		{"invalid-target", false},
	}

	for _, tt := range tests {
		actual := IsBluetoothTarget(tt.target)
		if actual != tt.expected {
			t.Errorf("IsBluetoothTarget(%q) = %v, expected %v", tt.target, actual, tt.expected)
		}
	}
}

func TestIsValidPrinterTarget(t *testing.T) {
	tests := []struct {
		target   string
		expected bool
	}{
		{"192.168.1.50", true},
		{"10.0.0.1", true},
		{"::1", true},
		{"ble://34:14:B5:B3:89:4A", true},
		{"34:14:B5:B3:89:4A", true},
		{"34-14-B5-B3-89-4A", true},
		{"ble", true},
		{"bluetooth", true},
		{"auto", true},
		{"", false},
		{"not-a-valid-target", false},
	}

	for _, tt := range tests {
		actual := IsValidPrinterTarget(tt.target)
		if actual != tt.expected {
			t.Errorf("IsValidPrinterTarget(%q) = %v, expected %v", tt.target, actual, tt.expected)
		}
	}
}

func TestResolveBLETarget(t *testing.T) {
	addr, err := resolveBLEAddress("ble")
	t.Logf("resolveBLEAddress(ble) result: addr=%s, err=%v", addr.String(), err)
	if err != nil {
		t.Fatalf("resolveBLEAddress failed: %v", err)
	}
}

func TestParsePrinterDataBLEResponse(t *testing.T) {
	data := "\"50J203002218\" \x02ZD410-200dpi,V84.20.21Z,8,8192KB\x03\r\nERRORS: 0 00000000 00000000\r\nWARNINGS: 0 00000000 00000000\r\n\"261\""
	info := parsePrinterData("34:14:B5:B3:89:4A", data)

	if info.Serial != "50J203002218" {
		t.Errorf("expected Serial=50J203002218, got %q", info.Serial)
	}
	if info.Model != "ZD410-200dpi" {
		t.Errorf("expected Model=ZD410-200dpi, got %q", info.Model)
	}
	if info.LabelLength == nil || *info.LabelLength != 261 {
		t.Errorf("expected LabelLength=261, got %v", info.LabelLength)
	}
	if !info.Ready {
		t.Errorf("expected Ready=true, got false")
	}
	if info.Status != "online" {
		t.Errorf("expected Status=online, got %q", info.Status)
	}
}

func TestBLEPrinterOperations(t *testing.T) {
	target := "34:14:B5:B3:89:4A"

	// 1. Test QueryPrinterInfo over BLE
	info := QueryPrinterInfo(target)
	if info.Status == "offline" {
		t.Skip("Live Zebra BLE printer not connected or unreachable; skipping live hardware test")
	}
	t.Logf("BLE Query Result: Model=%s, Serial=%s, Ready=%v, Status=%s",
		info.Model, info.Serial, info.Ready, info.Status)

	if info.Serial != "50J203002218" {
		t.Errorf("expected serial 50J203002218, got %q", info.Serial)
	}
	if info.Model != "ZD410-200dpi" {
		t.Errorf("expected model ZD410-200dpi, got %q", info.Model)
	}
	if info.Errors.Unreachable {
		t.Errorf("expected printer to be reachable")
	}

	// 2. Test QueryStatus (~HS) over the active BLE connection
	st, err := QueryStatus(target)
	if err != nil {
		t.Fatalf("QueryStatus failed: %v", err)
	}
	t.Logf("BLE ~HS Result: PaperOut=%v, Paused=%v, HeadUp=%v, RibbonOut=%v, FormatsInBuffer=%d, LabelsRemaining=%d",
		st.PaperOut, st.Paused, st.HeadUp, st.RibbonOut, st.FormatsInBuffer, st.LabelsRemaining)

	// 3. Test sendZPL over BLE
	if err := sendZPL(target, "~JP"); err != nil {
		t.Fatalf("sendZPL failed: %v", err)
	}
	t.Log("sendZPL succeeded over BLE")
}
