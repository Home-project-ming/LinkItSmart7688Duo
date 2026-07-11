// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import "testing"

func TestValidateAtmegaADCRead_InvalidChannel(t *testing.T) {
	if err := validateAtmegaADCRead(-1, 1000); err == nil {
		t.Fatal("expected error for negative channel, got nil")
	}
	if err := validateAtmegaADCRead(12, 1000); err == nil {
		t.Fatal("expected error for channel >= 12, got nil")
	}
}

func TestValidateAtmegaADCRead_ValidChannelBoundaries(t *testing.T) {
	if err := validateAtmegaADCRead(0, 1000); err != nil {
		t.Fatalf("expected nil for channel 0, got %v", err)
	}
	if err := validateAtmegaADCRead(11, 1000); err != nil {
		t.Fatalf("expected nil for channel 11, got %v", err)
	}
}

func TestValidateAtmegaADCRead_InvalidTimeout(t *testing.T) {
	if err := validateAtmegaADCRead(0, 0); err == nil {
		t.Fatal("expected error for zero timeout_ms, got nil")
	}
	if err := validateAtmegaADCRead(0, -1); err == nil {
		t.Fatal("expected error for negative timeout_ms, got nil")
	}
	if err := validateAtmegaADCRead(0, 10001); err == nil {
		t.Fatal("expected error for timeout_ms > 10000, got nil")
	}
}

func TestValidateAtmegaADCRead_Valid(t *testing.T) {
	if err := validateAtmegaADCRead(5, 500); err != nil {
		t.Fatalf("expected nil for valid input, got %v", err)
	}
}

func TestValidateAtmegaBridgeRaw_InvalidDataLength(t *testing.T) {
	big := make([]byte, 257)
	if err := validateAtmegaBridgeRaw(big, 100, 256); err == nil {
		t.Fatal("expected error for tx_data length > 256, got nil")
	}
}

func TestValidateAtmegaBridgeRaw_EmptyDataIsValid(t *testing.T) {
	if err := validateAtmegaBridgeRaw([]byte{}, 100, 256); err != nil {
		t.Fatalf("expected nil for empty tx_data, got %v", err)
	}
}

func TestValidateAtmegaBridgeRaw_InvalidTimeout(t *testing.T) {
	if err := validateAtmegaBridgeRaw([]byte{0x01}, -1, 256); err == nil {
		t.Fatal("expected error for negative timeout_ms, got nil")
	}
	if err := validateAtmegaBridgeRaw([]byte{0x01}, 25501, 256); err == nil {
		t.Fatal("expected error for timeout_ms > 25500, got nil")
	}
}

func TestValidateAtmegaBridgeRaw_ZeroTimeoutIsValid(t *testing.T) {
	if err := validateAtmegaBridgeRaw([]byte{0x01}, 0, 256); err != nil {
		t.Fatalf("expected nil for zero timeout_ms, got %v", err)
	}
}

func TestValidateAtmegaBridgeRaw_InvalidMaxReadBytes(t *testing.T) {
	if err := validateAtmegaBridgeRaw([]byte{0x01}, 100, 0); err == nil {
		t.Fatal("expected error for zero max_read_bytes, got nil")
	}
	if err := validateAtmegaBridgeRaw([]byte{0x01}, 100, 4097); err == nil {
		t.Fatal("expected error for max_read_bytes > 4096, got nil")
	}
}

func TestValidateAtmegaBridgeRaw_Valid(t *testing.T) {
	if err := validateAtmegaBridgeRaw([]byte{0x41}, 500, 256); err != nil {
		t.Fatalf("expected nil for valid input, got %v", err)
	}
}
