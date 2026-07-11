// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"time"
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
