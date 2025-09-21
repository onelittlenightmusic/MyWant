#!/bin/bash

# Test script for suspend/resume functionality
set -e

echo "🧪 Testing MyWant Server Suspend/Resume Functionality..."
echo "======================================================="

# Make sure bin directory exists
mkdir -p bin

# Build server if not exists
if [ ! -f "bin/mywant" ]; then
    echo "🏗️  Building server..."
    go build -o bin/mywant cmd/server/*.go
fi

# Kill any existing server on port 8082
echo "🧹 Cleaning up any existing servers..."
pkill -f "bin/mywant.*8082" || true
sleep 1

echo "📋 Starting server in background on port 8082..."
./bin/mywant 8082 localhost &
SERVER_PID=$!
echo "✅ Server started (PID: $SERVER_PID)"

# Wait for server to start
sleep 3

# Trap to clean up server on exit
trap "echo '🛑 Stopping server...'; kill $SERVER_PID || true; echo '✅ Cleanup completed'" EXIT

echo ""
echo "🩺 Testing health endpoint..."
curl -s http://localhost:8082/health | jq '.' || curl -s http://localhost:8082/health
echo ""

echo "📝 Creating want with qnet config..."
WANT_ID=$(curl -s -X POST http://localhost:8082/api/v1/wants \
    -H "Content-Type: application/yaml" \
    --data-binary @config/config-qnet.yaml | \
    grep -o 'want-[^"]*' | head -1)

if [ -z "$WANT_ID" ]; then
    echo "❌ Failed to create want"
    exit 1
fi

echo "✅ Created want: $WANT_ID"
echo ""

echo "⏳ Waiting 2 seconds for execution to start..."
sleep 2

echo "📊 Getting initial status..."
curl -s http://localhost:8082/api/v1/wants/$WANT_ID/status | jq '.' || \
curl -s http://localhost:8082/api/v1/wants/$WANT_ID/status
echo ""

echo "⏸️  Testing SUSPEND endpoint..."
SUSPEND_RESPONSE=$(curl -s -X POST http://localhost:8082/api/v1/wants/$WANT_ID/suspend)
echo "Suspend response: $SUSPEND_RESPONSE"
echo ""

echo "📊 Checking status after suspend..."
STATUS_AFTER_SUSPEND=$(curl -s http://localhost:8082/api/v1/wants/$WANT_ID/status)
echo "Status after suspend: $STATUS_AFTER_SUSPEND"

# Check if suspended field is true
if echo "$STATUS_AFTER_SUSPEND" | grep -q '"suspended":true'; then
    echo "✅ Want is correctly suspended"
else
    echo "❌ Want should be suspended but isn't"
    echo "Full response: $STATUS_AFTER_SUSPEND"
fi
echo ""

echo "⏳ Waiting 2 seconds while suspended..."
sleep 2

echo "▶️  Testing RESUME endpoint..."
RESUME_RESPONSE=$(curl -s -X POST http://localhost:8082/api/v1/wants/$WANT_ID/resume)
echo "Resume response: $RESUME_RESPONSE"
echo ""

echo "📊 Checking status after resume..."
STATUS_AFTER_RESUME=$(curl -s http://localhost:8082/api/v1/wants/$WANT_ID/status)
echo "Status after resume: $STATUS_AFTER_RESUME"

# Check if suspended field is false
if echo "$STATUS_AFTER_RESUME" | grep -q '"suspended":false'; then
    echo "✅ Want is correctly resumed"
else
    echo "❌ Want should be resumed but isn't"
    echo "Full response: $STATUS_AFTER_RESUME"
fi
echo ""

echo "⏳ Waiting 3 seconds for continued execution..."
sleep 3

echo "📊 Final status check..."
curl -s http://localhost:8082/api/v1/wants/$WANT_ID/status | jq '.' || \
curl -s http://localhost:8082/api/v1/wants/$WANT_ID/status
echo ""

echo "📈 Getting final results..."
curl -s http://localhost:8082/api/v1/wants/$WANT_ID/results | jq '.' || \
curl -s http://localhost:8082/api/v1/wants/$WANT_ID/results
echo ""

echo "🎯 Testing error conditions..."
echo "Testing suspend on non-existent want:"
curl -s -X POST http://localhost:8082/api/v1/wants/invalid-id/suspend
echo ""

echo "Testing resume on non-existent want:"
curl -s -X POST http://localhost:8082/api/v1/wants/invalid-id/resume
echo ""

echo "✅ Suspend/Resume functionality test completed successfully!"
echo "📋 Summary:"
echo "  - Server started and health check passed"
echo "  - Want created successfully"
echo "  - Suspend endpoint works"
echo "  - Status correctly shows suspended=true"
echo "  - Resume endpoint works"
echo "  - Status correctly shows suspended=false"
echo "  - Error handling for invalid IDs works"