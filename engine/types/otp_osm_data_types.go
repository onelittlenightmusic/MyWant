package types

import (
	"fmt"
	"os"
	"path/filepath"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[OtpOsmDataWant, OtpOsmDataLocals]("otp_osm_data")
}

type OtpOsmDataLocals struct{}

// OtpOsmDataWant ensures an OSM PBF file exists in data_dir.
// IsAchieved returns true as soon as the file is present on disk.
// Initialize downloads the file via docker (alpine/wget) if it is missing.
type OtpOsmDataWant struct {
	Want
}

func (o *OtpOsmDataWant) GetLocals() *OtpOsmDataLocals {
	return CheckLocalsInitialized[OtpOsmDataLocals](&o.Want)
}

func (o *OtpOsmDataWant) targetPath() string {
	dataDir := o.GetStringParam("data_dir", "/tmp/otp-data")
	filename := o.GetStringParam("osm_filename", "map.osm.pbf")
	return filepath.Join(dataDir, filename)
}

func (o *OtpOsmDataWant) Initialize() {
	target := o.targetPath()

	if fileExists(target) {
		o.SetCurrent("docker_phase", "done")
		o.SetCurrent("osm_file_path", target)
		o.StoreLog("[OSM] File already exists: %s — skipping download", target)
		return
	}

	dataDir := o.GetStringParam("data_dir", "/tmp/otp-data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		o.SetCurrent("docker_phase", "failed")
		o.SetCurrent("docker_error", fmt.Sprintf("mkdir %s: %v", dataDir, err))
		return
	}

	osmURL := o.GetStringParam("osm_url", "https://download.bbbike.org/osm/bbbike/Tokyo/Tokyo.osm.pbf")
	osmFilename := o.GetStringParam("osm_filename", "map.osm.pbf")
	volume := fmt.Sprintf("[\"%s:/data\"]", dataDir)
	cmdArgs := fmt.Sprintf("[\"sh\", \"-c\", \"wget -O /data/%s '%s'\"]", osmFilename, osmURL)

	o.SetCurrent("docker_image", "alpine:latest")
	o.SetCurrent("docker_container_name", "otp-download-osm")
	o.SetCurrent("docker_volumes", volume)
	o.SetCurrent("docker_command_args", cmdArgs)
	o.SetCurrent("docker_wait_for_exit", true)
	o.SetCurrent("docker_ports", "[]")
	o.SetCurrent("docker_env", "{}")
	o.SetCurrent("docker_phase", "starting")
	o.SetCurrent("osm_file_path", target)

	o.StoreLog("[OSM] Downloading %s → %s", osmURL, target)
	if err := o.ExecuteAgents(); err != nil {
		o.SetCurrent("docker_phase", "failed")
		o.SetCurrent("docker_error", err.Error())
		return
	}
	if code := GetCurrent(o, "docker_exit_code", -1); code != 0 {
		o.SetCurrent("docker_phase", "failed")
		o.SetCurrent("docker_error", fmt.Sprintf("wget exited with code %d", code))
		return
	}
	o.SetCurrent("docker_phase", "done")
	o.StoreLog("[OSM] Download complete: %s", target)
}

func (o *OtpOsmDataWant) IsAchieved() bool {
	return fileExists(o.targetPath())
}

func (o *OtpOsmDataWant) CalculateAchievingPercentage() float64 {
	if o.IsAchieved() {
		return 100
	}
	phase := GetCurrent(o, "docker_phase", "")
	if phase == "starting" {
		return 30
	}
	return 5
}

func (o *OtpOsmDataWant) Progress() {
	o.SetCurrent("achieving_percentage", o.CalculateAchievingPercentage())
}
