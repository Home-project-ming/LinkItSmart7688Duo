# MCP Server for Board Hardware Control — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Go MCP server, cross-compiled for the board, exposing GPIO
read/write/direction and I2C read/write as tools an MCP client (Claude Code)
can call over SSH.

**Architecture:** Static Go binary (`CGO_ENABLED=0`), cross-compiled
`GOOS=linux GOARCH=mipsle GOMIPS=softfloat`, deployed via `pscp` to
`/usr/local/bin/mcp-server` on the board (SD-backed overlay, normal path —
no special mount handling; see the design spec for why static linking alone
is sufficient). Client spawns it per-session over SSH (stdio transport),
authenticating with the RSA key already installed in
`/etc/dropbear/authorized_keys`, using the three legacy-algorithm overrides
this board's old Dropbear requires.

**Tech Stack:** Go 1.22+, `github.com/modelcontextprotocol/go-sdk/mcp`
v1.6.1, `golang.org/x/sys/unix` for raw ioctl access. No other
dependencies.

**Design doc:** `docs/superpowers/specs/2026-07-10-mcp-server-design.md`

---

## File Structure

```
mt7688-mcp-server/
  go.mod
  go.sum
  main.go              -- MCP server setup, tool registration, tool handlers
  blocklist.go          -- GPIO pin blocklist (pure logic)
  blocklist_test.go
  gpio.go               -- GPIO read/write/direction via /sys/class/gpio
  gpio_test.go
  i2c.go                -- I2C read/write via ioctl on /dev/i2c-0
  i2c_test.go
  build.ps1             -- cross-compiles ./mcp-server for the board
  deploy.ps1            -- pscp's ./mcp-server to the board
```

`build.ps1` and `deploy.ps1` are separate scripts (build produces an
artifact, deploy ships it) matching the existing separation of concerns in
`provisioning/lib/`.

---

### Task 1: Project scaffold and toolchain verification

**Files:**
- Create: `mt7688-mcp-server/go.mod`
- Create: `mt7688-mcp-server/main.go`
- Create: `mt7688-mcp-server/build.ps1`

This task's only goal is proving the toolchain works end-to-end —
dependency resolves, natively builds, and cross-compiles for
`mipsle`/softfloat — before any real hardware logic is written. This is
the single highest-risk unknown in the whole plan; better to find out now.

- [ ] **Step 1: Initialize the git repo (project has none yet)**

Run from the repo root (`D:\LinkItSmart7688Duo`):

```powershell
git init
git add CLAUDE.md dev-notes.html provisioning docs
git commit -m "chore: initial commit of existing project files"
```

Expected: a new `.git` directory, and a commit containing everything that
already existed (provisioning scripts, dev-notes.html, the specs/plans
written so far).

- [ ] **Step 2: Scaffold the Go module**

```powershell
mkdir mt7688-mcp-server
cd mt7688-mcp-server
go mod init mt7688-mcp-server
```

Expected: `go.mod` created with `module mt7688-mcp-server` and a `go`
directive matching the locally installed Go version.

- [ ] **Step 3: Add the MCP SDK dependency**

```powershell
go get github.com/modelcontextprotocol/go-sdk@v1.6.1
```

Expected: `go.mod` gains a `require` line for the SDK, `go.sum` is
created/populated.

- [ ] **Step 4: Write a minimal placeholder `main.go` to prove the SDK compiles**

```go
package main

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type PingInput struct {
	Message string `json:"message" jsonschema:"text to echo back"`
}

type PingOutput struct {
	Reply string `json:"reply" jsonschema:"the echoed text"`
}

func pingHandler(ctx context.Context, req *mcp.CallToolRequest, input PingInput) (*mcp.CallToolResult, PingOutput, error) {
	return nil, PingOutput{Reply: input.Message}, nil
}

func main() {
	server := mcp.NewServer(&mcp.Implementation{Name: "linkit7688-hw", Version: "v0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "ping", Description: "echo a message back"}, pingHandler)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 5: Native build, to confirm the dependency itself is sound**

```powershell
go build -o mcp-server-native.exe .
```

Expected: succeeds, produces `mcp-server-native.exe`. Delete it afterward
(`Remove-Item .\mcp-server-native.exe`) — it's a throwaway check, not a
deliverable.

- [ ] **Step 6: Write `build.ps1` for cross-compilation**

```powershell
# Cross-compiles the MCP server for the board (mipsel, soft-float,
# statically linked). Run as .\build.ps1 (not dot-sourced) so the GOOS/
# GOARCH/GOMIPS/CGO_ENABLED overrides don't leak into the calling shell.
$env:GOOS = 'linux'
$env:GOARCH = 'mipsle'
$env:GOMIPS = 'softfloat'
$env:CGO_ENABLED = '0'

go build -ldflags="-s -w" -o mcp-server .
if ($LASTEXITCODE -ne 0) { throw "build failed" }

$size = (Get-Item .\mcp-server).Length
Write-Output "built .\mcp-server ($size bytes)"
```

- [ ] **Step 7: Run the cross-compile and confirm it succeeds**

```powershell
.\build.ps1
```

Expected: prints `built .\mcp-server (N bytes)` with no errors. This
`mcp-server` file cannot be run or tested on the Windows dev machine (wrong
OS/arch) — that's expected, it's only verified by successfully deploying
it to the board in Task 6.

- [ ] **Step 8: Commit**

```powershell
git add mt7688-mcp-server/go.mod mt7688-mcp-server/go.sum mt7688-mcp-server/main.go mt7688-mcp-server/build.ps1
git commit -m "feat: scaffold MCP server, verify mipsle cross-compile toolchain"
```

---

### Task 2: GPIO blocklist (TDD)

**Files:**
- Create: `mt7688-mcp-server/blocklist.go`
- Test: `mt7688-mcp-server/blocklist_test.go`

Pure logic, no hardware or OS dependency — runs on any machine.

- [ ] **Step 1: Write the failing test**

```go
package main

import "testing"

func TestCheckBlocked_BlockedPins(t *testing.T) {
	blocked := []int{20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 38}
	for _, pin := range blocked {
		if err := CheckBlocked(pin); err == nil {
			t.Errorf("pin %d: expected error, got nil", pin)
		}
	}
}

func TestCheckBlocked_AllowedPins(t *testing.T) {
	allowed := []int{0, 1, 18, 19, 30, 39, 95}
	for _, pin := range allowed {
		if err := CheckBlocked(pin); err != nil {
			t.Errorf("pin %d: expected nil, got %v", pin, err)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```powershell
go test ./... -run TestCheckBlocked -v
```

Expected: FAIL — `CheckBlocked` is undefined.

- [ ] **Step 3: Write the implementation**

```go
package main

import "fmt"

var blockedPins = map[int]string{
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

- [ ] **Step 4: Run test to verify it passes**

```powershell
go test ./... -run TestCheckBlocked -v
```

Expected: PASS, both tests.

- [ ] **Step 5: Commit**

```powershell
git add mt7688-mcp-server/blocklist.go mt7688-mcp-server/blocklist_test.go
git commit -m "feat: add GPIO pin blocklist"
```

---

### Task 3: GPIO operations (TDD)

**Files:**
- Create: `mt7688-mcp-server/gpio.go`
- Test: `mt7688-mcp-server/gpio_test.go`

Uses the classic sysfs GPIO ABI (`/sys/class/gpio/...`) — this kernel
(3.18) predates the character-device GPIO interface, so sysfs is the
correct interface here, not `/dev/gpiochipN`. Only the blocklist-rejection
and input-validation paths are testable off the board (they return before
touching the filesystem); actual pin I/O is verified manually in Task 7.

- [ ] **Step 1: Write the failing tests**

```go
package main

import "testing"

func TestSetGPIODirection_BlockedPin(t *testing.T) {
	err := SetGPIODirection(21, "out")
	if err == nil {
		t.Fatal("expected error for blocked pin 21, got nil")
	}
}

func TestReadGPIO_BlockedPin(t *testing.T) {
	_, err := ReadGPIO(38)
	if err == nil {
		t.Fatal("expected error for blocked pin 38, got nil")
	}
}

func TestWriteGPIO_BlockedPin(t *testing.T) {
	err := WriteGPIO(25, true)
	if err == nil {
		t.Fatal("expected error for blocked pin 25, got nil")
	}
}

func TestSetGPIODirection_InvalidDirection(t *testing.T) {
	// pin 0 is not blocked, so this must fail on direction validation,
	// not the blocklist - proving validation order and staying testable
	// off real hardware.
	err := SetGPIODirection(0, "sideways")
	if err == nil {
		t.Fatal("expected error for invalid direction, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```powershell
go test ./... -run "TestSetGPIODirection|TestReadGPIO_BlockedPin|TestWriteGPIO_BlockedPin" -v
```

Expected: FAIL — `SetGPIODirection`, `ReadGPIO`, `WriteGPIO` undefined.

- [ ] **Step 3: Write the implementation**

```go
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const gpioSysfsPath = "/sys/class/gpio"

func exportGPIO(pin int) error {
	path := fmt.Sprintf("%s/gpio%d", gpioSysfsPath, pin)
	if _, err := os.Stat(path); err == nil {
		return nil // already exported
	}
	f, err := os.OpenFile(gpioSysfsPath+"/export", os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open export: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(strconv.Itoa(pin)); err != nil {
		return fmt.Errorf("write export: %w", err)
	}
	return nil
}

func SetGPIODirection(pin int, direction string) error {
	if err := CheckBlocked(pin); err != nil {
		return err
	}
	if direction != "in" && direction != "out" {
		return fmt.Errorf("direction must be \"in\" or \"out\", got %q", direction)
	}
	if err := exportGPIO(pin); err != nil {
		return err
	}
	path := fmt.Sprintf("%s/gpio%d/direction", gpioSysfsPath, pin)
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open direction: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(direction); err != nil {
		return fmt.Errorf("write direction: %w", err)
	}
	return nil
}

func ReadGPIO(pin int) (bool, error) {
	if err := CheckBlocked(pin); err != nil {
		return false, err
	}
	if err := exportGPIO(pin); err != nil {
		return false, err
	}
	path := fmt.Sprintf("%s/gpio%d/value", gpioSysfsPath, pin)
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read value: %w", err)
	}
	return strings.TrimSpace(string(data)) == "1", nil
}

func WriteGPIO(pin int, value bool) error {
	if err := CheckBlocked(pin); err != nil {
		return err
	}
	if err := exportGPIO(pin); err != nil {
		return err
	}
	path := fmt.Sprintf("%s/gpio%d/value", gpioSysfsPath, pin)
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open value: %w", err)
	}
	defer f.Close()
	v := "0"
	if value {
		v = "1"
	}
	if _, err := f.WriteString(v); err != nil {
		return fmt.Errorf("write value: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```powershell
go test ./... -v
```

Expected: PASS, all tests (including Task 2's).

- [ ] **Step 5: Commit**

```powershell
git add mt7688-mcp-server/gpio.go mt7688-mcp-server/gpio_test.go
git commit -m "feat: add GPIO read/write/direction via sysfs"
```

---

### Task 4: I2C operations (TDD)

**Files:**
- Create: `mt7688-mcp-server/i2c.go`
- Test: `mt7688-mcp-server/i2c_test.go`

Bus is fixed to `/dev/i2c-0`. Uses raw `ioctl(I2C_SLAVE)` via
`golang.org/x/sys/unix` — `I2C_SLAVE` (`0x0703`) is a flat constant from
`<linux/i2c-dev.h>`, not affected by the MIPS `_IOC` bit-layout difference
that trips up SPI's ioctls (SPI is out of scope for v1 anyway). Only
input-validation is testable off the board.

- [ ] **Step 1: Add the `golang.org/x/sys` dependency**

```powershell
cd mt7688-mcp-server
go get golang.org/x/sys/unix
```

Expected: `go.mod`/`go.sum` updated.

- [ ] **Step 2: Write the failing tests**

```go
package main

import "testing"

func TestReadI2C_InvalidAddress(t *testing.T) {
	_, err := ReadI2C(0x80, 0x00, 1)
	if err == nil {
		t.Fatal("expected error for out-of-range address, got nil")
	}
}

func TestReadI2C_InvalidLength(t *testing.T) {
	if _, err := ReadI2C(0x50, 0x00, 0); err == nil {
		t.Fatal("expected error for zero length, got nil")
	}
	if _, err := ReadI2C(0x50, 0x00, 300); err == nil {
		t.Fatal("expected error for length > 256, got nil")
	}
}

func TestWriteI2C_InvalidAddress(t *testing.T) {
	err := WriteI2C(0x80, 0x00, []byte{0x01})
	if err == nil {
		t.Fatal("expected error for out-of-range address, got nil")
	}
}

func TestWriteI2C_EmptyData(t *testing.T) {
	err := WriteI2C(0x50, 0x00, []byte{})
	if err == nil {
		t.Fatal("expected error for empty data, got nil")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

```powershell
go test ./... -run "I2C" -v
```

Expected: FAIL — `ReadI2C`, `WriteI2C` undefined.

- [ ] **Step 4: Write the implementation**

```go
package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

const i2cDevPath = "/dev/i2c-0"
const i2cSlaveIoctl = 0x0703 // I2C_SLAVE, from <linux/i2c-dev.h>

func openI2C(addr byte) (*os.File, error) {
	if addr > 0x7f {
		return nil, fmt.Errorf("i2c address %#x out of 7-bit range", addr)
	}
	f, err := os.OpenFile(i2cDevPath, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", i2cDevPath, err)
	}
	if err := unix.IoctlSetInt(int(f.Fd()), i2cSlaveIoctl, int(addr)); err != nil {
		f.Close()
		return nil, fmt.Errorf("set i2c slave address %#x: %w", addr, err)
	}
	return f, nil
}

func ReadI2C(addr byte, reg byte, length int) ([]byte, error) {
	if length <= 0 || length > 256 {
		return nil, fmt.Errorf("length must be 1-256, got %d", length)
	}
	f, err := openI2C(addr)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := f.Write([]byte{reg}); err != nil {
		return nil, fmt.Errorf("write register %#x: %w", reg, err)
	}
	buf := make([]byte, length)
	n, err := f.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("read %d bytes: %w", length, err)
	}
	return buf[:n], nil
}

func WriteI2C(addr byte, reg byte, data []byte) error {
	if len(data) == 0 || len(data) > 256 {
		return fmt.Errorf("data length must be 1-256, got %d", len(data))
	}
	f, err := openI2C(addr)
	if err != nil {
		return err
	}
	defer f.Close()

	payload := append([]byte{reg}, data...)
	if _, err := f.Write(payload); err != nil {
		return fmt.Errorf("write %d bytes to register %#x: %w", len(data), reg, err)
	}
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

```powershell
go test ./... -v
```

Expected: PASS, all tests (Tasks 2, 3, and 4).

- [ ] **Step 6: Commit**

```powershell
git add mt7688-mcp-server/i2c.go mt7688-mcp-server/i2c_test.go mt7688-mcp-server/go.mod mt7688-mcp-server/go.sum
git commit -m "feat: add I2C read/write via ioctl on /dev/i2c-0"
```

---

### Task 5: Wire up the real MCP tools

**Files:**
- Modify: `mt7688-mcp-server/main.go` (replaces the Task 1 `ping` placeholder)

- [ ] **Step 1: Replace `main.go` with the five real tools**

```go
package main

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GPIOReadInput struct {
	Pin int `json:"pin" jsonschema:"GPIO pin number to read"`
}
type GPIOReadOutput struct {
	Value bool `json:"value" jsonschema:"true if the pin reads high"`
}

func gpioReadHandler(ctx context.Context, req *mcp.CallToolRequest, input GPIOReadInput) (*mcp.CallToolResult, GPIOReadOutput, error) {
	v, err := ReadGPIO(input.Pin)
	if err != nil {
		return nil, GPIOReadOutput{}, err
	}
	return nil, GPIOReadOutput{Value: v}, nil
}

type GPIOWriteInput struct {
	Pin   int  `json:"pin" jsonschema:"GPIO pin number to write"`
	Value bool `json:"value" jsonschema:"true to drive the pin high, false for low"`
}
type GPIOWriteOutput struct {
	OK bool `json:"ok"`
}

func gpioWriteHandler(ctx context.Context, req *mcp.CallToolRequest, input GPIOWriteInput) (*mcp.CallToolResult, GPIOWriteOutput, error) {
	if err := WriteGPIO(input.Pin, input.Value); err != nil {
		return nil, GPIOWriteOutput{}, err
	}
	return nil, GPIOWriteOutput{OK: true}, nil
}

type GPIODirectionInput struct {
	Pin       int    `json:"pin" jsonschema:"GPIO pin number"`
	Direction string `json:"direction" jsonschema:"\"in\" or \"out\""`
}
type GPIODirectionOutput struct {
	OK bool `json:"ok"`
}

func gpioSetDirectionHandler(ctx context.Context, req *mcp.CallToolRequest, input GPIODirectionInput) (*mcp.CallToolResult, GPIODirectionOutput, error) {
	if err := SetGPIODirection(input.Pin, input.Direction); err != nil {
		return nil, GPIODirectionOutput{}, err
	}
	return nil, GPIODirectionOutput{OK: true}, nil
}

type I2CReadInput struct {
	Addr   int `json:"addr" jsonschema:"7-bit I2C device address"`
	Reg    int `json:"reg" jsonschema:"register address to read from"`
	Length int `json:"length" jsonschema:"number of bytes to read"`
}
type I2CReadOutput struct {
	Data []byte `json:"data" jsonschema:"bytes read from the device"`
}

func i2cReadHandler(ctx context.Context, req *mcp.CallToolRequest, input I2CReadInput) (*mcp.CallToolResult, I2CReadOutput, error) {
	data, err := ReadI2C(byte(input.Addr), byte(input.Reg), input.Length)
	if err != nil {
		return nil, I2CReadOutput{}, err
	}
	return nil, I2CReadOutput{Data: data}, nil
}

type I2CWriteInput struct {
	Addr int    `json:"addr" jsonschema:"7-bit I2C device address"`
	Reg  int    `json:"reg" jsonschema:"register address to write to"`
	Data []byte `json:"data" jsonschema:"bytes to write"`
}
type I2CWriteOutput struct {
	OK bool `json:"ok"`
}

func i2cWriteHandler(ctx context.Context, req *mcp.CallToolRequest, input I2CWriteInput) (*mcp.CallToolResult, I2CWriteOutput, error) {
	if err := WriteI2C(byte(input.Addr), byte(input.Reg), input.Data); err != nil {
		return nil, I2CWriteOutput{}, err
	}
	return nil, I2CWriteOutput{OK: true}, nil
}

func main() {
	server := mcp.NewServer(&mcp.Implementation{Name: "linkit7688-hw", Version: "v0.1.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{Name: "gpio_read", Description: "Read a GPIO pin's current value"}, gpioReadHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "gpio_write", Description: "Set a GPIO pin's output value"}, gpioWriteHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "gpio_set_direction", Description: "Set a GPIO pin's direction (in/out)"}, gpioSetDirectionHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "i2c_read", Description: "Read bytes from an I2C device register on /dev/i2c-0"}, i2cReadHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "i2c_write", Description: "Write bytes to an I2C device register on /dev/i2c-0"}, i2cWriteHandler)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 2: Run the full test suite**

```powershell
go test ./... -v
```

Expected: PASS, all tests. This step doesn't add new tests — the handlers
are thin wrappers with no new branching logic, so Tasks 2-4's tests are
the coverage for the underlying behavior they call into.

- [ ] **Step 3: Native build to confirm `main.go` compiles**

```powershell
go build -o mcp-server-native.exe .
Remove-Item .\mcp-server-native.exe
```

Expected: succeeds.

- [ ] **Step 4: Cross-compile**

```powershell
.\build.ps1
```

Expected: succeeds, produces `.\mcp-server`.

- [ ] **Step 5: Commit**

```powershell
git add mt7688-mcp-server/main.go
git commit -m "feat: wire up gpio_read/write/set_direction and i2c_read/write MCP tools"
```

---

### Task 6: Deploy script and CLAUDE.md

**Files:**
- Create: `mt7688-mcp-server/deploy.ps1`
- Modify: `CLAUDE.md`

- [ ] **Step 1: Write `deploy.ps1`**

```powershell
# Ships .\mcp-server to the board's normal SD-backed /usr/local/bin/.
# Run .\build.ps1 first.
param(
    [string] $BoardHost = '192.168.50.216',
    [string] $HostKey   = 'SHA256:pmSxVegIX1+q4Mkux0AB1hPZYylPT12RuMC0hAWLNa4',
    [string] $KeyFile   = '..\provisioning\lib\keys\board_rsa'
)

if (-not (Test-Path .\mcp-server)) { throw "mcp-server not found - run .\build.ps1 first" }

$sshOpts = @(
    '-i', $KeyFile,
    '-o', 'KexAlgorithms=+diffie-hellman-group14-sha1',
    '-o', 'HostKeyAlgorithms=+ssh-rsa',
    '-o', 'PubkeyAcceptedAlgorithms=+ssh-rsa'
)

scp -O @sshOpts .\mcp-server "root@${BoardHost}:/tmp/mcp-server"
if ($LASTEXITCODE -ne 0) { throw "scp failed" }

& ssh @sshOpts "root@${BoardHost}" 'mv /tmp/mcp-server /usr/local/bin/mcp-server && chmod 755 /usr/local/bin/mcp-server && echo deployed'
if ($LASTEXITCODE -ne 0) { throw "remote install failed" }
```

Note: `scp -O` forces the legacy SCP protocol (matches `pscp -scp`'s
reasoning — this Dropbear has no `sftp-server`). `-o HostKeyAlgorithms`
here doesn't need `StrictHostKeyChecking`/`accept-new` because the host key
was already trusted into this machine's `known_hosts` in Task "install SSH
key" earlier this session; a fresh machine would need that flag added.

- [ ] **Step 2: Run it**

```powershell
.\deploy.ps1
```

Expected: prints `deployed`.

- [ ] **Step 3: Confirm it's there and runs**

```powershell
ssh -i ..\provisioning\lib\keys\board_rsa -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa root@192.168.50.216 'ls -la /usr/local/bin/mcp-server && echo hello | timeout 2 /usr/local/bin/mcp-server; echo "exit=$?"'
```

Expected: `ls` shows the file, mode `755`. The server reads `hello` from
stdin (not valid MCP JSON-RPC), so it should either error out or hang until
the 2s `timeout` kills it (`exit=124`) rather than crash with a signal
(`exit=132`/`139`/`137` etc.) — a signal exit here would indicate the
static-linking fix didn't hold and needs investigating before going
further, not something to paper over.

- [ ] **Step 4: Update `CLAUDE.md`**

Replace the MT7688-side table row and the "Run / test / lint" TODO in
`CLAUDE.md` (the file at the repo root) to record the real commands now
that they exist:

In the "What this board is" table, change:
```
| Code lives in | (TODO: e.g. `mt7688/`, `services/`) | (TODO: e.g. `firmware/`, `sketch/`) |
```
to:
```
| Code lives in | `mt7688-mcp-server/` (MCP server) | (TODO: e.g. `firmware/`, `sketch/`) |
```

Replace:
```
- **Run / test / lint:** TODO — no test or lint tooling configured yet. Add the exact commands (including how to run a *single* test) when the harness exists.
```
with:
```
- **Run / test / lint (MCP server, `mt7688-mcp-server/`):**
  - Test: `go test ./...` (single test: `go test ./... -run TestName -v`)
  - Build for the board: `.\build.ps1` (cross-compiles `GOOS=linux GOARCH=mipsle GOMIPS=softfloat CGO_ENABLED=0`)
  - Deploy: `.\deploy.ps1` (ships to `/usr/local/bin/mcp-server` via `scp -O`)
```

- [ ] **Step 5: Commit**

```powershell
git add mt7688-mcp-server/deploy.ps1 CLAUDE.md
git commit -m "feat: add deploy script, record real build/test/deploy commands in CLAUDE.md"
```

---

### Task 7: MCP client config and manual hardware verification

**Files:**
- Modify: Claude Code's MCP server configuration (location depends on
  scope — project-level `.mcp.json` at the repo root, or user-level
  config; ask which if unclear rather than guessing)

This task has no automated tests — GPIO/I2C behavior can only be verified
against the real board.

- [ ] **Step 1: Add the server to the MCP client config**

```json
{
  "mcpServers": {
    "linkit7688-hw": {
      "command": "ssh",
      "args": [
        "-i", "D:\\LinkItSmart7688Duo\\provisioning\\lib\\keys\\board_rsa",
        "-o", "KexAlgorithms=+diffie-hellman-group14-sha1",
        "-o", "HostKeyAlgorithms=+ssh-rsa",
        "-o", "PubkeyAcceptedAlgorithms=+ssh-rsa",
        "-o", "MACs=+hmac-sha1",
        "root@192.168.50.216",
        "/usr/local/bin/mcp-server"
      ]
    }
  }
}
```

(Updated during Task 6: a 4th flag, `MACs=+hmac-sha1`, turned out to be
necessary too — specifically for Windows' bundled OpenSSH client, which
doesn't enable `hmac-sha1` by default the way Git-Bash/MSYS's build does.
Harmless to include regardless of which `ssh` ends up on PATH, so it's
included unconditionally here. See `provisioning/README.md`'s SSH key
section for the full 4-flag rationale table.)

- [ ] **Step 2: Reload MCP connections and confirm the tools appear**

Use whatever this Claude Code installation's mechanism is for listing
connected MCP servers/tools (e.g. `/mcp` command) and confirm all five —
`gpio_read`, `gpio_write`, `gpio_set_direction`, `i2c_read`, `i2c_write` —
are listed under `linkit7688-hw`.

- [ ] **Step 3: Verify the blocklist from a live tool call**

Call `gpio_read` with `pin: 21`. Expected: an error result containing
"blocked" and "console" — not a `value`.

- [ ] **Step 4: Verify a real GPIO round-trip**

Pick a pin already confirmed safe from the original hardware survey (the
LED pin identified during that survey is the best choice — it gives a
visible result). Call `gpio_set_direction` with that pin and
`direction: "out"`, then `gpio_write` with `value: true`, and physically
confirm the expected result (LED on). Then `gpio_write` with `value: false`
and confirm it turns off. If using a different pin instead of the LED,
confirm expected behavior via `gpio_read` round-tripping the value written.

- [ ] **Step 5: Verify I2C**

If a real I2C device is attached to `/dev/i2c-0` at a known address, call
`i2c_read` with that device's address and a register it documents, and
confirm the returned bytes look sane (matches a known register value from
the device's datasheet). If no I2C device is available to test against
right now, call `i2c_read` with a plausible address (e.g. `0x50`) anyway
and confirm the failure mode is a clean I/O error (no device at that
address) rather than a panic or hang — this at least proves the ioctl path
executes correctly end-to-end.

- [ ] **Step 6: Update the design spec's status line**

Change the `Status:` line at the top of
`docs/superpowers/specs/2026-07-10-mcp-server-design.md` from
`approved, not yet implemented` to `implemented, verified <today's date>`.

- [ ] **Step 7: Commit**

```powershell
git add docs/superpowers/specs/2026-07-10-mcp-server-design.md
git commit -m "docs: mark MCP server design as implemented and verified"
```
