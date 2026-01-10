package commands

import (
	"fmt"
	"os"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"
)

var PsCmd = &cobra.Command{
	Use:   "ps",
	Short: "Show status of MyWant processes (Server, GUI, Mock)",
	Run: func(cmd *cobra.Command, args []string) {
		serverPort, _ := cmd.Flags().GetInt("server-port")
		guiPort, _ := cmd.Flags().GetInt("gui-port")
		mockPort, _ := cmd.Flags().GetInt("mock-port")

		fmt.Println("MyWant Process Status:")
		fmt.Println("----------------------")

		// 1. Check Server
		checkProcessStatus("Backend Server", pidFile, serverPort)

		// 2. Check GUI
		checkProcessStatus("Frontend GUI  ", guiPidFile, guiPort)

		// 3. Check Mock Server
		checkProcessStatus("Mock Flight   ", "", mockPort)
	},
}

func checkProcessStatus(label string, pidFileName string, port int) {
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

	fmt.Printf("%s:\n", label)
	fmt.Printf("  Port:    %d\n", port)
	if running {
		fmt.Printf("  Status:  RUNNING (Background)\n")
		fmt.Printf("  PID:     %d\n", pid)
	} else if portInUse {
		fmt.Printf("  Status:  RUNNING (Active on port)\n")
	} else {
		fmt.Printf("  Status:  STOPPED\n")
	}
	fmt.Println()
}

func init() {
	PsCmd.Flags().Int("server-port", 8080, "Backend server port")
	PsCmd.Flags().Int("gui-port", 3000, "Frontend GUI port")
	PsCmd.Flags().Int("mock-port", 8081, "Mock flight server port")
}