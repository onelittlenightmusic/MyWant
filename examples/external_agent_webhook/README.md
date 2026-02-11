# External Agent Webhook Example

This example demonstrates how to implement external agents that communicate with MyWant via HTTP webhooks.

## Overview

External agents run as separate processes/services and communicate with MyWant through:
- **HTTP requests** from MyWant to the external agent service
- **Webhook callbacks** from external agents back to MyWant

## Architecture

```
MyWant Server                    External Agent Service
     |                                    |
     |  POST /flight/execute              |
     |--------------------------------->  |
     |  (ExecuteRequest)                  |
     |                                    | (process booking)
     |  ExecuteResponse                   |
     |<---------------------------------  |
     |                                    |
     |  (or async callback)               |
     |  POST /api/v1/agents/webhook/callback
     |<---------------------------------  |
     |  (WebhookCallback)                 |
```

## Running the Example

### 1. Start the External Agent Server

```bash
# Set authentication token
export WEBHOOK_AUTH_TOKEN=test123

# Set MyWant server URL (optional, defaults to http://localhost:8080)
export MYWANT_URL=http://localhost:8080

# Run the external agent server
cd examples/external_agent_webhook
go run main.go
```

Server will start on port 9000 with endpoints:
- `POST /flight/execute` - Flight booking DoAgent
- `POST /reaction/monitor/start` - User reaction MonitorAgent
- `GET /health` - Health check

### 2. Start MyWant Server

```bash
# Set the same auth token
export WEBHOOK_AUTH_TOKEN=test123

# Start MyWant server
./bin/mywant start -D --port 8080
```

### 3. Deploy a Want with Webhook Agent

Create `test-webhook-agent.yaml`:

```yaml
wants:
  - name: test-webhook-flight
    type: base_travel
    spec:
      params:
        departure: "NRT"
        arrival: "LAX"
      requires:
        - "flight_api_agency"

agents:
  - name: agent_webhook_flight
    type: do
    capabilities:
      - flight_api_agency
    execution:
      mode: webhook
      webhook:
        service_url: "http://localhost:9000/flight"
        callback_url: "http://localhost:8080/api/v1/agents/webhook/callback"
        auth_token: "test123"
        timeout_ms: 30000
```

Deploy:

```bash
./bin/mywant wants create -f test-webhook-agent.yaml
```

### 4. Verify Execution

Check want status:

```bash
./bin/mywant wants get test-webhook-flight --history
```

You should see:
- Agent execution with `execution_mode: "webhook"`
- State updates from the external agent
- Booking confirmation details

## Agent Types

### DoAgent (Synchronous Execution)

**Flight Booking Agent** (`/flight/execute`):
- Receives execution request with want state
- Performs flight booking simulation
- Returns immediate response with state updates
- Optionally sends async callback for long-running operations

Example request:
```json
{
  "want_id": "test-webhook-flight",
  "agent_name": "agent_webhook_flight",
  "operation": "execute",
  "want_state": {
    "departure": "NRT",
    "arrival": "LAX"
  },
  "callback_url": "http://localhost:8080/api/v1/agents/webhook/callback"
}
```

Example response:
```json
{
  "status": "completed",
  "state_updates": {
    "flight_booking_id": "FLT-1738396800",
    "booking_status": "confirmed",
    "booking_time": "2026-02-01T10:00:00Z"
  },
  "execution_time_ms": 2000
}
```

### MonitorAgent (Asynchronous Monitoring)

**User Reaction Monitor** (`/reaction/monitor/start`):
- Starts background monitoring process
- Periodically checks for user reactions
- Sends callback when state change detected
- Runs until condition met or stopped

Example request:
```json
{
  "want_id": "test-webhook-flight",
  "agent_name": "monitor_reaction",
  "callback_url": "http://localhost:8080/api/v1/agents/webhook/callback",
  "want_state": {
    "booking_id": "FLT-123"
  }
}
```

Example callback (sent after detection):
```json
{
  "agent_name": "monitor_reaction",
  "want_id": "test-webhook-flight",
  "status": "state_changed",
  "state_updates": {
    "user_reaction": {
      "approved": true,
      "timestamp": "2026-02-01T10:01:00Z",
      "comment": "Looks good!"
    }
  }
}
```

## MyWant API Endpoints for External Agents

### Get Want State
```bash
curl -H "Authorization: Bearer test123" \
  http://localhost:8080/api/v1/wants/test-webhook-flight/state
```

Response:
```json
{
  "want_id": "test-webhook-flight",
  "state": {
    "departure": "NRT",
    "arrival": "LAX"
  },
  "status": "reaching",
  "timestamp": "2026-02-01T10:00:00Z"
}
```

### Update Want State
```bash
curl -X POST \
  -H "Authorization: Bearer test123" \
  -H "Content-Type: application/json" \
  -d '{
    "agent_name": "agent_webhook_flight",
    "state_updates": {
      "booking_id": "FLT-123",
      "status": "confirmed"
    }
  }' \
  http://localhost:8080/api/v1/wants/test-webhook-flight/state
```

### Send Webhook Callback
```bash
curl -X POST \
  -H "Authorization: Bearer test123" \
  -H "Content-Type: application/json" \
  -d '{
    "agent_name": "agent_webhook_flight",
    "want_id": "test-webhook-flight",
    "status": "completed",
    "state_updates": {
      "booking_id": "FLT-123"
    }
  }' \
  http://localhost:8080/api/v1/agents/webhook/callback
```

## Implementing Your Own External Agent

### Step 1: Create HTTP Server

```go
package main

import (
    "encoding/json"
    "net/http"
    "github.com/gorilla/mux"
)

func main() {
    router := mux.NewRouter()
    router.HandleFunc("/execute", handleExecute).Methods("POST")
    http.ListenAndServe(":9000", router)
}
```

### Step 2: Implement Execution Handler

```go
func handleExecute(w http.ResponseWriter, r *http.Request) {
    var req ExecuteRequest
    json.NewDecoder(r.Body).Decode(&req)

    // Process the request
    result := doWork(req.WantState)

    // Return response
    response := ExecuteResponse{
        Status: "completed",
        StateUpdates: result,
    }
    json.NewEncoder(w).Encode(response)
}
```

### Step 3: Send Callbacks (for async operations)

```go
func sendCallback(callbackURL string, updates map[string]any) error {
    callback := WebhookCallback{
        AgentName: "my_agent",
        WantID: "my-want",
        Status: "completed",
        StateUpdates: updates,
    }

    body, _ := json.Marshal(callback)
    req, _ := http.NewRequest("POST", callbackURL, bytes.NewReader(body))
    req.Header.Set("Authorization", "Bearer "+authToken)
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    return err
}
```

### Step 4: Define Agent in YAML

```yaml
agents:
  - name: my_custom_agent
    type: do
    capabilities:
      - my_capability
    execution:
      mode: webhook
      webhook:
        service_url: "http://localhost:9000"
        callback_url: "http://localhost:8080/api/v1/agents/webhook/callback"
        auth_token: "${WEBHOOK_AUTH_TOKEN}"
        timeout_ms: 30000
```

## Security

### Authentication
All requests between MyWant and external agents use Bearer token authentication:

```bash
export WEBHOOK_AUTH_TOKEN=your-secret-token
```

This token must be:
- Set in both MyWant server and external agent
- Included in all HTTP requests
- Kept secret and rotated regularly

### Best Practices
- Use HTTPS in production
- Validate all incoming requests
- Implement request signing for additional security
- Use firewall rules to restrict access
- Monitor for unusual activity

## Troubleshooting

### Agent not executing
- Check auth token matches on both sides
- Verify external agent server is running
- Check firewall/network connectivity
- Review logs on both sides

### Callbacks not received
- Verify callback URL is accessible
- Check auth token in callback requests
- Ensure MyWant server is listening
- Check for network issues

### State not updating
- Verify state_updates format is correct
- Check for validation errors in logs
- Ensure field names match expectations
- Review agent execution history

## Next Steps

- Implement custom business logic in external agents
- Add error handling and retry logic
- Deploy external agents to production
- Implement monitoring and alerting
- Scale agents horizontally
