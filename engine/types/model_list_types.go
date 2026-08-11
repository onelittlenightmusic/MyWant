package types

import (
	"context"
	"os/exec"
	"strings"
	"time"

	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[ModelListWant, ModelListLocals]("model_list")
	})
}

// probeTimeout bounds one candidate probe. A probe never reaches the network
// (see probeClaude), so this only has to cover CLI startup.
const probeTimeout = 20 * time.Second

// defaultClaudeCandidates are the names probed when `candidates` is left empty.
// The probe decides which of them this installation actually recognizes, so
// listing a model that does not exist here costs nothing but a dropped entry.
var defaultClaudeCandidates = []string{
	"fable", "opus", "sonnet", "haiku",
	"claude-fable-5", "claude-opus-5", "claude-sonnet-5", "claude-haiku-4-5",
	"claude-opus-4-8", "claude-opus-4-7", "claude-opus-4-6", "claude-sonnet-4-6",
}

// defaultGeminiCandidates are the gemini equivalents. Unlike the claude list
// these are not probe-verified — see probeGemini.
var defaultGeminiCandidates = []string{
	"gemini-2.5-pro", "gemini-2.5-flash",
}

// ModelListLocals holds type-specific local state for the model_list want.
type ModelListLocals struct {
	Provider    string `mywant:"internal,provider"`
	GlobalParam string `mywant:"internal,global_param"`
	NextCheckAt int64  `mywant:"internal,next_check_at"`
}

// ModelListWant discovers which models the installed provider CLI recognizes
// and publishes them as a global parameter, so a coding want can name one via
// `model: {fromGlobalParam: ...}` (or the operator can just read the list).
//
// Discovery goes through the CLI itself rather than the provider's HTTP API:
// the CLI is what will actually run the request, so what it recognizes is the
// only answer that matters, and it needs no separate credentials.
type ModelListWant struct{ Want }

func (m *ModelListWant) GetLocals() *ModelListLocals {
	return CheckLocalsInitialized[ModelListLocals](&m.Want)
}

func (m *ModelListWant) Initialize() {
	locals := m.GetLocals()
	locals.Provider = m.GetStringParam("provider", "claude_code")
	locals.GlobalParam = m.GetStringParam("global_param", "available_models")
	locals.NextCheckAt = 0

	m.SetGoal("provider", locals.Provider)
	m.SetGoal("global_param", locals.GlobalParam)
	m.SetCurrent("models", "")
	m.SetCurrent("model_count", 0)
	m.SetCurrent("checked_at", "")
}

// IsAchieved always returns false — the recognized set changes whenever the CLI
// is upgraded, so the want stays live and re-checks on its interval.
func (m *ModelListWant) IsAchieved() bool { return false }

func (m *ModelListWant) Progress() {
	locals := m.GetLocals()

	interval := int64(m.GetIntParam("refresh_interval_seconds", 3600))
	now := time.Now().Unix()
	if locals.NextCheckAt != 0 && now < locals.NextCheckAt {
		return
	}
	locals.NextCheckAt = now + interval

	candidates := splitCandidates(m.GetStringParam("candidates", ""))
	if len(candidates) == 0 {
		if locals.Provider == "gemini" {
			candidates = defaultGeminiCandidates
		} else {
			candidates = defaultClaudeCandidates
		}
	}

	var recognized []string
	for _, c := range candidates {
		ok, err := probeModel(locals.Provider, c)
		if err != nil {
			// The CLI is missing or unrunnable — say so once and leave the
			// previously published list alone rather than blanking it.
			m.StoreLog("[MODEL_LIST] probe failed for %q: %v", c, err)
			m.SetCurrent("last_error", err.Error())
			return
		}
		if ok {
			recognized = append(recognized, c)
		}
	}

	m.SetCurrent("last_error", "")
	m.SetCurrent("models", strings.Join(recognized, ","))
	m.SetCurrent("model_count", len(recognized))
	m.SetCurrent("checked_at", time.Now().Format(time.RFC3339))

	if locals.GlobalParam != "" {
		if err := SetGlobalParameter(locals.GlobalParam, recognized); err != nil {
			m.StoreLog("[MODEL_LIST] failed to publish %s: %v", locals.GlobalParam, err)
		} else {
			publishModelsParamDef(locals.GlobalParam)
		}
	}

	m.StoreLog("[MODEL_LIST] %s recognizes %d of %d candidates: %s",
		locals.Provider, len(recognized), len(candidates), strings.Join(recognized, ", "))
}

// splitCandidates accepts either a comma- or whitespace-separated list, since
// the same string is typed by hand into a parameter field.
func splitCandidates(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// probeModel reports whether the provider CLI recognizes the model name.
func probeModel(provider, model string) (bool, error) {
	if provider == "gemini" {
		return probeGemini(model)
	}
	return probeClaude(model)
}

// probeClaude asks the claude CLI whether it recognizes a model, without
// spending a request.
//
// `claude --print --model <m> ""` fails on the empty prompt before any API call
// is made, so the run is local and free. The model check happens first, which is
// what makes the two outcomes distinguishable:
//
//	recognized   -> "Error: Input must be provided either through stdin or ..."
//	unrecognized -> "<m>" is not a model this version of Claude Code recognizes, ...
//
// Both exit 0, so the answer is in the output, not the status code.
func probeClaude(model string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", "--print", "--model", model, "")
	cmd.Env = sanitizedSubprocessEnv()
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	// A non-zero exit is expected here (the empty prompt is refused); only a
	// failure to *run* the CLI at all is an error worth reporting.
	if err != nil && len(out) == 0 {
		return false, err
	}
	return !strings.Contains(string(out), "is not a model this version of Claude Code recognizes"), nil
}

// probeGemini checks that the gemini CLI exists and takes the candidate at face
// value. Unlike claude, gemini has no output that separates "unknown model"
// from "prompt refused" without spending a request, so an unrecognized name
// here surfaces later as a failed coding request rather than being filtered out
// now. Narrow the `candidates` parameter if that matters.
func probeGemini(model string) (bool, error) {
	if _, err := exec.LookPath("gemini"); err != nil {
		return false, err
	}
	return model != "", nil
}

// publishModelsParamDef records the global parameter's type alongside its value,
// so the list reads as models everywhere rather than as bare strings. Defs are
// stored as one flat list, hence the read-modify-write.
func publishModelsParamDef(name string) {
	defs := GetAllGlobalParamDefs()
	for i, d := range defs {
		if d.Name == name {
			if d.Type == "array" && d.SubType == "models" {
				return
			}
			defs[i].Type = "array"
			defs[i].SubType = "models"
			_ = SetGlobalParamDefs(defs)
			return
		}
	}
	defs = append(defs, ParameterDef{
		Name:        name,
		Type:        "array",
		SubType:     "models",
		Description: "Models the installed provider CLI recognizes (published by a model_list want)",
	})
	_ = SetGlobalParamDefs(defs)
}
