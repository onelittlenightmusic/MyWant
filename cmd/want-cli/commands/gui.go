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

const guiPidFile = "gui.pid"

var GuiCmd = &cobra.Command{
	Use:   "gui",
	Short: "Manage the MyWant Frontend (GUI)",
	Long:  `Start and manage the MyWant React frontend directly from the CLI.`,
}

var guiStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the frontend server (GUI)",
	Run: func(cmd *cobra.Command, args []string) {
		detach, _ := cmd.Flags().GetBool("detach")
		port, _ := cmd.Flags().GetInt("port")
		host, _ := cmd.Flags().GetString("host")
		isDev, _ := cmd.Flags().GetBool("dev")

		if isDev {
			// Legacy development mode using npm run dev
			runDevMode(port, detach)
			return
		}

		// Production mode: use internal Go server with embedded assets
		if detach {
			runGoServerDetached(port, host)
			return
		}

		// Foreground Go server
		fmt.Printf("Starting MyWant GUI on http://%s:%d (Embedded Mode)...\n", host, port)
		cfg := server.Config{
			Port:  port,
			Host:  host,
			Debug: false,
		}
		s := server.New(cfg)
		if err := s.Start(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func runDevMode(port int, detach bool) {
	cwd, _ := os.Getwd()
	webDir := filepath.Join(cwd, "web")
	if _, err := os.Stat(webDir); os.IsNotExist(err) {
		fmt.Printf("Error: Frontend source directory not found at %s\n", webDir)
		os.Exit(1)
	}

	// Check node_modules
	nodeModules := filepath.Join(webDir, "node_modules")
	if _, err := os.Stat(nodeModules); os.IsNotExist(err) {
		fmt.Println("node_modules missing. Running 'npm install'...")
		installCmd := exec.Command("npm", "install")
		installCmd.Dir = webDir
		installCmd.Stdout = os.Stdout
		installCmd.Stderr = os.Stderr
		installCmd.Run()
	}

	execArgs := []string{"run", "dev", "--", "--port", strconv.Itoa(port), "--host", "0.0.0.0"}

	if detach {
		os.MkdirAll("logs", 0755)
		logFile, _ := os.OpenFile(filepath.Join("logs", "gui.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		process := exec.Command("npm", execArgs...)
		process.Dir = webDir
		process.Stdout = logFile
		process.Stderr = logFile
		process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		process.Start()
		os.WriteFile(guiPidFile, []byte(strconv.Itoa(process.Process.Pid)), 0644)
		fmt.Printf("Frontend started in background (Dev Mode, PID: %d)\n", process.Process.Pid)
		os.Exit(0)
	} else {
		runCmd := exec.Command("npm", execArgs...)
		runCmd.Dir = webDir
		runCmd.Stdout = os.Stdout
		runCmd.Stderr = os.Stderr
		fmt.Printf("Starting Frontend in Dev Mode on http://localhost:%d...\n", port)
		runCmd.Run()
	}
}

func runGoServerDetached(port int, host string) {
	executable, _ := os.Executable()
	args := []string{"gui", "start", "--port", strconv.Itoa(port), "--host", host}
	
	os.MkdirAll("logs", 0755)
	logFile, err := os.OpenFile(filepath.Join("logs", "gui.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Printf("Failed to open log file: %v\n", err)
		os.Exit(1)
	}

	cmd := exec.Command(executable, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	
	// Set pgid to allow killing the whole group later
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	err = cmd.Start()
	if err != nil {
		fmt.Printf("Failed to start background GUI: %v\n", err)
		os.Exit(1)
	}

	os.WriteFile(guiPidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644)
	fmt.Printf("MyWant GUI started in background (Embedded Mode, PID: %d)\n", cmd.Process.Pid)
	fmt.Printf("URL: http://%s:%d\n", host, port)
	os.Exit(0)
}

var guiStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the background frontend server",
	Run: func(cmd *cobra.Command, args []string) {
		data, err := os.ReadFile(guiPidFile)
		if err != nil {
			fmt.Printf("Error: Could not read PID file (%s). Is the frontend running in background?\n", guiPidFile)
			os.Exit(1)
		}

		pid, _ := strconv.Atoi(string(data))

		// Find the process group to kill all sub-processes started by npm
		// On Unix, the pgid is usually the same as pid of the leader
		fmt.Printf("Stopping Frontend process group (PID: %d)...\n", pid)
		
		// Use negative PID to kill the entire process group
		err = syscall.Kill(-pid, syscall.SIGTERM)
		if err != nil {
			// Fallback to single process kill if pgid kill fails
			process, _ := os.FindProcess(pid)
			if process != nil {
				process.Signal(syscall.SIGTERM)
			}
		}

		os.Remove(guiPidFile)
		fmt.Println("Frontend stopped.")
	},
}

func init() {
	GuiCmd.AddCommand(guiStartCmd)
	GuiCmd.AddCommand(guiStopCmd)

	guiStartCmd.Flags().IntP("port", "p", 3000, "Port to listen on")
	guiStartCmd.Flags().StringP("host", "H", "localhost", "Host to bind to")
	guiStartCmd.Flags().BoolP("detach", "D", false, "Run frontend in background")
	guiStartCmd.Flags().Bool("dev", false, "Run in development mode (requires npm)")
}
