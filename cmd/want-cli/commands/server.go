package commands

import (
		"time"
		"fmt"
		"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"mywant/pkg/server"

	"github.com/spf13/cobra"
)

const pidFile = "server.pid"

var ServerCmd = &cobra.Command{
	Use:   "server",
	Short: "Manage the MyWant server",
	Long:  `Start and manage the MyWant backend API server directly from the CLI.`,
}

var serverStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the API server",
	Run: func(cmd *cobra.Command, args []string) {
		port, _ := cmd.Flags().GetInt("port")
		host, _ := cmd.Flags().GetString("host")
		debug, _ := cmd.Flags().GetBool("debug")
		detach, _ := cmd.Flags().GetBool("detach")

		// 1. Guard: Check if already running (for detached mode)
		if detach {
			// Check PID file
			data, err := os.ReadFile(pidFile)
			if err == nil {
				pid, err := strconv.Atoi(string(data))
				if err == nil {
					if process, err := os.FindProcess(pid); err == nil {
						// Check if process is actually running
						if err := process.Signal(syscall.Signal(0)); err == nil {
							fmt.Printf("Error: MyWant Server is already running (PID: %d). Stop it first.\n", pid)
							os.Exit(1)
						}
					}
				}
			}

			// Check Port
			if isPortOpen(port) {
				fmt.Printf("Error: Port %d is already in use. Stop the existing process or use a different port.\n", port)
				os.Exit(1)
			}
		}

		// Create logs directory
		logDir := "logs"
		if err := os.MkdirAll(logDir, 0755); err != nil {
			fmt.Printf("Failed to create logs directory: %v\n", err)
			os.Exit(1)
		}

		if detach {
			// Prepare background execution
			executable, _ := os.Executable()

			// Build arguments for re-execution (excluding --detach)
			newArgs := []string{"server", "start"}
			newArgs = append(newArgs, "--port", strconv.Itoa(port))
			newArgs = append(newArgs, "--host", host)
			if debug {
				newArgs = append(newArgs, "--debug")
			}

			logFilePath := filepath.Join(logDir, "server.log")
			logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				fmt.Printf("Failed to open log file: %v\n", err)
				os.Exit(1)
			}

			process := exec.Command(executable, newArgs...)
			process.Stdout = logFile
			process.Stderr = logFile

			err = process.Start()
			if err != nil {
				fmt.Printf("Failed to start background process: %v\n", err)
				os.Exit(1)
			}

			// Save PID
			err = os.WriteFile(pidFile, []byte(strconv.Itoa(process.Process.Pid)), 0644)
			if err != nil {
				fmt.Printf("Warning: Failed to write PID file: %v\n", err)
			}

			fmt.Printf("MyWant Server started in background (PID: %d)\n", process.Process.Pid)
			fmt.Printf("Logs: %s\n", logFilePath)
			os.Exit(0)
		}

		cfg := server.Config{
			Port:  port,
			Host:  host,
			Debug: debug,
		}

		fmt.Printf("Starting MyWant Server on %s:%d (debug=%v)...\n", host, port, debug)
		s := server.New(cfg)
		if err := s.Start(); err != nil {
			fmt.Printf("Server error: %v\n", err)
			os.Exit(1)
		}
	},
}

var serverPsCmd = &cobra.Command{
	Use:   "ps",
	Short: "Show server process status",
	Run: func(cmd *cobra.Command, args []string) {
		port, _ := cmd.Flags().GetInt("port")

		running := false
		pid := 0

		// Check PID file
		data, err := os.ReadFile(pidFile)
		if err == nil {
			p, err := strconv.Atoi(string(data))
			if err == nil {
				if process, err := os.FindProcess(p); err == nil {
					if err := process.Signal(syscall.Signal(0)); err == nil {
						running = true
						pid = p
					}
				}
			}
		}

		// Also check port
		portInUse := isPortOpen(port)

		fmt.Println("MyWant Server Status:")
		fmt.Printf("  Port:    %d\n", port)
		if running {
			fmt.Printf("  Status:  RUNNING (Background)\n")
			fmt.Printf("  PID:     %d\n", pid)
		} else if portInUse {
			fmt.Printf("  Status:  RUNNING (Active on port but PID file missing or invalid)\n")
		} else {
			fmt.Printf("  Status:  STOPPED\n")
		}
	},
}

var serverStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the background API server",
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
	ServerCmd.AddCommand(serverStartCmd)
	ServerCmd.AddCommand(serverStopCmd)
	ServerCmd.AddCommand(serverPsCmd)

	serverStartCmd.Flags().IntP("port", "p", 8080, "Port to listen on")
	serverStopCmd.Flags().IntP("port", "p", 8080, "Port to stop")
	serverPsCmd.Flags().IntP("port", "p", 8080, "Port to check")
	serverStartCmd.Flags().StringP("host", "H", "localhost", "Host to bind to")
	serverStartCmd.Flags().BoolP("debug", "d", false, "Enable debug mode")
	serverStartCmd.Flags().BoolP("detach", "D", false, "Run server in background")
}
