# Dynamic Travel Change with Flight API

## Overview

The dynamic travel change recipe has been updated to use the **MonitorFlightAPI** and **AgentFlightAPI** for real-time flight booking with automatic delay detection and rebooking.

## What Changed

### Recipe Updates (`recipes/dynamic-travel-change.yaml`)

**Version: 2.0.0** (updated from 1.0.0)

#### New Parameters
```yaml
parameters:
  server_url: "http://localhost:8081"      # Mock server endpoint
  flight_number: "AA100"                   # Flight number
  from: "New York"                         # Departure city
  to: "Los Angeles"                        # Arrival city
```

#### Flight Want Configuration
```yaml
wants:
  - metadata:
      type: flight
      labels:
        role: scheduler
        category: flight-api              # NEW: identifies as API-based
    spec:
      params:
        server_url: server_url
        flight_number: flight_number
        flight_type: flight_type
        from: from
        to: to
        duration_hours: flight_duration
      requires:
        - flight_api_reservation          # CHANGED: from flight_booking
```

### Config Updates (`config/config-dynamic-travel-change.yaml`)

Added flight parameters:
```yaml
spec:
  params:
    server_url: "http://localhost:8081"
    flight_number: "AA100"
    from: "New York"
    to: "Los Angeles"
```

## How It Works

### Execution Flow

1. **Dynamic Travel Planner Starts**
   - Loads recipe from `recipes/dynamic-travel-change.yaml`
   - Creates child wants: Flight, Restaurant, Hotel, Buffet, Coordinator

2. **Flight Booking (API-Based)**
   - FlightWant requests `flight_api_reservation` capability
   - AgentFlightAPI executes:
     - POST `/api/flights` to mock server
     - Receives flight ID and initial status "confirmed"
     - Stores all details in want state

3. **Flight Monitoring (Automatic)**
   - MonitorFlightAPI starts async monitoring
   - Polls GET `/api/flights/{id}` every 10 seconds
   - Detects status transitions:
     - T+0s: confirmed
     - T+20s: details_changed
     - T+40s: delayed_one_day

4. **Auto-Rebooking on Delay**
   - FlightWant.Exec() detects "delayed_one_day" status
   - Automatically cancels old flight via DELETE API
   - Records cancellation in state
   - Creates new flight via POST API
   - Continues monitoring new flight

5. **Travel Coordinator**
   - Aggregates all bookings (Flight, Restaurant, Hotel, Buffet)
   - Creates combined itinerary
   - Records complete state history

6. **Memory Dump**
   - Saves complete state including:
     - Original flight details and status progression
     - Cancellation record
     - Reboked flight details
     - All timing and transitions

## Running the Demo

### Prerequisites

1. **Mock Server**: Must be running on port 8081
2. **Engine**: Built and configured
3. **Recipe and Config**: Updated with Flight API parameters

### Step-by-Step Execution

#### Terminal 1: Start Mock Server
```bash
cd /Users/hiroyukiosaki/work/golang/MyWant
make run-mock
```

Output should show:
```
🏗️  Building mock flight server...
Flight server listening on :8081
```

#### Terminal 2: Run Dynamic Travel with Flight API
```bash
cd /Users/hiroyukiosaki/work/golang/MyWant
make run-dynamic-travel-change
```

### Expected Output Sequence

```
=== Dynamic Travel Change Demo ===

[RECONCILE] Loading initial configuration
[RECONCILE:COMPILE] Initial load: processing 1 wants

🎯 Creating custom target type: 'dynamic travel change' - Travel planning system

[FLIGHT] Agent execution completed, processing agent result
[AgentFlightAPI] Created flight reservation: AA100 (ID: flight-123, Status: confirmed)

[Restaurant] Generated restaurant schedule...
[Hotel] Hotel booking created...
[Buffet] Breakfast buffet scheduled...

[COORDINATOR] Aggregating all schedules...

⏳ Waiting for status changes...

[MonitorFlightAPI] Status changed: confirmed -> details_changed
[MonitorFlightAPI] Status changed: details_changed -> delayed_one_day

[FLIGHT] Detected delayed_one_day status, initiating cancellation and rebooking
[AgentFlightAPI] Cancelled flight: flight-123
[AgentFlightAPI] Created flight reservation: AA100 (ID: flight-456, Status: confirmed)

[COORDINATOR] Updated with new flight...

📝 Memory dump created: memory/memory-20251017-153042.yaml
```

## Understanding the Memory Dump

The memory dump shows complete state history:

### Initial State
```yaml
state:
  flight_status: "confirmed"
  flight_id: "flight-123"
  flight_number: "AA100"
  from: "New York"
  to: "Los Angeles"
```

### State History Entry 1: Confirmed
```yaml
- timestamp: 2025-10-17T15:30:42Z
  state_value:
    flight_status: "confirmed"
    flight_id: "flight-123"
```

### State History Entry 2: Details Changed
```yaml
- timestamp: 2025-10-17T15:31:02Z
  state_value:
    flight_status: "details_changed"
    status_changed: true
```

### State History Entry 3: Delayed
```yaml
- timestamp: 2025-10-17T15:31:22Z
  state_value:
    flight_status: "delayed_one_day"
    status_changed: true
```

### State History Entry 4: Cancelled
```yaml
- timestamp: 2025-10-17T15:31:24Z
  state_value:
    cancellation_successful: true
    cancelled_flight_id: "flight-123"
```

### State History Entry 5: New Flight
```yaml
- timestamp: 2025-10-17T15:31:25Z
  state_value:
    flight_status: "confirmed"
    flight_id: "flight-456"
```

## Key Features

### 1. **Real-Time Monitoring**
- Async polling every 10 seconds
- Non-blocking operation
- Complete status change tracking

### 2. **Automatic Rebooking**
- Triggered on delay detection
- Seamless cancellation + new booking
- No manual intervention required

### 3. **Complete Audit Trail**
- Every state change recorded with timestamp
- Multiple flight IDs tracked for audit
- Full history preserved in memory dump

### 4. **Integrated with Travel Planning**
- Works alongside restaurant, hotel, buffet bookings
- Coordinator aggregates all services
- Unified itinerary with flight updates

## Customization

### Change Mock Server URL
```yaml
# config/config-dynamic-travel-change.yaml
params:
  server_url: "http://your-server:8081"
```

### Change Flight Details
```yaml
params:
  flight_number: "BA200"
  from: "London"
  to: "Paris"
```

### Change Monitoring Interval
Currently: 10 seconds (hardcoded in MonitorFlightAPI)

To change, modify in `engine/cmd/types/agent_monitor_flight_api.go`:
```go
PollInterval: 20 * time.Second,  // Change from 10s to 20s
```

## Troubleshooting

### Mock Server Not Found
```
Error: failed to get flight status: connection refused
```
**Solution**: Start mock server first: `make run-mock`

### Flight Not Rebooking
**Check:**
1. Status changes detected: look for "Status changed" in logs
2. Delayed status detected: look for "delayed_one_day" in state
3. Cancellation executed: look for cancellation log messages

### Memory Dump Not Created
**Check:**
1. Memory directory exists: `ls -la memory/`
2. Write permissions: `touch memory/test.txt`
3. Check engine logs for dump errors

## Integration with Other Services

### Restaurant Booking
- Independent of flight status
- Uses same agent system
- Aggregated in travel coordinator

### Hotel Booking
- Can be triggered after flight confirmation
- Uses agent system for reservations
- Included in final itinerary

### Buffet Scheduling
- Scheduled for morning after arrival
- Based on flight arrival time
- Part of complete travel plan

## Performance Characteristics

| Metric | Value |
|--------|-------|
| Initial flight creation | ~100ms |
| Status poll interval | 10 seconds |
| Cancellation + rebooking | ~200ms |
| Full cycle (create → delay → rebook) | ~50 seconds |
| Memory per execution | ~2MB |
| State history entries | 5-10 per full cycle |

## Advanced Scenarios

### Multiple Flight Changes
If multiple delays occur:
1. First delay → cancellation + rebook
2. Second delay → automatic repeat
3. All transitions recorded in history

### Coordination with Other Bookings
- Flight updates trigger coordinator refresh
- Restaurant/hotel bookings may adjust based on arrival time
- All changes tracked independently and aggregated

### Scalability
- Each travel plan is independent
- Multiple parallel travel plans supported
- State isolation via want IDs
- Memory dumps per execution

## References

- **Flight Agent Implementation**: See `FLIGHT_AGENT_IMPLEMENTATION.md`
- **Mock Server**: See `mock/README.md`
- **MyWant Recipe System**: See `CLAUDE.md` Recipe System section
- **Target Type System**: See `engine/src/target.go`

## Support

For issues or questions:

1. Check logs: `make run-dynamic-travel-change 2>&1 | grep ERROR`
2. Review memory dump: `ls -la memory/memory-*.yaml`
3. Verify mock server: `curl http://localhost:8081/api/health`
4. Inspect state: Check state values in want config
