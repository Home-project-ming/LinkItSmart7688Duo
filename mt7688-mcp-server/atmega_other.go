// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

//go:build !linux

package main

import "fmt"

// atmegaADCRead/atmegaBridgeRaw on non-linux build hosts. This stub
// exists solely so `go build`/`go test` succeed on a non-linux dev
// machine; the real target is always GOOS=linux GOARCH=mipsle (see
// build.ps1), where atmega_linux.go is compiled in instead of this file.
// Matches uart_other.go's/i2c_other.go's/spi_other.go's existing
// rationale exactly.
func atmegaADCRead(channel byte, timeoutMs int) (int, error) {
	return 0, fmt.Errorf("atmega bridge is only supported on linux (target device); this build is not linux")
}

func atmegaBridgeRaw(txData []byte, timeoutMs int, maxReadBytes int) ([]byte, error) {
	return nil, fmt.Errorf("atmega bridge is only supported on linux (target device); this build is not linux")
}
