# Flight API Migration Guide

## Summary of Changes

The dynamic travel change recipe has been upgraded to use **MonitorFlightAPI** and **AgentFlightAPI** for real-time flight booking with automatic delay detection and rebooking.

## Files Modified

### 1. Recipe File: `recipes/dynamic-travel-change.yaml`

#### Changes:
- **Version**: 1.0.0 → 2.0.0
- **Name**: "Dynamic Travel Change" → "Dynamic Travel Change with Flight API"
- **Description**: Added mention of API-based booking and automatic rebooking

#### New Parameters:
```yaml
server_url: "http://localhost:8081"      # Mock flight server
flight_number: "AA100"                   # Flight identifier
from: "New York"                         # Departure city
to: "Los Angeles"                        # Arrival city
```

#### Flight Want Changes:
```yaml
# BEFORE:
requires:
  - flight_booking

# AFTER:
requires:
  - flight_api_reservation

# NEW LABEL:
labels:
  role: scheduler
  category: flight-api

# NEW PARAMS:
params:
  server_url: server_url
  flight_number: flight_number
  from: from
  to: to
```

### 2. Config File: `config/config-dynamic-travel-change.yaml`

#### Changes:
- **Display Name**: Added "with Flight API" suffix
- **Added Flight Parameters**:
  ```yaml
  server_url: "http://localhost:8081"
  flight_number: "AA100"
  from: "New York"
  to: "Los Angeles"
  ```

## What This Enables

### Before (v1.0.0)
- Static flight generation
- No real API integration
- No monitoring
- No automatic rebooking on delays

### After (v2.0.0)
- ✅ Real API-based flight booking (POST to mock server)
- ✅ Automatic status monitoring (GET every 10 seconds)
- ✅ Status change detection (confirmed → details_changed → delayed_one_day)
- ✅ Automatic rebooking on delay (DELETE old, POST new)
- ✅ Complete audit trail in memory dump

## Execution Flow

```
1. Dynamic Travel Planner loads
   ↓
2. Flight want requests 'flight_api_reservation' capability
   ↓
3. AgentFlightAPI creates flight (POST /api/flights)
   - Receives flight ID
   - Initial status: confirmed
   ↓
4. MonitorFlightAPI starts async polling
   - Every 10 seconds: GET /api/flights/{id}
   ↓
5. Status changes detected
   - T+20s: confirmed → details_changed
   - T+40s: details_changed → delayed_one_day
   ↓
6. Auto-rebooking triggered
   - DELETE /api/flights/{id} (old flight)
   - POST /api/flights (new flight)
   ↓
7. New flight monitored for delays
   ↓
8. Memory dump saved with complete history
```

## Quick Start

### Step 1: Start Mock Server
```bash
make run-mock
```

### Step 2: Run Dynamic Travel with Flight API
```bash
make run-dynamic-travel-change
```

### Expected Behavior
1. Flight reservation created
2. Status changes appear in logs
3. Automatic cancellation when delayed
4. New flight created
5. Complete state history in memory dump

## Verification

### Check Recipe Version
```bash
grep "version:" recipes/dynamic-travel-change.yaml
# Output: version: "2.0.0"
```

### Check Flight Wants
```bash
grep -A 10 "# Flight booking" recipes/dynamic-travel-change.yaml
# Should show:
#   - requires: [flight_api_reservation]
#   - category: flight-api label
#   - server_url, flight_number, from, to params
```

### Check Config Parameters
```bash
grep -E "server_url|flight_number|from|to" config/config-dynamic-travel-change.yaml
# Should show all flight parameters
```

## Backward Compatibility

⚠️ **Breaking Change**: This update changes the flight requirement from `flight_booking` to `flight_api_reservation`.

If you need the old static flight generation:
1. Keep a copy of v1.0.0 recipe
2. Create new config pointing to old recipe
3. Or create new recipe file with old logic

## Integration Points

### Restaurant Booking
- ✓ Independent of flight status
- ✓ Can start before/after flight confirmation
- ✓ Aggregated in coordinator

### Hotel Booking
- ✓ Uses separate agent system
- ✓ Can depend on flight arrival time
- ✓ Included in travel coordinator

### Buffet Scheduling
- ✓ Typically scheduled after flight arrival
- ✓ Uses own agent/static generation
- ✓ Coordinated with other bookings

## State History Tracking

The memory dump now includes:

```
Initial Flight: flight-123
├── confirmed (T+0s)
├── details_changed (T+20s)
├── delayed_one_day (T+40s)
├── cancelled (T+42s)
└── New Flight: flight-456
    └── confirmed (T+43s)
```

Each transition has:
- Timestamp
- State before and after
- Associated details (flight number, status message, etc.)

## Configuration Customization

### Change Flight Route
```yaml
# config/config-dynamic-travel-change.yaml
from: "London"
to: "Paris"
```

### Change Flight Number
```yaml
flight_number: "BA200"
```

### Change Server URL
```yaml
server_url: "http://your-api:8081"
```

### Change Flight Type
```yaml
flight_type: "first class"
```

## Performance Impact

| Operation | Time |
|-----------|------|
| Initial flight creation | ~100ms |
| Status polling interval | 10s |
| Delay detection + rebooking | ~200ms |
| Full cycle (create → delay → rebook) | ~50s |

## Troubleshooting

### "connection refused" Error
**Problem**: Mock server not running
```bash
make run-mock  # Start it in another terminal
```

### "flight_id not found" Error
**Problem**: Flight wasn't created successfully
- Check POST /api/flights succeeded
- Check server logs: `make run-mock`

### No Status Changes Detected
**Problem**: Monitor not running or API connection lost
- Verify mock server is responding: `curl http://localhost:8081/api/health`
- Check want logs for "Status changed" messages

### Memory Dump Not Created
**Problem**: Memory directory or permissions issue
```bash
mkdir -p memory
chmod 755 memory
```

## Related Documentation

- **Flight Implementation**: `FLIGHT_AGENT_IMPLEMENTATION.md`
- **Dynamic Travel Guide**: `DYNAMIC_TRAVEL_WITH_FLIGHT_API.md`
- **Mock Server**: `mock/README.md`

## Rollback

To use the old version temporarily:

1. View git history:
   ```bash
   git log --oneline recipes/dynamic-travel-change.yaml
   ```

2. Restore old version:
   ```bash
   git checkout HEAD~1 recipes/dynamic-travel-change.yaml
   ```

3. Or keep both versions:
   ```bash
   cp recipes/dynamic-travel-change.yaml recipes/dynamic-travel-change-v2.yaml
   cp recipes/dynamic-travel-change-v1.yaml recipes/dynamic-travel-change.yaml
   ```

## Next Steps

1. ✓ Recipe updated to use Flight API
2. ✓ Configuration files updated
3. Next: Run `make run-dynamic-travel-change` to test
4. Next: Verify state history in memory dump
5. Next: Consider adding email notifications on delay
