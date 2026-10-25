package mywant

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// The Python SDK (client-py/, the `mywant` package), for plugin scripts.
//
// A plugin script is called for one want at a time, and the SDK is how it
// knows which: `mywant.current()` is that want — its id, name, params,
// labels, its state as it is now (imports included) — with a client already
// pointed at this server. What the script says about its OWN want — stop
// itself, move its own tile — comes back as a directive line ("_control",
// "_tile"), and the agent applies it to that want and no other.
//
// The package is embedded here (pysdk/, a copy of client-py/mywant kept in
// step by `make pysdk`; TestPySDKMatchesClientPy says when it is not) and
// written out once to a directory every interpreter the engine starts has
// on its PYTHONPATH.

//go:embed pysdk/mywant/*.py
var pySDK embed.FS

// PluginServerURL is this server, for the SDK a plugin script is handed —
// set by the server as it starts. Scripts run beside it, so its own address.
var PluginServerURL = ""

var (
	pySDKOnce sync.Once
	pySDKDir  string
)

// pySDKPath writes the SDK out (once per version) and is the directory that
// holds the `mywant` package; "" when it cannot be written.
func pySDKPath() string {
	pySDKOnce.Do(func() {
		h := sha256.New()
		_ = fs.WalkDir(pySDK, "pysdk", func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				b, _ := pySDK.ReadFile(p)
				h.Write([]byte(p))
				h.Write(b)
			}
			return nil
		})
		dir := filepath.Join(os.TempDir(), "mywant-pysdk-"+hex.EncodeToString(h.Sum(nil)[:6]))
		if _, err := os.Stat(filepath.Join(dir, "mywant", "__init__.py")); err == nil {
			pySDKDir = dir
			return
		}
		tmp := dir + fmt.Sprintf(".%d.tmp", os.Getpid())
		if err := os.MkdirAll(filepath.Join(tmp, "mywant"), 0o755); err != nil {
			return
		}
		err := fs.WalkDir(pySDK, "pysdk", func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, rerr := pySDK.ReadFile(p)
			if rerr != nil {
				return rerr
			}
			return os.WriteFile(filepath.Join(tmp, strings.TrimPrefix(p, "pysdk/")), b, 0o644)
		})
		if err != nil {
			_ = os.RemoveAll(tmp)
			return
		}
		if err := os.Rename(tmp, dir); err != nil {
			_ = os.RemoveAll(tmp) // another process wrote it first
		}
		pySDKDir = dir
	})
	return pySDKDir
}

// mrsPythonEnv is env with the SDK on PYTHONPATH (ahead of what was there).
func mrsPythonEnv(env []string) []string {
	dir := pySDKPath()
	if dir == "" {
		return env
	}
	out := make([]string, 0, len(env)+1)
	had := false
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "PYTHONPATH="); ok {
			had = true
			if v != "" {
				kv = "PYTHONPATH=" + dir + string(os.PathListSeparator) + v
			} else {
				kv = "PYTHONPATH=" + dir
			}
		}
		out = append(out, kv)
	}
	if !had {
		out = append(out, "PYTHONPATH="+dir)
	}
	return out
}

// mrsContextEnv is the want a call runs for, as the environment a spawned
// script's SDK reads (mywant/plugin.py: _from_env).
func mrsContextEnv(c map[string]any) []string {
	if c == nil {
		return nil
	}
	str := func(k string) string { s, _ := c[k].(string); return s }
	js := func(k string) string {
		b, err := json.Marshal(c[k])
		if err != nil || c[k] == nil {
			return "{}"
		}
		return string(b)
	}
	env := []string{
		"MYWANT_WANT_ID=" + str("want_id"),
		"MYWANT_WANT_NAME=" + str("want_name"),
		"MYWANT_WANT_TYPE=" + str("want_type"),
		"MYWANT_WANT_STATE=" + js("state"),
		"MYWANT_WANT_PARAMS=" + js("params"),
		"MYWANT_WANT_LABELS=" + js("labels"),
	}
	if srv, ok := c["server"].(map[string]any); ok {
		if u, _ := srv["url"].(string); u != "" {
			env = append(env, "MYWANT_URL="+u)
		}
	}
	return env
}

// MRSWantContext is the want a plugin call runs for, as its SDK is handed
// it: who it is, its params, labels, and its state now — imported values
// resolved — and this server.
func MRSWantContext(want *Want) map[string]any {
	params := map[string]any{}
	for k := range want.GetSpec().Params {
		if v, ok := want.GetParameter(k); ok {
			params[k] = v
		}
	}
	md := want.GetMetadata()
	return map[string]any{
		"want_id":   md.ID,
		"want_name": md.Name,
		"want_type": md.Type,
		"state":     want.GetAllState(),
		"params":    params,
		"labels":    md.Labels,
		"server":    map[string]any{"url": PluginServerURL},
	}
}

// mrsDirective hands a directive line to opt.OnDirective and says whether
// obj was one (so never progress, never the result).
func mrsDirective(obj map[string]any, opt MRSRunOptions) bool {
	_, control := obj["_control"]
	_, tile := obj["_tile"]
	if !control && !tile {
		return false
	}
	if opt.OnDirective != nil {
		opt.OnDirective(obj)
	}
	return true
}

// MRSApplyDirective does what a plugin script asked of its own want: stop
// itself ("_control": "suspend"), or move its own tile ("_tile": {x, y}).
// Only ever that want — nothing in the line names another.
func MRSApplyDirective(want *Want, obj map[string]any) {
	if c, _ := obj["_control"].(string); c != "" {
		trigger := map[string]ControlTrigger{
			"suspend": ControlTriggerSuspend,
			"resume":  ControlTriggerResume,
			"restart": ControlTriggerRestart,
			"stop":    ControlTriggerStop,
		}[c]
		if trigger == "" {
			want.StoreLog("[MRS] unknown _control %q; ignored", c)
		} else {
			reason, _ := obj["_reason"].(string)
			_ = want.SendControlCommand(&ControlCommand{
				Trigger: trigger, WantID: want.Metadata.ID, Timestamp: time.Now(), Reason: reason,
			})
		}
	}
	if t, ok := obj["_tile"].(map[string]any); ok {
		cb := GetGlobalChainBuilder()
		x, okX := t["x"].(float64)
		y, okY := t["y"].(float64)
		if cb != nil && okX && okY {
			_ = cb.QueueWantAddLabel(want.Metadata.ID, "mywant.io/canvas-x", fmt.Sprintf("%d", int(x)))
			_ = cb.QueueWantAddLabel(want.Metadata.ID, "mywant.io/canvas-y", fmt.Sprintf("%d", int(y)))
		}
	}
}
