<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# ATmega Bridge Phase D: MCP Tools Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `atmega_adc_read` and `atmega_bridge_raw`, the two new `mt7688-mcp-server` MCP tools designed in `docs/superpowers/specs/2026-07-11-atmega-phase-d-mcp-tools.md`, completing the ATmega UART bridge (Phases A-C already done and live-verified).

**Architecture:** Four code tasks plus one deploy/verify task. (1) `firmata.go` — pure, portable Firmata encode/decode logic, built TDD-first since it's fully unit-testable with no hardware. (2) `atmega.go` — parameter validation and the two public wrapper functions, mirroring `uart.go`'s existing shape exactly. (3) `atmega_linux.go`/`atmega_other.go` — real termios/syscall I/O plus the shared bridge mutex, following `uart_linux.go`'s established raw-`unix.Open` pattern (not TDD'd — this is hardware-facing code verified by the final live task, matching how every prior platform-specific file in this codebase was built). (4) `main.go` — register the two new tools, matching every existing tool's input/output struct and handler shape exactly. (5) Build, deploy, and live-verify against the real board — handled personally by the controller, matching every prior MCP-tool plan's deploy step and Phase C's flash+verify task.

**Tech Stack:** Go, `golang.org/x/sys/unix` (already a dependency, used by `uart_linux.go`/`spi_linux.go`), the existing `build.ps1`/`deploy.ps1` cross-compile+ship pipeline.

---

## Context carried in from Phase B/C (don't re-derive)

- Bridge UART device: `/dev/ttyS0`, confirmed live during Phase C.
- Wire protocol: Firmata subset — `REPORT_ANALOG` (`0xC0 | channel`, 1 byte: `0x00`/`0x01`) sent host→device; `ANALOG_MESSAGE` (`0xE0 | channel`, 2 data bytes: `value = byte1 | (byte2 << 7)`) sent device→host. Channels 0-11 (A0-A11). Baud 57600.
- **New finding from Phase C's code review, must be handled correctly:** the ATmega firmware sends one unsolicited `REPORT_VERSION` (`0xF9`, 2 data bytes) at boot, via the Firmata library's internal `printVersion()`. The decoder must recognize and silently discard this — not treat it as an error, not surface it to callers.
- `mt7688-mcp-server` already has 11 shipped tools; this plan follows the exact same file-per-concern layout (`<name>.go` portable + `<name>_linux.go` real syscalls + `<name>_other.go` non-linux stub + `<name>_test.go`) used by every prior tool, most directly comparable to `uart.go`/`uart_linux.go`/`uart_other.go`/`uart_test.go` (also a raw-`unix.Open`-based tty device, also a write-then-read shape).
- Board host for the live task: `192.168.50.216`, SSH key `provisioning/lib/keys/board_rsa`, 4 legacy algorithm flags (`KexAlgorithms=+diffie-hellman-group14-sha1`, `HostKeyAlgorithms=+ssh-rsa`, `PubkeyAcceptedAlgorithms=+ssh-rsa`, `MACs=+hmac-sha1`).
- Build: from `mt7688-mcp-server/`, run `.\build.ps1` (cross-compiles `GOOS=linux GOARCH=mipsle GOMIPS=softfloat CGO_ENABLED=0`, produces `.\mcp-server`). Deploy: `.\deploy.ps1` (ships to `/usr/local/bin/mcp-server` via `scp -O`).
- The user explicitly declined physical jumper wiring during Phase C's live verification and accepted a floating-A0-pin noise reading (jittering 0-21 across samples) as sufficient proof the protocol round-trips correctly. The same standard applies to this plan's live verification — no jumper wire is required, but the user is free to add one if they want a more deterministic signal this time.

## Task 1: `firmata.go` — pure Firmata encode/decode (TDD)

**Files:**
- Create: `mt7688-mcp-server/firmata.go`
- Test: `mt7688-mcp-server/firmata_test.go`

This is the only genuinely novel logic in this plan — everything else is thin validation/wrapper/registration code or a close variation on `uart_linux.go`'s existing termios pattern. It's also the only piece that's meaningfully unit-testable without hardware, so it gets full TDD treatment.

- [ ] **Step 1: Write the failing tests**

Create `d:\LinkItSmart7688Duo\mt7688-mcp-server\firmata_test.go`:

```go
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import "testing"

func TestEncodeReportAnalog(t *testing.T) {
	got := encodeReportAnalog(3, true)
	want := []byte{0xC3, 0x01}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("encodeReportAnalog(3, true) = %v, want %v", got, want)
	}

	got = encodeReportAnalog(11, false)
	want = []byte{0xCB, 0x00}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("encodeReportAnalog(11, false) = %v, want %v", got, want)
	}
}

func TestParseFirmataMessages_SingleCompleteMessage(t *testing.T) {
	buf := []byte{0xE0, 0x05, 0x00} // channel 0, value 5
	messages, remaining := parseFirmataMessages(buf)
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].Channel != 0 || messages[0].Value != 5 {
		t.Fatalf("expected {channel:0 value:5}, got %+v", messages[0])
	}
	if len(remaining) != 0 {
		t.Fatalf("expected empty remainder, got %v", remaining)
	}
}

func TestParseFirmataMessages_SplitAcrossReads(t *testing.T) {
	partial := []byte{0xE1, 0x0A}
	messages, remaining := parseFirmataMessages(partial)
	if len(messages) != 0 {
		t.Fatalf("expected 0 messages from a partial 2-byte buffer, got %d", len(messages))
	}
	if len(remaining) != 2 {
		t.Fatalf("expected the 2 partial bytes preserved as remainder, got %v", remaining)
	}

	full := append(remaining, 0x00)
	messages, remaining = parseFirmataMessages(full)
	if len(messages) != 1 {
		t.Fatalf("expected 1 message after completing the split read, got %d", len(messages))
	}
	if messages[0].Channel != 1 || messages[0].Value != 10 {
		t.Fatalf("expected {channel:1 value:10}, got %+v", messages[0])
	}
	if len(remaining) != 0 {
		t.Fatalf("expected empty remainder, got %v", remaining)
	}
}

func TestParseFirmataMessages_ReportVersionDiscarded(t *testing.T) {
	buf := []byte{0xF9, 0x02, 0x05} // REPORT_VERSION, major 2, minor 5
	messages, remaining := parseFirmataMessages(buf)
	if len(messages) != 0 {
		t.Fatalf("expected REPORT_VERSION to be silently discarded, got %d messages", len(messages))
	}
	if len(remaining) != 0 {
		t.Fatalf("expected empty remainder after consuming REPORT_VERSION, got %v", remaining)
	}
}

func TestParseFirmataMessages_ResyncOnStrayByte(t *testing.T) {
	buf := []byte{0x05, 0xE2, 0x03, 0x00} // stray byte, then channel 2 value 3
	messages, remaining := parseFirmataMessages(buf)
	if len(messages) != 1 {
		t.Fatalf("expected 1 message after skipping the stray byte, got %d", len(messages))
	}
	if messages[0].Channel != 2 || messages[0].Value != 3 {
		t.Fatalf("expected {channel:2 value:3}, got %+v", messages[0])
	}
	if len(remaining) != 0 {
		t.Fatalf("expected empty remainder, got %v", remaining)
	}
}

func TestParseFirmataMessages_MultipleMessagesInOneBuffer(t *testing.T) {
	buf := []byte{0xE0, 0x01, 0x00, 0xE3, 0x02, 0x00}
	messages, remaining := parseFirmataMessages(buf)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Channel != 0 || messages[0].Value != 1 {
		t.Fatalf("expected first {channel:0 value:1}, got %+v", messages[0])
	}
	if messages[1].Channel != 3 || messages[1].Value != 2 {
		t.Fatalf("expected second {channel:3 value:2}, got %+v", messages[1])
	}
	if len(remaining) != 0 {
		t.Fatalf("expected empty remainder, got %v", remaining)
	}
}

func TestParseFirmataMessages_EmptyBuffer(t *testing.T) {
	messages, remaining := parseFirmataMessages(nil)
	if len(messages) != 0 || len(remaining) != 0 {
		t.Fatalf("expected no messages and no remainder for empty input, got %d messages, %v remainder", len(messages), remaining)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run (from `d:\LinkItSmart7688Duo\mt7688-mcp-server`): `go test ./... -run TestEncodeReportAnalog -v`
Expected: FAIL — compile error, `encodeReportAnalog`/`parseFirmataMessages`/`firmataMessage` undefined.

- [ ] **Step 3: Write the implementation**

Create `d:\LinkItSmart7688Duo\mt7688-mcp-server\firmata.go`:

```go
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

// Firmata command bytes this bridge actually speaks, per
// docs/superpowers/specs/2026-07-11-atmega-phase-b-wire-protocol.md.
// REPORT_ANALOG is host->device (we send it). ANALOG_MESSAGE is
// device->host (the ATmega sends it back). REPORT_VERSION is also
// device->host - not something we asked for, but Firmata.begin(Serial1)
// on the ATmega side sends one unconditionally at every boot (confirmed
// during Phase C's code review), so the parser below must recognize and
// discard it rather than choke on an unexpected message.
const (
	firmataReportAnalog  = 0xC0
	firmataAnalogMessage = 0xE0
	firmataReportVersion = 0xF9
)

// firmataMessage is a decoded ANALOG_MESSAGE - the only message type
// parseFirmataMessages ever returns to its caller (REPORT_VERSION is
// consumed silently, see below).
type firmataMessage struct {
	Channel byte
	Value   int
}

// encodeReportAnalog builds the 2-byte REPORT_ANALOG command that
// enables or disables periodic ANALOG_MESSAGE reporting for one channel.
func encodeReportAnalog(channel byte, enable bool) []byte {
	var e byte
	if enable {
		e = 1
	}
	return []byte{firmataReportAnalog | (channel & 0x0F), e}
}

// parseFirmataMessages scans buf from the start, extracting every
// complete message it finds. It returns the decoded ANALOG_MESSAGE
// entries (in order) plus whatever trailing bytes weren't enough to form
// a complete message - the caller is expected to prepend those leftover
// bytes to the next chunk read off the wire, since a message can
// legitimately arrive split across two separate reads.
//
// REPORT_VERSION is recognized and consumed but never added to the
// returned messages - callers only ever need to look for ANALOG_MESSAGE.
// Any other leading byte is treated as a desynced/stray byte and skipped
// one at a time; this shouldn't happen in normal operation (the ATmega
// firmware only ever sends the two message shapes above), but keeps a
// single corrupted byte from wedging the caller's read loop forever.
func parseFirmataMessages(buf []byte) ([]firmataMessage, []byte) {
	var messages []firmataMessage
	i := 0
	for i < len(buf) {
		b := buf[i]
		switch {
		case b&0xF0 == firmataAnalogMessage:
			if i+2 >= len(buf) {
				return messages, buf[i:]
			}
			channel := b & 0x0F
			value := int(buf[i+1]) | (int(buf[i+2]) << 7)
			messages = append(messages, firmataMessage{Channel: channel, Value: value})
			i += 3
		case b == firmataReportVersion:
			if i+2 >= len(buf) {
				return messages, buf[i:]
			}
			i += 3
		default:
			i++
		}
	}
	return messages, buf[i:]
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run 'TestEncodeReportAnalog|TestParseFirmataMessages' -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add mt7688-mcp-server/firmata.go mt7688-mcp-server/firmata_test.go
git commit -m "feat: add Firmata encode/decode logic for ATmega bridge (Phase D)"
```

---

## Task 2: `atmega.go` — validation and wrapper functions

**Files:**
- Create: `mt7688-mcp-server/atmega.go`
- Test: `mt7688-mcp-server/atmega_test.go`

Mirrors `uart.go`'s exact shape: constants, a `validate*` function per tool, and a public wrapper that validates then delegates to a platform function (implemented in Task 3).

- [ ] **Step 1: Write the failing tests**

Create `d:\LinkItSmart7688Duo\mt7688-mcp-server\atmega_test.go`:

```go
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import "testing"

func TestValidateAtmegaADCRead_InvalidChannel(t *testing.T) {
	if err := validateAtmegaADCRead(-1, 1000); err == nil {
		t.Fatal("expected error for negative channel, got nil")
	}
	if err := validateAtmegaADCRead(12, 1000); err == nil {
		t.Fatal("expected error for channel >= 12, got nil")
	}
}

func TestValidateAtmegaADCRead_ValidChannelBoundaries(t *testing.T) {
	if err := validateAtmegaADCRead(0, 1000); err != nil {
		t.Fatalf("expected nil for channel 0, got %v", err)
	}
	if err := validateAtmegaADCRead(11, 1000); err != nil {
		t.Fatalf("expected nil for channel 11, got %v", err)
	}
}

func TestValidateAtmegaADCRead_InvalidTimeout(t *testing.T) {
	if err := validateAtmegaADCRead(0, 0); err == nil {
		t.Fatal("expected error for zero timeout_ms, got nil")
	}
	if err := validateAtmegaADCRead(0, -1); err == nil {
		t.Fatal("expected error for negative timeout_ms, got nil")
	}
	if err := validateAtmegaADCRead(0, 10001); err == nil {
		t.Fatal("expected error for timeout_ms > 10000, got nil")
	}
}

func TestValidateAtmegaADCRead_Valid(t *testing.T) {
	if err := validateAtmegaADCRead(5, 500); err != nil {
		t.Fatalf("expected nil for valid input, got %v", err)
	}
}

func TestValidateAtmegaBridgeRaw_InvalidDataLength(t *testing.T) {
	big := make([]byte, 257)
	if err := validateAtmegaBridgeRaw(big, 100, 256); err == nil {
		t.Fatal("expected error for tx_data length > 256, got nil")
	}
}

func TestValidateAtmegaBridgeRaw_EmptyDataIsValid(t *testing.T) {
	if err := validateAtmegaBridgeRaw([]byte{}, 100, 256); err != nil {
		t.Fatalf("expected nil for empty tx_data, got %v", err)
	}
}

func TestValidateAtmegaBridgeRaw_InvalidTimeout(t *testing.T) {
	if err := validateAtmegaBridgeRaw([]byte{0x01}, -1, 256); err == nil {
		t.Fatal("expected error for negative timeout_ms, got nil")
	}
	if err := validateAtmegaBridgeRaw([]byte{0x01}, 25501, 256); err == nil {
		t.Fatal("expected error for timeout_ms > 25500, got nil")
	}
}

func TestValidateAtmegaBridgeRaw_ZeroTimeoutIsValid(t *testing.T) {
	if err := validateAtmegaBridgeRaw([]byte{0x01}, 0, 256); err != nil {
		t.Fatalf("expected nil for zero timeout_ms, got %v", err)
	}
}

func TestValidateAtmegaBridgeRaw_InvalidMaxReadBytes(t *testing.T) {
	if err := validateAtmegaBridgeRaw([]byte{0x01}, 100, 0); err == nil {
		t.Fatal("expected error for zero max_read_bytes, got nil")
	}
	if err := validateAtmegaBridgeRaw([]byte{0x01}, 100, 4097); err == nil {
		t.Fatal("expected error for max_read_bytes > 4096, got nil")
	}
}

func TestValidateAtmegaBridgeRaw_Valid(t *testing.T) {
	if err := validateAtmegaBridgeRaw([]byte{0x41}, 500, 256); err != nil {
		t.Fatalf("expected nil for valid input, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestValidateAtmega -v`
Expected: FAIL — compile error, `validateAtmegaADCRead`/`validateAtmegaBridgeRaw` undefined.

- [ ] **Step 3: Write the implementation**

Create `d:\LinkItSmart7688Duo\mt7688-mcp-server\atmega.go`:

```go
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import "fmt"

// atmegaBridgeDevPath is the ATmega UART bridge, confirmed live during
// Phase C via dmesg's pinmux allocation log (uart0 group -> pins 12/13 ->
// 10000c00.uartlite -> ttyS0) - a different physical UART from the one
// uart_transact uses (ttyS1, pins 45/46).
const atmegaBridgeDevPath = "/dev/ttyS0"

// atmegaBridgeBaud is fixed, not caller-adjustable like uart_transact's
// baud_rate - the ATmega firmware's Firmata.begin(Serial1) is hardcoded
// to this rate (see atmega-firmware/adc-bridge/adc-bridge.ino), so a
// caller-supplied mismatch would just desync communication with no
// benefit.
const atmegaBridgeBaud = 57600

// atmegaChannelCount matches ANALOG_CHANNEL_COUNT in the ATmega firmware
// (confirmed against the real board's analog_pin_to_channel_PGM array
// during Phase B/C - 12 channels, A0-A11).
const atmegaChannelCount = 12

func validateAtmegaADCRead(channel int, timeoutMs int) error {
	if channel < 0 || channel >= atmegaChannelCount {
		return fmt.Errorf("channel must be 0-%d, got %d", atmegaChannelCount-1, channel)
	}
	if timeoutMs < 1 || timeoutMs > 10000 {
		return fmt.Errorf("timeout_ms must be 1-10000, got %d", timeoutMs)
	}
	return nil
}

// AtmegaADCRead validates input then performs the enable/wait/disable
// sequence against atmegaBridgeDevPath. Real termios/read-loop work is in
// atmega_linux.go (atmega_other.go stubs it out on non-linux dev
// machines), mirroring the i2c.go/spi.go/uart.go platform splits.
func AtmegaADCRead(channel int, timeoutMs int) (int, error) {
	if err := validateAtmegaADCRead(channel, timeoutMs); err != nil {
		return 0, err
	}
	return atmegaADCRead(byte(channel), timeoutMs)
}

func validateAtmegaBridgeRaw(txData []byte, timeoutMs int, maxReadBytes int) error {
	if len(txData) > 256 {
		return fmt.Errorf("tx_data length must be 0-256, got %d", len(txData))
	}
	if timeoutMs < 0 || timeoutMs > 25500 {
		return fmt.Errorf("timeout_ms must be 0-25500, got %d", timeoutMs)
	}
	if maxReadBytes < 1 || maxReadBytes > 4096 {
		return fmt.Errorf("max_read_bytes must be 1-4096, got %d", maxReadBytes)
	}
	return nil
}

// AtmegaBridgeRaw validates input then performs a write-then-timed-read
// transaction against atmegaBridgeDevPath, structurally identical to
// UARTTransact but fixed to this bridge's device and baud rate.
func AtmegaBridgeRaw(txData []byte, timeoutMs int, maxReadBytes int) ([]byte, error) {
	if err := validateAtmegaBridgeRaw(txData, timeoutMs, maxReadBytes); err != nil {
		return nil, err
	}
	return atmegaBridgeRaw(txData, timeoutMs, maxReadBytes)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestValidateAtmega -v`
Expected: all PASS. (Full-package `go test ./...` will still fail at this point — `atmegaADCRead`/`atmegaBridgeRaw` platform functions don't exist yet, that's Task 3.)

- [ ] **Step 5: Commit**

```bash
git add mt7688-mcp-server/atmega.go mt7688-mcp-server/atmega_test.go
git commit -m "feat: add ATmega bridge validation and wrapper functions"
```

---

## Task 3: `atmega_linux.go`/`atmega_other.go` — platform I/O and the bridge mutex

**Files:**
- Create: `mt7688-mcp-server/atmega_linux.go`
- Create: `mt7688-mcp-server/atmega_other.go`

Not TDD'd — this is real termios/syscall code that can't be meaningfully exercised without the actual board, matching how `uart_linux.go`/`spi_linux.go`/`i2c_linux.go` were all written directly and verified by a later live-deploy task rather than a unit test. Written carefully against the same, already-proven termios pattern `uart_linux.go` uses (raw `unix.Open`, not `os.File`, for the VMIN/VTIME-vs-Go's-runtime-poller reason documented there).

- [ ] **Step 1: Write `atmega_linux.go`**

Create `d:\LinkItSmart7688Duo\mt7688-mcp-server\atmega_linux.go`:

```go
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
```

- [ ] **Step 2: Write `atmega_other.go`**

Create `d:\LinkItSmart7688Duo\mt7688-mcp-server\atmega_other.go`:

```go
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

//go:build !linux

package main

import "fmt"

// atmegaADCRead/atmegaBridgeRaw on non-linux build hosts. This stub
// exists solely so `go build`/`go test` succeed on a non-linux dev
// machine; the real target is always GOOS=linux GOARCH=mipsle (see
// build.ps1), where atmega_linux.go is compiled in instead of this file.
// Matches uart_other.go's/i2c_other.go's/spi_other.go's existing
// rationale exactly.
func atmegaADCRead(channel byte, timeoutMs int) (int, error) {
	return 0, fmt.Errorf("atmega bridge is only supported on linux (target device); this build is not linux")
}

func atmegaBridgeRaw(txData []byte, timeoutMs int, maxReadBytes int) ([]byte, error) {
	return nil, fmt.Errorf("atmega bridge is only supported on linux (target device); this build is not linux")
}
```

- [ ] **Step 3: Verify the full package builds and all existing tests still pass**

Run: `go build ./...`
Expected: success, no errors.

Run: `go test ./...`
Expected: all PASS (this now includes Task 1's and Task 2's tests running for real against the complete package, plus every pre-existing test in the package).

- [ ] **Step 4: Commit**

```bash
git add mt7688-mcp-server/atmega_linux.go mt7688-mcp-server/atmega_other.go
git commit -m "feat: implement ATmega bridge platform I/O and shared mutex"
```

---

## Task 4: `main.go` — register the two new tools

**Files:**
- Modify: `mt7688-mcp-server/main.go`

Add two input/output struct pairs, two handlers, and two `mcp.AddTool` registrations, in exactly the same shape as every existing tool in this file (see `uartTransactHandler`/`UARTTransactInput`/`UARTTransactOutput` immediately above for the closest match — same `[]int`-to-`[]byte` conversion-with-range-check pattern).

- [ ] **Step 1: Add the new input/output structs and handlers**

In `d:\LinkItSmart7688Duo\mt7688-mcp-server\main.go`, add this block immediately after the existing `i2sPlayHandler` function (i.e., right before `func main()`):

```go
type AtmegaADCReadInput struct {
	Channel   int `json:"channel" jsonschema:"analog channel to read, 0-11 (A0-A11)"`
	TimeoutMs int `json:"timeout_ms" jsonschema:"how long to wait for a reading, in milliseconds (1-10000)"`
}
type AtmegaADCReadOutput struct {
	Value int `json:"value" jsonschema:"the ATmega's raw 10-bit ADC reading, 0-1023"`
}

func atmegaADCReadHandler(ctx context.Context, req *mcp.CallToolRequest, input AtmegaADCReadInput) (*mcp.CallToolResult, AtmegaADCReadOutput, error) {
	value, err := AtmegaADCRead(input.Channel, input.TimeoutMs)
	if err != nil {
		return nil, AtmegaADCReadOutput{}, err
	}
	return nil, AtmegaADCReadOutput{Value: value}, nil
}

type AtmegaBridgeRawInput struct {
	TxData       []int `json:"tx_data" jsonschema:"raw bytes to send over the ATmega bridge (each 0-255); may be empty for a read-only/listen call"`
	TimeoutMs    int   `json:"timeout_ms" jsonschema:"how long to wait for a reply, in milliseconds (0-25500, rounded down to the nearest 100ms); 0 means non-blocking"`
	MaxReadBytes int   `json:"max_read_bytes" jsonschema:"maximum bytes to read, 1-4096"`
}
type AtmegaBridgeRawOutput struct {
	RxData []int `json:"rx_data" jsonschema:"bytes received within the timeout window; may be shorter than max_read_bytes or empty if nothing arrived"`
}

func atmegaBridgeRawHandler(ctx context.Context, req *mcp.CallToolRequest, input AtmegaBridgeRawInput) (*mcp.CallToolResult, AtmegaBridgeRawOutput, error) {
	txData := make([]byte, len(input.TxData))
	for i, v := range input.TxData {
		if v < 0 || v > 255 {
			return nil, AtmegaBridgeRawOutput{}, fmt.Errorf("tx_data[%d] out of byte range (0-255): %d", i, v)
		}
		txData[i] = byte(v)
	}
	rxData, err := AtmegaBridgeRaw(txData, input.TimeoutMs, input.MaxReadBytes)
	if err != nil {
		return nil, AtmegaBridgeRawOutput{}, err
	}
	out := make([]int, len(rxData))
	for i, b := range rxData {
		out[i] = int(b)
	}
	return nil, AtmegaBridgeRawOutput{RxData: out}, nil
}
```

- [ ] **Step 2: Register both tools**

In `main.go`'s `func main()`, add these two lines immediately after the existing `i2s_play` registration line, before `if err := server.Run(...)`:

```go
	mcp.AddTool(server, &mcp.Tool{Name: "atmega_adc_read", Description: "Read one analog channel (0-11, A0-A11) from the ATmega32U4 side over the UART bridge (/dev/ttyS0), using the Firmata REPORT_ANALOG/ANALOG_MESSAGE protocol. Blocks until a reading for that channel arrives or timeout_ms elapses."}, atmegaADCReadHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "atmega_bridge_raw", Description: "Send raw bytes to the ATmega32U4 over the UART bridge (/dev/ttyS0, fixed 57600 baud) and read back whatever arrives within timeout_ms. Low-level escape hatch for speaking Firmata (or any future ATmega firmware protocol) directly; most callers should use atmega_adc_read instead."}, atmegaBridgeRawHandler)
```

- [ ] **Step 3: Build and test**

Run: `go build ./...`
Expected: success.

Run: `go test ./...`
Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add mt7688-mcp-server/main.go
git commit -m "feat: register atmega_adc_read and atmega_bridge_raw MCP tools"
```

---

## Task 5: Build, deploy, and live-verify on real hardware

**Files:** None (deployment/verification), plus a `CLAUDE.md`/spec status update in the final step.

This task requires live board SSH access and a live MCP client session — per this project's established practice, it's executed directly by the controller, not delegated to a subagent (same as Phase C's Task 3 and the final deploy/verify step of every prior MCP-tool plan).

- [ ] **Step 1: Build and deploy**

```powershell
cd d:\LinkItSmart7688Duo\mt7688-mcp-server
.\build.ps1
.\deploy.ps1
```

Expected: `build.ps1` produces `.\mcp-server`; `deploy.ps1` reports `deployed`.

- [ ] **Step 2: Reload the MCP client session**

Ask the user to reload/reconnect their MCP client session (the same step done after every prior deploy in this project) so the two new tools become available as `atmega_adc_read`/`atmega_bridge_raw` (surfaced to this session as `mcp__linkit7688-hw__atmega_adc_read` / `mcp__linkit7688-hw__atmega_bridge_raw`, matching the naming pattern of the 11 already-connected tools).

- [ ] **Step 3: Verify `atmega_adc_read`**

Call `atmega_adc_read` with `channel=0`, `timeout_ms=1000`. Expected: returns `{"value": N}` where N is 0-1023, with no error. Per the project's established precedent from Phase C, A0 does not need a jumper wire connected — a floating pin producing a real (if noisy) value is sufficient confirmation that the tool's enable/wait/decode/disable sequence works end to end against the real firmware. Call it 2-3 times and confirm the values aren't suspiciously identical every time (some jitter is expected and is itself part of the confirmation, same as Phase C's manual test).

If this fails or times out, **stop and report BLOCKED** with the exact error — don't guess a workaround; check whether the ATmega firmware is still running (a fresh `run-avrdude` re-flash may be needed if the board was power-cycled since Phase C) before assuming the new Go code is wrong.

- [ ] **Step 4: Verify `atmega_bridge_raw`**

Call `atmega_bridge_raw` with `tx_data=[192, 1]` (i.e. `0xC0, 0x01` — `REPORT_ANALOG` enable for channel 0), `timeout_ms=1000`, `max_read_bytes=60`. Expected: `rx_data` comes back as a sequence of 3-byte groups each starting with `224` (`0xE0`), matching the shape Phase C's manual `hexdump` test already confirmed by hand. Then call `atmega_bridge_raw` again with `tx_data=[192, 0]` (disable) to stop the stream, matching Phase C's cleanup step.

- [ ] **Step 5: Update `CLAUDE.md`**

Add a line to the ATmega section of `CLAUDE.md` noting the bridge is now fully wired up end-to-end: `atmega_adc_read`/`atmega_bridge_raw` are live MCP tools, completing Phases A-D.

- [ ] **Step 6: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: record Phase D live verification, ATmega bridge complete"
```

---

## Plan self-review notes

**Spec coverage:** every section of the Phase D design spec maps to a task — tool surface (Task 4), Firmata encode/decode (Task 1), read loop/timeout semantics (Task 3's `atmegaADCRead`), the shared bridge mutex (Task 3's `atmegaBridgeMu`), validation bounds (Task 2), file layout (Tasks 1-3 create exactly the files the spec named), error handling (Task 3's wrapped errors, Task 2's validation errors), and live verification of both tools (Task 5).

**Type consistency check:** `firmataMessage{Channel byte, Value int}` (Task 1) is used identically in Task 3's `atmegaADCRead` loop (`m.Channel`, `m.Value`). `atmegaADCRead(channel byte, timeoutMs int) (int, error)` and `atmegaBridgeRaw(txData []byte, timeoutMs int, maxReadBytes int) ([]byte, error)` (Task 2's expected platform-function signatures) match exactly what Task 3 implements and what `atmega_other.go`'s stub signatures declare. `AtmegaADCRead`/`AtmegaBridgeRaw` (Task 2, exported) match what Task 4's handlers call.
