//go:build linux

package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Pure-Go USB printer access via usbfs (/dev/bus/usb/BBB/DDD), no libusb/cgo.
// The user running the service needs read/write access to the device node
// (see 99-usbprintagent.rules in usbZebraAgent: group plugdev).

const (
	usbfsMaxChunk     = 16 * 1024
	usbDefaultTimeout = 5 * time.Second
)

func ioc(dir, nr, size uintptr) uintptr {
	return dir<<30 | size<<16 | uintptr('U')<<8 | nr
}

const (
	iocRead  = 2
	iocWrite = 1
)

type usbfsBulk struct {
	ep      uint32
	length  uint32
	timeout uint32
	data    unsafe.Pointer
}

type usbfsDisconnectClaim struct {
	iface  uint32
	flags  uint32
	driver [256]byte
}

type usbfsIoctl struct {
	ifno int32
	code int32
	data unsafe.Pointer
}

var (
	usbdevfsBulk            = ioc(iocRead|iocWrite, 2, unsafe.Sizeof(usbfsBulk{}))
	usbdevfsClaimInterface  = ioc(iocRead, 15, 4)
	usbdevfsReleaseIface    = ioc(iocRead, 16, 4)
	usbdevfsIoctl           = ioc(iocRead|iocWrite, 18, unsafe.Sizeof(usbfsIoctl{}))
	usbdevfsClearHalt       = ioc(iocRead, 21, 4)
	usbdevfsDisconnectClaim = ioc(iocRead, 27, unsafe.Sizeof(usbfsDisconnectClaim{}))
	usbdevfsConnectCode     = uintptr('U')<<8 | 23
)

func ioctl(fd int, req uintptr, arg unsafe.Pointer) (int, error) {
	r, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), req, uintptr(arg))
	if errno != 0 {
		return int(r), errno
	}
	return int(r), nil
}

func readSysfs(dir, name string) string {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// listUSBPrinters enumerates USB printer-class devices via sysfs.
func listUSBPrinters() []usbPrinterDev {
	entries, err := os.ReadDir("/sys/bus/usb/devices")
	if err != nil {
		return nil
	}
	var out []usbPrinterDev
	for _, e := range entries {
		name := e.Name()
		if strings.Contains(name, ":") { // interface entries
			continue
		}
		dir := filepath.Join("/sys/bus/usb/devices", name)
		raw, err := os.ReadFile(filepath.Join(dir, "descriptors"))
		if err != nil {
			continue
		}
		dev, ok := parseUSBDescriptors(raw)
		if !ok {
			continue
		}
		bus, err1 := strconv.Atoi(readSysfs(dir, "busnum"))
		addr, err2 := strconv.Atoi(readSysfs(dir, "devnum"))
		if err1 != nil || err2 != nil {
			continue
		}
		dev.Bus, dev.Addr = bus, addr
		dev.Serial = readSysfs(dir, "serial")
		dev.Product = readSysfs(dir, "product")
		out = append(out, dev)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Bus != out[j].Bus {
			return out[i].Bus < out[j].Bus
		}
		return out[i].Addr < out[j].Addr
	})
	return out
}

// usbConn adapts a claimed USB printer interface to net.Conn so the existing
// ZPL query/send code can treat it like a TCP connection.
type usbConn struct {
	fd      int
	dev     usbPrinterDev
	rdl     time.Time
	wdl     time.Time
	pending []byte
	rbuf    []byte
}

type usbAddr string

func (a usbAddr) Network() string { return "usb" }
func (a usbAddr) String() string  { return string(a) }

func dialUSB(target string) (net.Conn, error) {
	t, ok := parseUSBTarget(target)
	if !ok {
		return nil, fmt.Errorf("invalid USB target %q", target)
	}
	var dev usbPrinterDev
	found := false
	for _, d := range listUSBPrinters() {
		if d.matches(t) {
			dev, found = d, true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("USB printer %q not found", target)
	}

	path := fmt.Sprintf("/dev/bus/usb/%03d/%03d", dev.Bus, dev.Addr)
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	// Detach any kernel driver (e.g. usblp) and claim the interface.
	dc := usbfsDisconnectClaim{iface: uint32(dev.Iface)}
	if _, err := ioctl(fd, usbdevfsDisconnectClaim, unsafe.Pointer(&dc)); err != nil {
		iface := uint32(dev.Iface)
		if _, err2 := ioctl(fd, usbdevfsClaimInterface, unsafe.Pointer(&iface)); err2 != nil {
			unix.Close(fd)
			return nil, fmt.Errorf("claim interface %d on %s: %w", dev.Iface, path, err2)
		}
	}

	mps := dev.MpsIn
	if mps <= 0 {
		mps = 512
	}
	return &usbConn{
		fd:   fd,
		dev:  dev,
		rbuf: make([]byte, (4096/mps+1)*mps),
	}, nil
}

func (c *usbConn) bulk(ep uint8, buf []byte, timeout time.Duration) (int, error) {
	ms := timeout.Milliseconds()
	if ms < 1 {
		ms = 1
	}
	x := usbfsBulk{ep: uint32(ep), length: uint32(len(buf)), timeout: uint32(ms)}
	if len(buf) > 0 {
		x.data = unsafe.Pointer(&buf[0])
	}
	n, err := ioctl(c.fd, usbdevfsBulk, unsafe.Pointer(&x))
	runtime.KeepAlive(buf)
	if err != nil {
		switch err {
		case unix.ETIMEDOUT:
			return 0, os.ErrDeadlineExceeded
		case unix.EPIPE:
			ep32 := uint32(ep)
			_, _ = ioctl(c.fd, usbdevfsClearHalt, unsafe.Pointer(&ep32))
		case unix.ENODEV, unix.ESHUTDOWN:
			return 0, fmt.Errorf("USB printer disconnected: %w", err)
		}
		return 0, err
	}
	return n, nil
}

func remaining(deadline time.Time) time.Duration {
	if deadline.IsZero() {
		return usbDefaultTimeout
	}
	return time.Until(deadline)
}

func (c *usbConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if c.dev.EpIn == 0 {
		return 0, errors.New("USB printer interface has no bulk IN endpoint")
	}
	if len(c.pending) > 0 {
		n := copy(p, c.pending)
		c.pending = c.pending[n:]
		return n, nil
	}
	for {
		d := remaining(c.rdl)
		if d <= 0 {
			return 0, os.ErrDeadlineExceeded
		}
		n, err := c.bulk(c.dev.EpIn, c.rbuf, d)
		if err != nil {
			return 0, err
		}
		if n == 0 { // zero-length packet
			continue
		}
		m := copy(p, c.rbuf[:n])
		if m < n {
			c.pending = append(c.pending[:0], c.rbuf[m:n]...)
		}
		return m, nil
	}
}

func (c *usbConn) Write(p []byte) (int, error) {
	written := 0
	for written < len(p) {
		d := remaining(c.wdl)
		if d <= 0 {
			return written, os.ErrDeadlineExceeded
		}
		end := written + usbfsMaxChunk
		if end > len(p) {
			end = len(p)
		}
		n, err := c.bulk(c.dev.EpOut, p[written:end], d)
		written += n
		if err != nil {
			return written, err
		}
		if n == 0 {
			return written, errors.New("USB bulk write made no progress")
		}
	}
	return written, nil
}

func (c *usbConn) Close() error {
	if c.fd < 0 {
		return nil
	}
	iface := uint32(c.dev.Iface)
	_, _ = ioctl(c.fd, usbdevfsReleaseIface, unsafe.Pointer(&iface))
	// Hand the interface back to the kernel driver (e.g. usblp) if there was one.
	ur := usbfsIoctl{ifno: int32(c.dev.Iface), code: int32(usbdevfsConnectCode)}
	_, _ = ioctl(c.fd, usbdevfsIoctl, unsafe.Pointer(&ur))
	err := unix.Close(c.fd)
	c.fd = -1
	return err
}

func (c *usbConn) LocalAddr() net.Addr  { return usbAddr("local") }
func (c *usbConn) RemoteAddr() net.Addr { return usbAddr(c.dev.Target()) }
func (c *usbConn) SetDeadline(t time.Time) error {
	c.rdl, c.wdl = t, t
	return nil
}
func (c *usbConn) SetReadDeadline(t time.Time) error  { c.rdl = t; return nil }
func (c *usbConn) SetWriteDeadline(t time.Time) error { c.wdl = t; return nil }

// scanUSBZPLPrinters queries every attached USB printer-class device.
func scanUSBZPLPrinters() []PrinterInfo {
	var found []PrinterInfo
	for _, d := range listUSBPrinters() {
		info := QueryPrinterInfo(d.Target())
		if info.Errors.Unreachable || (info.Model == "" && info.Serial == "") {
			continue
		}
		if info.Serial == "" {
			info.Serial = d.Serial
		}
		if info.Model == "" {
			info.Model = d.Product
		}
		found = append(found, info)
	}
	return found
}

