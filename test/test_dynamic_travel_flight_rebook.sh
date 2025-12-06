#!/bin/bash

echo "=== Dynamic Travel System with Flight Rebook Test ==="
echo "Note: Assumes server is running (start with: make restart-all)"
echo ""

echo "Cleaning existing wants..."
curl -s http://localhost:8080/api/v1/wants | jq -r '.wants[].metadata.id' 2>/dev/null | while read ID; do
  if [[ ! -z "$ID" && "$ID" != "null" ]]; then
    curl -s -X DELETE "http://localhost:8080/api/v1/wants/$ID" > /dev/null 2>&1
  fi
done
sleep 1

echo ""
echo "=== Deploying Dynamic Travel Change Want ==="

# Deploy single dynamic travel change want
PAYLOAD='{"wants":[{"metadata":{"name":"dynamic_travel","type":"dynamic travel change"},"spec":{"params":{}}}]}'

RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/wants \
  -H "Content-Type: application/json" \
  -d "$PAYLOAD")

echo "Deployment response:"
echo "$RESPONSE" | jq '.'

WANT_ID=$(echo "$RESPONSE" | jq -r '.want_ids[0]' 2>/dev/null)
echo ""
echo "Want ID: $WANT_ID"

echo ""
echo "=== Waiting for All Wants to Achieve 'achieved' Status ==="
echo "This includes Flight's 60-second monitoring phase for rebook detection..."
echo ""

# Maximum wait time (70 seconds to allow 60-second Flight monitoring + buffer)
MAX_WAIT=70
ELAPSED=0
POLL_INTERVAL=2

# First, find the coordinator want
ALL_WANTS=$(curl -s http://localhost:8080/api/v1/wants)
COORDINATOR_ID=$(echo "$ALL_WANTS" | jq -r '.wants[] | select(.metadata.type == "coordinator") | .metadata.id' | tail -1)

if [[ -z "$COORDINATOR_ID" || "$COORDINATOR_ID" == "null" ]]; then
  echo "❌ ERROR: Could not find coordinator child want"
  echo "Available wants:"
  echo "$ALL_WANTS" | jq '.wants[] | {id: .metadata.id, type: .metadata.type}'
  exit 1
fi

echo "Coordinator ID: $COORDINATOR_ID"
echo ""

# Polling loop to wait for all wants to achieve "achieved" status
# We specifically wait for the coordinator to achieve since that's what matters for rebook detection
COORDINATOR_ACHIEVED=false
CHILD_WANTS_ACHIEVED=0
TARGET_CHILD_WANTS=3  # restaurant, hotel, buffet (excluding flight which may not reach achieved)

while [ $ELAPSED -lt $MAX_WAIT ]; do
  ALL_WANTS=$(curl -s http://localhost:8080/api/v1/wants)

  # Count coordinator achieved status
  COORDINATOR_STATUS=$(echo "$ALL_WANTS" | jq -r ".wants[] | select(.metadata.id == \"$COORDINATOR_ID\") | .status" 2>/dev/null)

  # Count key child wants
  HOTEL_STATUS=$(echo "$ALL_WANTS" | jq -r '.wants[] | select(.metadata.type == "hotel") | .status' 2>/dev/null)
  RESTAURANT_STATUS=$(echo "$ALL_WANTS" | jq -r '.wants[] | select(.metadata.type == "restaurant") | .status' 2>/dev/null)
  BUFFET_STATUS=$(echo "$ALL_WANTS" | jq -r '.wants[] | select(.metadata.type == "buffet") | .status' 2>/dev/null)
  FLIGHT_STATUS=$(echo "$ALL_WANTS" | jq -r '.wants[] | select(.metadata.type == "flight") | .status' 2>/dev/null)

  echo "[$(date '+%H:%M:%S')] Elapsed: ${ELAPSED}s - Coordinator: $COORDINATOR_STATUS, Hotel: $HOTEL_STATUS, Restaurant: $RESTAURANT_STATUS, Buffet: $BUFFET_STATUS, Flight: $FLIGHT_STATUS"

  # Check if coordinator has achieved (this is what matters for rebook detection)
  # Note: We need to verify it stays achieved, so we'll check multiple times
  if [[ "$COORDINATOR_STATUS" == "achieved" ]]; then
    # Wait a moment and verify coordinator is still achieved
    sleep 1
    VERIFY_STATUS=$(curl -s http://localhost:8080/api/v1/wants/$COORDINATOR_ID 2>/dev/null | jq -r '.status // "NOT_FOUND"')
    if [[ "$VERIFY_STATUS" == "achieved" ]]; then
      echo ""
      echo "✅ Coordinator has reached 'achieved' status (all schedules received and finalized)"
      COORDINATOR_ACHIEVED=true
      ELAPSED=$((ELAPSED + 1))  # Account for verification sleep
      break
    fi
  fi

  sleep $POLL_INTERVAL
  ELAPSED=$((ELAPSED + POLL_INTERVAL))
done

if [ "$COORDINATOR_ACHIEVED" != "true" ]; then
  echo ""
  echo "⚠️  Timeout after ${MAX_WAIT}s waiting for coordinator to achieve 'achieved' status"
  echo "Final state:"
  curl -s http://localhost:8080/api/v1/wants | jq '.wants[] | {id: .metadata.id, type: .metadata.type, status: .status}'
  exit 1
fi

echo ""
echo "=== Checking Coordinator Final State ==="
WANT_RESPONSE=$(curl -s http://localhost:8080/api/v1/wants/$COORDINATOR_ID)

FINAL_RESULT=$(echo "$WANT_RESPONSE" | jq -r '.state.finalResult // "NOT_FOUND"' 2>/dev/null)
STATUS=$(echo "$WANT_RESPONSE" | jq -r '.status // "NOT_FOUND"' 2>/dev/null)

echo ""
echo "=== Results ==="
echo "Want Status: $STATUS"
echo ""
echo "Final Result:"
echo "$FINAL_RESULT"
echo ""

# Check for rebook information in the final result
if echo "$FINAL_RESULT" | grep -q -i "rebook\|rebooking\|rebooked"; then
  echo "✅ SUCCESS: Coordinator finalResult contains REBOOK information"
  echo ""
  echo "Flight rebook detection was triggered successfully during the 60-second monitoring phase."
  exit 0
elif echo "$FINAL_RESULT" | grep -q "Flight:"; then
  echo "⚠️  PARTIAL SUCCESS: Coordinator has Flight information but NO rebook detected"
  echo ""
  echo "The coordinator received the flight schedule, but rebook was not triggered."
  echo "This could mean:"
  echo "  - Flight monitoring completed without detecting delays"
  echo "  - Or rebook was triggered but not captured in finalResult"
  exit 1
else
  echo "❌ FAILURE: Coordinator finalResult does not contain Flight or Rebook information"
  echo ""
  echo "Expected to find Flight and/or Rebook information in finalResult"
  echo ""

  echo "=== Want State Details ==="
  echo "$WANT_RESPONSE" | jq '.state'

  exit 1
fi
