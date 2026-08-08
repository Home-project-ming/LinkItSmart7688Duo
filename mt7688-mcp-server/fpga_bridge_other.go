// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

//go:build !linux

package main

import "fmt"

// openAtmegaStream on non-linux build hosts. Same rationale as
// atmega_other.go: this stub only exists so `go build`/`go test` work on
// a dev machine, since the real target is always GOOS=linux
// GOARCH=mipsle.
func openAtmegaStream() (atmegaStream, error) {
	return nil, fmt.Errorf("atmega bridge is only supported on linux (target device); this build is not linux")
}
