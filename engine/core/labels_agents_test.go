package mywant

import (
	"os"
	"path/filepath"
	"testing"
)

// Capabilities and agents carry labels from their YAML (checked against
// want-spec's schema, which allows them), as MRS plugin agents do from their
// metadata.
func TestAgentAndCapabilityLabels(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("capability-labelled.yaml", `capabilities:
  - name: labelled_cap
    gives: [labelled_give]
    labels: {team: robot}
`)
	write("agent-labelled.yaml", `agents:
  - name: labelled_agent
    type: do
    capabilities: [labelled_cap]
    labels: {team: robot, tier: gold}
`)
	r := NewAgentRegistry()
	if err := r.LoadCapabilities(dir); err != nil {
		t.Fatal(err)
	}
	if err := r.LoadAgents(dir); err != nil {
		t.Fatal(err)
	}
	if c, ok := r.GetCapability("labelled_cap"); !ok || c.Labels["team"] != "robot" {
		t.Errorf("capability labels: %+v", c)
	}
	a, ok := r.GetAgent("labelled_agent")
	if !ok || a.GetLabels()["tier"] != "gold" {
		t.Fatalf("agent labels: %v", a)
	}

	script := filepath.Join(dir, "skill.py")
	write("skill.py", "print('{}')\n")
	if err := r.RegisterMRSAgentFromYAML([]byte(`agent:
  metadata:
    name: labelled_mrs
    capability: labelled_mrs_cap
    type: do
    labels: {source: plugin}
  script:
    path: `+script+`
`), filepath.Join(dir, "agent.yaml")); err != nil {
		t.Fatal(err)
	}
	if m, ok := r.GetAgent("labelled_mrs"); !ok || m.GetLabels()["source"] != "plugin" {
		t.Errorf("MRS agent labels: %v", m)
	}
}
