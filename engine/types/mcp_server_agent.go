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
	mywant.RegisterDoAgentType("mcp_server_do",
		[]mywant.Capability{mywant.Cap("mcp_server_management")},
		startMCPServer)
	mywant.RegisterMonitorAgentType("mcp_server_monitor",
		[]mywant.Capability{mywant.Cap("mcp_server_monitoring")},
		monitorMCPServer)
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

// startMCPServer (DoAgent Action) サーバを起動する
func startMCPServer(ctx context.Context, want *mywant.Want) error {
	serverName := want.GetStringParam("mcp_server_name", "default")
	command := want.GetStringParam("mcp_command", "npx")
	argsRaw, _ := want.GetState("mcp_args")

	// レジストリを確認
	registry := GetMCPServerRegistry()
	if proc, ok := registry.Get(serverName); ok {
		if proc.Cmd.Process != nil && proc.Cmd.ProcessState == nil {
			want.StoreLog("[MCP-PROCESS] Server '%s' is already running", serverName)
			return nil
		}
	}

	var args []string
	if a, ok := argsRaw.([]string); ok {
		args = a
	} else if a, ok := argsRaw.([]interface{}); ok {
		for _, v := range a {
			args = append(args, fmt.Sprintf("%v", v))
		}
	}

	cmd := exec.CommandContext(ctx, command, args...)

	// 環境変数の設定 (GMAIL_TOKEN_PATH, GOOGLE_CLIENT_ID など)
	envMap, _ := want.GetState("mcp_env")
	cmd.Env = os.Environ()
	if envs, ok := envMap.(map[string]interface{}); ok {
		for k, v := range envs {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%v", k, v))
			want.StoreLog("[MCP-PROCESS] Injecting env: %s", k)
		}
	} else if envs, ok := envMap.(map[string]string); ok {
		for k, v := range envs {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
			want.StoreLog("[MCP-PROCESS] Injecting env: %s", k)
		}
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

	want.StoreState("mcp_server_status", "running")
	want.StoreState("mcp_server_pid", cmd.Process.Pid)
	return nil
}

// monitorMCPServer (MonitorAgent Poll) プロセスの生存確認を行う
func monitorMCPServer(ctx context.Context, want *mywant.Want) error {
	serverName := want.GetStringParam("mcp_server_name", "default")
	registry := GetMCPServerRegistry()

	proc, ok := registry.Get(serverName)
	if !ok {
		want.StoreState("mcp_server_status", "stopped")
		return nil
	}

	if proc.Cmd.Process == nil {
		want.StoreState("mcp_server_status", "stopped")
		return nil
	}

	// プロセスの生存確認 (Waitせずに状態だけチェック)
	// プロセスが存在し、終了していないことを確認
	if proc.Cmd.ProcessState != nil && proc.Cmd.ProcessState.Exited() {
		want.StoreState("mcp_server_status", "stopped")
		want.StoreLog("[MCP-PROCESS] Server '%s' has exited", serverName)
	} else {
		want.StoreState("mcp_server_status", "running")
	}

	return nil
}
