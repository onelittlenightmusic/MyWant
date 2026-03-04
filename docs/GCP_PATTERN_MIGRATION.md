# Migration Guide: Agent-State Interaction Rule (GCP Pattern)

This document outlines the specific implementation changes required for each agent type to adopt the Goal-Current-Plan (GCP) pattern.

## 1. Naming Conventions

All agents must interact with the `Want` state using the following key prefixes:

| Prefix | Category | Owner | Description |
| :--- | :--- | :--- | :--- |
| `goal.*` | Goal | Think Agent | The desired end state (e.g., `goal.status: "confirmed"`). |
| `current.*` | Current | Monitor Agent | The observed reality from the world (e.g., `current.status: "none"`). |
| `plan.*` | Plan | Think Agent | Specific instructions for Do Agents (e.g., `plan.action: "reserve"`). |

---

## 2. Specific Implementation Changes

### A. Think Agent (The Planner)
Think Agents are the orchestrators of the GCP loop.

**Changes:**
1. **Initialize Goals**: On the first run, read `spec.params` and initialize `goal.*` fields.
   - *Logic*: `if state["goal.xyz"] == nil { state["goal.xyz"] = params["xyz"] }`
2. **Monitor Context**: Wait for `current.*` fields to be populated by Monitor Agents.
   - *Logic*: `if state["current.xyz"] == nil { return wait }`
3. **Generate Plans**: Compare `goal` and `current`. If they differ, set a `plan.*`.
   - *Logic*: `if goal.xyz != current.xyz { state["plan.xyz"] = "execute" }`

### B. Monitor Agent (The Observer)
Monitor Agents focus purely on observing the external world.

**Changes:**
1. **Redirect Outputs**: Change all state writes to use the `current.` prefix.
   - *Example*: `want.StoreState("status", "confirmed")` → `want.StoreState("current.status", "confirmed")`
2. **Stateless Observation**: Ensure the monitor does not check goals or plans; it only reports facts.

### C. Do Agent (The Actor)
Do Agents execute based on the plans provided by Think Agents.

**Changes:**
1. **Check Plan**: Before executing, check if a relevant `plan.*` exists.
   - *Logic*: `if state["plan.reserve"] == "execute" { ... }`
2. **Execute & Feedback**: Perform the action and update `current.*` if the result is immediate.
3. **Cleanup**: Upon success, remove or overwrite the `plan.*` field.
   - *Logic*: `want.StoreState("plan.reserve", nil)`
4. **Backward Compatibility**: Ensure the agent can still run via direct `ExecuteAgents()` calls if the plan is absent but required by legacy code.

---

## 3. Case Studies: Comprehensive Mappings

How legacy keys transform into the GCP pattern across different agent types:

### A. Think Agent Case Study: `condition_thinker`
The Think Agent is responsible for the transition from "what we want" (Goal) to "what we should do" (Plan).

| Legacy / Source | New GCP Key | Responsibility |
| :--- | :--- | :--- |
| `spec.params["hotel_type"]` | `goal.hotel_type` | **Think**: Initialize goal from parameters. |
| `parent.state["target_budgets"]`| `current.budget_limit` | **Think**: Fetch context from parent and store as current fact. |
| `good_to_reserve` | `plan.execute_booking` | **Think**: Set plan when goal != current and budget is OK. |
| `_thinker_itinerary_registered` | `_thinker.itinerary_done` | **Internal**: Use underscore for internal tracking flags. |

### B. Monitor Agent Case Study: `monitor_flight_api`
The Monitor Agent focuses purely on updating the `current.*` state based on external reality.

| Legacy Key | New GCP Key | Responsibility |
| :--- | :--- | :--- |
| `flight_status` | `current.status` | **Monitor**: Update with latest observation (e.g., "delayed"). |
| `departure_time` | `current.departure_time` | **Monitor**: Reflect actual time from API. |
| `status_message` | `current.message` | **Monitor**: Provide details about the current status. |
| `_monitor_state_hash` | `_monitor.last_hash` | **Internal**: Tracking for differential updates. |

### C. Do Agent Case Study: `agent_premium` (Reservation)
The Do Agent acts on the `plan.*` and provides initial feedback to `current.*`.

| Plan / Trigger Key | Action & Result Key | Responsibility |
| :--- | :--- | :--- |
| `plan.execute_booking` | (Read Trigger) | **Do**: Start execution if this is true. |
| `reservation_id` | `current.res_id` | **Do**: Store the ID immediately after API success. |
| `status` | `current.res_status` | **Do**: Update to "confirmed" upon completion. |
| (Post-Execution) | `plan.execute_booking` | **Do**: Set to `nil` or `false` to mark plan as handled. |

### D. Orchestration Case Study: `dispatch_thinker`
For complex parent wants managing children.

| Legacy Key | New GCP Key | Responsibility |
| :--- | :--- | :--- |
| `directions` | `goal.directions` | **Think**: The sequence of sub-wants to achieve. |
| `_dispatched_directions` | `current.dispatched` | **Monitor/Think**: Tracking what has actually been created. |
| (New Dispatch Request) | `plan.dispatch_child` | **Think**: Signal to the system to create a new sub-want. |

---

## 4. Implementation Steps

1. **Helper Methods**: Implement `SetGoal`, `GetCurrent`, `SetPlan` in `engine/core/want.go`.
2. **Refactor Thinkers**: Update `condition_thinker` and `budget_thinker`.
3. **Refactor Monitors**: Update `monitor_flight_api` and generic webhook monitors.
4. **Refactor Actors**: Update `agent_premium` and reservation logic.
