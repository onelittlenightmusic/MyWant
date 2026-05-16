.PHONY: clean build build-gui build-cli build-mywant-gui build-playwright-app release test test-build fmt lint vet check run-qnet run-prime run-fibonacci run-fibonacci-loop run-travel run-sample-owner run-qnet-target run-qnet-using-recipe run-hierarchical-approval run-travel-recipe run-travel-agent restart-all test-all-runs build-mock build-mock-plugin run-mock run-flight test-all troubleshoot-mcp fix-mcp

# Code quality targets
fmt:
	@echo "🔧 Formatting Go code..."
	@find . -name "*.go" -not -path "./archive/*" -not -path "./web/*" | xargs gofmt -w -s
	@find . -name "*.go" -not -path "./archive/*" -not -path "./web/*" | xargs goimports -w 2>/dev/null || echo "goimports not available, using gofmt only"

vet:
	@echo "🔍 Running go vet..."
	go vet -C engine ./core/...
	go vet -C client ./...

lint:
	@echo "🧹 Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run -C engine ./core/...; \
		golangci-lint run -C client ./...; \
	else \
		echo "⚠️  golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		echo "📋 Running basic checks instead..."; \
		$(MAKE) vet; \
	fi

test:
	@echo "🧪 Running tests..."
	@-(go test -C engine -v ./core/... ./server/... ./core/chain/... ./core/pubsub/... && \
	   go test -C client -v ./...) || (echo "⚠️  Some tests failed"; $(MAKE) clean; exit 1)
	@$(MAKE) clean

check: fmt vet test
	@echo "✅ All code quality checks completed"

# Build targets
build-gui:
	@echo "📦 Building frontend assets..."
	cd web && npm install && npm run build

build-cli:
	@echo "🔨 Building mywant backend..."
	@mkdir -p bin
	go build -C client -o ../bin/mywant ./cmd/mywant

build-mywant-gui:
	@echo "📦 Building mywant-gui frontend assets..."
	cd ../mywant-gui/web && npm install && npm run build
	@echo "🔨 Building mywant-gui..."
	@mkdir -p bin
	go build -C ../mywant-gui -o ../mywant/bin/mywant-gui ./cmd/mywant-gui


build-playwright-app:
	@echo "🎭 Building Playwright MCP App Server..."
	@cd mcp/playwright-app && npm install && npm run build
	@echo "✅ playwright-app built: mcp/playwright-app/dist/server.js"

install-playwright-browsers:
	@echo "🌐 Installing Playwright browsers (Chromium)..."
	@cd mcp/playwright-app && npx playwright install chromium
	@echo "✅ Playwright browsers installed"

release: build-gui build-cli build-playwright-app
	@echo "🚀 Release build complete: bin/mywant + mcp/playwright-app/dist/server.js"

# Build the mywant library
build: check
	@echo "🔨 Building mywant library..."
	go build -C engine ./core/...
# Test that module builds correctly
test-build:
	cd engine && go mod tidy && go build ./core/...

run-fibonacci-loop:
	go run -C engine ./demos/demo_fibonacci_loop ../examples/configs/config-fibonacci-loop.yaml

run-fibonacci-recipe:
	go run -C engine ./demos/demo_fibonacci_recipe ../examples/configs/config-fibonacci-recipe.yaml

run-prime:
	go run -C engine ./demos/demo_prime ../examples/configs/config-prime.yaml

run-qnet:
	go run -C engine ./demos/demo_qnet ../examples/configs/config-qnet.yaml

run-qnet-recipe:
	go run -C engine ./demos/demo_qnet_owner ../examples/configs/config-qnet-recipe.yaml

run-travel:
	go run -C engine ./demos/demo_travel ../examples/configs/config-travel.yaml

# Recipe-based execution targets
run-travel-recipe:
	go run -C engine ./demos/demo_travel_recipe ../examples/configs/config-travel-recipe.yaml

run-travel-agent:
	go run -C engine ./demos/demo_travel_agent ../examples/configs/config-travel-agent.yaml

run-travel-agent-full:
	go run -C engine ./demos/demo_travel_agent_full ../examples/configs/config-travel-agent-full.yaml

run-travel-agent-direct:
	go run -C engine ./demos/demo_travel_agent_full ../examples/configs/config-travel-agent-direct.yaml

run-hierarchical-approval:
	go run -C engine ./demos/demo_hierarchical_approval ../examples/configs/config-hierarchical-approval.yaml

run-dynamic-travel-change:
	timeout 140 go run -C engine ./demos/demo_travel_recipe ../examples/configs/config-dynamic-travel-change.yaml 120

run-flight:
	go run -C engine ./demos/demo_flight ../examples/configs/config-flight.yaml

# Tests removed - no longer functional or environment-dependent

# Test All Server-Based Tests
test-all: restart-all
	@echo ""
	@echo "🧪 Running All Server-Based Tests..."
	@echo "======================================================="
	@echo ""
	@echo "⏳ Waiting for server startup..."
	@sleep 7
	@echo ""
	@echo "📊 Test Suite:"
	@echo ""

	@echo ""
	@echo "======================================================="
	@echo "✅ All server-based tests completed!"
	@echo ""
	@echo "📊 Test Results:"
	@echo "  No dedicated server-based tests currently enabled."
	@echo ""
	@echo "======================================================="

# Build the mock flight server
build-mock:
	@echo "🏗️  Building mock flight server..."
	@mkdir -p bin
	@cd tools/mock && go build -o ../../bin/flight-server

# Build the mock CLI plugin (mywant-mock)
build-mock-plugin:
	@echo "🏗️  Building mock plugin (mywant-mock)..."
	@mkdir -p bin
	@cd tools/mock-plugin && go build -o ../../bin/mywant-mock

# Run the mock flight server
run-mock: build-mock
	@./bin/flight-server

clean:
	@echo "🧹 Cleaning build artifacts..."
	@rm -rf bin/
	@rm -f qnet
	@rm -f tools/mock/flight-server
	@rm -f mywant
	@rm -f engine/main
	@rm -f engine/engine/server
	@rm -f engine/demo_*
	@rm -f engine/server-packet-test.log
	@rm -rf mcp/playwright-app/dist/
	@go clean

help:
	@echo "📋 Available targets:"
	@echo ""
	@echo "🔧 Code Quality:"
	@echo "  fmt            - Format Go code"
	@echo "  vet            - Run go vet"
	@echo "  lint           - Run linter (requires golangci-lint)"
	@echo "  test           - Run tests"
	@echo "  check          - Run all code quality checks (fmt + vet + test)"
	@echo "  test-all-runs  - Test all run targets (with 10s timeout each)"
	@echo ""
	@echo "🔨 Build:"
	@echo "  build                     - Build mywant library (with quality checks)"
	@echo "  test-build                - Quick build test"
	@echo "  build-mock                - Build mock flight server (bin/flight-server)"
	@echo "  build-mock-plugin         - Build mock CLI plugin (bin/mywant-mock)"
	@echo "  build-playwright-app      - Build Playwright MCP App Server (Node.js)"
	@echo "  install-playwright-browsers - Install Chromium for Playwright (first-time setup)"
	@echo ""
	@echo "🏃 Run Examples:"
	@echo "  run-qnet              - Queue network example"
	@echo "  run-prime             - Prime number example"
	@echo "  run-fibonacci         - Fibonacci sequence example"
	@echo "  run-fibonacci-loop    - Fibonacci loop example"
	@echo "  run-travel            - Travel planning example"
	@echo "  run-flight            - Flight booking with automatic rebooking"
	@echo "  run-sample-owner      - QNet with dynamic recipe loading"
	@echo "  run-qnet-target       - QNet with target want"
	@echo "  run-dynamic-travel-change - Run the dynamic travel change demo"
	@echo "  run-hierarchical-approval - Hierarchical approval workflow"
	@echo ""
	@echo "🧪 Testing:"
	@echo "  test-all                  - Run all server-based tests (builds and starts servers)"
	@echo ""
	@echo "📜 Recipe-based Examples:"
	@echo "  run-travel-recipe     - Travel with recipe system"
	@echo "  run-travel-agent      - Travel with agent system integration"
	@echo "  run-travel-agent-full - Complete travel system with all agents"
	@echo "  run-travel-agent-direct - Direct config with all agents (no recipes)"
	@echo "  run-qnet-using-recipe - QNet with using field connections"
	@echo ""
	@echo "🔧 Server:"
	@echo "  run-mock         - Start mock flight server directly (for development)"
	@echo "  restart-all      - Rebuild and restart the MyWant server"
	@echo ""
	@echo "  Mock flight management (via plugin):"
	@echo "    mywant mock flight start   - Start mock flight server"
	@echo "    mywant mock flight stop    - Stop mock flight server"
	@echo "    mywant mock flight status  - Show status"
	@echo "    mywant mock list           - List all mock servers"
	@echo ""
	@echo "🧹 Utility:"
	@echo "  clean - Clean build artifacts"
	@echo "  help  - Show this help"

all: build

# Kill and restart the MyWant server (mock flight server is managed separately via mywant mock)
restart-all:
	@echo "🔄 Restarting MyWant server..."
	@echo "🛑 Stopping existing processes..."
	@./bin/mywant stop 2>/dev/null || echo "  Server not running"
	@./bin/mywant-gui stop 2>/dev/null || true
	@echo "🧹 Cleaning logs..."
	@rm -f ~/.mywant/server.log ~/.mywant/gui.log
	@echo ""
	@echo "🧹 Cleaning Go build cache..."
	@go clean -cache
	@$(MAKE) build-gui
	@$(MAKE) build-cli
	@$(MAKE) build-mywant-gui
	@$(MAKE) build-mock
	@$(MAKE) build-mock-plugin
	@mkdir -p ~/.mywant
	@echo "🚀 Starting MyWant backend..."
	@nohup ./bin/mywant start -D --port 8080 --host 0.0.0.0 > /dev/null 2>&1 &
	@echo "🌐 Starting mywant-gui frontend..."
	@nohup ./bin/mywant-gui start -D --port 8081 --host 0.0.0.0 > /dev/null 2>&1 &
	@sleep 2
	@echo "✅ Server started"
	@echo "🌐 Backend: http://0.0.0.0:8080"
	@echo "🖥️  Frontend: http://0.0.0.0:8081"
	@echo ""
	@echo "📋 Server management:"
	@echo "  Stop:        ./bin/mywant stop && ./bin/mywant-gui stop"
	@echo "  View status: ./bin/mywant ps"
	@echo "  Mock flight: ./bin/mywant-mock flight start"

# Gmail MCP troubleshooting targets
troubleshoot-mcp:
	@echo "🔍 Running Gmail MCP troubleshooting..."
	@./tools/scripts/troubleshoot-gmail-mcp.sh

fix-mcp:
	@echo "🔧 Quick fix for Gmail MCP..."
	@./tools/scripts/fix-gmail-mcp.sh

# Default target
.DEFAULT_GOAL := help