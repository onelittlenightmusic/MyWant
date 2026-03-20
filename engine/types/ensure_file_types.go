package types

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[EnsureFileWant, EnsureFileLocals]("ensure_file")
}

type EnsureFileLocals struct{}

// EnsureFileWant ensures a file exists at data_dir/filename.
// If the file is missing it creates it using the configured method:
//
//   - "curl" (default): download via Go net/http — no docker needed
//   - "docker_wget":    download via docker alpine/wget
//   - "docker_build":  run a one-shot docker container to produce the file
//
// The method can be set globally via parameters.yaml (ensure_file_method).
// Prerequisites (filenames relative to data_dir) can be declared; Initialize()
// returns early until all prerequisites exist, relying on the using-retrigger
// mechanism to be called again once upstream Wants achieve.
type EnsureFileWant struct {
	Want
}

func (f *EnsureFileWant) GetLocals() *EnsureFileLocals {
	return CheckLocalsInitialized[EnsureFileLocals](&f.Want)
}

func (f *EnsureFileWant) targetPath() string {
	dataDir := f.GetStringParam("data_dir", "/tmp/ensure-file")
	filename := f.GetStringParam("filename", "")
	return filepath.Join(dataDir, filename)
}

func (f *EnsureFileWant) Initialize() {
	filename := f.GetStringParam("filename", "")
	if filename == "" {
		f.SetCurrent("ensure_file_phase", "failed")
		f.SetCurrent("ensure_file_error", "filename parameter is required")
		return
	}

	dataDir := f.GetStringParam("data_dir", "/tmp/ensure-file")
	target := filepath.Join(dataDir, filename)

	if fileExists(target) {
		f.SetCurrent("ensure_file_phase", "done")
		f.SetCurrent("ensure_file_path", target)
		f.StoreLog("[ENSURE-FILE] Already exists: %s — skipping", target)
		return
	}

	// Check prerequisites (filenames relative to data_dir).
	for _, prereq := range f.parsePrerequisites() {
		p := filepath.Join(dataDir, prereq)
		if !fileExists(p) {
			f.StoreLog("[ENSURE-FILE] Waiting for prerequisite: %s", prereq)
			return
		}
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		f.SetCurrent("ensure_file_phase", "failed")
		f.SetCurrent("ensure_file_error", fmt.Sprintf("mkdir %s: %v", dataDir, err))
		return
	}

	f.SetCurrent("ensure_file_phase", "starting")
	f.SetCurrent("ensure_file_path", target)

	method := f.GetStringParam("method", "curl")
	switch method {
	case "curl":
		f.runCurl(target)
	case "docker_wget":
		f.runDockerWget(dataDir, filename)
	case "docker_build":
		f.runDockerBuild(dataDir)
	default:
		f.SetCurrent("ensure_file_phase", "failed")
		f.SetCurrent("ensure_file_error", fmt.Sprintf("unknown method %q (use: curl, docker_wget, docker_build)", method))
	}
}

// runCurl downloads a file using Go's net/http — no docker required.
func (f *EnsureFileWant) runCurl(target string) {
	url := f.GetStringParam("url", "")
	if url == "" {
		f.SetCurrent("ensure_file_phase", "failed")
		f.SetCurrent("ensure_file_error", "url parameter is required for curl method")
		return
	}

	f.StoreLog("[ENSURE-FILE] Downloading %s (curl)", url)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		f.SetCurrent("ensure_file_phase", "failed")
		f.SetCurrent("ensure_file_error", fmt.Sprintf("build request: %v", err))
		return
	}
	req.Header.Set("User-Agent", "mywant-ensure-file/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		f.SetCurrent("ensure_file_phase", "failed")
		f.SetCurrent("ensure_file_error", fmt.Sprintf("http get: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		f.SetCurrent("ensure_file_phase", "failed")
		f.SetCurrent("ensure_file_error", fmt.Sprintf("http status %d", resp.StatusCode))
		return
	}

	out, err := os.Create(target)
	if err != nil {
		f.SetCurrent("ensure_file_phase", "failed")
		f.SetCurrent("ensure_file_error", fmt.Sprintf("create file: %v", err))
		return
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		f.SetCurrent("ensure_file_phase", "failed")
		f.SetCurrent("ensure_file_error", fmt.Sprintf("write file: %v", err))
		return
	}

	f.SetCurrent("ensure_file_phase", "done")
	f.StoreLog("[ENSURE-FILE] Downloaded: %s", target)
}

// runDockerWget downloads a file via docker alpine/wget.
func (f *EnsureFileWant) runDockerWget(dataDir, filename string) {
	url := f.GetStringParam("url", "")
	if url == "" {
		f.SetCurrent("ensure_file_phase", "failed")
		f.SetCurrent("ensure_file_error", "url parameter is required for docker_wget method")
		return
	}

	containerName := fmt.Sprintf("ensure-file-%s", f.Metadata.Name)
	volume := fmt.Sprintf("[\"%s:/data\"]", dataDir)
	cmdArgs := fmt.Sprintf("[\"sh\", \"-c\", \"wget -O /data/%s '%s'\"]", filename, url)

	f.SetCurrent("docker_image", "alpine:latest")
	f.SetCurrent("docker_container_name", containerName)
	f.SetCurrent("docker_volumes", volume)
	f.SetCurrent("docker_command_args", cmdArgs)
	f.SetCurrent("docker_wait_for_exit", true)
	f.SetCurrent("docker_ports", "[]")
	f.SetCurrent("docker_env", "{}")

	f.StoreLog("[ENSURE-FILE] Downloading %s → %s (docker wget)", url, filepath.Join(dataDir, filename))
	if err := f.ExecuteAgents(); err != nil {
		f.SetCurrent("ensure_file_phase", "failed")
		f.SetCurrent("ensure_file_error", err.Error())
		return
	}
	if code := GetCurrent(f, "docker_exit_code", -1); code != 0 {
		f.SetCurrent("ensure_file_phase", "failed")
		f.SetCurrent("ensure_file_error", fmt.Sprintf("wget exited with code %d", code))
		return
	}
	f.SetCurrent("ensure_file_phase", "done")
	f.StoreLog("[ENSURE-FILE] Download complete: %s", filepath.Join(dataDir, filename))
}

// runDockerBuild runs a one-shot docker container to produce the target file.
func (f *EnsureFileWant) runDockerBuild(dataDir string) {
	image := f.GetStringParam("docker_image", "")
	if image == "" {
		f.SetCurrent("ensure_file_phase", "failed")
		f.SetCurrent("ensure_file_error", "docker_image parameter is required for docker_build method")
		return
	}

	containerName := f.GetStringParam("docker_container_name",
		fmt.Sprintf("ensure-file-build-%s", f.Metadata.Name))
	// Default volume mounts data_dir to /var/opentripplanner (OTP convention).
	volumes := f.GetStringParam("docker_volumes",
		fmt.Sprintf("[\"%s:/var/opentripplanner\"]", dataDir))
	cmdArgs := f.GetStringParam("docker_command_args", "[]")
	env := f.GetStringParam("docker_env", "{}")

	f.SetCurrent("docker_image", image)
	f.SetCurrent("docker_container_name", containerName)
	f.SetCurrent("docker_volumes", volumes)
	f.SetCurrent("docker_command_args", cmdArgs)
	f.SetCurrent("docker_env", env)
	f.SetCurrent("docker_wait_for_exit", true)
	f.SetCurrent("docker_ports", "[]")

	f.StoreLog("[ENSURE-FILE] Building via docker image=%s args=%s", image, cmdArgs)
	if err := f.ExecuteAgents(); err != nil {
		f.SetCurrent("ensure_file_phase", "failed")
		f.SetCurrent("ensure_file_error", err.Error())
		return
	}
	if code := GetCurrent(f, "docker_exit_code", -1); code != 0 {
		f.SetCurrent("ensure_file_phase", "failed")
		f.SetCurrent("ensure_file_error", fmt.Sprintf("docker build exited with code %d", code))
		return
	}
	f.SetCurrent("ensure_file_phase", "done")
	f.StoreLog("[ENSURE-FILE] Build complete: %s", f.GetStringParam("filename", ""))
}

func (f *EnsureFileWant) parsePrerequisites() []string {
	s := f.GetStringParam("prerequisites", "")
	if s == "" {
		return nil
	}
	var prereqs []string
	if err := json.Unmarshal([]byte(s), &prereqs); err != nil {
		return nil
	}
	return prereqs
}

func (f *EnsureFileWant) IsAchieved() bool {
	return fileExists(f.targetPath())
}

func (f *EnsureFileWant) CalculateAchievingPercentage() float64 {
	if f.IsAchieved() {
		return 100
	}
	phase := GetCurrent(f, "ensure_file_phase", "")
	if phase == "starting" {
		return 30
	}
	return 5
}

func (f *EnsureFileWant) Progress() {
	f.SetCurrent("achieving_percentage", f.CalculateAchievingPercentage())
}
