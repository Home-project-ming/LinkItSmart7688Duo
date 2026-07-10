package main

import "testing"

func TestSetGPIODirection_BlockedPin(t *testing.T) {
	err := SetGPIODirection(21, "out")
	if err == nil {
		t.Fatal("expected error for blocked pin 21, got nil")
	}
}

func TestReadGPIO_BlockedPin(t *testing.T) {
	_, err := ReadGPIO(38)
	if err == nil {
		t.Fatal("expected error for blocked pin 38, got nil")
	}
}

func TestWriteGPIO_BlockedPin(t *testing.T) {
	err := WriteGPIO(25, true)
	if err == nil {
		t.Fatal("expected error for blocked pin 25, got nil")
	}
}

func TestSetGPIODirection_InvalidDirection(t *testing.T) {
	// pin 0 is not blocked, so this must fail on direction validation,
	// not the blocklist - proving validation order and staying testable
	// off real hardware.
	err := SetGPIODirection(0, "sideways")
	if err == nil {
		t.Fatal("expected error for invalid direction, got nil")
	}
}
