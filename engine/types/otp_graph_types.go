package types

import (
	"fmt"
	"path/filepath"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[OtpGraphWant, OtpGraphLocals]("otp_graph")
}

type OtpGraphLocals struct{}

// OtpGraphWant ensures graph.obj exists in data_dir.
// IsAchieved returns true as soon as the file is present on disk.
// Initialize builds the graph via docker (opentripplanner:2.6.0 --build --save) if missing.
// It guards on all prerequisite files so it can be safely retriggered before they are ready.
type OtpGraphWant struct {
	Want
}

func (o *OtpGraphWant) GetLocals() *OtpGraphLocals {
	return CheckLocalsInitialized[OtpGraphLocals](&o.Want)
}

func (o *OtpGraphWant) targetPath() string {
	dataDir := o.GetStringParam("data_dir", "/tmp/otp-data")
	return filepath.Join(dataDir, "graph.obj")
}

func (o *OtpGraphWant) Initialize() {
	target := o.targetPath()
	dataDir := o.GetStringParam("data_dir", "/tmp/otp-data")

	if fileExists(target) {
		o.SetCurrent("docker_phase", "done")
		o.SetCurrent("graph_path", target)
		o.StoreLog("[GRAPH] graph.obj already exists: %s — skipping build", target)
		return
	}

	// Guard: OSM data must be present.
	osmFilename := o.GetStringParam("osm_filename", "map.osm.pbf")
	if !fileExists(filepath.Join(dataDir, osmFilename)) {
		o.StoreLog("[GRAPH] Waiting for OSM file: %s", osmFilename)
		return
	}

	// Guard: if GTFS is configured, build-config.json and all GTFS zips must be present.
	feeds := resolveGTFSFeeds(o.Spec.Params)
	if len(feeds) > 0 {
		if !fileExists(filepath.Join(dataDir, "build-config.json")) {
			o.StoreLog("[GRAPH] Waiting for build-config.json")
			return
		}
		for _, f := range feeds {
			if !fileExists(filepath.Join(dataDir, f.Filename)) {
				o.StoreLog("[GRAPH] Waiting for GTFS file: %s", f.Filename)
				return
			}
		}
	}

	buildMem := o.GetStringParam("build_memory", "-Xmx6G")
	volume := fmt.Sprintf("[\"%s:/var/opentripplanner\"]", dataDir)

	o.SetCurrent("docker_image", "docker.io/opentripplanner/opentripplanner:2.6.0")
	o.SetCurrent("docker_container_name", "otp-builder")
	o.SetCurrent("docker_volumes", volume)
	o.SetCurrent("docker_env", fmt.Sprintf("{\"JAVA_OPTIONS\": \"%s\"}", buildMem))
	o.SetCurrent("docker_command_args", "[\"--build\", \"--save\"]")
	o.SetCurrent("docker_wait_for_exit", true)
	o.SetCurrent("docker_ports", "[]")
	o.SetCurrent("docker_phase", "starting")
	o.SetCurrent("graph_path", target)

	o.StoreLog("[GRAPH] Building OTP graph → %s", target)
	if err := o.ExecuteAgents(); err != nil {
		o.SetCurrent("docker_phase", "failed")
		o.SetCurrent("docker_error", err.Error())
		return
	}
	if code := GetCurrent(o, "docker_exit_code", -1); code != 0 {
		o.SetCurrent("docker_phase", "failed")
		o.SetCurrent("docker_error", fmt.Sprintf("OTP builder exited with code %d", code))
		return
	}
	o.SetCurrent("docker_phase", "done")
	o.StoreLog("[GRAPH] Build complete: %s", target)
}

func (o *OtpGraphWant) IsAchieved() bool {
	return fileExists(o.targetPath())
}

func (o *OtpGraphWant) CalculateAchievingPercentage() float64 {
	if o.IsAchieved() {
		return 100
	}
	phase := GetCurrent(o, "docker_phase", "")
	if phase == "starting" {
		return 30
	}
	return 5
}

func (o *OtpGraphWant) Progress() {
	o.SetCurrent("achieving_percentage", o.CalculateAchievingPercentage())
}
