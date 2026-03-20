package types

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[OtpBuildConfigWant, OtpBuildConfigLocals]("otp_build_config")
}

type OtpBuildConfigLocals struct{}

// OtpBuildConfigWant ensures build-config.json exists in data_dir with the
// correct transitFeeds entries for all configured GTFS files.
// If no GTFS feeds are configured, IsAchieved returns true immediately.
// File is written directly from Go (no docker needed).
type OtpBuildConfigWant struct {
	Want
}

func (o *OtpBuildConfigWant) GetLocals() *OtpBuildConfigLocals {
	return CheckLocalsInitialized[OtpBuildConfigLocals](&o.Want)
}

func (o *OtpBuildConfigWant) configPath() string {
	dataDir := o.GetStringParam("data_dir", "/tmp/otp-data")
	return filepath.Join(dataDir, "build-config.json")
}

func (o *OtpBuildConfigWant) Initialize() {
	feeds := resolveGTFSFeeds(o.Spec.Params)

	if len(feeds) == 0 {
		// No GTFS — no config file needed; OTP will build from OSM only.
		o.SetCurrent("config_written", true)
		o.StoreLog("[BUILD-CONFIG] No GTFS feeds configured — skipping build-config.json")
		return
	}

	configPath := o.configPath()
	if fileExists(configPath) {
		o.SetCurrent("config_written", true)
		o.StoreLog("[BUILD-CONFIG] Already exists: %s — skipping", configPath)
		return
	}

	dataDir := o.GetStringParam("data_dir", "/tmp/otp-data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		o.SetCurrent("config_written", false)
		o.StoreLog("[BUILD-CONFIG] ERROR: mkdir %s: %v", dataDir, err)
		return
	}

	// Build JSON content.
	var entries []string
	for _, f := range feeds {
		entries = append(entries, fmt.Sprintf(`{"type":"gtfs","source":"%s"}`, f.Filename))
	}
	content := fmt.Sprintf(`{"transitFeeds":[%s]}`, strings.Join(entries, ","))

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		o.SetCurrent("config_written", false)
		o.StoreLog("[BUILD-CONFIG] ERROR: write %s: %v", configPath, err)
		return
	}

	o.SetCurrent("config_written", true)
	o.StoreLog("[BUILD-CONFIG] Written: %s", configPath)
}

func (o *OtpBuildConfigWant) IsAchieved() bool {
	feeds := resolveGTFSFeeds(o.Spec.Params)
	if len(feeds) == 0 {
		return true
	}
	return fileExists(o.configPath())
}

func (o *OtpBuildConfigWant) CalculateAchievingPercentage() float64 {
	if o.IsAchieved() {
		return 100
	}
	return 10
}

func (o *OtpBuildConfigWant) Progress() {
	o.SetCurrent("achieving_percentage", o.CalculateAchievingPercentage())
}
