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
echo "Waiting for execution (10 seconds)..."
sleep 10

echo ""
echo "=== Checking Want State ==="
WANT_RESPONSE=$(curl -s http://localhost:8080/api/v1/wants/$WANT_ID)

FINAL_RESULT=$(echo "$WANT_RESPONSE" | jq -r '.state.finalResult // "NOT_FOUND"' 2>/dev/null)
STATUS=$(echo "$WANT_RESPONSE" | jq -r '.status // "NOT_FOUND"' 2>/dev/null)

echo ""
echo "=== Results ==="
echo "Want Status: $STATUS"
echo "Final Result: $FINAL_RESULT"
echo ""

if echo "$FINAL_RESULT" | grep -q "Rebook"; then
  echo "✅ SUCCESS: Rebook found in finalResult"
  exit 0
else
  echo "❌ FAILURE: Rebook NOT found in finalResult"
  echo ""
  echo "Expected to find 'Rebook' in finalResult but found:"
  echo "$FINAL_RESULT"
  echo ""

  echo "=== Want State Details ==="
  echo "$WANT_RESPONSE" | jq '.state'

  exit 1
fi
