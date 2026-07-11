<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# I2S Playback MCP Tool Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `i2s_play` as an MCP tool on `mt7688-mcp-server`, per `docs/superpowers/specs/2026-07-11-i2s-design.md`.

**Architecture:** Unlike every prior tool (GPIO/I2C/SPI/PWM/UART, all direct syscall/ioctl/sysfs work), this tool shells out to the board's already-installed `aplay` binary via `os/exec` — the design doc's investigation found the board's ALSA/I2S stack already works for playback, and reimplementing ALSA PCM programming in Go would be substantially riskier for no benefit. Because `os/exec` is portable Go with no platform-specific syscalls, this is the first tool in the codebase that does **not** need a `//go:build linux` / `//go:build !linux` file split — one file, works everywhere, fails naturally (binary not found) on the Windows dev machine.

**Tech Stack:** Go 1.26, standard library only (`encoding/base64`, `os/exec`, `context`) — no new dependencies.

---

## Task 1: Extend the GPIO blocklist for I2S pins

**Files:**
- Modify: `mt7688-mcp-server/blocklist.go`
- Modify: `mt7688-mcp-server/blocklist_test.go`

Pins 0-3 (the `i2s` pinmux group, feeding the WM8960 codec) are about to be claimed by `i2s_play`. Same rationale as every prior blocklist extension: a `gpio_write`/`gpio_set_direction` on these pins wouldn't error today, it would just risk interfering with the audio codec's I2S signaling.

- [ ] **Step 1: Update the test to expect the new pins as blocked**

Replace the full contents of `mt7688-mcp-server/blocklist_test.go`:

```go
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import "testing"

func TestCheckBlocked_BlockedPins(t *testing.T) {
	blocked := []int{0, 1, 2, 3, 6, 7, 8, 9, 10, 12, 13, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 38, 45, 46}
	for _, pin := range blocked {
		if err := CheckBlocked(pin); err == nil {
			t.Errorf("pin %d: expected error, got nil", pin)
		}
	}
}

func TestCheckBlocked_AllowedPins(t *testing.T) {
	allowed := []int{30, 39, 95}
	for _, pin := range allowed {
		if err := CheckBlocked(pin); err != nil {
			t.Errorf("pin %d: expected nil, got %v", pin, err)
		}
	}
}
```

(Note: pins 0 and 1 move from `allowed` to `blocked` in this change — they were previously used as "known-safe" test pins precisely because nothing claimed them yet. That's no longer true, so `allowed` drops to `{30, 39, 95}`, which is still enough pins to meaningfully test the "not blocked" path.)

- [ ] **Step 2: Run the test, verify it fails**

Run (from `mt7688-mcp-server/`): `go test ./... -run TestCheckBlocked -v`
Expected: `TestCheckBlocked_BlockedPins` FAILs on pins 0, 1, 2, 3 (not yet in `blockedPins`).

- [ ] **Step 3: Add the new entries to the blocklist**

In `mt7688-mcp-server/blocklist.go`, add these four lines to the `blockedPins` map:

```go
	0: "I2S (i2s pinmux group, WM8960 codec)",
	1: "I2S (i2s pinmux group, WM8960 codec)",
	2: "I2S (i2s pinmux group, WM8960 codec)",
	3: "I2S (i2s pinmux group, WM8960 codec)",
```

The full map should read:

```go
var blockedPins = map[int]string{
	0:  "I2S (i2s pinmux group, WM8960 codec)",
	1:  "I2S (i2s pinmux group, WM8960 codec)",
	2:  "I2S (i2s pinmux group, WM8960 codec)",
	3:  "I2S (i2s pinmux group, WM8960 codec)",
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
git commit -m "feat: block GPIO access to pins claimed by I2S"
```

---

## Task 2: WAV payload decoding and validation

**Files:**
- Create: `mt7688-mcp-server/i2s.go`
- Create: `mt7688-mcp-server/i2s_test.go`

Pure Go, no OS-specific calls, no hardware needed — compiles and is fully tested by plain `go test ./...` on the Windows dev machine.

- [ ] **Step 1: Write the failing tests**

Create `mt7688-mcp-server/i2s_test.go`:

```go
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import (
	"encoding/base64"
	"testing"
)

// validWAVBytes is a minimal, well-formed WAV file: a 44-byte PCM header
// (RIFF/WAVE, S16_LE mono at 8000 Hz) followed by 4 bytes (2 samples) of
// silence. Used as a baseline "this is valid" fixture across tests.
func validWAVBytes() []byte {
	return []byte{
		'R', 'I', 'F', 'F', 0x28, 0x00, 0x00, 0x00, 'W', 'A', 'V', 'E',
		'f', 'm', 't', ' ', 0x10, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00,
		0x40, 0x1f, 0x00, 0x00, 0x80, 0x3e, 0x00, 0x00, 0x02, 0x00, 0x10, 0x00,
		'd', 'a', 't', 'a', 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
}

func TestDecodeWAV_InvalidBase64(t *testing.T) {
	if _, err := decodeWAV("not valid base64!!!"); err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

func TestDecodeWAV_EmptyData(t *testing.T) {
	if _, err := decodeWAV(base64.StdEncoding.EncodeToString([]byte{})); err == nil {
		t.Fatal("expected error for empty data, got nil")
	}
}

func TestDecodeWAV_TooLarge(t *testing.T) {
	big := make([]byte, i2sMaxWAVBytes+1)
	copy(big, validWAVBytes())
	if _, err := decodeWAV(base64.StdEncoding.EncodeToString(big)); err == nil {
		t.Fatal("expected error for data > i2sMaxWAVBytes, got nil")
	}
}

func TestDecodeWAV_TooShortForHeader(t *testing.T) {
	if _, err := decodeWAV(base64.StdEncoding.EncodeToString([]byte("RIFF"))); err == nil {
		t.Fatal("expected error for data shorter than a WAV header, got nil")
	}
}

func TestDecodeWAV_MissingRIFFHeader(t *testing.T) {
	notWAV := []byte("this is not a wav file at all, just plain text")
	if _, err := decodeWAV(base64.StdEncoding.EncodeToString(notWAV)); err == nil {
		t.Fatal("expected error for missing RIFF/WAVE header, got nil")
	}
}

func TestDecodeWAV_Valid(t *testing.T) {
	data, err := decodeWAV(base64.StdEncoding.EncodeToString(validWAVBytes()))
	if err != nil {
		t.Fatalf("expected nil for valid WAV data, got %v", err)
	}
	if len(data) != len(validWAVBytes()) {
		t.Fatalf("expected decoded length %d, got %d", len(validWAVBytes()), len(data))
	}
}
```

- [ ] **Step 2: Run the tests, verify they fail to compile**

Run: `go test ./... -run TestDecodeWAV -v`
Expected: FAIL — `decodeWAV`, `i2sMaxWAVBytes` undefined.

- [ ] **Step 3: Write the implementation**

Create `mt7688-mcp-server/i2s.go`:

```go
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import (
	"encoding/base64"
	"fmt"
)

// i2sMaxWAVBytes bounds the decoded WAV payload i2s_play accepts. Base64
// is high-entropy from an LLM tokenizer's perspective (roughly 3-4
// characters per token), so this isn't primarily a device/hardware
// limit - it bounds how much of a caller's context budget one i2s_play
// call can cost. 100000 bytes decodes to ~133KB of base64 text, on the
// order of 30,000-45,000 tokens - see design doc's "Payload size cap"
// section for the earlier, much larger (and wrong) estimate this
// replaced.
const i2sMaxWAVBytes = 100000

// decodeWAV decodes and validates a base64-encoded WAV payload before any
// device access, matching the validation-before-hardware pattern in
// i2c.go/spi.go/uart.go. Deeper WAV validation (sample rate, channel
// count, bit depth) is intentionally left to aplay itself - see design
// doc's "Format sanity check" section for why duplicating a WAV parser
// here isn't worth it.
func decodeWAV(wavBase64 string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(wavBase64)
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}
	if len(data) == 0 || len(data) > i2sMaxWAVBytes {
		return nil, fmt.Errorf("wav data length must be 1-%d bytes, got %d", i2sMaxWAVBytes, len(data))
	}
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, fmt.Errorf("data does not look like a WAV file (missing RIFF/WAVE header)")
	}
	return data, nil
}
```

- [ ] **Step 4: Run the tests, verify they pass**

Run: `go test ./... -run TestDecodeWAV -v`
Expected: PASS

- [ ] **Step 5: Run the full test suite to check for regressions**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add mt7688-mcp-server/i2s.go mt7688-mcp-server/i2s_test.go
git commit -m "feat: add WAV payload decoding and validation for I2S playback"
```

---

## Task 3: PlayI2S — spawn aplay and play the clip

**Files:**
- Modify: `mt7688-mcp-server/i2s.go` (add `PlayI2S`)

This is the part that can't be meaningfully unit-tested off real hardware (`aplay` doesn't exist on the Windows dev machine, and even if it did, there'd be no audio device to play through) — same category as `i2c_linux.go`/`spi_linux.go`/`uart_linux.go` having no dedicated tests. Unlike those, no `//go:build` split is needed: `os/exec` is portable, so this compiles fine on Windows too, it just fails with a clear "executable file not found" error if `aplay` isn't on `PATH` — which is itself the correct, natural behavior for a non-Linux dev machine, no stub file required.

- [ ] **Step 1: Append `PlayI2S` to `i2s.go`**

Append to the end of `mt7688-mcp-server/i2s.go`:

```go

// PlayI2S decodes and validates wavBase64, then plays it through the
// board's audio output via aplay - the first tool in this server to
// spawn a subprocess rather than doing direct syscall/ioctl/sysfs work.
// See design doc's "Implementation approach" section for why: ALSA PCM
// programming has no x/sys/unix built-in support the way termios did,
// and aplay is already proven working end-to-end on this exact hardware
// (unlike arecord, which reliably wedges the board - see design doc's
// "Investigation" section - which is why there is no PlayI2S counterpart
// for recording).
func PlayI2S(wavBase64 string) error {
	data, err := decodeWAV(wavBase64)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), i2sPlaybackTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "aplay", "-D", i2sDevice, "-q")
	cmd.Stdin = bytes.NewReader(data)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("aplay timed out after %s - if this recurs, the board may need a reboot (see design doc's capture-hang findings for the same failure class on the record path): %w", i2sPlaybackTimeout, err)
		}
		return fmt.Errorf("aplay failed: %w (stderr: %s)", err, stderr.String())
	}
	return nil
}

// i2sDevice is the ALSA device targeted directly (not plug:-wrapped),
// matching what was actually tested live - see design doc's "Non-goals"
// section on why automatic format conversion isn't provided.
const i2sDevice = "hw:0,0"

// i2sPlaybackTimeout is a generous safety net against aplay hanging, not
// a tight bound on normal playback time - comfortably above the
// ~6.25-second worst-case duration implied by i2sMaxWAVBytes at the
// lowest supported bitrate (8kHz mono).
const i2sPlaybackTimeout = 30 * time.Second
```

- [ ] **Step 2: Update the imports at the top of `i2s.go`**

The import block at the top of `mt7688-mcp-server/i2s.go` needs to grow from:

```go
import (
	"encoding/base64"
	"fmt"
)
```

to:

```go
import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"time"
)
```

- [ ] **Step 3: Build and run the full test suite**

Run: `go build ./...`
Expected: succeeds (native Windows build — `os/exec`/`context`/etc. are all portable, no build tags involved)

Run: `go test ./...`
Expected: PASS, no regressions

Also cross-compile for the real target, to confirm nothing about this file is accidentally non-portable:

Run: `$env:GOOS="linux"; $env:GOARCH="mipsle"; $env:GOMIPS="softfloat"; $env:CGO_ENABLED="0"; go build ./...; go vet ./...; Remove-Item Env:\GOOS,Env:\GOARCH,Env:\GOMIPS,Env:\CGO_ENABLED`
Expected: both succeed with exit 0

- [ ] **Step 4: Commit**

```bash
git add mt7688-mcp-server/i2s.go
git commit -m "feat: implement I2S playback via aplay subprocess"
```

---

## Task 4: Wire up the `i2s_play` MCP tool

**Files:**
- Modify: `mt7688-mcp-server/main.go`

Unlike every prior tool's wiring task, this one does **not** need a `[]int`/`[]byte` boundary conversion — the MCP-facing field is a base64 `string` directly (the whole reason base64 was chosen over a `[]int` sample array, see design doc), so there's no per-element loop and correspondingly no new boundary-conversion test to add (matching how simple passthrough handlers like `gpioReadHandler`/`pwmConfigureHandler` don't have dedicated handler-level tests either — the real logic being tested is already fully covered by Task 2's `decodeWAV` tests).

- [ ] **Step 1: Add the tool struct, handler, and registration**

In `mt7688-mcp-server/main.go`, insert the following after the existing `uartTransactHandler` function (i.e. after the last UART handler, before `func main()`):

```go

type I2SPlayInput struct {
	WavBase64 string `json:"wav_base64" jsonschema:"base64-encoded WAV audio to play (max 100000 decoded bytes, roughly 6 seconds at 8kHz mono)"`
}
type I2SPlayOutput struct {
	OK bool `json:"ok"`
}

func i2sPlayHandler(ctx context.Context, req *mcp.CallToolRequest, input I2SPlayInput) (*mcp.CallToolResult, I2SPlayOutput, error) {
	if err := PlayI2S(input.WavBase64); err != nil {
		return nil, I2SPlayOutput{}, err
	}
	return nil, I2SPlayOutput{OK: true}, nil
}
```

Then, inside `func main()`, add this registration right after the existing `uart_transact` registration (before `if err := server.Run(...)`):

```go
	mcp.AddTool(server, &mcp.Tool{Name: "i2s_play", Description: "Play a WAV audio clip through the board's speaker/headphone output. Blocks until playback completes. Max 100000 bytes of decoded WAV data (roughly 6 seconds at 8kHz mono). Recording is not available - the board's capture path has a confirmed driver-level bug that hangs the device."}, i2sPlayHandler)
```

- [ ] **Step 2: Run the full test suite and build to check for regressions**

Run: `go test ./...`
Expected: PASS

Run: `go build ./...`
Expected: succeeds

- [ ] **Step 3: Commit**

```bash
git add mt7688-mcp-server/main.go
git commit -m "feat: register i2s_play MCP tool"
```

---

## Task 5: Cross-compile, deploy, and live-verify on real hardware

**Files:**
- Modify: `docs/superpowers/specs/2026-07-11-i2s-design.md` (status + live verification section)
- Modify: `README.md`, `README.zh-TW.md` (status section)

No code changes in this task. This is the first tool in the project where "verified" includes a human actually hearing something, not just a call returning without error.

- [ ] **Step 1: Build**

Run (from `mt7688-mcp-server/`): `.\build.ps1`

- [ ] **Step 2: Deploy**

Run: `.\deploy.ps1`

- [ ] **Step 3: Reload the MCP client session**

Ask the user to reload/reconnect their MCP session so `i2s_play` becomes visible (11 tools total now).

- [ ] **Step 4: Verify the GPIO blocklist extension live**

Call `gpio_write` (or `gpio_set_direction`) on pin 0, 1, 2, or 3. Expected: rejected with the "I2S (i2s pinmux group, WM8960 codec)" blocklist message.

- [ ] **Step 5: Verify `i2s_play` with a real, audible clip**

Generate a short, actually-audible WAV (not silence — silence would "succeed" even if playback were completely broken) — e.g. a 1-second 440Hz sine tone at 8000 Hz, mono, 16-bit — base64-encode it, and call `i2s_play` with it. Ask the user to confirm they heard a tone (through whatever's connected to the board's audio output — headphone jack or onboard speaker, depending on what's wired up). This is the actual verification; a clean `{"ok": true}` response alone doesn't prove the earlier `XRUN` buffer-underrun warnings (see design doc's "Investigation" section) didn't make the tone glitchy or silent.

Also confirm: calling `i2s_play` with `wav_base64` set to something larger than 100000 decoded bytes is rejected with a clear error before anything is sent to `aplay`.

- [ ] **Step 6: Record what was found**

If Step 4 or 5 didn't match expectations, fix the relevant code (Tasks 1-4) and repeat this task's verification steps before continuing. If Step 5's tone was audibly glitchy (consistent with the `XRUN` warnings seen during design-phase testing), note that specifically rather than just marking it pass/fail — it's a known, already-documented caveat, not a new bug, but worth confirming whether it's audible in practice.

Update `docs/superpowers/specs/2026-07-11-i2s-design.md`: change the `Status:` line to `implemented, verified live on real hardware <date>`, and add a "Live verification" section documenting exactly what was confirmed (including whether the tone was audibly clean or showed the XRUN-related glitching).

Update `README.md` and `README.zh-TW.md`: update the tool count/status line to include `i2s_play` (11 tools total), and note that I2S recording is confirmed unavailable (driver bug, not just unimplemented) rather than merely "not yet exposed" like the remaining ATmega bridge/ADC items.

- [ ] **Step 7: Commit**

```bash
git add docs/superpowers/specs/2026-07-11-i2s-design.md README.md README.zh-TW.md
git commit -m "docs: record I2S playback live verification results"
```
