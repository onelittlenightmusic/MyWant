package mywant

// Customs are installable packages that extend a running MyWant server: want
// types, canvas design plugins, recipes and icon styles. A custom is kept in
// ~/.mywant/customs/<name> and linked into the runtime directory each of its
// component kinds is read from. ~/.mywant/customs.yaml records what was
// installed, from where, and which links were created, so uninstall knows
// exactly what to undo.
//
// Both the CLI (local installs) and the HTTP API (remote installs onto the
// server's own filesystem) drive this file.

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// MyWantHomeDir returns ~/.mywant, creating it when missing.
func MyWantHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".mywant"
	}
	dir := filepath.Join(home, ".mywant")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// DefaultCustomOwner is the GitHub owner used when a custom is installed by bare name.
const DefaultCustomOwner = "onelittlenightmusic"

// CustomKindSpec describes where a component kind is installed and how it is detected.
type CustomKindSpec struct {
	Dir      func() string          // destination directory
	LinkYAML bool                   // link individual .yaml files (dest scanner does not follow symlinked dirs)
	Detect   func(path string) bool // auto-detection when the custom has no custom.yaml
	Note     string                 // extra hint printed after install
}

const (
	kindCustomType = "custom-type"
	kindDesign     = "design"
	kindRecipe     = "recipe"
	kindIcon       = "icon"
)

// CustomKindOrder keeps output and detection deterministic.
var CustomKindOrder = []string{kindCustomType, kindDesign, kindRecipe, kindIcon}

var CustomKinds = map[string]CustomKindSpec{
	kindCustomType: {
		Dir:    func() string { return filepath.Join(MyWantHomeDir(), "custom-types") },
		Detect: func(path string) bool { return yamlTreeHasKey(path, "wantType") || yamlTreeHasKey(path, "agent") },
	},
	kindDesign: {
		Dir:    func() string { return filepath.Join(MyWantHomeDir(), "design-plugin") },
		Detect: func(path string) bool { return findPluginEntry(path) != "" },
	},
	kindRecipe: {
		Dir:      func() string { return filepath.Join(MyWantHomeDir(), "recipes") },
		LinkYAML: true, // the recipe scanner uses filepath.Walk, which does not descend into symlinked dirs
		Detect:   func(path string) bool { return yamlTreeHasKey(path, "recipe") },
	},
	kindIcon: {
		Dir: func() string { return filepath.Join(MyWantHomeDir(), "icons") },
		Detect: func(path string) bool {
			return isDir(filepath.Join(path, "icons")) || fileExists(filepath.Join(path, "icon-style.yaml"))
		},
		Note: "icon styles are not read by the server yet; the files are installed for forward compatibility",
	},
}

// ---------------------------------------------------------------------------
// ~/.mywant/customs.yaml
// ---------------------------------------------------------------------------

// CustomComponent is one linked part of a custom: a subdirectory of the custom
// store published into the runtime directory for its kind.
type CustomComponent struct {
	Kind string `yaml:"kind" json:"kind"`
	Path string `yaml:"path" json:"path"` // relative to the custom store root ("." = whole custom)
	Link string `yaml:"link" json:"link"` // absolute path created in the destination directory
}

// CustomRecord is one entry of ~/.mywant/customs.yaml.
type CustomRecord struct {
	Name        string            `yaml:"name" json:"name"`
	Source      string            `yaml:"source" json:"source"`
	Origin      string            `yaml:"origin" json:"origin"` // git | local
	Commit      string            `yaml:"commit,omitempty" json:"commit,omitempty"`
	InstalledAt string            `yaml:"installed_at,omitempty" json:"installed_at,omitempty"`
	UpdatedAt   string            `yaml:"updated_at,omitempty" json:"updated_at,omitempty"`
	Components  []CustomComponent `yaml:"components" json:"components"`
	WantTypes   []string          `yaml:"want_types,omitempty" json:"want_types,omitempty"`
	Agents      []string          `yaml:"agents,omitempty" json:"agents,omitempty"`
	// Status is filled in by the API layer; it is derived, never persisted.
	Status string `yaml:"-" json:"status,omitempty"`
}

// CustomRegistry is the whole ~/.mywant/customs.yaml document.
type CustomRegistry struct {
	Customs []CustomRecord `yaml:"customs"`
}

func CustomsFilePath() string {
	return filepath.Join(MyWantHomeDir(), "customs.yaml")
}

func CustomStoreDir() string {
	return filepath.Join(MyWantHomeDir(), "customs")
}

func CustomStorePath(name string) string {
	return filepath.Join(CustomStoreDir(), name)
}

func LoadCustomRegistry() (*CustomRegistry, error) {
	reg := &CustomRegistry{}
	data, err := os.ReadFile(CustomsFilePath())
	if os.IsNotExist(err) {
		return reg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, reg); err != nil {
		return nil, err
	}
	return reg, nil
}

func SaveCustomRegistry(reg *CustomRegistry) error {
	sort.Slice(reg.Customs, func(i, j int) bool { return reg.Customs[i].Name < reg.Customs[j].Name })
	data, err := yaml.Marshal(reg)
	if err != nil {
		return err
	}
	header := "# MyWant customs installed via 'mywant custom install'.\n" +
		"# Managed by the CLI - edit only if you know what you are doing.\n"
	return os.WriteFile(CustomsFilePath(), append([]byte(header), data...), 0644)
}

func (r *CustomRegistry) Find(name string) *CustomRecord {
	for i := range r.Customs {
		if r.Customs[i].Name == name {
			return &r.Customs[i]
		}
	}
	return nil
}

func (r *CustomRegistry) Upsert(rec CustomRecord) {
	if existing := r.Find(rec.Name); existing != nil {
		*existing = rec
		return
	}
	r.Customs = append(r.Customs, rec)
}

func (r *CustomRegistry) Remove(name string) {
	kept := r.Customs[:0]
	for _, a := range r.Customs {
		if a.Name != name {
			kept = append(kept, a)
		}
	}
	r.Customs = kept
}

func (a CustomRecord) Kinds() []string {
	var kinds []string
	seen := map[string]bool{}
	for _, c := range a.Components {
		if !seen[c.Kind] {
			seen[c.Kind] = true
			kinds = append(kinds, c.Kind)
		}
	}
	return kinds
}

func (a CustomRecord) Provides() []string {
	return append(append([]string{}, a.WantTypes...), a.Agents...)
}

// status reports whether the store and every link are still in place.
func (a CustomRecord) DeriveStatus() string {
	if !isDir(CustomStorePath(a.Name)) {
		return "store missing"
	}
	for _, c := range a.Components {
		if _, err := os.Lstat(c.Link); err != nil {
			return "link missing"
		}
	}
	return "ok"
}

// ---------------------------------------------------------------------------
// custom.yaml manifest (optional, inside the custom)
// ---------------------------------------------------------------------------

type customManifest struct {
	Custom struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
		Components  []struct {
			Kind string `yaml:"kind"`
			Path string `yaml:"path"`
			Name string `yaml:"name"` // destination name, defaults to the custom name
		} `yaml:"components"`
	} `yaml:"custom"`
}

func readCustomManifest(storePath string) *customManifest {
	data, err := os.ReadFile(filepath.Join(storePath, "custom.yaml"))
	if err != nil {
		return nil
	}
	var m customManifest
	if yaml.Unmarshal(data, &m) != nil || m.Custom.Name == "" && len(m.Custom.Components) == 0 {
		return nil
	}
	return &m
}

// ---------------------------------------------------------------------------
// install
// ---------------------------------------------------------------------------

func InstallCustom(source, overrideName, kindFilter string, force bool) (CustomRecord, error) {
	resolved, origin, derivedName := ResolveCustomSource(source)
	name := overrideName
	if name == "" {
		name = derivedName
	}
	if err := validateCustomName(name); err != nil {
		return CustomRecord{}, fmt.Errorf("%w; pass --name", err)
	}

	reg, err := LoadCustomRegistry()
	if err != nil {
		return CustomRecord{}, err
	}
	previous := reg.Find(name)

	store := CustomStorePath(name)
	if err := os.MkdirAll(CustomStoreDir(), 0755); err != nil {
		return CustomRecord{}, err
	}

	// Fetch (or refresh) the custom into the store.
	switch _, statErr := os.Stat(store); {
	case statErr == nil && previous == nil && !force:
		return CustomRecord{}, fmt.Errorf("%s already exists but is not tracked in %s; use --force to replace it",
			store, filepath.Base(CustomsFilePath()))
	case statErr == nil && origin == "git" && isGitRepo(store) && !force:
		fmt.Printf("%s already installed, updating...\n", name)
		if err := runGit(store, "pull", "--ff-only"); err != nil {
			return CustomRecord{}, err
		}
	case statErr == nil:
		if err := os.RemoveAll(store); err != nil {
			return CustomRecord{}, err
		}
		fallthrough
	default:
		if origin == "local" {
			if err := replaceDirFromLocal(resolved, store); err != nil {
				return CustomRecord{}, err
			}
		} else if err := runGit("", "clone", resolved, store); err != nil {
			return CustomRecord{}, err
		}
	}

	// Decide which components this custom publishes.
	components, err := planComponents(store, name, kindFilter)
	if err != nil {
		return CustomRecord{}, err
	}

	// Drop links from a previous install that are no longer part of the custom.
	if previous != nil {
		for _, old := range previous.Components {
			if !componentLinked(components, old.Link) {
				_ = removeCustomLink(old, force)
			}
		}
	}

	for i := range components {
		if err := linkComponent(store, &components[i], force); err != nil {
			return CustomRecord{}, err
		}
	}

	now := time.Now().Format(time.RFC3339)
	rec := CustomRecord{
		Name:        name,
		Source:      resolved,
		Origin:      origin,
		Commit:      gitCommit(store),
		InstalledAt: now,
		UpdatedAt:   now,
		Components:  components,
	}
	if previous != nil && previous.InstalledAt != "" {
		rec.InstalledAt = previous.InstalledAt
	}
	rec.WantTypes, rec.Agents = ScanCustomYAML(store)

	reg.Upsert(rec)
	if err := SaveCustomRegistry(reg); err != nil {
		return rec, fmt.Errorf("custom installed but %s could not be updated: %w", CustomsFilePath(), err)
	}
	return rec, nil
}

// planComponents builds the component list from custom.yaml, an explicit --kind,
// or auto-detection of the custom root.
func planComponents(store, name, kindFilter string) ([]CustomComponent, error) {
	var wanted []string
	if kindFilter != "" {
		for k := range strings.SplitSeq(kindFilter, ",") {
			k = strings.TrimSpace(k)
			if _, ok := CustomKinds[k]; !ok {
				return nil, fmt.Errorf("unknown custom kind %q (expected one of %s)", k, strings.Join(CustomKindOrder, ", "))
			}
			wanted = append(wanted, k)
		}
	}

	var components []CustomComponent
	if m := readCustomManifest(store); m != nil && len(m.Custom.Components) > 0 {
		for _, c := range m.Custom.Components {
			if _, ok := CustomKinds[c.Kind]; !ok {
				return nil, fmt.Errorf("custom.yaml declares unknown kind %q", c.Kind)
			}
			if len(wanted) > 0 && !slices.Contains(wanted, c.Kind) {
				continue
			}
			path := c.Path
			if path == "" {
				path = "."
			}
			if !isDir(filepath.Join(store, path)) {
				return nil, fmt.Errorf("custom.yaml component path %q does not exist in the custom", path)
			}
			destName := c.Name
			if destName == "" {
				destName = name
			}
			components = append(components, CustomComponent{
				Kind: c.Kind,
				Path: path,
				Link: filepath.Join(CustomKinds[c.Kind].Dir(), destName),
			})
		}
	} else {
		kinds := wanted
		if len(kinds) == 0 {
			for _, k := range CustomKindOrder {
				if CustomKinds[k].Detect(store) {
					kinds = append(kinds, k)
				}
			}
		}
		for _, k := range kinds {
			components = append(components, CustomComponent{
				Kind: k,
				Path: ".",
				Link: filepath.Join(CustomKinds[k].Dir(), name),
			})
		}
	}

	if len(components) == 0 {
		return nil, fmt.Errorf("could not tell what this custom provides; pass --kind (%s)", strings.Join(CustomKindOrder, "|"))
	}
	return components, nil
}

// linkComponent publishes one component into its runtime directory. Directories
// are symlinked; for kinds whose scanner does not follow symlinked directories
// the YAML files are linked individually inside a real directory.
func linkComponent(store string, c *CustomComponent, force bool) error {
	src := filepath.Join(store, c.Path)
	if err := os.MkdirAll(filepath.Dir(c.Link), 0755); err != nil {
		return err
	}

	if !CustomKinds[c.Kind].LinkYAML {
		return replaceSymlink(src, c.Link, force)
	}

	if info, err := os.Lstat(c.Link); err == nil {
		if info.Mode()&os.ModeSymlink == 0 && !isCustomOwnedDir(c.Link, store) && !force {
			return fmt.Errorf("%s already exists and was not created by this custom; use --force to replace it", c.Link)
		}
		if err := os.RemoveAll(c.Link); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(c.Link, 0755); err != nil {
		return err
	}
	linked := 0
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != src && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if ext := filepath.Ext(p); ext != ".yaml" && ext != ".yml" {
			return nil
		}
		rel, relErr := filepath.Rel(src, p)
		if relErr != nil {
			return nil
		}
		dst := filepath.Join(c.Link, strings.ReplaceAll(rel, string(os.PathSeparator), "-"))
		if err := replaceSymlink(p, dst, true); err != nil {
			return err
		}
		linked++
		return nil
	})
	if err != nil {
		return err
	}
	if linked == 0 {
		return fmt.Errorf("no YAML files found in %s for kind %s", src, c.Kind)
	}
	return nil
}

// isCustomOwnedDir reports whether dir only holds symlinks pointing into store,
// i.e. it was created by a previous linkComponent for this custom.
func isCustomOwnedDir(dir, store string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		return false
	}
	for _, e := range entries {
		target, err := os.Readlink(filepath.Join(dir, e.Name()))
		if err != nil || !strings.HasPrefix(target, store) {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// uninstall
// ---------------------------------------------------------------------------

// UninstallCustom removes the links and the store copy of a custom. Customs that
// predate customs.yaml are handled by looking for a directory of that name in
// each runtime directory.
func UninstallCustom(name string, force bool) (removed []string, hadAgents bool, err error) {
	if err := validateCustomName(name); err != nil {
		return nil, false, err
	}

	reg, err := LoadCustomRegistry()
	if err != nil {
		return nil, false, err
	}

	rec := reg.Find(name)
	if rec == nil {
		return uninstallUntrackedCustom(name, force)
	}
	hadAgents = len(rec.Agents) > 0

	for _, c := range rec.Components {
		if err := removeCustomLink(c, force); err != nil {
			return removed, hadAgents, err
		}
		removed = append(removed, c.Link)
	}

	store := CustomStorePath(name)
	if info, statErr := os.Lstat(store); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(store); err != nil {
				return removed, hadAgents, err
			}
		} else if !customRefetchable(*rec, store) && !force {
			return removed, hadAgents, fmt.Errorf(
				"%s cannot be re-fetched (source %s is gone and it is not a git clone); use --force to delete it", store, rec.Source)
		} else if err := os.RemoveAll(store); err != nil {
			return removed, hadAgents, err
		}
		removed = append(removed, store)
	}

	reg.Remove(name)
	if err := SaveCustomRegistry(reg); err != nil {
		return removed, hadAgents, err
	}
	return removed, hadAgents, nil
}

// uninstallUntrackedCustom removes a directory installed before customs.yaml existed
// (e.g. cloned into ~/.mywant/custom-types by hand).
func uninstallUntrackedCustom(name string, force bool) (removed []string, hadAgents bool, err error) {
	found := false
	for _, kind := range CustomKindOrder {
		path := filepath.Join(CustomKinds[kind].Dir(), name)
		info, statErr := os.Lstat(path)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return removed, hadAgents, statErr
		}
		found = true

		if _, agents := ScanCustomYAML(path); len(agents) > 0 {
			hadAgents = true
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(path); err != nil {
				return removed, hadAgents, err
			}
		} else {
			if !isGitRepo(path) && !force {
				return removed, hadAgents, fmt.Errorf(
					"%s is not tracked in %s and is not a git clone, so its contents cannot be re-fetched; use --force to delete it",
					path, filepath.Base(CustomsFilePath()))
			}
			if err := os.RemoveAll(path); err != nil {
				return removed, hadAgents, err
			}
		}
		removed = append(removed, path)
	}

	if !found {
		return nil, false, fmt.Errorf("custom %q is not installed", name)
	}
	return removed, hadAgents, nil
}

// customRefetchable reports whether deleting the store loses nothing that
// "custom install" could not fetch again.
func customRefetchable(rec CustomRecord, store string) bool {
	if rec.Origin == "git" || isGitRepo(store) {
		return true
	}
	return rec.Origin == "local" && isDir(rec.Source)
}

func removeCustomLink(c CustomComponent, force bool) error {
	info, err := os.Lstat(c.Link)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return os.Remove(c.Link)
	}
	if CustomKinds[c.Kind].LinkYAML || force {
		return os.RemoveAll(c.Link)
	}
	return fmt.Errorf("%s is not a symlink created by this custom; use --force to remove it", c.Link)
}

// ---------------------------------------------------------------------------
// inspection helpers
// ---------------------------------------------------------------------------

type UntrackedCustom struct {
	Name string `json:"name" yaml:"name"`
	// Kind is the runtime directory it sits in, i.e. how the server treats it.
	Kind   string `json:"kind" yaml:"kind"`
	Path   string `json:"path" yaml:"path"`
	Source string `json:"source" yaml:"source"` // git remote, symlink target, or "-"
	// Looks lists the kinds the content actually matches, which spots misplaced directories.
	Looks []string `json:"looks,omitempty" yaml:"looks,omitempty"`
}

// FindUntrackedCustoms lists directories sitting in the runtime directories that
// no customs.yaml record accounts for.
func FindUntrackedCustoms(reg *CustomRegistry) []UntrackedCustom {
	linked := map[string]bool{}
	for _, a := range reg.Customs {
		for _, c := range a.Components {
			linked[c.Link] = true
		}
	}

	var out []UntrackedCustom
	for _, kind := range CustomKindOrder {
		base := CustomKinds[kind].Dir()
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, e := range entries {
			path := filepath.Join(base, e.Name())
			if linked[path] {
				continue
			}
			if info, err := os.Stat(path); err != nil || !info.IsDir() {
				continue
			}
			out = append(out, UntrackedCustom{
				Name:   e.Name(),
				Kind:   kind,
				Path:   path,
				Source: CustomLiveSource(path),
				Looks:  DetectCustomKinds(path),
			})
		}
	}
	return out
}

// DetectCustomKinds reports which component kinds a directory's content matches.
func DetectCustomKinds(path string) []string {
	var kinds []string
	for _, kind := range CustomKindOrder {
		if CustomKinds[kind].Detect(path) {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

// CustomLiveSource derives where a directory came from without consulting customs.yaml:
// the git remote for a clone, the target for a symlink. A plain copied directory
// keeps no provenance, so there is nothing to report.
func CustomLiveSource(path string) string {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if target, err := os.Readlink(path); err == nil {
			return "link:" + target
		}
	}
	if isGitRepo(path) {
		out, err := exec.Command("git", "-C", path, "config", "--get", "remote.origin.url").Output()
		if remote := strings.TrimSpace(string(out)); err == nil && remote != "" {
			if commit := gitCommit(path); commit != "" {
				return remote + " @" + commit
			}
			return remote
		}
		return "git (no remote)"
	}
	return "-"
}

// ScanCustomYAML collects the wantType and agent names a custom declares.
func ScanCustomYAML(path string) (wantTypes []string, agents []string) {
	_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != path && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if ext := filepath.Ext(p); ext != ".yaml" && ext != ".yml" {
			return nil
		}
		data, readErr := os.ReadFile(p)
		if readErr != nil {
			return nil
		}
		var doc struct {
			WantType struct {
				Metadata struct {
					Name string `yaml:"name"`
				} `yaml:"metadata"`
			} `yaml:"wantType"`
			Agent struct {
				Name string `yaml:"name"`
			} `yaml:"agent"`
		}
		if yaml.Unmarshal(data, &doc) != nil {
			return nil
		}
		if n := doc.WantType.Metadata.Name; n != "" {
			wantTypes = append(wantTypes, n)
		}
		if n := doc.Agent.Name; n != "" {
			agents = append(agents, n)
		}
		return nil
	})
	sort.Strings(wantTypes)
	sort.Strings(agents)
	return wantTypes, agents
}

// yamlTreeHasKey reports whether any YAML file under path has the given top-level key.
func yamlTreeHasKey(path, key string) bool {
	found := false
	_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if d.IsDir() {
			if p != path && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if ext := filepath.Ext(p); ext != ".yaml" && ext != ".yml" {
			return nil
		}
		data, readErr := os.ReadFile(p)
		if readErr != nil {
			return nil
		}
		var doc map[string]any
		if yaml.Unmarshal(data, &doc) != nil {
			return nil
		}
		if _, ok := doc[key]; ok {
			found = true
		}
		return nil
	})
	return found
}

// findPluginEntry returns the design plugin entry file at the root of path, if any.
func findPluginEntry(path string) string {
	for _, name := range []string{"plugin.jsx", "plugin.tsx", "plugin.js", "plugin.ts"} {
		candidate := filepath.Join(path, name)
		if fileExists(candidate) {
			return candidate
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// source resolution and small utilities
// ---------------------------------------------------------------------------

// ResolveCustomSource turns a user-supplied source into a local directory path or
// a git URL, together with the default install name.
func ResolveCustomSource(source string) (resolved, origin, name string) {
	if isLocalDir(source) {
		abs, err := filepath.Abs(source)
		if err != nil {
			abs = source
		}
		return abs, "local", filepath.Base(abs)
	}

	url := source
	switch {
	case strings.Contains(source, "://") || strings.HasPrefix(source, "git@"):
		// full git URL, use as-is
	case strings.Count(source, "/") == 1:
		url = fmt.Sprintf("https://github.com/%s.git", strings.TrimSuffix(source, ".git"))
	default:
		repo := strings.TrimSuffix(source, ".git")
		if !strings.HasPrefix(repo, "mywant-") {
			repo = "mywant-" + repo
		}
		url = fmt.Sprintf("https://github.com/%s/%s.git", DefaultCustomOwner, repo)
	}

	name = strings.TrimSuffix(filepath.Base(strings.TrimSuffix(url, "/")), ".git")
	return url, "git", name
}

func validateCustomName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") || strings.HasPrefix(name, ".") {
		return fmt.Errorf("invalid custom name %q", name)
	}
	return nil
}

func componentLinked(components []CustomComponent, link string) bool {
	for _, c := range components {
		if c.Link == link {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// filesystem helpers
// ---------------------------------------------------------------------------

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isLocalDir(path string) bool {
	if strings.Contains(path, "://") || strings.HasSuffix(path, ".git") {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isGitRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}

func gitCommit(dir string) string {
	if !isGitRepo(dir) {
		return ""
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func replaceSymlink(src, dst string, force bool) error {
	if info, err := os.Lstat(dst); err == nil {
		if info.Mode()&os.ModeSymlink == 0 && !force {
			return fmt.Errorf("%s already exists and is not a symlink; use force to replace it", dst)
		}
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(src, dst)
}

func replaceDirFromLocal(src, dst string) error {
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	if srcAbs == dstAbs {
		return nil
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	return copyDir(src, dst)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		out := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, out, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
