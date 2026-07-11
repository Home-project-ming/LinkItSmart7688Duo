// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

//go:build linux

package main

import (
	"fmt"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// atmegaBridgeMu serializes ALL access to atmegaBridgeDevPath across both
// AtmegaADCRead and AtmegaBridgeRaw. Unlike this server's other tools
// (gpio_read, i2c_read, etc.), where each call is independently atomic
// against the hardware, REPORT_ANALOG is a stateful enable/disable
// protocol over a shared byte stream - two concurrent bridge operations
// could genuinely misattribute or lose each other's bytes without this
// lock. See docs/superpowers/specs/2026-07-11-atmega-phase-d-mcp-tools.md
// "Concurrency: a single bridge mutex".
var atmegaBridgeMu sync.Mutex

// atmegaOpenBridge opens the bridge UART and configures it at the fixed
// baud rate this link always uses. subTimeoutDs is the VMIN=0/VTIME
// value (deciseconds) applied to each individual unix.Read() call -
// callers that need to wait longer than 25.5s worth of deciseconds loop
// repeatedly rather than requesting one huge VTIME (which can't represent
// more than 25500ms anyway, being a uint8).
func atmegaOpenBridge(subTimeoutDs uint8) (int, error) {
	fd, err := unix.Open(atmegaBridgeDevPath, unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return -1, fmt.Errorf("open %s: %w", atmegaBridgeDevPath, err)
	}

	t, err := unix.IoctlGetTermios(fd, unix.TCGETS2)
	if err != nil {
		unix.Close(fd)
		return -1, fmt.Errorf("get termios on %s: %w", atmegaBridgeDevPath, err)
	}

	t.Cflag &^= unix.CBAUD | unix.CSIZE | unix.PARENB | unix.CSTOPB | unix.CRTSCTS
	t.Cflag |= unix.CS8 | unix.CREAD | unix.CLOCAL | unix.BOTHER
	t.Ispeed = atmegaBridgeBaud
	t.Ospeed = atmegaBridgeBaud
	t.Iflag = 0
	t.Oflag = 0
	t.Lflag = 0
	t.Cc[unix.VMIN] = 0
	t.Cc[unix.VTIME] = subTimeoutDs

	if err := unix.IoctlSetTermios(fd, unix.TCSETS2, t); err != nil {
		unix.Close(fd)
		return -1, fmt.Errorf("set termios on %s: %w", atmegaBridgeDevPath, err)
	}

	return fd, nil
}

// atmegaReadLoopSubTimeoutDs is the VTIME applied to each individual
// unix.Read() call inside atmegaADCRead's loop - a short, fixed pacing
// value independent of the caller's own timeoutMs, not a user-facing
// parameter (see design spec's "Read loop and timeout semantics").
const atmegaReadLoopSubTimeoutDs = 2 // 200ms

func atmegaADCRead(channel byte, timeoutMs int) (int, error) {
	atmegaBridgeMu.Lock()
	defer atmegaBridgeMu.Unlock()

	fd, err := atmegaOpenBridge(atmegaReadLoopSubTimeoutDs)
	if err != nil {
		return 0, err
	}
	defer unix.Close(fd)

	if _, err := unix.Write(fd, encodeReportAnalog(channel, true)); err != nil {
		return 0, fmt.Errorf("write REPORT_ANALOG enable to %s: %w", atmegaBridgeDevPath, err)
	}

	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	var buf []byte
	value := -1
	for time.Now().Before(deadline) {
		chunk := make([]byte, 64)
		n, err := unix.Read(fd, chunk)
		if err != nil {
			unix.Write(fd, encodeReportAnalog(channel, false))
			return 0, fmt.Errorf("read from %s: %w", atmegaBridgeDevPath, err)
		}
		if n > 0 {
			buf = append(buf, chunk[:n]...)
			var messages []firmataMessage
			messages, buf = parseFirmataMessages(buf)
			for _, m := range messages {
				if m.Channel == channel {
					value = m.Value
					break
				}
			}
			if value >= 0 {
				break
			}
		}
	}

	if _, err := unix.Write(fd, encodeReportAnalog(channel, false)); err != nil {
		if value >= 0 {
			return value, nil
		}
		return 0, fmt.Errorf("write REPORT_ANALOG disable to %s: %w", atmegaBridgeDevPath, err)
	}

	if value < 0 {
		return 0, fmt.Errorf("timeout waiting for ANALOG_MESSAGE on channel %d after %dms", channel, timeoutMs)
	}
	return value, nil
}

func atmegaBridgeRaw(txData []byte, timeoutMs int, maxReadBytes int) ([]byte, error) {
	atmegaBridgeMu.Lock()
	defer atmegaBridgeMu.Unlock()

	fd, err := atmegaOpenBridge(uint8(timeoutMs / 100))
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)

	if len(txData) > 0 {
		if _, err := unix.Write(fd, txData); err != nil {
			return nil, fmt.Errorf("write %d bytes to %s: %w", len(txData), atmegaBridgeDevPath, err)
		}
	}

	buf := make([]byte, maxReadBytes)
	n, err := unix.Read(fd, buf)
	if err != nil {
		return nil, fmt.Errorf("read from %s (timeout=%dms): %w", atmegaBridgeDevPath, timeoutMs, err)
	}
	return buf[:n], nil
}
