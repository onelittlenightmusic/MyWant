#!/bin/bash

# Gmail MCP Troubleshooting Script
# Handles "MCP agent returned no result" errors

set -e

echo "=================================================="
echo "Gmail MCP Troubleshooting Tool"
echo "=================================================="
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Step 1: Check for running Goose processes
echo "Step 1: Checking for running Goose processes..."
GOOSE_PIDS=$(pgrep -f "goose run" || true)
if [ -n "$GOOSE_PIDS" ]; then
    echo -e "${YELLOW}Found running Goose processes:${NC}"
    ps aux | grep "goose run" | grep -v grep
    echo ""
    read -p "Kill these processes? (y/n): " -n 1 -r
    echo ""
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        pkill -9 -f "goose run"
        echo -e "${GREEN}✓ Goose processes killed${NC}"
    fi
else
    echo -e "${GREEN}✓ No stale Goose processes found${NC}"
fi
echo ""

# Step 2: Check for NPX MCP server processes
echo "Step 2: Checking for NPX MCP server processes..."
NPX_PIDS=$(pgrep -f "server-gmail-autoauth-mcp" || true)
if [ -n "$NPX_PIDS" ]; then
    echo -e "${YELLOW}Found running MCP server processes:${NC}"
    ps aux | grep "server-gmail-autoauth-mcp" | grep -v grep
    echo ""
    read -p "Kill these processes? (y/n): " -n 1 -r
    echo ""
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        pkill -9 -f "server-gmail-autoauth-mcp"
        echo -e "${GREEN}✓ MCP server processes killed${NC}"
    fi
else
    echo -e "${GREEN}✓ No stale MCP server processes found${NC}"
fi
echo ""

# Step 3: Verify Goose config
echo "Step 3: Verifying Goose configuration..."
CONFIG_FILE="$HOME/.config/goose/config.yaml"
if [ ! -f "$CONFIG_FILE" ]; then
    echo -e "${RED}✗ Config file not found: $CONFIG_FILE${NC}"
    exit 1
fi

# Check for Gmail extension
if grep -q "gmail:" "$CONFIG_FILE"; then
    echo -e "${GREEN}✓ Gmail extension found in config${NC}"

    # Show Gmail config
    echo ""
    echo "Gmail MCP Configuration:"
    echo "------------------------"
    awk '/gmail:/,/^  [a-z]/' "$CONFIG_FILE" | head -n -1
    echo ""
else
    echo -e "${RED}✗ Gmail extension not found in config${NC}"
    echo "Please run: goose configure"
    exit 1
fi

# Step 4: Check Gmail MCP package availability
echo "Step 4: Checking Gmail MCP package..."
if npx --yes @gongrzhe/server-gmail-autoauth-mcp --version 2>/dev/null; then
    echo -e "${GREEN}✓ Gmail MCP package accessible${NC}"
else
    echo -e "${YELLOW}⚠ Gmail MCP package may need updating${NC}"
    echo "Attempting to clear NPX cache..."
    rm -rf ~/.npm/_npx
    echo -e "${GREEN}✓ NPX cache cleared${NC}"
fi
echo ""

# Step 5: Test Goose basic functionality
echo "Step 5: Testing Goose basic functionality..."
echo "Running test prompt: 'What is 2+2?'"
GOOSE_TEST=$(timeout 15 bash -c "echo 'What is 2+2? Answer with just the number.' | goose run -i -" 2>&1 || true)
if echo "$GOOSE_TEST" | grep -q "4"; then
    echo -e "${GREEN}✓ Goose is responding correctly${NC}"
else
    echo -e "${YELLOW}⚠ Goose response unclear:${NC}"
    echo "$GOOSE_TEST" | tail -5
fi
echo ""

# Step 6: Test Gmail MCP integration
echo "Step 6: Testing Gmail MCP integration..."
read -p "Run Gmail MCP test? This will attempt to search emails (y/n): " -n 1 -r
echo ""
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "Testing Gmail search via Goose..."
    GMAIL_TEST=$(timeout 30 bash -c "echo 'Use the Gmail MCP server to search for emails with query: is:unread. Return just the count as a number.' | goose run -i -" 2>&1 || true)

    if echo "$GMAIL_TEST" | grep -qi "error\|failed\|cannot"; then
        echo -e "${RED}✗ Gmail MCP test failed${NC}"
        echo "Error output:"
        echo "$GMAIL_TEST" | grep -i "error" | tail -5
        echo ""
        echo -e "${YELLOW}Possible causes:${NC}"
        echo "  1. Gmail authentication expired - run: goose configure"
        echo "  2. Network connectivity issue"
        echo "  3. MCP server package corrupted - clear NPX cache"
    else
        echo -e "${GREEN}✓ Gmail MCP appears to be working${NC}"
        echo "Sample output:"
        echo "$GMAIL_TEST" | tail -10
    fi
fi
echo ""

# Step 7: Check MyWant server logs
echo "Step 7: Checking MyWant server logs..."
if [ -f "/tmp/server.log" ]; then
    echo "Recent MCP-related errors:"
    echo "-------------------------"
    grep -i "error\|failed\|warning" /tmp/server.log | grep -i "mcp\|goose\|gmail" | tail -10 || echo "No errors found"
else
    echo -e "${YELLOW}⚠ Server log not found at /tmp/server.log${NC}"
fi
echo ""

# Step 8: Recommendations
echo "=================================================="
echo "Troubleshooting Summary"
echo "=================================================="
echo ""
echo -e "${GREEN}Quick Fixes:${NC}"
echo "  1. Restart MyWant server: make restart-all"
echo "  2. Clear Goose sessions: rm -rf ~/.config/goose/sessions/*"
echo "  3. Re-authenticate Gmail: goose configure (select Gmail extension)"
echo ""
echo -e "${YELLOW}Common Issues:${NC}"
echo "  • 'No result' error → Gmail auth expired or MCP crashed"
echo "  • Timeout errors → Increase timeout in config.yaml"
echo "  • JSON parse errors → Goose output format changed"
echo ""
echo "For persistent issues, check:"
echo "  • Server logs: tail -f /tmp/server.log"
echo "  • Goose version: goose --version"
echo "  • MCP package: npx @gongrzhe/server-gmail-autoauth-mcp --help"
echo ""
