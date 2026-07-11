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
