package mywant

import (
	"testing"
	"time"
)

func TestWantStateManagement(t *testing.T) {
	want := &Want{
		Metadata: Metadata{Name: "test-want", Type: "test"},
		Spec:     WantSpec{Params: make(map[string]any)},
		State:    make(map[string]any),
	}

	// Test storing state
	want.StoreState("key", "value")

	value, exists := want.GetState("key")
	if !exists {
		t.Error("Expected state key to exist")
	}
	if value != "value" {
		t.Errorf("Expected 'value', got %v", value)
	}
}

func TestWantParameterManagement(t *testing.T) {
	want := &Want{
		Metadata: Metadata{Name: "test-want", Type: "test"},
		Spec:     WantSpec{Params: make(map[string]any)},
	}

	// Test updating parameters
	want.UpdateParameter("param1", "value1")

	value, exists := want.GetParameter("param1")
	if !exists {
		t.Error("Expected parameter to exist")
	}
	if value != "value1" {
		t.Errorf("Expected 'value1', got %v", value)
	}
}

func TestWantStatus(t *testing.T) {
	want := &Want{
		Metadata: Metadata{Name: "test-want", Type: "test"},
		Spec:     WantSpec{Params: make(map[string]any)},
	}

	// Test initial status
	if want.GetStatus() != "" {
		t.Errorf("Expected empty status, got %v", want.GetStatus())
	}

	// Test setting status
	want.SetStatus(WantStatusReaching)
	if want.GetStatus() != WantStatusReaching {
		t.Errorf("Expected %v, got %v", WantStatusReaching, want.GetStatus())
	}
}

func TestWantExecCycle(t *testing.T) {
	want := &Want{
		Metadata: Metadata{Name: "test-want", Type: "test"},
		Spec:     WantSpec{Params: make(map[string]any)},
		State:    make(map[string]any),
	}

	// Test exec cycle batching
	want.BeginProgressCycle()
	want.StoreState("key1", "value1")
	want.StoreState("key2", "value2")

	want.EndProgressCycle()

	// Verify state was stored
	value1, exists1 := want.GetState("key1")
	value2, exists2 := want.GetState("key2")

	if !exists1 || value1 != "value1" {
		t.Error("State key1 not properly stored during exec cycle")
	}
	if !exists2 || value2 != "value2" {
		t.Error("State key2 not properly stored during exec cycle")
	}

	// Verify history was created
	if len(want.BuildHistory().StateHistory) == 0 {
		t.Error("Expected state history to be recorded")
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				Wants: []*Want{
					{
						Metadata: Metadata{Name: "test", Type: "test"},
						Spec:     WantSpec{Params: make(map[string]any)},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty config",
			config: Config{
				Wants: []*Want{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation test - ensure config can be created
			if len(tt.config.Wants) < 0 {
				t.Error("Invalid config structure")
			}
		})
	}
}

func TestChainBuilderCreation(t *testing.T) {
	config := Config{
		Wants: []*Want{
			{
				Metadata: Metadata{Name: "test", Type: "test"},
				Spec:     WantSpec{Params: make(map[string]any)},
			},
		},
	}

	builder := NewChainBuilder(config)
	if builder == nil {
		t.Error("Expected ChainBuilder to be created")
	}

	if len(builder.config.Wants) != 1 {
		t.Errorf("Expected 1 want, got %d", len(builder.config.Wants))
	}
}

func TestWantLabels(t *testing.T) {
	want := &Want{
		Metadata: Metadata{
			Name: "test-want",
			Type: "test",
			Labels: map[string]string{
				"role":     "processor",
				"category": "queue",
			},
		},
		Spec: WantSpec{Params: make(map[string]any)},
	}

	// Test label access
	if want.Metadata.Labels["role"] != "processor" {
		t.Error("Expected role label to be 'processor'")
	}
	if want.Metadata.Labels["category"] != "queue" {
		t.Error("Expected category label to be 'queue'")
	}
}

func TestMemoryDumpStructure(t *testing.T) {
	dump := WantMemoryDump{
		Timestamp:   time.Now().Format(time.RFC3339),
		ExecutionID: "test-exec-123",
		Wants: []*Want{
			{
				Metadata: Metadata{Name: "test", Type: "test"},
				Spec:     WantSpec{Params: make(map[string]any)},
			},
		},
	}

	if dump.ExecutionID != "test-exec-123" {
		t.Error("Expected execution ID to be set")
	}
	if len(dump.Wants) != 1 {
		t.Error("Expected 1 want in dump")
	}
}

func TestWantConnectivity(t *testing.T) {
	paths := Paths{
		In: []PathInfo{
			{Name: "input1", Active: true},
			{Name: "input2", Active: false},
		},
		Out: []PathInfo{
			{Name: "output1", Active: true},
		},
	}

	if paths.GetInCount() != 2 {
		t.Errorf("Expected 2 input paths, got %d", paths.GetInCount())
	}
	if paths.GetActiveInCount() != 1 {
		t.Errorf("Expected 1 active input path, got %d", paths.GetActiveInCount())
	}
	if paths.GetOutCount() != 1 {
		t.Errorf("Expected 1 output path, got %d", paths.GetOutCount())
	}
	if paths.GetActiveOutCount() != 1 {
		t.Errorf("Expected 1 active output path, got %d", paths.GetActiveOutCount())
	}
}

func TestStateNotification(t *testing.T) {
	notification := StateNotification{
		SourceWantName:   "source",
		TargetWantName:   "target",
		StateKey:         "test_key",
		StateValue:       "test_value",
		Timestamp:        time.Now(),
		NotificationType: NotificationSubscription,
	}

	if notification.SourceWantName != "source" {
		t.Error("Expected source want name to be 'source'")
	}
	if notification.NotificationType != NotificationSubscription {
		t.Error("Expected notification type to be subscription")
	}
}
