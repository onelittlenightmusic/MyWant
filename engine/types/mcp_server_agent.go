package types

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	mywant "mywant/engine/core"
)

func init() {
	mywant.RegisterDoAgent("mcp_server_do", startMCPServer)
	mywant.RegisterMonitorAgent("mcp_server_monitor", monitorMCPServer)
}

// MCPServerProcess 実行中のMCPサーバプロセスを管理する構造体
type MCPServerProcess struct {
	Cmd    *exec.Cmd
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Name   string
}

// MCPServerRegistry プロセスをエージェント間で共有するためのレジストリ
type MCPServerRegistry struct {
	mu        sync.Mutex
	processes map[string]*MCPServerProcess
}

var (
	globalMCPServerRegistry = &MCPServerRegistry{
		processes: make(map[string]*MCPServerProcess),
	}
)

func GetMCPServerRegistry() *MCPServerRegistry {
	return globalMCPServerRegistry
}

func (r *MCPServerRegistry) Register(name string, proc *MCPServerProcess) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.processes[name] = proc
}

func (r *MCPServerRegistry) Get(name string) (*MCPServerProcess, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.processes[name]
	return p, ok
}

func (r *MCPServerRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.processes, name)
}

// startMCPServer (DoAgent Action) サーバを起動する
func startMCPServer(ctx context.Context, want *mywant.Want) error {
	serverName := want.GetStringParam("mcp_server_name", "default")
	command := want.GetStringParam("mcp_command", "npx")
	args := mywant.GetCurrent(want, "mcp_args", []string{})

	// レジストリを確認
	registry := GetMCPServerRegistry()
	if proc, ok := registry.Get(serverName); ok {
		if proc.Cmd.Process != nil && proc.Cmd.ProcessState == nil {
			want.StoreLog("[MCP-PROCESS] Server '%s' is already running", serverName)
			return nil
		}
	}

	cmd := exec.CommandContext(ctx, command, args...)

	// 環境変数の設定 (GMAIL_TOKEN_PATH, GOOGLE_CLIENT_ID など)
	envMap := mywant.GetCurrent(want, "mcp_env", map[string]any{})
	cmd.Env = os.Environ()
	for k, v := range envMap {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%v", k, v))
		want.StoreLog("[MCP-PROCESS] Injecting env: %s", k)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr

	want.StoreLog("[MCP-PROCESS] Starting server: %s %v", command, args)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start MCP server process: %w", err)
	}

	registry.Register(serverName, &MCPServerProcess{
		Cmd:    cmd,
		Stdin:  stdin,
		Stdout: stdout,
		Name:   serverName,
	})

	want.SetCurrent("mcp_server_status", "running")
	want.SetCurrent("mcp_server_pid", cmd.Process.Pid)
	return nil
}

// monitorMCPServer (MonitorAgent Poll) プロセスの生存確認を行う
func monitorMCPServer(ctx context.Context, want *mywant.Want) (bool, error) {
	serverName := want.GetStringParam("mcp_server_name", "default")
	registry := GetMCPServerRegistry()

	proc, ok := registry.Get(serverName)
	if !ok {
		want.SetCurrent("mcp_server_status", "stopped")
		return false, nil
	}

	if proc.Cmd.Process == nil {
		want.SetCurrent("mcp_server_status", "stopped")
		return false, nil
	}

	// プロセスの生存確認 (Waitせずに状態だけチェック)
	// プロセスが存在し、終了していないことを確認
	if proc.Cmd.ProcessState != nil && proc.Cmd.ProcessState.Exited() {
		want.SetCurrent("mcp_server_status", "stopped")
		want.StoreLog("[MCP-PROCESS] Server '%s' has exited", serverName)
	} else {
		want.SetCurrent("mcp_server_status", "running")
	}

	return false, nil
}
