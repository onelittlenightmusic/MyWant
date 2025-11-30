#!/bin/bash
# Test Scenario: Dynamic Travel Change with Async Retrigger
#
# Purpose: Verify that the async retrigger mechanism works correctly when:
# 1. Flight generates initial packet (AA100)
# 2. Coordinator receives it and completes
# 3. Flight detects delay and rebbooks (AA100B, AA100A, etc.)
# 4. Coordinator receives new packets via async retrigger
#
# Expected behavior:
# - Coordinator's final_itinerary should update from AA100 to AA100A
# - total_processed should increment from 4 to higher values
# - State history should show multiple updates

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TEST_RESULTS_DIR="$PROJECT_ROOT/test_results"
mkdir -p "$TEST_RESULTS_DIR"

TEST_NAME="dynamic_travel_retrigger"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
RESULTS_FILE="$TEST_RESULTS_DIR/${TEST_NAME}_${TIMESTAMP}.json"
LOG_FILE="$TEST_RESULTS_DIR/${TEST_NAME}_${TIMESTAMP}.log"

echo "=== Dynamic Travel Retrigger Test ===" | tee "$LOG_FILE"
echo "Start time: $(date)" | tee -a "$LOG_FILE"
echo "Results file: $RESULTS_FILE" | tee -a "$LOG_FILE"
echo "" | tee -a "$LOG_FILE"

# Configuration
SERVER_URL="http://localhost:8080"
WAIT_INITIAL=15
WAIT_REBOOKING=20
TOTAL_WAIT=$((WAIT_INITIAL + WAIT_REBOOKING))

# Create deployment config
DEPLOY_CONFIG=$(cat <<'CONFIGEOF'
{
  "wants": [
    {
      "metadata": {
        "name": "dynamic-travel",
        "type": "dynamic travel change",
        "labels": {
          "role": "dynamic-travel-planner"
        }
      },
      "spec": {
        "params": {
          "prefix": "dynamic-travel",
          "display_name": "Dynamic Travel Itinerary with Flight API",
          "server_url": "http://localhost:8081",
          "flight_number": "AA100",
          "flight_type": "business class",
          "from": "New York",
          "to": "Los Angeles",
          "departure_date": "2026-12-20",
          "flight_duration": 12.0,
          "restaurant_type": "fine dining",
          "hotel_type": "luxury",
          "buffet_type": "international",
          "dinner_duration": 2.0
        }
      }
    }
  ]
}
CONFIGEOF
)

# Initialize results
RESULTS='{
  "test_name": "'$TEST_NAME'",
  "timestamp": "'$TIMESTAMP'",
  "duration_seconds": 0,
  "phases": {}
}'

# Helper function to add result
add_result() {
  local phase=$1
  local data=$2
  RESULTS=$(echo "$RESULTS" | jq ".phases.\"$phase\" = $data")
}

# Phase 1: Deploy (logs not cleared to preserve execution logs)
echo "[PHASE 1] Deploying..." | tee -a "$LOG_FILE"
# Note: mywant-backend.log is NOT deleted to preserve test execution logs

DEPLOY_RESPONSE=$(curl -s -X POST "$SERVER_URL/api/v1/wants" \
  -H "Content-Type: application/json" \
  -d "$DEPLOY_CONFIG")

echo "Deployment response: $DEPLOY_RESPONSE" | tee -a "$LOG_FILE"
add_result "deployment" "$DEPLOY_RESPONSE"

# Phase 2: Initial execution (Flight sends AA100)
echo "" | tee -a "$LOG_FILE"
echo "[PHASE 2] Waiting $WAIT_INITIAL seconds for initial execution..." | tee -a "$LOG_FILE"
sleep "$WAIT_INITIAL"

INITIAL_STATE=$(curl -s "$SERVER_URL/api/v1/wants" | jq '.wants[] | select(.metadata.name == "dynamic-travel-coordinator-5") | {
  status: .status,
  flight: (.state.final_itinerary[] | select(.Type == "flight") | .Name),
  total_processed: .state.total_processed
}')

echo "Coordinator state after initial execution:" | tee -a "$LOG_FILE"
echo "$INITIAL_STATE" | tee -a "$LOG_FILE"
add_result "initial_state" "$INITIAL_STATE"

# Phase 3: Wait for Flight rebooking
echo "" | tee -a "$LOG_FILE"
echo "[PHASE 3] Waiting $WAIT_REBOOKING seconds for Flight rebooking..." | tee -a "$LOG_FILE"

# Get the flight ID first
FLIGHT_ID=$(curl -s "$SERVER_URL/api/v1/wants" | jq -r '.wants[] | select(.metadata.name == "dynamic-travel-flight-1") | .state.flight_id')
echo "  Flight ID: $FLIGHT_ID" | tee -a "$LOG_FILE"

if [ -n "$FLIGHT_ID" ] && [ "$FLIGHT_ID" != "null" ]; then
  echo "  Requesting flight delay from mock server..." | tee -a "$LOG_FILE"
  curl -s -X POST "http://localhost:8081/api/flights/$FLIGHT_ID/status" \
    -H "Content-Type: application/json" \
    -d '{"status":"delayed_one_day"}' > /dev/null 2>&1 || true
  echo "  Delay status requested" | tee -a "$LOG_FILE"
fi

sleep "$WAIT_REBOOKING"

FLIGHT_FINAL=$(curl -s "$SERVER_URL/api/v1/wants" | jq '.wants[] | select(.metadata.name == "dynamic-travel-flight-1") | {
  status: .status,
  flight_number: .state.flight_number,
  total_processed: .state.total_processed
}')

COORDINATOR_FINAL=$(curl -s "$SERVER_URL/api/v1/wants" | jq '.wants[] | select(.metadata.name == "dynamic-travel-coordinator-5") | {
  status: .status,
  flight: (.state.final_itinerary[] | select(.Type == "flight") | .Name),
  total_processed: .state.total_processed,
  state_history_count: (.history.stateHistory | length)
}')

echo "Final Flight state:" | tee -a "$LOG_FILE"
echo "$FLIGHT_FINAL" | tee -a "$LOG_FILE"
echo "" | tee -a "$LOG_FILE"
echo "Final Coordinator state:" | tee -a "$LOG_FILE"
echo "$COORDINATOR_FINAL" | tee -a "$LOG_FILE"

add_result "flight_final" "$FLIGHT_FINAL"
add_result "coordinator_final" "$COORDINATOR_FINAL"

# Phase 4: Verify retrigger worked
echo "" | tee -a "$LOG_FILE"
echo "[PHASE 4] Verifying async retrigger behavior..." | tee -a "$LOG_FILE"

FLIGHT_NUM=$(echo "$FLIGHT_FINAL" | jq -r '.flight_number')
COORD_FLIGHT=$(echo "$COORDINATOR_FINAL" | jq -r '.flight')
INITIAL_FLIGHT=$(echo "$INITIAL_STATE" | jq -r '.flight')

if [[ "$COORD_FLIGHT" != "$INITIAL_FLIGHT" ]]; then
  echo "✅ SUCCESS: Coordinator flight changed from '$INITIAL_FLIGHT' to '$COORD_FLIGHT'" | tee -a "$LOG_FILE"
  RETRIGGER_SUCCESS=true
else
  echo "❌ FAILURE: Coordinator still shows '$COORD_FLIGHT' (no change from initial)" | tee -a "$LOG_FILE"
  RETRIGGER_SUCCESS=false
fi

if [ "$RETRIGGER_SUCCESS" = true ]; then
  echo "❓ Note: Flight is '$FLIGHT_NUM' but Coordinator shows '$COORD_FLIGHT'" | tee -a "$LOG_FILE"
fi

# Save complete results
RESULTS=$(echo "$RESULTS" | jq ".retrigger_success = $RETRIGGER_SUCCESS")
RESULTS=$(echo "$RESULTS" | jq ".duration_seconds = $TOTAL_WAIT")

echo "$RESULTS" | jq '.' > "$RESULTS_FILE"

echo "" | tee -a "$LOG_FILE"
echo "=== Test Complete ===" | tee -a "$LOG_FILE"
echo "Results saved to: $RESULTS_FILE" | tee -a "$LOG_FILE"
echo "Log saved to: $LOG_FILE" | tee -a "$LOG_FILE"
echo "End time: $(date)" | tee -a "$LOG_FILE"

# Return exit code based on success
[ "$RETRIGGER_SUCCESS" = true ]
