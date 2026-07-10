package main

import "testing"

func TestCheckBlocked_BlockedPins(t *testing.T) {
	blocked := []int{12, 13, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 38}
	for _, pin := range blocked {
		if err := CheckBlocked(pin); err == nil {
			t.Errorf("pin %d: expected error, got nil", pin)
		}
	}
}

func TestCheckBlocked_AllowedPins(t *testing.T) {
	allowed := []int{0, 1, 18, 19, 30, 39, 95}
	for _, pin := range allowed {
		if err := CheckBlocked(pin); err != nil {
			t.Errorf("pin %d: expected nil, got %v", pin, err)
		}
	}
}
