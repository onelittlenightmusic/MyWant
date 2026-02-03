# External Agent gRPC Example

This example demonstrates how to implement external agents using gRPC for high-performance, type-safe communication with MyWant.

## Overview

gRPC provides:
- Type-safe communication via Protocol Buffers
- Higher performance than HTTP/JSON webhooks
- Bi-directional streaming support
- Built-in error handling and status codes
- Multi-language support

## Architecture

```
MyWant Server                    External gRPC Agent
     |                                    |
     |  gRPC: Execute()                   |
     |--------------------------------->  |
     |  (ExecuteRequest)                  |
     |                                    | (process booking)
     |  ExecuteResponse                   |
     |<---------------------------------  |
     |  (with state_updates)              |
```

## Running the Example

### 1. Start the External gRPC Agent Server

```bash
cd examples/external_agent_grpc

# Initialize go module (first time only)
go mod init external-agent-grpc
go mod tidy

# Run the gRPC server
go run main.go
```

Server will start on port 9001.

### 2. Start MyWant Server

```bash
./mywant start -D --port 8080
```

### 3. Deploy a Want with gRPC Agent

Create `test-grpc-agent.yaml`:

```yaml
wants:
  - name: grpc-flight-test
    type: base_travel
    spec:
      params:
        departure: "NRT"
        arrival: "LAX"
        date: "2026-03-15"
      requires:
        - "flight_api_agency"

agents:
  - name: agent_grpc_flight
    type: do
    capabilities:
      - flight_api_agency
    execution:
      mode: rpc
      rpc:
        endpoint: "localhost:9001"
        protocol: "grpc"
        use_tls: false
```

Deploy:

```bash
./mywant wants create -f test-grpc-agent.yaml
```

### 4. Verify Execution

```bash
# Check want status
./mywant wants get grpc-flight-test --history

# View agent execution history
./mywant wants get grpc-flight-test | jq '.history.agent_history'
```

You should see:
- Agent execution with `execution_mode: "rpc"`
- State updates from gRPC agent
- Booking details in want state

## Protocol Buffers Definition

The gRPC service is defined in `engine/src/proto/agent_service.proto`:

```protobuf
service AgentService {
  rpc Execute(ExecuteRequest) returns (ExecuteResponse);
  rpc StartMonitor(MonitorRequest) returns (MonitorResponse);
  rpc StopMonitor(StopMonitorRequest) returns (StopMonitorResponse);
}
```

### ExecuteRequest

```protobuf
message ExecuteRequest {
  string want_id = 1;
  string agent_name = 2;
  string operation = 3;
  map<string, string> want_state = 4;
  map<string, string> params = 5;
  string callback_url = 6;
}
```

### ExecuteResponse

```protobuf
message ExecuteResponse {
  string status = 1;  // "completed", "failed"
  map<string, string> state_updates = 2;
  string error = 3;
  int64 execution_time_ms = 4;
}
```

## Implementing Your Own gRPC Agent

### Step 1: Generate Proto Code for Your Language

**Go:**
```bash
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       engine/src/proto/agent_service.proto
```

**Python:**
```bash
python -m grpc_tools.protoc -I. \
  --python_out=. --grpc_python_out=. \
  engine/src/proto/agent_service.proto
```

**Node.js:**
```bash
grpc_tools_node_protoc --js_out=import_style=commonjs:. \
  --grpc_out=grpc_js:. \
  engine/src/proto/agent_service.proto
```

### Step 2: Implement the AgentService

**Go Example:**

```go
package main

import (
    "context"
    pb "path/to/proto"
    "google.golang.org/grpc"
)

type AgentServer struct {
    pb.UnimplementedAgentServiceServer
}

func (s *AgentServer) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
    // Your business logic here
    result := processTask(req.WantState)

    return &pb.ExecuteResponse{
        Status: "completed",
        StateUpdates: map[string]string{
            "result": result,
        },
    }, nil
}

func main() {
    lis, _ := net.Listen("tcp", ":9001")
    grpcServer := grpc.NewServer()
    pb.RegisterAgentServiceServer(grpcServer, &AgentServer{})
    grpcServer.Serve(lis)
}
```

**Python Example:**

```python
import grpc
from concurrent import futures
import agent_service_pb2
import agent_service_pb2_grpc

class AgentService(agent_service_pb2_grpc.AgentServiceServicer):
    def Execute(self, request, context):
        # Your business logic here
        result = process_task(request.want_state)

        return agent_service_pb2.ExecuteResponse(
            status="completed",
            state_updates={"result": result}
        )

server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
agent_service_pb2_grpc.add_AgentServiceServicer_to_server(
    AgentService(), server
)
server.add_insecure_port('[::]:9001')
server.start()
server.wait_for_termination()
```

### Step 3: Configure Agent in YAML

```yaml
agents:
  - name: my_custom_grpc_agent
    type: do
    capabilities:
      - my_capability
    execution:
      mode: rpc
      rpc:
        endpoint: "localhost:9001"
        protocol: "grpc"
        use_tls: false
```

## Performance Comparison

| Mode | Latency | Throughput | Type Safety | Language Support |
|------|---------|------------|-------------|------------------|
| **Local** | Lowest | Highest | Go only | Go only |
| **Webhook** | Medium | Medium | No | Any (HTTP) |
| **gRPC** | Low | High | Yes | Any (protobuf) |

gRPC is ideal when:
- You need high performance
- Type safety is important
- You have high-frequency agent calls
- Your agents are in non-Go languages but still need performance

## TLS/Security

### Enable TLS (Production)

**Server side:**

```go
creds, err := credentials.NewServerTLSFromFile("server.crt", "server.key")
grpcServer := grpc.NewServer(grpc.Creds(creds))
```

**Client side (MyWant YAML):**

```yaml
execution:
  mode: rpc
  rpc:
    endpoint: "agent.example.com:9001"
    protocol: "grpc"
    use_tls: true
```

### Mutual TLS (mTLS)

For additional security, implement mTLS where both client and server verify each other's certificates.

## Monitoring and Debugging

### View gRPC Logs

MyWant logs all gRPC interactions:

```
[gRPC] Connected to gRPC server at localhost:9001
[gRPC] Executing DoAgent agent_grpc_flight at localhost:9001
[gRPC] Applied 4 state updates from DoAgent agent_grpc_flight
[gRPC] DoAgent agent_grpc_flight completed in 1523ms
```

### Server-side Logs

The external agent logs:

```
[gRPC DoAgent] Executing agent agent_grpc_flight for want grpc-flight-test
[gRPC DoAgent] Operation: execute
[gRPC DoAgent] Want state: map[arrival:LAX departure:NRT]
[gRPC DoAgent] Flight booked: GRPC-FLT-1738396800 (took 1002ms)
```

### gRPC Reflection

Enable gRPC reflection for debugging with tools like `grpcurl`:

```go
import "google.golang.org/grpc/reflection"

reflection.Register(grpcServer)
```

Test with grpcurl:

```bash
# List services
grpcurl -plaintext localhost:9001 list

# Call Execute method
grpcurl -plaintext -d '{
  "want_id": "test",
  "agent_name": "test_agent",
  "operation": "execute",
  "want_state": {"key": "value"}
}' localhost:9001 mywant.agent.AgentService/Execute
```

## Advanced Features

### Streaming (Future Enhancement)

gRPC supports bi-directional streaming, which could be useful for:
- Real-time agent progress updates
- Large file transfers
- Continuous monitoring

Example service definition:

```protobuf
service AgentService {
  rpc ExecuteStream(stream ExecuteRequest) returns (stream ExecuteResponse);
}
```

### Load Balancing

For production, use gRPC load balancing:

```yaml
execution:
  mode: rpc
  rpc:
    endpoint: "dns:///agents.example.com:9001"  # DNS-based load balancing
    protocol: "grpc"
    use_tls: true
```

### Health Checking

Implement gRPC health checking protocol:

```go
import "google.golang.org/grpc/health"
import "google.golang.org/grpc/health/grpc_health_v1"

healthServer := health.NewServer()
grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
```

## Troubleshooting

### Connection Refused

```
Error: failed to connect to gRPC server: connection refused
```

**Solution:**
- Verify gRPC agent is running: `lsof -i :9001`
- Check endpoint in YAML matches server address
- Ensure firewall allows connections

### Proto Version Mismatch

```
Error: incompatible proto versions
```

**Solution:**
- Regenerate proto files with same protoc version on both sides
- Ensure both use compatible protobuf library versions

### TLS Handshake Failed

```
Error: TLS handshake failed
```

**Solution:**
- Verify certificates are valid
- Check `use_tls` setting matches server configuration
- For development, use `use_tls: false`

## Migration from Webhook to gRPC

### Benefits

- **~3x faster** - Protocol Buffers vs JSON serialization
- **Type safety** - Compile-time type checking
- **Better error handling** - Rich status codes and metadata
- **Smaller payloads** - Binary encoding vs text

### Migration Steps

1. Generate proto code for your language
2. Implement gRPC server (similar interface to webhook)
3. Update agent YAML to use `mode: rpc`
4. Test with same wants
5. Compare performance metrics

## Next Steps

- Explore bi-directional streaming for real-time updates
- Implement custom interceptors for logging/auth
- Set up gRPC gateway for HTTP/REST compatibility
- Deploy with service mesh (Istio, Linkerd) for production

## References

- [gRPC Official Documentation](https://grpc.io/docs/)
- [Protocol Buffers](https://protobuf.dev/)
- [Agent Execution Modes](../../docs/AgentExecutionModes.md)
- [Agent System Documentation](../../docs/agent-system.md)
