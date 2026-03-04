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

## 3. Case Study: Travel Reservation

How the keys change for a Hotel booking:

| Legacy Key | New GCP Key | Responsible Agent |
| :--- | :--- | :--- |
| `hotel_type` | `goal.hotel_type` | Think |
| `status` | `current.res_status` | Monitor |
| `good_to_reserve` | `plan.reserve` | Think |
| `reservation_id` | `current.res_id` | Do (initial) / Monitor |

---

## 4. Implementation Steps

1. **Helper Methods**: Implement `SetGoal`, `GetCurrent`, `SetPlan` in `engine/core/want.go`.
2. **Refactor Thinkers**: Update `condition_thinker` and `budget_thinker`.
3. **Refactor Monitors**: Update `monitor_flight_api` and generic webhook monitors.
4. **Refactor Actors**: Update `agent_premium` and reservation logic.
