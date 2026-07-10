// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import (
	"context"
	"testing"
)

func TestI2CWriteHandler_DataElementOutOfRange(t *testing.T) {
	_, _, err := i2cWriteHandler(context.Background(), nil, I2CWriteInput{
		Addr: 0x50, Reg: 0, Data: []int{1, 300, 2},
	})
	if err == nil {
		t.Fatal("expected error for data element > 255, got nil")
	}
}

func TestI2CWriteHandler_DataElementNegative(t *testing.T) {
	_, _, err := i2cWriteHandler(context.Background(), nil, I2CWriteInput{
		Addr: 0x50, Reg: 0, Data: []int{-1},
	})
	if err == nil {
		t.Fatal("expected error for negative data element, got nil")
	}
}
