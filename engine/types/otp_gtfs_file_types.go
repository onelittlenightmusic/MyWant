package types

import (
	"fmt"
	"os"
	"path/filepath"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[OtpGtfsFileWant, OtpGtfsFileLocals]("otp_gtfs_file")
}

type OtpGtfsFileLocals struct{}

// OtpGtfsFileWant ensures a single GTFS zip file exists in data_dir.
// One Want instance per GTFS feed. IsAchieved returns true when the file exists.
type OtpGtfsFileWant struct {
	Want
}

func (o *OtpGtfsFileWant) GetLocals() *OtpGtfsFileLocals {
	return CheckLocalsInitialized[OtpGtfsFileLocals](&o.Want)
}

func (o *OtpGtfsFileWant) targetPath() string {
	dataDir := o.GetStringParam("data_dir", "/tmp/otp-data")
	filename := o.GetStringParam("filename", "gtfs.zip")
	return filepath.Join(dataDir, filename)
}

func (o *OtpGtfsFileWant) Initialize() {
	target := o.targetPath()

	if fileExists(target) {
		o.SetCurrent("docker_phase", "done")
		o.SetCurrent("gtfs_file_path", target)
		o.StoreLog("[GTFS] File already exists: %s — skipping download", target)
		return
	}

	dataDir := o.GetStringParam("data_dir", "/tmp/otp-data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		o.SetCurrent("docker_phase", "failed")
		o.SetCurrent("docker_error", fmt.Sprintf("mkdir %s: %v", dataDir, err))
		return
	}

	gtfsURL := o.GetStringParam("url", "")
	if gtfsURL == "" {
		o.SetCurrent("docker_phase", "failed")
		o.SetCurrent("docker_error", "url parameter is required")
		return
	}
	filename := o.GetStringParam("filename", "gtfs.zip")
	// container_name must be unique per feed; derive from want name
	containerName := fmt.Sprintf("otp-gtfs-%s", o.Metadata.Name)
	volume := fmt.Sprintf("[\"%s:/data\"]", dataDir)
	cmdArgs := fmt.Sprintf("[\"sh\", \"-c\", \"wget -O /data/%s '%s'\"]", filename, gtfsURL)

	o.SetCurrent("docker_image", "alpine:latest")
	o.SetCurrent("docker_container_name", containerName)
	o.SetCurrent("docker_volumes", volume)
	o.SetCurrent("docker_command_args", cmdArgs)
	o.SetCurrent("docker_wait_for_exit", true)
	o.SetCurrent("docker_ports", "[]")
	o.SetCurrent("docker_env", "{}")
	o.SetCurrent("docker_phase", "starting")
	o.SetCurrent("gtfs_file_path", target)

	o.StoreLog("[GTFS] Downloading %s → %s", gtfsURL, target)
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
	o.StoreLog("[GTFS] Download complete: %s", target)
}

func (o *OtpGtfsFileWant) IsAchieved() bool {
	return fileExists(o.targetPath())
}

func (o *OtpGtfsFileWant) CalculateAchievingPercentage() float64 {
	if o.IsAchieved() {
		return 100
	}
	phase := GetCurrent(o, "docker_phase", "")
	if phase == "starting" {
		return 30
	}
	return 5
}

func (o *OtpGtfsFileWant) Progress() {
	o.SetCurrent("achieving_percentage", o.CalculateAchievingPercentage())
}
