package commands

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var PsCmd = &cobra.Command{
	Use:   "ps",
	Short: "Show status of MyWant processes (Server, GUI, Mock)",
	Run: func(cmd *cobra.Command, args []string) {
		serverPort, _ := cmd.Flags().GetInt("server-port")
		guiPort, _ := cmd.Flags().GetInt("gui-port")
		mockPort, _ := cmd.Flags().GetInt("mock-port")

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "NAME\tPORT\tSTATUS\tPID")

		// 1. Check Server
		name, port, status, pid := getProcessStatus("Backend Server", pidFile, serverPort)
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", name, port, status, pid)

		// 2. Check GUI
		name, port, status, pid = getProcessStatus("Frontend GUI", guiPidFile, guiPort)
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", name, port, status, pid)

		// 3. Check Mock Server
		name, port, status, pid = getProcessStatus("Mock Flight", "", mockPort)
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", name, port, status, pid)

		w.Flush()
	},
}

func getProcessStatus(label string, pidFileName string, port int) (string, int, string, string) {
	running := false
	pid := 0

	// Check PID file if provided
	if pidFileName != "" {
		data, err := os.ReadFile(pidFileName)
		if err == nil {
			p, err := strconv.Atoi(string(data))
			if err == nil {
				if process, err := os.FindProcess(p); err == nil {
					// On Unix, findProcess always succeeds, so we need to check with signal 0
					if err := process.Signal(syscall.Signal(0)); err == nil {
						running = true
						pid = p
					}
				}
			}
		}
	}

	// Also check port
	portInUse := isPortOpen(port)

	status := "STOPPED"
	pidStr := "-"

	if running {
		status = "RUNNING"
		pidStr = strconv.Itoa(pid)
	} else if portInUse {
		status = "RUNNING (Active)"
	}

	return label, port, status, pidStr
}

func init() {
	PsCmd.Flags().Int("server-port", 8080, "Backend server port")
	PsCmd.Flags().Int("gui-port", 8080, "Frontend GUI port")
	PsCmd.Flags().Int("mock-port", 8081, "Mock flight server port")
}
