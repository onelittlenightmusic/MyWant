package pubsub

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FilePersister provides file-based persistence for message caches.
// Each topic is persisted as a separate JSON file.
type FilePersister struct {
	baseDir string
	mu      sync.Mutex
}

// NewFilePersister creates a new file-based persister.
// Creates the base directory if it doesn't exist.
func NewFilePersister(baseDir string) (*FilePersister, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create persistence directory %s: %w", baseDir, err)
	}

	log.Printf("[FilePersister] Initialized with base directory: %s", baseDir)
	return &FilePersister{
		baseDir: baseDir,
	}, nil
}

// filePath returns the file path for a topic.
func (fp *FilePersister) filePath(topic string) string {
	// Sanitize topic name to make valid filename
	sanitized := sanitizeTopicName(topic)
	return filepath.Join(fp.baseDir, sanitized+".json")
}

// sanitizeTopicName converts a topic name to a valid filename.
func sanitizeTopicName(topic string) string {
	// Simple sanitization: replace unsafe chars with underscore
	result := ""
	for _, ch := range topic {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-' {
			result += string(ch)
		} else {
			result += "_"
		}
	}
	return result
}

// messageJSON is a serializable representation of a Message.
type messageJSON struct {
	Payload   any    `json:"payload"`
	Sequence  int64  `json:"sequence"`
	Timestamp string `json:"timestamp"`
	Done      bool   `json:"done"`
}

// Save persists messages for a topic to disk.
func (fp *FilePersister) Save(topic string, messages []*Message) error {
	fp.mu.Lock()
	defer fp.mu.Unlock()

	// Convert to serializable format
	data := make([]messageJSON, len(messages))
	for i, msg := range messages {
		data[i] = messageJSON{
			Payload:   msg.Payload,
			Sequence:  msg.Sequence,
			Timestamp: msg.Timestamp.String(),
			Done:      msg.Done,
		}
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal messages for topic %s: %w", topic, err)
	}

	// Write to file
	path := fp.filePath(topic)
	if err := os.WriteFile(path, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write persistence file for topic %s: %w", topic, err)
	}

	log.Printf("[FilePersister] Saved %d messages for topic %s to %s", len(messages), topic, path)
	return nil
}

// Load loads persisted messages for a topic from disk.
func (fp *FilePersister) Load(topic string) ([]*Message, error) {
	fp.mu.Lock()
	defer fp.mu.Unlock()

	path := fp.filePath(topic)

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Printf("[FilePersister] No persisted data for topic %s", topic)
		return []*Message{}, nil
	}

	// Read file
	jsonData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read persistence file for topic %s: %w", topic, err)
	}

	// Unmarshal JSON
	var data []messageJSON
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal messages for topic %s: %w", topic, err)
	}

	// Convert to Message structs
	messages := make([]*Message, len(data))
	for i, mj := range data {
		// Parse timestamp (best effort)
		var ts int64
		if t, err := time.Parse(time.RFC3339Nano, mj.Timestamp); err == nil {
			ts = t.Unix()
		}

		messages[i] = &Message{
			Payload:   mj.Payload,
			Sequence:  mj.Sequence,
			Timestamp: time.UnixMilli(ts),
			Done:      mj.Done,
		}
	}

	log.Printf("[FilePersister] Loaded %d messages for topic %s from %s", len(messages), topic, path)
	return messages, nil
}

// Delete removes persisted messages for a topic.
func (fp *FilePersister) Delete(topic string) error {
	fp.mu.Lock()
	defer fp.mu.Unlock()

	path := fp.filePath(topic)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete persistence file for topic %s: %w", topic, err)
	}

	log.Printf("[FilePersister] Deleted persisted data for topic %s", topic)
	return nil
}

// Close closes the persister (no-op for file persister).
func (fp *FilePersister) Close() error {
	log.Printf("[FilePersister] Closed")
	return nil
}

// DeleteAll deletes all persisted data.
func (fp *FilePersister) DeleteAll() error {
	fp.mu.Lock()
	defer fp.mu.Unlock()

	if err := os.RemoveAll(fp.baseDir); err != nil {
		return fmt.Errorf("failed to delete persistence directory: %w", err)
	}

	if err := os.MkdirAll(fp.baseDir, 0755); err != nil {
		return fmt.Errorf("failed to recreate persistence directory: %w", err)
	}

	log.Printf("[FilePersister] Deleted all persisted data")
	return nil
}
