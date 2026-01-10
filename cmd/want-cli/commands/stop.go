package commands

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var StopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the MyWant server",
	Run: func(cmd *cobra.Command, args []string) {
		port, _ := cmd.Flags().GetInt("port")

		// 1. Try stopping via PID file
		data, err := os.ReadFile(pidFile)
		if err == nil {
			pid, err := strconv.Atoi(string(data))
			if err == nil {
				process, err := os.FindProcess(pid)
				if err == nil {
					fmt.Printf("Stopping MyWant Server (PID: %d)...\n", pid)
					// Try SIGTERM first
					process.Signal(syscall.SIGTERM)

					// Wait a moment and check if it's still alive
					time.Sleep(1 * time.Second)
					if err := process.Signal(syscall.Signal(0)); err == nil {
						// Still alive, force kill
						fmt.Println("Process still alive, sending SIGKILL...")
						process.Kill()
					}
				}
			}
			os.Remove(pidFile)
		}

		// 2. Fallback: Kill anything on the port
		fmt.Printf("Ensuring port %d is free...\n", port)
		if killed := killProcessOnPort(port); killed {
			fmt.Printf("Terminated lingering process on port %d\n", port)
		}

		fmt.Println("Cleanup complete.")
	},
}

func init() {
	StopCmd.Flags().IntP("port", "p", 8080, "Port to stop")
}
