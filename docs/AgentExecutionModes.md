# Agent Execution Modes

## Overview

MyWant supports three execution modes for agents, allowing flexible deployment and integration strategies:

1. **Local Mode** - Agents run in-process as goroutines (default, existing behavior)
2. **Webhook Mode** - Agents run as external HTTP services communicating via webhooks
3. **RPC Mode** - Agents run as external services using gRPC/JSON-RPC (planned)

This architecture enables:
- Polyglot agent development (agents in any language)
- Independent agent scaling and deployment
- Separation of concerns between MyWant core and agent logic
- Integration with external systems and services

## Architecture

### Execution Flow

```
Want.ExecuteAgents()
    ↓
Want.executeAgent(agent)
    ↓
NewExecutor(agent.ExecutionConfig)  ← Factory creates appropriate executor
    ↓
Executor.Execute(ctx, agent, want)
    ├─ LocalExecutor   → agent.Exec() (goroutine)
    ├─ WebhookExecutor → HTTP POST to external service
    └─ RPCExecutor     → gRPC/JSON-RPC call
```

### Executor Pattern

All executors implement the `AgentExecutor` interface:

```go
type AgentExecutor interface {
    Execute(ctx context.Context, agent Agent, want *Want) error
    GetMode() ExecutionMode
}
```

The factory pattern automatically selects the correct executor based on agent configuration:

```go
executor, err := NewExecutor(agent.ExecutionConfig)
```

## Configuration

### Agent YAML Configuration

Agents specify their execution mode in the `execution` field:

```yaml
agents:
  # Local execution (default)
  - name: agent_local
    type: do
    capabilities: ["flight_api"]
    execution:
      mode: local

  # Webhook execution
  - name: agent_webhook
    type: do
    capabilities: ["flight_api"]
    execution:
      mode: webhook
      webhook:
        service_url: "http://external-agent:9000/flight/execute"
        callback_url: "http://mywant:8080/api/v1/agents/webhook/callback"
        auth_token: "${WEBHOOK_AUTH_TOKEN}"
        timeout_ms: 30000

  # RPC execution (planned)
  - name: agent_rpc
    type: do
    capabilities: ["flight_api"]
    execution:
      mode: rpc
      rpc:
        endpoint: "external-agent:9001"
        protocol: "grpc"
        use_tls: false
```

### Execution Configuration Types

```go
type ExecutionConfig struct {
    Mode          ExecutionMode      // local, webhook, rpc
    WebhookConfig *WebhookConfig     // For webhook mode
    RPCConfig     *RPCConfig         // For rpc mode
}

type WebhookConfig struct {
    ServiceURL  string  // External agent endpoint
    CallbackURL string  // MyWant callback endpoint
    AuthToken   string  // Bearer token (supports ${ENV_VAR})
    TimeoutMs   int     // Request timeout
}

type RPCConfig struct {
    Endpoint string  // host:port
    Protocol string  // grpc or jsonrpc
    UseTLS   bool    // Enable TLS
}
```

## Local Mode (Default)

### Behavior

- Agents run in-process as goroutines
- Direct function calls to agent `Exec()` methods
- Lowest latency, highest throughput
- Existing implementation (backward compatible)

### When to Use

- Simple agents with minimal dependencies
- High-performance requirements
- Development and testing
- Agents that need direct access to Want internals

### Example

```yaml
agents:
  - name: agent_local_command
    type: do
    capabilities: ["command_execution"]
    execution:
      mode: local  # Can be omitted (default)
```

## Webhook Mode

### Behavior

- Agents run as separate HTTP services
- MyWant sends HTTP POST requests to agent service
- Agent processes request and returns response or sends callback
- Supports both synchronous (DoAgent) and asynchronous (MonitorAgent) patterns

### Communication Flow

#### DoAgent (Synchronous)

```
MyWant                          External Agent
   |  POST /execute                  |
   |-------------------------------->|
   |  ExecuteRequest                 |
   |                                 | (process)
   |  ExecuteResponse                |
   |<--------------------------------|
   |  (with state_updates)           |
```

#### MonitorAgent (Asynchronous)

```
MyWant                          External Agent
   |  POST /monitor/start            |
   |-------------------------------->|
   |  MonitorRequest                 |
   |                                 | (start background monitor)
   |  MonitorResponse                |
   |<--------------------------------|
   |                                 |
   |         ... later ...           |
   |                                 | (detect state change)
   |  POST /callback                 |
   |<--------------------------------|
   |  WebhookCallback                |
```

### Request/Response Types

#### ExecuteRequest (to external agent)

```json
{
  "want_id": "my-want",
  "agent_name": "agent_webhook_flight",
  "operation": "execute",
  "want_state": {
    "departure": "NRT",
    "arrival": "LAX"
  },
  "callback_url": "http://mywant:8080/api/v1/agents/webhook/callback"
}
```

#### ExecuteResponse (from external agent)

```json
{
  "status": "completed",
  "state_updates": {
    "booking_id": "FLT-123",
    "status": "confirmed"
  },
  "execution_time_ms": 1500
}
```

#### WebhookCallback (from external agent to MyWant)

```json
{
  "agent_name": "agent_webhook_flight",
  "want_id": "my-want",
  "status": "completed",
  "state_updates": {
    "booking_id": "FLT-123"
  }
}
```

### MyWant API Endpoints (for External Agents)

External agents can interact with MyWant through these endpoints:

#### GET /api/v1/wants/{id}/state

Get current want state.

```bash
curl -H "Authorization: Bearer ${TOKEN}" \
  http://localhost:8080/api/v1/wants/my-want/state
```

Response:
```json
{
  "want_id": "my-want",
  "state": { "departure": "NRT" },
  "status": "reaching",
  "timestamp": "2026-02-01T10:00:00Z"
}
```

#### POST /api/v1/wants/{id}/state

Update want state.

```bash
curl -X POST \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"agent_name": "my_agent", "state_updates": {"key": "value"}}' \
  http://localhost:8080/api/v1/wants/my-want/state
```

#### POST /api/v1/agents/webhook/callback

Send callback (used by external agents).

```bash
curl -X POST \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "agent_name": "my_agent",
    "want_id": "my-want",
    "status": "completed",
    "state_updates": {"result": "success"}
  }' \
  http://localhost:8080/api/v1/agents/webhook/callback
```

### When to Use

- Agents in non-Go languages (Python, Node.js, etc.)
- Integration with existing services
- Independent agent scaling
- Isolation from MyWant core failures
- Testing and development flexibility

### Example

See `examples/external_agent_webhook/` for complete working example.

```yaml
agents:
  - name: agent_webhook_flight
    type: do
    capabilities: ["flight_api_agency"]
    execution:
      mode: webhook
      webhook:
        service_url: "http://localhost:9000/flight/execute"
        callback_url: "http://localhost:8080/api/v1/agents/webhook/callback"
        auth_token: "${WEBHOOK_AUTH_TOKEN}"
        timeout_ms: 30000
```

## RPC Mode (Planned)

### Behavior

- Agents run as gRPC or JSON-RPC services
- Type-safe, high-performance communication
- Bi-directional streaming support (gRPC)
- Better performance than HTTP/JSON webhooks

### When to Use

- High-performance requirements
- Type-safe communication needed
- Streaming data scenarios
- Microservices architecture

### Example

```yaml
agents:
  - name: agent_rpc_flight
    type: do
    capabilities: ["flight_api_agency"]
    execution:
      mode: rpc
      rpc:
        endpoint: "localhost:9001"
        protocol: "grpc"
        use_tls: false
```

## Security

### Authentication

All external agent communication uses Bearer token authentication:

```yaml
execution:
  mode: webhook
  webhook:
    auth_token: "${WEBHOOK_AUTH_TOKEN}"  # Environment variable
```

Set the token:

```bash
export WEBHOOK_AUTH_TOKEN=your-secret-token
```

MyWant validates tokens on:
- All incoming webhook callbacks
- All state query/update requests

### Best Practices

1. **Use environment variables** for auth tokens (never hardcode)
2. **Use HTTPS/TLS** in production
3. **Rotate tokens regularly**
4. **Implement request signing** for additional security
5. **Use firewall rules** to restrict access
6. **Monitor for anomalies** in agent communication

## Monitoring and Debugging

### Agent Execution History

Each agent execution is tracked in `AgentExecution` history:

```json
{
  "agent_name": "agent_webhook_flight",
  "agent_type": "do",
  "execution_mode": "webhook",
  "start_time": "2026-02-01T10:00:00Z",
  "end_time": "2026-02-01T10:00:02Z",
  "status": "achieved",
  "activity": "Flight booked successfully"
}
```

### Viewing Agent History

```bash
# Get want with agent history
./mywant wants get my-want --history

# Check specific agent executions
./mywant wants get my-want | jq '.history.agent_history'
```

### Logs

MyWant logs all external agent interactions:

```
[WEBHOOK] Executing DoAgent agent_webhook_flight at http://localhost:9000/flight/execute
[WEBHOOK] Applied 2 state updates from DoAgent agent_webhook_flight
[WEBHOOK CALLBACK] Received from agent agent_webhook_flight for want my-want (status: completed)
[AGENT HISTORY] Updated agent agent_webhook_flight status to achieved
```

## Testing

### Unit Tests

Run executor tests:

```bash
go test -v mywant/engine/src -run "TestLocalExecutor|TestWebhookExecutor|TestNewExecutor"
```

### Integration Testing

1. Start external agent:
```bash
cd examples/external_agent_webhook
WEBHOOK_AUTH_TOKEN=test123 go run main.go
```

2. Start MyWant:
```bash
WEBHOOK_AUTH_TOKEN=test123 ./mywant start -D --port 8080
```

3. Deploy test want:
```bash
./mywant wants create -f yaml/config/test-webhook-agent.yaml
```

4. Verify execution:
```bash
./mywant wants get webhook-flight-test --history
```

## Migration Guide

### Migrating Existing Agents to Webhook Mode

1. **Extract agent logic** to standalone service
2. **Implement HTTP endpoints** (`/execute` or `/monitor/start`)
3. **Update agent YAML** to use webhook execution config
4. **Deploy external agent service**
5. **Test with MyWant server**

Example:

**Before (Local):**
```go
// In MyWant codebase
func flightBookingAction(ctx context.Context, want *Want) error {
    // Booking logic here
    want.StageStateChange("booking_id", result)
    return nil
}
```

**After (Webhook):**
```go
// In external agent service
func handleFlightExecute(w http.ResponseWriter, r *http.Request) {
    var req ExecuteRequest
    json.NewDecoder(r.Body).Decode(&req)

    // Booking logic here
    result := bookFlight(req.WantState)

    response := ExecuteResponse{
        Status: "completed",
        StateUpdates: map[string]any{
            "booking_id": result,
        },
    }
    json.NewEncoder(w).Encode(response)
}
```

## Future Enhancements

- **Agent Discovery**: Auto-register external agents via service discovery
- **Load Balancing**: Distribute requests across multiple agent instances
- **Circuit Breaker**: Auto-fallback on agent failures
- **Retry Logic**: Configurable retry strategies for transient failures
- **Metrics Dashboard**: Real-time monitoring of agent performance
- **Advanced Auth**: OAuth2, mTLS, API key rotation
- **Agent Versioning**: Support multiple agent versions simultaneously

## Files

### Core Implementation

- `engine/src/agent_execution_config.go` - Execution configuration types
- `engine/src/agent_executor.go` - Executor implementations (Local, Webhook, RPC)
- `engine/src/agent_types.go` - Extended BaseAgent with ExecutionConfig
- `engine/src/want_agent.go` - Modified to use executor pattern
- `engine/src/declarative.go` - AgentExecution with execution_mode field

### Server API

- `engine/cmd/server/agent_api_handlers.go` - External agent API endpoints
- `engine/cmd/server/main.go` - Route registration

### Examples

- `examples/external_agent_webhook/main.go` - External webhook agent server
- `examples/external_agent_webhook/README.md` - Complete setup guide
- `yaml/config/test-webhook-agent.yaml` - Test configuration

### Tests

- `engine/src/agent_executor_test.go` - Executor unit tests

## References

- [Agent System Documentation](agent-system.md)
- [Want Developer Guide](WantDeveloperGuide.md)
- [External Agent Webhook Example](../examples/external_agent_webhook/README.md)
