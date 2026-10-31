package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
)

var (
	configMu           sync.Mutex
	configPathOverride string
)

// setConfigPathOverride sets an explicit config file path, primarily for unit tests.
func setConfigPathOverride(path string) {
	configMu.Lock()
	defer configMu.Unlock()
	configPathOverride = path
}

// expandHome resolves "~/" or "~\" or "~" at the start of a path using os.UserHomeDir().
func expandHome(path string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err == nil && home != "" {
			return filepath.Join(home, path[2:])
		}
	} else if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil && home != "" {
			return home
		}
	}
	return path
}

// getPlatformConfigPath determines the path to the config file for non-Windows platforms.
// Preference order:
// 1. In-memory override (e.g. from tests)
// 2. ZEBRA_CONFIG_PATH environment variable
// 3. ~/.whatever (if it already exists)
// 4. ~/.zebraagent (if it already exists)
// 5. Defaults to ~/.whatever
func getPlatformConfigPath() string {
	configMu.Lock()
	override := configPathOverride
	configMu.Unlock()

	if override != "" {
		return expandHome(override)
	}

	if env := os.Getenv("ZEBRA_CONFIG_PATH"); env != "" {
		return expandHome(env)
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".whatever"
	}

	whateverPath := filepath.Join(home, ".whatever")
	zebraPath := filepath.Join(home, ".zebraagent")

	if _, err := os.Stat(whateverPath); err == nil {
		return whateverPath
	}
	if _, err := os.Stat(zebraPath); err == nil {
		return zebraPath
	}

	return whateverPath
}

// readFileConfig reads and parses the JSON configuration file at path.
// If the file does not exist, an empty Config is returned without error.
func readFileConfig(path string) *Config {
	path = expandHome(path)
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			logf("Failed to read config file [%s]: %v", path, err)
		}
		return &Config{}
	}

	cfg, err := parseFileConfig(path, data)
	if err != nil {
		logf("Failed to parse config file [%s]: %v", path, err)
		return &Config{}
	}
	return cfg
}

// parseFileConfig parses JSON config data into a Config struct.
// It accepts flexible key casing (snake_case, camelCase, PascalCase).
func parseFileConfig(path string, data []byte) (*Config, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return &Config{}, nil
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	c := &Config{}
	var portVal string

	checkLog := func(name, val string) {
		if val != "" {
			logf("Config file read [%s]: %s=%q", path, name, maskValue(name, val))
		}
	}

	for k, v := range raw {
		norm := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(k, "_", ""), "-", ""))
		switch norm {
		case "token":
			if s, ok := v.(string); ok && c.Token == "" {
				c.Token = strings.TrimSpace(s)
				checkLog("Token", c.Token)
			}
		case "name", "agentname":
			if s, ok := v.(string); ok && c.AgentName == "" {
				c.AgentName = strings.TrimSpace(s)
				checkLog("Name", c.AgentName)
			}
		case "apiurl":
			if s, ok := v.(string); ok && c.ApiUrl == "" {
				c.ApiUrl = strings.TrimSpace(s)
				checkLog("ApiUrl", c.ApiUrl)
			}
		case "printerip":
			if s, ok := v.(string); ok && c.PrinterIp == "" {
				c.PrinterIp = strings.TrimSpace(s)
				checkLog("PrinterIp", c.PrinterIp)
			}
		case "zplprinters":
			if len(c.ZplPrinters) == 0 {
				switch val := v.(type) {
				case []any:
					for _, item := range val {
						if s, ok := item.(string); ok {
							s = strings.TrimSpace(s)
							if s != "" {
								c.ZplPrinters = append(c.ZplPrinters, s)
							}
						}
					}
				case []string:
					for _, s := range val {
						s = strings.TrimSpace(s)
						if s != "" {
							c.ZplPrinters = append(c.ZplPrinters, s)
						}
					}
				case string:
					parts := strings.FieldsFunc(val, func(r rune) bool {
						return r == ',' || r == ';' || r == '\n' || r == '\r'
					})
					for _, p := range parts {
						p = strings.TrimSpace(p)
						if p != "" {
							c.ZplPrinters = append(c.ZplPrinters, p)
						}
					}
				}
				if len(c.ZplPrinters) > 0 {
					logf("Config file read [%s]: ZplPrinters=%v", path, c.ZplPrinters)
				}
			}
		case "eventspath":
			if s, ok := v.(string); ok && c.EventsPath == "" {
				c.EventsPath = strings.TrimSpace(s)
				checkLog("EventsPath", c.EventsPath)
			}
		case "statepath":
			if s, ok := v.(string); ok && c.StatePath == "" {
				c.StatePath = strings.TrimSpace(s)
				checkLog("StatePath", c.StatePath)
			}
		case "pairpath":
			if s, ok := v.(string); ok && c.PairPath == "" {
				c.PairPath = strings.TrimSpace(s)
				checkLog("PairPath", c.PairPath)
			}
		case "localaddr":
			if s, ok := v.(string); ok && c.LocalAddr == "" {
				c.LocalAddr = strings.TrimSpace(s)
			}
		case "port":
			switch val := v.(type) {
			case float64:
				portVal = strconv.Itoa(int(val))
			case int:
				portVal = strconv.Itoa(val)
			case string:
				portVal = strings.TrimSpace(val)
			}
			checkLog("Port", portVal)
		case "clientserial":
			if s, ok := v.(string); ok && c.ClientSerial == "" {
				c.ClientSerial = strings.TrimSpace(s)
				checkLog("ClientSerial", c.ClientSerial)
			}
		case "clientmodel":
			if s, ok := v.(string); ok && c.ClientModel == "" {
				c.ClientModel = strings.TrimSpace(s)
				checkLog("ClientModel", c.ClientModel)
			}
		}
	}

	if portVal != "" && c.LocalAddr == "" {
		c.LocalAddr = "127.0.0.1:" + portVal
	}

	return c, nil
}

// saveFileConfigField writes a named configuration field to the platform config file.
func saveFileConfigField(name string, value any) error {
	return saveFileConfigFieldWithPath(getPlatformConfigPath(), name, value)
}

// saveFileConfigFieldWithPath writes a named configuration field to the specified file path.
func saveFileConfigFieldWithPath(path, name string, value any) error {
	configMu.Lock()
	defer configMu.Unlock()

	path = expandHome(path)

	// Avoid unnecessary write if the current config already has this exact value
	cur := readFileConfig(path)
	switch name {
	case "Token":
		if str, ok := value.(string); ok && cur.Token == strings.TrimSpace(str) {
			return nil
		}
	case "Name":
		if str, ok := value.(string); ok && cur.AgentName == strings.TrimSpace(str) {
			return nil
		}
	case "ApiUrl":
		if str, ok := value.(string); ok && cur.ApiUrl == CleanApiUrl(str) {
			return nil
		}
	case "PrinterIp":
		if str, ok := value.(string); ok && cur.PrinterIp == strings.TrimSpace(str) {
			return nil
		}
	case "ZplPrinters":
		if printers, ok := value.([]string); ok && slices.Equal(cur.ZplPrinters, printers) {
			return nil
		}
	case "Port":
		var portStr string
		switch v := value.(type) {
		case int:
			portStr = strconv.Itoa(v)
		case string:
			portStr = v
		}
		if portStr != "" && cur.LocalAddr == "127.0.0.1:"+portStr {
			return nil
		}
	}

	raw := make(map[string]any)
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &raw)
	}

	setField := func(targetKey string, val any) {
		normTarget := strings.ToLower(strings.ReplaceAll(targetKey, "_", ""))
		for k := range raw {
			normK := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(k, "_", ""), "-", ""))
			if normK == normTarget {
				delete(raw, k)
			}
		}
		raw[targetKey] = val
	}

	switch name {
	case "Token":
		setField("token", strings.TrimSpace(fmt.Sprint(value)))
	case "Name":
		setField("name", strings.TrimSpace(fmt.Sprint(value)))
	case "ApiUrl":
		setField("api_url", CleanApiUrl(fmt.Sprint(value)))
	case "PrinterIp":
		setField("printer_ip", strings.TrimSpace(fmt.Sprint(value)))
	case "ZplPrinters":
		var list []string
		switch v := value.(type) {
		case []string:
			list = v
		case []any:
			for _, item := range v {
				list = append(list, fmt.Sprint(item))
			}
		}
		setField("zpl_printers", list)
		if len(list) > 0 {
			setField("printer_ip", list[0])
		}
	case "Port":
		switch v := value.(type) {
		case int:
			setField("port", v)
		case string:
			if i, err := strconv.Atoi(v); err == nil {
				setField("port", i)
			} else {
				setField("port", v)
			}
		default:
			setField("port", v)
		}
	default:
		setField(strings.ToLower(name), value)
	}

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		logf("Config file write failed for %s [%s]: %v", name, path, err)
		return err
	}

	logVal := fmt.Sprint(value)
	logf("Config file write [%s]: %s=%s", path, name, maskValue(name, logVal))
	return nil
}

