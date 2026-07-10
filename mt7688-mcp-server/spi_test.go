// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateSPITransfer_InvalidMode(t *testing.T) {
	if err := validateSPITransfer(4, 1000000, 8, []byte{0x01}); err == nil {
		t.Fatal("expected error for mode > 3, got nil")
	}
	if err := validateSPITransfer(-1, 1000000, 8, []byte{0x01}); err == nil {
		t.Fatal("expected error for negative mode, got nil")
	}
}

func TestValidateSPITransfer_InvalidBitsPerWord(t *testing.T) {
	if err := validateSPITransfer(0, 1000000, 0, []byte{0x01}); err == nil {
		t.Fatal("expected error for bits_per_word 0, got nil")
	}
	if err := validateSPITransfer(0, 1000000, 33, []byte{0x01}); err == nil {
		t.Fatal("expected error for bits_per_word > 32, got nil")
	}
}

func TestValidateSPITransfer_InvalidSpeed(t *testing.T) {
	if err := validateSPITransfer(0, 0, 8, []byte{0x01}); err == nil {
		t.Fatal("expected error for zero speed_hz, got nil")
	}
	if err := validateSPITransfer(0, -1, 8, []byte{0x01}); err == nil {
		t.Fatal("expected error for negative speed_hz, got nil")
	}
}

func TestValidateSPITransfer_InvalidDataLength(t *testing.T) {
	if err := validateSPITransfer(0, 1000000, 8, []byte{}); err == nil {
		t.Fatal("expected error for empty tx_data, got nil")
	}
	big := make([]byte, 257)
	if err := validateSPITransfer(0, 1000000, 8, big); err == nil {
		t.Fatal("expected error for tx_data length > 256, got nil")
	}
}

func TestValidateSPITransfer_Valid(t *testing.T) {
	if err := validateSPITransfer(0, 1000000, 8, []byte{0x01, 0x02}); err != nil {
		t.Fatalf("expected nil for valid input, got %v", err)
	}
}

func TestDiscoverSPIDevice_None(t *testing.T) {
	dir := t.TempDir()
	old := spiDevGlob
	spiDevGlob = dir + "/spidev*"
	defer func() { spiDevGlob = old }()

	if _, err := discoverSPIDevice(); err == nil {
		t.Fatal("expected error when no device nodes exist, got nil")
	}
}

func TestDiscoverSPIDevice_ExactlyOne(t *testing.T) {
	dir := t.TempDir()
	// filepath.Join (not raw "/" concatenation) so the expected path uses
	// the same separator style filepath.Glob normalizes its matches to -
	// on Windows, t.TempDir() returns a backslash-separated path, and
	// filepath.Glob rebuilds matches via filepath.Join internally, so a
	// hardcoded "/" here would falsely mismatch on that platform alone.
	path := filepath.Join(dir, "spidev32766.1")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	old := spiDevGlob
	spiDevGlob = filepath.Join(dir, "spidev*")
	defer func() { spiDevGlob = old }()

	got, err := discoverSPIDevice()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got != path {
		t.Fatalf("expected %q, got %q", path, got)
	}
}

func TestDiscoverSPIDevice_Multiple(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"spidev0.0", "spidev0.1"} {
		if err := os.WriteFile(dir+"/"+name, nil, 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	old := spiDevGlob
	spiDevGlob = dir + "/spidev*"
	defer func() { spiDevGlob = old }()

	if _, err := discoverSPIDevice(); err == nil {
		t.Fatal("expected error for multiple device nodes, got nil")
	}
}

func TestMipsIOC_WRMode(t *testing.T) {
	// Hand-derived: _IOW('k', 1, 1) on MIPS.
	// MIPS _IOC layout: WRITE=4, DIRSHIFT=29, TYPESHIFT=8, SIZESHIFT=16.
	// (4<<29) | (0x6b<<8) | (1<<0) | (1<<16)
	// = 0x80000000 | 0x00006B00 | 0x00000001 | 0x00010000 = 0x80016B01
	got := mipsIOC(mipsIOCWrite, spiIOCMagic, 1, 1)
	want := uintptr(0x80016B01)
	if got != want {
		t.Fatalf("SPI_IOC_WR_MODE: got %#x, want %#x", got, want)
	}
	if spiIOCWRMode != want {
		t.Fatalf("spiIOCWRMode: got %#x, want %#x", spiIOCWRMode, want)
	}
}

func TestMipsIOC_Message1(t *testing.T) {
	// Hand-derived: _IOW('k', 0, char[32]) on MIPS.
	// (4<<29) | (0x6b<<8) | (0<<0) | (32<<16)
	// = 0x80000000 | 0x00006B00 | 0 | 0x00200000 = 0x80206B00
	got := mipsIOC(mipsIOCWrite, spiIOCMagic, 0, spiIOCTransferSize)
	want := uintptr(0x80206B00)
	if got != want {
		t.Fatalf("SPI_IOC_MESSAGE(1): got %#x, want %#x", got, want)
	}
	if spiIOCMessage1 != want {
		t.Fatalf("spiIOCMessage1: got %#x, want %#x", spiIOCMessage1, want)
	}
}
