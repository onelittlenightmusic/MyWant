# Flight Code Path Investigation - Debug Report

## Overview

Comprehensive debugging investigation into why the async retrigger mechanism is not being activated when Flight sends new packets during rebooking.

## Work Completed

### 1. Logging Added to Flight.Exec() Method
**File**: `engine/cmd/types/flight_types.go`

Added debug logging at key decision points in the Flight execution lifecycle:

**Lines 119-120**: Entry to monitoring phase
```go
if f.monitoringActive {
    InfoLog("[FLIGHT:EXEC] In monitoring phase\n")
```

**Line 134**: Cancellation and rebooking trigger detection
```go
if f.shouldCancelAndRebook() {
    InfoLog("[FLIGHT:EXEC] shouldCancelAndRebook returned true, initiating cancellation\n")
```

**Line 161**: Monitoring duration expiration
```go
} else {
    // Monitoring duration exceeded, complete the monitoring phase
    InfoLog("[FLIGHT:EXEC] Monitoring duration exceeded, completing\n")
```

**Line 168**: Normal execution phase
```go
InfoLog("[FLIGHT:EXEC] Monitoring not active, proceeding with normal execution\n")
```

**Lines 252-263**: Previous flight detected, rebooking started
```go
if hasPrevFlight && prevFlightID != nil && prevFlightID != "" {
    InfoLog("[FLIGHT:EXEC] Previous flight detected, preparing for rebooking\n")
    ...
    InfoLog("[FLIGHT:EXEC] Calling tryAgentExecution for rebooking\n")
    f.tryAgentExecution()
```

### 2. Logging Added to Retrigger Trigger Sending
**File**: `engine/cmd/types/flight_types.go`

Added comprehensive logging around BOTH trigger sends (initial agent execution and rebooking):

**Lines 298-305**: Trigger sending after rebooked flight (2 occurrences)
```go
// Trigger completed want retrigger check for any dependents
InfoLog("[FLIGHT:EXEC] About to trigger retrigger check\n")
cb := GetGlobalChainBuilder()
if cb != nil {
    InfoLog("[FLIGHT:EXEC] GlobalChainBuilder found, calling TriggerCompletedWantRetriggerCheck()\n")
    cb.TriggerCompletedWantRetriggerCheck()
    InfoLog("[FLIGHT:EXEC] TriggerCompletedWantRetriggerCheck() call completed\n")
} else {
    InfoLog("[FLIGHT:EXEC] ERROR: GlobalChainBuilder is nil!\n")
}
```

## Key Findings

### Log Output Status
- ❌ **NO [FLIGHT:EXEC] logs appear in any test run**
- ❌ **NO logs indicating code paths are being executed**
- ✅ **Flight IS executing** (shown by Flight state changes in API responses)
- ✅ **Flight IS rebooking** (shown by "Detected delayed_one_day status" logs)
- ✅ **Server IS logging** (other non-Flight debug logs appear in output)

### Critical Observation

The absence of ANY [FLIGHT:EXEC] logs, including the very first entry point log `[FLIGHT:EXEC] In monitoring phase` or `[FLIGHT:EXEC] Monitoring not active`, indicates one of two possibilities:

1. **The Exec() method itself is not being called** during Flight's execution cycles
2. **The logging calls are being optimized away** by the Go compiler (unlikely with InfoLog)
3. **The method exists but is a different implementation** than what we modified

### Code Path Analysis

**Expected sequence during rebooking**:
1. Flight.Exec() called by reconcile loop
2. Check `if f.monitoringActive` → should log [FLIGHT:EXEC] In monitoring phase
3. Detect delay via `shouldCancelAndRebook()` → should log [FLIGHT:EXEC] shouldCancelAndRebook returned true
4. Set `flight_action = "cancel_flight"`
5. Return false to continue monitoring
6. Next cycle: `previousFlightID` exists → should log [FLIGHT:EXEC] Previous flight detected
7. Call `tryAgentExecution()` for rebooking → should log [FLIGHT:EXEC] Calling tryAgentExecution
8. Agent returns result, send packet
9. Call `TriggerCompletedWantRetriggerCheck()` → should log [FLIGHT:EXEC] About to trigger retrigger check

**Actual sequence observed**:
- Flight state changes occur (detected via API)
- Flight sends packets (observed in Coordinator receiving them)
- ❌ **No [FLIGHT:EXEC] logs appear**
- ❌ **No [RETRIGGER:SEND] logs appear**
- ❌ Retrigger never triggered

### Server Startup and Logging

**Log location**: `/Users/hiroyukiosaki/work/golang/MyWant/engine/logs/mywant-backend.log`

**Server startup command**:
```bash
cd /Users/hiroyukiosaki/work/golang/MyWant/engine
./bin/mywant 8080 localhost > logs/mywant-backend.log 2>&1
```

**Verified**:
- ✅ Server starts with new binary after rebuild
- ✅ Logging infrastructure is working (other debug logs appear)
- ✅ InfoLog() calls in other code successfully write to log file

### Testing Issues Encountered

1. **Test script deletes logs**: The `dynamic_travel_retrigger_test.sh` script deletes logs at start, making it hard to capture output
2. **Server working directory matters**: Server must be started from `engine/` directory for logs to be created at the correct location
3. **Type registration**: "dynamic travel change" IS registered as a custom type, but deployment may require different metadata format

## Hypothesis

### Most Likely Cause
The `Flight.Exec()` method being called is NOT the same `Exec()` method we modified in `flight_types.go`.

**Why?**:
- The absence of logging at the very first line of Exec() suggests the method isn't being called, OR
- A different Flight implementation is being used, OR
- The code path uses a different want type at runtime

### Alternative Causes
1. **Interface method**: There might be an interface-based call to Exec() that bypasses our concrete implementation
2. **Runtime delegation**: The want system might be delegating to a different execution method
3. **Code not rebuilt**: Though unlikely since other recent changes appear in the binary

## Recommendations for Next Phase

### Immediate Actions
1. **Verify FlightWant type existence**: Check if the FlightWant we modified is actually the one being instantiated
   - Search for all Exec() implementations that could be called
   - Add logging to the FlightWant constructor to confirm creation

2. **Verify method call**: Add logging to the Want reconciliation loop where Exec() is called to confirm FlightWant.Exec() is invoked

3. **Check interface compliance**: Ensure FlightWant properly implements the Want interface for Exec()

### Debugging Strategy
1. Add logging to `chain_builder.go::ReconcileLoop()` at the exact point where want.Exec() is called, with the want type name
2. Add logging to FlightWant struct creation to confirm instantiation
3. Add logging at the START of every code path in Exec() to ensure we catch execution
4. Use `printf` style logging in addition to InfoLog() to ensure output

## Files Modified

- `engine/cmd/types/flight_types.go`: Added [FLIGHT:EXEC] debug logging at all major code path decision points

## Test Scenario

The test uses the "dynamic travel change" recipe which:
1. Creates Flight, Restaurant, Hotel, Buffet, and Coordinator wants
2. Flight starts with AA100
3. After 15 seconds, Coordinator receives all packets and reaches "achieved" status
4. Flight monitoring detects delay and initiates rebooking
5. Flight should send new AA100A/AA100B packets
6. Coordinator should receive them via retrigger and update its state
7. **Current behavior**: Coordinator state remains unchanged

## Conclusion

Despite comprehensive logging additions, the complete absence of [FLIGHT:EXEC] logs suggests the modified Exec() method is not being called at runtime. This requires deeper investigation into:
1. Which Exec() implementation is actually being executed
2. How wants are instantiated and executed during reconciliation
3. Whether the FlightWant type is correctly registered and being used

The async retrigger infrastructure itself appears sound (reconcile loop receives other triggers), but the specific code path that should trigger it (Flight sending new packets) is not executing the instrumented code.
