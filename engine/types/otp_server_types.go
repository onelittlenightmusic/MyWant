package types

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[OtpServerWant, OtpServerLocals]("otp_server")
}

type OtpServerLocals struct{}

// OtpServerWant starts the OTP routing server and waits for it to become healthy.
// IsAchieved returns true once docker_phase == "running".
// The container is stopped and removed when the Want is deleted.
type OtpServerWant struct {
	Want
}

func (o *OtpServerWant) GetLocals() *OtpServerLocals {
	return CheckLocalsInitialized[OtpServerLocals](&o.Want)
}

func (o *OtpServerWant) Initialize() {
	// Idempotent: if already running, skip.
	if GetCurrent(o, "docker_phase", "") == "running" {
		o.StoreLog("[OTP-SERVER] Container already running — skipping start")
		return
	}

	dataDir := o.GetStringParam("data_dir", "/tmp/otp-data")

	// Guard: graph.obj must exist before starting the server.
	graphPath := filepath.Join(dataDir, "graph.obj")
	if !fileExists(graphPath) {
		o.StoreLog("[OTP-SERVER] Waiting for graph.obj: %s", graphPath)
		return
	}

	// Remove any stopped container with the same name to avoid "name already in use" errors.
	exec.Command("docker", "rm", "-f", "otp-server").Run() //nolint:errcheck

	serverMem := o.GetStringParam("server_memory", "-Xmx4G")
	serverPort := o.GetStringParam("server_port", "8080")

	volume := fmt.Sprintf("[\"%s:/var/opentripplanner\"]", dataDir)
	ports := fmt.Sprintf("[\"%s:8080\"]", serverPort)
	healthURL := fmt.Sprintf("http://localhost:%s/otp", serverPort)

	o.SetCurrent("docker_image", "docker.io/opentripplanner/opentripplanner:2.6.0")
	o.SetCurrent("docker_container_name", "otp-server")
	o.SetCurrent("docker_volumes", volume)
	o.SetCurrent("docker_ports", ports)
	o.SetCurrent("docker_env", fmt.Sprintf("{\"JAVA_OPTIONS\": \"%s\"}", serverMem))
	o.SetCurrent("docker_command_args", "[\"--load\", \"--serve\"]")
	o.SetCurrent("docker_wait_for_exit", false)
	o.SetCurrent("docker_phase", "starting")
	o.SetCurrent("server_health_check_url", healthURL)
	o.SetCurrent("server_health_check_interval", "5s")
	o.SetCurrent("server_health_check_max_retries", 120)

	o.StoreLog("[OTP-SERVER] Starting OTP server on port %s", serverPort)
	if err := o.ExecuteAgents(); err != nil {
		o.SetCurrent("docker_phase", "failed")
		o.SetCurrent("docker_error", err.Error())
		return
	}

	containerID := GetCurrent(o, "docker_container_id", "")
	if containerID == "" {
		o.SetCurrent("docker_phase", "failed")
		o.SetCurrent("docker_error", "docker agent did not return container ID")
		return
	}

	if _, err := waitForHealthCheck(context.Background(), &o.Want, healthURL); err != nil {
		o.SetCurrent("docker_phase", "failed")
		o.SetCurrent("docker_error", fmt.Sprintf("health check failed: %v", err))
		return
	}

	o.SetCurrent("docker_phase", "running")
	o.StoreLog("[OTP-SERVER] OTP server is up at %s", healthURL)
}

func (o *OtpServerWant) IsAchieved() bool {
	return GetCurrent(o, "docker_phase", "") == "running"
}

func (o *OtpServerWant) CalculateAchievingPercentage() float64 {
	if o.IsAchieved() {
		return 100
	}
	phase := GetCurrent(o, "docker_phase", "")
	if phase == "starting" {
		return 30
	}
	return 5
}

func (o *OtpServerWant) Progress() {
	o.SetCurrent("achieving_percentage", o.CalculateAchievingPercentage())

	// If docker_phase is empty, Initialize() returned early (graph.obj wasn't ready).
	// Re-try initialization now — graph.obj may have appeared since then.
	if GetCurrent(o, "docker_phase", "") == "" {
		o.Initialize()
		return
	}

	if GetCurrent(o, "docker_phase", "") != "running" {
		return
	}

	containerName := GetCurrent(o, "docker_container_name", "otp-server")
	status, exitCode := dockerContainerStatus(containerName)
	o.SetCurrent("docker_container_status", status)

	if status == "exited" || status == "dead" {
		logs := dockerTailLogs(containerName, 30)
		o.SetCurrent("docker_logs", logs)
		o.SetCurrent("docker_exit_code", exitCode)
		o.SetCurrent("docker_phase", "exited")
		o.SetCurrent("docker_error", fmt.Sprintf("container %s exited unexpectedly (code %d)", containerName, exitCode))
		o.StoreLog("[OTP-SERVER] Container %s exited (code %d). Last logs:\n%s", containerName, exitCode, logs)
	}
}

// OnDelete stops and removes the OTP server container when the Want is deleted.
func (o *OtpServerWant) OnDelete() {
	containerName := GetCurrent(o, "docker_container_name", "otp-server")
	o.StoreLog("[OTP-SERVER] Stopping container %s", containerName)
	exec.Command("docker", "stop", containerName).Run()  //nolint:errcheck
	exec.Command("docker", "rm", containerName).Run()    //nolint:errcheck
	o.SetCurrent("docker_phase", "stopped")
}
