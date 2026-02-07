package mywant

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	pb "mywant/engine/src/proto"
)

// Mock gRPC server for testing
type mockAgentServer struct {
	pb.UnimplementedAgentServiceServer
	executeFunc      func(*pb.ExecuteRequest) (*pb.ExecuteResponse, error)
	startMonitorFunc func(*pb.MonitorRequest) (*pb.MonitorResponse, error)
}

func (s *mockAgentServer) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	if s.executeFunc != nil {
		return s.executeFunc(req)
	}
	return &pb.ExecuteResponse{
		Status: "completed",
		StateUpdates: map[string]string{
			"test_key": "test_value",
		},
		ExecutionTimeMs: 100,
	}, nil
}

func (s *mockAgentServer) StartMonitor(ctx context.Context, req *pb.MonitorRequest) (*pb.MonitorResponse, error) {
	if s.startMonitorFunc != nil {
		return s.startMonitorFunc(req)
	}
	return &pb.MonitorResponse{
		MonitorId: "monitor-123",
		Status:    "started",
	}, nil
}

// createTestGRPCServer creates an in-memory gRPC server for testing
func createTestGRPCServer(t *testing.T) (*grpc.Server, *bufconn.Listener, func()) {
	buffer := 1024 * 1024
	lis := bufconn.Listen(buffer)

	server := grpc.NewServer()
	pb.RegisterAgentServiceServer(server, &mockAgentServer{})

	go func() {
		if err := server.Serve(lis); err != nil {
			t.Logf("Server exited with error: %v", err)
		}
	}()

	cleanup := func() {
		server.Stop()
		lis.Close()
	}

	return server, lis, cleanup
}

// createTestGRPCClient creates a client connected to the test server
func createTestGRPCClient(t *testing.T, lis *bufconn.Listener) (*grpc.ClientConn, func()) {
	ctx := context.Background()

	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))

	if err != nil {
		t.Fatalf("Failed to dial bufnet: %v", err)
	}

	cleanup := func() {
		conn.Close()
	}

	return conn, cleanup
}

func TestRPCExecutor_DoAgent(t *testing.T) {
	// Create mock gRPC server
	_, lis, cleanup := createTestGRPCServer(t)
	defer cleanup()

	// Create gRPC client connection
	conn, clientCleanup := createTestGRPCClient(t, lis)
	defer clientCleanup()

	// Create RPC executor with mock connection
	executor := &RPCExecutor{
		config: RPCConfig{
			Protocol: "grpc",
		},
		grpcClient: pb.NewAgentServiceClient(conn),
		grpcConn:   conn,
	}

	// Create test DoAgent
	agent := &DoAgent{
		BaseAgent: BaseAgent{
			Name: "grpc-test-agent",
			Type: DoAgentType,
		},
	}

	// Create test want
	want := NewWantWithLocals(
		Metadata{Name: "test-want"},
		WantSpec{},
		nil,
		"base",
	)
	want.State = map[string]any{
		"departure": "NRT",
		"arrival":   "LAX",
	}

	// Execute
	err := executor.Execute(context.Background(), agent, want)
	if err != nil {
		t.Errorf("Execution failed: %v", err)
	}

	// Verify state was updated
	if val, exists := want.GetState("test_key"); !exists || val != "test_value" {
		t.Errorf("Expected test_key 'test_value', got %v (exists: %v)", val, exists)
	}
}

func TestRPCExecutor_MonitorAgent(t *testing.T) {
	// Create mock gRPC server
	_, lis, cleanup := createTestGRPCServer(t)
	defer cleanup()

	// Create gRPC client connection
	conn, clientCleanup := createTestGRPCClient(t, lis)
	defer clientCleanup()

	// Create RPC executor with mock connection
	executor := &RPCExecutor{
		config: RPCConfig{
			Protocol: "grpc",
		},
		grpcClient: pb.NewAgentServiceClient(conn),
		grpcConn:   conn,
	}

	// Create test MonitorAgent
	agent := &MonitorAgent{
		BaseAgent: BaseAgent{
			Name: "grpc-monitor-agent",
			Type: MonitorAgentType,
		},
	}

	// Create test want
	want := NewWantWithLocals(
		Metadata{Name: "test-want"},
		WantSpec{},
		nil,
		"base",
	)

	// Execute
	err := executor.Execute(context.Background(), agent, want)
	if err != nil {
		t.Errorf("Monitor start failed: %v", err)
	}
}

func TestRPCConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  RPCConfig
		wantErr bool
	}{
		{
			name: "valid grpc config",
			config: RPCConfig{
				Endpoint: "localhost:9001",
				Protocol: "grpc",
				UseTLS:   false,
			},
			wantErr: false,
		},
		{
			name: "default protocol",
			config: RPCConfig{
				Endpoint: "localhost:9001",
			},
			wantErr: false,
		},
		{
			name: "missing endpoint",
			config: RPCConfig{
				Protocol: "grpc",
			},
			wantErr: true,
		},
		{
			name: "invalid protocol",
			config: RPCConfig{
				Endpoint: "localhost:9001",
				Protocol: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Check default protocol assignment
			if !tt.wantErr && tt.config.Protocol == "" {
				if tt.config.Protocol != "grpc" {
					t.Errorf("Expected default protocol 'grpc', got '%s'", tt.config.Protocol)
				}
			}
		})
	}
}

func TestNewRPCExecutor_Connection(t *testing.T) {
	// This test requires an actual gRPC server
	// Skip if no server is available
	t.Skip("Requires external gRPC server - run manually")

	config := RPCConfig{
		Endpoint: "localhost:9001",
		Protocol: "grpc",
		UseTLS:   false,
	}

	executor, err := NewRPCExecutor(config)
	if err != nil {
		t.Errorf("Failed to create RPC executor: %v", err)
		return
	}

	if executor.GetMode() != ExecutionModeRPC {
		t.Errorf("Expected mode 'rpc', got '%s'", executor.GetMode())
	}

	// Clean up
	executor.Close()
}

func TestRPCExecutor_ErrorHandling(t *testing.T) {
	// Create mock server with error response
	mockServer := &mockAgentServer{
		executeFunc: func(req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
			return &pb.ExecuteResponse{
				Status: "failed",
				Error:  "test error",
			}, nil
		},
	}

	buffer := 1024 * 1024
	lis := bufconn.Listen(buffer)

	server := grpc.NewServer()
	pb.RegisterAgentServiceServer(server, mockServer)

	go func() {
		server.Serve(lis)
	}()
	defer server.Stop()
	defer lis.Close()

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()

	executor := &RPCExecutor{
		config: RPCConfig{
			Protocol: "grpc",
		},
		grpcClient: pb.NewAgentServiceClient(conn),
		grpcConn:   conn,
	}

	agent := &DoAgent{
		BaseAgent: BaseAgent{
			Name: "error-test-agent",
			Type: DoAgentType,
		},
	}

	want := NewWantWithLocals(
		Metadata{Name: "test-want"},
		WantSpec{},
		nil,
		"base",
	)

	// Should return error
	err = executor.Execute(context.Background(), agent, want)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	if err.Error() != "agent execution failed: test error" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestRPCExecutor_Timeout(t *testing.T) {
	// Create mock server with slow response
	mockServer := &mockAgentServer{
		executeFunc: func(req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
			time.Sleep(3 * time.Second)
			return &pb.ExecuteResponse{Status: "completed"}, nil
		},
	}

	buffer := 1024 * 1024
	lis := bufconn.Listen(buffer)

	server := grpc.NewServer()
	pb.RegisterAgentServiceServer(server, mockServer)

	go func() {
		server.Serve(lis)
	}()
	defer server.Stop()
	defer lis.Close()

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()

	executor := &RPCExecutor{
		config: RPCConfig{
			Protocol: "grpc",
		},
		grpcClient: pb.NewAgentServiceClient(conn),
		grpcConn:   conn,
	}

	agent := &DoAgent{
		BaseAgent: BaseAgent{
			Name: "timeout-test",
			Type: DoAgentType,
		},
	}

	want := NewWantWithLocals(
		Metadata{Name: "test-want"},
		WantSpec{},
		nil,
		"base",
	)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Should timeout
	err = executor.Execute(ctx, agent, want)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
}
