// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

//go:build !linux

package main

import "fmt"

// transferSPI on non-linux build hosts (e.g. Windows, where this module
// is developed and cross-compiled from). This stub exists solely so
// `go build`/`go test` succeed on a non-linux dev machine; the real
// target is always GOOS=linux GOARCH=mipsle (see build.ps1), where
// spi_linux.go is compiled in instead of this file. Matches i2c_other.go's
// existing rationale exactly.
func transferSPI(device string, mode int, speedHz int, bitsPerWord int, txData []byte) ([]byte, error) {
	return nil, fmt.Errorf("spi is only supported on linux (target device); this build is not linux")
}
