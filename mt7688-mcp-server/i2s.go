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
