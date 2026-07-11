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
