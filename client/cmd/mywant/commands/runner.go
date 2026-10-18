package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	mywant "mywant/engine/core"

	"github.com/spf13/cobra"
)

// The runner is where plugin scripts run, shut in one call at a time (see
// engine/core/mrs_sandbox.go). On a Mac it is a Docker container, because the
// jail is Linux namespaces; on fly it is a container of the Machine.
//
//	mywant runner start    the container, and ~/.mywant/runner.json for the engine
//	mywant runner stop     both gone; scripts run in the engine again
//	mywant runner status   where it is and whether it answers
//	mywant runner serve    what the container runs

const (
	runnerContainer    = "mywant-runner"
	runnerDefaultImage = "ghcr.io/onelittlenightmusic/mywant-backend:latest"
	runnerDefaultPort  = 8090
)

var RunnerCmd = &cobra.Command{
	Use:   "runner",
	Short: "Run plugin scripts in a sandbox container",
}

var runnerServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Be the runner (inside its container)",
	RunE: func(cmd *cobra.Command, args []string) error {
		host, _ := cmd.Flags().GetString("host")
		port, _ := cmd.Flags().GetInt("port")
		noJail, _ := cmd.Flags().GetBool("no-jail")
		expose, _ := cmd.Flags().GetStringArray("expose")
		apiURL, _ := cmd.Flags().GetString("api-url")
		return mywant.ServeRunner(mywant.RunnerOptions{
			Host: host, Port: port, NoJail: noJail, Expose: expose, APIURL: apiURL,
		})
	},
}

var runnerStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the runner container and send plugin scripts to it",
	RunE: func(cmd *cobra.Command, args []string) error {
		image, _ := cmd.Flags().GetString("image")
		port, _ := cmd.Flags().GetInt("port")
		extra, _ := cmd.Flags().GetStringArray("expose")

		if _, err := exec.LookPath("docker"); err != nil {
			return fmt.Errorf("docker is not installed")
		}
		if out, err := exec.Command("docker", "info", "--format", "{{.ServerVersion}}").CombinedOutput(); err != nil {
			return fmt.Errorf("docker is not running (start Docker Desktop): %s", strings.TrimSpace(string(out)))
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		apiPort := 8080
		if config, _ := LoadConfig(); config != nil && config.ServerPort > 0 {
			apiPort = config.ServerPort
		}

		_ = exec.Command("docker", "rm", "-f", runnerContainer).Run()
		dockerArgs := runnerDockerArgs(home, image, port, apiPort, runnerMounts(home, extra))
		out, err := exec.Command("docker", dockerArgs...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("docker run: %v: %s", err, strings.TrimSpace(string(out)))
		}

		url := fmt.Sprintf("http://127.0.0.1:%d", port)
		if err := waitRunner(url, 30*time.Second); err != nil {
			logs, _ := exec.Command("docker", "logs", "--tail", "20", runnerContainer).CombinedOutput()
			return fmt.Errorf("%v\n%s", err, logs)
		}
		data, _ := json.MarshalIndent(mywant.MRSRunnerConfig{URL: url}, "", "  ")
		if err := os.WriteFile(runnerFile(), append(data, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Printf("Runner started at %s (%s). Plugin scripts now run there.\n", url, image)
		return nil
	},
}

var runnerStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the runner container; plugin scripts run in the engine again",
	RunE: func(cmd *cobra.Command, args []string) error {
		_ = os.Remove(runnerFile()) // first, so no call is sent to a runner going away
		out, err := exec.Command("docker", "rm", "-f", runnerContainer).CombinedOutput()
		if err != nil && !strings.Contains(string(out), "No such container") {
			return fmt.Errorf("docker rm: %v: %s", err, strings.TrimSpace(string(out)))
		}
		fmt.Println("Runner stopped.")
		return nil
	},
}

var runnerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show where plugin scripts run",
	Run: func(cmd *cobra.Command, args []string) {
		url := mywant.MRSRunnerURL()
		if url == "" {
			fmt.Println("No runner: plugin scripts run in the engine, unsandboxed.")
			return
		}
		if err := waitRunner(url, 0); err != nil {
			fmt.Printf("Runner %s is not answering: %v\n", url, err)
			return
		}
		fmt.Printf("Runner %s is answering.\n", url)
	},
}

func runnerFile() string {
	return filepath.Join(mywant.MyWantHomeDir(), mywant.MRSRunnerFile)
}

// runnerMounts is what the container must see, each at the path the engine
// knows it by: ~/.mywant, and the directories its plugins are links to or
// look for outside it (a checkout under ~/work). All read-only but ~/.mywant,
// which the runner needs writable for plugin-data; what a call may write is
// decided by the jail, not by the mount.
func runnerMounts(home string, extra []string) []string {
	mywantDir := filepath.Join(home, ".mywant")
	seen := map[string]bool{}
	var dirs []string
	add := func(p string) {
		p = filepath.Clean(p)
		if seen[p] || strings.HasPrefix(p+string(filepath.Separator), mywantDir+string(filepath.Separator)) {
			return
		}
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			seen[p] = true
			dirs = append(dirs, p)
		}
	}
	for _, root := range []string{"custom-types", "customs"} {
		entries, _ := os.ReadDir(filepath.Join(mywantDir, root))
		for _, e := range entries {
			if real, err := filepath.EvalSymlinks(filepath.Join(mywantDir, root, e.Name())); err == nil {
				add(real)
			}
		}
	}
	// mrs_browser's plugins look for a development checkout here first.
	add(filepath.Join(home, "work", "machine-readable-skills-browser"))
	for _, p := range extra {
		if abs, err := filepath.Abs(p); err == nil {
			add(abs)
		}
	}
	sort.Strings(dirs)
	return dirs
}

func runnerDockerArgs(home, image string, port, apiPort int, mounts []string) []string {
	mywantDir := filepath.Join(home, ".mywant")
	args := []string{"run", "-d", "--name", runnerContainer, "--restart", "unless-stopped",
		"-p", fmt.Sprintf("127.0.0.1:%d:%d", port, runnerDefaultPort),
		"-e", "HOME=" + home,
		"-v", mywantDir + ":" + mywantDir,
		// unshare and mount, which the jail is made of; Docker's defaults
		// forbid both.
		"--cap-add", "SYS_ADMIN",
		"--security-opt", "seccomp=unconfined",
		"--security-opt", "apparmor=unconfined",
	}
	if tz := os.Getenv("TZ"); tz != "" {
		args = append(args, "-e", "TZ="+tz)
	}
	for _, m := range mounts {
		args = append(args, "-v", m+":"+m+":ro")
	}
	args = append(args, "--entrypoint", "mywant", image,
		"runner", "serve", "--host", "0.0.0.0", "--port", fmt.Sprint(runnerDefaultPort),
		// From inside the container the engine on this Mac is the host.
		"--api-url", fmt.Sprintf("http://host.docker.internal:%d", apiPort))
	for _, m := range mounts {
		args = append(args, "--expose", m)
	}
	return args
}

// waitRunner asks the runner's /health until it answers or wait runs out.
func waitRunner(url string, wait time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(wait)
	for {
		resp, err := client.Get(url + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			err = fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func init() {
	runnerServeCmd.Flags().String("host", "127.0.0.1", "Address to listen on")
	runnerServeCmd.Flags().Int("port", runnerDefaultPort, "Port to listen on")
	runnerServeCmd.Flags().Bool("no-jail", false, "Fork each call without shutting it in (for a machine without Linux namespaces; never deploy this)")
	runnerServeCmd.Flags().StringArray("expose", nil, "A directory plugin scripts may also read (repeatable)")
	runnerServeCmd.Flags().String("api-url", "", "MYWANT_URL as seen from inside the runner")

	runnerStartCmd.Flags().String("image", runnerDefaultImage, "Image to run (one with this mywant in it)")
	runnerStartCmd.Flags().Int("port", runnerDefaultPort, "Port on this machine for the runner")
	runnerStartCmd.Flags().StringArray("expose", nil, "Another directory plugin scripts may read (repeatable)")

	RunnerCmd.AddCommand(runnerServeCmd, runnerStartCmd, runnerStopCmd, runnerStatusCmd)
}
