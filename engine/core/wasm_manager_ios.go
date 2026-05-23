//go:build ios

package mywant

import (
	"context"
	"fmt"
)

// WasmManager is a stub on iOS — WASM execution is not supported.
type WasmManager struct{}

func NewWasmManager(_ context.Context) *WasmManager {
	return &WasmManager{}
}

func (m *WasmManager) ExecuteFunction(_ context.Context, _ string, _ string, _ []byte) ([]byte, error) {
	return nil, fmt.Errorf("WASM execution is not supported on iOS")
}

func (m *WasmManager) Close(_ context.Context) error { return nil }
