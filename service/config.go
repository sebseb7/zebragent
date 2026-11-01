package main

import (
	"errors"
	"strings"
)

const (
	AgentVersion  = "1"
	AgentBasePath = "/api/agent"
)

// Config contains the service configuration.
type Config struct {
	Token        string   // persistent token obtained from pairing
	AgentName    string   // friendly name assigned to agent
	ApiUrl       string   // base domain only (e.g. https://api.example.com)
	PrinterIp    string   // primary printer IP address (TCP 9100) or Bluetooth target (e.g. ble://34:14:B5:B3:89:4A or ble:auto)
	ZplPrinters  []string // list of ZPL printer IP addresses
	EventsPath   string   // default "/events" (appended under /api/agent)
	StatePath    string   // default "/state" (appended under /api/agent)
	PairPath     string   // default "/pair" (appended under /api/agent)
	LocalAddr    string   // local monitor address, default "127.0.0.1:41782"
	ClientSerial string   // optional serial override passed via flag
	ClientModel  string   // optional model override passed via flag
}

// CleanApiUrl normalizes the API base domain, stripping any trailing slashes or /api/agent.
func CleanApiUrl(raw string) string {
	u := strings.TrimRight(strings.TrimSpace(raw), "/")
	u = strings.TrimSuffix(u, AgentBasePath)
	u = strings.TrimRight(u, "/")
	if u != "" && !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		if strings.HasPrefix(u, "localhost") || strings.HasPrefix(u, "127.0.0.1") {
			u = "http://" + u
		} else {
			u = "https://" + u
		}
	}
	return u
}

// maskValue masks sensitive values like Token for logging.
func maskValue(name, val string) string {
	if strings.EqualFold(name, "token") && len(val) > 8 {
		return val[:4] + "..." + val[len(val)-4:]
	}
	return val
}

// Endpoint builds the full URL: <ApiUrl domain> + /api/agent/<subpath>.
func (c *Config) Endpoint(subpath string) string {
	domain := CleanApiUrl(c.ApiUrl)
	sub := strings.TrimPrefix(strings.TrimSpace(subpath), "/")
	sub = strings.TrimPrefix(sub, "api/agent/")
	sub = strings.TrimPrefix(sub, "api/agent")
	sub = strings.TrimPrefix(sub, "/")

	if sub == "" {
		return domain + AgentBasePath
	}
	return domain + AgentBasePath + "/" + sub
}

// LoadConfig reads configuration:
// - In service mode: strictly and only from HKLM\SOFTWARE\ZebraAgent
// - In non-service mode: reads HKLM as base, overridden by command line parameters
func LoadConfig(isService bool) (*Config, error) {
	c := readPlatformConfig()

	// In non-service mode, command line parameters take precedence
	if !isService {
		if flagApiUrl != "" {
			c.ApiUrl = flagApiUrl
		}
		if flagPrinterIp != "" {
			c.PrinterIp = flagPrinterIp
		}
		if flagToken != "" {
			c.Token = flagToken
		}
		if flagLocalAddr != "" {
			c.LocalAddr = flagLocalAddr
		}
		if flagClientSerial != "" {
			c.ClientSerial = flagClientSerial
		}
		if flagClientModel != "" {
			c.ClientModel = flagClientModel
		}
	}

	c.Token = strings.TrimSpace(c.Token)
	c.PrinterIp = strings.TrimSpace(c.PrinterIp)
	c.ApiUrl = CleanApiUrl(c.ApiUrl)

	var validPrinters []string
	for _, p := range c.ZplPrinters {
		p = strings.TrimSpace(p)
		if p != "" && IsValidPrinterTarget(p) {
			validPrinters = append(validPrinters, p)
		}
	}
	c.ZplPrinters = validPrinters

	if len(c.ZplPrinters) == 0 && c.PrinterIp != "" {
		c.ZplPrinters = []string{c.PrinterIp}
	} else if c.PrinterIp == "" && len(c.ZplPrinters) > 0 {
		c.PrinterIp = c.ZplPrinters[0]
	}

	if c.EventsPath == "" {
		c.EventsPath = "/events"
	}
	if c.StatePath == "" {
		c.StatePath = "/state"
	}
	if c.PairPath == "" {
		c.PairPath = "/pair"
	}
	if c.LocalAddr == "" {
		c.LocalAddr = "127.0.0.1:41782"
	}

	if c.PrinterIp != "" && !IsValidPrinterTarget(c.PrinterIp) {
		if isService {
			return nil, errors.New(`PrinterIp invalid in HKLM\SOFTWARE\ZebraAgent\PrinterIp`)
		}
		return nil, errors.New(`PrinterIp invalid (specify valid IP or Bluetooth address via -printerip or set in HKLM\SOFTWARE\ZebraAgent\PrinterIp)`)
	}

	return c, nil
}

// SaveToken persists the token to registry.
func SaveToken(token string) error {
	return savePlatformToken(token)
}

// SaveApiUrl persists the ApiUrl domain to registry.
func SaveApiUrl(apiUrl string) error {
	return savePlatformApiUrl(apiUrl)
}

// SavePrinterIp persists the PrinterIp to registry.
func SavePrinterIp(printerIp string) error {
	return savePlatformPrinterIp(printerIp)
}

// SaveZplPrinters persists the list of ZPL printer IPs to registry.
func SaveZplPrinters(printers []string) error {
	return savePlatformZplPrinters(printers)
}

// SavePort persists the active local monitor port to HKCU on launch.
func SavePort(port int) error {
	return savePlatformPort(port)
}

// SaveName persists the assigned agent name to registry.
func SaveName(name string) error {
	return savePlatformName(name)
}
