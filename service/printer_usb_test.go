package main

import "testing"

func TestParseUSBTarget(t *testing.T) {
	tests := []struct {
		in     string
		ok     bool
		auto   bool
		hasBus bool
		bus    int
		dev    int
		serial string
	}{
		{"usb", true, true, false, 0, 0, ""},
		{"USB:auto", true, true, false, 0, 0, ""},
		{"usb:", true, true, false, 0, 0, ""},
		{"usb:001:004", true, false, true, 1, 4, ""},
		{"usb://50J203002218", true, false, false, 0, 0, "50J203002218"},
		{"usb:ABC123", true, false, false, 0, 0, "ABC123"},
		{"192.168.1.5", false, false, false, 0, 0, ""},
		{"ble:auto", false, false, false, 0, 0, ""},
	}
	for _, tt := range tests {
		got, ok := parseUSBTarget(tt.in)
		if ok != tt.ok {
			t.Errorf("%q ok=%v want %v", tt.in, ok, tt.ok)
			continue
		}
		if ok && (got.auto != tt.auto || got.hasBus != tt.hasBus || got.bus != tt.bus || got.dev != tt.dev || got.serial != tt.serial) {
			t.Errorf("%q got %+v", tt.in, got)
		}
	}
}

func TestParseUSBDescriptors(t *testing.T) {
	desc := []byte{
		18, 1, 0, 2, 0, 0, 0, 64, 0x5f, 0x0a, 0x01, 0x00, 0, 1, 1, 2, 3, 1, // device
		9, 2, 32, 0, 1, 1, 0, 0x80, 50, // config
		9, 4, 0, 0, 2, 7, 1, 2, 0, // interface 0, printer class
		7, 5, 0x01, 0x02, 0x00, 0x02, 0, // bulk OUT 0x01
		7, 5, 0x82, 0x02, 0x00, 0x02, 0, // bulk IN 0x82 mps 512
	}
	d, ok := parseUSBDescriptors(desc)
	if !ok || d.Iface != 0 || d.EpOut != 0x01 || d.EpIn != 0x82 || d.MpsIn != 512 {
		t.Fatalf("unexpected result ok=%v dev=%+v", ok, d)
	}
	if _, ok := parseUSBDescriptors(desc[:27]); ok {
		t.Fatal("expected no printer interface")
	}
}

func TestIsValidPrinterTargetUSB(t *testing.T) {
	if !IsValidPrinterTarget("usb:auto") || !IsValidPrinterTarget("usb://ABC") {
		t.Error("usb targets should be valid")
	}
}

