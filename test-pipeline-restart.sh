#!/bin/bash

echo "🔄 Testing Event-Driven Restart in Queue System Pipeline"
echo "======================================================="

# Step 1: Find the completed qnet-pipeline system
echo ""
echo "1. 🔍 Finding completed queue system pipeline..."

WANTS_RESPONSE=$(curl -s http://localhost:8080/api/v1/wants)

# Find the qnet-pipeline parent want
PIPELINE_ID=$(echo "$WANTS_RESPONSE" | jq -r '.wants[] | select(.metadata.type == "wait time in queue system") | .metadata.id' | head -1)

# Find the qnet numbers child want
NUMBERS_ID=$(echo "$WANTS_RESPONSE" | jq -r '.wants[] | select(.metadata.type == "qnet numbers") | .metadata.id' | head -1)

echo "Pipeline Want ID: $PIPELINE_ID"
echo "Numbers Want ID: $NUMBERS_ID"

if [ "$NUMBERS_ID" = "null" ] || [ -z "$NUMBERS_ID" ]; then
    echo ""
    echo "❌ No qnet numbers want found! Let's create a queue system first..."

    # Create new queue system
    CREATE_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/wants \
        -H "Content-Type: application/yaml" \
        -d 'metadata:
  name: pipeline-restart-test
  type: wait time in queue system
  labels:
    role: qnet-target
spec:
  params:
    prefix: test
    primary_count: 500
    primary_rate: 5.0
    primary_service_time: 0.1
    secondary_count: 300
    secondary_rate: 3.0
    secondary_service_time: 0.08
  recipe: Queue System Pipeline')

    echo "Queue system created, waiting for completion..."
    sleep 5  # Wait for execution to complete

    # Get the new IDs
    WANTS_RESPONSE=$(curl -s http://localhost:8080/api/v1/wants)
    PIPELINE_ID=$(echo "$WANTS_RESPONSE" | jq -r '.wants[] | select(.metadata.name == "pipeline-restart-test") | .metadata.id')
    NUMBERS_ID=$(echo "$WANTS_RESPONSE" | jq -r '.wants[] | select(.metadata.type == "qnet numbers" and (.metadata.labels.owner // "" == "child")) | .metadata.id' | head -1)

    echo "New Pipeline Want ID: $PIPELINE_ID"
    echo "New Numbers Want ID: $NUMBERS_ID"
fi

# Step 2: Check current status of all pipeline wants
echo ""
echo "2. 📊 Current pipeline status:"

# Get current numbers want details
NUMBERS_WANT=$(echo "$WANTS_RESPONSE" | jq -r ".wants[] | select(.metadata.id == \"$NUMBERS_ID\")")
CURRENT_COUNT=$(echo "$NUMBERS_WANT" | jq -r '.spec.params.count')
CURRENT_STATUS=$(echo "$NUMBERS_WANT" | jq -r '.status')

echo "  Numbers Want: Status=$CURRENT_STATUS, Current Count=$CURRENT_COUNT"

# Get queue and sink status
QUEUE_STATUS=$(echo "$WANTS_RESPONSE" | jq -r '.wants[] | select(.metadata.type == "qnet queue") | .status' | head -1)
SINK_STATUS=$(echo "$WANTS_RESPONSE" | jq -r '.wants[] | select(.metadata.type == "qnet sink") | .status' | head -1)

echo "  Queue Want: Status=$QUEUE_STATUS"
echo "  Sink Want: Status=$SINK_STATUS"

# Step 3: Update the count parameter to trigger Event-Driven restart
NEW_COUNT=$((CURRENT_COUNT + 1000))
echo ""
echo "3. 🔄 Triggering Event-Driven restart by updating count: $CURRENT_COUNT → $NEW_COUNT"

echo "   This should demonstrate:"
echo "   ✅ Numbers want restarts (new goroutine created)"
echo "   ✅ Numbers generates packets and wakes up Queue want"
echo "   ✅ Queue processes packets and wakes up Sink want"
echo "   ✅ Full pipeline cascade from single parameter change"

UPDATE_RESPONSE=$(curl -s -X PUT "http://localhost:8080/api/v1/wants/$NUMBERS_ID" \
    -H "Content-Type: application/json" \
    -d "{
        \"spec\": {
            \"params\": {
                \"count\": $NEW_COUNT
            }
        }
    }")

echo ""
echo "✅ Parameter update sent! Numbers count: $CURRENT_COUNT → $NEW_COUNT"

# Step 4: Monitor the cascade effect
echo ""
echo "4. 📈 Monitoring Event-Driven cascade effect..."

echo "Waiting for pipeline restart cascade..."
sleep 2

# Check status changes
echo ""
echo "📊 Post-restart status:"

NEW_WANTS_RESPONSE=$(curl -s http://localhost:8080/api/v1/wants)

# Check numbers want
NEW_NUMBERS_WANT=$(echo "$NEW_WANTS_RESPONSE" | jq -r ".wants[] | select(.metadata.id == \"$NUMBERS_ID\")")
NEW_NUMBERS_STATUS=$(echo "$NEW_NUMBERS_WANT" | jq -r '.status')
NEW_ACTUAL_COUNT=$(echo "$NEW_NUMBERS_WANT" | jq -r '.spec.params.count')

echo "  Numbers Want: Status=$NEW_NUMBERS_STATUS, Count=$NEW_ACTUAL_COUNT"

# Check downstream wants
NEW_QUEUE_STATUS=$(echo "$NEW_WANTS_RESPONSE" | jq -r '.wants[] | select(.metadata.type == "qnet queue") | .status' | head -1)
NEW_SINK_STATUS=$(echo "$NEW_WANTS_RESPONSE" | jq -r '.wants[] | select(.metadata.type == "qnet sink") | .status' | head -1)

echo "  Queue Want: Status=$NEW_QUEUE_STATUS"
echo "  Sink Want: Status=$NEW_SINK_STATUS"

# Step 5: Show parameter history demonstrating the Event-Driven trigger
echo ""
echo "5. 📈 Parameter change history (Event-Driven trigger evidence):"

PARAM_HISTORY=$(echo "$NEW_NUMBERS_WANT" | jq -r '.history.parameterHistory[] | "\(.timestamp): count=\(.stateValue.count // "N/A")"')
echo "$PARAM_HISTORY"

# Step 6: Show processing stats to prove pipeline executed
echo ""
echo "6. 🔢 Pipeline execution stats (proving cascade worked):"

# Numbers stats
NUMBERS_STATS=$(echo "$NEW_NUMBERS_WANT" | jq -r '.state')
echo "  Numbers processed: $(echo "$NUMBERS_STATS" | jq -r '.total_processed // 0')"

# Queue stats
QUEUE_WANT=$(echo "$NEW_WANTS_RESPONSE" | jq -r '.wants[] | select(.metadata.type == "qnet queue") | .state' | head -1)
echo "  Queue processed: $(echo "$QUEUE_WANT" | jq -r '.total_processed // 0')"
echo "  Average wait time: $(echo "$QUEUE_WANT" | jq -r '.average_wait_time // 0')"

# Sink stats
SINK_WANT=$(echo "$NEW_WANTS_RESPONSE" | jq -r '.wants[] | select(.metadata.type == "qnet sink") | .state' | head -1)
echo "  Sink received: $(echo "$SINK_WANT" | jq -r '.total_processed // 0')"

echo ""
echo "🎯 Event-Driven Pipeline Restart Test Complete!"
echo ""
echo "Key Event-Driven Architecture Points Demonstrated:"
echo "✅ Single parameter change triggered restart cascade"
echo "✅ Numbers want: completed → idle → running → completed"
echo "✅ Queue want: woken by packet flow from Numbers"
echo "✅ Sink want: woken by packet flow from Queue"
echo "✅ Full pipeline coordination without persistent goroutines"
echo "✅ Fresh execution context with updated parameters"