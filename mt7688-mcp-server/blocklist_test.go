// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import (
	"strings"
	"testing"
)

func TestCheckBlocked_BlockedPins(t *testing.T) {
	blocked := []int{0, 1, 2, 3, 6, 7, 8, 9, 10, 12, 13, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 38, 45, 46}
	for _, pin := range blocked {
		if err := CheckBlocked(pin); err == nil {
			t.Errorf("pin %d: expected error, got nil", pin)
		}
	}
}

// The reason strings are user-facing documentation, and these two pins
// historically had confused roles: the "ATmega ISP reset" label sat on
// pin 38 for months while the real reset line (per the board's live
// /etc/avrdude.conf and the vendor pin-out) is pin 3. Pin down the
// corrected wording so the mix-up cannot silently come back.
func TestCheckBlocked_ResetPinReasons(t *testing.T) {
	err3 := CheckBlocked(3)
	if err3 == nil || !strings.Contains(err3.Error(), "ATmega ISP reset") {
		t.Errorf("pin 3 reason must mention its ATmega ISP reset role, got %v", err3)
	}
	err38 := CheckBlocked(38)
	if err38 == nil || strings.Contains(err38.Error(), "ATmega") {
		t.Errorf("pin 38 reason must not claim an ATmega role (it is the Wi-Fi reset button), got %v", err38)
	}
}

func TestCheckBlocked_AllowedPins(t *testing.T) {
	allowed := []int{30, 39, 95}
	for _, pin := range allowed {
		if err := CheckBlocked(pin); err != nil {
			t.Errorf("pin %d: expected nil, got %v", pin, err)
		}
	}
}
