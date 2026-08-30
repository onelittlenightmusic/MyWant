package mywant

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Secrets that must not ride in want state — OAuth refresh tokens, mostly —
// live in ~/.mywant/secrets/<namespace>.json instead: chmod 600, keyed by want
// id, one small JSON object per field. This keeps them out of state.yaml and
// out of anything that dumps want state (a Drive backup, a world export).
//
// It is the same place and shape the Spotify plugin's Python script already
// uses (~/.mywant/secrets/spotify_tokens.json); this is the Go side of that
// convention, so any localGo agent doing OAuth reaches for the one helper.
//
//   LoadSecretField("backup_google", wantID, "refresh_token")
//   SaveSecretField("backup_google", wantID, "refresh_token", tok)
//   ClearSecret("backup_google", wantID)

var secretStoreMu sync.Mutex

// SecretsDir returns ~/.mywant/secrets.
func SecretsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".mywant", "secrets")
}

func secretFilePath(namespace string) string {
	dir := SecretsDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, namespace+".json")
}

type secretFile map[string]map[string]string // wantID -> field -> value

func readSecretFile(namespace string) secretFile {
	p := secretFilePath(namespace)
	if p == "" {
		return secretFile{}
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return secretFile{}
	}
	var m secretFile
	if json.Unmarshal(b, &m) != nil || m == nil {
		return secretFile{}
	}
	return m
}

func writeSecretFile(namespace string, m secretFile) error {
	p := secretFilePath(namespace)
	if p == "" {
		return os.ErrInvalid
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, out, 0o600); err != nil {
		return err
	}
	return os.Chmod(p, 0o600)
}

// LoadSecretField returns the stored value, or "" if none.
func LoadSecretField(namespace, wantID, field string) string {
	secretStoreMu.Lock()
	defer secretStoreMu.Unlock()
	return readSecretFile(namespace)[wantID][field]
}

// SaveSecretField writes one field for one want, leaving other fields/wants intact.
func SaveSecretField(namespace, wantID, field, value string) error {
	secretStoreMu.Lock()
	defer secretStoreMu.Unlock()
	m := readSecretFile(namespace)
	if m[wantID] == nil {
		m[wantID] = map[string]string{}
	}
	m[wantID][field] = value
	return writeSecretFile(namespace, m)
}

// ClearSecret drops every field for one want.
func ClearSecret(namespace, wantID string) error {
	secretStoreMu.Lock()
	defer secretStoreMu.Unlock()
	m := readSecretFile(namespace)
	if _, ok := m[wantID]; !ok {
		return nil
	}
	delete(m, wantID)
	return writeSecretFile(namespace, m)
}
