package mywant

import (
	"fmt"
	"os"
	"strings"
)

// ExecutionMode defines how an agent is executed
type ExecutionMode string

const (
	ExecutionModeLocal   ExecutionMode = "local"
	ExecutionModeWebhook ExecutionMode = "webhook"
	ExecutionModeRPC     ExecutionMode = "rpc"
)

// ExecutionConfig defines how an agent should be executed
type ExecutionConfig struct {
	Mode          ExecutionMode      `yaml:"mode" json:"mode"`
	WebhookConfig *WebhookConfig     `yaml:"webhook,omitempty" json:"webhook,omitempty"`
	RPCConfig     *RPCConfig         `yaml:"rpc,omitempty" json:"rpc,omitempty"`
}

// WebhookConfig contains webhook execution settings
type WebhookConfig struct {
	ServiceURL        string `yaml:"service_url" json:"service_url"`               // External agent service endpoint
	CallbackURL       string `yaml:"callback_url" json:"callback_url"`             // MyWant callback endpoint
	AuthToken         string `yaml:"auth_token" json:"auth_token"`                 // Authentication token
	TimeoutMs         int    `yaml:"timeout_ms" json:"timeout_ms"`                 // Request timeout in milliseconds
	MonitorIntervalMs int    `yaml:"monitor_interval_ms" json:"monitor_interval_ms"` // Monitor polling interval (default: 30000ms)
	MonitorMode       string `yaml:"monitor_mode" json:"monitor_mode"`             // "one-shot" or "periodic" (default: "periodic")
}

// RPCConfig contains RPC execution settings
type RPCConfig struct {
	Endpoint string `yaml:"endpoint" json:"endpoint"`   // host:port
	Protocol string `yaml:"protocol" json:"protocol"`   // grpc or jsonrpc
	UseTLS   bool   `yaml:"use_tls" json:"use_tls"`     // Enable TLS
}

// DefaultExecutionConfig returns a default local execution config
func DefaultExecutionConfig() ExecutionConfig {
	return ExecutionConfig{
		Mode: ExecutionModeLocal,
	}
}

// Validate validates the execution configuration
func (ec *ExecutionConfig) Validate() error {
	switch ec.Mode {
	case ExecutionModeLocal:
		// Local mode requires no additional config
		return nil
	case ExecutionModeWebhook:
		if ec.WebhookConfig == nil {
			return fmt.Errorf("webhook mode requires webhook configuration")
		}
		return ec.WebhookConfig.Validate()
	case ExecutionModeRPC:
		if ec.RPCConfig == nil {
			return fmt.Errorf("rpc mode requires rpc configuration")
		}
		return ec.RPCConfig.Validate()
	case "":
		// Empty mode defaults to local
		ec.Mode = ExecutionModeLocal
		return nil
	default:
		return fmt.Errorf("unknown execution mode: %s", ec.Mode)
	}
}

// Validate validates webhook configuration
func (wc *WebhookConfig) Validate() error {
	if wc.ServiceURL == "" {
		return fmt.Errorf("webhook service_url is required")
	}
	if wc.CallbackURL == "" {
		return fmt.Errorf("webhook callback_url is required")
	}
	if wc.TimeoutMs <= 0 {
		wc.TimeoutMs = 30000 // Default 30s timeout
	}
	if wc.MonitorIntervalMs <= 0 {
		wc.MonitorIntervalMs = 30000 // Default 30s monitor interval
	}
	if wc.MonitorMode == "" {
		wc.MonitorMode = "periodic" // Default to periodic monitoring
	}
	if wc.MonitorMode != "one-shot" && wc.MonitorMode != "periodic" {
		return fmt.Errorf("invalid monitor_mode: %s (must be 'one-shot' or 'periodic')", wc.MonitorMode)
	}

	// Expand environment variables in auth token
	if wc.AuthToken != "" && strings.HasPrefix(wc.AuthToken, "${") && strings.HasSuffix(wc.AuthToken, "}") {
		envVar := strings.TrimSuffix(strings.TrimPrefix(wc.AuthToken, "${"), "}")
		wc.AuthToken = os.Getenv(envVar)
		if wc.AuthToken == "" {
			return fmt.Errorf("environment variable %s not set for auth token", envVar)
		}
	}

	return nil
}

// Validate validates RPC configuration
func (rc *RPCConfig) Validate() error {
	if rc.Endpoint == "" {
		return fmt.Errorf("rpc endpoint is required")
	}
	if rc.Protocol == "" {
		rc.Protocol = "grpc" // Default to gRPC
	}
	if rc.Protocol != "grpc" && rc.Protocol != "jsonrpc" {
		return fmt.Errorf("unsupported rpc protocol: %s (must be 'grpc' or 'jsonrpc')", rc.Protocol)
	}
	return nil
}

// GetExecutionMode returns the execution mode, defaulting to local if not set
func (ec *ExecutionConfig) GetExecutionMode() ExecutionMode {
	if ec.Mode == "" {
		return ExecutionModeLocal
	}
	return ec.Mode
}
