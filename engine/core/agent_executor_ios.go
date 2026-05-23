//go:build ios

package mywant

import (
	"context"
	"fmt"
)

// AgentExecutor defines the interface for executing agents.
type AgentExecutor interface {
	Execute(ctx context.Context, agent Agent, want *Want) error
	GetMode() ExecutionMode
}

// LocalExecutor executes agents in-process as goroutines.
type LocalExecutor struct{}

func NewLocalExecutor() *LocalExecutor { return &LocalExecutor{} }

func (e *LocalExecutor) GetMode() ExecutionMode { return ExecutionModeLocal }

func (e *LocalExecutor) Execute(ctx context.Context, agent Agent, want *Want) error {
	shouldStop, err := agent.Exec(ctx, want)
	if err != nil {
		return fmt.Errorf("agent execution failed: %w", err)
	}
	_ = shouldStop
	return nil
}

// RPCExecutor is a stub on iOS — gRPC is not supported.
type RPCExecutor struct{}

func NewRPCExecutor(_ RPCConfig) (*RPCExecutor, error) {
	return nil, fmt.Errorf("gRPC execution is not supported on iOS")
}

func (e *RPCExecutor) GetMode() ExecutionMode { return ExecutionModeLocal }

func (e *RPCExecutor) Execute(_ context.Context, _ Agent, _ *Want) error {
	return fmt.Errorf("gRPC execution is not supported on iOS")
}

// NewExecutor creates the appropriate executor (iOS: gRPC/RPC not supported).
func NewExecutor(config ExecutionConfig) (AgentExecutor, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid execution config: %w", err)
	}
	switch config.GetExecutionMode() {
	case ExecutionModeLocal:
		return NewLocalExecutor(), nil
	case ExecutionModeWebhook:
		return NewWebhookExecutor(*config.WebhookConfig), nil
	case ExecutionModeRPC:
		return nil, fmt.Errorf("RPC/gRPC execution is not supported on iOS")
	default:
		return nil, fmt.Errorf("unknown execution mode: %s", config.Mode)
	}
}
