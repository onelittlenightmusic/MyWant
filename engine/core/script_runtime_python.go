package mywant

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// pythonRuntime executes inline scripts via python3.
type pythonRuntime struct{}

func (p *pythonRuntime) ExecuteThink(want *Want, script string) error {
	return p.run(want, script, "PYTHON-THINK", func(out *scriptOutput) {
		applyScriptOutput(want, out, "PYTHON-THINK")
	})
}

func (p *pythonRuntime) ExecuteDo(want *Want, script string) error {
	return p.run(want, script, "PYTHON-DO", func(out *scriptOutput) {
		applyScriptOutput(want, out, "PYTHON-DO")
	})
}

func (p *pythonRuntime) ExecuteMonitor(want *Want, script string) (bool, error) {
	var shouldStop bool
	err := p.run(want, script, "PYTHON-MONITOR", func(out *scriptOutput) {
		applyScriptOutput(want, out, "PYTHON-MONITOR")
		shouldStop = out.ShouldStop
	})
	return shouldStop, err
}

func (p *pythonRuntime) run(want *Want, script, label string, apply func(*scriptOutput)) error {
	tmpDir, err := os.MkdirTemp("", "python-runtime-*")
	if err != nil {
		return scriptErr(want, label, fmt.Sprintf("failed to create temp dir: %v", err))
	}
	defer os.RemoveAll(tmpDir)

	goalPath, currentPath, planPath, inputPath, err := writeStateFiles(want, tmpDir)
	if err != nil {
		return scriptErr(want, label, err.Error())
	}

	// Write inline script to a temp file.
	scriptPath := filepath.Join(tmpDir, "script.py")
	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		return scriptErr(want, label, fmt.Sprintf("failed to write script: %v", err))
	}

	pythonCmd := GetCurrent(want, "python_command", "python3")
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, pythonCmd, scriptPath)
	cmd.Env = append(os.Environ(), stateEnv(goalPath, currentPath, planPath, inputPath)...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	want.DirectLog("[%s] executing python script", label)
	if err := cmd.Run(); err != nil {
		return scriptErr(want, label, fmt.Sprintf("script failed: %v\nstderr:\n%s", err, stderr.String()))
	}

	out, err := parseScriptOutput(stdout.Bytes())
	if err != nil {
		return scriptErr(want, label, err.Error())
	}

	apply(out)
	return nil
}
