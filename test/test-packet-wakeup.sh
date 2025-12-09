#!/bin/bash

# Test automatic wake-up of completed wants when receiving new packets

API_BASE="http://localhost:8080/api/v1"

echo "=== Testing Automatic Packet Wake-up Mechanism ==="
echo ""

# Step 1: Wait for initial execution to complete
echo "Step 1: Waiting for initial execution to complete (10 seconds)..."
sleep 10

# Step 2: Check that all wants are completed
echo ""
echo "Step 2: Checking want statuses..."
WANTS=$(curl -s "${API_BASE}/wants" | jq -r '.wants[] | "\(.metadata.name): \(.status)"')
echo "$WANTS"

# Step 3: Update qnet number parameter to trigger restart
echo ""
echo "Step 3: Updating qnet_number count parameter to 50 (will trigger restart)..."
curl -s -X PUT "${API_BASE}/wants/qnet_number/params" \
  -H "Content-Type: application/json" \
  -d '{"count": 50}' | jq '.'

# Step 4: Wait a bit and monitor for automatic wake-ups
echo ""
echo "Step 4: Monitoring for automatic wake-ups (should see MONITOR and TRIGGER:PACKET logs)..."
sleep 3

# Step 5: Check that downstream wants automatically restarted
echo ""
echo "Step 5: Checking if downstream wants (qnet_queue, qnet_sink) automatically woke up..."
WANTS_AFTER=$(curl -s "${API_BASE}/wants" | jq -r '.wants[] | "\(.metadata.name): \(.status)"')
echo "$WANTS_AFTER"

# Step 6: Wait for processing to complete
echo ""
echo "Step 6: Waiting for new processing to complete..."
sleep 5

# Step 7: Check final statuses
echo ""
echo "Step 7: Final want statuses:"
curl -s "${API_BASE}/wants" | jq -r '.wants[] | "\(.metadata.name): \(.status)"'

echo ""
echo "=== Test Complete ==="
echo ""
echo "Expected behavior:"
echo "  1. qnet_number restarts when parameter changes (parameter trigger)"
echo "  2. qnet_queue automatically wakes up when qnet_number sends packets (packet trigger)"
echo "  3. qnet_sink automatically wakes up when qnet_queue sends packets (packet trigger)"
echo ""
echo "Check server logs for [MONITOR] and [TRIGGER:PACKET] messages to verify automatic wake-ups!"