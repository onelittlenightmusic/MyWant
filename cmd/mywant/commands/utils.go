package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"mywant/pkg/client"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// getMyWantDir returns ~/.mywant directory path and creates it if needed
func getMyWantDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".mywant"
	}
	dir := filepath.Join(home, ".mywant")
	os.MkdirAll(dir, 0755)
	return dir
}

var (
	pidFile    = filepath.Join(getMyWantDir(), "server.pid")
	guiPidFile = filepath.Join(getMyWantDir(), "gui.pid")
)

// isPortOpen checks if a port is in use
func isPortOpen(port int) bool {
	cmd := exec.Command("lsof", "-t", fmt.Sprintf("-i:%d", port))
	output, err := cmd.Output()
	return err == nil && len(output) > 0
}

// killProcessOnPort finds and kills processes listening on the given port
func killProcessOnPort(port int) bool {
	// Use lsof to find the PID
	cmd := exec.Command("lsof", "-t", fmt.Sprintf("-i:%d", port))
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		return false
	}

	pids := strings.Split(strings.TrimSpace(string(output)), "\n")
	killedAny := false
	for _, pidStr := range pids {
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		if pid == os.Getpid() {
			continue // Don't kill ourselves
		}

		process, err := os.FindProcess(pid)
		if err == nil {
			process.Kill()
			killedAny = true
		}
	}
	return killedAny
}

var LogsCmd = &cobra.Command{
	Use:     "logs",
	Aliases: []string{"l"},
	Short:   "View system logs",
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		resp, err := c.GetLogs()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "Timestamp\tMethod\tEndpoint\tStatus\tDetails")

		for _, log := range resp.Logs {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				log.Timestamp,
				log.Method,
				log.Endpoint,
				fmt.Sprintf("%s (%d)", log.Status, log.StatusCode),
				log.Details,
			)
		}
		w.Flush()
	},
}
