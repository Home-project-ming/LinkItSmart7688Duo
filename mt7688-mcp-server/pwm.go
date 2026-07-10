// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

const pwmSysfsPath = "/sys/class/pwm/pwmchip0"

// validatePWMChannel restricts operations to the two PWM channels
// confirmed (2026-07-11, see design doc) to be routed to a known pin
// (channel 0 = GPIO18, channel 1 = GPIO19). pwmchip0 reports npwm=4, but
// channels 2/3 have no verified pin mapping in mt7688_pinmux - refusing
// them is the same "don't guess" posture applied to i2c_write against an
// unknown device address.
func validatePWMChannel(channel int) error {
	if channel != 0 && channel != 1 {
		return fmt.Errorf("pwm channel %d not verified routed to a known pin (only 0 and 1 are supported)", channel)
	}
	return nil
}

// exportPWM exports channel via the sysfs PWM ABI. Idempotent under
// concurrent invocation, same EBUSY-tolerant pattern as exportGPIO in
// gpio.go and for the same reason: each MCP client session is a separate
// OS process and two sessions can race to export the same channel.
func exportPWM(channel int) error {
	f, err := os.OpenFile(pwmSysfsPath+"/export", os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open export: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(strconv.Itoa(channel)); err != nil {
		if errors.Is(err, syscall.EBUSY) {
			return nil // already exported by a concurrent session
		}
		return fmt.Errorf("write export: %w", err)
	}
	return nil
}

func pwmChannelPath(channel int) string {
	return fmt.Sprintf("%s/pwm%d", pwmSysfsPath, channel)
}

func readPWMAttr(channel int, attr string) (string, error) {
	path := pwmChannelPath(channel) + "/" + attr
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", attr, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func writePWMAttr(channel int, attr string, value string) error {
	path := pwmChannelPath(channel) + "/" + attr
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", attr, err)
	}
	defer f.Close()
	if _, err := f.WriteString(value); err != nil {
		return fmt.Errorf("write %s: %w", attr, err)
	}
	return nil
}

// ConfigurePWM sets channel's period and duty cycle (both in
// nanoseconds). The sysfs PWM ABI requires duty_cycle <= period at all
// times - writing duty_cycle first when it wouldn't fit under the
// *current* period fails. This reads the current period first and picks
// a write order that never violates the constraint mid-update (see
// design doc, "PWM write ordering").
func ConfigurePWM(channel int, periodNs int, dutyNs int) error {
	if err := validatePWMChannel(channel); err != nil {
		return err
	}
	if periodNs <= 0 {
		return fmt.Errorf("period_ns must be positive, got %d", periodNs)
	}
	if dutyNs < 0 || dutyNs > periodNs {
		return fmt.Errorf("duty_ns must be 0-period_ns (%d), got %d", periodNs, dutyNs)
	}
	if err := exportPWM(channel); err != nil {
		return err
	}

	currentPeriodStr, err := readPWMAttr(channel, "period")
	if err != nil {
		return err
	}
	currentPeriod, err := strconv.Atoi(currentPeriodStr)
	if err != nil {
		return fmt.Errorf("parse current period %q: %w", currentPeriodStr, err)
	}

	if periodNs >= currentPeriod {
		if err := writePWMAttr(channel, "period", strconv.Itoa(periodNs)); err != nil {
			return err
		}
		if err := writePWMAttr(channel, "duty_cycle", strconv.Itoa(dutyNs)); err != nil {
			return err
		}
	} else {
		if err := writePWMAttr(channel, "duty_cycle", strconv.Itoa(dutyNs)); err != nil {
			return err
		}
		if err := writePWMAttr(channel, "period", strconv.Itoa(periodNs)); err != nil {
			return err
		}
	}
	return nil
}

func SetPWMEnabled(channel int, enabled bool) error {
	if err := validatePWMChannel(channel); err != nil {
		return err
	}
	if err := exportPWM(channel); err != nil {
		return err
	}
	v := "0"
	if enabled {
		v = "1"
	}
	return writePWMAttr(channel, "enable", v)
}

type PWMState struct {
	PeriodNs int
	DutyNs   int
	Enabled  bool
	Polarity string
}

func ReadPWM(channel int) (PWMState, error) {
	if err := validatePWMChannel(channel); err != nil {
		return PWMState{}, err
	}
	if err := exportPWM(channel); err != nil {
		return PWMState{}, err
	}
	periodStr, err := readPWMAttr(channel, "period")
	if err != nil {
		return PWMState{}, err
	}
	period, err := strconv.Atoi(periodStr)
	if err != nil {
		return PWMState{}, fmt.Errorf("parse period %q: %w", periodStr, err)
	}
	dutyStr, err := readPWMAttr(channel, "duty_cycle")
	if err != nil {
		return PWMState{}, err
	}
	duty, err := strconv.Atoi(dutyStr)
	if err != nil {
		return PWMState{}, fmt.Errorf("parse duty_cycle %q: %w", dutyStr, err)
	}
	enabledStr, err := readPWMAttr(channel, "enable")
	if err != nil {
		return PWMState{}, err
	}
	polarity, err := readPWMAttr(channel, "polarity")
	if err != nil {
		return PWMState{}, err
	}
	return PWMState{
		PeriodNs: period,
		DutyNs:   duty,
		Enabled:  enabledStr == "1",
		Polarity: polarity,
	}, nil
}
