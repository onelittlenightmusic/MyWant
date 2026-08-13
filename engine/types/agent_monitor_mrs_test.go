package types

import (
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
