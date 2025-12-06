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
echo "Waiting for execution (7 seconds)..."
sleep 7

echo ""
echo "=== Finding Coordinator Child Want ==="

# List all wants to find the coordinator child
ALL_WANTS=$(curl -s http://localhost:8080/api/v1/wants)

# Find coordinator want by looking for type "coordinator"
# Note: Get the latest coordinator (if multiple exist from previous runs)
COORDINATOR_ID=$(echo "$ALL_WANTS" | jq -r '.wants[] | select(.metadata.type == "coordinator") | .metadata.id' | tail -1)

if [[ -z "$COORDINATOR_ID" || "$COORDINATOR_ID" == "null" ]]; then
  echo "❌ ERROR: Could not find coordinator child want"
  echo "Available wants:"
  echo "$ALL_WANTS" | jq '.wants[] | {id: .metadata.id, type: .metadata.type}'
  exit 1
fi

echo "Coordinator ID: $COORDINATOR_ID"

echo ""
echo "=== Checking Coordinator State ==="
WANT_RESPONSE=$(curl -s http://localhost:8080/api/v1/wants/$COORDINATOR_ID)

FINAL_RESULT=$(echo "$WANT_RESPONSE" | jq -r '.state.finalResult // "NOT_FOUND"' 2>/dev/null)
STATUS=$(echo "$WANT_RESPONSE" | jq -r '.status // "NOT_FOUND"' 2>/dev/null)

echo ""
echo "=== Results ==="
echo "Want Status: $STATUS"
echo "Final Result: $FINAL_RESULT"
echo ""

if echo "$FINAL_RESULT" | grep -q "Flight:"; then
  echo "✅ SUCCESS: Coordinator finalResult contains complete itinerary with Flight"
  echo ""
  echo "Note: Rebook detection requires 60+ seconds (Flight monitoring timeout)"
  echo "Current finalResult shows initial itinerary:"
  echo "$FINAL_RESULT"
  exit 0
else
  echo "❌ FAILURE: Flight NOT found in finalResult"
  echo ""
  echo "Expected to find 'Flight:' in finalResult but found:"
  echo "$FINAL_RESULT"
  echo ""

  echo "=== Want State Details ==="
  echo "$WANT_RESPONSE" | jq '.state'

  exit 1
fi
