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
echo "=== Deploying Dynamic Travel System ==="
PAYLOAD=$(cat <<'PAYLOAD_EOF'
{
  "wants": [
    {
      "metadata": {
        "name": "restaurant",
        "type": "restaurant",
        "labels": {"role": "travel_provider"}
      },
      "spec": {
        "params": {"restaurant_type": "fine dining"}
      }
    },
    {
      "metadata": {
        "name": "hotel",
        "type": "hotel",
        "labels": {"role": "travel_provider"}
      },
      "spec": {
        "params": {"hotel_type": "luxury"}
      }
    },
    {
      "metadata": {
        "name": "buffet",
        "type": "buffet",
        "labels": {"role": "travel_provider"}
      },
      "spec": {
        "params": {"buffet_type": "continental"}
      }
    },
    {
      "metadata": {
        "name": "flight",
        "type": "flight",
        "labels": {"role": "travel_provider"}
      },
      "spec": {
        "params": {}
      }
    },
    {
      "metadata": {
        "name": "travel_coordinator",
        "type": "travel_coordinator"
      },
      "spec": {
        "using": [{"role": "travel_provider"}]
      }
    }
  ]
}
PAYLOAD_EOF
)

RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/wants \
  -H "Content-Type: application/json" \
  -d "$PAYLOAD")

echo "Deployment response:"
echo "$RESPONSE" | jq '.'

COORDINATOR_ID=$(echo "$RESPONSE" | jq -r '.want_ids[-1]' 2>/dev/null)
echo ""
echo "Coordinator ID: $COORDINATOR_ID"

echo ""
echo "Waiting for execution (7 seconds)..."
sleep 7

echo ""
echo "=== Checking Coordinator State ==="
COORD_RESPONSE=$(curl -s http://localhost:8080/api/v1/wants/$COORDINATOR_ID)

FINAL_RESULT=$(echo "$COORD_RESPONSE" | jq -r '.state.finalResult // "NOT_FOUND"' 2>/dev/null)
STATUS=$(echo "$COORD_RESPONSE" | jq -r '.status // "NOT_FOUND"' 2>/dev/null)

echo ""
echo "=== Results ==="
echo "Coordinator Status: $STATUS"
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
  
  echo "=== Investigating Issue ==="
  echo ""
  echo "Checking all wants states:"
  curl -s http://localhost:8080/api/v1/wants | jq '.wants[] | {id: .metadata.id, name: .metadata.name, status: .status, state: .state}' 2>/dev/null
  
  exit 1
fi
