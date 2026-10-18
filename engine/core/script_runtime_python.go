package mywant

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
	r := inlineRun{
		Want:  want,
		Argv:  []string{GetCurrent(want, "python_command", "python3"), "script.py"},
		Files: map[string][]byte{"script.py": []byte(script)},
	}
	return runPrintingScript(r, label, "python script", apply)
}
