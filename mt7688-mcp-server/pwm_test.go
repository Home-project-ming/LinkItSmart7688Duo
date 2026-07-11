// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import "testing"

func TestConfigurePWM_InvalidChannel(t *testing.T) {
	if err := ConfigurePWM(2, 1000, 500); err == nil {
		t.Fatal("expected error for unverified channel 2, got nil")
	}
}

func TestConfigurePWM_InvalidPeriod(t *testing.T) {
	// channel 0 is valid, so these must fail on period/duty validation,
	// not channel validation - proving validation order and staying
	// testable off real hardware (same pattern as gpio_test.go's
	// TestSetGPIODirection_InvalidDirection).
	if err := ConfigurePWM(0, 0, 0); err == nil {
		t.Fatal("expected error for zero period, got nil")
	}
	if err := ConfigurePWM(0, -1, 0); err == nil {
		t.Fatal("expected error for negative period, got nil")
	}
}

func TestConfigurePWM_DutyExceedsPeriod(t *testing.T) {
	if err := ConfigurePWM(0, 1000, 1500); err == nil {
		t.Fatal("expected error for duty_ns > period_ns, got nil")
	}
}

func TestConfigurePWM_NegativeDuty(t *testing.T) {
	if err := ConfigurePWM(0, 1000, -1); err == nil {
		t.Fatal("expected error for negative duty_ns, got nil")
	}
}

func TestSetPWMEnabled_InvalidChannel(t *testing.T) {
	if err := SetPWMEnabled(3, true); err == nil {
		t.Fatal("expected error for unverified channel 3, got nil")
	}
}

func TestReadPWM_InvalidChannel(t *testing.T) {
	if _, err := ReadPWM(2); err == nil {
		t.Fatal("expected error for unverified channel 2, got nil")
	}
}

func TestPwmWritePeriodFirst(t *testing.T) {
	cases := []struct {
		currentPeriod, newPeriod int
		want                     bool
	}{
		{1000, 1500, true}, // increasing period: period-first
		{1000, 500, false}, // decreasing period: duty-first
		{1000, 1000, true}, // unchanged period: period-first (boundary, either is safe, period-first is the >= branch)
		{0, 1, true},       // fresh export (period=0): always period-first
	}
	for _, c := range cases {
		got := pwmWritePeriodFirst(c.currentPeriod, c.newPeriod)
		if got != c.want {
			t.Errorf("pwmWritePeriodFirst(%d, %d) = %v, want %v", c.currentPeriod, c.newPeriod, got, c.want)
		}
	}
}
