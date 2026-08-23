package types

import (
	"encoding/json"
	"testing"

	. "mywant/engine/core"

	"github.com/stretchr/testify/assert"
)

// The want's own name/ID are not otherwise visible to an MRS script, so they are
// exposed as %{want_name} / %{want_id} placeholders. Scripts that call back into
// the API (OAuth state, state PUTs) need them to identify the want they run for.
func TestMRSRebuildSkillArg_WantIdentityPlaceholders(t *testing.T) {
	want := &Want{
		Metadata: Metadata{ID: "want-abc123", Name: "spotify-instance", Type: "spotify"},
	}
	want.StateLabels = map[string]StateLabel{
		"skill_json_arg_template": LabelCurrent,
		"skill_json_arg":          LabelCurrent,
	}

	want.BeginProgressCycle()
	want.SetCurrent("skill_json_arg_template", `{"name":"%{want_name}","id":"%{want_id}"}`)
	want.EndProgressCycle()

	want.BeginProgressCycle()
	MRSRebuildSkillArg(want)
	want.EndProgressCycle()

	assert.Equal(t, `{"name":"spotify-instance","id":"want-abc123"}`,
		GetCurrent(want, "skill_json_arg", ""))
}

// A param or state field of the same name is more specific than the metadata
// fallback, so it must win.
func TestMRSRebuildSkillArg_ParamOverridesWantName(t *testing.T) {
	want := &Want{
		Metadata: Metadata{ID: "want-abc123", Name: "spotify-instance", Type: "spotify"},
		Spec:     WantSpec{Params: map[string]any{"want_name": "explicit-override"}},
	}
	want.StateLabels = map[string]StateLabel{
		"skill_json_arg_template": LabelCurrent,
		"skill_json_arg":          LabelCurrent,
	}

	want.BeginProgressCycle()
	want.SetCurrent("skill_json_arg_template", `{"name":"%{want_name}"}`)
	want.EndProgressCycle()

	want.BeginProgressCycle()
	MRSRebuildSkillArg(want)
	want.EndProgressCycle()

	assert.Equal(t, `{"name":"explicit-override"}`, GetCurrent(want, "skill_json_arg", ""))
}

// A value carrying a double quote used to be pasted into the template raw,
// producing an argument the skill could not parse. That is worse than one bad
// tick: a skill given unparseable JSON answers with nothing, so the state
// holding the quote survived, and every later tick rebuilt the same broken
// argument. The want went quiet permanently and no restart cleared it, because
// the offending value was persisted state — deleting the want was the only way
// out. It was reached by a script reporting an error that named its device in
// quotes, into a template ending "last_error":"%{error}".
func TestMRSRebuildSkillArg_ValueWithQuotesStaysParseable(t *testing.T) {
	want := &Want{
		Metadata: Metadata{ID: "want-reg", Name: "registry-terminal", Type: "rpg_generator"},
	}
	want.StateLabels = map[string]StateLabel{
		"skill_json_arg_template": LabelCurrent,
		"skill_json_arg":          LabelCurrent,
		"error":                   LabelCurrent,
	}

	want.BeginProgressCycle()
	want.SetCurrent("skill_json_arg_template", `{"last_error":"%{error}"}`)
	want.SetCurrent("error", `unknown device "registry_terminal" in "fortress1"`)
	want.EndProgressCycle()

	want.BeginProgressCycle()
	MRSRebuildSkillArg(want)
	want.EndProgressCycle()

	built := GetCurrent(want, "skill_json_arg", "")
	var decoded map[string]string
	assert.NoError(t, json.Unmarshal([]byte(built), &decoded), "built arg must parse: %s", built)
	assert.Equal(t, `unknown device "registry_terminal" in "fortress1"`, decoded["last_error"],
		"the script must still receive the text it sent")
}

// Escaping must not disturb the ordinary case: an id or a number is pasted
// through unchanged, including where the template uses it outside a string.
func TestMRSRebuildSkillArg_PlainValuesUnchanged(t *testing.T) {
	want := &Want{
		Metadata: Metadata{ID: "want-reg", Name: "registry-terminal", Type: "rpg_generator"},
		Spec:     WantSpec{Params: map[string]any{"stage_id": "fortress1", "level": 5}},
	}
	want.StateLabels = map[string]StateLabel{
		"skill_json_arg_template": LabelCurrent,
		"skill_json_arg":          LabelCurrent,
	}

	want.BeginProgressCycle()
	want.SetCurrent("skill_json_arg_template", `{"stage_id":"%{stage_id}","level":%{level}}`)
	want.EndProgressCycle()

	want.BeginProgressCycle()
	MRSRebuildSkillArg(want)
	want.EndProgressCycle()

	assert.Equal(t, `{"stage_id":"fortress1","level":5}`, GetCurrent(want, "skill_json_arg", ""))
}
