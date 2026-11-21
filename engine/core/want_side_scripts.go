package mywant

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// A type's side scripts: small functions beside its YAML, each for one part
// of how its wants talk, apart from its Progress — run by the engine with
// what it needs and nothing more, as its voice script is (want_voice.go).
//
//	<type>.advice.py   advice on a new answer (want_advice.go)
//	<type>.hear.py     what the want proposes on a murmur it heard (want_hearer.go)
//
// A custom package may name them plainly (advice.py, hear.py) in its own
// directory. A Go type registers a function instead (RegisterAdvisor,
// RegisterHearer); a type may have both, the function first.
const (
	SideScriptAdvice = "advice"
	SideScriptHear   = "hear"
)

var sideScriptKinds = []string{SideScriptAdvice, SideScriptHear}

// sideScriptTimeout bounds one run.
const sideScriptTimeout = 5 * time.Second

func sideScriptOnDisk(yamlPath, kind string, allowPlain bool) string {
	named := strings.TrimSuffix(yamlPath, filepath.Ext(yamlPath)) + "." + kind + ".py"
	if fileExists(named) {
		return realScriptPath(named)
	}
	if allowPlain {
		plain := filepath.Join(filepath.Dir(yamlPath), kind+".py")
		if fileExists(plain) {
			return realScriptPath(plain)
		}
	}
	return ""
}

func sideScriptEmbedded(fsys fs.FS, yamlPath, kind string) string {
	named := strings.TrimSuffix(yamlPath, filepath.Ext(yamlPath)) + "." + kind + ".py"
	data, err := fs.ReadFile(fsys, named)
	if err != nil {
		return ""
	}
	return cacheVoiceScript(filepath.Base(named), data)
}

func sideScriptBundled(yamlPath, kind string) string {
	named := sideScriptOnDisk(yamlPath, kind, false)
	if named == "" {
		return ""
	}
	data, err := os.ReadFile(named)
	if err != nil {
		return ""
	}
	return cacheVoiceScript(filepath.Base(named), data)
}

// setSideScripts records a type's side scripts as found by find (one kind at
// a time). Must be called with mu held.
func (w *WantTypeLoader) setSideScripts(name string, find func(kind string) string) {
	for _, kind := range sideScriptKinds {
		if w.sideScripts[kind] == nil {
			w.sideScripts[kind] = map[string]string{}
		}
		script := ""
		if find != nil {
			script = find(kind)
		}
		if script == "" {
			delete(w.sideScripts[kind], name)
		} else {
			w.sideScripts[kind][name] = script
		}
	}
}

// GetSideScript is the path of a type's side script of a kind, or "".
func (w *WantTypeLoader) GetSideScript(kind, name string) string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.sideScripts[kind][name]
}

// runSideScript is how a side script is run — a variable so tests need no
// python3. The input is handed as its first argument, as a voice script's is.
var runSideScript = func(ctx context.Context, script string, input []byte) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, sideScriptTimeout)
	defer cancel()
	return RunMRSScript(ctx, script, MRSRunOptions{Args: []string{string(input)}, Profile: MRSProfileVoice})
}
