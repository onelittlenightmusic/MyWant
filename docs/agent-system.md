# MyWant Agent System Documentation

## Overview

The MyWant Agent System provides capability-based, autonomous agents that can execute actions and monitor state changes for wants. This system enables wants to delegate specific tasks to specialized agents while maintaining clean separation of concerns.

## Architecture

### Conceptual Overview

The MyWant Agent System operates on a reactive, state-centric architecture where specialized agents collaborate through a central **State**.

#### Agent Control Loop

```mermaid
flowchart TD
    %% Top: Human/Parent
    Parent([Human / Parent Want])

    subgraph Want [Want]
        direction TB
        
        %% Center: State
        State[(State)]
        
        %% Top of State: Think
        Think[Think Agent]
        
        %% Bottom of State: Monitor and Do side-by-side
        subgraph Agents [ ]
            direction LR
            Monitor[Monitor Agent]
            Do[Do Agent]
        end

        %% Connections: Think reads/writes State
        Think <--> State
        
        %% Connections: Monitor writes to State
        Monitor --> State
        
        %% Connections: Do reads from State to act
        State --> Do
    end

    %% Bottom: World
    World[/World / External Systems/]

    %% External Connections
    Parent <--> Think
    World --- Monitor
    Do --- World
```

#### State-Agent Interaction Pattern

```text
       [ Human / Parent Want ]
                 ▲
                 | (read/write parent state)
                 ▼
+----------------|-------------------+
| Want           |                   |
|        [  Think Agent  ]           |
|                ▲                   |
|                | (read/write)      |
|                ▼                   |
|          [   State   ]             |
|             /     \                |
|    (write) /       \ (read)        |
|           /         \              |
| [ Monitor Agent ]  [   Do Agent   ] |
+-------▲------------------|---------+
        |                  |
        | (observe)        | (action)
        |                  ▼
       [      World / External      ]
```

### Core Components

1. **Capabilities** - Define what services are available
2. **Agents** - Implement specific capabilities with two types:
   - **DoAgent** - Performs actions (e.g., make API calls, reserve resources)
   - **MonitorAgent** - Monitors state (e.g., check status, validate conditions)
3. **Agent Registry** - Manages capability-to-agent mapping
4. **Want Integration** - Wants specify requirements and execute matching agents

### Agent Types (Standard Operational Principles)

The MyWant system follows the **Agent-State Interaction Rule (GCP Pattern)** to coordinate agent activities through a structured state flow.

#### ThinkAgent
- **Purpose**: React to state changes and propagate computed values across want boundaries.
- **Operational Principles**:
  - **Goal Setting**: If goal state is missing, initialize it via `SetGoal()`.
  - **Wait for Context**: Wait until `GetCurrent()` returns values populated by Monitor Agents.
  - **Planning**: Compare Goal with Current state to generate a Plan via `SetPlan()`.
- **Examples**: Register in coordinator itinerary, await budget allocation, propagate reservation costs.
- **Execution Characteristic**: **Background Ticker** (default 2s).
- **State Access**: Reads/writes own State and ParentState.

#### MonitorAgent
- **Purpose**: Monitor and validate state by observing the external world.
- **Operational Principles**:
  - **Observation**: Continuously observe external systems or resources.
  - **State Update**: Write the observed external information into state using `SetCurrent()`.
- **Examples**: Check reservation status, validate payments, monitor resources.
- **Execution Characteristic**: **Asynchronous**.

#### DoAgent
- **Purpose**: Perform actions that change external state based on generated plans.
- **Operational Principles**:
  - **Execution**: Read the Plan fields (via `GetPlan()`) and execute the corresponding actions in the external world.
  - **Flexibility**: **DoAgents can be executed even if no Plan is present** (e.g., direct invocation via `ExecuteAgents()`).
  - **Cleanup/Feedback**: Clear the Plan (via `ClearPlan()`) upon successful execution to prevent redundant actions, and update progress via `SetCurrent()`.
- **Examples**: Make hotel reservations, process payments, send notifications.
- **Execution Characteristic**: **Synchronous**.

#### PollAgent / BackgroundAgent
- **Purpose**: Long-running or system-level background operations.
- **Examples**: Task scheduling, continuous health checks, user reaction polling.
- **Execution Characteristic**: **Persistent**. Registered via `AddBackgroundAgent()`, these agents are initialized when the want starts and remain active throughout the entire lifecycle of the want.
- **Management**: Managed by the want's internal background registry with explicit `Start`/`Stop` signals.

### State-Centric Architecture

All agent types interact with a want's **State** as the central integration point. The diagrams below show each agent type's read/write access pattern and how ThinkAgent uniquely crosses want boundaries via **ParentState**.

#### Agent–State Access Patterns

```
┌──────────────────────────────────────────────────────────────────────┐
│                              Want                                     │
│                                                                       │
│  Triggered via ExecuteAgents()                                       │
│  ┌──────────────────┐  write    ┌──────────────────────────────┐    │
│  │    DoAgent       │──────────►│                              │    │
│  │  (sync, one-shot)│           │           State              │    │
│  └──────────────────┘           │        (key-value store)     │    │
│                                 │                              │    │
│  ┌──────────────────┐  r/w      │                              │    │
│  │  MonitorAgent    │◄─────────►│                              │    │
│  │ (async, contin.) │           │                              │    │
│  └──────────────────┘           └──────────────────────────────┘    │
│                                              ▲                       │
│  Registered via AddBackgroundAgent()         │                       │
│  ┌──────────────────┐  r/w                   │                       │
│  │    PollAgent     │◄──────────────────────►│                       │
│  │ (bg, stop signal)│                        │                       │
│  └──────────────────┘                        │ read / write          │
│                                              │                       │
│  ┌──────────────────┐  r/w                   │                       │
│  │   ThinkAgent     │◄──────────────────────►│                       │
│  │ (bg ticker, 2s)  │                        │                       │
│  └────────┬─────────┘                        │                       │
│           │                                  │                       │
└───────────┼──────────────────────────────────┴───────────────────────┘
            │
            │  GetParentState() / MergeParentState()
            │  (ThinkAgent only)
            ▼
┌──────────────────────────────────────────────────────────────────────┐
│                         Parent Want State                             │
│    e.g. itinerary, target_budgets, costs  (coordinator namespace)    │
└──────────────────────────────────────────────────────────────────────┘
```

#### Agent Access Pattern Summary

| Agent Type | Trigger | Execution | Own State | Parent State |
|:-----------|:--------|:----------|:----------|:-------------|
| DoAgent | `ExecuteAgents()` | Sync, one-shot | write | — |
| MonitorAgent | `ExecuteAgents()` | Async goroutine, continuous | read/write | — |
| PollAgent | `AddBackgroundAgent()` | Persistent bg, stop signal | read/write | — |
| **ThinkAgent** | `AddBackgroundAgent()` | **Persistent bg, ticker (2s)** | **read/write** | **read/write** |

### GPC (Goal, Plan, Current) Access Matrix

The MyWant system uses the **GPC** (Goal → Plan → Current) logic to define the flow of intent to execution. Each agent type has specific permissions for these state fields:

| Agent Type | Goal (Intent) | Plan (Instructions) | Current (Reality) | Primary Responsibility |
|:---|:---:|:---:|:---:|:---|
| **Thinker** | **Write** | **Write** | Read | Initializing goals and generating plans based on Current state. |
| **Monitor** | Read | Read | **Write** | Observing the external world and updating Current state. |
| **Do** | Read | **Read/Clear** | **Write** | Executing plans, clearing them on success, and updating Current. |

#### `child-role` on Want Metadata

Even when an operation is implemented as a **Want** (e.g., an MRS-scriptable want) rather than a standalone agent, it can assume these roles by setting the `child-role` in its metadata. This property determines which labels the child want is permitted to write in its parent coordinator's state:

| `child-role` | Writable Parent Labels | Characteristic |
|:---|:---|:---|
| `thinker` | `plan` | Periodically calculates and writes plans (e.g., budget allocation). |
| `monitor` | `current` | Background polling to update status. |
| `doer` | `current` | Foreground, one-shot execution to write results. |
| `admin` | `goal`, `plan`, `current`, `internal` | Full access for management/coordination wants. |

#### Parent–Child State Coordination

When a want has a parent coordinator, ThinkAgents enable state sharing across the want hierarchy. The example below shows how `ConditionThinker` (on each child want) and `BudgetThinker` (on the budget want) collaborate through the coordinator's State:

```
┌───────────────────────────────────────────────────────────────────────┐
│                        Coordinator Want                                │
│                                                                        │
│  ┌─────────────────────────────────────────────────────────────┐      │
│  │                         State                               │      │
│  │  itinerary:      { hotel: {type, name}, flight: {...}, ... }│◄──┐  │
│  │  target_budgets: { hotel: {budget:1666}, flight: {...}, ... }│   │  │
│  │  costs:          { hotel: 450, flight: 800 }                 │   │  │
│  └─────────────────────────────────────────────────────────────┘   │  │
│          ▲ MergeParentState              │ GetParentState           │  │
│          │                              │                  ThinkAgent  │
│          │                     (BudgetThinker)             reads+writes│
└──────────┼──────────────────────────────┼───────────────────────────┘
           │                              │
  ┌────────┼──────────────────────────────┼──────────────────┐
  │        │       Child Want (hotel)     │                  │
  │        │                             ▼                   │
  │  ┌─────────────────────────────────────────────────────┐ │
  │  │                      State                          │ │
  │  │  good_to_reserve: true                              │ │
  │  │  target_budget:   1666.67                           │◄┤ ThinkAgent
  │  │  cost:            450.00                            │ │ (ConditionThinker)
  │  │  _thinker_*:      (internal flags)                  │ │ reads+writes own
  │  └─────────────────────────────────────────────────────┘ │ + parent state
  └──────────────────────────────────────────────────────────┘
```

##### Coordination Sequence (ConditionThinker ↔ BudgetThinker)

```
Child ConditionThinker          Parent BudgetThinker
        │                               │
  [Phase 1: register]                   │
        │                               │
        ├─ MergeParentState ───────────►│  itinerary: {hotel: {type, name}}
        │                               │
        │                         [Phase 1: allocate]
        │                               │
        │                  GetParentState(itinerary)
        │                               │
        │                  MergeParentState ──────────► target_budgets: {hotel: {budget: 1666}}
        │
  [Phase 2: await budget]
        │
        ├── GetParentState(target_budgets) ──────────────────────────────►
        │
        ├─ StoreState ── good_to_reserve = true, target_budget = 1666
        │
  (DoAgent executes reservation)
        │
        ├─ StoreState ── cost = 450
        │
  [Phase 3: propagate cost]
        │
        ├─ MergeParentState ───────────►│  costs: {hotel: 450}
        │                               │
        │                         [Phase 2: aggregate]
        │                               │
        │                  GetParentState(costs)
        │                               │
        │                  StoreState ─────────────────► total_cost = 450
        │                                                remaining_budget = 1216
```

## Configuration

### Capability Definition (`yaml/capabilities/capability-*.yaml`)

```yaml
capabilities:
  - name: hotel_agency
    gives:
      - hotel_reservation
      - hotel_cancellation
    description: "Provides hotel booking and management services"
    version: "1.2.0"
```

### Agent Definition (`yaml/agents/agent-*.yaml`)

```yaml
agents:
  - name: agent_premium
    type: do
    runtime: localGo
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
    runtime: localGo
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

### Runtime Configuration

The `runtime` field specifies the execution environment for the agent:

- **localGo**: (Default) The agent executes as a registered Go function within the same process.
- **docker**: (Planned) The agent executes inside a dedicated Docker container.

Before an agent starts, the system performs a **Preparation Phase** (`PrepareAgent` status) where it validates the availability of the specified runtime via `bootAgent()`.

### Want Requirements (`yaml/config/config-*.yaml`)

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

    // Check if we have a plan to execute
    if plan, ok := want.GetPlan("reservation_status"); ok && plan == "confirmed" {
        // Perform the actual external action
        reservationID := "HTL-12345"

        // Clear the plan to prevent redundant execution
        want.ClearPlan("reservation_status")

        // Set the results in Current state
        want.SetCurrent("reservation_id", reservationID)
        want.SetCurrent("status", "confirmed")
    }

    return nil
}
```

### MonitorAgent Implementation

```go
func (r *AgentRegistry) hotelReservationMonitor(ctx context.Context, want *Want) error {
    fmt.Printf("Hotel reservation monitor checking status for want: %s\n", want.Metadata.Name)

    if reservationID, ok := want.GetCurrent("reservation_id"); ok {
        // Check external system status
        status := checkReservationStatus(reservationID.(string))

        // Update current status
        want.SetCurrent("status", status)
        want.SetCurrent("last_checked", time.Now().Format(time.RFC3339))
    }

    return nil
}
```

## Execution Flow

### 1. Want Execution
```go
func (hw *HotelWant) Exec(using []chain.Chan, outputs []chain.Chan) bool {
    // Begin exec cycle
    hw.Want.BeginProgressCycle()
    defer hw.Want.EndProgressCycle()

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
Agents should use the semantic labeled methods to maintain GPC consistency. These methods automatically validate that the key has been declared with the correct label in the want's state definition.

```go
// Preferred: Semantic labeled methods
want.SetGoal("budget", 1000)
want.SetPlan("action", "reserve")
want.SetCurrent("status", "confirmed")

// Direct state access (bypasses label validation)
want.StoreState("key", "value")
```

The system handles history recording and persistence automatically. For `DoAgent` and `MonitorAgent`, a `CommitStateChanges()` is performed automatically by the executor after the agent's `Exec` function returns.

## Lifecycle Management

### Agent Lifecycle

1. **Discovery**: A want specifies requirements, and the `AgentRegistry` finds matching agents based on capabilities.
2. **Preparation (`PrepareAgent`)**: The want transitions to the `prepare_agent` status. The `bootAgent()` function checks if the required runtime (localGo or Docker) is ready.
3. **Execution**: Agents are dispatched based on their type:
   - **DoAgents** are executed synchronously.
   - **MonitorAgents** are launched in background goroutines.
4. **State Updates**: Agents perform tasks and stage state changes, which are committed atomically to the want's state.
5. **Cleanup**: When a want is achieved or fails, it automatically stops all associated agents and background tasks.

### Execution Patterns

| Pattern | Trigger Method | Agent Types | Characteristics |
| :--- | :--- | :--- | :--- |
| **Dynamic Task** | `ExecuteAgents()` | `DoAgent`, `MonitorAgent` | Triggered by want logic when specific capabilities are needed. |
| **Persistent Task** | `AddBackgroundAgent()` | `BackgroundAgent`, `PollAgent` | Constant monitoring or system services that live as long as the want. |
| **State-Reactive** | `AddBackgroundAgent()` | `ThinkAgent` | Ticker-based background loop that reads/writes own State and ParentState for cross-want coordination. |

### Status Transitions

During agent execution, a want follows this status flow:
`Reaching` → **`PrepareAgent`** (booting) → `Executing` (sync/async) → `Reaching` (after boot/sync completion) → `Achieved/Failed` (terminal)

### Goroutine Management

```go
// Step 1: Preparation
n.SetStatus(WantStatusPrepareAgent)
err := n.bootAgent(ctx, agent)

// Step 2: Dispatch
if agent.GetType() == DoAgentType {
    // Synchronous execution
    err = executor.Execute(ctx, agent, n)
} else {
    // Asynchronous execution
    go executor.Execute(ctx, agent, n)
}
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
Hotel reservation monitor checking status for want: luxury-hotel-booking
[REGISTRY] Linked agent 'agent_premium' to capability value 'hotel_reservation'
Hotel reservation agent executing for want: luxury-hotel-booking
✅ Hotel Agent Demo completed
```

## Best Practices