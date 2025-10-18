# Testing Guide: Dynamic Travel Change with Flight API

## Quick Start

### Step 1: Start Mock Server (Terminal 1)
```bash
make run-mock
```

Expected output:
```
🏗️  Building mock flight server...
Flight server listening on :8081
```

### Step 2: Run Test (Terminal 2)
```bash
make test-dynamic-travel-with-flight-api
```

## Test Execution Flow

### What the Test Does

1. **Verifies Prerequisites**
   - Checks mock server is running on port 8081
   - Confirms configuration files exist
   - Creates memory directory

2. **Runs Dynamic Travel Demo**
   - Loads recipe from `recipes/dynamic-travel-change.yaml`
   - Creates dynamic travel planner with child wants:
     - Flight (with Flight API agent)
     - Restaurant
     - Hotel
     - Buffet
     - Travel Coordinator

3. **Monitors Flight Status Changes**
   - AgentFlightAPI creates flight (POST)
   - MonitorFlightAPI polls every 10 seconds (GET)
   - Status progression:
     - T+0s: confirmed
     - T+20s: details_changed
     - T+40s: delayed_one_day

4. **Detects Automatic Rebooking**
   - FlightWant detects delay
   - Cancels old flight (DELETE)
   - Creates new flight (POST)
   - Records all transitions

5. **Generates Memory Dump**
   - Saves complete state history
   - Records all state transitions
   - Preserves agent execution history

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
[RECONCILE:COMPILE] Initial load: processing 1 wants

🎯 Creating custom target type: 'dynamic travel change with flight api'

[FLIGHT] Agent execution completed
[AgentFlightAPI] Created flight reservation: AA100 (ID: flight-123, Status: confirmed)

[Restaurant] Generated restaurant schedule
[Hotel] Hotel booking created
[Buffet] Breakfast buffet scheduled
[COORDINATOR] Aggregating all schedules

⏳ Waiting for status changes (approximately 50 seconds)...

[MonitorFlightAPI] Status changed: confirmed -> details_changed
[MonitorFlightAPI] Status changed: details_changed -> delayed_one_day

[FLIGHT] Detected delayed_one_day status, initiating cancellation and rebooking
[AgentFlightAPI] Cancelled flight: flight-123
[AgentFlightAPI] Created flight reservation: AA100 (ID: flight-456, Status: confirmed)

[COORDINATOR] Updated with new flight

📝 Memory dump created at: memory/memory-20251017-153042.yaml

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
...
flight_status: "details_changed"
...
flight_status: "delayed_one_day"
...
cancelled_flight_id: "flight-123"
...
flight_id: "flight-456"
flight_status: "confirmed"

✅ Test completed
```

## Memory Dump Analysis

The test creates a memory dump with complete state history. Key sections:

### 1. Initial Flight Creation
```yaml
state:
  flight_id: "flight-123"
  flight_number: "AA100"
  flight_status: "confirmed"
  from: "New York"
  to: "Los Angeles"
```

### 2. State History Entries
```yaml
history:
  state_history:
    - timestamp: 2025-10-17T15:30:42Z
      state_value:
        flight_id: "flight-123"
        flight_status: "confirmed"
    - timestamp: 2025-10-17T15:31:02Z
      state_value:
        flight_status: "details_changed"
    - timestamp: 2025-10-17T15:31:22Z
      state_value:
        flight_status: "delayed_one_day"
    - timestamp: 2025-10-17T15:31:24Z
      state_value:
        cancelled_flight_id: "flight-123"
        cancellation_successful: true
    - timestamp: 2025-10-17T15:31:25Z
      state_value:
        flight_id: "flight-456"
        flight_status: "confirmed"
```

### 3. Agent History
```yaml
history:
  agent_history:
    - agent_name: "agent_flight_api"
      agent_type: "do"
      status: "completed"
    - agent_name: "monitor_flight_api"
      agent_type: "monitor"
      status: "completed"
```

## Viewing Test Results

### View Latest Memory Dump
```bash
cat memory/memory-0000-latest.yaml
```

### View All Memory Dumps
```bash
ls -lh memory/memory-*.yaml
```

### Search for Specific States
```bash
grep "flight_status" memory/memory-0000-latest.yaml
```

### See Full State Timeline
```bash
grep -A 2 "timestamp:" memory/memory-0000-latest.yaml | head -30
```

## Test Scenarios

### Scenario 1: Full Flight Lifecycle
**Duration**: ~50 seconds

**Steps**:
1. Flight created with "confirmed" status
2. Status changes to "details_changed" (T+20s)
3. Status changes to "delayed_one_day" (T+40s)
4. Automatic cancellation triggered
5. New flight created
6. All transitions recorded in history

**Success Criteria**:
- ✅ Memory dump created
- ✅ Flight ID found in state
- ✅ Status history entries >= 5
- ✅ Delay detected
- ✅ Cancellation recorded
- ✅ New flight created

### Scenario 2: Travel Coordination
**Parallel Bookings**:
- Flight with API agent and monitoring
- Restaurant with static generation
- Hotel with agent support
- Buffet with static generation
- Coordinator aggregates all

**Success Criteria**:
- ✅ All 5 wants created
- ✅ Coordinator aggregates schedules
- ✅ State tracked for each want

### Scenario 3: State History Tracking
**Validation**:
- Each state change has timestamp
- State values are preserved
- Agent history is recorded
- Complete audit trail available

**Success Criteria**:
- ✅ State history entries >= 5
- ✅ Each entry has timestamp
- ✅ Flight status progression visible
- ✅ Cancellation recorded with details

## Troubleshooting

### Mock Server Not Running
```
Error: connection refused
```
**Solution**: Start mock server first
```bash
make run-mock
```

### "flight_api_reservation" Not Found
```
Error: no agent provides flight_api_reservation
```
**Check**:
- Agent registration: `grep -r "flight_api_agency" agents/`
- Capability mapping: `grep -r "flight_api_reservation" capabilities/`
- Config requirement: `grep -r "flight_api_reservation" config/`

### Memory Dump Not Created
```
No memory dump created
```
**Check**:
- Memory directory: `ls -la memory/`
- Write permissions: `touch memory/test.txt`
- Disk space: `df -h`

### Test Timeout
```
Demo running longer than expected
```
**Possible Causes**:
- Slow mock server response
- Network latency
- Multiple retries happening

**Solution**: Increase wait time or check mock server logs

## Performance Metrics

| Phase | Duration | Notes |
|-------|----------|-------|
| Initial flight creation | ~100ms | POST /api/flights |
| First status check | ~100ms | GET /api/flights/{id} |
| Confirmed→Details change | ~20s | Automatic on mock server |
| Details→Delayed change | ~20s | Automatic on mock server |
| Delay detection | <100ms | By FlightWant.Exec() |
| Cancellation | ~100ms | DELETE /api/flights/{id} |
| New flight creation | ~100ms | POST /api/flights (new) |
| **Total Duration** | **~50s** | From creation to rebooking |

## Integration with CI/CD

### Add to CI Pipeline
```bash
# Terminal 1: Start mock server
make run-mock &
MOCK_PID=$!

# Terminal 2: Run test
make test-dynamic-travel-with-flight-api

# Cleanup
kill $MOCK_PID
```

### Example GitHub Actions
```yaml
- name: Start Mock Server
  run: make run-mock &

- name: Run Dynamic Travel Test
  run: make test-dynamic-travel-with-flight-api
  timeout-minutes: 2

- name: Upload Memory Dump
  if: always()
  uses: actions/upload-artifact@v3
  with:
    name: memory-dumps
    path: memory/memory-*.yaml
```

## Advanced Testing

### Manual Status Inspection
```bash
# While test is running in another terminal
watch -n 5 'curl -s http://localhost:8081/api/flights | jq ".[] | {id, status}"'
```

### Check API Directly
```bash
# List all flights
curl http://localhost:8081/api/flights

# Get specific flight
curl http://localhost:8081/api/flights/{flight-id}

# Check health
curl http://localhost:8081/api/health
```

### Monitor Logs
```bash
# Follow mock server logs
tail -f mock_server.log

# Monitor memory dumps creation
watch -n 1 'ls -lh memory/memory-*.yaml | tail -5'
```

## Success Checklist

After running the test, verify:

- [ ] Mock server running without errors
- [ ] Dynamic travel demo started successfully
- [ ] Flight created with initial status "confirmed"
- [ ] Status changes detected (details_changed, delayed_one_day)
- [ ] Cancellation executed
- [ ] New flight created
- [ ] Memory dump generated
- [ ] Memory dump contains complete state history
- [ ] Flight status progression visible in dump
- [ ] Agent history recorded
- [ ] All transitions timestamped

## Next Steps

After successful test:

1. **Analyze State History**
   ```bash
   cat memory/memory-0000-latest.yaml | grep -A 5 "state_history"
   ```

2. **Verify Timestamps**
   ```bash
   grep "timestamp:" memory/memory-0000-latest.yaml
   ```

3. **Check Agent Execution**
   ```bash
   grep -A 5 "agent_history:" memory/memory-0000-latest.yaml
   ```

4. **Inspect Complete Flow**
   ```bash
   cat memory/memory-0000-latest.yaml
   ```

## References

- **Flight Implementation**: See `FLIGHT_AGENT_IMPLEMENTATION.md`
- **Migration Guide**: See `FLIGHT_API_MIGRATION.md`
- **Dynamic Travel Guide**: See `DYNAMIC_TRAVEL_WITH_FLIGHT_API.md`
- **Mock Server**: See `mock/README.md`
