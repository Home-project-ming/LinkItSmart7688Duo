// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

//go:build linux

package main

import (
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

// serialStream is an atmegaStream over the real bridge UART, holding one
// fd open for the whole download.
//
// It takes atmegaBridgeMu for its entire lifetime rather than per call.
// A download is a stateful multi-message conversation (BEGIN, hundreds
// of DATA, END); an atmega_adc_read slipping between two of those would
// both corrupt the stream and leave the FPGA half-configured with the
// bus gate still held.
type serialStream struct {
	fd  int
	buf []byte // bytes read but not yet consumed by ReadSysex
}

// serialStreamSubTimeoutDs is the VTIME on each individual read, the
// same short-poll pattern atmegaADCRead uses: deadlines are enforced by
// the loop in ReadSysex, not by asking termios for one long timeout.
const serialStreamSubTimeoutDs = 2 // 200ms

func openAtmegaStream() (atmegaStream, error) {
	atmegaBridgeMu.Lock()
	fd, err := atmegaOpenBridge(serialStreamSubTimeoutDs)
	if err != nil {
		atmegaBridgeMu.Unlock()
		return nil, err
	}
	return &serialStream{fd: fd}, nil
}

func (s *serialStream) Write(p []byte) error {
	for len(p) > 0 {
		n, err := unix.Write(s.fd, p)
		if err != nil {
			return fmt.Errorf("write to %s: %w", atmegaBridgeDevPath, err)
		}
		p = p[n:]
	}
	return nil
}

// ReadSysex returns the bytes between the next F0 and F7.
//
// Anything outside a SysEx frame is discarded rather than treated as an
// error: Firmata.begin() emits an unsolicited 3-byte REPORT_VERSION at
// every ATmega boot (see adc-bridge.ino's wire-format note), and a
// download may well be the first thing to run after one.
func (s *serialStream) ReadSysex(timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	for {
		if body, ok := takeSysex(&s.buf); ok {
			return body, nil
		}
		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("no SysEx reply within %s", timeout)
		}
		chunk := make([]byte, 64)
		n, err := unix.Read(s.fd, chunk)
		if err != nil {
			return nil, fmt.Errorf("read from %s: %w", atmegaBridgeDevPath, err)
		}
		s.buf = append(s.buf, chunk[:n]...)
	}
}

func (s *serialStream) Close() error {
	err := unix.Close(s.fd)
	atmegaBridgeMu.Unlock()
	return err
}
