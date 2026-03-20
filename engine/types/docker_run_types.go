package types

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[DockerRunWant, DockerRunLocals]("docker_run")
}

// DockerRunLocals holds per-instance runtime state.
type DockerRunLocals struct {
	ContainerName string
	ContainerID   string
	Phase         string // starting | running | done | failed | stopped
}

// DockerRunWant manages a Docker container whose lifecycle mirrors the Want's lifecycle.
//
// Two modes:
//   - Server mode (default): container runs indefinitely; achieved once running
//     (+ optional health check). Stopped when the Want is deleted.
//   - Task mode (wait_for_exit: true): runs docker run, waits for the container
//     to exit 0, then achieves. Useful for one-shot jobs like OTP graph building.
type DockerRunWant struct {
	Want
}

func (d *DockerRunWant) GetLocals() *DockerRunLocals {
	return CheckLocalsInitialized[DockerRunLocals](&d.Want)
}

// Initialize starts the Docker container. Called once on Want creation (and on each `when` trigger).
func (d *DockerRunWant) Initialize() {
	locals := d.GetLocals()
	locals.ContainerID = ""
	locals.Phase = "starting"

	image := d.GetStringParam("image", "")
	if image == "" {
		d.dockerFail(locals, "image parameter is required")
		return
	}

	containerName := d.GetStringParam("container_name", fmt.Sprintf("mywant-%s", d.Metadata.Name))
	locals.ContainerName = containerName

	// Promote params → state so the agent can read them
	d.SetCurrent("docker_image", image)
	d.SetCurrent("docker_container_name", containerName)
	d.SetCurrent("docker_ports", d.GetStringParam("ports", "[]"))
	d.SetCurrent("docker_volumes", d.GetStringParam("volumes", "[]"))
	d.SetCurrent("docker_env", d.GetStringParam("env", "{}"))
	d.SetCurrent("docker_command_args", d.GetStringParam("command_args", "[]"))
	d.SetCurrent("docker_wait_for_exit", d.GetBoolParam("wait_for_exit", false))
	d.SetCurrent("docker_phase", "starting")

	d.DirectLog("[DOCKER] Starting container %s from image %s", containerName, image)

	if err := d.ExecuteAgents(); err != nil {
		d.dockerFail(locals, fmt.Sprintf("docker agent failed: %v", err))
		return
	}

	containerID := GetCurrent(d, "docker_container_id", "")
	if containerID == "" {
		d.dockerFail(locals, "docker agent did not return container ID")
		return
	}
	locals.ContainerID = containerID

	waitForExit := d.GetBoolParam("wait_for_exit", false)
	if waitForExit {
		// Agent already waited; check stored exit code
		exitCode := GetCurrent(d, "docker_exit_code", -1)
		if exitCode != 0 {
			d.dockerFail(locals, fmt.Sprintf("container exited with code %d", exitCode))
			return
		}
		locals.Phase = "done"
		d.SetCurrent("docker_phase", "done")
		d.DirectLog("[DOCKER] Task container %s finished successfully", containerName)
		return
	}

	// Server mode: optional health check
	healthURL := d.GetStringParam("health_check_url", "")
	if healthURL != "" {
		interval := d.GetStringParam("health_check_interval", "5s")
		maxRetries := d.GetIntParam("health_check_max_retries", 60)
		d.SetCurrent("server_health_check_url", healthURL)
		d.SetCurrent("server_health_check_interval", interval)
		d.SetCurrent("server_health_check_max_retries", maxRetries)

		if _, err := waitForHealthCheck(context.Background(), &d.Want, healthURL); err != nil {
			d.dockerFail(locals, fmt.Sprintf("health check failed: %v", err))
			return
		}
	}

	locals.Phase = "running"
	d.SetCurrent("docker_phase", "running")
	d.DirectLog("[DOCKER] Container %s is running (ID: %s)", containerName, containerID)
}

func (d *DockerRunWant) IsAchieved() bool {
	phase := GetCurrent(d, "docker_phase", "")
	return phase == "running" || phase == "done"
}

func (d *DockerRunWant) CalculateAchievingPercentage() float64 {
	if d.IsAchieved() {
		return 100
	}
	phase := GetCurrent(d, "docker_phase", "")
	if phase == "starting" {
		return 20
	}
	return 0
}

func (d *DockerRunWant) Progress() {
	d.SetCurrent("achieving_percentage", d.CalculateAchievingPercentage())

	// In server mode, poll container status each tick to detect unexpected exits.
	phase := GetCurrent(d, "docker_phase", "")
	if phase != "running" {
		return
	}

	containerName := GetCurrent(d, "docker_container_name", "")
	if containerName == "" {
		return
	}

	status, exitCode := dockerContainerStatus(containerName)
	d.SetCurrent("docker_container_status", status)

	if status == "exited" || status == "dead" {
		logs := dockerTailLogs(containerName, 30)
		d.SetCurrent("docker_logs", logs)
		d.SetCurrent("docker_exit_code", exitCode)
		d.SetCurrent("docker_phase", "exited")
		d.SetCurrent("docker_error", fmt.Sprintf("container %s exited unexpectedly (code %d)", containerName, exitCode))
		d.StoreLog("[DOCKER] Container %s exited (code %d). Last logs:\n%s", containerName, exitCode, logs)
	}
}

// OnDelete stops and removes the container when the Want is deleted.
func (d *DockerRunWant) OnDelete() {
	containerName := GetCurrent(d, "docker_container_name", "")
	if containerName == "" {
		containerName = d.GetLocals().ContainerName
	}
	if containerName == "" {
		return
	}
	d.StoreLog("[DOCKER] Stopping container %s", containerName)
	exec.Command("docker", "stop", containerName).Run()  //nolint:errcheck
	exec.Command("docker", "rm", containerName).Run()    //nolint:errcheck
	d.SetCurrent("docker_phase", "stopped")
}

func (d *DockerRunWant) dockerFail(locals *DockerRunLocals, msg string) {
	d.DirectLog("[DOCKER ERROR] %s", msg)
	locals.Phase = "failed"
	d.SetCurrent("docker_phase", "failed")
	d.SetCurrent("docker_error", msg)
	d.Status = "failed"
}

// dockerShortID returns the first 12 chars of a container ID.
func dockerShortID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
