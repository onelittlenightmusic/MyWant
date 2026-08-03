package mywant

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"gopkg.in/yaml.v3"
)

// The `environments:` block of config.yaml holds env vars the server process
// runs with — that is how a custom type's skill reaches its API key, since
// skills are spawned with cmd.Env = os.Environ().
//
// Applying them is not a one-shot startup step: `mywant config env set` writes
// the file while the server is running, and the point of writing it is to
// affect that server. So this can be re-run, and the re-run has to be careful
// about one thing — an env var exported in the shell that started the server
// outranks the file, and must survive every reload.

var envState = struct {
	mu sync.Mutex
	// path is the config file to read. Empty means the default location, so
	// `mywant --config x.yaml start` keeps reloading x.yaml.
	path string
	// applied names the keys this process set from the file. Only these may be
	// overwritten or removed on a reload; anything else in the environment
	// belongs to the shell.
	applied map[string]bool
}{applied: map[string]bool{}}

// SetConfigEnvPath points the environment reloads at a specific config file.
func SetConfigEnvPath(path string) {
	envState.mu.Lock()
	defer envState.mu.Unlock()
	envState.path = path
}

func configEnvPath() string {
	if envState.path != "" {
		return envState.path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".mywant", "config.yaml")
}

// readConfigEnvironments pulls just the `environments:` block out of the config
// file. Only that key is decoded, so the CLI stays the owner of the schema.
func readConfigEnvironments(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}

	var file struct {
		Environments map[string]string `yaml:"environments"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	if file.Environments == nil {
		return map[string]string{}, nil
	}
	return file.Environments, nil
}

// ApplyConfigEnvironments sets the config's environments on this process and
// returns the key names now in force, sorted.
//
// Precedence, in order:
//   - a variable already exported in the shell wins and is never touched
//   - a key this process applied earlier is updated, or unset when it has been
//     removed from the file, so `config env set/unset` takes effect at once
func ApplyConfigEnvironments() ([]string, error) {
	path := configEnvPath()
	if path == "" {
		return nil, fmt.Errorf("cannot locate the config file")
	}

	wanted, err := readConfigEnvironments(path)
	if err != nil {
		return nil, err
	}

	envState.mu.Lock()
	defer envState.mu.Unlock()

	// Keys dropped from the file go away — but only the ones we put there.
	for key := range envState.applied {
		if _, stillWanted := wanted[key]; !stillWanted {
			os.Unsetenv(key)
			delete(envState.applied, key)
		}
	}

	names := make([]string, 0, len(wanted))
	for key, value := range wanted {
		if value == "" {
			continue
		}
		// Ours to update; anyone else's to leave alone.
		if os.Getenv(key) == "" || envState.applied[key] {
			os.Setenv(key, value)
			envState.applied[key] = true
		}
		names = append(names, key)
	}
	sort.Strings(names)

	return names, nil
}
