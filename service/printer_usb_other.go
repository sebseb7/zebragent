//go:build !linux

package main

import (
	"errors"
	"net"
)

func dialUSB(target string) (net.Conn, error) {
	return nil, errors.New("USB printers are only supported on Linux")
}

func scanUSBZPLPrinters() []PrinterInfo { return nil }

