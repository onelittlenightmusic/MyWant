package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"mywant/pkg/server"

	"github.com/spf13/cobra"
)

var StartCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"s"},
	Short:   "Start the MyWant server (API and GUI)",
	Long:    `Start the MyWant backend API server and embedded React frontend from a single command.`,
	Run: func(cmd *cobra.Command, args []string) {
		port, _ := cmd.Flags().GetInt("port")
		host, _ := cmd.Flags().GetString("host")
		debug, _ := cmd.Flags().GetBool("debug")
		detach, _ := cmd.Flags().GetBool("detach")

		// 1. Guard: Check if already running (only when starting a new detached process)
		if detach && isPortOpen(port) {
			fmt.Printf("Error: Port %d is already in use. Stop the existing process first.\n", port)
			os.Exit(1)
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
			newArgs := []string{"start"}
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
			fmt.Printf("URL:  http://%s:%d\n", host, port)
			os.Exit(0)
		}

		cfg := server.Config{
			Port:  port,
			Host:  host,
			Debug: debug,
		}

		fmt.Printf("Starting MyWant Server on http://%s:%d (debug=%v).\n", host, port, debug)
		s := server.New(cfg)
		if err := s.Start(); err != nil {
			fmt.Printf("Server error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	StartCmd.Flags().IntP("port", "p", 8080, "Port to listen on")
	StartCmd.Flags().StringP("host", "H", "localhost", "Host to bind to")
	StartCmd.Flags().BoolP("debug", "d", false, "Enable debug mode")
	StartCmd.Flags().BoolP("detach", "D", false, "Run server in background")
}
