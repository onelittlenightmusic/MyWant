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
	Use:     "stop",
	Aliases: []string{"st"},
	Short:   "Stop the MyWant server",
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

					// Wait for graceful shutdown (up to 12 seconds)
					fmt.Print("Waiting for graceful shutdown...")
					for i := 0; i < 12; i++ {
						time.Sleep(1 * time.Second)
						if err := process.Signal(syscall.Signal(0)); err != nil {
							// Process is gone
							fmt.Println(" Done.")
							break
						}
						fmt.Print(".")
						if i == 11 {
							// Still alive after 12 seconds, force kill
							fmt.Println("\nProcess still alive after timeout, sending SIGKILL...")
							process.Kill()
						}
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
