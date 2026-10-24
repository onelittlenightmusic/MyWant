package mywant

import "testing"

func TestImpactValueText(t *testing.T) {
	cases := []struct {
		in   any
		want string
		ok   bool
	}{
		{"新宿", "新宿", true},
		{17.5, "17.5", true},
		{true, "true", true},
		{"", "", false},
		{nil, "", false},
		{map[string]any{"lat": 1}, "", false},
		{"とても長い駅名がたくさん並んでいる文字列でございますよね", "とても長い駅名がたくさん並んでいる文字列でござ…", true},
	}
	for _, c := range cases {
		got, ok := impactValueText(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("impactValueText(%v) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestGlobalParamReaders(t *testing.T) {
	reader := &Want{Metadata: Metadata{Name: "impact-test-reader"}, Spec: WantSpec{Params: map[string]any{"from": map[string]any{"fromGlobalParam": "impact_test.key"}}}}
	other := &Want{Metadata: Metadata{Name: "impact-test-other"}, Spec: WantSpec{Params: map[string]any{"from": "x"}}}
	wantRegistryMutex.Lock()
	wantRegistry[reader.Metadata.Name] = reader
	wantRegistry[other.Metadata.Name] = other
	wantRegistryMutex.Unlock()
	defer func() {
		wantRegistryMutex.Lock()
		delete(wantRegistry, reader.Metadata.Name)
		delete(wantRegistry, other.Metadata.Name)
		wantRegistryMutex.Unlock()
	}()
	got := globalParamReaders("impact_test.key")
	if len(got) != 1 || got[0] != "impact-test-reader" {
		t.Fatalf("globalParamReaders = %v; want [impact-test-reader]", got)
	}
}
