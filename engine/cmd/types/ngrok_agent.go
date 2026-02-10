package types

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"time"

	mywant "mywant/engine/src"
)

func init() {
	mywant.RegisterDoAgentType("ngrok_tunnel_manager",
		[]mywant.Capability{mywant.Cap("ngrok_tunnel_management")},
		manageNgrokTunnel)
}

// ngrokTunnelResponse represents the ngrok local API response
type ngrokTunnelResponse struct {
	Tunnels []struct {
		PublicURL string `json:"public_url"`
		Proto     string `json:"proto"`
		Config    struct {
			Addr string `json:"addr"`
		} `json:"config"`
	} `json:"tunnels"`
}

// manageNgrokTunnel handles ngrok tunnel start and stop based on want phase
func manageNgrokTunnel(ctx context.Context, want *mywant.Want) error {
	want.StoreLog("[AGENT] manageNgrokTunnel called for want %s", want.Metadata.Name)

	phase, ok := want.GetStateString("ngrok_phase", "")
	if !ok || phase == "" {
		want.StoreLog("[AGENT] No ngrok_phase set, returning")
		return nil
	}

	existingPID, _ := want.GetStateInt("ngrok_pid", 0)

	switch phase {
	case "starting":
		if existingPID == 0 {
			port := "8080"
			if p, ok := want.Spec.Params["port"]; ok {
				port = fmt.Sprintf("%v", p)
			}
			protocol := "http"
			if pr, ok := want.Spec.Params["protocol"]; ok {
				protocol = fmt.Sprintf("%v", pr)
			}

			pid, err := startNgrokTunnel(protocol, port, want)
			if err != nil {
				want.StoreLog("[ERROR] Failed to start ngrok: %v", err)
				return err
			}
			want.StoreState("ngrok_pid", pid)
			want.StoreLog("[INFO] Started ngrok with PID %d", pid)

			// Wait for ngrok API to become available then fetch public URL
			publicURL, err := waitForNgrokURL(ctx, want)
			if err != nil {
				want.StoreLog("[ERROR] Failed to get ngrok public URL: %v", err)
				// Kill the process since we can't get the URL
				if proc, findErr := os.FindProcess(pid); findErr == nil {
					proc.Signal(syscall.SIGTERM)
				}
				want.StoreState("ngrok_pid", 0)
				return err
			}
			want.StoreState("ngrok_public_url", publicURL)
			want.StoreLog("[INFO] Ngrok public URL: %s", publicURL)
		} else {
			want.StoreLog("[INFO] Ngrok already running with PID %d", existingPID)
		}

	case "stopping":
		if existingPID != 0 {
			err := stopNgrokTunnel(existingPID, want)
			if err != nil {
				want.StoreLog("[WARN] Failed to stop ngrok PID %d: %v", existingPID, err)
			} else {
				want.StoreLog("[INFO] Stopped ngrok PID %d", existingPID)
				want.StoreState("ngrok_pid", 0)
				want.StoreState("ngrok_public_url", "")
			}
		}
	}

	return nil
}

// startNgrokTunnel starts an ngrok tunnel process
func startNgrokTunnel(protocol, port string, want *mywant.Want) (int, error) {
	// Check if ngrok is available
	ngrokPath, err := exec.LookPath("ngrok")
	if err != nil {
		return 0, fmt.Errorf("ngrok not found in PATH: %w", err)
	}

	cmd := exec.Command(ngrokPath, protocol, port, "--log=stdout")
	cmd.Stdout = nil
	cmd.Stderr = nil

	// Set process group so we can kill it properly
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to start ngrok: %w", err)
	}

	pid := cmd.Process.Pid
	want.StoreLog("[INFO] Ngrok started: %s %s %s (PID: %d)", ngrokPath, protocol, port, pid)

	go func() {
		cmd.Wait()
	}()

	return pid, nil
}

// waitForNgrokURL polls the ngrok local API to get the public URL
func waitForNgrokURL(ctx context.Context, want *mywant.Want) (string, error) {
	const apiURL = "http://127.0.0.1:4040/api/tunnels"
	const maxRetries = 15
	const retryInterval = 500 * time.Millisecond

	client := &http.Client{Timeout: 2 * time.Second}

	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		resp, err := client.Get(apiURL)
		if err != nil {
			want.StoreLog("[DEBUG] Waiting for ngrok API (attempt %d/%d)...", i+1, maxRetries)
			time.Sleep(retryInterval)
			continue
		}

		var tunnelResp ngrokTunnelResponse
		if err := json.NewDecoder(resp.Body).Decode(&tunnelResp); err != nil {
			resp.Body.Close()
			time.Sleep(retryInterval)
			continue
		}
		resp.Body.Close()

		// Find the HTTPS tunnel (prefer https over http)
		for _, t := range tunnelResp.Tunnels {
			if t.Proto == "https" {
				return t.PublicURL, nil
			}
		}
		// Fall back to any tunnel
		if len(tunnelResp.Tunnels) > 0 {
			return tunnelResp.Tunnels[0].PublicURL, nil
		}

		time.Sleep(retryInterval)
	}

	return "", fmt.Errorf("timed out waiting for ngrok tunnel to become available")
}

// stopNgrokTunnel stops the ngrok process by PID
func stopNgrokTunnel(pid int, want *mywant.Want) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", pid, err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		want.StoreLog("[WARN] SIGTERM failed for PID %d, trying SIGKILL: %v", pid, err)
		if err := process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process %d: %w", pid, err)
		}
	}

	want.StoreLog("[INFO] Sent termination signal to ngrok PID %d", pid)
	return nil
}
