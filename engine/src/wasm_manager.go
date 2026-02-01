package mywant

import (
	"context"
	"fmt"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// WasmManager handles loading and executing WASM modules for dynamic logic
type WasmManager struct {
	runtime wazero.Runtime
}

// NewWasmManager creates a new WasmManager instance
func NewWasmManager(ctx context.Context) *WasmManager {
	r := wazero.NewRuntime(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	return &WasmManager{runtime: r}
}

// ExecuteFunction executes a function in a WASM module
// Function signature expected: (ptr uint32, size uint32) -> uint64 (packed ptr and size)
func (m *WasmManager) ExecuteFunction(ctx context.Context, wasmPath string, funcName string, input []byte) ([]byte, error) {
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read WASM file: %w", err)
	}

	mod, err := m.runtime.Instantiate(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate WASM module: %w", err)
	}
	defer mod.Close(ctx)

	fn := mod.ExportedFunction(funcName)
	if fn == nil {
		return nil, fmt.Errorf("function %s not found in WASM module", funcName)
	}

	inputSize := uint64(len(input))
	malloc := mod.ExportedFunction("__guest_malloc")
	if malloc == nil {
		malloc = mod.ExportedFunction("malloc")
	}
	if malloc == nil {
		malloc = mod.ExportedFunction("allocate")
	}
	if malloc == nil {
		malloc = mod.ExportedFunction("__mywant_allocate")
	}
	if malloc == nil {
		return nil, fmt.Errorf("allocation function (__guest_malloc, malloc, allocate or __mywant_allocate) not found in WASM module")
	}

	results, err := malloc.Call(ctx, inputSize)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate memory in WASM: %w", err)
	}
	inputPtr := uint32(results[0])

	if !mod.Memory().Write(inputPtr, input) {
		return nil, fmt.Errorf("failed to write input to WASM memory")
	}

	packedResult, err := fn.Call(ctx, uint64(inputPtr), inputSize)
	if err != nil {
		return nil, fmt.Errorf("failed to execute WASM function %s: %w", funcName, err)
	}

	resultPtr := uint32(packedResult[0] >> 32)
	resultSize := uint32(packedResult[0])

	output, ok := mod.Memory().Read(resultPtr, resultSize)
	if !ok {
		return nil, fmt.Errorf("failed to read output from WASM memory")
	}

	// Copy output bytes
	result := make([]byte, len(output))
	copy(result, output)

	return result, nil
}

// Close closes the WASM runtime
func (m *WasmManager) Close(ctx context.Context) error {
	return m.runtime.Close(ctx)
}
