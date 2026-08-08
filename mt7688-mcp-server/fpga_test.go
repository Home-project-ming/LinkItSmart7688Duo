// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The two testdata files are real bitgen 4.2i output for xcs20-4-pq208,
// kept because their headers are DIFFERENT lengths (71 vs 73 bytes) --
// that is the whole reason the parser may never hardcode a payload
// offset. See the design doc's "Bitstream facts" section.
const (
	xcs20Part        = "s20pq208"
	xcs20PayloadLen  = 22268  // bytes
	xcs20PayloadBits = 178144 // XCS20 configuration length, exact
)

func readTestBit(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return data
}

func TestParseBitstream_RealTopBit(t *testing.T) {
	data := readTestBit(t, "top.bit")

	bs, err := ParseBitstream(data)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if bs.Design != "top_par.ncd" {
		t.Errorf("design: want %q, got %q", "top_par.ncd", bs.Design)
	}
	if bs.Part != xcs20Part {
		t.Errorf("part: want %q, got %q", xcs20Part, bs.Part)
	}
	if bs.Date != "2026/07/24" {
		t.Errorf("date: want %q, got %q", "2026/07/24", bs.Date)
	}
	if len(bs.Payload) != xcs20PayloadLen {
		t.Errorf("payload: want %d bytes, got %d", xcs20PayloadLen, len(bs.Payload))
	}
	if header := len(data) - len(bs.Payload); header != 71 {
		t.Errorf("header: want 71 bytes, got %d", header)
	}
}

// Same part, same payload length, but a 73-byte header -- the regression
// lock against reintroducing a fixed payload offset.
func TestParseBitstream_RealBlinkBit_HasDifferentHeaderLength(t *testing.T) {
	data := readTestBit(t, "blink.bit")

	bs, err := ParseBitstream(data)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if bs.Design != "blink_par.ncd" {
		t.Errorf("design: want %q, got %q", "blink_par.ncd", bs.Design)
	}
	if bs.Part != xcs20Part {
		t.Errorf("part: want %q, got %q", xcs20Part, bs.Part)
	}
	if len(bs.Payload) != xcs20PayloadLen {
		t.Errorf("payload: want %d bytes, got %d", xcs20PayloadLen, len(bs.Payload))
	}
	if header := len(data) - len(bs.Payload); header != 73 {
		t.Errorf("header: want 73 bytes, got %d", header)
	}
}

// The stream self-frames: 8 fill '1's, then the 0010 preamble, then the
// 24-bit length count. Checking it here means a byte-order or
// bit-order mistake upstream cannot reach the FPGA silently.
func TestParseBitstream_PayloadStartsWithFillAndPreamble(t *testing.T) {
	bs, err := ParseBitstream(readTestBit(t, "top.bit"))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got := bs.Payload[0]; got != 0xff {
		t.Errorf("fill byte: want 0xff, got %#02x", got)
	}
	if got := bs.Payload[1] >> 4; got != 0b0010 {
		t.Errorf("preamble nibble: want 0010, got %04b", got)
	}
	// The 24 bits after the preamble are the length count, 0x02B7D9.
	lengthCount := uint32(bs.Payload[1]&0x0f)<<20 |
		uint32(bs.Payload[2])<<12 | uint32(bs.Payload[3])<<4 | uint32(bs.Payload[4])>>4
	if lengthCount != 0x02B7D9 {
		t.Errorf("length count: want 0x02B7D9, got %#06X", lengthCount)
	}
}

func TestParseBitstream_TruncatedPayload(t *testing.T) {
	data := readTestBit(t, "top.bit")

	if _, err := ParseBitstream(data[:len(data)-1]); err == nil {
		t.Fatal("expected error for a payload one byte short, got nil")
	}
}

func TestParseBitstream_TrailingBytes(t *testing.T) {
	data := append(readTestBit(t, "top.bit"), 0x00)

	if _, err := ParseBitstream(data); err == nil {
		t.Fatal("expected error for trailing bytes after the payload, got nil")
	}
}

func TestParseBitstream_TruncatedHeader(t *testing.T) {
	data := readTestBit(t, "top.bit")

	// Every prefix of the header must be rejected, not panic: the header
	// is length-prefixed all the way down, so a short read has to be
	// caught at whichever field runs off the end.
	for _, n := range []int{0, 1, 2, 5, 11, 30, 60, 70} {
		if _, err := ParseBitstream(data[:n]); err == nil {
			t.Errorf("expected error for a %d-byte prefix, got nil", n)
		}
	}
}

func TestParseBitstream_NoPayloadTag(t *testing.T) {
	// A header whose field chain ends without ever reaching tag 'e'.
	data := []byte{
		0x00, 0x01, 0x00, // prologue, len 1
		0x00, 0x01, 'a', // the 1-byte tag field, tag 'a'
		0x00, 0x02, 'x', 0x00, // field 'a' = "x"
	}
	if _, err := ParseBitstream(data); err == nil {
		t.Fatal("expected error when the field chain ends before tag 'e', got nil")
	}
}

// --- slave-serial sequencer ---------------------------------------------
//
// These run against fakePort, never real GPIO, so the whole sequence is
// checked on a dev machine. The three properties that would cost real
// hardware if they regressed: the /PROGRAM pulse must precede any CCLK
// edge, bytes must go out MSB-first, and the bus gate (PD3) must be
// released no matter which step fails.

type fakePort struct {
	trace   []string
	din     bool
	shifted []bool // DIN as sampled on each CCLK rising edge
	cclk    bool
	clocks  int

	// done reports what the DONE pin reads given the number of CCLK
	// rising edges so far. nil means "stays low forever".
	done func(clocks int) bool

	// failAt makes the failAt'th pin operation (1-based) return failErr;
	// 0 disables the injection.
	failAt  int
	ops     int
	failErr error
}

func (f *fakePort) op(event string) error {
	f.ops++
	f.trace = append(f.trace, event)
	if f.failAt != 0 && f.ops == f.failAt {
		return f.failErr
	}
	return nil
}

func bit(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (f *fakePort) SetDIN(high bool) error {
	f.din = high
	return f.op(fmt.Sprintf("din=%d", bit(high)))
}

func (f *fakePort) SetCCLK(high bool) error {
	if high && !f.cclk { // rising edge: this is when the FPGA samples DIN
		f.shifted = append(f.shifted, f.din)
		f.clocks++
	}
	f.cclk = high
	return f.op(fmt.Sprintf("cclk=%d", bit(high)))
}

func (f *fakePort) SetProgram(high bool) error {
	return f.op(fmt.Sprintf("prog=%d", bit(high)))
}

func (f *fakePort) SetGate(high bool) error {
	return f.op(fmt.Sprintf("gate=%d", bit(high)))
}

func (f *fakePort) ReadDone() (bool, error) {
	v := false
	if f.done != nil {
		v = f.done(f.clocks)
	}
	return v, f.op(fmt.Sprintf("done?=%d", bit(v)))
}

// firstIndex returns the position of event in the trace, or -1.
func firstIndex(trace []string, event string) int {
	for i, e := range trace {
		if e == event {
			return i
		}
	}
	return -1
}

// newTestPort returns a port whose DONE rises two clocks into the
// startup phase -- i.e. a download that succeeds normally.
func newTestPort(payloadBits int) *fakePort {
	return &fakePort{done: func(clocks int) bool { return clocks >= payloadBits+2 }}
}

// noSleep drops the sequencer's real protocol delays for the duration of
// one test. Those delays are hardware timing, not logic under test, and
// 110ms per call adds up across the error-injection table. The durations
// themselves stay covered by TestConfigureSlaveSerial_HonoursProtocolDelays.
func noSleep(t *testing.T) {
	t.Helper()
	old := fpgaSleep
	fpgaSleep = func(time.Duration) {}
	t.Cleanup(func() { fpgaSleep = old })
}

// The two waits are the only places this protocol depends on wall-clock
// time, and both have a hardware minimum: the /PROGRAM low hold, and the
// configuration-memory clear that follows it (INIT is not wired out on
// this board, so nothing else detects a too-short wait).
func TestConfigureSlaveSerial_HonoursProtocolDelays(t *testing.T) {
	var slept []time.Duration
	old := fpgaSleep
	fpgaSleep = func(d time.Duration) { slept = append(slept, d) }
	t.Cleanup(func() { fpgaSleep = old })

	payload := []byte{0x5A}
	if _, err := ConfigureSlaveSerial(newTestPort(len(payload)*8), payload); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(slept) != 2 {
		t.Fatalf("expected exactly 2 protocol delays, got %d: %v", len(slept), slept)
	}
	if slept[0] < 10*time.Millisecond {
		t.Errorf("/PROGRAM low hold: want >= 10ms, got %v", slept[0])
	}
	if slept[1] < 100*time.Millisecond {
		t.Errorf("configuration-memory clear wait: want >= 100ms, got %v", slept[1])
	}
}

func TestConfigureSlaveSerial_PulsesProgramBeforeAnyCCLK(t *testing.T) {
	noSleep(t)
	payload := []byte{0xA5, 0x3C}
	port := newTestPort(len(payload) * 8)

	if _, err := ConfigureSlaveSerial(port, payload); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	progLow := firstIndex(port.trace, "prog=0")
	progHigh := -1
	for i := progLow + 1; i < len(port.trace); i++ {
		if port.trace[i] == "prog=1" {
			progHigh = i
			break
		}
	}
	firstClock := firstIndex(port.trace, "cclk=1")
	if progLow < 0 || progHigh < 0 {
		t.Fatalf("expected a /PROGRAM low-then-high pulse, trace: %v", port.trace[:min(12, len(port.trace))])
	}
	if firstClock < 0 {
		t.Fatal("expected at least one CCLK rising edge")
	}
	if progHigh > firstClock {
		t.Errorf("/PROGRAM pulse must complete before the first CCLK rising edge: pulse ends at %d, first clock at %d", progHigh, firstClock)
	}
}

func TestConfigureSlaveSerial_TakesBusBeforePulsingProgram(t *testing.T) {
	noSleep(t)
	payload := []byte{0xFF}
	port := newTestPort(len(payload) * 8)

	if _, err := ConfigureSlaveSerial(port, payload); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	// PD3 low = the 74HC125s drive the board. It has to happen before we
	// try to pull /PROGRAM, otherwise the pulse goes nowhere.
	gateLow := firstIndex(port.trace, "gate=0")
	progLow := firstIndex(port.trace, "prog=0")
	if gateLow < 0 {
		t.Fatal("expected the gate to be taken low")
	}
	if gateLow > progLow {
		t.Errorf("gate must go low (index %d) before /PROGRAM is pulled low (index %d)", gateLow, progLow)
	}
	// ...and the very first thing written must be gate=1, so a half-booted
	// board never sees us drive the bus before we mean to.
	if port.trace[0] != "gate=1" {
		t.Errorf("first pin operation should park the gate high, got %q", port.trace[0])
	}
}

func TestConfigureSlaveSerial_ShiftsBytesMSBFirst(t *testing.T) {
	noSleep(t)
	payload := []byte{0x80, 0x01, 0xA5}
	port := newTestPort(len(payload) * 8)

	if _, err := ConfigureSlaveSerial(port, payload); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	want := []bool{
		true, false, false, false, false, false, false, false, // 0x80
		false, false, false, false, false, false, false, true, // 0x01
		true, false, true, false, false, true, false, true, // 0xA5
	}
	if len(port.shifted) < len(want) {
		t.Fatalf("expected at least %d clocked bits, got %d", len(want), len(port.shifted))
	}
	for i, w := range want {
		if port.shifted[i] != w {
			t.Fatalf("bit %d: want %d, got %d (full: %v)", i, bit(w), bit(port.shifted[i]), port.shifted[:len(want)])
		}
	}
}

func TestConfigureSlaveSerial_IssuesStartupClocksWithDINHigh(t *testing.T) {
	noSleep(t)
	payload := []byte{0x00}
	port := newTestPort(len(payload) * 8)

	if _, err := ConfigureSlaveSerial(port, payload); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	payloadBits := len(payload) * 8
	if got := len(port.shifted) - payloadBits; got < 16 {
		t.Errorf("expected at least 16 startup clocks after the payload, got %d", got)
	}
	for i := payloadBits; i < len(port.shifted); i++ {
		if !port.shifted[i] {
			t.Errorf("startup clock %d sampled DIN low; DIN must be held high during startup", i-payloadBits)
		}
	}
}

func TestConfigureSlaveSerial_ReportsDoneHigh(t *testing.T) {
	noSleep(t)
	payload := []byte{0x5A}
	port := newTestPort(len(payload) * 8)

	done, err := ConfigureSlaveSerial(port, payload)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !done {
		t.Error("expected done=true when DONE rises during startup")
	}
}

// DONE never rising is not a transport error -- the stream went out
// fine, the FPGA just did not accept it. Report it, do not fail.
func TestConfigureSlaveSerial_ReportsDoneLowWithoutError(t *testing.T) {
	noSleep(t)
	port := &fakePort{done: nil}

	done, err := ConfigureSlaveSerial(port, []byte{0x5A})
	if err != nil {
		t.Fatalf("expected nil error when DONE never rises, got %v", err)
	}
	if done {
		t.Error("expected done=false")
	}
}

// Step 3 of the protocol: after the /PROGRAM pulse DONE must read low.
// If it does not, the wiring is wrong and streaming 22KB into an
// unknown state is pointless -- abort, and hand the bus back.
func TestConfigureSlaveSerial_AbortsWhenProgramDoesNotClearDone(t *testing.T) {
	noSleep(t)
	port := &fakePort{done: func(int) bool { return true }}

	_, err := ConfigureSlaveSerial(port, []byte{0x5A})
	if err == nil {
		t.Fatal("expected an error when DONE stays high after the /PROGRAM pulse, got nil")
	}
	if firstIndex(port.trace, "cclk=1") >= 0 {
		t.Error("no CCLK edge should be issued after the DONE check fails")
	}
	if last := port.trace[len(port.trace)-1]; last != "gate=1" {
		t.Errorf("gate must be released on abort; last operation was %q", last)
	}
}

// The gate is the one thing that must be handed back on every exit path,
// including a mid-stream I/O error -- otherwise the 74HC125s keep
// driving the FPGA's config pins after we are gone.
func TestConfigureSlaveSerial_ReleasesGateOnPinError(t *testing.T) {
	noSleep(t)
	payload := make([]byte, 64)
	sentinel := errors.New("sysfs write failed")

	// Walk the injection point across the whole sequence so no step is
	// left with an un-deferred release.
	for _, failAt := range []int{2, 3, 4, 5, 6, 8, 12, 40, 120, 400} {
		port := &fakePort{
			done:    func(clocks int) bool { return clocks >= len(payload)*8+2 },
			failAt:  failAt,
			failErr: sentinel,
		}

		_, err := ConfigureSlaveSerial(port, payload)
		if err == nil {
			t.Errorf("failAt=%d: expected an error, got nil", failAt)
			continue
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("failAt=%d: expected the sentinel to be wrapped, got %v", failAt, err)
		}
		if last := port.trace[len(port.trace)-1]; last != "gate=1" {
			t.Errorf("failAt=%d: gate must be released on error; last operation was %q", failAt, last)
		}
	}
}

// A failure of the release itself must not be silently swallowed when
// the sequence would otherwise have succeeded.
func TestConfigureSlaveSerial_ReportsGateReleaseFailure(t *testing.T) {
	noSleep(t)
	payload := []byte{0x5A}
	port := newTestPort(len(payload) * 8)
	sentinel := errors.New("release failed")
	port.failErr = sentinel

	// Count the operations of a clean run, then fail the last one.
	clean := newTestPort(len(payload) * 8)
	if _, err := ConfigureSlaveSerial(clean, payload); err != nil {
		t.Fatalf("setup: %v", err)
	}
	port.failAt = len(clean.trace)

	if _, err := ConfigureSlaveSerial(port, payload); !errors.Is(err, sentinel) {
		t.Errorf("expected the gate-release error to surface, got %v", err)
	}
}

// --- top-level configure --------------------------------------------------

// makeBit builds a synthetic .bit container so a part name other than
// the XCS20's can be tested without a second real bitstream.
func makeBit(design, part string, payload []byte) []byte {
	var out []byte
	lenPrefixed := func(b []byte) {
		out = append(out, byte(len(b)>>8), byte(len(b)))
		out = append(out, b...)
	}
	lenPrefixed([]byte{0x0f, 0xf0, 0x0f, 0xf0, 0x0f, 0xf0, 0x0f, 0xf0, 0x00})
	lenPrefixed([]byte{'a'})
	for _, f := range []struct {
		value string
		next  byte
	}{
		{design, 'b'}, {part, 'c'}, {"2026/08/08", 'd'}, {"12:00:00", 'e'},
	} {
		lenPrefixed(append([]byte(f.value), 0x00))
		out = append(out, f.next)
	}
	n := len(payload)
	out = append(out, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	return append(out, payload...)
}

func writeTempBit(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "design.bit")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return path
}

// A dry run parses and validates everything but never drives a pin --
// which is checkable here precisely because gpioSysfsPath still points
// at the real /sys, absent on this dev machine: any attempt to touch
// GPIO would surface as an error.
func TestConfigureFPGA_DryRunReportsHeaderWithoutDrivingPins(t *testing.T) {
	path := writeTempBit(t, readTestBit(t, "blink.bit"))

	res, err := ConfigureFPGA(path, true, false)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !res.DryRun {
		t.Error("expected dry_run to be reported back")
	}
	if res.Part != xcs20Part {
		t.Errorf("part: want %q, got %q", xcs20Part, res.Part)
	}
	if res.Design != "blink_par.ncd" {
		t.Errorf("design: want %q, got %q", "blink_par.ncd", res.Design)
	}
	if res.Bits != xcs20PayloadBits {
		t.Errorf("bits: want %d, got %d", xcs20PayloadBits, res.Bits)
	}
	if res.Done {
		t.Error("a dry run cannot have observed DONE")
	}
}

// Streaming a bitstream built for another part cannot work and wastes a
// bring-up cycle chasing wiring that is fine.
func TestConfigureFPGA_RejectsWrongPart(t *testing.T) {
	path := writeTempBit(t, makeBit("other_par.ncd", "s30pq208", make([]byte, 16)))

	_, err := ConfigureFPGA(path, true, false)
	if err == nil {
		t.Fatal("expected an error for a bitstream built for another part, got nil")
	}
	if !strings.Contains(err.Error(), "s30pq208") || !strings.Contains(err.Error(), xcs20Part) {
		t.Errorf("error should name both the found and the expected part, got %v", err)
	}
}

func TestConfigureFPGA_WrongPartAllowedWithForce(t *testing.T) {
	path := writeTempBit(t, makeBit("other_par.ncd", "s30pq208", make([]byte, 16)))

	res, err := ConfigureFPGA(path, true, true)
	if err != nil {
		t.Fatalf("expected force to override the part check, got %v", err)
	}
	if res.Part != "s30pq208" {
		t.Errorf("part: want %q, got %q", "s30pq208", res.Part)
	}
}

func TestConfigureFPGA_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.bit")

	if _, err := ConfigureFPGA(path, true, false); err == nil {
		t.Fatal("expected an error for a missing .bit, got nil")
	}
}

// --- pin wiring -----------------------------------------------------------

// Five pins that are not in blockedPins, from the design doc's
// candidate list. The real wiring is not frozen yet, which is exactly
// why the pins are tool inputs rather than constants.
func testPins() FPGAPins {
	return FPGAPins{DIN: 39, CCLK: 40, Program: 41, Gate: 42, Done: 43}
}

func TestValidateFPGAPins_AcceptsDistinctUnblockedPins(t *testing.T) {
	if err := validateFPGAPins(testPins()); err != nil {
		t.Fatalf("expected nil for valid pins, got %v", err)
	}
}

func TestValidateFPGAPins_RejectsUnspecified(t *testing.T) {
	pins := testPins()
	pins.CCLK = 0
	if err := validateFPGAPins(pins); err == nil {
		t.Fatal("expected an error when a pin is left unspecified, got nil")
	}
}

func TestValidateFPGAPins_RejectsNegative(t *testing.T) {
	pins := testPins()
	pins.Done = -1
	if err := validateFPGAPins(pins); err == nil {
		t.Fatal("expected an error for a negative pin number, got nil")
	}
}

// Two signals on one pin would silently produce a download that cannot
// work, with no electrical symptom to debug.
func TestValidateFPGAPins_RejectsDuplicates(t *testing.T) {
	pins := testPins()
	pins.Gate = pins.CCLK
	if err := validateFPGAPins(pins); err == nil {
		t.Fatal("expected an error when two signals share a pin, got nil")
	}
}

func TestValidateFPGAPins_RejectsBlockedPin(t *testing.T) {
	pins := testPins()
	pins.Program = 22 // SDXC/extroot
	err := validateFPGAPins(pins)
	if err == nil {
		t.Fatal("expected an error for a blocked pin, got nil")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("expected the blocklist reason to surface, got %v", err)
	}
}

// --- sysfs port -----------------------------------------------------------

// fakeSysfs builds the subset of /sys/class/gpio the port touches, so
// the open/direction/value plumbing is exercised without real hardware.
func fakeSysfs(t *testing.T, pins ...int) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "export"), nil, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	for _, p := range pins {
		dir := filepath.Join(root, fmt.Sprintf("gpio%d", p))
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		for _, f := range []string{"direction", "value"} {
			if err := os.WriteFile(filepath.Join(dir, f), nil, 0644); err != nil {
				t.Fatalf("setup: %v", err)
			}
		}
	}
	old := gpioSysfsPath
	gpioSysfsPath = root
	t.Cleanup(func() { gpioSysfsPath = old })
	return root
}

func readSysfsValue(t *testing.T, root string, pin int) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, fmt.Sprintf("gpio%d", pin), "value"))
	if err != nil {
		t.Fatalf("read value: %v", err)
	}
	return string(b)
}

func TestOpenSysfsPort_SetsDirections(t *testing.T) {
	pins := testPins()
	root := fakeSysfs(t, pins.DIN, pins.CCLK, pins.Program, pins.Gate, pins.Done)

	port, err := openSysfsPort(pins)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	defer port.Close()

	for _, tc := range []struct {
		pin  int
		want string
	}{
		{pins.DIN, "out"}, {pins.CCLK, "out"}, {pins.Program, "out"},
		{pins.Gate, "out"}, {pins.Done, "in"},
	} {
		b, err := os.ReadFile(filepath.Join(root, fmt.Sprintf("gpio%d", tc.pin), "direction"))
		if err != nil {
			t.Fatalf("read direction: %v", err)
		}
		if string(b) != tc.want {
			t.Errorf("gpio%d direction: want %q, got %q", tc.pin, tc.want, string(b))
		}
	}
}

func TestSysfsPort_WritesEachSignalToItsOwnPin(t *testing.T) {
	pins := testPins()
	root := fakeSysfs(t, pins.DIN, pins.CCLK, pins.Program, pins.Gate, pins.Done)

	port, err := openSysfsPort(pins)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	defer port.Close()

	if err := port.SetDIN(true); err != nil {
		t.Fatalf("SetDIN: %v", err)
	}
	if err := port.SetCCLK(false); err != nil {
		t.Fatalf("SetCCLK: %v", err)
	}
	if err := port.SetProgram(true); err != nil {
		t.Fatalf("SetProgram: %v", err)
	}
	if err := port.SetGate(false); err != nil {
		t.Fatalf("SetGate: %v", err)
	}

	for _, tc := range []struct {
		name string
		pin  int
		want string
	}{
		{"DIN", pins.DIN, "1"}, {"CCLK", pins.CCLK, "0"},
		{"/PROGRAM", pins.Program, "1"}, {"gate", pins.Gate, "0"},
	} {
		if got := readSysfsValue(t, root, tc.pin); got != tc.want {
			t.Errorf("%s (gpio%d): want %q, got %q", tc.name, tc.pin, tc.want, got)
		}
	}
}

// Each value fd is opened once and reused for the whole 178k-bit stream,
// so a rewrite must land at offset 0 rather than appending.
func TestSysfsPort_RewritesValueInPlace(t *testing.T) {
	pins := testPins()
	root := fakeSysfs(t, pins.DIN, pins.CCLK, pins.Program, pins.Gate, pins.Done)

	port, err := openSysfsPort(pins)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	defer port.Close()

	for _, v := range []bool{true, false, true, true, false} {
		if err := port.SetCCLK(v); err != nil {
			t.Fatalf("SetCCLK: %v", err)
		}
	}
	if got := readSysfsValue(t, root, pins.CCLK); got != "0" {
		t.Errorf("after five writes the value file should hold just the last value, got %q", got)
	}
}

func TestSysfsPort_ReadsDone(t *testing.T) {
	pins := testPins()
	root := fakeSysfs(t, pins.DIN, pins.CCLK, pins.Program, pins.Gate, pins.Done)

	port, err := openSysfsPort(pins)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	defer port.Close()

	donePath := filepath.Join(root, fmt.Sprintf("gpio%d", pins.Done), "value")
	// The kernel's value attribute is "0\n"/"1\n"; the trailing newline
	// must not be mistaken for part of the value.
	for _, tc := range []struct {
		content string
		want    bool
	}{{"0\n", false}, {"1\n", true}, {"0", false}, {"1", true}} {
		if err := os.WriteFile(donePath, []byte(tc.content), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		got, err := port.ReadDone()
		if err != nil {
			t.Fatalf("ReadDone: %v", err)
		}
		if got != tc.want {
			t.Errorf("value %q: want %v, got %v", tc.content, tc.want, got)
		}
	}
}

func TestOpenSysfsPort_RejectsInvalidPins(t *testing.T) {
	pins := testPins()
	pins.Gate = pins.DIN
	if _, err := openSysfsPort(pins); err == nil {
		t.Fatal("expected openSysfsPort to reject invalid pins before touching sysfs, got nil")
	}
}

func TestConfigureSlaveSerial_RejectsEmptyPayload(t *testing.T) {
	noSleep(t)
	port := newTestPort(0)

	if _, err := ConfigureSlaveSerial(port, nil); err == nil {
		t.Fatal("expected an error for an empty payload, got nil")
	}
}
