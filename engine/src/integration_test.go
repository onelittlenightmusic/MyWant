package mywant

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigLoadingFromYAML(t *testing.T) {
	// Test loading valid config
	config, err := loadConfigFromYAML("testdata/valid-config.yaml")
	if err != nil {
		t.Fatalf("Failed to load valid config: %v", err)
	}

	if len(config.Wants) != 3 {
		t.Errorf("Expected 3 wants, got %d", len(config.Wants))
	}

	// Verify first want
	firstWant := config.Wants[0]
	if firstWant.Metadata.Name != "test-sequence" {
		t.Error("First want name incorrect")
	}
	if firstWant.Metadata.Type != "sequence" {
		t.Error("First want type incorrect")
	}

	// Test loading invalid config
	_, err = loadConfigFromYAML("testdata/invalid-config.yaml")
	if err == nil {
		t.Error("Expected error loading invalid config")
	}
}

func TestChainBuilderWithRealConfig(t *testing.T) {
	config, err := loadConfigFromYAML("testdata/valid-config.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	builder := NewChainBuilder(config)
	if builder == nil {
		t.Fatal("Failed to create chain builder")
	}

	// Test that builder stores config correctly
	if len(builder.config.Wants) != 3 {
		t.Error("Builder config not properly stored")
	}

	// Test path generation
	paths := builder.generatePathsFromConnections()
	if len(paths) == 0 {
		t.Error("Expected paths to be generated")
	}

	// Verify connection logic
	sinkPaths, exists := paths["test-sink"]
	if !exists {
		t.Error("Expected sink paths to exist")
	}
	if len(sinkPaths.In) == 0 {
		t.Error("Expected sink to have input connections")
	}
}

func TestMemoryReconciliation(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()
	memoryPath := filepath.Join(tempDir, "test-memory.yaml")

	config := Config{
		Wants: []*Want{
			{
				Metadata: Metadata{Name: "test", Type: "test"},
				Spec:     WantSpec{Params: make(map[string]interface{})},
				State:    map[string]interface{}{"test_key": "test_value"},
			},
		},
	}

	builder := NewChainBuilderWithPaths("", memoryPath)
	builder.config = config

	// Test memory copy
	err := builder.copyConfigToMemory()
	if err != nil {
		t.Fatalf("Failed to copy config to memory: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(memoryPath); os.IsNotExist(err) {
		t.Error("Memory file was not created")
	}

	// Test loading from memory
	loadedConfig, err := builder.loadMemoryConfig()
	if err != nil {
		t.Fatalf("Failed to load memory config: %v", err)
	}

	if len(loadedConfig.Wants) != 1 {
		t.Error("Memory config not properly loaded")
	}
}

func TestDynamicWantAddition(t *testing.T) {
	config := Config{Wants: []*Want{}}
	builder := NewChainBuilder(config)

	// Add dynamic want
	dynamicWant := &Want{
		Metadata: Metadata{Name: "dynamic", Type: "test"},
		Spec:     WantSpec{Params: make(map[string]interface{})},
	}

	builder.AddDynamicWants([]*Want{dynamicWant})

	// Verify want was added
	if len(builder.config.Wants) != 1 {
		t.Error("Dynamic want not added to config")
	}

	if builder.config.Wants[0].Metadata.Name != "dynamic" {
		t.Error("Dynamic want not properly stored")
	}
}

func TestStateNotificationSystem(t *testing.T) {
	// Test notification creation
	notification := StateNotification{
		SourceWantName:   "source",
		TargetWantName:   "target",
		StateKey:         "test_key",
		StateValue:       "test_value",
		Timestamp:        time.Now(),
		NotificationType: NotificationSubscription,
	}

	// Verify notification structure
	if notification.SourceWantName == "" {
		t.Error("Source want name not set")
	}
	if notification.NotificationType != NotificationSubscription {
		t.Error("Notification type not correct")
	}

	// Test subscription structure
	subscription := StateSubscription{
		WantName:  "test-want",
		StateKeys: []string{"key1", "key2"},
	}

	if len(subscription.StateKeys) != 2 {
		t.Error("Subscription state keys not properly set")
	}
}

func TestConnectivityValidation(t *testing.T) {
	builder := NewChainBuilder(Config{Wants: []*Want{}})

	// Create test paths
	pathMap := map[string]Paths{
		"want1": {
			In:  []PathInfo{{Name: "input", Active: true}},
			Out: []PathInfo{{Name: "output", Active: true}},
		},
	}

	// Test validation with valid connections
	err := builder.validateConnections(pathMap)
	if err != nil {
		t.Errorf("Validation failed for valid connections: %v", err)
	}
}

func TestChangeDetection(t *testing.T) {
	builder := NewChainBuilder(Config{Wants: []*Want{}})

	oldConfig := Config{
		Wants: []*Want{
			{
				Metadata: Metadata{Name: "old", Type: "test"},
				Spec:     WantSpec{Params: make(map[string]interface{})},
			},
		},
	}

	newConfig := Config{
		Wants: []*Want{
			{
				Metadata: Metadata{Name: "old", Type: "test"},
				Spec:     WantSpec{Params: map[string]interface{}{"new": "param"}},
			},
			{
				Metadata: Metadata{Name: "new", Type: "test"},
				Spec:     WantSpec{Params: make(map[string]interface{})},
			},
		},
	}

	changes := builder.detectConfigChanges(oldConfig, newConfig)

	// Should detect one update and one addition
	updateCount := 0
	addCount := 0

	for _, change := range changes {
		switch change.Type {
		case ChangeEventUpdate:
			updateCount++
		case ChangeEventAdd:
			addCount++
		}
	}

	if updateCount != 1 {
		t.Errorf("Expected 1 update, got %d", updateCount)
	}
	if addCount != 1 {
		t.Errorf("Expected 1 addition, got %d", addCount)
	}
}

func TestWantEquality(t *testing.T) {
	builder := NewChainBuilder(Config{Wants: []*Want{}})

	want1 := &Want{
		Metadata: Metadata{Name: "test", Type: "test"},
		Spec:     WantSpec{Params: map[string]interface{}{"key": "value"}},
	}

	want2 := &Want{
		Metadata: Metadata{Name: "test", Type: "test"},
		Spec:     WantSpec{Params: map[string]interface{}{"key": "value"}},
	}

	want3 := &Want{
		Metadata: Metadata{Name: "test", Type: "test"},
		Spec:     WantSpec{Params: map[string]interface{}{"key": "different"}},
	}

	// Test equal wants
	if !builder.wantsEqual(want1, want2) {
		t.Error("Equal wants not detected as equal")
	}

	// Test different wants
	if builder.wantsEqual(want1, want3) {
		t.Error("Different wants detected as equal")
	}
}

func TestFileHashCalculation(t *testing.T) {
	builder := NewChainBuilder(Config{Wants: []*Want{}})

	// Create temporary file
	tempFile := filepath.Join(t.TempDir(), "test.txt")
	err := os.WriteFile(tempFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	hash1, err := builder.calculateFileHash(tempFile)
	if err != nil {
		t.Fatalf("Failed to calculate hash: %v", err)
	}

	hash2, err := builder.calculateFileHash(tempFile)
	if err != nil {
		t.Fatalf("Failed to calculate hash second time: %v", err)
	}

	// Same file should produce same hash
	if hash1 != hash2 {
		t.Error("File hash not consistent")
	}

	// Different content should produce different hash
	err = os.WriteFile(tempFile, []byte("different content"), 0644)
	if err != nil {
		t.Fatalf("Failed to update test file: %v", err)
	}

	hash3, err := builder.calculateFileHash(tempFile)
	if err != nil {
		t.Fatalf("Failed to calculate hash for modified file: %v", err)
	}

	if hash1 == hash3 {
		t.Error("Different file content produced same hash")
	}
}