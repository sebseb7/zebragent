//go:build windows

package main

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows/registry"
	"tinygo.org/x/bluetooth"
)

var (
	zebraServiceUUID, _ = bluetooth.ParseUUID("38eb4a80-c570-11e3-9507-0002a5d5c51b")
	bleAdapter          = bluetooth.DefaultAdapter
	bleAdapterEnabled   bool
	bleAdapterMu        sync.Mutex

	bleMgr = &bleManager{}
)

// ensureBLEAdapter enables the default Bluetooth adapter once.
func ensureBLEAdapter() error {
	bleAdapterMu.Lock()
	defer bleAdapterMu.Unlock()
	if bleAdapterEnabled {
		return nil
	}
	if err := bleAdapter.Enable(); err != nil {
		return fmt.Errorf("enabling bluetooth adapter: %w", err)
	}
	bleAdapterEnabled = true
	return nil
}

// IsBluetoothTarget checks whether a target string represents a Bluetooth LE printer.
func IsBluetoothTarget(target string) bool {
	clean := strings.TrimSpace(strings.ToLower(target))
	if clean == "" {
		return false
	}
	if clean == "ble" || clean == "bluetooth" || clean == "auto" {
		return true
	}
	if strings.HasPrefix(clean, "ble://") || strings.HasPrefix(clean, "ble:") ||
		strings.HasPrefix(clean, "bt://") || strings.HasPrefix(clean, "bt:") {
		return true
	}
	// Check MAC address patterns
	if isMACAddress(clean) {
		return true
	}
	// Check if matches a known 12-char hex MAC without separators
	if len(clean) == 12 && isHexString(clean) {
		return true
	}
	return false
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
	return IsBluetoothTarget(target) || IsUSBTarget(target)
}

func isMACAddress(s string) bool {
	s = cleanTargetPrefix(s)
	_, err := net.ParseMAC(s)
	return err == nil
}

func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func cleanTargetPrefix(target string) string {
	s := strings.TrimSpace(target)
	s = strings.TrimPrefix(s, "ble://")
	s = strings.TrimPrefix(s, "ble:")
	s = strings.TrimPrefix(s, "bt://")
	s = strings.TrimPrefix(s, "bt:")
	return strings.TrimSpace(s)
}

// resolveBLEAddress turns a target string into a Bluetooth Address.
// If target is "auto", "ble", "bluetooth", or a serial/name, it scans for a matching Zebra device.
func resolveBLEAddress(target string) (bluetooth.Address, error) {
	if err := ensureBLEAdapter(); err != nil {
		return bluetooth.Address{}, err
	}

	clean := cleanTargetPrefix(target)
	lower := strings.ToLower(clean)

	// If clean is standard MAC e.g. 34:14:B5:B3:89:4A or 34-14-B5-B3-89-4A
	formattedMAC := strings.ToUpper(strings.ReplaceAll(clean, "-", ":"))
	if mac, err := bluetooth.ParseMAC(formattedMAC); err == nil {
		return bluetooth.Address{MACAddress: bluetooth.MACAddress{MAC: mac}}, nil
	}

	// 12-char hex string
	if len(clean) == 12 && isHexString(clean) {
		var parts [6]string
		for i := 0; i < 6; i++ {
			parts[i] = strings.ToUpper(clean[i*2 : i*2+2])
		}
		formattedMAC = strings.Join(parts[:], ":")
		if mac, err := bluetooth.ParseMAC(formattedMAC); err == nil {
			return bluetooth.Address{MACAddress: bluetooth.MACAddress{MAC: mac}}, nil
		}
	}

	// If target requests automatic discovery, check Windows paired BTHLE devices first
	if lower == "ble" || lower == "auto" || lower == "bluetooth" || lower == "" {
		if macStr, ok := findPairedZebraBLE(); ok {
			if mac, err := bluetooth.ParseMAC(macStr); err == nil {
				return bluetooth.Address{MACAddress: bluetooth.MACAddress{MAC: mac}}, nil
			}
		}
	}

	// Otherwise, scan for Zebra printer
	return scanForZebraPrinter(lower)
}

// findPairedZebraBLE inspects Windows registry for already paired Zebra BLE devices.
func findPairedZebraBLE() (string, bool) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Enum\BTHLE`, registry.READ)
	if err != nil {
		return "", false
	}
	defer k.Close()

	names, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return "", false
	}

	for _, name := range names {
		clean := strings.TrimPrefix(strings.ToLower(name), "dev_")
		if len(clean) == 12 && isHexString(clean) {
			var parts [6]string
			for i := 0; i < 6; i++ {
				parts[i] = strings.ToUpper(clean[i*2 : i*2+2])
			}
			mac := strings.Join(parts[:], ":")
			if strings.HasPrefix(mac, "34:14:B5") {
				return mac, true
			}
			// Also inspect child device keys for friendly name
			subKey, err := registry.OpenKey(k, name, registry.READ)
			if err == nil {
				subNames, _ := subKey.ReadSubKeyNames(-1)
				for _, sn := range subNames {
					devKey, err := registry.OpenKey(subKey, sn, registry.READ)
					if err == nil {
						fn, _, _ := devKey.GetStringValue("FriendlyName")
						devKey.Close()
						if strings.HasPrefix(strings.ToUpper(fn), "50J") || strings.Contains(strings.ToLower(fn), "zebra") {
							subKey.Close()
							return mac, true
						}
					}
				}
				subKey.Close()
			}
		}
	}
	return "", false
}

func scanForZebraPrinter(matcher string) (bluetooth.Address, error) {
	var (
		foundAddr bluetooth.Address
		found     bool
		scanMu    sync.Mutex
		doneOnce  sync.Once
	)

	stopScan := make(chan struct{})
	finish := func() {
		doneOnce.Do(func() {
			_ = bleAdapter.StopScan()
			close(stopScan)
		})
	}

	timer := time.AfterFunc(3*time.Second, finish)
	defer timer.Stop()

	err := bleAdapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
		name := strings.ToLower(result.LocalName())
		addrStr := result.Address.String()

		isMatch := false
		if matcher == "auto" || matcher == "ble" || matcher == "bluetooth" || matcher == "" {
			// Look for Zebra service UUID, Zebra name, or known Zebra OUI / serial pattern
			for _, u := range result.ServiceUUIDs() {
				if u == zebraServiceUUID {
					isMatch = true
					break
				}
			}
			if !isMatch && (strings.Contains(name, "zebra") ||
				strings.HasPrefix(strings.ToUpper(addrStr), "34:14:B5") ||
				(len(name) >= 10 && strings.HasPrefix(name, "50j"))) {
				isMatch = true
			}
		} else {
			if strings.EqualFold(name, matcher) || strings.EqualFold(addrStr, matcher) {
				isMatch = true
			}
		}

		if isMatch {
			scanMu.Lock()
			if !found {
				found = true
				foundAddr = result.Address
				go finish()
			}
			scanMu.Unlock()
		}
	})

	<-stopScan
	if err != nil && !found {
		return bluetooth.Address{}, fmt.Errorf("bluetooth scan error: %w", err)
	}

	if !found {
		return bluetooth.Address{}, fmt.Errorf("no Zebra BLE printer found matching %q", matcher)
	}
	return foundAddr, nil
}

type bleManager struct {
	mu           sync.Mutex
	target       string
	resolvedAddr bluetooth.Address
	device       bluetooth.Device
	writeChar    bluetooth.DeviceCharacteristic
	readChar     bluetooth.DeviceCharacteristic
	connected    bool

	recvMu   sync.Mutex
	recvBuf  strings.Builder
	notifyCh chan struct{}
}

func disconnectBLE() {
	bleMgr.mu.Lock()
	defer bleMgr.mu.Unlock()
	if bleMgr.connected {
		_ = bleMgr.device.Disconnect()
		bleMgr.connected = false
		bleMgr.target = ""
	}
}

func (m *bleManager) ensureConnected(target string) error {

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.connected && m.target == target {
		return nil
	}

	// Disconnect existing if changing target or reconnecting
	if m.connected {
		_ = m.device.Disconnect()
		m.connected = false
	}

	addr, err := resolveBLEAddress(target)
	if err != nil {
		return err
	}

	device, err := bleAdapter.Connect(addr, bluetooth.ConnectionParams{
		ConnectionTimeout: bluetooth.NewDuration(8 * time.Second),
	})
	if err != nil {
		return fmt.Errorf("connect to %s: %w", addr.String(), err)
	}
	time.Sleep(400 * time.Millisecond)

	services, err := device.DiscoverServices([]bluetooth.UUID{zebraServiceUUID})
	if err != nil || len(services) == 0 {
		_ = device.Disconnect()
		return fmt.Errorf("discovering zebra service on %s: %w", addr.String(), err)
	}

	var chars []bluetooth.DeviceCharacteristic
	for attempt := 0; attempt < 3; attempt++ {
		chars, err = services[0].DiscoverCharacteristics(nil)
		if err == nil && len(chars) > 0 {
			break
		}
		time.Sleep(400 * time.Millisecond)
	}
	if err != nil || len(chars) == 0 {
		_ = device.Disconnect()
		return fmt.Errorf("discovering characteristics on %s: %w", addr.String(), err)
	}

	var readChar, writeChar bluetooth.DeviceCharacteristic
	for _, c := range chars {
		u := strings.ToLower(c.UUID().String())
		if strings.HasPrefix(u, "38eb4a81") {
			readChar = c
		} else if strings.HasPrefix(u, "38eb4a82") {
			writeChar = c
		}
	}

	if readChar.UUID() == (bluetooth.UUID{}) || writeChar.UUID() == (bluetooth.UUID{}) {
		_ = device.Disconnect()
		return fmt.Errorf("required Zebra read/write characteristics not found on %s", addr.String())
	}

	err = readChar.EnableNotifications(func(buf []byte) {
		m.recvMu.Lock()
		defer m.recvMu.Unlock()
		m.recvBuf.Write(buf)
		if m.notifyCh != nil {
			select {
			case m.notifyCh <- struct{}{}:
			default:
			}
		}
	})
	if err != nil {
		_ = device.Disconnect()
		return fmt.Errorf("enabling indications on %s: %w", addr.String(), err)
	}

	m.device = device
	m.readChar = readChar
	m.writeChar = writeChar
	m.target = target
	m.resolvedAddr = addr
	m.connected = true

	return nil
}

func (m *bleManager) disconnect() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.connected {
		_ = m.device.Disconnect()
		m.connected = false
	}
}

func (m *bleManager) writeChunks(data []byte) error {
	const chunkSize = 120
	for len(data) > 0 {
		n := len(data)
		if n > chunkSize {
			n = chunkSize
		}
		if _, err := m.writeChar.Write(data[:n]); err != nil {
			m.connected = false
			_ = m.device.Disconnect()
			return err
		}
		data = data[n:]
	}
	return nil
}

func (m *bleManager) query(target, cmd string, until func(string) bool, timeout time.Duration) (string, error) {
	if err := m.ensureConnected(target); err != nil {
		return "", err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.recvMu.Lock()
	m.recvBuf.Reset()
	notifyCh := make(chan struct{}, 20)
	m.notifyCh = notifyCh
	m.recvMu.Unlock()

	if err := m.writeChunks([]byte(cmd)); err != nil {
		return "", err
	}

	deadline := time.Now().Add(timeout)
	for {
		m.recvMu.Lock()
		str := m.recvBuf.String()
		m.recvMu.Unlock()

		if until(str) {
			return str, nil
		}

		remain := time.Until(deadline)
		if remain <= 0 {
			// Return whatever was accumulated even on timeout
			return str, fmt.Errorf("timeout waiting for response after %v", timeout)
		}

		select {
		case <-notifyCh:
		case <-time.After(remain):
		}
	}
}

func queryBLEPrinterInfoLocked(target string) PrinterInfo {
	info := PrinterInfo{
		IP:     target,
		Type:   "zpl",
		Status: "offline",
		Errors: PrinterErrors{
			Unreachable: true,
		},
	}

	// Efficient query for Model (~HI), Errors (~HQES), Serial (device.unique_id), and Label Length (zpl.label_length)
	cmd := "~HI~HQES! U1 getvar \"device.unique_id\"\r\n! U1 getvar \"zpl.label_length\"\r\n"
	data, err := bleMgr.query(target, cmd, func(str string) bool {
		hqesEnd := strings.Index(str, "WARNINGS:")
		hasCompleteHQES := hqesEnd != -1 && strings.Contains(str[hqesEnd:], "\x03")
		hasSGD := strings.Count(str, "\"") >= 4
		return hasCompleteHQES && hasSGD
	}, 3*time.Second)

	if err != nil && data == "" {
		logf("BLE query failed for %s: %v", target, err)
		bleMgr.disconnect()
		return info
	}

	parsed := parsePrinterData(target, data)
	return parsed
}

func sendZPLBLELocked(target, zpl string) error {
	if err := bleMgr.ensureConnected(target); err != nil {
		return err
	}

	bleMgr.mu.Lock()
	defer bleMgr.mu.Unlock()

	return bleMgr.writeChunks([]byte(zpl))
}

func queryStatusBLELocked(target string) (HostStatus, error) {
	var s HostStatus
	data, err := bleMgr.query(target, "~HS\r\n", func(str string) bool {
		return strings.Count(str, "\n") >= 3 && strings.Count(str, ",") >= 20
	}, 3*time.Second)

	if err != nil && data == "" {
		return s, err
	}

	var lines []string
	for _, l := range strings.Split(data, "\n") {
		trimmed := strings.Trim(l, "\x02\x03\r\n ")
		if trimmed != "" && strings.Contains(trimmed, ",") {
			lines = append(lines, trimmed)
		}
	}

	if len(lines) < 2 {
		return s, fmt.Errorf("unexpected ~HS response via BLE: %q", data)
	}

	f1 := strings.Split(lines[0], ",")
	f2 := strings.Split(lines[1], ",")
	if len(f1) < 5 || len(f2) < 10 {
		return s, fmt.Errorf("malformed ~HS fields: %q", lines)
	}

	s.PaperOut = f1[1] == "1"
	s.Paused = f1[2] == "1"
	s.FormatsInBuffer = atoi(f1[4])
	s.HeadUp = f2[2] == "1"
	s.RibbonOut = f2[3] == "1"
	s.LabelsRemaining = atoi(f2[8])
	s.PrintingFormat = f2[9] == "1"

	return s, nil
}
