//go:build !ios

package mywant

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "mywant/engine/core/proto"
)

// AgentExecutor defines the interface for executing agents
type AgentExecutor interface {
	Execute(ctx context.Context, agent Agent, want *Want) error
	GetMode() ExecutionMode
}

// ============================================================================
// Local Executor (existing in-process execution)
// ============================================================================

// LocalExecutor executes agents in-process as goroutines
type LocalExecutor struct{}

func NewLocalExecutor() *LocalExecutor {
	return &LocalExecutor{}
}

func (e *LocalExecutor) GetMode() ExecutionMode {
	return ExecutionModeLocal
}

func (e *LocalExecutor) Execute(ctx context.Context, agent Agent, want *Want) error {
	// Execute agent directly (existing logic from want_agent.go:163)
	shouldStop, err := agent.Exec(ctx, want)
	if err != nil {
		return fmt.Errorf("agent execution failed: %w", err)
	}

	_ = shouldStop // Currently unused at framework level
	return nil
}

// ============================================================================
// RPC Executor (gRPC/JSON-RPC based external execution)
// ============================================================================

// RPCExecutor executes agents via RPC protocols
type RPCExecutor struct {
	config     RPCConfig
	grpcClient pb.AgentServiceClient
	grpcConn   *grpc.ClientConn
}

func NewRPCExecutor(config RPCConfig) (*RPCExecutor, error) {
	executor := &RPCExecutor{
		config: config,
	}

	// Initialize gRPC client if using gRPC protocol
	if config.Protocol == "grpc" {
		var opts []grpc.DialOption
		if config.UseTLS {
			// TODO: Add TLS credentials
			opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		} else {
			opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		}

		conn, err := grpc.NewClient(config.Endpoint, opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to gRPC server: %w", err)
		}

		executor.grpcConn = conn
		executor.grpcClient = pb.NewAgentServiceClient(conn)
		log.Printf("[RPC] Connected to gRPC server at %s", config.Endpoint)
	}

	return executor, nil
}

func (e *RPCExecutor) GetMode() ExecutionMode {
	return ExecutionModeRPC
}

func (e *RPCExecutor) Execute(ctx context.Context, agent Agent, want *Want) error {
	if e.config.Protocol == "grpc" {
		return e.executeGRPC(ctx, agent, want)
	}
	return fmt.Errorf("unsupported RPC protocol: %s", e.config.Protocol)
}

// executeGRPC executes agent via gRPC
func (e *RPCExecutor) executeGRPC(ctx context.Context, agent Agent, want *Want) error {
	if agent.GetType() == DoAgentType {
		return e.executeGRPCDoAgent(ctx, agent, want)
	}
	return e.executeGRPCMonitorAgent(ctx, agent, want)
}

// executeGRPCDoAgent executes DoAgent via gRPC (synchronous)
func (e *RPCExecutor) executeGRPCDoAgent(ctx context.Context, agent Agent, want *Want) error {
	// Convert want state to map[string]string for proto
	stateMap := make(map[string]string)
	want.State.Range(func(key, value any) bool {
		stateMap[key.(string)] = fmt.Sprintf("%v", value)
		return true
	})

	req := &pb.ExecuteRequest{
		WantId:    want.Metadata.Name,
		AgentName: agent.GetName(),
		Operation: "execute",
		WantState: stateMap,
	}

	log.Printf("[gRPC] Executing DoAgent %s at %s", agent.GetName(), e.config.Endpoint)

	resp, err := e.grpcClient.Execute(ctx, req)
	if err != nil {
		return fmt.Errorf("gRPC Execute failed: %w", err)
	}

	// Apply state updates
	if len(resp.StateUpdates) > 0 {
		want.BeginProgressCycle()
		for key, value := range resp.StateUpdates {
			want.storeState(key, value)
		}
		want.EndProgressCycle()
		log.Printf("[gRPC] Applied %d state updates from DoAgent %s", len(resp.StateUpdates), agent.GetName())
	}

	if resp.Status == "failed" && resp.Error != "" {
		return fmt.Errorf("agent execution failed: %s", resp.Error)
	}

	log.Printf("[gRPC] DoAgent %s completed in %dms", agent.GetName(), resp.ExecutionTimeMs)
	return nil
}

// executeGRPCMonitorAgent executes MonitorAgent via gRPC (asynchronous)
func (e *RPCExecutor) executeGRPCMonitorAgent(ctx context.Context, agent Agent, want *Want) error {
	// Convert want state to map[string]string for proto
	stateMap := make(map[string]string)
	want.State.Range(func(key, value any) bool {
		stateMap[key.(string)] = fmt.Sprintf("%v", value)
		return true
	})

	req := &pb.MonitorRequest{
		WantId:      want.Metadata.Name,
		AgentName:   agent.GetName(),
		CallbackUrl: "", // TODO: Get callback URL from config
		WantState:   stateMap,
	}

	log.Printf("[gRPC] Starting MonitorAgent %s at %s", agent.GetName(), e.config.Endpoint)

	resp, err := e.grpcClient.StartMonitor(ctx, req)
	if err != nil {
		return fmt.Errorf("gRPC StartMonitor failed: %w", err)
	}

	if resp.Status == "failed" && resp.Error != "" {
		return fmt.Errorf("monitor start failed: %s", resp.Error)
	}

	log.Printf("[gRPC] MonitorAgent %s started with monitor_id: %s", agent.GetName(), resp.MonitorId)
	return nil
}

// Close closes the gRPC connection
func (e *RPCExecutor) Close() error {
	if e.grpcConn != nil {
		return e.grpcConn.Close()
	}
	return nil
}

// ============================================================================
// Executor Factory
// ============================================================================

// NewExecutor creates the appropriate executor based on configuration
func NewExecutor(config ExecutionConfig) (AgentExecutor, error) {
	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid execution config: %w", err)
	}

	switch config.GetExecutionMode() {
	case ExecutionModeLocal:
		return NewLocalExecutor(), nil
	case ExecutionModeWebhook:
		return NewWebhookExecutor(*config.WebhookConfig), nil
	case ExecutionModeRPC:
		executor, err := NewRPCExecutor(*config.RPCConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create RPC executor: %w", err)
		}
		return executor, nil
	default:
		return nil, fmt.Errorf("unknown execution mode: %s", config.Mode)
	}
}

