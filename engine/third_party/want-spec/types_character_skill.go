package want_spec

// CharacterSkill is something a character can do on the board — one of the
// skills the Y button's slot equips with its digits (投げる, 接続, 型, …). It
// belongs to whatever gives the board that skill (a GUI extension), which puts
// it to the backend; the backend keeps it and says what each is — what it is
// called, which digit equips it, and what it does. Not a Machine-Readable
// Skill (MRSAgentDef), which is what an agent runs.
//
//	apiVersion: mywant.io/v1
//	kind: CharacterSkill
//	metadata:
//	  name: throw
//	spec:
//	  label: 投げる
//	  digit: 1
//	  description: …
//
// See spec/character-skill-spec.yaml.
type CharacterSkill struct {
	APIVersion string                 `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                 `yaml:"kind" json:"kind"`
	Metadata   CharacterSkillMetadata `yaml:"metadata" json:"metadata"`
	Spec       CharacterSkillSpec     `yaml:"spec" json:"spec"`
}

// CharacterSkillMetadata names a character skill.
type CharacterSkillMetadata struct {
	// Name is its id: what equips it and what it is kept by (throw, connect, …).
	Name   string            `yaml:"name" json:"name"`
	Labels map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
}

// CharacterSkillSpec is what a character skill is.
type CharacterSkillSpec struct {
	// Label is what it is called on the board, by the badge at the character's shoulder.
	Label string `yaml:"label" json:"label"`
	// Digit is the key that equips it — its rung on the slot's ladder.
	Digit int `yaml:"digit" json:"digit"`
	// Description is what it does, for a person — and what its picture is drawn from.
	Description string `yaml:"description" json:"description"`
}

// CharacterSkillKind is a CharacterSkill document's kind.
const CharacterSkillKind = "CharacterSkill"
