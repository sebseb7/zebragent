package main

import (
	"encoding/binary"
	"strconv"
	"strings"
)

// usbTarget is a parsed USB printer target string.
//
// Supported forms (prefix is case-insensitive):
//
//	usb, usb:, usb:auto   first USB printer found
//	usb:BUS:ADDR          explicit bus number / device address (e.g. usb:001:004)
//	usb:SERIAL            printer with the given USB serial number
//	usb://SERIAL          same as above
type usbTarget struct {
	auto     bool
	hasBus   bool
	bus, dev int
	serial   string
}

// IsUSBTarget checks whether a target string represents a USB printer.
func IsUSBTarget(target string) bool {
	_, ok := parseUSBTarget(target)
	return ok
}

func parseUSBTarget(target string) (usbTarget, bool) {
	clean := strings.TrimSpace(target)
	lower := strings.ToLower(clean)
	var rest string
	switch {
	case lower == "usb":
		return usbTarget{auto: true}, true
	case strings.HasPrefix(lower, "usb://"):
		rest = clean[len("usb://"):]
	case strings.HasPrefix(lower, "usb:"):
		rest = clean[len("usb:"):]
	default:
		return usbTarget{}, false
	}
	rest = strings.TrimSpace(rest)
	if rest == "" || strings.EqualFold(rest, "auto") {
		return usbTarget{auto: true}, true
	}
	if b, d, ok := strings.Cut(rest, ":"); ok {
		bus, err1 := strconv.Atoi(b)
		dev, err2 := strconv.Atoi(d)
		if err1 == nil && err2 == nil && bus >= 0 && dev >= 0 {
			return usbTarget{hasBus: true, bus: bus, dev: dev}, true
		}
	}
	return usbTarget{serial: rest}, true
}

// usbPrinterDev describes a USB device exposing a printer-class (0x07) interface.
type usbPrinterDev struct {
	Bus, Addr int
	Serial    string
	Product   string
	Iface     int
	EpIn      uint8 // 0 if the interface has no bulk IN endpoint
	EpOut     uint8
	MpsIn     int
}

// Target returns a stable target string for this device.
func (d usbPrinterDev) Target() string {
	if d.Serial != "" {
		return "usb://" + d.Serial
	}
	return "usb:" + strconv.Itoa(d.Bus) + ":" + strconv.Itoa(d.Addr)
}

func (d usbPrinterDev) matches(t usbTarget) bool {
	switch {
	case t.auto:
		return true
	case t.hasBus:
		return d.Bus == t.bus && d.Addr == t.dev
	default:
		return strings.EqualFold(d.Serial, t.serial)
	}
}

// parseUSBDescriptors walks a raw USB descriptor blob (as exposed by sysfs
// "descriptors") and returns the first printer-class interface that has a
// bulk OUT endpoint. ok is false when none exists.
func parseUSBDescriptors(b []byte) (dev usbPrinterDev, ok bool) {
	const (
		descInterface = 4
		descEndpoint  = 5
		classPrinter  = 7
	)
	inPrinter := false
	var cur usbPrinterDev
	flush := func() bool {
		if inPrinter && cur.EpOut != 0 {
			dev = cur
			return true
		}
		return false
	}
	for i := 0; i+2 <= len(b); {
		l := int(b[i])
		if l < 2 || i+l > len(b) {
			break
		}
		switch b[i+1] {
		case descInterface:
			if flush() {
				return dev, true
			}
			inPrinter = l >= 9 && b[i+5] == classPrinter
			cur = usbPrinterDev{Iface: int(b[i+2])}
		case descEndpoint:
			if inPrinter && l >= 7 && b[i+3]&0x03 == 0x02 { // bulk
				addr := b[i+2]
				if addr&0x80 != 0 {
					cur.EpIn = addr
					cur.MpsIn = int(binary.LittleEndian.Uint16(b[i+4:]) & 0x7ff)
				} else {
					cur.EpOut = addr
				}
			}
		}
		i += l
	}
	if flush() {
		return dev, true
	}
	return usbPrinterDev{}, false
}

