# MyWant Agent System Documentation

## Overview

The MyWant Agent System provides capability-based, autonomous agents that can execute actions and monitor state changes for wants. This system enables wants to delegate specific tasks to specialized agents while maintaining clean separation of concerns.

## Architecture

### Core Components

1. **Capabilities** - Define what services are available
2. **Agents** - Implement specific capabilities with two types:
   - **DoAgent** - Performs actions (e.g., make API calls, reserve resources)
   - **MonitorAgent** - Monitors state (e.g., check status, validate conditions)
3. **Agent Registry** - Manages capability-to-agent mapping
4. **Want Integration** - Wants specify requirements and execute matching agents

### Agent Types

#### DoAgent
- **Purpose**: Perform actions that change external state
- **Examples**: Make hotel reservations, process payments, send notifications
- **Characteristics**:
  - Typically short-lived
  - Makes external API calls
  - Updates want state with results

#### MonitorAgent
- **Purpose**: Monitor and validate state
- **Examples**: Check reservation status, validate payments, monitor resources
- **Characteristics**:
  - Can be long-running
  - Reads from external systems
  - Updates want state with monitoring data

## Configuration

### Capability Definition (`capabilities/capability-*.yaml`)

```yaml
capabilities:
  - name: hotel_agency
    gives:
      - hotel_reservation
      - hotel_cancellation
    description: "Provides hotel booking and management services"
    version: "1.2.0"
```

### Agent Definition (`agents/agent-*.yaml`)

```yaml
agents:
  - name: agent_premium
    type: do
    capabilities:
      - hotel_agency
    uses:
      - booking_api
      - payment_gateway
    description: "Premium hotel booking agent"
    priority: 80
    enabled: true
    tags: ["premium", "hotel"]
    version: "2.1.0"

  - name: hotel_monitor
    type: monitor
    capabilities:
      - hotel_agency
    uses:
      - monitoring_api
    description: "Hotel reservation monitoring agent"
    priority: 60
    enabled: true
    tags: ["monitor", "hotel"]
    version: "1.5.0"
```

### Want Requirements (`config/config-*.yaml`)

```yaml
wants:
  - metadata:
      name: luxury-hotel-booking
      type: hotel
    spec:
      params:
        hotel_type: luxury
        check_in: "2025-09-20"
        check_out: "2025-09-22"
      requires:
        - hotel_reservation  # This triggers agent selection
```

## Agent Implementation

### Agent Interface

```go
type Agent interface {
    Exec(ctx context.Context, want *Want) error
    GetCapabilities() []string
    GetName() string
    GetType() AgentType
    GetUses() []string
}
```

### DoAgent Implementation

```go
func (r *AgentRegistry) hotelReservationAction(ctx context.Context, want *Want) error {
    fmt.Printf("Hotel reservation agent executing for want: %s\n", want.Metadata.Name)

    // Stage all state changes as a single object
    want.StageStateChange(map[string]interface{}{
        "reservation_id": "HTL-12345",
        "status":        "confirmed",
        "hotel_name":    "Premium Hotel",
        "check_in":      "2025-09-20",
        "check_out":     "2025-09-22",
    })

    // Commit all changes at once
    want.CommitStateChanges()

    return nil
}
```

### MonitorAgent Implementation

```go
func (r *AgentRegistry) hotelReservationMonitor(ctx context.Context, want *Want) error {
    fmt.Printf("Hotel reservation monitor checking status for want: %s\n", want.Metadata.Name)

    if reservationID, exists := want.GetState("reservation_id"); exists {
        // Check external system status
        status := checkReservationStatus(reservationID)

        // Stage monitoring updates
        want.StageStateChange(map[string]interface{}{
            "reservation_id": reservationID,
            "status":        status,
            "last_checked":  time.Now().Format(time.RFC3339),
            "room_ready":    true,
        })

        // Commit all updates
        want.CommitStateChanges()
    }

    return nil
}
```

## Execution Flow

### 1. Want Execution
```go
func (hw *HotelWant) Exec(using []chain.Chan, outputs []chain.Chan) bool {
    // Begin exec cycle
    hw.Want.BeginExecCycle()
    defer hw.Want.EndExecCycle()

    // Execute agents based on requirements
    if err := hw.Want.ExecuteAgents(); err != nil {
        // Handle error
        return false
    }

    return true
}
```

### 2. Agent Discovery and Execution
```go
// ExecuteAgents finds matching agents and runs them in goroutines
func (n *Want) ExecuteAgents() error {
    for _, requirement := range n.Spec.Requires {
        agents := n.agentRegistry.FindAgentsByGives(requirement)
        for _, agent := range agents {
            // Execute in goroutine with context cancellation
            go n.executeAgent(agent)
        }
    }
    return nil
}
```

### 3. State Management
Agents use batched state updates for efficiency:

```go
// Single key-value staging
want.StageStateChange("key", "value")

// Object-based staging (preferred)
want.StageStateChange(map[string]interface{}{
    "key1": "value1",
    "key2": "value2",
    "key3": "value3",
})

// Atomic commit
want.CommitStateChanges()
```

## Lifecycle Management

### Agent Lifecycle

1. **Discovery**: Want specifies requirements, system finds matching agents
2. **Execution**: Agents run in separate goroutines with context
3. **State Updates**: Agents stage changes and commit atomically
4. **Cleanup**: Want automatically stops agents on completion/failure

### Goroutine Management

```go
// Agents run with cancellable context
ctx, cancel := context.WithCancel(context.Background())
want.runningAgents[agent.GetName()] = cancel

// Automatic cleanup on want completion
defer want.StopAllAgents()
```

## Validation

The system uses OpenAPI 3.0.3 specifications for validation:

- **`spec/agent-spec.yaml`** - Defines agent and capability schemas
- **Validation**: All YAML files validated at load time
- **Error Handling**: Detailed error messages for invalid configurations

### Validation Example Output

```
✅ [VALIDATION] Capability validated successfully against OpenAPI spec
✅ [VALIDATION] Agent validated successfully against OpenAPI spec
❌ agent structure validation failed: agent at index 0 missing required 'type' field
```

## Integration with Want Types

### Hotel Want Example

```go
type HotelWant struct {
    Want *Want
}

func RegisterHotelWantTypes(builder *ChainBuilder, agentRegistry *AgentRegistry) {
    builder.RegisterWantType("hotel", func(metadata Metadata, spec WantSpec) interface{} {
        want := &Want{
            Metadata: metadata,
            Spec:     spec,
            State:    make(map[string]interface{}),
        }
        if agentRegistry != nil {
            want.SetAgentRegistry(agentRegistry)
        }
        return &HotelWant{Want: want}
    })
}
```

## Demo Usage

### Running the Hotel Agent Demo

```bash
make run-hotel-agent
```

### Expected Output

```
🏨 Starting Hotel Agent Demo
[VALIDATION] Capability validated successfully against OpenAPI spec
[VALIDATION] Agent validated successfully against OpenAPI spec
🔧 Loaded Capabilities:
  Want 'luxury-hotel-booking' requires: [hotel_reservation]
    Agents for 'hotel_reservation': agent_premium(do) hotel_monitor(monitor)
🚀 Executing chain...
Hotel reservation agent executing for want: luxury-hotel-booking
💾 Committed 5 state changes for want luxury-hotel-booking in single batch
Hotel reservation monitor checking status for want: luxury-hotel-booking
💾 Committed 4 state changes for want luxury-hotel-booking in single batch
✅ Hotel Agent Demo completed
```

## Best Practices

### 1. Agent Design
- Keep agents focused on single responsibilities
- Use DoAgents for actions, MonitorAgents for status checking
- Implement proper error handling and timeouts

### 2. State Management
- Use object-based staging for multiple related updates
- Commit changes atomically to maintain consistency
- Include timestamps and metadata in state updates

### 3. Configuration
- Use descriptive names for capabilities and agents
- Include version information for tracking
- Add tags for easy filtering and organization

### 4. Error Handling
- Validate configurations early with OpenAPI schemas
- Implement graceful degradation for agent failures
- Log detailed information for debugging

## Troubleshooting

### Common Issues

1. **Agent Not Found**
   - Check that capability `gives` matches want `requires`
   - Verify agent has the required capability

2. **Validation Failures**
   - Ensure YAML follows OpenAPI schema
   - Check required fields are present
   - Validate agent types are 'do' or 'monitor'

3. **State Not Updated**
   - Confirm agent calls `CommitStateChanges()`
   - Check for goroutine panics in logs
   - Verify agent execution completes successfully

### Debug Tips

- Use validation output to identify configuration issues
- Check agent execution logs for runtime errors
- Monitor state history for unexpected changes
- Verify capability-to-agent mappings are correct