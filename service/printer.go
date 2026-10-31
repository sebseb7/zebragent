package main

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

const printerPort = "9100"

// PrinterErrors tracks specific error states parsed from ~HQES.
type PrinterErrors struct {
	HeadOpen    bool `json:"headOpen"`
	Pause       bool `json:"pause"`
	Other       bool `json:"other"`
	Unreachable bool `json:"unreachable"`
}

// PrinterInfo holds label length, serial, model, type, and error flags from ^HH and ~HQES queries.
type PrinterInfo struct {
	IP          string        `json:"ip"`
	Serial      string        `json:"serial"`
	Model       string        `json:"model"`
	Type        string        `json:"type"` // always "zpl"
	LabelLength *int          `json:"labelLength"` // null if not detected/reported
	Errors      PrinterErrors `json:"errors"`
	Ready       bool          `json:"ready"`
	Status      string        `json:"status"` // "online", "headOpen", "pause", "error", "offline", "unknown"
}

// QueryPrinterInfo sends ^XA^HH^XZ~HI~HQES! U1 getvar "device.unique_id"\r\n to the printer on port 9100,
// collects the response, and parses label length, serial, model, and error bits.
func QueryPrinterInfo(ip string) PrinterInfo {
	info := PrinterInfo{
		IP:     ip,
		Type:   "zpl",
		Status: "offline",
		Errors: PrinterErrors{
			Unreachable: true,
		},
	}

	if ip == "" {
		return info
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, printerPort), 3*time.Second)
	if err != nil {
		return info
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
	if _, err := conn.Write([]byte("^XA^HH^XZ~HI~HQES! U1 getvar \"device.unique_id\"\r\n")); err != nil {
		return info
	}

	var received strings.Builder
	buf := make([]byte, 4096)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(600 * time.Millisecond))
		n, err := conn.Read(buf)
		if n > 0 {
			received.Write(buf[:n])
			str := received.String()
			hasLabelLength := strings.Contains(str, "LABEL LENGTH")
			hasErrors := strings.Contains(str, "ERRORS:")
			hasSerial := strings.Count(str, "\"") >= 2 || strings.Contains(str, "SERIAL NUMBER")
			hasModel := strings.Contains(str, ",") || strings.Contains(str, "ZD") || strings.Contains(str, "ZT")
			if hasLabelLength && hasErrors && hasSerial && hasModel {
				break
			}
		}
		if err != nil {
			break
		}
	}

	data := received.String()
	if data == "" {
		return info
	}

	return parsePrinterData(ip, data)
}

// parsePrinterData parses ^HH configuration, ~HI host identification, SGD unique_id, and ~HQES error status from ZPL response text.
func parsePrinterData(ip, data string) PrinterInfo {
	info := PrinterInfo{
		IP:     ip,
		Type:   "zpl",
		Ready:  false,
		Status: "unknown",
		Errors: PrinterErrors{
			Unreachable: false,
		},
	}

	normalized := strings.ReplaceAll(data, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	// 1. Parse Serial from SGD getvar "device.unique_id" (quoted alphanumeric value e.g. "50J203002218")
	for _, chunk := range strings.Split(normalized, "\"") {
		chunk = strings.TrimSpace(chunk)
		if len(chunk) >= 6 && len(chunk) <= 24 && !strings.Contains(chunk, " ") && !strings.Contains(chunk, "device.") {
			info.Serial = chunk
			break
		}
	}

	// 2. Parse ~HI (Host Identification) response for Model (and serial if 7-param format):
	for _, token := range strings.FieldsFunc(data, func(r rune) bool {
		return r == '\x02' || r == '\x03' || r == '\r' || r == '\n'
	}) {
		token = strings.TrimSpace(token)
		if strings.Contains(token, ",") && !strings.Contains(token, "ERRORS:") && !strings.Contains(token, "WARNINGS:") {
			parts := strings.Split(token, ",")
			if len(parts) >= 1 && info.Model == "" {
				modelCandidate := strings.TrimSpace(parts[0])
				if idx := strings.LastIndex(modelCandidate, "\""); idx != -1 {
					modelCandidate = strings.TrimSpace(modelCandidate[idx+1:])
				}
				if modelCandidate != "" {
					info.Model = modelCandidate
				}
			}
			if len(parts) >= 7 && info.Serial == "" {
				candidate := strings.TrimPrefix(strings.TrimSpace(parts[6]), "+")
				upper := strings.ToUpper(candidate)
				if !strings.HasSuffix(upper, "KB") && !strings.HasSuffix(upper, "MB") && !strings.EqualFold(upper, "NONE") {
					info.Serial = candidate
				}
			}
		}
	}

	lines := strings.Split(normalized, "\n")

	// 3. Parse Label Length from ^HH response: line example "  0261                LABEL LENGTH      "
	for _, line := range lines {
		if strings.Contains(line, "LABEL LENGTH") {
			parts := strings.Fields(strings.TrimSpace(line))
			if len(parts) > 0 {
				if n, err := strconv.Atoi(parts[0]); err == nil {
					val := n
					info.LabelLength = &val
					break
				}
			}
		}
	}

	// 4. Parse Serial Number from ^HH response if not already parsed from SGD or ~HI
	if info.Serial == "" {
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			upper := strings.ToUpper(trimmed)
			if strings.HasSuffix(upper, "SERIAL NUMBER") {
				parts := strings.Fields(trimmed)
				if len(parts) >= 3 {
					info.Serial = strings.TrimPrefix(parts[0], "+")
					break
				}
			} else if strings.HasPrefix(upper, "SERIAL NUMBER") {
				if idx := strings.Index(trimmed, ":"); idx != -1 {
					info.Serial = strings.TrimPrefix(strings.TrimSpace(trimmed[idx+1:]), "+")
					break
				}
				parts := strings.Fields(trimmed)
				if len(parts) >= 3 {
					info.Serial = strings.TrimPrefix(parts[2], "+")
					break
				}
			}
		}
	}

	// 5. Parse Model from ^HH response if not already parsed from ~HI
	if info.Model == "" {
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			upper := strings.ToUpper(trimmed)
			if strings.Contains(upper, "MODEL") && !strings.Contains(upper, "CLIENTMODEL") {
				if strings.Contains(trimmed, ":") {
					_, val, _ := strings.Cut(trimmed, ":")
					info.Model = strings.TrimSpace(val)
					break
				}
				parts := strings.Fields(trimmed)
				if len(parts) > 1 {
					idx := -1
					for i, p := range parts {
						if strings.EqualFold(p, "MODEL") {
							idx = i
							break
						}
					}
					if idx > 0 {
						info.Model = strings.Join(parts[:idx], " ")
						break
					}
				}
			}
		}
	}

	// Also check Zebra product line headers if model wasn't found above
	if info.Model == "" {
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "ZTC ") || strings.HasPrefix(trimmed, "Zebra ") {
				info.Model = trimmed
				break
			}
		}
	}

	// 6. Parse Errors from ~HQES response: line example "ERRORS: 0 0 00010004"
	errorsParsed := false
	hasError := false
	for _, line := range lines {
		if strings.Contains(line, "ERRORS:") {
			parts := strings.Fields(strings.TrimSpace(line))
			if len(parts) >= 2 {
				// Last field or second-to-last field with hex error bits
				var hexStr string
				for i := len(parts) - 1; i >= 1; i-- {
					p := strings.Trim(parts[i], "\x02\x03\r\n ")
					if len(p) == 8 {
						hexStr = p
						break
					}
				}
				if hexStr == "" {
					hexStr = strings.Trim(parts[len(parts)-1], "\x02\x03\r\n ")
				}
				if val, err := strconv.ParseUint(hexStr, 16, 64); err == nil {
					errorsParsed = true
					if val != 0 {
						hasError = true
						if (val & 0x04) != 0 {
							info.Errors.HeadOpen = true
						}
						if (val & 0x10000) != 0 {
							info.Errors.Pause = true
						}
						if !info.Errors.HeadOpen && !info.Errors.Pause {
							info.Errors.Other = true
						}
					}
				}
			}
		}
	}

	if !errorsParsed {
		info.Ready = false
		info.Status = "unknown"
		info.Errors.Other = true
	} else if hasError {
		info.Ready = false
		if info.Errors.HeadOpen {
			info.Status = "headOpen"
		} else if info.Errors.Pause {
			info.Status = "pause"
		} else {
			info.Status = "error"
		}
	} else {
		info.Ready = true
		info.Status = "online"
	}

	return info
}

// HostStatus is the parsed result of the ZPL ~HS command.
type HostStatus struct {
	PaperOut, Paused, HeadUp, RibbonOut bool
	FormatsInBuffer, LabelsRemaining    int
	PrintingFormat                      bool
}

func (s HostStatus) Busy() bool {
	return s.FormatsInBuffer > 0 || s.LabelsRemaining > 0 || s.PrintingFormat
}

// Fault returns a human readable error condition or "".
func (s HostStatus) Fault() string {
	switch {
	case s.PaperOut:
		return "paper out"
	case s.HeadUp:
		return "head open"
	case s.RibbonOut:
		return "ribbon out"
	case s.Paused:
		return "paused"
	}
	return ""
}

func sendZPL(ip, zpl string) error {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, printerPort), 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	_, err = conn.Write([]byte(zpl))
	return err
}

// QueryStatus sends ~HS and parses the three-line response (used during print job polling).
func QueryStatus(ip string) (HostStatus, error) {
	var s HostStatus
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, printerPort), 5*time.Second)
	if err != nil {
		return s, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte("~HS")); err != nil {
		return s, err
	}
	r := bufio.NewReader(conn)
	var lines [3]string
	for i := range lines {
		l, err := r.ReadString('\n')
		if err != nil {
			return s, fmt.Errorf("reading ~HS line %d: %w", i+1, err)
		}
		lines[i] = strings.Trim(l, "\x02\x03\r\n ")
	}
	f1 := strings.Split(lines[0], ",")
	f2 := strings.Split(lines[1], ",")
	if len(f1) < 5 || len(f2) < 10 {
		return s, fmt.Errorf("unexpected ~HS response %q", lines)
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

func atoi(s string) int {
	n := 0
	fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
	return n
}
