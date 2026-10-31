//go:build !windows

package main

func readPlatformConfig() *Config {
	return readFileConfig(getPlatformConfigPath())
}

func savePlatformToken(token string) error {
	return saveFileConfigField("Token", token)
}

func savePlatformName(name string) error {
	return saveFileConfigField("Name", name)
}

func savePlatformApiUrl(apiUrl string) error {
	clean := CleanApiUrl(apiUrl)
	return saveFileConfigField("ApiUrl", clean)
}

func savePlatformPrinterIp(printerIp string) error {
	return saveFileConfigField("PrinterIp", printerIp)
}

func savePlatformZplPrinters(printers []string) error {
	return saveFileConfigField("ZplPrinters", printers)
}

func savePlatformPort(port int) error {
	return saveFileConfigField("Port", port)
}
