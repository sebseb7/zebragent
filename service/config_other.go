//go:build !windows

package main

func readPlatformConfig() *Config {
	return &Config{}
}

func savePlatformToken(token string) error {
	return nil
}

func savePlatformName(name string) error {
	return nil
}

func savePlatformApiUrl(apiUrl string) error {
	return nil
}

func savePlatformPrinterIp(printerIp string) error {
	return nil
}

func savePlatformZplPrinters(printers []string) error {
	return nil
}

func savePlatformPort(port int) error {
	return nil
}
