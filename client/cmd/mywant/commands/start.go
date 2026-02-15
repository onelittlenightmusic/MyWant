package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"mywant/engine/server"
	"mywant/engine/worker"
	"mywant/web"

	"github.com/spf13/cobra"
)

var StartCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"s"},
	Short:   "Start the MyWant server (API and GUI)",
	Long:    `Start the MyWant backend API server and embedded React frontend from a single command.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Load configuration
		config, err := LoadConfig()
		if err != nil {
			fmt.Printf("Warning: Failed to load config, using defaults: %v\n", err)
			config = DefaultConfig()
		}

		// Override with command-line flags if provided
		port, _ := cmd.Flags().GetInt("port")
		host, _ := cmd.Flags().GetString("host")
		debug, _ := cmd.Flags().GetBool("debug")
		detach, _ := cmd.Flags().GetBool("detach")
		dev, _ := cmd.Flags().GetBool("dev")
		worker, _ := cmd.Flags().GetBool("worker")

		// Use config values if flags not explicitly set
		if !cmd.Flags().Changed("port") {
			if worker {
				port = config.AgentServicePort
			} else {
				port = config.ServerPort
			}
		}
		if !cmd.Flags().Changed("host") {
			if worker {
				host = config.AgentServiceHost
			} else {
				host = config.ServerHost
			}
		}

		// Worker mode: Start Agent Service only
		if worker {
			startAgentService(host, port, debug, detach)
			return
		}

		// 1. Guard: Check if already running (only when starting a new detached process)
		if detach && isPortOpen(port) {
			fmt.Printf("Error: Port %d is already in use. Stop the existing process first.\n", port)
			os.Exit(1)
		}

		// Use ~/.mywant for logs and pid files
		logDir := getMyWantDir()
		if err := os.MkdirAll(logDir, 0755); err != nil {
			fmt.Printf("Failed to create MyWant directory: %v\n", err)
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
			if dev {
				newArgs = append(newArgs, "--dev")
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
			if dev {
				fmt.Println("Frontend development server enabled (npm run dev)")
			}
			fmt.Printf("Logs: %s\n", logFilePath)
			fmt.Printf("URL:  http://%s:%d\n", host, port)
			os.Exit(0)
		}

		// Start frontend dev server if requested
		if dev {
			fmt.Println("Starting frontend development server (npm run dev)...")
			npmCmd := exec.Command("npm", "run", "dev")
			npmCmd.Dir = "web"
			npmCmd.Stdout = os.Stdout
			npmCmd.Stderr = os.Stderr

			if err := npmCmd.Start(); err != nil {
				fmt.Printf("Failed to start npm run dev: %v\n", err)
			} else {
				fmt.Printf("Frontend dev server started (PID: %d)\n", npmCmd.Process.Pid)
			}
		}

		cfg := server.Config{
			Port:           port,
			Host:           host,
			Debug:          debug,
			HeaderPosition: config.HeaderPosition,
			ColorMode:      config.ColorMode,
			ConfigPath:     getConfigPath(),
			MemoryPath:     filepath.Join(getMyWantDir(), "state.yaml"),
			WebFS:          web.GetFileSystem(!debug),
		}

		fmt.Printf("Starting MyWant Server on http://%s:%d (debug=%v)...\n", host, port, debug)
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
	StartCmd.Flags().Bool("dev", false, "Run frontend in development mode (npm run dev)")
	StartCmd.Flags().BoolP("worker", "w", false, "Start as Agent Service worker (stateless, no Want/State management)")
}

// startAgentService starts the Agent Service in worker mode
func startAgentService(host string, port int, debug bool, detach bool) {
	if detach {
		// Prepare background execution
		executable, _ := os.Executable()
		newArgs := []string{"start", "--worker"}
		newArgs = append(newArgs, "--port", strconv.Itoa(port))
		newArgs = append(newArgs, "--host", host)
		if debug {
			newArgs = append(newArgs, "--debug")
		}

		logDir := getMyWantDir()
		os.MkdirAll(logDir, 0755)
		logFilePath := filepath.Join(logDir, "agent-service.log")
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
			fmt.Printf("Failed to start Agent Service: %v\n", err)
			os.Exit(1)
		}

		pidFilePath := filepath.Join(logDir, "agent-service.pid")
		os.WriteFile(pidFilePath, []byte(strconv.Itoa(process.Process.Pid)), 0644)

		// Save port information for ps command
		portFilePath := filepath.Join(logDir, "agent-service.port")
		os.WriteFile(portFilePath, []byte(strconv.Itoa(port)), 0644)

		fmt.Printf("Agent Service started in background (PID: %d)\n", process.Process.Pid)
		fmt.Printf("Logs: %s\n", logFilePath)
		fmt.Printf("URL:  http://%s:%d\n", host, port)
		os.Exit(0)
	}

	// Start Agent Service in foreground
	cfg := worker.Config{
		Port:  port,
		Host:  host,
		Debug: debug,
	}

	fmt.Printf("Starting Agent Service (worker mode) on http://%s:%d (debug=%v)...\n", host, port, debug)
	fmt.Println("Note: This is a stateless worker - all Want/State management is on the main server.")

	w := worker.New(cfg)
	if err := w.Start(); err != nil {
		fmt.Printf("Agent Service error: %v\n", err)
		os.Exit(1)
	}
}
