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
			value := int(buf[i+1]&0x7F) | (int(buf[i+2]&0x7F) << 7)
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
