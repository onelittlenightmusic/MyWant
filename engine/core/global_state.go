package mywant

import (
	"crypto/md5"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// GetGlobalStateValue returns a single value from the global state.
func (cb *ChainBuilder) GetGlobalStateValue(key string) (any, bool) {
	return cb.globalState.Load(key)
}

// StoreGlobalState stores a single key-value in the global state and persists to disk.
func (cb *ChainBuilder) StoreGlobalState(key string, value any) {
	cb.globalState.Store(key, value)
	cb.saveGlobalStateToFile()
}

// MergeGlobalState deep-merges a map into the global state and persists to disk.
func (cb *ChainBuilder) MergeGlobalState(updates map[string]any) {
	for key, value := range updates {
		existing, ok := cb.globalState.Load(key)
		if ok {
			// If both are maps, deep merge
			existingMap, existingIsMap := existing.(map[string]any)
			updateMap, updateIsMap := value.(map[string]any)
			if existingIsMap && updateIsMap {
				merged := make(map[string]any, len(existingMap))
				for k, v := range existingMap {
					merged[k] = v
				}
				for k, v := range updateMap {
					merged[k] = v
				}
				cb.globalState.Store(key, merged)
				continue
			}
		}
		cb.globalState.Store(key, value)
	}
	cb.saveGlobalStateToFile()
}

// GetGlobalStateAll returns a snapshot copy of the entire global state.
func (cb *ChainBuilder) GetGlobalStateAll() map[string]any {
	result := make(map[string]any)
	cb.globalState.Range(func(key, value any) bool {
		if k, ok := key.(string); ok {
			result[k] = value
		}
		return true
	})
	return result
}

// loadGlobalStateFromFile loads persisted global state from disk at startup.
func (cb *ChainBuilder) loadGlobalStateFromFile() {
	if cb.globalStatePath == "" {
		return
	}

	data, err := os.ReadFile(cb.globalStatePath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[WARN] Failed to read global state file %s: %v", cb.globalStatePath, err)
		}
		return
	}

	var stateMap map[string]any
	if err := yaml.Unmarshal(data, &stateMap); err != nil {
		log.Printf("[WARN] Failed to unmarshal global state file %s: %v", cb.globalStatePath, err)
		return
	}

	for k, v := range stateMap {
		cb.globalState.Store(k, v)
	}

	cb.lastGlobalStateHash = fmt.Sprintf("%x", md5.Sum(data))
	log.Printf("[ChainBuilder] Loaded global state from %s (%d keys)", cb.globalStatePath, len(stateMap))
}

// saveGlobalStateToFile writes the global state to disk, skipping if unchanged.
func (cb *ChainBuilder) saveGlobalStateToFile() {
	if cb.globalStatePath == "" {
		return
	}

	stateMap := cb.GetGlobalStateAll()

	data, err := yaml.Marshal(stateMap)
	if err != nil {
		log.Printf("[WARN] Failed to marshal global state: %v", err)
		return
	}

	newHash := fmt.Sprintf("%x", md5.Sum(data))
	if newHash == cb.lastGlobalStateHash {
		return
	}

	// Ensure directory exists
	dir := filepath.Dir(cb.globalStatePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("[WARN] Failed to create global state directory %s: %v", dir, err)
		return
	}

	if err := os.WriteFile(cb.globalStatePath, data, 0644); err != nil {
		log.Printf("[WARN] Failed to write global state file %s: %v", cb.globalStatePath, err)
		return
	}

	cb.lastGlobalStateHash = newHash
}

// --- Package-level functions (callable from any Want or code) ---

// GetGlobalState returns the value for key from the global state.
func GetGlobalState(key string) (any, bool) {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return nil, false
	}
	return cb.GetGlobalStateValue(key)
}

// StoreGlobalState stores a value in the global state.
func StoreGlobalState(key string, value any) {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return
	}
	cb.StoreGlobalState(key, value)
}

// MergeGlobalState deep-merges updates into the global state.
func MergeGlobalState(updates map[string]any) {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return
	}
	cb.MergeGlobalState(updates)
}

// GetAllGlobalState returns a copy of the entire global state.
func GetAllGlobalState() map[string]any {
	cb := GetGlobalChainBuilder()
	if cb == nil {
		return nil
	}
	return cb.GetGlobalStateAll()
}

// --- Want convenience methods ---

// GetGlobalState is callable from any Want instance.
func (n *Want) GetGlobalState(key string) (any, bool) {
	return GetGlobalState(key)
}

// StoreGlobalState is callable from any Want instance.
func (n *Want) StoreGlobalState(key string, value any) {
	StoreGlobalState(key, value)
}

// MergeGlobalState is callable from any Want instance.
func (n *Want) MergeGlobalState(updates map[string]any) {
	MergeGlobalState(updates)
}
