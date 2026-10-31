//go:build windows

package main

import (
	"strconv"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func rootName(root registry.Key) string {
	if root == registry.CURRENT_USER {
		return "HKCU"
	}
	return "HKLM"
}

func readRegistryKey(root registry.Key, path string, c *Config) {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return
	}
	defer k.Close()

	rn := rootName(root)

	checkStr := func(name string, target *string) {
		if val, _, err := k.GetStringValue(name); err == nil && val != "" && *target == "" {
			*target = val
			logf("Registry read [%s\\%s]: %s=%q", rn, path, name, maskValue(name, val))
		}
	}

	checkStr("Token", &c.Token)
	checkStr("Name", &c.AgentName)
	checkStr("ApiUrl", &c.ApiUrl)
	checkStr("PrinterIp", &c.PrinterIp)

	// Read ZplPrinters (REG_MULTI_SZ or REG_SZ)
	if multi, _, err := k.GetStringsValue("ZplPrinters"); err == nil && len(multi) > 0 && len(c.ZplPrinters) == 0 {
		c.ZplPrinters = multi
		logf("Registry read [%s\\%s]: ZplPrinters=%v", rn, path, multi)
	} else if val, _, err := k.GetStringValue("ZplPrinters"); err == nil && val != "" && len(c.ZplPrinters) == 0 {
		parts := strings.FieldsFunc(val, func(r rune) bool {
			return r == ',' || r == ';' || r == '\n' || r == '\r'
		})
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				c.ZplPrinters = append(c.ZplPrinters, p)
			}
		}
		logf("Registry read [%s\\%s]: ZplPrinters=%v", rn, path, c.ZplPrinters)
	}

	checkStr("EventsPath", &c.EventsPath)
	checkStr("StatePath", &c.StatePath)
	checkStr("PairPath", &c.PairPath)
	var portVal string
	checkStr("Port", &portVal)
	if portVal != "" && c.LocalAddr == "" {
		c.LocalAddr = "127.0.0.1:" + portVal
	}
	checkStr("ClientSerial", &c.ClientSerial)
	checkStr("ClientModel", &c.ClientModel)
}

func readPlatformConfig() *Config {
	c := &Config{}
	// Read HKCU first (user configuration)
	readRegistryKey(registry.CURRENT_USER, `Software\ZebraAgent`, c)
	// Fallback to HKLM (machine / service configuration)
	readRegistryKey(registry.LOCAL_MACHINE, `SOFTWARE\ZebraAgent`, c)
	return c
}

func saveRegistryString(name, value string) error {
	value = strings.TrimSpace(value)

	// Do not write if the registry already holds this exact value
	cur := readPlatformConfig()
	existing := ""
	switch name {
	case "Token":
		existing = cur.Token
	case "Name":
		existing = cur.AgentName
	case "ApiUrl":
		existing = cur.ApiUrl
	case "PrinterIp":
		existing = cur.PrinterIp
	case "ZplPrinters":
		existing = strings.Join(cur.ZplPrinters, ",")
	case "ClientSerial":
		existing = cur.ClientSerial
	case "ClientModel":
		existing = cur.ClientModel
	}
	if existing == value {
		return nil
	}

	// Try HKLM first (works when elevated or running as service)
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, `SOFTWARE\ZebraAgent`, registry.SET_VALUE|registry.WOW64_64KEY)
	if err == nil {
		defer k.Close()
		setErr := k.SetStringValue(name, value)
		if setErr == nil {
			logf("Registry write [HKLM\\SOFTWARE\\ZebraAgent]: %s=%q", name, maskValue(name, value))
		}
		return setErr
	}

	// Fallback to HKCU (works for standard non-elevated user)
	kUser, _, errUser := registry.CreateKey(registry.CURRENT_USER, `Software\ZebraAgent`, registry.SET_VALUE|registry.WOW64_64KEY)
	if errUser == nil {
		defer kUser.Close()
		setErr := kUser.SetStringValue(name, value)
		if setErr == nil {
			logf("Registry write [HKCU\\Software\\ZebraAgent]: %s=%q", name, maskValue(name, value))
		}
		return setErr
	}

	logf("Registry write failed for %s: HKLM error: %v, HKCU error: %v", name, err, errUser)
	return err
}

func savePlatformToken(token string) error {
	return saveRegistryString("Token", token)
}

func savePlatformName(name string) error {
	return saveRegistryString("Name", name)
}

func savePlatformApiUrl(apiUrl string) error {
	clean := CleanApiUrl(apiUrl)
	return saveRegistryString("ApiUrl", clean)
}

func savePlatformPrinterIp(printerIp string) error {
	return saveRegistryString("PrinterIp", printerIp)
}

func savePlatformZplPrinters(printers []string) error {
	joined := strings.Join(printers, ",")
	_ = saveRegistryString("ZplPrinters", joined)
	if len(printers) > 0 {
		_ = saveRegistryString("PrinterIp", printers[0])
	}
	return nil
}

func savePlatformPort(port int) error {
	kUser, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\ZebraAgent`, registry.SET_VALUE|registry.WOW64_64KEY)
	if err != nil {
		logf("Registry write failed for Port: HKCU error: %v", err)
		return err
	}
	defer kUser.Close()

	portStr := strconv.Itoa(port)
	_ = kUser.SetStringValue("Port", portStr)
	logf("Registry write [HKCU\\Software\\ZebraAgent]: Port=%s", portStr)
	return nil
}
