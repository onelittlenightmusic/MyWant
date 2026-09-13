package mywant

import (
	"crypto/md5"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// What this install knows about the browsers that connect to it.
//
// These three settings used to live in the gui_state want's state, which is the
// wrong shelf for them twice over. A world's GUI snapshot is written when the
// world is left and replayed over the live state when it is entered
// (world_gui_state.go), so switching worlds restored whichever browser was
// pinned the last time you left that world — and `homeBrowserDevice`, unlike
// the two location fields, had no second copy to be rescued from. That is why
// "set home" kept coming undone. The two location fields were rescued by being
// duplicated into config.yaml and overlaid on every read, which worked and said
// plainly that they did not belong where they were.
//
// A pinned browser is a fact about THIS MACHINE, not about a world: which
// window browser work runs in, and which one is sending its location. Nothing
// about it should change because you looked at a different board. So it gets a
// file of its own, beside characters.yaml, and gui_state carries it only as a
// live mirror for clients to read.
//
// The device ROSTER is deliberately not here. Who is connected right now is
// presence, not configuration — entries expire in minutes and every browser
// re-announces itself every thirty seconds — so it stays in gui_state where the
// heartbeat writes it, and is simply kept out of world snapshots along with
// these (see guiStateVolatileKeys). A file that remembered who was online would
// only ever be wrong.
type DeviceSettings struct {
	// HomeBrowserDevice is the browser that browser-run work is pinned to.
	// Empty means whichever browser polls first, which is how it behaved
	// before there was a home.
	HomeBrowserDevice string `yaml:"homeBrowserDevice,omitempty" json:"homeBrowserDevice,omitempty"`
	// ActiveLocationDevice is the browser currently sending geolocation.
	ActiveLocationDevice string `yaml:"activeLocationDevice,omitempty" json:"activeLocationDevice,omitempty"`
	// LocationWantID is the want those geolocation updates are written to.
	LocationWantID string `yaml:"locationWantId,omitempty" json:"locationWantId,omitempty"`
}

// The gui_state keys these settings are mirrored under, so that the one place
// they are named is shared by everything that has to know: the PUT handler that
// routes them here, the read overlay that puts them back, and the volatile-key
// set that keeps world snapshots from carrying them.
const (
	DeviceKeyHome           = "homeBrowserDevice"
	DeviceKeyActiveLocation = "activeLocationDevice"
	DeviceKeyLocationWant   = "locationWantId"
	// DeviceKeyRoster is the live list of connected browsers. Not stored here
	// (see the note above) — named only so it can be excluded from world
	// snapshots with the rest.
	DeviceKeyRoster = "devices"
)

// DeviceGUIStateKeys are every gui_state key that is about devices rather than
// about a board.
var DeviceGUIStateKeys = []string{
	DeviceKeyHome, DeviceKeyActiveLocation, DeviceKeyLocationWant, DeviceKeyRoster,
}

type deviceManager struct {
	path     string
	mu       sync.RWMutex
	settings DeviceSettings
	lastHash string
}

var (
	globalDeviceStore *deviceManager
	deviceStoreOnce   sync.Once
)

// NewDeviceStoreAt opens a device store on an explicit path.
//
// The process uses the one from GetDeviceStore; this exists so a test can have
// its own file instead of writing on the machine it is running on — the store's
// whole point is that it persists, which makes a shared singleton the one thing
// a test must not reach for.
func NewDeviceStoreAt(path string) *deviceManager {
	m := &deviceManager{path: path}
	m.load()
	return m
}

// GetDeviceStore returns the process-wide device settings store, loading
// ~/.mywant/devices.yaml on first use.
func GetDeviceStore() *deviceManager {
	deviceStoreOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Printf("[WARN] device_store: cannot determine home dir: %v", err)
			home = "."
		}
		m := &deviceManager{path: filepath.Join(home, ".mywant", "devices.yaml")}
		m.load()
		globalDeviceStore = m
	})
	return globalDeviceStore
}

func (m *deviceManager) load() {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[WARN] device_store: failed to read %s: %v", m.path, err)
		}
		return
	}
	var s DeviceSettings
	if err := yaml.Unmarshal(data, &s); err != nil {
		log.Printf("[WARN] device_store: failed to unmarshal %s: %v", m.path, err)
		return
	}
	m.settings = s
	m.lastHash = fmt.Sprintf("%x", md5.Sum(data))
	log.Printf("[DeviceStore] Loaded device settings from %s (home=%q location=%q)",
		m.path, s.HomeBrowserDevice, s.ActiveLocationDevice)
}

// save writes only when the bytes would differ — same guard character_store
// uses, and it matters more here: the GUI pushes device state on a heartbeat,
// so most calls have nothing to say.
func (m *deviceManager) save() {
	data, err := yaml.Marshal(&m.settings)
	if err != nil {
		log.Printf("[WARN] device_store: marshal failed: %v", err)
		return
	}
	newHash := fmt.Sprintf("%x", md5.Sum(data))
	if newHash == m.lastHash {
		return
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0755); err != nil {
		log.Printf("[WARN] device_store: mkdir failed: %v", err)
		return
	}
	if err := os.WriteFile(m.path, data, 0644); err != nil {
		log.Printf("[WARN] device_store: write failed: %v", err)
		return
	}
	m.lastHash = newHash
}

// Settings returns a copy of the current device settings.
func (m *deviceManager) Settings() DeviceSettings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings
}

// Home is the browser browser-run work is pinned to, or "" for whichever polls
// first.
func (m *deviceManager) Home() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings.HomeBrowserDevice
}

// ApplyGUIUpdates takes whichever device settings appear in a PUT /gui/state
// body and persists them. Keys that are absent are left alone: the GUI sends
// one key at a time, and a missing key is not an empty one.
//
// An EMPTY value is kept, not ignored: "" is how the GUI unpins a device, and
// dropping it would make unpinning impossible.
func (m *deviceManager) ApplyGUIUpdates(updates map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	changed := false
	for key, dst := range map[string]*string{
		DeviceKeyHome:           &m.settings.HomeBrowserDevice,
		DeviceKeyActiveLocation: &m.settings.ActiveLocationDevice,
		DeviceKeyLocationWant:   &m.settings.LocationWantID,
	} {
		v, ok := updates[key]
		if !ok {
			continue
		}
		s, _ := v.(string) // a null clears it, same as ""
		if *dst != s {
			*dst = s
			changed = true
		}
	}
	if changed {
		m.save()
	}
}

// AdoptLegacy takes the values that used to be duplicated into config.yaml,
// for an install that had them there before this file existed. Only fills what
// is still empty, so it can run on every startup without ever undoing a
// setting made since.
func (m *deviceManager) AdoptLegacy(activeLocationDevice, locationWantID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	changed := false
	if m.settings.ActiveLocationDevice == "" && activeLocationDevice != "" {
		m.settings.ActiveLocationDevice = activeLocationDevice
		changed = true
	}
	if m.settings.LocationWantID == "" && locationWantID != "" {
		m.settings.LocationWantID = locationWantID
		changed = true
	}
	if changed {
		log.Printf("[DeviceStore] Adopted device settings from config.yaml")
		m.save()
	}
}

// AdoptHome takes a home pinned before this file existed, out of wherever it
// was last stored (the gui_state want). Same "only if still empty" rule as
// AdoptLegacy, and the reason it is separate: home never had a config.yaml
// copy, so its only prior home is the want's own state.
func (m *deviceManager) AdoptHome(home string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.settings.HomeBrowserDevice != "" || home == "" {
		return
	}
	m.settings.HomeBrowserDevice = home
	log.Printf("[DeviceStore] Adopted pinned browser %q from gui_state", home)
	m.save()
}
