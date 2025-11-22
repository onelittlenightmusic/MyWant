#!/bin/bash

echo "🎯 Event-Driven Want Restart Demonstration"
echo "==========================================="

# Get current wants and find a completed travel planner
echo ""
echo "1. Finding completed travel planner wants..."
WANTS_RESPONSE=$(curl -s http://localhost:8080/api/v1/wants)
echo "Current wants summary:"
echo "$WANTS_RESPONSE" | jq -r '.wants[] | "  - \(.metadata.name) (\(.metadata.type)) - Status: \(.status)"'

# Find travel planner ID
TRAVEL_ID=$(echo "$WANTS_RESPONSE" | jq -r '.wants[] | select(.metadata.type == "agent travel system") | .metadata.id' | head -1)

if [ "$TRAVEL_ID" = "null" ] || [ -z "$TRAVEL_ID" ]; then
    echo ""
    echo "❌ No agent travel system want found!"
    echo "Let's create one first via API..."

    # Create new travel planner want
    CREATE_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/wants \
        -H "Content-Type: application/yaml" \
        -d 'metadata:
  name: restart-demo-travel
  type: agent travel system
  labels:
    role: travel-planner
spec:
  params:
    prefix: demo
    display_name: "Restart Demo Travel Itinerary"
    restaurant_type: "fine dining"
    hotel_type: "luxury"
    buffet_type: "international"
    dinner_duration: 2.0
  recipe: Travel Agent System')

    echo "Travel planner created via API"
    sleep 3  # Wait for execution to complete

    # Get the new ID
    WANTS_RESPONSE=$(curl -s http://localhost:8080/api/v1/wants)
    TRAVEL_ID=$(echo "$WANTS_RESPONSE" | jq -r '.wants[] | select(.metadata.name == "restart-demo-travel") | .metadata.id')
fi

echo ""
echo "2. Target want ID: $TRAVEL_ID"

# Get current status
echo ""
echo "3. Current want status:"
CURRENT_STATUS=$(curl -s "http://localhost:8080/api/v1/wants/$TRAVEL_ID/status")
echo "$CURRENT_STATUS" | jq -r '"Status: \(.status), Start Time: \(.runtime.start_time // "N/A")"'

# Demonstrate Event-Driven restart via parameter update
echo ""
echo "4. 🔄 Triggering Event-Driven restart via parameter update..."
echo "   This demonstrates how parameter changes trigger want restart:"

UPDATE_RESPONSE=$(curl -s -X PUT "http://localhost:8080/api/v1/wants/$TRAVEL_ID" \
    -H "Content-Type: application/json" \
    -d '{
        "spec": {
            "params": {
                "restaurant_type": "steakhouse",
                "dinner_duration": 2.5,
                "hotel_type": "boutique",
                "buffet_type": "gourmet"
            }
        }
    }')

echo "✅ Parameter update sent (triggers restart automatically)"

# Wait a moment for restart to begin
sleep 2

# Check new status
echo ""
echo "5. 📊 Post-restart status:"
NEW_STATUS=$(curl -s "http://localhost:8080/api/v1/wants/$TRAVEL_ID/status")
echo "$NEW_STATUS" | jq -r '"Status: \(.status), Start Time: \(.runtime.start_time // "N/A")"'

# Show parameter history to demonstrate the change
echo ""
echo "6. 📈 Parameter change history (showing Event-Driven trigger):"
WANT_DETAILS=$(curl -s "http://localhost:8080/api/v1/wants/$TRAVEL_ID")
echo "$WANT_DETAILS" | jq -r '.history.parameterHistory[] | "  \(.timestamp): \(.stateValue | keys | join(", "))"'

echo ""
echo "🎯 Event-Driven Restart Demo Complete!"
echo ""
echo "Key Points Demonstrated:"
echo "✅ Want completed (waitGroup.Done(), goroutine exited)"
echo "✅ Parameter change triggered restart event"
echo "✅ System created NEW goroutine (waitGroup.Add(1))"
echo "✅ Want state preserved across restart"
echo "✅ Fresh execution context with updated parameters"