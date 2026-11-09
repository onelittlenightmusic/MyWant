package mywant

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

// Labels are written while the want runs and read by everyone else at the
// same time (a handler's SetCurrent, the GUI's state): run with -race.
func TestStateLabelSetConcurrent(t *testing.T) {
	var s StateLabelSet
	s.Set("phase", LabelCurrent)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(2)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				s.Set(fmt.Sprintf("k%d_%d", g, i), LabelInternal)
			}
		}(g)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				if !s.Is("phase", LabelCurrent) {
					t.Error("phase lost its label")
					return
				}
				s.Range(func(string, StateLabel) bool { return true })
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.Replace(map[string]StateLabel{"phase": LabelCurrent}) // a type reloaded
	}()
	wg.Wait()
	if !s.Is("phase", LabelCurrent) {
		t.Errorf("phase = %v", s.Map())
	}
}

// Written as the map it was: state.yaml and the API read the same.
func TestStateLabelSetSerializedAsBefore(t *testing.T) {
	w := &Want{Metadata: Metadata{Name: "w"}}
	j, _ := json.Marshal(w)
	y, _ := yaml.Marshal(w)
	if strings.Contains(string(j), "state_labels") {
		t.Errorf("no labels, none written: %s", j)
	}
	if strings.Contains(string(y), "state_labels") {
		t.Errorf("no labels, none written: %s", y)
	}

	w.StateLabels.Set("phase", LabelCurrent)
	w.StateLabels.Set("budget", LabelGoal)
	j, _ = json.Marshal(w)
	if !strings.Contains(string(j), `"state_labels":{"budget":1,"phase":2}`) {
		t.Errorf("json = %s", j)
	}
	y, _ = yaml.Marshal(w)
	if !strings.Contains(string(y), "state_labels:\n    budget: 1\n    phase: 2") {
		t.Errorf("yaml = %s", y)
	}

	var back Want
	if err := yaml.Unmarshal(y, &back); err != nil {
		t.Fatal(err)
	}
	if !back.StateLabels.Is("phase", LabelCurrent) || !back.StateLabels.Is("budget", LabelGoal) {
		t.Errorf("read back from yaml = %v", back.StateLabels.Map())
	}
	var backJ Want
	if err := json.Unmarshal(j, &backJ); err != nil {
		t.Fatal(err)
	}
	if backJ.StateLabels.Len() != 2 {
		t.Errorf("read back from json = %v", backJ.StateLabels.Map())
	}
}
