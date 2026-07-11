// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

//go:build linux

package main

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// uartTransact opens uartDevPath directly via raw syscalls (unix.Open/
// unix.Read/unix.Write/unix.Close), NOT via os.OpenFile. This is
// deliberate: Go's os.File wraps file descriptors that support polling
// (which includes tty/serial character devices) with the runtime's
// internal async I/O poller, which can put the fd into non-blocking mode
// for its own purposes - interacting unpredictably with the kernel-level
// VMIN/VTIME blocking-read-with-timeout mechanism this function relies on
// to implement read_timeout_ms. Raw unix.Open bypasses Go's poller
// entirely, which is the standard approach real-world Go serial libraries
// use for exactly this reason.
func uartTransact(baudRate int, txData []byte, readTimeoutMs int, maxReadBytes int) ([]byte, error) {
	fd, err := unix.Open(uartDevPath, unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", uartDevPath, err)
	}
	defer unix.Close(fd)

	t, err := unix.IoctlGetTermios(fd, unix.TCGETS2)
	if err != nil {
		return nil, fmt.Errorf("get termios on %s: %w", uartDevPath, err)
	}

	t.Cflag &^= unix.CBAUD | unix.CSIZE | unix.PARENB | unix.CSTOPB
	t.Cflag |= unix.CS8 | unix.CREAD | unix.CLOCAL | unix.BOTHER
	t.Ispeed = uint32(baudRate)
	t.Ospeed = uint32(baudRate)
	t.Iflag = 0
	t.Oflag = 0
	t.Lflag = 0
	t.Cc[unix.VMIN] = 0
	t.Cc[unix.VTIME] = uint8(readTimeoutMs / 100)

	if err := unix.IoctlSetTermios(fd, unix.TCSETS2, t); err != nil {
		return nil, fmt.Errorf("set termios on %s (baud_rate=%d): %w", uartDevPath, baudRate, err)
	}

	if len(txData) > 0 {
		if _, err := unix.Write(fd, txData); err != nil {
			return nil, fmt.Errorf("write %d bytes to %s: %w", len(txData), uartDevPath, err)
		}
	}

	buf := make([]byte, maxReadBytes)
	n, err := unix.Read(fd, buf)
	if err != nil {
		return nil, fmt.Errorf("read from %s (timeout=%dms): %w", uartDevPath, readTimeoutMs, err)
	}
	return buf[:n], nil
}
