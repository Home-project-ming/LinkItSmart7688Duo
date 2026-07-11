// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

//go:build !linux

package main

import "fmt"

// uartTransact on non-linux build hosts (e.g. Windows, where this module
// is developed and cross-compiled from). This stub exists solely so
// `go build`/`go test` succeed on a non-linux dev machine; the real
// target is always GOOS=linux GOARCH=mipsle (see build.ps1), where
// uart_linux.go is compiled in instead of this file. Matches
// i2c_other.go's/spi_other.go's existing rationale exactly.
func uartTransact(baudRate int, txData []byte, readTimeoutMs int, maxReadBytes int) ([]byte, error) {
	return nil, fmt.Errorf("uart is only supported on linux (target device); this build is not linux")
}
