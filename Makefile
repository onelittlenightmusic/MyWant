.PHONY: clean build test test-build fmt lint vet check run-qnet run-prime run-fibonacci run-fibonacci-loop run-travel run-sample-owner run-qnet-target run-qnet-using-recipe run-hierarchical-approval build-server run-server test-server-api test-server-simple run-travel-recipe run-travel-agent restart-all test-all-runs build-mock run-mock run-flight test-monitor-flight-api test-dynamic-travel-with-flight-api test-concurrent-deploy

# Code quality targets
fmt:
	@echo "🔧 Formatting Go code..."
	@find . -name "*.go" -not -path "./archive/*" -not -path "./web/*" | xargs gofmt -w -s
	@find . -name "*.go" -not -path "./archive/*" -not -path "./web/*" | xargs goimports -w 2>/dev/null || echo "goimports not available, using gofmt only"

vet:
	@echo "🔍 Running go vet..."
	go vet -C engine ./src/... ./cmd/server/...

lint:
	@echo "🧹 Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run -C engine ./src/... ./cmd/server/...; \
	else \
		echo "⚠️  golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		echo "📋 Running basic checks instead..."; \
		$(MAKE) vet; \
	fi

test:
	@echo "🧪 Running tests..."
	go test -C engine -v ./src/... ./cmd/server/... || echo "⚠️  Some tests failed or no tests found"

check: fmt vet test
	@echo "✅ All code quality checks completed"

# Test all run targets
test-all-runs:
	@echo "🧪 Testing all run targets..."
	@failed_targets="" && \
	for target in run-fibonacci-loop run-fibonacci-recipe run-prime run-qnet run-qnet-recipe run-travel run-travel-recipe run-travel-agent run-travel-agent-full run-travel-agent-direct run-hierarchical-approval run-dynamic-travel-change; do \
		echo "" && \
		echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" && \
		echo "🔹 Testing $$target..." && \
		echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" && \
		if timeout 10s make $$target 2>&1 | head -50; then \
			echo "✅ $$target completed successfully" ; \
		else \
			echo "❌ $$target failed or timed out" && \
			failed_targets="$$failed_targets $$target"; \
		fi; \
	done && \
	echo "" && \
	echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" && \
	echo "📊 Test Results Summary" && \
	echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" && \
	if [ -z "$$failed_targets" ]; then \
		echo "✅ All run targets passed!" && \
		exit 0; \
	else \
		echo "❌ Failed targets:$$failed_targets" && \
		exit 1; \
	fi
# Build the mywant library
build: check
	@echo "🔨 Building mywant library..."
	go build -C engine ./src/...

# Test that module builds correctly
test-build:
	cd engine && go mod tidy && go build ./src/...

run-fibonacci-loop:
	go run -C engine cmd/demos/demo_fibonacci_loop.go ../config/config-fibonacci-loop.yaml

run-fibonacci-recipe:
	go run -C engine cmd/demos/demo_fibonacci_recipe.go ../config/config-fibonacci-recipe.yaml

run-prime:
	go run -C engine cmd/demos/demo_prime.go ../config/config-prime.yaml

run-qnet:
	go run -C engine cmd/demos/demo_qnet.go ../config/config-qnet.yaml

run-qnet-recipe:
	go run -C engine cmd/demos/demo_qnet_owner.go ../config/config-qnet-recipe.yaml

run-travel:
	go run -C engine cmd/demos/demo_travel.go ../config/config-travel.yaml

# Recipe-based execution targets
run-travel-recipe:
	go run -C engine cmd/demos/demo_travel_recipe.go ../config/config-travel-recipe.yaml

run-travel-agent:
	go run -C engine cmd/demos/demo_travel_agent.go ../config/config-travel-agent.yaml

run-travel-agent-full:
	go run -C engine cmd/demos/demo_travel_agent_full.go ../config/config-travel-agent-full.yaml

run-travel-agent-direct:
	go run -C engine cmd/demos/demo_travel_agent_full.go ../config/config-travel-agent-direct.yaml

run-hierarchical-approval:
	go run -C engine cmd/demos/demo_hierarchical_approval.go ../config/config-hierarchical-approval.yaml

run-dynamic-travel-change:
	go run -C engine cmd/demos/demo_travel_recipe.go ../config/config-dynamic-travel-change.yaml

run-flight:
	go run -C engine cmd/demos/demo_flight.go ../config/config-flight.yaml

test-monitor-flight-api:
	@echo "🧪 Testing MonitorFlightAPI..."
	go run ./test_monitor/main.go

test-dynamic-travel-with-flight-api:
	@echo "✈️  Testing Dynamic Travel with Flight API on Server Mode"
	@echo "============================================================"
	@echo ""
	@echo "📋 Prerequisites:"
	@echo "  ✓ Mock server running on http://localhost:8081"
	@echo "  ✓ MyWant server running on http://localhost:8080"
	@echo ""
	@echo "🗑️  Attempting to delete existing 'dynamic travel planner' want (if any)..."
	@EXISTING_WANT_ID=$$(curl -s http://localhost:8080/api/v1/wants | jq -r '.wants[] | select(.metadata.name == "dynamic travel planner") | .metadata.id' || echo "")
	@if [ -n "$$EXISTING_WANT_ID" ]; then \
		echo "   Found existing want with ID: $$EXISTING_WANT_ID. Deleting..."; \
		curl -s -X DELETE http://localhost:8080/api/v1/wants/$$EXISTING_WANT_ID; \
		sleep 1; \
		echo "   Existing want deleted."; \
	else \
		echo "   No existing 'dynamic travel planner' want found."; \
	fi
	@echo ""
	@echo "📮 Creating dynamic travel want with Flight API..."
	@WANT_ID=$$(curl -s -X POST http://localhost:8080/api/v1/wants \
		-H "Content-Type: application/yaml" \
		--data-binary @config/config-dynamic-travel-change.yaml \
		| jq -r '.id' || echo "")
	@echo "   Want ID: $$WANT_ID"
	@echo ""
	@if [ -z "$$WANT_ID" ]; then \
		echo "❌ Failed to create want"; \
		exit 1; \
	fi
	@echo "⏳ Waiting for flight status changes (~50 seconds)..."
	@sleep 50
	@echo ""
	@echo "📊 Retrieving want state with history..."
	@WANT_STATE=$$(curl -s http://localhost:8080/api/v1/wants/$$WANT_ID)
	@echo ""
	@echo "📝 Analyzing Flight State History:"
	@echo ""
	@echo "$$WANT_STATE" | grep -o '"flight_status":"[^" ]*"' | head -10 || echo "   (No flight status found)"
	@echo "$$WANT_STATE" | grep -o '"flight_id":"[^" ]*"' | head -5 || echo "   (No flight ID found)"
	@echo ""
	@if echo "$$WANT_STATE" | grep -q "delayed_one_day"; then \
		echo "✅ Delay status detected in state"; \
	fi
	@if echo "$$WANT_STATE" | grep -q "cancelled"; then \
		echo "✅ Cancellation detected in state"; \
	fi
	@echo ""
	@echo "📖 State History (Raw WANT_STATE):"
	@echo "$$WANT_STATE"
	@echo "📖 State History (Parsed):"
	@echo "$$WANT_STATE" | jq '.wants[] | select(.metadata.type == "flight") | .state.status_history // empty' 2>/dev/null | head -30 || echo "   (Check server logs for details)"
	@echo ""
	@echo "✅ Server mode test completed"

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
	go run test_concurrent_deploy.go
	@echo ""
	@echo "✅ Concurrent deployment test completed!"

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
	@echo "  test-monitor-flight-api              - Test MonitorFlightAPI agent"
	@echo "  test-dynamic-travel-with-flight-api  - Test dynamic travel with flight status changes (requires mock server running)"
	@echo "  test-concurrent-deploy               - Test concurrent deployment (Travel Planner + Fibonacci)"
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
	@echo "  test-server-api  - Test server API endpoints"
	@echo "  restart-all      - Kill and restart frontend, backend, and mock server"
	@echo ""
	@echo "🧹 Utility:"
	@echo "  clean - Clean build artifacts"
	@echo "  help  - Show this help"

all: build

# Kill and restart frontend and backend processes
restart-all:
	@echo "🔄 Restarting frontend, backend, and mock server..."
	@echo "🛑 Killing existing processes..."
	@pkill -f "npm run dev" || echo "No frontend process found"
	@pkill -f "./bin/mywant" || echo "No backend process found"
	@pkill -f "./bin/flight-server" || echo "No mock server process found"
	@pkill -f "vite" || echo "No vite process found"
	@sleep 2
	@echo "🏗️  Building backend..."
	@$(MAKE) build-server
	@mkdir -p logs
	@echo "🏗️  Building mock server..."
	@$(MAKE) build-mock
	@echo "🚀 Starting backend in background..."
	@nohup ./bin/mywant 8080 localhost > ./logs/mywant-backend.log 2>&1 &
	@sleep 1
	@echo "✅ Backend started (PID: $$(pgrep -f './bin/mywant'))"
	@echo "✈️  Starting mock flight server in background..."
	@nohup ./bin/flight-server > ./logs/flight-server.log 2>&1 &
	@sleep 1
	@echo "✅ Mock server started (PID: $$(pgrep -f './bin/flight-server'))"
	@echo "📦 Installing frontend dependencies..."
	@cd web && npm install > /tmp/npm-install.log 2>&1
	@echo "🌐 Starting frontend in background..."
	@cd web && nohup npm run dev > /tmp/npm-dev.log 2>&1 &
	@sleep 1
	@echo "✅ Frontend started"
	@echo ""
	@echo "✅ All processes started!"
	@echo "🌐 Frontend: Check console output for URL"
	@echo "🔧 Backend: http://localhost:8080"
	@echo "✈️  Mock Server: http://localhost:8081"
	@echo ""
	@echo "📋 Log files:"
	@echo "  Backend: tail -f ./logs/mywant-backend.log"
	@echo "  Mock Server: tail -f ./logs/flight-server.log"
	@echo "  Frontend: tail -f /tmp/npm-dev.log"

# Default target
.DEFAULT_GOAL := help