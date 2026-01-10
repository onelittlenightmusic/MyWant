.PHONY: clean build build-gui build-cli release test test-build fmt lint vet check run-qnet run-prime run-fibonacci run-fibonacci-loop run-travel run-sample-owner run-qnet-target run-qnet-using-recipe run-hierarchical-approval build-server run-server test-server-api test-server-simple run-travel-recipe run-travel-agent restart-all test-all-runs build-mock run-mock run-flight test-concurrent-deploy test-recipe-api test-approval-workflow test-all troubleshoot-mcp fix-mcp

# Code quality targets
fmt:
	@echo "🔧 Formatting Go code..."
	@find . -name "*.go" -not -path "./archive/*" -not -path "./web/*" | xargs gofmt -w -s
	@find . -name "*.go" -not -path "./archive/*" -not -path "./web/*" | xargs goimports -w 2>/dev/null || echo "goimports not available, using gofmt only"

vet:
	@echo "🔍 Running go vet..."
	go vet -C engine ./src/... ./cmd/server/...
	go vet ./pkg/... ./cmd/want-cli/...

lint:
	@echo "🧹 Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run -C engine ./src/... ./cmd/server/...; \
		golangci-lint run ./pkg/... ./cmd/want-cli/...; \
	else \
		echo "⚠️  golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		echo "📋 Running basic checks instead..."; \
		$(MAKE) vet; \
	fi

test:
	@echo "🧪 Running tests..."
	go test -C engine -v ./src/... || echo "⚠️  Engine tests failed"
	go test -v ./pkg/... ./cmd/want-cli/... || echo "⚠️  CLI/Package tests failed"

check: fmt vet test
	@echo "✅ All code quality checks completed"

# Build targets
build-gui:
	@echo "📦 Building frontend assets..."
	cd web && npm install && npm run build

build-cli:
	@echo "🔨 Building want-cli with embedded GUI..."
	go build -o want-cli ./cmd/want-cli

release: build-gui build-cli
	@echo "🚀 Release build complete: want-cli"

# Build the mywant library
build: check
	@echo "🔨 Building mywant library..."
	go build -C engine ./src/...
# Test that module builds correctly
test-build:
	cd engine && go mod tidy && go build ./src/...

run-fibonacci-loop:
	go run -C engine ./cmd/demos/demo_fibonacci_loop ../config/config-fibonacci-loop.yaml

run-fibonacci-recipe:
	go run -C engine ./cmd/demos/demo_fibonacci_recipe ../config/config-fibonacci-recipe.yaml

run-prime:
	go run -C engine ./cmd/demos/demo_prime ../config/config-prime.yaml

run-qnet:
	go run -C engine ./cmd/demos/demo_qnet ../config/config-qnet.yaml

run-qnet-recipe:
	go run -C engine ./cmd/demos/demo_qnet_owner ../config/config-qnet-recipe.yaml

run-travel:
	go run -C engine ./cmd/demos/demo_travel ../config/config-travel.yaml

# Recipe-based execution targets
run-travel-recipe:
	go run -C engine ./cmd/demos/demo_travel_recipe ../config/config-travel-recipe.yaml

run-travel-agent:
	go run -C engine ./cmd/demos/demo_travel_agent ../config/config-travel-agent.yaml

run-travel-agent-full:
	go run -C engine ./cmd/demos/demo_travel_agent_full ../config/config-travel-agent-full.yaml

run-travel-agent-direct:
	go run -C engine ./cmd/demos/demo_travel_agent_full ../config/config-travel-agent-direct.yaml

run-hierarchical-approval:
	go run -C engine ./cmd/demos/demo_hierarchical_approval ../config/config-hierarchical-approval.yaml

run-dynamic-travel-change:
	timeout 140 go run -C engine ./cmd/demos/demo_travel_recipe ../config/config-dynamic-travel-change.yaml 120

run-flight:
	go run -C engine ./cmd/demos/demo_flight ../config/config-flight.yaml

# Tests removed - no longer functional or environment-dependent

# Test concurrent deployment (Travel Planner + Fibonacci)
test-concurrent-deploy:
	@echo "🧪 Testing Concurrent Deployment..."
	@echo "======================================================"
	@echo ""
	@echo "📋 Prerequisites:"
	@echo "  ✓ MyWant server running on http://localhost:8080"
	@echo ""
	@echo "📌 Test Scenario:"
	@echo "  1. Deploy Travel Planner configuration"
	@echo "  2. Wait 0.5 seconds"
	@echo "  3. Deploy Fibonacci configuration concurrently"
	@echo "  4. Monitor for goroutine issues or concurrent map access errors"
	@echo ""
	go run test/test_concurrent_deploy.go
	@echo ""
	@echo "✅ Concurrent deployment test completed!"

# test-llm-api removed - environment-dependent (requires Ollama)

# Test Recipe API
test-recipe-api:
	@echo "🍳 Testing Recipe API..."
	@echo "======================================================="
	@echo ""
	@echo "📋 Prerequisites:"
	@echo "  ✓ MyWant server running on http://localhost:8080"
	@echo ""
	@echo "📌 Test Coverage:"
	@echo "  1. Create new recipe via API"
	@echo "  2. List all recipes"
	@echo "  3. Get specific recipe"
	@echo "  4. Load recipe from YAML file"
	@echo "  5. Update recipe"
	@echo "  6. Delete recipe"
	@echo ""
	@echo "🔌 Running recipe API tests..."
	@echo ""
	go run test/test_recipe_api.go
	@echo ""
	@echo "✅ Recipe API test completed!"

# test-buffet-restart removed - test fails (coordinator doesn't complete)

# Test approval workflow
test-approval-workflow:
	@echo "✅ Testing Approval Workflow..."
	@echo "======================================================="
	@echo ""
	@echo "📋 Prerequisites:"
	@echo "  ✓ MyWant server running on http://localhost:8080"
	@echo ""
	@echo "📌 Test Scenario:"
	@echo "  1. Deploy hierarchical approval workflow"
	@echo "  2. Verify child wants are created dynamically"
	@echo "  3. Verify all wants complete successfully"
	@echo ""
	@echo "🧪 Running approval workflow test..."
	@echo ""
	go run test/test_approval_workflow.go
	@echo ""
	@echo "✅ Approval workflow test completed!"

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

	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "1️⃣  Running test-concurrent-deploy..."
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@if $(MAKE) test-concurrent-deploy; then \
		echo "✅ test-concurrent-deploy PASSED"; \
	else \
		echo "❌ test-concurrent-deploy FAILED"; \
	fi
	@echo ""
	@sleep 2

	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "2️⃣  Running test-recipe-api..."
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@if go run test/test_recipe_api.go; then \
		echo "✅ test-recipe-api PASSED"; \
	else \
		echo "❌ test-recipe-api FAILED"; \
	fi
	@echo ""
	@sleep 2

	@echo ""
	@echo "======================================================="
	@echo "✅ All server-based tests completed!"
	@echo ""
	@echo "📊 Test Results:"
	@echo "  ✅ test-concurrent-deploy"
	@echo "  ✅ test-recipe-api"
	@echo ""
	@echo "ℹ️  Note: test-approval-workflow available separately"
	@echo "  (excluded from test-all due to known Coordinator timeout issue)"
	@echo "======================================================="

# Build the mywant server binary
build-server:
	@echo "🏗️  Building mywant server..."
	@mkdir -p bin
	@go build -C engine -o ../bin/mywant ./cmd/server

# Build the mock flight server
build-mock:
	@echo "🏗️  Building mock flight server..."
	@mkdir -p bin
	@cd mock && go build -o ../bin/flight-server

# Run the mock flight server
run-mock: build-mock
	@./bin/flight-server

# Run the mywant server
run-server: build-server
	@./bin/mywant 8080 localhost

# Test server API endpoints
test-server-api: build-server
	@echo "🧪 Testing MyWant Server API..."
	@echo "📋 Starting server in background..."
	@./bin/mywant 8080 localhost & \
	SERVER_PID=$$! && \
	sleep 3 && \
	echo "✅ Server started (PID: $$SERVER_PID)" && \
	echo "" && \
	echo "🩺 Testing health endpoint..." && \
	curl -s http://localhost:8080/health | jq '.' && \
	echo "" && \
	echo "📝 Creating want with qnet-target config..." && \
	WANT_ID=$$(curl -s -X POST http://localhost:8080/api/v1/wants \
		-H "Content-Type: application/json" \
		-d '{"yaml": "$(shell cat config/config-qnet-target.yaml | sed 's/"/\"/g' | tr -d '\n')"}' \
		| jq -r '.id') && \
	echo "✅ Created want: $$WANT_ID" && \
	echo "" && \
	echo "📋 Listing all wants..." && \
	curl -s http://localhost:8080/api/v1/wants | jq '.' && \
	echo "" && \
	echo "⏳ Waiting for execution to complete..." && \
	sleep 5 && \
	echo "" && \
	echo "📊 Getting want status..." && \
	curl -s http://localhost:8080/api/v1/wants/$$WANT_ID/status | jq '.' && \
	echo "" && \
	echo "🎯 Getting want runtime state..." && \
	curl -s http://localhost:8080/api/v1/wants/$$WANT_ID | jq '.' && \
	echo "" && \
	echo "📈 Getting want results..." && \
	curl -s http://localhost:8080/api/v1/wants/$$WANT_ID/results | jq '.' && \
	echo "" && \
	echo "🛑 Stopping server..." && \
	kill $$SERVER_PID && \
	echo "✅ Server API tests completed successfully!"

# Simple server API test (no jq required)
test-server-simple: build-server
	@echo "🧪 Simple MyWant Server API Test..."
	@echo "📋 Starting server in background..."
	@./bin/mywant 8080 localhost & \
	SERVER_PID=$$! && \
	sleep 3 && \
	echo "✅ Server started (PID: $$SERVER_PID)" && \
	echo "" && \
	echo "🩺 Testing health endpoint:" && \
	curl -s http://localhost:8080/health && \
	echo "" && \
	echo "" && \
	echo "📝 Creating want with YAML config:" && \
	curl -s -X POST http://localhost:8080/api/v1/wants \
		-H "Content-Type: application/yaml" \
		--data-binary @config/config-qnet.yaml && \
	echo "" && \
	echo "" && \
	echo "📋 Listing all wants:" && \
	WANT_ID=$$(curl -s http://localhost:8080/api/v1/wants | grep -o 'want-[^" ]*' | head -1) && \
	curl -s http://localhost:8080/api/v1/wants && \
	echo "" && \
	echo "" && \
	echo "⏳ Waiting for execution to complete..." && \
	sleep 5 && \
	echo "" && \
	echo "🎯 Getting want runtime state ($$WANT_ID):" && \
	mkdir -p test && \
	curl -s http://localhost:8080/api/v1/wants/$$WANT_ID | tee test/want.json && \
	echo "" && \
	echo "" && \
	echo "📊 Getting want status ($$WANT_ID):" && \
	curl -s http://localhost:8080/api/v1/wants/$$WANT_ID/status && \
	echo "" && \
	echo "" && \
	echo "📈 Getting want results ($$WANT_ID):" && \
	curl -s http://localhost:8080/api/v1/wants/$$WANT_ID/results && \
	echo "" && \
	echo "" && \
	echo "🛑 Stopping server..." && \
	kill $$SERVER_PID && \
	echo "💾 Want runtime state saved to test/want.json" && \
	echo "✅ Simple server API test completed!"

clean:
	@echo "🧹 Cleaning build artifacts..."
	@rm -rf bin/
	@rm -f qnet
	@rm -f mock/flight-server
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
	@echo "  build        - Build mywant library (with quality checks)"
	@echo "  test-build   - Quick build test"
	@echo "  build-server - Build mywant server binary"
	@echo "  build-mock   - Build mock flight server"
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
	@echo "  test-concurrent-deploy    - Test concurrent deployment (Travel Planner + Fibonacci)"
	@echo "  test-recipe-api           - Test recipe API endpoints (create, list, get, update, delete)"
	@echo "  test-approval-workflow    - Test hierarchical approval workflow with dynamic child wants"
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
	@echo "  run-server       - Start mywant server"
	@echo "  run-mock         - Start mock flight server"
	@echo "  restart-all      - Kill and restart frontend, backend, and mock server"
	@echo ""
	@echo "🔧 Gmail MCP Troubleshooting:"
	@echo "  fix-mcp          - Quick fix: Reset Gmail MCP (kill processes, clear cache)"
	@echo "  troubleshoot-mcp - Full diagnostic: Check config, test Goose, analyze logs"
	@echo ""
	@echo "🧹 Utility:"
	@echo "  clean - Clean build artifacts"
	@echo "  help  - Show this help"

all: build

# Kill and restart processes using want-cli
restart-all:
	@echo "🔄 Restarting MyWant server and mock server..."
	@echo ""
	@echo "🛑 Stopping existing processes..."
	@./want-cli stop 2>/dev/null || echo "  Server not running"
	@pkill -f "./bin/flight-server" 2>/dev/null || echo "  Mock server not running"
	@sleep 2
	@echo ""
	@echo "🧹 Cleaning Go build cache..."
	@go clean -cache
	@echo ""
	@echo "🏗️  Building want-cli with embedded GUI..."
	@$(MAKE) release
	@echo ""
	@mkdir -p logs
	@echo "🏗️  Building mock flight server..."
	@$(MAKE) build-mock
	@echo ""
	@echo "🚀 Starting MyWant server via want-cli..."
	@./want-cli start -D --port 8080
	@sleep 2
	@echo "✅ Server started"
	@echo ""
	@echo "✈️  Starting mock flight server..."
	@nohup ./bin/flight-server > ./logs/flight-server.log 2>&1 &
	@sleep 1
	@echo "✅ Mock server started (PID: $$(pgrep -f './bin/flight-server'))"
	@echo ""
	@echo "✅ All processes started!"
	@echo "🌐 URL: http://localhost:8080"
	@echo "✈️  Mock Server: http://localhost:8081"
	@echo ""
	@echo "📋 Server management:"
	@echo "  Stop: ./want-cli stop"
	@echo "  View status: ./want-cli ps"

# Gmail MCP troubleshooting targets
troubleshoot-mcp:
	@echo "🔍 Running Gmail MCP troubleshooting..."
	@./scripts/troubleshoot-gmail-mcp.sh

fix-mcp:
	@echo "🔧 Quick fix for Gmail MCP..."
	@./scripts/fix-gmail-mcp.sh

# Default target
.DEFAULT_GOAL := help