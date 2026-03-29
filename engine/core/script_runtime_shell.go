package mywant

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
)

// shellRuntime executes inline scripts via /bin/bash.
type shellRuntime struct{}

func (s *shellRuntime) ExecuteThink(want *Want, script string) error {
	return s.run(want, script, "SHELL-THINK", func(out *scriptOutput) {
		applyScriptOutput(want, out, "SHELL-THINK")
	})
}

func (s *shellRuntime) ExecuteDo(want *Want, script string) error {
	return s.run(want, script, "SHELL-DO", func(out *scriptOutput) {
		applyScriptOutput(want, out, "SHELL-DO")
	})
}

func (s *shellRuntime) ExecuteMonitor(want *Want, script string) (bool, error) {
	var shouldStop bool
	err := s.run(want, script, "SHELL-MONITOR", func(out *scriptOutput) {
		applyScriptOutput(want, out, "SHELL-MONITOR")
		shouldStop = out.ShouldStop
	})
	return shouldStop, err
}

func (s *shellRuntime) run(want *Want, script, label string, apply func(*scriptOutput)) error {
	tmpDir, err := os.MkdirTemp("", "shell-runtime-*")
	if err != nil {
		return scriptErr(want, label, fmt.Sprintf("failed to create temp dir: %v", err))
	}
	defer os.RemoveAll(tmpDir)

	goalPath, currentPath, planPath, inputPath, err := writeStateFiles(want, tmpDir)
	if err != nil {
		return scriptErr(want, label, err.Error())
	}

	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "/bin/bash", "-c", script)
	cmd.Env = append(os.Environ(), stateEnv(goalPath, currentPath, planPath, inputPath)...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	want.DirectLog("[%s] executing shell script", label)
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

func scriptErr(want *Want, label, msg string) error {
	err := fmt.Errorf("[%s] %s", label, msg)
	want.DirectLog("%v", err)
	want.SetStatus(WantStatusModuleError)
	return err
}
