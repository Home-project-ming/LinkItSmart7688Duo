// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import "testing"

func TestValidateUARTTransact_InvalidBaudRate(t *testing.T) {
	if err := validateUARTTransact(0, []byte{0x01}, 100, 256); err == nil {
		t.Fatal("expected error for zero baud_rate, got nil")
	}
	if err := validateUARTTransact(-1, []byte{0x01}, 100, 256); err == nil {
		t.Fatal("expected error for negative baud_rate, got nil")
	}
	if err := validateUARTTransact(2500001, []byte{0x01}, 100, 256); err == nil {
		t.Fatal("expected error for baud_rate above uartBaseBaud, got nil")
	}
}

func TestValidateUARTTransact_InvalidDataLength(t *testing.T) {
	big := make([]byte, 257)
	if err := validateUARTTransact(9600, big, 100, 256); err == nil {
		t.Fatal("expected error for tx_data length > 256, got nil")
	}
}

func TestValidateUARTTransact_EmptyDataIsValid(t *testing.T) {
	if err := validateUARTTransact(9600, []byte{}, 100, 256); err != nil {
		t.Fatalf("expected nil for empty tx_data (read-only call), got %v", err)
	}
}

func TestValidateUARTTransact_InvalidTimeout(t *testing.T) {
	if err := validateUARTTransact(9600, []byte{0x01}, -1, 256); err == nil {
		t.Fatal("expected error for negative read_timeout_ms, got nil")
	}
	if err := validateUARTTransact(9600, []byte{0x01}, 25501, 256); err == nil {
		t.Fatal("expected error for read_timeout_ms > 25500, got nil")
	}
}

func TestValidateUARTTransact_ZeroTimeoutIsValid(t *testing.T) {
	if err := validateUARTTransact(9600, []byte{0x01}, 0, 256); err != nil {
		t.Fatalf("expected nil for zero read_timeout_ms (non-blocking), got %v", err)
	}
}

func TestValidateUARTTransact_InvalidMaxReadBytes(t *testing.T) {
	if err := validateUARTTransact(9600, []byte{0x01}, 100, 0); err == nil {
		t.Fatal("expected error for zero max_read_bytes, got nil")
	}
	if err := validateUARTTransact(9600, []byte{0x01}, 100, 4097); err == nil {
		t.Fatal("expected error for max_read_bytes > 4096, got nil")
	}
}

func TestValidateUARTTransact_Valid(t *testing.T) {
	if err := validateUARTTransact(115200, []byte{0x41, 0x54, 0x0D}, 500, 256); err != nil {
		t.Fatalf("expected nil for valid input, got %v", err)
	}
}
