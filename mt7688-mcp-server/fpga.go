// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Bitstream is a parsed Xilinx XC4000/Spartan .bit container.
type Bitstream struct {
	Design  string // tag 'a', the .ncd the bitstream was generated from
	Part    string // tag 'b', e.g. "s20pq208"
	Date    string // tag 'c'
	Time    string // tag 'd'
	Payload []byte // tag 'e', the raw configuration data
}

// Container layout, verified against ISE 4.2i bitgen output for
// xcs20-4-pq208 (Go port of bitparse.py in the FPGA_XCS20 repo):
//
//	<u16 len> <len bytes>          magic/prologue (len 9: 0f f0 .. 00)
//	<u16 len> <len bytes>          len is 1; the byte is the first tag, 'a'
//	repeating: <u16 len> <len bytes, NUL-terminated string> <u8 next_tag>
//	  'a' design name, 'b' part name, 'c' date, 'd' time
//	tag 'e' (0x65): <u32 len> <len bytes>   the configuration payload
//
// The header length VARIES with the design name (71 bytes for top.bit,
// 73 for blink.bit), so the payload offset must never be hardcoded --
// it is only ever found by walking this chain.
const bitPayloadTag = 0x65 // 'e'

// bitCursor walks the container, bounds-checking every read so a
// truncated or corrupt file produces an error instead of a panic.
type bitCursor struct {
	data []byte
	p    int
}

func (c *bitCursor) take(n int, what string) ([]byte, error) {
	if n < 0 || c.p+n > len(c.data) {
		return nil, fmt.Errorf("truncated .bit: %s wants %d bytes at offset %d, only %d remain", what, n, c.p, len(c.data)-c.p)
	}
	b := c.data[c.p : c.p+n]
	c.p += n
	return b, nil
}

func (c *bitCursor) u8(what string) (byte, error) {
	b, err := c.take(1, what)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

func (c *bitCursor) u16(what string) (int, error) {
	b, err := c.take(2, what)
	if err != nil {
		return 0, err
	}
	return int(binary.BigEndian.Uint16(b)), nil
}

func (c *bitCursor) u32(what string) (int64, error) {
	b, err := c.take(4, what)
	if err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint32(b)), nil
}

// lengthPrefixed reads <u16 len> <len bytes>.
func (c *bitCursor) lengthPrefixed(what string) ([]byte, error) {
	n, err := c.u16(what + " length")
	if err != nil {
		return nil, err
	}
	return c.take(n, what)
}

func ParseBitstream(data []byte) (*Bitstream, error) {
	c := &bitCursor{data: data}

	if _, err := c.lengthPrefixed("prologue"); err != nil {
		return nil, err
	}
	firstTag, err := c.lengthPrefixed("first tag field")
	if err != nil {
		return nil, err
	}
	if len(firstTag) != 1 {
		return nil, fmt.Errorf("expected a 1-byte tag field, got %d bytes", len(firstTag))
	}

	bs := &Bitstream{}
	tag := firstTag[0]
	for tag != bitPayloadTag {
		value, err := c.lengthPrefixed(fmt.Sprintf("field %q", tag))
		if err != nil {
			return nil, err
		}
		text := strings.TrimRight(string(value), "\x00")
		switch tag {
		case 'a':
			bs.Design = text
		case 'b':
			bs.Part = text
		case 'c':
			bs.Date = text
		case 'd':
			bs.Time = text
		}
		// Unknown tags are kept walkable rather than fatal: the chain is
		// self-describing, so a field this tool does not care about
		// still tells us where the next one starts.
		if tag, err = c.u8("next field tag"); err != nil {
			return nil, fmt.Errorf("field chain ended before the payload tag 'e': %w", err)
		}
	}

	plen, err := c.u32("payload length")
	if err != nil {
		return nil, err
	}
	if plen > int64(len(data)) {
		return nil, fmt.Errorf("payload declares %d bytes, larger than the whole %d-byte file", plen, len(data))
	}
	payload, err := c.take(int(plen), "payload")
	if err != nil {
		return nil, err
	}
	if c.p != len(data) {
		return nil, fmt.Errorf("%d trailing bytes after the payload", len(data)-c.p)
	}
	bs.Payload = payload
	return bs, nil
}

// FPGAPins maps each slave-serial signal to the 7688 GPIO that drives
// it through the 74LS125 level-shifting adapter.
//
// NOT reachable from the fpga_configure tool any more. This board cannot
// spare five GPIOs -- the Duo breaks out P1, P8 and P9, and P8/P9 are the
// console UART -- so the download runs on the ATmega instead (see
// fpga_bridge.go). This type and sysfsPort below are kept, tested, for a
// board that does have the pins.
type FPGAPins struct {
	DIN     int `json:"din" jsonschema:"GPIO driving DB25 pin 2 (PD0 / DIN)"`
	CCLK    int `json:"cclk" jsonschema:"GPIO driving DB25 pin 3 (PD1 / CCLK)"`
	Program int `json:"prog" jsonschema:"GPIO driving DB25 pin 4 (PD2 / /PROGRAM)"`
	Gate    int `json:"gate" jsonschema:"GPIO driving DB25 pin 5 (PD3 / buffer gate; LOW hands the bus to us)"`
	Done    int `json:"done" jsonschema:"GPIO reading DB25 pin 13 (SLCT / DONE) through the 10K:15K divider"`
}

// signals lists the pins in a fixed order with the names used in error
// messages, so validation and setup cannot drift apart.
func (p FPGAPins) signals() []struct {
	name string
	pin  int
	dir  string
} {
	return []struct {
		name string
		pin  int
		dir  string
	}{
		{"din", p.DIN, "out"},
		{"cclk", p.CCLK, "out"},
		{"prog", p.Program, "out"},
		{"gate", p.Gate, "out"},
		{"done", p.Done, "in"},
	}
}

func validateFPGAPins(p FPGAPins) error {
	seen := make(map[int]string, 5)
	for _, s := range p.signals() {
		switch {
		case s.pin == 0:
			return fmt.Errorf("pins.%s is not set: all five pins (din, cclk, prog, gate, done) must be given explicitly because the adapter wiring is not frozen; GPIO 0 is blocked on this board anyway, so 0 can only mean unset", s.name)
		case s.pin < 0:
			return fmt.Errorf("pins.%s must be a GPIO number, got %d", s.name, s.pin)
		}
		if other, dup := seen[s.pin]; dup {
			return fmt.Errorf("pins.%s and pins.%s are both GPIO %d: each slave-serial signal needs its own pin", other, s.name, s.pin)
		}
		seen[s.pin] = s.name
		if err := CheckBlocked(s.pin); err != nil {
			return fmt.Errorf("pins.%s: %w", s.name, err)
		}
	}
	return nil
}

// sysfsPort drives the slave-serial signals over the sysfs GPIO ABI with
// one long-lived fd per pin.
//
// The open/write/close-per-call pattern in gpio.go is right for one-shot
// MCP calls but must not be reused here: this streams 178,144 bits at
// three pin writes each, and paying an open+close per write would make a
// download take minutes instead of seconds. The board's kernel is 3.18,
// which predates the fast gpiochip character-device uAPI, so pre-opened
// sysfs fds rewritten in place are the fastest thing available.
type sysfsPort struct {
	din, cclk, prog, gate *os.File // O_WRONLY
	done                  *os.File // O_RDONLY
}

func openSysfsPort(p FPGAPins) (port *sysfsPort, err error) {
	if err := validateFPGAPins(p); err != nil {
		return nil, err
	}

	port = &sysfsPort{}
	targets := map[string]**os.File{
		"din": &port.din, "cclk": &port.cclk, "prog": &port.prog,
		"gate": &port.gate, "done": &port.done,
	}
	// Any partial setup is closed again rather than leaked; a caller that
	// gets an error never receives a port to Close itself.
	defer func() {
		if err != nil {
			port.Close()
			port = nil
		}
	}()

	for _, s := range p.signals() {
		// SetGPIODirection exports the pin and re-checks the blocklist,
		// so nothing here can drive a pin the blocklist protects.
		if err := SetGPIODirection(s.pin, s.dir); err != nil {
			return nil, fmt.Errorf("pins.%s (gpio %d): %w", s.name, s.pin, err)
		}
		flag := os.O_WRONLY
		if s.dir == "in" {
			flag = os.O_RDONLY
		}
		path := fmt.Sprintf("%s/gpio%d/value", gpioSysfsPath, s.pin)
		f, err := os.OpenFile(path, flag, 0)
		if err != nil {
			return nil, fmt.Errorf("pins.%s (gpio %d): open value: %w", s.name, s.pin, err)
		}
		*targets[s.name] = f
	}
	return port, nil
}

// writeValue rewrites the value attribute at offset 0 (pwrite) instead
// of appending, which is what makes a single fd reusable for the whole
// stream.
func writeValue(f *os.File, high bool) error {
	v := []byte("0")
	if high {
		v = []byte("1")
	}
	_, err := f.WriteAt(v, 0)
	return err
}

func (p *sysfsPort) SetDIN(high bool) error     { return writeValue(p.din, high) }
func (p *sysfsPort) SetCCLK(high bool) error    { return writeValue(p.cclk, high) }
func (p *sysfsPort) SetProgram(high bool) error { return writeValue(p.prog, high) }
func (p *sysfsPort) SetGate(high bool) error    { return writeValue(p.gate, high) }

func (p *sysfsPort) ReadDone() (bool, error) {
	var buf [8]byte
	// ReadAt from offset 0 every time: the fd stays open across the whole
	// download, so a plain Read would return EOF after the first call.
	n, err := p.done.ReadAt(buf[:], 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("read DONE value: %w", err)
	}
	return strings.TrimSpace(string(buf[:n])) == "1", nil
}

// Close releases every fd it managed to open, reporting the first
// failure but never stopping early on one.
func (p *sysfsPort) Close() error {
	var firstErr error
	for _, f := range []*os.File{p.din, p.cclk, p.prog, p.gate, p.done} {
		if f == nil {
			continue
		}
		if err := f.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// slaveSerialPort is the pin-level interface the sequencer drives. The
// names are the DB25 net roles from the design doc so this code reads
// against the schematic.
type slaveSerialPort interface {
	SetDIN(high bool) error
	SetCCLK(high bool) error
	SetProgram(high bool) error // /PROGRAM, active low
	SetGate(high bool) error    // PD3: LOW = the 74HC125s drive the board, HIGH = Hi-Z
	ReadDone() (bool, error)
}

// xcs20PartName is the part string bitgen writes into tag 'b' for the
// XCS20-4-PQ208 this board carries. A bitstream for any other part
// cannot configure it, so it is rejected unless the caller forces it.
const xcs20PartName = "s20pq208"

// FPGAConfigureResult is what a download reports back.
type FPGAConfigureResult struct {
	Done   bool
	Bits   int
	Ms     int
	Part   string
	Design string
	DryRun bool
}

// ConfigureFPGA reads a .bit from the local filesystem (scp'd to the
// board's tmpfs by the host-side caller), checks it is for this part,
// and streams it into the FPGA through the ATmega bridge.
//
// There is no pin parameter. The signals are driven by the ATmega, not
// this chip, on pins its firmware has hardcoded (see fpga-download.ino)
// -- the Duo has only one free GPIO of its own, which is what pushed the
// download onto the bridge in the first place. See fpga_bridge.go.
//
// dryRun stops after parsing and validation: it reports what would be
// sent without touching the bridge or driving a single pin, which is the
// safe first call while the adapter wiring is still being brought up.
func ConfigureFPGA(bitPath string, dryRun, force bool) (*FPGAConfigureResult, error) {
	data, err := os.ReadFile(bitPath)
	if err != nil {
		return nil, fmt.Errorf("read bitstream: %w", err)
	}
	bs, err := ParseBitstream(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", bitPath, err)
	}
	if bs.Part != xcs20PartName && !force {
		return nil, fmt.Errorf("bitstream is built for part %q, but this board carries %q; pass force to stream it anyway", bs.Part, xcs20PartName)
	}

	res := &FPGAConfigureResult{
		Bits:   len(bs.Payload) * 8,
		Part:   bs.Part,
		Design: bs.Design,
		DryRun: dryRun,
	}
	if dryRun {
		return res, nil
	}

	stream, err := openAtmegaStream()
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	start := time.Now()
	done, err := ConfigureViaATmega(stream, bs.Payload)
	if err != nil {
		return nil, err
	}
	res.Done = done
	res.Ms = int(time.Since(start).Milliseconds())
	return res, nil
}

// Timing. Slave-serial CCLK has no minimum frequency (the configuration
// logic is fully static) and a maximum around 8MHz, so these delays are
// chosen with margins wide enough that datasheet precision is irrelevant.
const (
	// /PROGRAM low time. The spec minimum is sub-microsecond; 10ms is
	// ~30000x that, and costs nothing once per download.
	programLowHold = 10 * time.Millisecond
	// Time for the FPGA to clear its configuration memory after
	// /PROGRAM rises. INIT is not wired out to the DB25 on this board,
	// so this is a fixed wait rather than a poll of the real signal.
	programClearWait = 100 * time.Millisecond
	// Extra CCLK cycles after the payload to run the startup sequence
	// that asserts DONE.
	startupClocks = 16
)

// fpgaSleep is a var so tests can run the sequencer without waiting out
// the real 110ms of protocol delays; same test-seam pattern as
// spiDevGlob in spi.go.
var fpgaSleep = time.Sleep

// ConfigureSlaveSerial streams payload into the FPGA over the slave
// serial interface and reports whether DONE came up.
//
// A DONE that never rises is NOT an error: the bytes went out fine and
// the FPGA simply did not accept them (bit order, stream integrity or
// length count), which is a different diagnosis from a GPIO write that
// failed. Only transport failures return a non-nil error.
//
// The bus gate (PD3) is handed back on every exit path. Leaving it low
// would keep the 74HC125s driving the FPGA's config pins after this
// call returns, which is the one state on this board worth being
// paranoid about.
func ConfigureSlaveSerial(port slaveSerialPort, payload []byte) (done bool, err error) {
	if len(payload) == 0 {
		return false, fmt.Errorf("empty payload: nothing to configure")
	}

	// Park the gate high before touching anything else, so the buffers
	// are Hi-Z even if a later step fails immediately.
	if err := port.SetGate(true); err != nil {
		return false, fmt.Errorf("park bus gate: %w", err)
	}
	defer func() {
		if rerr := port.SetGate(true); rerr != nil && err == nil {
			err = fmt.Errorf("release bus gate: %w", rerr)
		}
	}()

	if err := port.SetProgram(true); err != nil {
		return false, fmt.Errorf("idle /PROGRAM high: %w", err)
	}
	if err := port.SetCCLK(false); err != nil {
		return false, fmt.Errorf("idle CCLK low: %w", err)
	}
	if err := port.SetDIN(true); err != nil {
		return false, fmt.Errorf("idle DIN high: %w", err)
	}
	if _, err := port.ReadDone(); err != nil {
		return false, fmt.Errorf("read DONE before configuring: %w", err)
	}

	// Take the bus: PD3 low enables the three 74HC125 channels.
	if err := port.SetGate(false); err != nil {
		return false, fmt.Errorf("take bus gate: %w", err)
	}

	// /PROGRAM pulse.
	if err := port.SetProgram(false); err != nil {
		return false, fmt.Errorf("drive /PROGRAM low: %w", err)
	}
	fpgaSleep(programLowHold)
	if err := port.SetProgram(true); err != nil {
		return false, fmt.Errorf("release /PROGRAM high: %w", err)
	}

	// DONE must have dropped. If it did not, either the gate is not
	// reaching the buffers or DONE readback is not connected -- streaming
	// 22KB into that is pointless and tells us nothing.
	cleared, err := port.ReadDone()
	if err != nil {
		return false, fmt.Errorf("read DONE after the /PROGRAM pulse: %w", err)
	}
	if cleared {
		return false, fmt.Errorf("DONE still reads high after the /PROGRAM pulse: the FPGA did not enter configuration mode (check the PD3 gate, /PROGRAM and DONE wiring)")
	}

	fpgaSleep(programClearWait)

	// Stream the payload, MSB of each byte first; DIN is sampled on the
	// rising edge of CCLK.
	for i, b := range payload {
		for shift := 7; shift >= 0; shift-- {
			if err := port.SetDIN(b>>uint(shift)&1 == 1); err != nil {
				return false, fmt.Errorf("set DIN at byte %d bit %d: %w", i, 7-shift, err)
			}
			if err := port.SetCCLK(true); err != nil {
				return false, fmt.Errorf("raise CCLK at byte %d bit %d: %w", i, 7-shift, err)
			}
			if err := port.SetCCLK(false); err != nil {
				return false, fmt.Errorf("lower CCLK at byte %d bit %d: %w", i, 7-shift, err)
			}
		}
	}

	// Startup: DIN held high, extra clocks, watching DONE come up. All
	// startupClocks are issued even once DONE rises -- the startup
	// sequence has further internal steps (I/O enable, GSR release)
	// clocked by the same CCLK.
	if err := port.SetDIN(true); err != nil {
		return false, fmt.Errorf("hold DIN high for startup: %w", err)
	}
	for i := range startupClocks {
		if err := port.SetCCLK(true); err != nil {
			return false, fmt.Errorf("raise CCLK on startup clock %d: %w", i, err)
		}
		if err := port.SetCCLK(false); err != nil {
			return false, fmt.Errorf("lower CCLK on startup clock %d: %w", i, err)
		}
		v, err := port.ReadDone()
		if err != nil {
			return false, fmt.Errorf("read DONE on startup clock %d: %w", i, err)
		}
		// Latch: DONE is reported if it came up at any point during
		// startup, not only on the last clock.
		done = done || v
	}
	return done, nil
}
