// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import (
	"bytes"
	"fmt"
	"time"
)

// Configuring the XCS20 through the ATmega32U4 instead of MT7688 GPIO.
//
// Why this exists at all: the design in
// docs/superpowers/specs/2026-07-30-xcs20-slave-serial-download-design.md
// assumed the 7688 could spare five GPIOs (sysfsPort in fpga.go). It
// cannot -- the Duo breaks out only P1, P8 and P9, and P8/P9 are the
// console UART, so exactly one pin is free. The four outputs and one
// input therefore have to come from the ATmega, which reaches the
// adapter over the existing 57600-baud bridge.
//
// Why the sequencer is NOT reused: ConfigureSlaveSerial drives one pin
// per call, three calls per bit. Over the bridge that would be 534,432
// round trips. The whole slave-serial sequence (gate, /PROGRAM pulse,
// clocking, startup) therefore lives in the ATmega firmware, which owns
// the pins, and this side only ships bytes and reads back status. That
// also keeps DONE honest: the firmware reads the real pin, rather than
// this side guessing at a value it cannot see.
//
// sysfsPort and ConfigureSlaveSerial in fpga.go are left intact. They
// are tested and correct for a board that does have free GPIOs; they
// are simply not the transport this board can use.

// SysEx command and subcommands. 0x7E is in Firmata's reserved-for-
// non-realtime range and is not claimed by any standard Firmata feature.
const (
	sysexStart = 0xF0
	sysexEnd   = 0xF7

	sysexFPGA = 0x7E // command byte

	fpgaSubBegin = 0x01 // gate, /PROGRAM pulse, verify DONE fell, clear-wait
	fpgaSubData  = 0x02 // one chunk of payload
	fpgaSubEnd   = 0x03 // startup clocks, read DONE, release gate
	fpgaSubAbort = 0x04 // release the gate, whatever state we were in
)

// fpgaChunkBytes is how much raw payload travels in one DATA message.
//
// The ceiling is Firmata's MAX_DATA_BYTES (64, read from the installed
// FirmataConstants.h). A DATA message spends 1 byte on the command, 1 on
// the subcommand and 1 on the length, leaving 61 for encoded data. 49
// raw bytes encode to exactly 56 (49*8 = 392 = 56*7, no padding), which
// fits with room to spare and keeps the common chunk free of ragged
// edges.
const fpgaChunkBytes = 49

// encode7bit packs a byte slice into 7-bit-clean bytes, MSB-first.
//
// SysEx data bytes must have bit 7 clear, so the payload cannot travel
// raw. Firmata's own helper doubles the size (one 7-bit byte per nibble-
// ish half); this packs the bits densely instead, 8 bits in for every 7
// out, which is a 14% expansion rather than 100%. Over 22,268 bytes that
// is the difference between ~4.7s and ~7.7s on the wire.
//
// Trailing bits in the final byte are zero-padded; the receiver knows
// the true length from the DATA message's length field and ignores them.
func encode7bit(payload []byte) []byte {
	out := make([]byte, (len(payload)*8+6)/7)
	totalBits := len(payload) * 8
	for i := range out {
		var v byte
		for b := range 7 {
			bit := i*7 + b
			if bit >= totalBits {
				break
			}
			v |= ((payload[bit/8] >> uint(7-bit%8)) & 1) << uint(6-b)
		}
		out[i] = v
	}
	return out
}

// decode7bit reverses encode7bit, recovering n bytes. It exists so the
// round trip can be tested here rather than only against real firmware.
func decode7bit(encoded []byte, n int) ([]byte, error) {
	if want := (n*8 + 6) / 7; len(encoded) < want {
		return nil, fmt.Errorf("need %d encoded bytes to decode %d, got %d", want, n, len(encoded))
	}
	out := make([]byte, n)
	for bit := range n * 8 {
		v := (encoded[bit/7] >> uint(6-bit%7)) & 1
		out[bit/8] |= v << uint(7-bit%8)
	}
	return out, nil
}

// buildDataMessage frames one chunk. The length field is the RAW byte
// count, not the encoded one: it is what tells the firmware how many
// bits of the last encoded byte are real padding.
func buildDataMessage(chunk []byte) ([]byte, error) {
	if len(chunk) == 0 || len(chunk) > fpgaChunkBytes {
		return nil, fmt.Errorf("chunk must be 1-%d bytes, got %d", fpgaChunkBytes, len(chunk))
	}
	encoded := encode7bit(chunk)
	msg := make([]byte, 0, 5+len(encoded))
	msg = append(msg, sysexStart, sysexFPGA, fpgaSubData, byte(len(chunk)))
	msg = append(msg, encoded...)
	return append(msg, sysexEnd), nil
}

func buildControlMessage(sub byte) []byte {
	return []byte{sysexStart, sysexFPGA, sub, sysexEnd}
}

// atmegaStream is the bridge as this file needs it: a byte pipe that
// stays open for the whole download. AtmegaBridgeRaw cannot be used --
// it opens and closes /dev/ttyS0 per call, and 455 chunks would mean 455
// open/termios/close cycles.
type atmegaStream interface {
	Write(p []byte) error
	// ReadSysex returns the bytes between the next F0 and F7, or an
	// error if none arrives before the deadline.
	ReadSysex(timeout time.Duration) ([]byte, error)
	Close() error
}

// takeSysex pulls the next complete F0..F7 frame out of buf, returning
// its body and consuming everything up to and including the F7.
//
// Bytes ahead of the F0 are dropped rather than reported: the ATmega
// emits an unsolicited REPORT_VERSION at boot, and anything else on the
// wire is noise this protocol has no use for.
func takeSysex(buf *[]byte) ([]byte, bool) {
	b := *buf
	start := bytes.IndexByte(b, sysexStart)
	if start < 0 {
		*buf = b[:0]
		return nil, false
	}
	end := bytes.IndexByte(b[start:], sysexEnd)
	if end < 0 {
		*buf = b[start:]
		return nil, false
	}
	// Copied, not aliased: the caller keeps this while the buffer it
	// came from goes on being appended to and resliced.
	body := append([]byte(nil), b[start+1:start+end]...)
	*buf = b[start+end+1:]
	return body, true
}

// fpgaReply is a decoded BEGIN/END status message.
type fpgaReply struct {
	Sub      byte
	Status   byte   // 0 = ok, non-zero = a firmware-side refusal
	Done     bool   // the real DONE pin, as read by the firmware
	Received uint32 // payload bytes the firmware counted
}

// firmware status codes, mirrored in fpga-download.ino.
const (
	fpgaStatusOK           = 0x00
	fpgaStatusDoneStuck    = 0x01 // DONE still high after the /PROGRAM pulse
	fpgaStatusBadLength    = 0x02 // DATA length field out of range
	fpgaStatusNotStreaming = 0x03 // DATA or END without a BEGIN
)

func fpgaStatusText(code byte) string {
	switch code {
	case fpgaStatusDoneStuck:
		return "DONE still reads high after the /PROGRAM pulse: the FPGA did not enter configuration mode (check the PD3 gate, /PROGRAM and DONE wiring)"
	case fpgaStatusBadLength:
		return "firmware rejected a chunk length"
	case fpgaStatusNotStreaming:
		return "firmware was not in streaming state (a BEGIN was lost)"
	default:
		return fmt.Sprintf("firmware status 0x%02x", code)
	}
}

// parseReply decodes the body of a SysEx reply (the bytes between F0 and
// F7). Layout: 7E <sub> <status> <done> <n0> <n1> <n2> <n3>, with the
// count in four 7-bit groups, little-endian, because SysEx bytes cannot
// carry bit 7.
func parseReply(body []byte) (*fpgaReply, error) {
	if len(body) < 8 {
		return nil, fmt.Errorf("short SysEx reply: %d bytes, want 8", len(body))
	}
	if body[0] != sysexFPGA {
		return nil, fmt.Errorf("SysEx reply for command 0x%02x, want 0x%02x", body[0], sysexFPGA)
	}
	var n uint32
	for i := range 4 {
		n |= uint32(body[4+i]&0x7F) << uint(7*i)
	}
	return &fpgaReply{
		Sub:      body[1],
		Status:   body[2],
		Done:     body[3] != 0,
		Received: n,
	}, nil
}

// Timeouts. BEGIN covers the firmware's own 10ms /PROGRAM hold plus the
// 100ms configuration-clear wait, END covers the startup clocks; both
// are given room far beyond what the firmware actually spends.
const (
	fpgaBeginTimeout = 3 * time.Second
	fpgaEndTimeout   = 3 * time.Second
)

// ConfigureViaATmega runs a whole download over the bridge.
//
// DATA messages are not acknowledged: at 57600 baud a 60-byte chunk
// takes ~10ms to arrive while the firmware needs well under 1ms to clock
// it out, so the firmware is never the bottleneck and a per-chunk round
// trip would only add latency. Loss is caught at the end instead -- the
// firmware counts the payload bytes it received and END reports the
// total, which this side compares against what it sent.
func ConfigureViaATmega(s atmegaStream, payload []byte) (done bool, err error) {
	if len(payload) == 0 {
		return false, fmt.Errorf("empty payload: nothing to configure")
	}

	// However this returns, the firmware is told to let go of the bus.
	// Leaving the gate low would keep the 74LS125s driving the FPGA's
	// config pins after the call returns.
	defer func() {
		if abortErr := s.Write(buildControlMessage(fpgaSubAbort)); abortErr != nil && err == nil {
			err = fmt.Errorf("release bus gate: %w", abortErr)
		}
	}()

	if err := s.Write(buildControlMessage(fpgaSubBegin)); err != nil {
		return false, fmt.Errorf("send BEGIN: %w", err)
	}
	body, err := s.ReadSysex(fpgaBeginTimeout)
	if err != nil {
		return false, fmt.Errorf("waiting for BEGIN reply: %w", err)
	}
	reply, err := parseReply(body)
	if err != nil {
		return false, fmt.Errorf("BEGIN reply: %w", err)
	}
	if reply.Status != fpgaStatusOK {
		return false, fmt.Errorf("BEGIN refused: %s", fpgaStatusText(reply.Status))
	}

	for off := 0; off < len(payload); off += fpgaChunkBytes {
		end := min(off+fpgaChunkBytes, len(payload))
		msg, err := buildDataMessage(payload[off:end])
		if err != nil {
			return false, err
		}
		if err := s.Write(msg); err != nil {
			return false, fmt.Errorf("send chunk at offset %d: %w", off, err)
		}
	}

	if err := s.Write(buildControlMessage(fpgaSubEnd)); err != nil {
		return false, fmt.Errorf("send END: %w", err)
	}
	body, err = s.ReadSysex(fpgaEndTimeout)
	if err != nil {
		return false, fmt.Errorf("waiting for END reply: %w", err)
	}
	reply, err = parseReply(body)
	if err != nil {
		return false, fmt.Errorf("END reply: %w", err)
	}
	if reply.Status != fpgaStatusOK {
		return false, fmt.Errorf("END refused: %s", fpgaStatusText(reply.Status))
	}
	// A short count means chunks were dropped on the wire, which makes
	// the DONE reading meaningless -- report the loss, not the DONE.
	if int(reply.Received) != len(payload) {
		return false, fmt.Errorf("firmware received %d of %d payload bytes: chunks were lost on the bridge", reply.Received, len(payload))
	}
	return reply.Done, nil
}
