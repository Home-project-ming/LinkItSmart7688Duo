// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// --- 7-bit packing --------------------------------------------------------

// Every length matters, not just the tidy multiple-of-7 one: the last
// chunk of a real 22,268-byte payload is 22 bytes, and an encoder that
// only handles whole groups silently drops it.
func TestEncode7bitRoundTripsEveryChunkLength(t *testing.T) {
	for n := 1; n <= fpgaChunkBytes; n++ {
		in := make([]byte, n)
		for i := range in {
			// 0x80-set bytes are the interesting ones: they are exactly
			// what cannot survive on a SysEx wire unencoded.
			in[i] = byte(i*7 + 0x80)
		}
		encoded := encode7bit(in)

		if want := (n*8 + 6) / 7; len(encoded) != want {
			t.Errorf("n=%d: encoded length %d, want %d", n, len(encoded), want)
		}
		for i, b := range encoded {
			if b&0x80 != 0 {
				t.Fatalf("n=%d: encoded byte %d is 0x%02x, bit 7 must be clear on a SysEx wire", n, i, b)
			}
		}

		out, err := decode7bit(encoded, n)
		if err != nil {
			t.Fatalf("n=%d: decode: %v", n, err)
		}
		if !bytes.Equal(in, out) {
			t.Errorf("n=%d: round trip differs\n in: %x\nout: %x", n, in, out)
		}
	}
}

func TestEncode7bitFullChunkPacksWithoutPadding(t *testing.T) {
	// 49*8 = 392 = 56*7 exactly, which is why 49 was chosen.
	if got := len(encode7bit(make([]byte, fpgaChunkBytes))); got != 56 {
		t.Errorf("a full chunk encodes to %d bytes, want 56", got)
	}
}

func TestDecode7bitRejectsShortInput(t *testing.T) {
	if _, err := decode7bit([]byte{0, 0}, 49); err == nil {
		t.Fatal("expected an error decoding 49 bytes from 2, got nil")
	}
}

// --- message framing ------------------------------------------------------

func TestBuildDataMessageFramesChunk(t *testing.T) {
	msg, err := buildDataMessage([]byte{0xFF, 0x00, 0xAA})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if msg[0] != sysexStart || msg[len(msg)-1] != sysexEnd {
		t.Errorf("message is not F0..F7 framed: % x", msg)
	}
	if msg[1] != sysexFPGA || msg[2] != fpgaSubData {
		t.Errorf("command/subcommand: got %02x %02x", msg[1], msg[2])
	}
	// The length field is the RAW count -- it is what tells the firmware
	// how much of the last encoded byte is padding.
	if msg[3] != 3 {
		t.Errorf("length field: got %d, want 3", msg[3])
	}
	for i, b := range msg[1 : len(msg)-1] {
		if b&0x80 != 0 {
			t.Fatalf("body byte %d is 0x%02x, bit 7 must be clear inside a SysEx frame", i, b)
		}
	}
}

func TestBuildDataMessageRejectsBadLengths(t *testing.T) {
	if _, err := buildDataMessage(nil); err == nil {
		t.Error("expected an error for an empty chunk")
	}
	if _, err := buildDataMessage(make([]byte, fpgaChunkBytes+1)); err == nil {
		t.Error("expected an error for an oversized chunk")
	}
}

// A full DATA message has to fit Firmata's parser buffer, or the
// firmware silently truncates it. MAX_DATA_BYTES is 64, read from the
// installed FirmataConstants.h.
func TestDataMessageFitsFirmataParserBuffer(t *testing.T) {
	msg, err := buildDataMessage(make([]byte, fpgaChunkBytes))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if buffered := len(msg) - 2; buffered > 64 {
		t.Errorf("a full DATA message buffers %d bytes, over Firmata's MAX_DATA_BYTES of 64", buffered)
	}
}

func TestTakeSysexSkipsNonSysexNoise(t *testing.T) {
	// Firmata.begin() emits an unsolicited REPORT_VERSION at every ATmega
	// boot; a download may well be the first thing that runs after one.
	buf := append([]byte{0xF9, 0x02, 0x05}, sysexStart, sysexFPGA, 0x01, sysexEnd)
	body, ok := takeSysex(&buf)
	if !ok {
		t.Fatal("expected a frame after the REPORT_VERSION noise")
	}
	if !bytes.Equal(body, []byte{sysexFPGA, 0x01}) {
		t.Errorf("body: % x", body)
	}
	if len(buf) != 0 {
		t.Errorf("consumed frame left %d bytes behind", len(buf))
	}
}

func TestTakeSysexWaitsForIncompleteFrame(t *testing.T) {
	buf := []byte{sysexStart, sysexFPGA, 0x01}
	if _, ok := takeSysex(&buf); ok {
		t.Fatal("a frame with no F7 must not be returned as complete")
	}
	if len(buf) != 3 {
		t.Errorf("the partial frame must be kept for the next read, got % x", buf)
	}

	buf = append(buf, 0x02, sysexEnd)
	body, ok := takeSysex(&buf)
	if !ok {
		t.Fatal("expected the frame once its F7 arrived")
	}
	if !bytes.Equal(body, []byte{sysexFPGA, 0x01, 0x02}) {
		t.Errorf("body: % x", body)
	}
}

// The returned body must not alias the buffer it came from: the caller
// holds it while the stream goes on appending.
func TestTakeSysexBodyDoesNotAliasBuffer(t *testing.T) {
	buf := []byte{sysexStart, sysexFPGA, 0x11, sysexEnd, sysexStart, sysexFPGA, 0x22, sysexEnd}
	first, _ := takeSysex(&buf)
	takeSysex(&buf)
	if first[1] != 0x11 {
		t.Errorf("first body was clobbered by the second frame: % x", first)
	}
}

func TestParseReplyDecodesSplitCount(t *testing.T) {
	// 22,268 does not fit in 7 bits, so the count travels as four 7-bit
	// groups; a parser that reads it as plain bytes gets a wrong answer.
	const n = 22268
	body := append([]byte{sysexFPGA}, encodeReplyFields(fpgaSubEnd, fpgaStatusOK, 1,
		byte(n&0x7F), byte((n>>7)&0x7F), byte((n>>14)&0x7F), byte((n>>21)&0x7F))...)

	reply, err := parseReply(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if reply.Received != n {
		t.Errorf("received: got %d, want %d", reply.Received, n)
	}
	if !reply.Done {
		t.Error("done flag was set on the wire but not decoded")
	}
}

// TestParseReplyOnCapturedFirmwareBytes pins the format against reality.
//
// These are the exact bytes the flashed ATmega put on the bridge on
// 2026-08-09 in reply to an END sent while not streaming (F0 7E 03 F7),
// read back through atmega_bridge_raw. The F7 fell outside the read window,
// which does not matter: takeSysex hands parseReply the body only.
//
// The previous parser read these as status=OK, done=true -- a clean success
// for a reply that actually says "you never sent a BEGIN".
func TestParseReplyOnCapturedFirmwareBytes(t *testing.T) {
	body := []byte{0x7E, 0x03, 0x00, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	reply, err := parseReply(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if reply.Sub != fpgaSubEnd {
		t.Errorf("Sub = 0x%02x, want 0x%02x", reply.Sub, fpgaSubEnd)
	}
	if reply.Status != fpgaStatusNotStreaming {
		t.Errorf("Status = 0x%02x, want 0x%02x (NOT_STREAMING)", reply.Status, fpgaStatusNotStreaming)
	}
	if reply.Done {
		t.Error("Done = true; the firmware reported false and a false success here is the whole bug")
	}
	if reply.Received != 0 {
		t.Errorf("Received = %d, want 0", reply.Received)
	}
}

func TestParseReplyRejectsShortBody(t *testing.T) {
	if _, err := parseReply(append([]byte{sysexFPGA}, encodeReplyFields(fpgaSubEnd, fpgaStatusOK)...)); err == nil {
		t.Fatal("expected an error for a truncated reply")
	}
}

// --- the download sequence ------------------------------------------------

// fakeStream records what was written and replies to BEGIN/END the way
// the firmware is specified to.
type fakeStream struct {
	writes   [][]byte
	received int  // payload bytes decoded out of the DATA messages
	done     bool // what END will report
	reportN  int  // count END reports; -1 means "whatever was received"
	beginErr byte
	closed   bool
}

func (f *fakeStream) Write(p []byte) error {
	f.writes = append(f.writes, append([]byte(nil), p...))
	if len(p) > 3 && p[1] == sysexFPGA && p[2] == fpgaSubData {
		f.received += int(p[3])
	}
	return nil
}

func (f *fakeStream) ReadSysex(time.Duration) ([]byte, error) {
	last := f.writes[len(f.writes)-1]
	sub := last[2]
	status := byte(fpgaStatusOK)
	if sub == fpgaSubBegin {
		status = f.beginErr
	}
	n := f.received
	if f.reportN >= 0 {
		n = f.reportN
	}
	done := byte(0)
	if f.done && sub == fpgaSubEnd {
		done = 1
	}
	// Built through encodeReplyFields, not hand-packed: an earlier version
	// of this fixture laid the fields out flat, which is not what
	// Firmata.sendSysex puts on the wire, and it hid a real parser bug.
	return append([]byte{sysexFPGA}, encodeReplyFields(sub, status, done,
		byte(n&0x7F), byte((n>>7)&0x7F), byte((n>>14)&0x7F), byte((n>>21)&0x7F))...), nil
}

func (f *fakeStream) Close() error { f.closed = true; return nil }

// subcommands returns the subcommand byte of every message written.
func (f *fakeStream) subcommands() []byte {
	var out []byte
	for _, w := range f.writes {
		out = append(out, w[2])
	}
	return out
}

// The whole point of the rewrite: a payload that is not a multiple of
// the chunk size must still arrive complete. 22,268 is 454 full chunks
// plus a 22-byte tail, and an implementation that only flushes full
// buffers drops that tail silently.
func TestConfigureViaATmegaSendsEveryByteIncludingShortTail(t *testing.T) {
	payload := make([]byte, 22268)
	for i := range payload {
		payload[i] = byte(i)
	}
	f := &fakeStream{done: true, reportN: -1}

	done, err := ConfigureViaATmega(f, payload)
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	if !done {
		t.Error("expected DONE to be reported back")
	}
	if f.received != len(payload) {
		t.Errorf("firmware saw %d of %d payload bytes", f.received, len(payload))
	}

	subs := f.subcommands()
	wantChunks := (len(payload) + fpgaChunkBytes - 1) / fpgaChunkBytes
	if got := bytes.Count(subs, []byte{fpgaSubData}); got != wantChunks {
		t.Errorf("sent %d DATA messages, want %d", got, wantChunks)
	}
	if subs[0] != fpgaSubBegin {
		t.Errorf("first message was 0x%02x, want BEGIN", subs[0])
	}
	// ABORT is the deferred bus release, so END is second from last.
	if subs[len(subs)-2] != fpgaSubEnd {
		t.Errorf("END is not second from last: % x", subs[len(subs)-4:])
	}
}

// The tail chunk must carry its real length, not a padded 49.
func TestConfigureViaATmegaTailChunkCarriesItsRealLength(t *testing.T) {
	payload := make([]byte, fpgaChunkBytes+7)
	f := &fakeStream{reportN: -1}

	if _, err := ConfigureViaATmega(f, payload); err != nil {
		t.Fatalf("configure: %v", err)
	}
	var lengths []byte
	for _, w := range f.writes {
		if w[2] == fpgaSubData {
			lengths = append(lengths, w[3])
		}
	}
	if want := []byte{fpgaChunkBytes, 7}; !bytes.Equal(lengths, want) {
		t.Errorf("chunk lengths: got %v, want %v", lengths, want)
	}
}

// A short count means chunks were lost, which makes DONE meaningless --
// reporting "done=false" alone would send someone probing the adapter
// for a fault that is actually on the bridge.
func TestConfigureViaATmegaReportsLostChunks(t *testing.T) {
	f := &fakeStream{done: true, reportN: 10}

	_, err := ConfigureViaATmega(f, make([]byte, 100))
	if err == nil {
		t.Fatal("expected an error when the firmware counted fewer bytes than were sent")
	}
	if !strings.Contains(err.Error(), "lost") {
		t.Errorf("error should say chunks were lost, got %v", err)
	}
}

func TestConfigureViaATmegaSurfacesBeginRefusal(t *testing.T) {
	f := &fakeStream{reportN: -1, beginErr: fpgaStatusDoneStuck}

	_, err := ConfigureViaATmega(f, make([]byte, 10))
	if err == nil {
		t.Fatal("expected an error when the firmware refuses BEGIN")
	}
	if !strings.Contains(err.Error(), "DONE still reads high") {
		t.Errorf("error should explain the refusal, got %v", err)
	}
	// No payload may go out after a refused BEGIN.
	if bytes.Contains(f.subcommands(), []byte{fpgaSubData}) {
		t.Error("streamed payload despite BEGIN being refused")
	}
}

// Leaving the gate low would keep the buffers driving the FPGA's config
// pins after the call returns, so the release has to survive every exit
// path, not just the happy one.
func TestConfigureViaATmegaAlwaysReleasesTheBus(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    *fakeStream
	}{
		{"success", &fakeStream{done: true, reportN: -1}},
		{"begin refused", &fakeStream{reportN: -1, beginErr: fpgaStatusDoneStuck}},
		{"chunks lost", &fakeStream{reportN: 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ConfigureViaATmega(tc.f, make([]byte, 60))
			if last := tc.f.subcommands(); last[len(last)-1] != fpgaSubAbort {
				t.Errorf("last message was 0x%02x, want ABORT to release the gate", last[len(last)-1])
			}
		})
	}
}

func TestConfigureViaATmegaRejectsEmptyPayload(t *testing.T) {
	if _, err := ConfigureViaATmega(&fakeStream{reportN: -1}, nil); err == nil {
		t.Fatal("expected an error for an empty payload")
	}
}

// Wall-clock sanity: this is what the tool promises callers.
func TestChunkCountForRealBitstreamIsWhatTheDocsClaim(t *testing.T) {
	const payloadBytes = 22268
	chunks := (payloadBytes + fpgaChunkBytes - 1) / fpgaChunkBytes
	wire := chunks*5 + (payloadBytes*8+6)/7 // framing + encoded payload
	seconds := float64(wire*10) / 57600.0

	if seconds > 6 {
		t.Errorf("a download would take %.1fs on the wire; the tool description promises ~5", seconds)
	}
	t.Logf("%d chunks, %d bytes on the wire, %.2fs at 57600 baud", chunks, wire, seconds)
}
