package main

import "testing"

func TestReadI2C_InvalidAddress(t *testing.T) {
	_, err := ReadI2C(0x80, 0x00, 1)
	if err == nil {
		t.Fatal("expected error for out-of-range address, got nil")
	}
}

func TestReadI2C_InvalidLength(t *testing.T) {
	if _, err := ReadI2C(0x50, 0x00, 0); err == nil {
		t.Fatal("expected error for zero length, got nil")
	}
	if _, err := ReadI2C(0x50, 0x00, 300); err == nil {
		t.Fatal("expected error for length > 256, got nil")
	}
}

func TestWriteI2C_InvalidAddress(t *testing.T) {
	err := WriteI2C(0x80, 0x00, []byte{0x01})
	if err == nil {
		t.Fatal("expected error for out-of-range address, got nil")
	}
}

func TestWriteI2C_EmptyData(t *testing.T) {
	err := WriteI2C(0x50, 0x00, []byte{})
	if err == nil {
		t.Fatal("expected error for empty data, got nil")
	}
}
