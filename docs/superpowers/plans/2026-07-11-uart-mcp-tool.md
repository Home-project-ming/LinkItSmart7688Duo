<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# UART MCP Tool Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `uart_transact` as an MCP tool on `mt7688-mcp-server`, per `docs/superpowers/specs/2026-07-11-uart-design.md`.

**Architecture:** Same three-way platform split as the existing SPI code: a platform-independent validation/orchestration file, a `//go:build linux` file with the real termios/syscall work, and a `//go:build !linux` stub. Unlike SPI, the ioctl request numbers here come pre-generated and arch-correct from `golang.org/x/sys/unix` rather than being hand-derived — but the exact `Termios` struct field names and which ioctl request constant (`TCGETS`/`TCGETS2`) applies on this platform are **not assumed**; Task 3 starts with a mandatory step that reads the real `golang.org/x/sys/unix` source before writing any implementation code, the same discipline that would have caught the SPI `SPI_IOC_WR_MODE` bug earlier if it had been applied there from the start.

**Tech Stack:** Go 1.26, `golang.org/x/sys/unix` (already a dependency, pinned at v0.47.0 in `go.mod`), no new dependencies.

---

## Task 1: Extend the GPIO blocklist for UART1 pins

**Files:**
- Modify: `mt7688-mcp-server/blocklist.go`
- Modify: `mt7688-mcp-server/blocklist_test.go`

Pins 45/46 are about to be claimed by `uart_transact`. Same rationale as the SPI/PWM blocklist entries: a `gpio_write` on them wouldn't error today, it would just be electrically meaningless once this tool and the kernel's `uart1` driver own them.

- [ ] **Step 1: Update the test to expect the new pins as blocked**

Replace the full contents of `mt7688-mcp-server/blocklist_test.go`:

```go
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import "testing"

func TestCheckBlocked_BlockedPins(t *testing.T) {
	blocked := []int{6, 7, 8, 9, 10, 12, 13, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 38, 45, 46}
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
Expected: `TestCheckBlocked_BlockedPins` FAILs on pins 45 and 46 (not yet in `blockedPins`).

- [ ] **Step 3: Add the new entries to the blocklist**

In `mt7688-mcp-server/blocklist.go`, add these two lines to the `blockedPins` map (any position is fine, but keeping numeric order matches the existing style):

```go
	45: "UART1 (uart1 pinmux group)",
	46: "UART1 (uart1 pinmux group)",
```

The full map should read:

```go
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
	45: "UART1 (uart1 pinmux group)",
	46: "UART1 (uart1 pinmux group)",
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
git commit -m "feat: block GPIO access to pins claimed by UART1"
```

---

## Task 2: UART input validation (platform-independent)

**Files:**
- Create: `mt7688-mcp-server/uart.go`
- Create: `mt7688-mcp-server/uart_test.go`

Pure Go, no OS-specific calls — compiles and is fully tested by plain `go test ./...` on the Windows dev machine.

- [ ] **Step 1: Write the failing tests**

Create `mt7688-mcp-server/uart_test.go`:

```go
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import "testing"

func TestValidateUARTTransact_InvalidBaudRate(t *testing.T) {
	if err := validateUARTTransact(0, []byte{0x01}, 100, 256); err == nil {
		t.Fatal("expected error for zero baud_rate, got nil")
	}
	if err := validateUARTTransact(-1, []byte{0x01}, 100, 256); err == nil {
		t.Fatal("expected error for negative baud_rate, got nil")
	}
	if err := validateUARTTransact(2500001, []byte{0x01}, 100, 256); err == nil {
		t.Fatal("expected error for baud_rate above uartBaseBaud, got nil")
	}
}

func TestValidateUARTTransact_InvalidDataLength(t *testing.T) {
	big := make([]byte, 257)
	if err := validateUARTTransact(9600, big, 100, 256); err == nil {
		t.Fatal("expected error for tx_data length > 256, got nil")
	}
}

func TestValidateUARTTransact_EmptyDataIsValid(t *testing.T) {
	if err := validateUARTTransact(9600, []byte{}, 100, 256); err != nil {
		t.Fatalf("expected nil for empty tx_data (read-only call), got %v", err)
	}
}

func TestValidateUARTTransact_InvalidTimeout(t *testing.T) {
	if err := validateUARTTransact(9600, []byte{0x01}, -1, 256); err == nil {
		t.Fatal("expected error for negative read_timeout_ms, got nil")
	}
	if err := validateUARTTransact(9600, []byte{0x01}, 25501, 256); err == nil {
		t.Fatal("expected error for read_timeout_ms > 25500, got nil")
	}
}

func TestValidateUARTTransact_ZeroTimeoutIsValid(t *testing.T) {
	if err := validateUARTTransact(9600, []byte{0x01}, 0, 256); err != nil {
		t.Fatalf("expected nil for zero read_timeout_ms (non-blocking), got %v", err)
	}
}

func TestValidateUARTTransact_InvalidMaxReadBytes(t *testing.T) {
	if err := validateUARTTransact(9600, []byte{0x01}, 100, 0); err == nil {
		t.Fatal("expected error for zero max_read_bytes, got nil")
	}
	if err := validateUARTTransact(9600, []byte{0x01}, 100, 4097); err == nil {
		t.Fatal("expected error for max_read_bytes > 4096, got nil")
	}
}

func TestValidateUARTTransact_Valid(t *testing.T) {
	if err := validateUARTTransact(115200, []byte{0x41, 0x54, 0x0D}, 500, 256); err != nil {
		t.Fatalf("expected nil for valid input, got %v", err)
	}
}
```

- [ ] **Step 2: Run the tests, verify they fail to compile**

Run: `go test ./... -run TestValidateUARTTransact -v`
Expected: FAIL — `validateUARTTransact` undefined.

- [ ] **Step 3: Write the implementation**

Create `mt7688-mcp-server/uart.go`:

```go
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import "fmt"

// uartDevPath is the sole free, pin-routed general-purpose UART on this
// board (uart1, pins 45/46, confirmed via dmesg: "10000d00.uart1: ttyS1
// ... base_baud = 2500000" - see design doc). uart0 (ttyS0, pins 12/13) is
// the ATmega bridge and uart2 (ttyS2, pins 20/21) is the console; both are
// already GPIO-blocklisted (Task 1) and neither is touched by this file.
// Hardcoded, not discovered like SPI's device path - unlike SPI's
// dynamically-numbered bus, this is a stable kernel-assigned path and
// there's exactly one candidate.
const uartDevPath = "/dev/ttyS1"

// uartBaseBaud is uart1's real hardware base_baud (16550A UART, confirmed
// via dmesg - see design doc), used to bound the baud_rate parameter to
// something the hardware can actually represent rather than leaving it
// unbounded.
const uartBaseBaud = 2500000

// validateUARTTransact runs before any device access, matching the
// pattern in i2c.go/spi.go. Unlike spi.go's validateSPITransfer, tx_data
// length 0 is valid here (a read-only/listen call) - UART's write and
// read aren't structurally symmetric the way SPI's are.
func validateUARTTransact(baudRate int, txData []byte, readTimeoutMs int, maxReadBytes int) error {
	if baudRate < 1 || baudRate > uartBaseBaud {
		return fmt.Errorf("baud_rate must be 1-%d, got %d", uartBaseBaud, baudRate)
	}
	if len(txData) > 256 {
		return fmt.Errorf("tx_data length must be 0-256, got %d", len(txData))
	}
	if readTimeoutMs < 0 || readTimeoutMs > 25500 {
		return fmt.Errorf("read_timeout_ms must be 0-25500, got %d", readTimeoutMs)
	}
	if maxReadBytes < 1 || maxReadBytes > 4096 {
		return fmt.Errorf("max_read_bytes must be 1-4096, got %d", maxReadBytes)
	}
	return nil
}
```

- [ ] **Step 4: Run the tests, verify they pass**

Run: `go test ./... -run TestValidateUARTTransact -v`
Expected: PASS

- [ ] **Step 5: Run the full test suite to check for regressions**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add mt7688-mcp-server/uart.go mt7688-mcp-server/uart_test.go
git commit -m "feat: add UART input validation"
```

---

## Task 3: UART transact — Linux implementation and non-Linux stub

**Files:**
- Create: `mt7688-mcp-server/uart_linux.go`
- Create: `mt7688-mcp-server/uart_other.go`
- Modify: `mt7688-mcp-server/uart.go` (add the `UARTTransact` wrapper)

This is the part that can't be unit-tested off real hardware — same as `i2c_linux.go`/`spi_linux.go`. **Unlike those, this task starts with a mandatory research step, not a direct implementation step**, because the design doc explicitly deferred two platform-API questions rather than guessing at them (see `docs/superpowers/specs/2026-07-11-uart-design.md`'s "Baud rate configuration" section): the exact `Termios` struct field names/types on `linux/mipsle`, and whether `TCGETS`/`TCSETS` or `TCGETS2`/`TCSETS2` is the correct ioctl request pair for this platform's `x/sys/unix` version. Guessing here is exactly what produced two real, hardware-faulting bugs in the SPI implementation's ioctl code (Task 3 of the SPI/PWM plan, commit `be2ee49`/fix `24ab242`) — this task exists to not repeat that.

- [ ] **Step 1: Investigate the real `golang.org/x/sys/unix` API before writing any code**

Find the module cache path:

Run: `go env GOMODCACHE`

The pinned version is `golang.org/x/sys v0.47.0` (confirm: `type go.mod | findstr golang.org/x/sys` on Windows, or open `mt7688-mcp-server/go.mod` directly).

In `<GOMODCACHE>\golang.org\x\sys@v0.47.0\unix\`, find and read:

1. The file defining `type Termios struct` for `linux`/`mipsle` (likely `ztypes_linux_mipsle.go`, or a shared `ztypes_linux.go` if the layout doesn't vary by arch — check both). Write down every field name and type you find (expecting something like `Iflag`, `Oflag`, `Cflag`, `Lflag`, `Cc`, and possibly `Ispeed`/`Ospeed` — but confirm, don't assume).
2. The file defining ioctl request constants for this platform (likely `zerrors_linux_mipsle.go` or similar). Search specifically for: `TCGETS`, `TCGETS2`, `TCSETS`, `TCSETS2`, `BOTHER`, `CBAUD`, `CS8`, `CSIZE`, `CREAD`, `CLOCAL`, `PARENB`, `CSTOPB`, `VMIN`, `VTIME`. Write down which of these actually exist for this arch, and their values if you can find them.
3. Wherever `IoctlGetTermios`/`IoctlSetTermios` are defined (likely `ioctl.go`, `ioctl_linux.go`, or similar). Write down their exact signatures — parameter types, and specifically what type/value the `req` parameter expects (a `TCGETS`-style constant, or something else).
4. Whether `unix.Open`, `unix.Read`, `unix.Write`, `unix.Close` exist with the signatures you'd expect for raw fd-based I/O (they're extremely standard, this is just a sanity check, not expected to surprise).

**If `TCGETS2`/`TCSETS2` or `BOTHER` or `Ispeed`/`Ospeed` fields don't exist for this platform/version, or `IoctlGetTermios`/`IoctlSetTermios` don't match the expected shape, STOP and report NEEDS_CONTEXT** rather than guessing or forcing Step 2's code to compile some other way — describe exactly what you found instead, so the plan can be corrected with real information rather than another guess.

- [ ] **Step 2: Add the `UARTTransact` wrapper to `uart.go`**

Append to the end of `mt7688-mcp-server/uart.go`:

```go

// UARTTransact validates input then performs a write-then-timed-read
// transaction against uartDevPath. Real termios/syscall work is in
// uart_linux.go (uart_other.go stubs it out on non-linux dev machines),
// mirroring the i2c.go / spi.go platform splits.
func UARTTransact(baudRate int, txData []byte, readTimeoutMs int, maxReadBytes int) ([]byte, error) {
	if err := validateUARTTransact(baudRate, txData, readTimeoutMs, maxReadBytes); err != nil {
		return nil, err
	}
	return uartTransact(baudRate, txData, readTimeoutMs, maxReadBytes)
}
```

- [ ] **Step 3: Write the Linux implementation, using what Step 1 found**

The following is a starting hypothesis, based on the standard/expected shape of this API — **adjust field names, constants, or function calls to match whatever Step 1's investigation actually found before treating this as final.** If everything Step 1 found matches what's used here, use it as-is.

Create `mt7688-mcp-server/uart_linux.go`:

```go
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
```

- [ ] **Step 4: Write the non-Linux stub**

Create `mt7688-mcp-server/uart_other.go`:

```go
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

//go:build !linux

package main

import "fmt"

// uartTransact on non-linux build hosts (e.g. Windows, where this module
// is developed and cross-compiled from). This stub exists solely so
// `go build`/`go test` succeed on a non-linux dev machine; the real
// target is always GOOS=linux GOARCH=mipsle (see build.ps1), where
// uart_linux.go is compiled in instead of this file. Matches
// i2c_other.go's/spi_other.go's existing rationale exactly.
func uartTransact(baudRate int, txData []byte, readTimeoutMs int, maxReadBytes int) ([]byte, error) {
	return nil, fmt.Errorf("uart is only supported on linux (target device); this build is not linux")
}
```

- [ ] **Step 5: Build and run the full test suite**

Run: `go build ./...`
Expected: succeeds (on Windows this compiles `uart.go` + `uart_other.go`; `uart_linux.go` is excluded by its build tag)

Run: `go test ./...`
Expected: PASS, no regressions

Also cross-compile for the real target to actually type-check `uart_linux.go` (it can't be exercised on Windows, but it CAN be compiled and vetted for the real target — do this regardless of whether Step 1's findings matched the hypothesis in Step 3):

Run: `$env:GOOS="linux"; $env:GOARCH="mipsle"; $env:GOMIPS="softfloat"; $env:CGO_ENABLED="0"; go build ./...; go vet ./...; Remove-Item Env:\GOOS,Env:\GOARCH,Env:\GOMIPS,Env:\CGO_ENABLED`
Expected: both succeed with exit 0

- [ ] **Step 6: Commit**

```bash
git add mt7688-mcp-server/uart.go mt7688-mcp-server/uart_linux.go mt7688-mcp-server/uart_other.go
git commit -m "feat: implement UART transact (write + timed read)"
```

---

## Task 4: Wire up the `uart_transact` MCP tool

**Files:**
- Modify: `mt7688-mcp-server/main.go`
- Modify: `mt7688-mcp-server/main_test.go`

`tx_data`/`rx_data` are `[]int`, not `[]byte` — the same standing rule applied to `i2c_read`/`i2c_write`/`spi_transfer`.

- [ ] **Step 1: Write the failing tests**

Append to the end of `mt7688-mcp-server/main_test.go`:

```go

func TestUartTransactHandler_DataElementOutOfRange(t *testing.T) {
	_, _, err := uartTransactHandler(context.Background(), nil, UARTTransactInput{
		BaudRate: 9600, TxData: []int{1, 300, 2}, ReadTimeoutMs: 100, MaxReadBytes: 256,
	})
	if err == nil {
		t.Fatal("expected error for tx_data element > 255, got nil")
	}
}

func TestUartTransactHandler_DataElementNegative(t *testing.T) {
	_, _, err := uartTransactHandler(context.Background(), nil, UARTTransactInput{
		BaudRate: 9600, TxData: []int{-1}, ReadTimeoutMs: 100, MaxReadBytes: 256,
	})
	if err == nil {
		t.Fatal("expected error for negative tx_data element, got nil")
	}
}
```

- [ ] **Step 2: Run the tests, verify they fail to compile**

Run: `go test ./... -run TestUartTransactHandler -v`
Expected: FAIL — `uartTransactHandler`, `UARTTransactInput` undefined.

- [ ] **Step 3: Add the tool struct, handler, and registration**

In `mt7688-mcp-server/main.go`, insert the following after the existing `pwmReadHandler` function (i.e. after the last SPI/PWM handler, before `func main()`):

```go

type UARTTransactInput struct {
	BaudRate      int   `json:"baud_rate" jsonschema:"baud rate, 1-2500000"`
	TxData        []int `json:"tx_data" jsonschema:"bytes to send (each 0-255); may be empty for a read-only/listen call"`
	ReadTimeoutMs int   `json:"read_timeout_ms" jsonschema:"how long to wait for a reply, in milliseconds (0-25500, rounded down to the nearest 100ms); 0 means non-blocking"`
	MaxReadBytes  int   `json:"max_read_bytes" jsonschema:"maximum bytes to read, 1-4096"`
}
type UARTTransactOutput struct {
	RxData []int `json:"rx_data" jsonschema:"bytes received within the timeout window; may be shorter than max_read_bytes or empty if nothing arrived"`
}

func uartTransactHandler(ctx context.Context, req *mcp.CallToolRequest, input UARTTransactInput) (*mcp.CallToolResult, UARTTransactOutput, error) {
	txData := make([]byte, len(input.TxData))
	for i, v := range input.TxData {
		if v < 0 || v > 255 {
			return nil, UARTTransactOutput{}, fmt.Errorf("tx_data[%d] out of byte range (0-255): %d", i, v)
		}
		txData[i] = byte(v)
	}
	rxData, err := UARTTransact(input.BaudRate, txData, input.ReadTimeoutMs, input.MaxReadBytes)
	if err != nil {
		return nil, UARTTransactOutput{}, err
	}
	out := make([]int, len(rxData))
	for i, b := range rxData {
		out[i] = int(b)
	}
	return nil, UARTTransactOutput{RxData: out}, nil
}
```

Then, inside `func main()`, add this registration right after the existing `pwm_read` registration (before `if err := server.Run(...)`):

```go
	mcp.AddTool(server, &mcp.Tool{Name: "uart_transact", Description: "Write bytes to /dev/ttyS1 (uart1, pins 45/46) and read back whatever arrives within read_timeout_ms. tx_data may be empty for a read-only/listen call. A timeout with no data is not an error - it means nothing was received in the window."}, uartTransactHandler)
```

- [ ] **Step 4: Run the tests, verify they pass**

Run: `go test ./... -run TestUartTransactHandler -v`
Expected: PASS

- [ ] **Step 5: Run the full test suite and build to check for regressions**

Run: `go test ./...`
Expected: PASS

Run: `go build ./...`
Expected: succeeds

- [ ] **Step 6: Commit**

```bash
git add mt7688-mcp-server/main.go mt7688-mcp-server/main_test.go
git commit -m "feat: register uart_transact MCP tool"
```

---

## Task 5: Cross-compile, deploy, and live-verify on real hardware

**Files:**
- Modify: `docs/superpowers/specs/2026-07-11-uart-design.md` (status + live verification section)
- Modify: `README.md`, `README.zh-TW.md` (status section)

No code changes in this task. This is the real gate for everything Task 3 built — `go test`/`go vet` on the cross-compiled target proves the code type-checks, not that the termios configuration actually works as intended on the real kernel (3.18.23).

- [ ] **Step 1: Build**

Run (from `mt7688-mcp-server/`): `.\build.ps1`

- [ ] **Step 2: Deploy**

Run: `.\deploy.ps1`

- [ ] **Step 3: Reload the MCP client session**

Ask the user to reload/reconnect their MCP session so `uart_transact` becomes visible (10 tools total now).

- [ ] **Step 4: Verify the GPIO blocklist extension live**

Call `gpio_write` (or `gpio_set_direction`) on pin 45 or 46. Expected: rejected with the "UART1 (uart1 pinmux group)" blocklist message.

- [ ] **Step 5: Verify `uart_transact`**

Ask the user whether they have any device to test against (a USB-serial loopback, a device known to echo, anything wired to pins 45/46) — per the design doc's own testing caveat, a call succeeding with empty `rx_data` and nothing attached proves much less than SPI's equivalent case did (it's indistinguishable from "wrong baud rate, nothing understood us"). If nothing is available:
- Call `uart_transact` with `baud_rate: 9600, tx_data: [], read_timeout_ms: 100, max_read_bytes: 256` (pure listen, minimal risk) and confirm it returns without error (proves open + termios configuration succeed on the real kernel, even without proving byte-level correctness).
- Note in the write-up that full round-trip correctness is unverified, same posture as `i2c_write` in the original MCP server phase.

If a real device or loopback is available: send known bytes and confirm what comes back matches expectations for that device/rate.

- [ ] **Step 6: Record what was found**

If Step 4 or 5 didn't match expectations, fix the relevant code (Tasks 1-4) and repeat this task's verification steps before continuing.

Update `docs/superpowers/specs/2026-07-11-uart-design.md`: change the `Status:` line to `implemented, verified live on real hardware <date>` (or `implemented, partially verified` if no real device was available to confirm byte-level correctness — be honest about which case applies), and add a "Live verification" section documenting exactly what was confirmed, including whatever Task 3 Step 1 actually found in the `x/sys/unix` source (record it even if it matched the hypothesis, so this isn't re-derived from scratch next time).

Update `README.md` and `README.zh-TW.md`: update the tool count/status line to include `uart_transact`, and update the roadmap note to reflect I2S and the ATmega bridge as the next two items (ADC still deferred behind the bridge).

- [ ] **Step 7: Commit**

```bash
git add docs/superpowers/specs/2026-07-11-uart-design.md README.md README.zh-TW.md
git commit -m "docs: record UART live verification results"
```
