package mywant

import "fmt"

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
	r := inlineRun{Want: want, Argv: []string{"/bin/bash", "-c", script}}
	return runPrintingScript(r, label, "shell script", apply)
}

func scriptErr(want *Want, label, msg string) error {
	err := fmt.Errorf("[%s] %s", label, msg)
	want.StoreLog("%v", err)
	want.SetStatus(WantStatusModuleError)
	return err
}
