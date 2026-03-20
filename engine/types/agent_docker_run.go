package types

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	mywant "mywant/engine/core"
)

func init() {
	mywant.RegisterDoAgent("docker_manager", executeDockerRun)
}

// executeDockerRun is the DoAgent that starts (or stops) a Docker container.
// It reads configuration from state (set by DockerRunWant.Initialize()).
func executeDockerRun(ctx context.Context, want *mywant.Want) error {
	phase := mywant.GetCurrent(want, "docker_phase", "")
	switch phase {
	case "starting":
		return dockerStart(ctx, want)
	case "stopping":
		return dockerStop(ctx, want)
	default:
		return nil
	}
}

func dockerStart(ctx context.Context, want *mywant.Want) error {
	image := mywant.GetCurrent(want, "docker_image", "")
	if image == "" {
		return fmt.Errorf("docker_image not set in state")
	}
	containerName := mywant.GetCurrent(want, "docker_container_name", "")

	// Remove any stale container with the same name
	exec.CommandContext(ctx, "docker", "rm", "-f", containerName).Run() //nolint:errcheck

	args := []string{"run", "-d", "--name", containerName}

	// Ports: JSON array of "host:container" strings
	if ports := mywant.GetCurrent(want, "docker_ports", "[]"); ports != "[]" {
		var portList []string
		if err := json.Unmarshal([]byte(ports), &portList); err == nil {
			for _, p := range portList {
				args = append(args, "-p", p)
			}
		}
	}

	// Volumes: JSON array of "host:container" strings
	if volumes := mywant.GetCurrent(want, "docker_volumes", "[]"); volumes != "[]" {
		var volList []string
		if err := json.Unmarshal([]byte(volumes), &volList); err == nil {
			for _, v := range volList {
				args = append(args, "-v", v)
			}
		}
	}

	// Env: JSON object {"KEY": "VALUE", ...}
	if env := mywant.GetCurrent(want, "docker_env", "{}"); env != "{}" {
		var envMap map[string]string
		if err := json.Unmarshal([]byte(env), &envMap); err == nil {
			for k, v := range envMap {
				args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
			}
		}
	}

	// Image
	args = append(args, image)

	// Container command args: JSON array appended after image
	if cmdArgs := mywant.GetCurrent(want, "docker_command_args", "[]"); cmdArgs != "[]" {
		var cmdList []string
		if err := json.Unmarshal([]byte(cmdArgs), &cmdList); err == nil {
			args = append(args, cmdList...)
		}
	}

	want.DirectLog("[DOCKER] Running: docker %s", strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("docker run failed: %w", err)
	}

	containerID := dockerShortID(string(out))
	want.SetCurrent("docker_container_id", containerID)
	want.DirectLog("[DOCKER] Container started: %s (ID: %s)", containerName, containerID)

	// Task mode: wait for container to finish
	waitForExit := mywant.GetCurrent(want, "docker_wait_for_exit", false)
	if waitForExit {
		want.DirectLog("[DOCKER] Waiting for container %s to finish...", containerName)
		waitCmd := exec.CommandContext(ctx, "docker", "wait", containerName)
		exitOut, err := waitCmd.Output()
		exitCode := 0
		if err != nil {
			exitCode = 1
		} else {
			exitCode, _ = strconv.Atoi(strings.TrimSpace(string(exitOut)))
		}
		want.SetCurrent("docker_exit_code", exitCode)
		want.DirectLog("[DOCKER] Container %s exited with code %d", containerName, exitCode)
	}

	return nil
}

// dockerContainerStatus returns the container status string and exit code.
// Status values: "running", "exited", "dead", "created", "paused", "" (not found).
func dockerContainerStatus(containerName string) (status string, exitCode int) {
	out, err := exec.Command("docker", "inspect",
		"--format", "{{.State.Status}} {{.State.ExitCode}}",
		containerName).Output()
	if err != nil {
		return "", 0
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) == 0 {
		return "", 0
	}
	status = parts[0]
	if len(parts) >= 2 {
		fmt.Sscanf(parts[1], "%d", &exitCode)
	}
	return status, exitCode
}

// dockerTailLogs returns the last n lines of docker container logs.
func dockerTailLogs(containerName string, lines int) string {
	out, err := exec.Command("docker", "logs", "--tail", fmt.Sprintf("%d", lines), containerName).CombinedOutput()
	if err != nil && len(out) == 0 {
		return fmt.Sprintf("(failed to get logs: %v)", err)
	}
	return strings.TrimSpace(string(out))
}

func dockerStop(ctx context.Context, want *mywant.Want) error {
	containerName := mywant.GetCurrent(want, "docker_container_name", "")
	if containerName == "" {
		return nil
	}
	exec.CommandContext(ctx, "docker", "stop", containerName).Run() //nolint:errcheck
	exec.CommandContext(ctx, "docker", "rm", containerName).Run()   //nolint:errcheck
	want.SetCurrent("docker_phase", "stopped")
	return nil
}
