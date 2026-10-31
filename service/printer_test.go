package main

import (
	"testing"
)

func TestParsePrinterDataNormal(t *testing.T) {
	data := `
                    CONFIGURATION
  0215                LABEL LENGTH
  00                  PRINT WIDTH
  42J150100123        SERIAL NUMBER
  ZT410-203dpi        MODEL
\x02
  00000000 00000000 00000000 00000000
  ERRORS: 0 0 0 00000000
\x03
`
	info := parsePrinterData("192.168.1.50", data)
	if info.LabelLength == nil || *info.LabelLength != 215 {
		t.Errorf("expected LabelLength=215, got %v", info.LabelLength)
	}
	if info.Serial != "42J150100123" {
		t.Errorf("expected Serial=42J150100123, got %s", info.Serial)
	}
	if info.Model != "ZT410-203dpi" {
		t.Errorf("expected Model=ZT410-203dpi, got %s", info.Model)
	}
	if !info.Ready {
		t.Errorf("expected Ready=true, got %v", info.Ready)
	}
	if info.Status != "online" {
		t.Errorf("expected Status=online, got %s", info.Status)
	}
	if info.Errors.HeadOpen || info.Errors.Pause || info.Errors.Other || info.Errors.Unreachable {
		t.Errorf("expected no errors, got %+v", info.Errors)
	}
}

func TestParsePrinterDataWithColonFormat(t *testing.T) {
	data := `
PRINTER CONFIGURATION
SERIAL NUMBER: JX987654
MODEL: ZD420-203DPI
LABEL LENGTH : 300
ERRORS: 0 0 00000000
`
	info := parsePrinterData("192.168.1.50", data)
	if info.Serial != "JX987654" {
		t.Errorf("expected Serial=JX987654, got %s", info.Serial)
	}
	if info.Model != "ZD420-203DPI" {
		t.Errorf("expected Model=ZD420-203DPI, got %s", info.Model)
	}
}

func TestParsePrinterDataHeadOpenAndPause(t *testing.T) {
	// 00010004 -> bit 2 (0x04) head open, bit 16 (0x10000) pause
	data := `
  0215                LABEL LENGTH
  ERRORS: 0 0 0 00010004
`
	info := parsePrinterData("192.168.1.50", data)
	// LabelLength is preserved (not zeroed out)
	if info.LabelLength == nil || *info.LabelLength != 215 {
		t.Errorf("expected LabelLength=215, got %v", info.LabelLength)
	}
	if info.Ready {
		t.Errorf("expected Ready=false, got %v", info.Ready)
	}
	if !info.Errors.HeadOpen {
		t.Errorf("expected HeadOpen=true, got false")
	}
	if !info.Errors.Pause {
		t.Errorf("expected Pause=true, got false")
	}
	if info.Status != "headOpen" {
		t.Errorf("expected Status=headOpen, got %s", info.Status)
	}
}

func TestParsePrinterDataOtherError(t *testing.T) {
	// 00000020 -> other error
	data := `
  0215                LABEL LENGTH
  ERRORS: 0 0 0 00000020
`
	info := parsePrinterData("192.168.1.50", data)
	// LabelLength is preserved (not zeroed out)
	if info.LabelLength == nil || *info.LabelLength != 215 {
		t.Errorf("expected LabelLength=215, got %v", info.LabelLength)
	}
	if !info.Errors.Other {
		t.Errorf("expected Other=true, got false")
	}
	if info.Status != "error" {
		t.Errorf("expected Status=error, got %s", info.Status)
	}
}

func TestParsePrinterDataHostIdentification(t *testing.T) {
	// Raw data containing ~HI response: \x02<model>,<firmware>,...,<serial>\x03
	data := "  0261                LABEL LENGTH\r\n\x02ZD420-203dpi,V84.20.21Z,4,203,16MB,none,41J190100456\x03\r\n\x02ERRORS: 0 0 00000000\x03"
	info := parsePrinterData("192.168.188.47", data)
	if info.LabelLength == nil || *info.LabelLength != 261 {
		t.Errorf("expected LabelLength=261, got %v", info.LabelLength)
	}
	if info.Model != "ZD420-203dpi" {
		t.Errorf("expected Model=ZD420-203dpi, got %s", info.Model)
	}
	if info.Serial != "41J190100456" {
		t.Errorf("expected Serial=41J190100456, got %s", info.Serial)
	}
	if !info.Ready {
		t.Errorf("expected Ready=true, got %v", info.Ready)
	}
}

func TestParsePrinterDataRealZD410Output(t *testing.T) {
	raw := "\"50J203002218\" ZD410-200dpi,V84.20.21Z,8,8192KB \r\n\r\n" +
		"  PRINTER STATUS                            \r\n" +
		"   ERRORS:         0 00000000 00000000      \r\n" +
		"   WARNINGS:       0 00000000 00000000      \r\n" +
		"  420                 PRINT WIDTH       \r\n" +
		"  0261                LABEL LENGTH      \r\n"

	info := parsePrinterData("192.168.188.47", raw)
	if info.Serial != "50J203002218" {
		t.Errorf("expected Serial=50J203002218, got %q", info.Serial)
	}
	if info.Model != "ZD410-200dpi" {
		t.Errorf("expected Model=ZD410-200dpi, got %q", info.Model)
	}
	if info.LabelLength == nil || *info.LabelLength != 261 {
		t.Errorf("expected LabelLength=261, got %v", info.LabelLength)
	}
	if !info.Ready || info.Status != "online" {
		t.Errorf("expected Ready=true and Status=online, got ready=%v status=%s", info.Ready, info.Status)
	}
}

func TestParsePrinterDataMissingErrorsAndLength(t *testing.T) {
	// Incomplete response without ERRORS or LABEL LENGTH
	data := "some random incomplete text"
	info := parsePrinterData("192.168.188.47", data)
	if info.LabelLength != nil {
		t.Errorf("expected LabelLength=nil (null), got %v", info.LabelLength)
	}
	if info.Ready {
		t.Errorf("expected Ready=false on unconfirmed state, got true")
	}
	if info.Status != "unknown" {
		t.Errorf("expected Status=unknown, got %s", info.Status)
	}
}

func TestParseCancelEvent(t *testing.T) {
	// 1. event: cancel, JSON payload
	ev1 := Event{
		Event: "cancel",
		Data:  `{"model":"ZD410-200dpi","serial":"50J203002218","type":"zpl"}`,
	}
	m, s, pt, ok := parseCancelEvent(ev1)
	if !ok || m != "ZD410-200dpi" || s != "50J203002218" || pt != "zpl" {
		t.Errorf("ev1 failed: got ok=%v, model=%q, serial=%q, type=%q", ok, m, s, pt)
	}

	// 2. data: cancel with JSON payload
	ev2 := Event{
		Data: `cancel {"model":"ZD410-200dpi","serial":"50J203002218","type":"zpl"}`,
	}
	m, s, pt, ok = parseCancelEvent(ev2)
	if !ok || m != "ZD410-200dpi" || s != "50J203002218" || pt != "zpl" {
		t.Errorf("ev2 failed: got ok=%v, model=%q, serial=%q, type=%q", ok, m, s, pt)
	}

	// 3. event: cancel with query string
	ev3 := Event{
		Event: "cancel",
		Data:  "model=ZD410-200dpi&serial=50J203002218&type=zpl",
	}
	m, s, pt, ok = parseCancelEvent(ev3)
	if !ok || m != "ZD410-200dpi" || s != "50J203002218" || pt != "zpl" {
		t.Errorf("ev3 failed: got ok=%v, model=%q, serial=%q, type=%q", ok, m, s, pt)
	}

	// 4. non-cancel event
	ev4 := Event{
		Event: "connected",
		Data:  `{"agentId":3}`,
	}
	_, _, _, ok = parseCancelEvent(ev4)
	if ok {
		t.Errorf("ev4 expected ok=false, got true")
	}
}

func TestParseJobEvent(t *testing.T) {
	// 1. event: job, created
	ev1 := Event{
		Event: "job",
		Data:  `{"action":"created","jobId":42,"agentprinterId":1,"type":"zpl","state":"pending","createdAt":"2026-10-31T21:05:00.000Z"}`,
	}
	je, ok := parseJobEvent(ev1)
	if !ok || je.Action != "created" || je.JobID.String() != "42" || je.AgentPrinterID != 1 || je.Type != "zpl" {
		t.Errorf("ev1 failed: got ok=%v, je=%+v", ok, je)
	}

	// 2. event: printerjob, cancelled
	ev2 := Event{
		Event: "printerjob",
		Data:  `{"action":"cancelled","jobId":42,"agentprinterId":1,"type":"zpl","state":"cancelled"}`,
	}
	je, ok = parseJobEvent(ev2)
	if !ok || je.Action != "cancelled" || je.JobID.String() != "42" {
		t.Errorf("ev2 failed: got ok=%v, je=%+v", ok, je)
	}

	// 3. no event name, but data contains action:created (like user's log)
	ev3 := Event{
		Data: `{"action":"created","jobId":1,"agentprinterId":1,"type":"zpl","state":"pending"}`,
	}
	je, ok = parseJobEvent(ev3)
	if !ok || je.Action != "created" || je.JobID.String() != "1" {
		t.Errorf("ev3 failed: got ok=%v, je=%+v", ok, je)
	}

	// 4. unrelated event
	ev4 := Event{
		Event: "connected",
		Data:  `{"agentId":3}`,
	}
	_, ok = parseJobEvent(ev4)
	if ok {
		t.Errorf("ev4 expected ok=false, got true")
	}
}
