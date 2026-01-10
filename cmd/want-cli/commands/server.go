package commands

import (
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

var serverStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the background API server",
	Run: func(cmd *cobra.Command, args []string) {
		data, err := os.ReadFile(pidFile)
		if err != nil {
			fmt.Printf("Error: Could not read PID file (%s). Is the server running in background?\n", pidFile)
			os.Exit(1)
		}

		pid, err := strconv.Atoi(string(data))
		if err != nil {
			fmt.Printf("Error: Invalid PID in %s\n", pidFile)
			os.Exit(1)
		}

		// Find process
		process, err := os.FindProcess(pid)
		if err != nil {
			fmt.Printf("Error: Process with PID %d not found\n", pid)
			os.Remove(pidFile)
			os.Exit(1)
		}

		// Send SIGTERM
		fmt.Printf("Stopping MyWant Server (PID: %d)...\n", pid)
		err = process.Signal(syscall.SIGTERM)
		if err != nil {
			fmt.Printf("Error stopping process: %v\n", err)
			os.Exit(1)
		}

		// Cleanup
		os.Remove(pidFile)
		fmt.Println("Server stopped.")
	},
}

func init() {
	ServerCmd.AddCommand(serverStartCmd)
	ServerCmd.AddCommand(serverStopCmd)

	serverStartCmd.Flags().IntP("port", "p", 8080, "Port to listen on")
	serverStartCmd.Flags().StringP("host", "H", "localhost", "Host to bind to")
	serverStartCmd.Flags().BoolP("debug", "d", false, "Enable debug mode")
	serverStartCmd.Flags().BoolP("detach", "D", false, "Run server in background")
}
