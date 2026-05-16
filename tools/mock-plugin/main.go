package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	pidFile     = "flight-server.pid"
	logFile     = "flight-server.log"
	defaultPort = "8090"
)

func mywantDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".mywant")
}

func pidFilePath() string { return filepath.Join(mywantDir(), pidFile) }
func logFilePath() string { return filepath.Join(mywantDir(), logFile) }

func flightServerBin() string {
	// Look relative to this binary's location, then fall back to PATH
	exe, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "flight-server")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "flight-server"
}

func readPID() (int, error) {
	data, err := os.ReadFile(pidFilePath())
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func isRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func flightStart(args []string) {
	port := defaultPort
	for i, a := range args {
		if (a == "--port" || a == "-p") && i+1 < len(args) {
			port = args[i+1]
		}
	}

	if pid, err := readPID(); err == nil && isRunning(pid) {
		fmt.Printf("Flight server is already running (PID %d, port %s)\n", pid, port)
		return
	}

	bin := flightServerBin()
	os.MkdirAll(mywantDir(), 0755)

	lf, err := os.OpenFile(logFilePath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Printf("Error opening log file: %v\n", err)
		os.Exit(1)
	}
	defer lf.Close()

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "PORT="+port)
	cmd.Stdout = lf
	cmd.Stderr = lf
	if err := cmd.Start(); err != nil {
		fmt.Printf("Error starting flight server: %v\n", err)
		fmt.Printf("(binary: %s)\n", bin)
		os.Exit(1)
	}

	os.WriteFile(pidFilePath(), []byte(strconv.Itoa(cmd.Process.Pid)), 0644)

	// Brief wait to confirm startup
	time.Sleep(500 * time.Millisecond)
	if !isRunning(cmd.Process.Pid) {
		fmt.Println("Flight server failed to start. Check logs:")
		fmt.Println(" ", logFilePath())
		os.Exit(1)
	}

	fmt.Printf("Flight server started (PID %d, port %s)\n", cmd.Process.Pid, port)
	fmt.Printf("URL:  http://localhost:%s\n", port)
	fmt.Printf("Logs: %s\n", logFilePath())
}

func flightStop() {
	pid, err := readPID()
	if err != nil {
		fmt.Println("Flight server is not running (no PID file)")
		return
	}
	if !isRunning(pid) {
		fmt.Println("Flight server is not running (process gone)")
		os.Remove(pidFilePath())
		return
	}
	proc, _ := os.FindProcess(pid)
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		fmt.Printf("Error stopping flight server (PID %d): %v\n", pid, err)
		os.Exit(1)
	}
	os.Remove(pidFilePath())
	fmt.Printf("Flight server stopped (PID %d)\n", pid)
}

func flightStatus() {
	pid, err := readPID()
	if err != nil {
		fmt.Println("Flight server: stopped")
		return
	}
	if isRunning(pid) {
		fmt.Printf("Flight server: running (PID %d)\n", pid)
		fmt.Printf("Logs: %s\n", logFilePath())
	} else {
		fmt.Println("Flight server: stopped (stale PID file)")
		os.Remove(pidFilePath())
	}
}

func flightLogs(args []string) {
	lines := "50"
	for i, a := range args {
		if (a == "-n" || a == "--lines") && i+1 < len(args) {
			lines = args[i+1]
		}
	}
	cmd := exec.Command("tail", "-n", lines, "-f", logFilePath())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func mockList() {
	fmt.Println("Available mock servers:")
	fmt.Println()

	pid, err := readPID()
	status := "stopped"
	if err == nil && isRunning(pid) {
		status = fmt.Sprintf("running (PID %d)", pid)
	}
	fmt.Printf("  %-12s %s\n", "flight", status)
	fmt.Printf("              port: %s   api: /api/flights\n", defaultPort)
}

func usage() {
	fmt.Println(`mywant-mock - Mock server management plugin

Usage:
  mywant mock list                    List available mock servers and status
  mywant mock flight start [--port N] Start the mock flight server (default: 8090)
  mywant mock flight stop             Stop the mock flight server
  mywant mock flight status           Show running status
  mywant mock flight logs [-n N]      Tail flight server logs (default: 50 lines)`)
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(0)
	}

	switch args[0] {
	case "list":
		mockList()
	case "flight":
		if len(args) < 2 {
			usage()
			os.Exit(1)
		}
		switch args[1] {
		case "start":
			flightStart(args[2:])
		case "stop":
			flightStop()
		case "status":
			flightStatus()
		case "logs":
			flightLogs(args[2:])
		default:
			fmt.Printf("Unknown flight subcommand: %s\n", args[1])
			usage()
			os.Exit(1)
		}
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Printf("Unknown subcommand: %s\n", args[0])
		usage()
		os.Exit(1)
	}
}
