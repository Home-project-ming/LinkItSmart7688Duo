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

func TestSPITransferHandler_DataElementOutOfRange(t *testing.T) {
	_, _, err := spiTransferHandler(context.Background(), nil, SPITransferInput{
		Mode: 0, SpeedHz: 1000000, BitsPerWord: 8, TxData: []int{1, 300, 2},
	})
	if err == nil {
		t.Fatal("expected error for tx_data element > 255, got nil")
	}
}

func TestSPITransferHandler_DataElementNegative(t *testing.T) {
	_, _, err := spiTransferHandler(context.Background(), nil, SPITransferInput{
		Mode: 0, SpeedHz: 1000000, BitsPerWord: 8, TxData: []int{-1},
	})
	if err == nil {
		t.Fatal("expected error for negative tx_data element, got nil")
	}
}

func TestUartTransactHandler_DataElementOutOfRange(t *testing.T) {
	_, _, err := uartTransactHandler(context.Background(), nil, UARTTransactInput{
		BaudRate: 9600, TxData: []int{1, 300, 2}, ReadTimeoutMs: 100, MaxReadBytes: 256,
	})
	if err == nil {
		t.Fatal("expected error for tx_data element > 255, got nil")
	}
}

func TestUartTransactHandler_DataElementNegative(t *testing.T) {
	_, _, err := uartTransactHandler(context.Background(), nil, UARTTransactInput{
		BaudRate: 9600, TxData: []int{-1}, ReadTimeoutMs: 100, MaxReadBytes: 256,
	})
	if err == nil {
		t.Fatal("expected error for negative tx_data element, got nil")
	}
}
