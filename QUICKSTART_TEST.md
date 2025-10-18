# Quick Start: Testing Dynamic Travel with Flight API

## 30-Second Overview

This test demonstrates a complete flight booking lifecycle with automatic delay detection and rebooking, all integrated with a travel planning system.

## Running the Test

### Prerequisites
- Mock server running on port 8081
- Go 1.21+
- ~60 seconds of time

### Step 1: Start Mock Server (Terminal 1)
```bash
cd /Users/hiroyukiosaki/work/golang/MyWant
make run-mock
```

Wait for:
```
Flight server listening on :8081
```

### Step 2: Run Test (Terminal 2)
```bash
cd /Users/hiroyukiosaki/work/golang/MyWant
make test-dynamic-travel-with-flight-api
```

### What Happens Next

The test will:

1. **Load Configuration** (~1s)
   - Loads recipe from `recipes/dynamic-travel-change.yaml`
   - Creates 5 child wants: Flight, Restaurant, Hotel, Buffet, Coordinator

2. **Create Flight** (~2s)
   - AgentFlightAPI calls POST /api/flights
   - Receives flight ID and "confirmed" status
   - Stores in want state

3. **Monitor for Changes** (~40s)
   - MonitorFlightAPI polls every 10 seconds
   - T+20s: Status changes to "details_changed"
   - T+40s: Status changes to "delayed_one_day"

4. **Automatic Rebooking** (~2s)
   - FlightWant detects delay
   - Cancels old flight via DELETE
   - Books new flight via POST
   - All transitions recorded

5. **Generate Report** (~1s)
   - Creates memory dump with complete history
   - Analyzes results
   - Shows state transitions

## Expected Output

```
🧳 Testing Dynamic Travel Change with Flight API
==================================================

📋 Prerequisites:
  ✓ Mock server running on http://localhost:8081
  ✓ Config: config/config-dynamic-travel-change.yaml
  ✓ Recipe: recipes/dynamic-travel-change.yaml

🚀 Starting Dynamic Travel Change Demo...

[RECONCILE] Loading initial configuration
[FLIGHT] Agent execution completed
[AgentFlightAPI] Created flight reservation: AA100 (ID: flight-123, Status: confirmed)
[Restaurant] Generated restaurant schedule
[Hotel] Hotel booking created
[Buffet] Breakfast buffet scheduled
[COORDINATOR] Aggregating all schedules

⏳ Waiting for status changes...

[MonitorFlightAPI] Status changed: confirmed -> details_changed
[MonitorFlightAPI] Status changed: details_changed -> delayed_one_day
[FLIGHT] Detected delayed_one_day status, initiating cancellation and rebooking
[AgentFlightAPI] Cancelled flight: flight-123
[AgentFlightAPI] Created flight reservation: AA100 (ID: flight-456, Status: confirmed)

📊 Test Results:

✅ Memory dump created
   Location: memory/memory-20251017-153042.yaml

📝 Analyzing Memory Dump...

✅ Flight status found in state
✅ Flight ID found in state
✅ State history entries: 8
✅ Delay status detected in history
✅ Flight cancellation recorded

📖 State History Summary:

flight_status: "confirmed"
flight_id: "flight-123"
cancelled_flight_id: "flight-123"
flight_id: "flight-456"

✅ Test completed
```

## What to Look For

### Success Indicators ✅

- [ ] Memory dump created (`memory/memory-*.yaml`)
- [ ] Flight ID appears in state
- [ ] Flight status shows progression
- [ ] Delay detection logged
- [ ] Cancellation recorded
- [ ] New flight created
- [ ] 5+ state history entries

### Key Outputs

1. **Flight Creation**
   ```
   [AgentFlightAPI] Created flight reservation: AA100 (ID: flight-123, Status: confirmed)
   ```

2. **Status Changes**
   ```
   [MonitorFlightAPI] Status changed: confirmed -> details_changed
   [MonitorFlightAPI] Status changed: details_changed -> delayed_one_day
   ```

3. **Automatic Rebooking**
   ```
   [FLIGHT] Detected delayed_one_day status, initiating cancellation and rebooking
   [AgentFlightAPI] Cancelled flight: flight-123
   [AgentFlightAPI] Created flight reservation: AA100 (ID: flight-456, Status: confirmed)
   ```

4. **Memory Dump Analysis**
   ```
   ✅ Memory dump created
   ✅ State history entries: 8
   ✅ Delay status detected in history
   ✅ Flight cancellation recorded
   ```

## After the Test

### View Memory Dump
```bash
cat memory/memory-0000-latest.yaml
```

### See Flight State History
```bash
grep -B 1 "flight_status" memory/memory-0000-latest.yaml | head -20
```

### See All Timestamps
```bash
grep "timestamp:" memory/memory-0000-latest.yaml
```

### List All Dumps
```bash
ls -lh memory/memory-*.yaml
```

## Timeline

| Time | Event |
|------|-------|
| T+0s | Flight created (status: confirmed) |
| T+1-10s | Initial setup and coordination |
| T+20s | Status changed to details_changed |
| T+20-30s | Monitor detects change, records in state |
| T+40s | Status changed to delayed_one_day |
| T+41s | Delay detected, cancellation triggered |
| T+42s | Old flight cancelled |
| T+43s | New flight created |
| T+44-50s | Memory dump and analysis |
| T+50s | Test completed |

## Troubleshooting

### "connection refused"
```bash
# Mock server not running
make run-mock  # Start it first
```

### "No memory dump created"
```bash
# Check if memory directory exists
mkdir -p memory
chmod 755 memory
```

### Test takes too long
```bash
# Mock server may be slow
# Check mock server logs
# Verify network connectivity
```

## Understanding the Flow

### 1. Configuration Loading
- Recipe loaded from `recipes/dynamic-travel-change.yaml`
- Config loaded from `config/config-dynamic-travel-change.yaml`
- Creates dynamic travel planner with 5 child wants

### 2. Agent Execution
- Flight want requests `flight_api_reservation` capability
- AgentFlightAPI matches and creates flight
- Result stored in want state

### 3. Monitoring (Async)
- MonitorFlightAPI polls every 10 seconds
- Detects status changes
- Records transitions with timestamps

### 4. Auto-Response
- FlightWant detects "delayed_one_day" status
- Cancels old flight (DELETE)
- Books new flight (POST)
- Records all changes in state

### 5. History Tracking
- Every state change captured via `StoreState()`
- Changes batched during execution cycle
- Committed to state history via `AggregateChanges()`
- Saved in memory dump on completion

## Key Files Involved

| File | Purpose |
|------|---------|
| `recipes/dynamic-travel-change.yaml` | Recipe template (v2.0.0) |
| `config/config-dynamic-travel-change.yaml` | Configuration with Flight API params |
| `engine/cmd/types/agent_flight_api.go` | Flight booking agent |
| `engine/cmd/types/agent_monitor_flight_api.go` | Flight monitoring agent |
| `engine/cmd/types/flight_types.go` | Flight want with delay detection |
| `agents/agent-flight.yaml` | Agent registration |
| `capabilities/capability-flight.yaml` | Capability definitions |

## Next Steps

After successful test:

1. **Study State History**
   - Examine timestamps
   - Trace state transitions
   - Understand delay detection

2. **Customize Parameters**
   - Change flight route, number, type
   - Adjust server URL if needed
   - Modify restaurant/hotel details

3. **Extend Functionality**
   - Add notification on delay
   - Implement custom rebooking strategy
   - Add retry logic

4. **Integration Testing**
   - Use in CI/CD pipeline
   - Combine with other tests
   - Add performance monitoring

## Support

For detailed information, see:
- **Implementation**: `FLIGHT_AGENT_IMPLEMENTATION.md`
- **Testing**: `TESTING_GUIDE.md`
- **Features**: `DYNAMIC_TRAVEL_WITH_FLIGHT_API.md`
- **Migration**: `FLIGHT_API_MIGRATION.md`

## Key Takeaways

- ✅ **Real API Integration**: Direct HTTP calls to mock server
- ✅ **Automatic Monitoring**: Async polling for status changes
- ✅ **Intelligent Rebooking**: Detects delays and responds automatically
- ✅ **Complete History**: Full audit trail with timestamps
- ✅ **Production Pattern**: Demonstrates resilient booking system

Ready? Start with `make run-mock` in one terminal, then `make test-dynamic-travel-with-flight-api` in another!
