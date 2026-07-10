<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# SPI + PWM MCP Tools Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `spi_transfer`, `pwm_configure`, `pwm_set_enabled`, and `pwm_read` as MCP tools on `mt7688-mcp-server`, extending the existing GPIO/I2C server per `docs/superpowers/specs/2026-07-11-spi-pwm-design.md`.

**Architecture:** Same shape as the existing GPIO/I2C code: a platform-independent validation/orchestration file per subsystem, a `//go:build linux` file with the real syscall/ioctl work, a `//go:build !linux` stub so the module still builds on the Windows dev machine, and new `mcp.AddTool` registrations in `main.go`. SPI needs hand-derived MIPS ioctl request numbers (the generic Linux `_IOC` encoding most references use is wrong for this arch — see design doc's "Known risk" section); those constants and their derivation live in the platform-independent file so they're covered by `go test` on Windows even though the real transfer isn't. PWM is plain sysfs file I/O, no ioctl needed, following `gpio.go`'s existing pattern directly.

**Tech Stack:** Go 1.26, `golang.org/x/sys/unix` (already a dependency, used the same way `i2c_linux.go` already uses it), no new dependencies.

---

## Task 1: Extend the GPIO blocklist for SPI/PWM pins

**Files:**
- Modify: `mt7688-mcp-server/blocklist.go`
- Modify: `mt7688-mcp-server/blocklist_test.go`

Pins 6-10 (SPI) and 18-19 (PWM) are about to be claimed by dedicated tools. A `gpio_write` on any of them wouldn't error today — it would just be electrically meaningless (or contend with the real controller), since pinmux already has them selected to their real function. Same rationale as the existing 12/13 entry for the ATmega bridge UART.

- [ ] **Step 1: Update the test to expect the new pins as blocked**

Replace the full contents of `mt7688-mcp-server/blocklist_test.go`:

```go
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import "testing"

func TestCheckBlocked_BlockedPins(t *testing.T) {
	blocked := []int{6, 7, 8, 9, 10, 12, 13, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 38}
	for _, pin := range blocked {
		if err := CheckBlocked(pin); err == nil {
			t.Errorf("pin %d: expected error, got nil", pin)
		}
	}
}

func TestCheckBlocked_AllowedPins(t *testing.T) {
	allowed := []int{0, 1, 30, 39, 95}
	for _, pin := range allowed {
		if err := CheckBlocked(pin); err != nil {
			t.Errorf("pin %d: expected nil, got %v", pin, err)
		}
	}
}
```

- [ ] **Step 2: Run the test, verify it fails**

Run (from `mt7688-mcp-server/`): `go test ./... -run TestCheckBlocked -v`
Expected: `TestCheckBlocked_BlockedPins` FAILs on pins 6, 7, 8, 9, 10, 18, 19 (not yet in `blockedPins`).

- [ ] **Step 3: Add the new entries to the blocklist**

Replace the full contents of `mt7688-mcp-server/blocklist.go`:

```go
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import "fmt"

var blockedPins = map[int]string{
	6:  "SPI CS1 (spi_cs1 pinmux group)",
	7:  "SPI bus (fixed-function pin, no GPIO alternative)",
	8:  "SPI bus (fixed-function pin, no GPIO alternative)",
	9:  "SPI bus (fixed-function pin, no GPIO alternative)",
	10: "SPI bus (fixed-function pin, no GPIO alternative)",
	12: "ATmega bridge UART (uartlite, ttyS0)",
	13: "ATmega bridge UART (uartlite, ttyS0)",
	18: "PWM channel 0 (pwm0 pinmux group)",
	19: "PWM channel 1 (pwm1 pinmux group)",
	20: "console",
	21: "console",
	22: "SDXC/extroot",
	23: "SDXC/extroot",
	24: "SDXC/extroot",
	25: "SDXC/extroot",
	26: "SDXC/extroot",
	27: "SDXC/extroot",
	28: "SDXC/extroot",
	29: "SDXC/extroot",
	38: "ATmega ISP reset (held by the core)",
}

func CheckBlocked(pin int) error {
	if reason, blocked := blockedPins[pin]; blocked {
		return fmt.Errorf("gpio %d is blocked: %s", pin, reason)
	}
	return nil
}
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `go test ./... -run TestCheckBlocked -v`
Expected: PASS

- [ ] **Step 5: Run the full test suite to check for regressions**

Run: `go test ./...`
Expected: PASS (all packages)

- [ ] **Step 6: Commit**

```bash
git add mt7688-mcp-server/blocklist.go mt7688-mcp-server/blocklist_test.go
git commit -m "feat: block GPIO access to pins claimed by SPI/PWM"
```

---

## Task 2: SPI validation, device discovery, and MIPS ioctl constants

**Files:**
- Create: `mt7688-mcp-server/spi.go`
- Create: `mt7688-mcp-server/spi_test.go`

Everything in this task is pure Go with no OS-specific calls, so it compiles and is fully tested by plain `go test ./...` on the Windows dev machine — including the MIPS ioctl-encoding arithmetic, which is the highest-risk part of the whole SPI implementation (see design doc's "Known risk" section: internet-sourced `spidev` ioctl constants are computed for the generic/x86 `_IOC` layout, which is wrong for MIPS).

- [ ] **Step 1: Write the failing tests**

Create `mt7688-mcp-server/spi_test.go`:

```go
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import (
	"os"
	"testing"
)

func TestValidateSPITransfer_InvalidMode(t *testing.T) {
	if err := validateSPITransfer(4, 1000000, 8, []byte{0x01}); err == nil {
		t.Fatal("expected error for mode > 3, got nil")
	}
	if err := validateSPITransfer(-1, 1000000, 8, []byte{0x01}); err == nil {
		t.Fatal("expected error for negative mode, got nil")
	}
}

func TestValidateSPITransfer_InvalidBitsPerWord(t *testing.T) {
	if err := validateSPITransfer(0, 1000000, 0, []byte{0x01}); err == nil {
		t.Fatal("expected error for bits_per_word 0, got nil")
	}
	if err := validateSPITransfer(0, 1000000, 33, []byte{0x01}); err == nil {
		t.Fatal("expected error for bits_per_word > 32, got nil")
	}
}

func TestValidateSPITransfer_InvalidSpeed(t *testing.T) {
	if err := validateSPITransfer(0, 0, 8, []byte{0x01}); err == nil {
		t.Fatal("expected error for zero speed_hz, got nil")
	}
	if err := validateSPITransfer(0, -1, 8, []byte{0x01}); err == nil {
		t.Fatal("expected error for negative speed_hz, got nil")
	}
}

func TestValidateSPITransfer_InvalidDataLength(t *testing.T) {
	if err := validateSPITransfer(0, 1000000, 8, []byte{}); err == nil {
		t.Fatal("expected error for empty tx_data, got nil")
	}
	big := make([]byte, 257)
	if err := validateSPITransfer(0, 1000000, 8, big); err == nil {
		t.Fatal("expected error for tx_data length > 256, got nil")
	}
}

func TestValidateSPITransfer_Valid(t *testing.T) {
	if err := validateSPITransfer(0, 1000000, 8, []byte{0x01, 0x02}); err != nil {
		t.Fatalf("expected nil for valid input, got %v", err)
	}
}

func TestDiscoverSPIDevice_None(t *testing.T) {
	dir := t.TempDir()
	old := spiDevGlob
	spiDevGlob = dir + "/spidev*"
	defer func() { spiDevGlob = old }()

	if _, err := discoverSPIDevice(); err == nil {
		t.Fatal("expected error when no device nodes exist, got nil")
	}
}

func TestDiscoverSPIDevice_ExactlyOne(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/spidev32766.1"
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	old := spiDevGlob
	spiDevGlob = dir + "/spidev*"
	defer func() { spiDevGlob = old }()

	got, err := discoverSPIDevice()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got != path {
		t.Fatalf("expected %q, got %q", path, got)
	}
}

func TestDiscoverSPIDevice_Multiple(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"spidev0.0", "spidev0.1"} {
		if err := os.WriteFile(dir+"/"+name, nil, 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	old := spiDevGlob
	spiDevGlob = dir + "/spidev*"
	defer func() { spiDevGlob = old }()

	if _, err := discoverSPIDevice(); err == nil {
		t.Fatal("expected error for multiple device nodes, got nil")
	}
}

func TestMipsIOC_WRMode(t *testing.T) {
	// Hand-derived: _IOW('k', 1, 1) on MIPS.
	// MIPS _IOC layout: WRITE=4, DIRSHIFT=29, TYPESHIFT=8, SIZESHIFT=16.
	// (4<<29) | (0x6b<<8) | (1<<0) | (1<<16)
	// = 0x80000000 | 0x00006B00 | 0x00000001 | 0x00010000 = 0x80016B01
	got := mipsIOC(mipsIOCWrite, spiIOCMagic, 1, 1)
	want := uintptr(0x80016B01)
	if got != want {
		t.Fatalf("SPI_IOC_WR_MODE: got %#x, want %#x", got, want)
	}
	if spiIOCWRMode != want {
		t.Fatalf("spiIOCWRMode: got %#x, want %#x", spiIOCWRMode, want)
	}
}

func TestMipsIOC_Message1(t *testing.T) {
	// Hand-derived: _IOW('k', 0, char[32]) on MIPS.
	// (4<<29) | (0x6b<<8) | (0<<0) | (32<<16)
	// = 0x80000000 | 0x00006B00 | 0 | 0x00200000 = 0x80206B00
	got := mipsIOC(mipsIOCWrite, spiIOCMagic, 0, spiIOCTransferSize)
	want := uintptr(0x80206B00)
	if got != want {
		t.Fatalf("SPI_IOC_MESSAGE(1): got %#x, want %#x", got, want)
	}
	if spiIOCMessage1 != want {
		t.Fatalf("spiIOCMessage1: got %#x, want %#x", spiIOCMessage1, want)
	}
}
```

- [ ] **Step 2: Run the tests, verify they fail to compile**

Run: `go test ./... -run 'TestValidateSPITransfer|TestDiscoverSPIDevice|TestMipsIOC' -v`
Expected: FAIL — `validateSPITransfer`, `spiDevGlob`, `discoverSPIDevice`, `mipsIOC`, `mipsIOCWrite`, `spiIOCMagic`, `spiIOCTransferSize`, `spiIOCWRMode`, `spiIOCMessage1` all undefined.

- [ ] **Step 3: Write the implementation**

Create `mt7688-mcp-server/spi.go`:

```go
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import (
	"fmt"
	"path/filepath"
)

// validateSPITransfer runs before any device access, matching the
// ReadI2C/WriteI2C pattern in i2c.go.
func validateSPITransfer(mode int, speedHz int, bitsPerWord int, txData []byte) error {
	if mode < 0 || mode > 3 {
		return fmt.Errorf("mode must be 0-3, got %d", mode)
	}
	if bitsPerWord < 1 || bitsPerWord > 32 {
		return fmt.Errorf("bits_per_word must be 1-32, got %d", bitsPerWord)
	}
	if speedHz <= 0 {
		return fmt.Errorf("speed_hz must be positive, got %d", speedHz)
	}
	if len(txData) == 0 || len(txData) > 256 {
		return fmt.Errorf("tx_data length must be 1-256, got %d", len(txData))
	}
	return nil
}

// spiDevGlob is a var (not a const) so tests can point it at a temp
// directory instead of /dev, exercising the 0/1/many-matches logic
// without needing real hardware.
var spiDevGlob = "/dev/spidev*"

// discoverSPIDevice finds the sole SPI device node on the system. Only
// one exists on this board today (/dev/spidev32766.1 - see design doc),
// with a kernel-internal bus number an LLM caller has no way to guess.
// Returns an error naming all matches if there isn't exactly one, so a
// second device (if one is ever added) fails loudly instead of picking
// one arbitrarily.
func discoverSPIDevice() (string, error) {
	matches, err := filepath.Glob(spiDevGlob)
	if err != nil {
		return "", fmt.Errorf("glob %s: %w", spiDevGlob, err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no SPI device node found matching %s", spiDevGlob)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("multiple SPI device nodes found %v: spi_transfer needs exactly one, this tool doesn't yet support selecting between them", matches)
	}
	return matches[0], nil
}

// --- MIPS ioctl request-number derivation ---
//
// MIPS uses a different _IOC bit layout than the generic one most
// internet-sourced ioctl constants assume (arch/mips/include/uapi/asm/
// ioctl.h vs include/uapi/asm-generic/ioctl.h in the Linux kernel
// source - shared with powerpc/sparc, the historical "BSD-style" ioctl
// encoding). Generic (x86/arm): SIZEBITS=14, dir values NONE=0/WRITE=1/
// READ=2. MIPS: SIZEBITS=13, dir values NONE=1/READ=2/WRITE=4. Getting
// this wrong doesn't fail loudly at compile time - it silently targets
// the wrong ioctl number at runtime. These constants are derived from
// the formula directly (not copied from an x86/generic reference) and
// cross-checked against hand-derived values in spi_test.go.
const (
	mipsIOCNRBITS   = 8
	mipsIOCTYPEBITS = 8
	mipsIOCSIZEBITS = 13

	mipsIOCNRSHIFT   = 0
	mipsIOCTYPESHIFT = mipsIOCNRSHIFT + mipsIOCNRBITS
	mipsIOCSIZESHIFT = mipsIOCTYPESHIFT + mipsIOCTYPEBITS
	mipsIOCDIRSHIFT  = mipsIOCSIZESHIFT + mipsIOCSIZEBITS

	mipsIOCNone  = 1
	mipsIOCRead  = 2
	mipsIOCWrite = 4
)

func mipsIOC(dir, typ, nr, size uintptr) uintptr {
	return (dir << mipsIOCDIRSHIFT) | (typ << mipsIOCTYPESHIFT) | (nr << mipsIOCNRSHIFT) | (size << mipsIOCSIZESHIFT)
}

const spiIOCMagic = 'k' // SPI_IOC_MAGIC, from <linux/spi/spidev.h>

// spiIOCTransferSize is sizeof(struct spi_ioc_transfer) from
// <linux/spi/spidev.h>: two 8-byte pointers (tx_buf, rx_buf), two 4-byte
// fields (len, speed_hz), one 2-byte field (delay_usecs), and six 1-byte
// fields (bits_per_word, cs_change, tx_nbits, rx_nbits, word_delay_usecs,
// pad) = 8+8+4+4+2+1+1+1+1+1+1 = 32 bytes. Stable uapi, unchanged since
// spidev was added to the kernel.
const spiIOCTransferSize = 32

var (
	spiIOCWRMode   = mipsIOC(mipsIOCWrite, spiIOCMagic, 1, 1)                    // SPI_IOC_WR_MODE
	spiIOCMessage1 = mipsIOC(mipsIOCWrite, spiIOCMagic, 0, spiIOCTransferSize)   // SPI_IOC_MESSAGE(1)
)
```

- [ ] **Step 4: Run the tests, verify they pass**

Run: `go test ./... -run 'TestValidateSPITransfer|TestDiscoverSPIDevice|TestMipsIOC' -v`
Expected: PASS

- [ ] **Step 5: Run the full test suite to check for regressions**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add mt7688-mcp-server/spi.go mt7688-mcp-server/spi_test.go
git commit -m "feat: add SPI validation, device discovery, and MIPS ioctl constants"
```

---

## Task 3: SPI transfer — Linux implementation and non-Linux stub

**Files:**
- Create: `mt7688-mcp-server/spi_linux.go`
- Create: `mt7688-mcp-server/spi_other.go`
- Modify: `mt7688-mcp-server/spi.go` (add the `TransferSPI` wrapper)

This is the part that cannot be unit-tested off real hardware (same as `i2c_linux.go`/`i2c_other.go` — no dedicated test file for either). The risky arithmetic (Task 2) is already tested; this task is the plumbing that uses it. Real verification happens live, in Task 6.

- [ ] **Step 1: Add the `TransferSPI` wrapper to `spi.go`**

Append to the end of `mt7688-mcp-server/spi.go`:

```go

// TransferSPI validates input, discovers the sole SPI device node, and
// performs a full-duplex transfer. Real ioctl work is in spi_linux.go
// (spi_other.go stubs it out on non-linux dev machines), mirroring the
// i2c.go / i2c_linux.go / i2c_other.go split.
func TransferSPI(mode int, speedHz int, bitsPerWord int, txData []byte) (rxData []byte, device string, err error) {
	if err := validateSPITransfer(mode, speedHz, bitsPerWord, txData); err != nil {
		return nil, "", err
	}
	device, err = discoverSPIDevice()
	if err != nil {
		return nil, "", err
	}
	rxData, err = transferSPI(device, mode, speedHz, bitsPerWord, txData)
	if err != nil {
		return nil, "", err
	}
	return rxData, device, nil
}
```

- [ ] **Step 2: Write the Linux implementation**

Create `mt7688-mcp-server/spi_linux.go`:

```go
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

//go:build linux

package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// transferSPI opens device, sets the SPI mode via SPI_IOC_WR_MODE, then
// issues a single full-duplex transfer via SPI_IOC_MESSAGE(1). The
// spi_ioc_transfer struct is built by hand into a 32-byte buffer
// (spiIOCTransferSize, see spi.go) rather than a Go struct, since the
// kernel ABI's tx_buf/rx_buf fields are raw 64-bit pointers with no safe
// Go-native representation. txData/rxData must stay alive and unmoved
// until the ioctl returns - Go's current garbage collector doesn't move
// heap objects, and both slices remain referenced by local variables for
// the whole function, so this holds.
func transferSPI(device string, mode int, speedHz int, bitsPerWord int, txData []byte) ([]byte, error) {
	f, err := os.OpenFile(device, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", device, err)
	}
	defer f.Close()

	if err := unix.IoctlSetInt(int(f.Fd()), uint(spiIOCWRMode), mode); err != nil {
		return nil, fmt.Errorf("set spi mode %d: %w", mode, err)
	}

	rxData := make([]byte, len(txData))
	xfer := make([]byte, spiIOCTransferSize)
	binary.LittleEndian.PutUint64(xfer[0:8], uint64(uintptr(unsafe.Pointer(&txData[0]))))
	binary.LittleEndian.PutUint64(xfer[8:16], uint64(uintptr(unsafe.Pointer(&rxData[0]))))
	binary.LittleEndian.PutUint32(xfer[16:20], uint32(len(txData)))
	binary.LittleEndian.PutUint32(xfer[20:24], uint32(speedHz))
	// xfer[24:26] delay_usecs = 0
	xfer[26] = byte(bitsPerWord)
	// xfer[27:32] cs_change, tx_nbits, rx_nbits, word_delay_usecs, pad = 0

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), spiIOCMessage1, uintptr(unsafe.Pointer(&xfer[0])))
	if errno != 0 {
		return nil, fmt.Errorf("spi transfer: %w", errno)
	}
	return rxData, nil
}
```

- [ ] **Step 3: Write the non-Linux stub**

Create `mt7688-mcp-server/spi_other.go`:

```go
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

//go:build !linux

package main

import "fmt"

// transferSPI on non-linux build hosts (e.g. Windows, where this module
// is developed and cross-compiled from). This stub exists solely so
// `go build`/`go test` succeed on a non-linux dev machine; the real
// target is always GOOS=linux GOARCH=mipsle (see build.ps1), where
// spi_linux.go is compiled in instead of this file. Matches i2c_other.go's
// existing rationale exactly.
func transferSPI(device string, mode int, speedHz int, bitsPerWord int, txData []byte) ([]byte, error) {
	return nil, fmt.Errorf("spi is only supported on linux (target device); this build is not linux")
}
```

- [ ] **Step 4: Build and run the full test suite**

Run: `go build ./...`
Expected: succeeds (on Windows this compiles `spi.go` + `spi_other.go`; `spi_linux.go` is excluded by its build tag)

Run: `go test ./...`
Expected: PASS, no regressions

- [ ] **Step 5: Commit**

```bash
git add mt7688-mcp-server/spi.go mt7688-mcp-server/spi_linux.go mt7688-mcp-server/spi_other.go
git commit -m "feat: implement SPI full-duplex transfer"
```

---

## Task 4: PWM configure/enable/read

**Files:**
- Create: `mt7688-mcp-server/pwm.go`
- Create: `mt7688-mcp-server/pwm_test.go`

Plain sysfs file I/O, no ioctl — follows `gpio.go`'s pattern directly (no platform split needed, same as `gpio.go` has none). Channel is restricted to `{0, 1}`: `pwmchip0` reports `npwm=4`, but only channels 0/1 have a verified pin mapping (see design doc's live hardware findings).

- [ ] **Step 1: Write the failing tests**

Create `mt7688-mcp-server/pwm_test.go`:

```go
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import "testing"

func TestConfigurePWM_InvalidChannel(t *testing.T) {
	if err := ConfigurePWM(2, 1000, 500); err == nil {
		t.Fatal("expected error for unverified channel 2, got nil")
	}
}

func TestConfigurePWM_InvalidPeriod(t *testing.T) {
	// channel 0 is valid, so these must fail on period/duty validation,
	// not channel validation - proving validation order and staying
	// testable off real hardware (same pattern as gpio_test.go's
	// TestSetGPIODirection_InvalidDirection).
	if err := ConfigurePWM(0, 0, 0); err == nil {
		t.Fatal("expected error for zero period, got nil")
	}
	if err := ConfigurePWM(0, -1, 0); err == nil {
		t.Fatal("expected error for negative period, got nil")
	}
}

func TestConfigurePWM_DutyExceedsPeriod(t *testing.T) {
	if err := ConfigurePWM(0, 1000, 1500); err == nil {
		t.Fatal("expected error for duty_ns > period_ns, got nil")
	}
}

func TestConfigurePWM_NegativeDuty(t *testing.T) {
	if err := ConfigurePWM(0, 1000, -1); err == nil {
		t.Fatal("expected error for negative duty_ns, got nil")
	}
}

func TestSetPWMEnabled_InvalidChannel(t *testing.T) {
	if err := SetPWMEnabled(3, true); err == nil {
		t.Fatal("expected error for unverified channel 3, got nil")
	}
}

func TestReadPWM_InvalidChannel(t *testing.T) {
	if _, err := ReadPWM(2); err == nil {
		t.Fatal("expected error for unverified channel 2, got nil")
	}
}
```

- [ ] **Step 2: Run the tests, verify they fail to compile**

Run: `go test ./... -run 'TestConfigurePWM|TestSetPWMEnabled|TestReadPWM' -v`
Expected: FAIL — `ConfigurePWM`, `SetPWMEnabled`, `ReadPWM` undefined.

- [ ] **Step 3: Write the implementation**

Create `mt7688-mcp-server/pwm.go`:

```go
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

const pwmSysfsPath = "/sys/class/pwm/pwmchip0"

// validatePWMChannel restricts operations to the two PWM channels
// confirmed (2026-07-11, see design doc) to be routed to a known pin
// (channel 0 = GPIO18, channel 1 = GPIO19). pwmchip0 reports npwm=4, but
// channels 2/3 have no verified pin mapping in mt7688_pinmux - refusing
// them is the same "don't guess" posture applied to i2c_write against an
// unknown device address.
func validatePWMChannel(channel int) error {
	if channel != 0 && channel != 1 {
		return fmt.Errorf("pwm channel %d not verified routed to a known pin (only 0 and 1 are supported)", channel)
	}
	return nil
}

// exportPWM exports channel via the sysfs PWM ABI. Idempotent under
// concurrent invocation, same EBUSY-tolerant pattern as exportGPIO in
// gpio.go and for the same reason: each MCP client session is a separate
// OS process and two sessions can race to export the same channel.
func exportPWM(channel int) error {
	f, err := os.OpenFile(pwmSysfsPath+"/export", os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open export: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(strconv.Itoa(channel)); err != nil {
		if errors.Is(err, syscall.EBUSY) {
			return nil // already exported by a concurrent session
		}
		return fmt.Errorf("write export: %w", err)
	}
	return nil
}

func pwmChannelPath(channel int) string {
	return fmt.Sprintf("%s/pwm%d", pwmSysfsPath, channel)
}

func readPWMAttr(channel int, attr string) (string, error) {
	path := pwmChannelPath(channel) + "/" + attr
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", attr, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func writePWMAttr(channel int, attr string, value string) error {
	path := pwmChannelPath(channel) + "/" + attr
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", attr, err)
	}
	defer f.Close()
	if _, err := f.WriteString(value); err != nil {
		return fmt.Errorf("write %s: %w", attr, err)
	}
	return nil
}

// ConfigurePWM sets channel's period and duty cycle (both in
// nanoseconds). The sysfs PWM ABI requires duty_cycle <= period at all
// times - writing duty_cycle first when it wouldn't fit under the
// *current* period fails. This reads the current period first and picks
// a write order that never violates the constraint mid-update (see
// design doc, "PWM write ordering").
func ConfigurePWM(channel int, periodNs int, dutyNs int) error {
	if err := validatePWMChannel(channel); err != nil {
		return err
	}
	if periodNs <= 0 {
		return fmt.Errorf("period_ns must be positive, got %d", periodNs)
	}
	if dutyNs < 0 || dutyNs > periodNs {
		return fmt.Errorf("duty_ns must be 0-period_ns (%d), got %d", periodNs, dutyNs)
	}
	if err := exportPWM(channel); err != nil {
		return err
	}

	currentPeriodStr, err := readPWMAttr(channel, "period")
	if err != nil {
		return err
	}
	currentPeriod, err := strconv.Atoi(currentPeriodStr)
	if err != nil {
		return fmt.Errorf("parse current period %q: %w", currentPeriodStr, err)
	}

	if periodNs >= currentPeriod {
		if err := writePWMAttr(channel, "period", strconv.Itoa(periodNs)); err != nil {
			return err
		}
		if err := writePWMAttr(channel, "duty_cycle", strconv.Itoa(dutyNs)); err != nil {
			return err
		}
	} else {
		if err := writePWMAttr(channel, "duty_cycle", strconv.Itoa(dutyNs)); err != nil {
			return err
		}
		if err := writePWMAttr(channel, "period", strconv.Itoa(periodNs)); err != nil {
			return err
		}
	}
	return nil
}

func SetPWMEnabled(channel int, enabled bool) error {
	if err := validatePWMChannel(channel); err != nil {
		return err
	}
	if err := exportPWM(channel); err != nil {
		return err
	}
	v := "0"
	if enabled {
		v = "1"
	}
	return writePWMAttr(channel, "enable", v)
}

type PWMState struct {
	PeriodNs int
	DutyNs   int
	Enabled  bool
	Polarity string
}

func ReadPWM(channel int) (PWMState, error) {
	if err := validatePWMChannel(channel); err != nil {
		return PWMState{}, err
	}
	if err := exportPWM(channel); err != nil {
		return PWMState{}, err
	}
	periodStr, err := readPWMAttr(channel, "period")
	if err != nil {
		return PWMState{}, err
	}
	period, err := strconv.Atoi(periodStr)
	if err != nil {
		return PWMState{}, fmt.Errorf("parse period %q: %w", periodStr, err)
	}
	dutyStr, err := readPWMAttr(channel, "duty_cycle")
	if err != nil {
		return PWMState{}, err
	}
	duty, err := strconv.Atoi(dutyStr)
	if err != nil {
		return PWMState{}, fmt.Errorf("parse duty_cycle %q: %w", dutyStr, err)
	}
	enabledStr, err := readPWMAttr(channel, "enable")
	if err != nil {
		return PWMState{}, err
	}
	polarity, err := readPWMAttr(channel, "polarity")
	if err != nil {
		return PWMState{}, err
	}
	return PWMState{
		PeriodNs: period,
		DutyNs:   duty,
		Enabled:  enabledStr == "1",
		Polarity: polarity,
	}, nil
}
```

- [ ] **Step 4: Run the tests, verify they pass**

Run: `go test ./... -run 'TestConfigurePWM|TestSetPWMEnabled|TestReadPWM' -v`
Expected: PASS

- [ ] **Step 5: Run the full test suite to check for regressions**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add mt7688-mcp-server/pwm.go mt7688-mcp-server/pwm_test.go
git commit -m "feat: implement PWM configure/enable/read"
```

---

## Task 5: Wire up the four new MCP tools

**Files:**
- Modify: `mt7688-mcp-server/main.go`
- Modify: `mt7688-mcp-server/main_test.go`

`tx_data`/`rx_data` are `[]int`, not `[]byte` — applying the standing rule this codebase adopted after the `i2c_read`/`i2c_write` bug: Go's `encoding/json` always base64-encodes `[]byte`, which the MCP SDK's schema generator doesn't account for.

- [ ] **Step 1: Write the failing tests**

Append to the end of `mt7688-mcp-server/main_test.go`:

```go

func TestSPITransferHandler_DataElementOutOfRange(t *testing.T) {
	_, _, err := spiTransferHandler(context.Background(), nil, SPITransferInput{
		Mode: 0, SpeedHz: 1000000, BitsPerWord: 8, TxData: []int{1, 300, 2},
	})
	if err == nil {
		t.Fatal("expected error for tx_data element > 255, got nil")
	}
}

func TestSPITransferHandler_DataElementNegative(t *testing.T) {
	_, _, err := spiTransferHandler(context.Background(), nil, SPITransferInput{
		Mode: 0, SpeedHz: 1000000, BitsPerWord: 8, TxData: []int{-1},
	})
	if err == nil {
		t.Fatal("expected error for negative tx_data element, got nil")
	}
}
```

- [ ] **Step 2: Run the tests, verify they fail to compile**

Run: `go test ./... -run TestSPITransferHandler -v`
Expected: FAIL — `spiTransferHandler`, `SPITransferInput` undefined.

- [ ] **Step 3: Add the tool structs, handlers, and registrations**

In `mt7688-mcp-server/main.go`, insert the following new types and handlers after the existing `i2cWriteHandler` function (i.e. after line 100 of the current file, before `func main()`):

```go

type SPITransferInput struct {
	Mode        int   `json:"mode" jsonschema:"SPI mode 0-3 (CPOL/CPHA)"`
	SpeedHz     int   `json:"speed_hz" jsonschema:"clock speed in Hz"`
	BitsPerWord int   `json:"bits_per_word" jsonschema:"bits per word, 1-32"`
	TxData      []int `json:"tx_data" jsonschema:"bytes to send (each 0-255); rx_data comes back the same length"`
}
type SPITransferOutput struct {
	RxData []int  `json:"rx_data" jsonschema:"bytes received during the transfer, same length as tx_data"`
	Device string `json:"device" jsonschema:"the /dev/spidev* node this transfer used"`
}

func spiTransferHandler(ctx context.Context, req *mcp.CallToolRequest, input SPITransferInput) (*mcp.CallToolResult, SPITransferOutput, error) {
	txData := make([]byte, len(input.TxData))
	for i, v := range input.TxData {
		if v < 0 || v > 255 {
			return nil, SPITransferOutput{}, fmt.Errorf("tx_data[%d] out of byte range (0-255): %d", i, v)
		}
		txData[i] = byte(v)
	}
	rxData, device, err := TransferSPI(input.Mode, input.SpeedHz, input.BitsPerWord, txData)
	if err != nil {
		return nil, SPITransferOutput{}, err
	}
	out := make([]int, len(rxData))
	for i, b := range rxData {
		out[i] = int(b)
	}
	return nil, SPITransferOutput{RxData: out, Device: device}, nil
}

type PWMConfigureInput struct {
	Channel  int `json:"channel" jsonschema:"PWM channel: 0 (GPIO18) or 1 (GPIO19)"`
	PeriodNs int `json:"period_ns" jsonschema:"PWM period in nanoseconds"`
	DutyNs   int `json:"duty_ns" jsonschema:"PWM duty cycle in nanoseconds, must be 0-period_ns"`
}
type PWMConfigureOutput struct {
	OK bool `json:"ok"`
}

func pwmConfigureHandler(ctx context.Context, req *mcp.CallToolRequest, input PWMConfigureInput) (*mcp.CallToolResult, PWMConfigureOutput, error) {
	if err := ConfigurePWM(input.Channel, input.PeriodNs, input.DutyNs); err != nil {
		return nil, PWMConfigureOutput{}, err
	}
	return nil, PWMConfigureOutput{OK: true}, nil
}

type PWMSetEnabledInput struct {
	Channel int  `json:"channel" jsonschema:"PWM channel: 0 (GPIO18) or 1 (GPIO19)"`
	Enabled bool `json:"enabled" jsonschema:"true to start the PWM output, false to stop it"`
}
type PWMSetEnabledOutput struct {
	OK bool `json:"ok"`
}

func pwmSetEnabledHandler(ctx context.Context, req *mcp.CallToolRequest, input PWMSetEnabledInput) (*mcp.CallToolResult, PWMSetEnabledOutput, error) {
	if err := SetPWMEnabled(input.Channel, input.Enabled); err != nil {
		return nil, PWMSetEnabledOutput{}, err
	}
	return nil, PWMSetEnabledOutput{OK: true}, nil
}

type PWMReadInput struct {
	Channel int `json:"channel" jsonschema:"PWM channel: 0 (GPIO18) or 1 (GPIO19)"`
}
type PWMReadOutput struct {
	PeriodNs int    `json:"period_ns" jsonschema:"current period in nanoseconds"`
	DutyNs   int    `json:"duty_ns" jsonschema:"current duty cycle in nanoseconds"`
	Enabled  bool   `json:"enabled" jsonschema:"true if the PWM output is currently running"`
	Polarity string `json:"polarity" jsonschema:"\"normal\" or \"inversed\""`
}

func pwmReadHandler(ctx context.Context, req *mcp.CallToolRequest, input PWMReadInput) (*mcp.CallToolResult, PWMReadOutput, error) {
	state, err := ReadPWM(input.Channel)
	if err != nil {
		return nil, PWMReadOutput{}, err
	}
	return nil, PWMReadOutput{
		PeriodNs: state.PeriodNs,
		DutyNs:   state.DutyNs,
		Enabled:  state.Enabled,
		Polarity: state.Polarity,
	}, nil
}
```

Then, inside `func main()`, add these four registrations right after the existing `i2c_write` registration (before `if err := server.Run(...)`):

```go
	mcp.AddTool(server, &mcp.Tool{Name: "spi_transfer", Description: "Full-duplex SPI transfer on the board's sole SPI device node. Send tx_data (dummy 0x00 bytes for a read-only transfer), get back rx_data of the same length."}, spiTransferHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "pwm_configure", Description: "Set a PWM channel's period and duty cycle in nanoseconds. Only channels 0 (GPIO18) and 1 (GPIO19) are supported."}, pwmConfigureHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "pwm_set_enabled", Description: "Start or stop a PWM channel's output. Call pwm_configure first to set a period/duty."}, pwmSetEnabledHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "pwm_read", Description: "Read a PWM channel's current period, duty cycle, enabled state, and polarity"}, pwmReadHandler)
```

- [ ] **Step 4: Run the tests, verify they pass**

Run: `go test ./... -run TestSPITransferHandler -v`
Expected: PASS

- [ ] **Step 5: Run the full test suite and build to check for regressions**

Run: `go test ./...`
Expected: PASS

Run: `go build ./...`
Expected: succeeds

- [ ] **Step 6: Commit**

```bash
git add mt7688-mcp-server/main.go mt7688-mcp-server/main_test.go
git commit -m "feat: register spi_transfer, pwm_configure, pwm_set_enabled, pwm_read MCP tools"
```

---

## Task 6: Cross-compile, deploy, and live-verify on real hardware

**Files:**
- Modify: `docs/superpowers/specs/2026-07-11-spi-pwm-design.md` (status + live verification section)
- Modify: `README.md`, `README.zh-TW.md` (status section)

No code changes in this task — this is the same "manual verification against the real board" step the original GPIO/I2C phase had, and it's the actual gate: `go test` cannot exercise real ioctl/sysfs behavior, and the MIPS ioctl encoding (Task 2/3) is unverified against a real kernel until this task runs. A wrong ioctl request number for `SPI_IOC_MESSAGE` doesn't corrupt anything on this driver — spidev's ioctl handler returns `-ENOTTY`/`-EINVAL` for any request number it doesn't recognize — so this is safe to try directly rather than needing a dry-run.

- [ ] **Step 1: Build**

Run (from `mt7688-mcp-server/`): `.\build.ps1`
Expected: produces the mipsle binary with no errors.

- [ ] **Step 2: Deploy**

Run: `.\deploy.ps1`
Expected: binary copied to `/usr/local/bin/mcp-server` on the board.

- [ ] **Step 3: Reload the MCP client session**

Tell the user to reload/reconnect their MCP session (e.g. restart the Claude Code session using `.mcp.json`) so the four new tools become visible. Confirm all 9 tools now show up (5 existing + 4 new).

- [ ] **Step 4: Verify the GPIO blocklist extension live**

Call `gpio_write` (or `gpio_set_direction`) on pin 18. Expected: rejected with the "PWM channel 0 (pwm0 pinmux group)" blocklist message — confirms Task 1 took effect on the real board, not just in `go test`.

- [ ] **Step 5: Verify PWM**

Call `pwm_configure` with `channel=0, period_ns=1000000, duty_ns=500000` (1kHz, 50% duty). Then call `pwm_read` with `channel=0` and confirm `period_ns`/`duty_ns` round-trip to what was set. Then call `pwm_configure` again with `channel=0, period_ns=200000, duty_ns=100000` — this forces the new period (200000) below the previously-stored period (1000000), exercising the "write duty_cycle before period" branch of the ordering logic (see design doc, "PWM write ordering") that the first call's period-increase never touches; confirm `pwm_read` shows the new values with no error. Then call `pwm_set_enabled` with `channel=0, enabled=true`. If the user has a multimeter or LED handy on GPIO18, ask them to confirm they see PWM activity (e.g. a dimmed LED, or ~2.5V average on a multimeter set to DC). Finish by calling `pwm_set_enabled` with `channel=0, enabled=false` to leave the pin in a stopped, safe state. Also confirm `pwm_configure`/`pwm_read`/`pwm_set_enabled` on `channel=2` are rejected with the "not verified routed to a known pin" error.

- [ ] **Step 6: Verify SPI**

Call `spi_transfer` with `mode=0, speed_hz=1000000, bits_per_word=8, tx_data=[0,0,0,0]`. Expected: no error, `device` field reads `/dev/spidev32766.1`, `rx_data` is 4 bytes long (content is whatever's on the bus with nothing attached — not meaningful without a real device, but the call succeeding at all confirms the MIPS ioctl encoding from Task 2/3 is accepted by the kernel). If a jumper wire is available to bridge MOSI (pin 8, need to confirm exact silkscreen label against the board) to MISO (pin 9) for a physical loopback, repeat with `tx_data=[85,170,15,240]` (`0x55,0xAA,0x0F,0xF0`) and confirm `rx_data` comes back identical — this is the strongest possible confirmation the transfer is byte-correct end to end, but is optional if no jumper wire is convenient.

- [ ] **Step 7: Record what was found**

If anything in Steps 4-6 didn't match expectations, fix the relevant code (Task 1-5 files) and repeat this task's verification steps before continuing.

Update `docs/superpowers/specs/2026-07-11-spi-pwm-design.md`: change the `Status:` line at the top to `implemented, verified live on real hardware <date>`, and add a "Live verification" section (mirroring the style of the 2026-07-10 spec's own such section) documenting exactly what was confirmed and any bugs found and fixed during this pass.

Update `README.md` and `README.zh-TW.md`: remove the "SPI, PWM, the ATmega UART bridge, and I2S are surveyed... but not yet exposed as tools" sentence in the Status section, replacing it with something reflecting SPI/PWM now shipped and I2S/ATmega bridge still deferred (keep the tool list and status wording consistent with what Task 5 actually registered).

- [ ] **Step 8: Commit**

```bash
git add docs/superpowers/specs/2026-07-11-spi-pwm-design.md README.md README.zh-TW.md
git commit -m "docs: record SPI/PWM live verification results"
```
