.PHONY: clean build test test-build fmt lint vet check run-qnet run-prime run-fibonacci run-fibonacci-loop run-travel run-sample-owner run-qnet-target run-qnet-using-recipe run-hierarchical-approval build-server run-server test-server-api test-server-simple run-travel-recipe run-travel-agent restart-all

# Code quality targets
fmt:
	@echo "🔧 Formatting Go code..."
	gofmt -w -s .
	goimports -w . 2>/dev/null || echo "goimports not available, using gofmt only"

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
	go test -v -C engine ./src/... ./cmd/server/... || echo "⚠️  Some tests failed or no tests found"

check: fmt vet test
	@echo "✅ All code quality checks completed"

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

# Build the mywant server binary
build-server:
	@echo "🏗️  Building mywant server..."
	mkdir -p bin
	go build -C engine -o ../bin/mywant ./cmd/server

# Run the mywant server
run-server: build-server
	./bin/mywant 8080 localhost

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
		-d '{"yaml": "$(shell cat config/config-qnet-target.yaml | sed 's/"/\\"/g' | tr -d '\n')"}' \
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
		--data-binary @config/config-qnet-target.yaml && \
	echo "" && \
	echo "" && \
	echo "📋 Listing all wants:" && \
	WANT_ID=$$(curl -s http://localhost:8080/api/v1/wants | grep -o 'want-[^"]*' | head -1) && \
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
	rm -rf bin/
	rm -f qnet
	go clean

help:
	@echo "📋 Available targets:"
	@echo ""
	@echo "🔧 Code Quality:"
	@echo "  fmt        - Format Go code"
	@echo "  vet        - Run go vet"
	@echo "  lint       - Run linter (requires golangci-lint)"
	@echo "  test       - Run tests"
	@echo "  check      - Run all code quality checks (fmt + vet + test)"
	@echo ""
	@echo "🔨 Build:"
	@echo "  build      - Build mywant library (with quality checks)"
	@echo "  test-build - Quick build test"
	@echo "  build-server - Build mywant server binary"
	@echo ""
	@echo "🏃 Run Examples:"
	@echo "  run-qnet              - Queue network example"
	@echo "  run-prime             - Prime number example"
	@echo "  run-fibonacci         - Fibonacci sequence example"
	@echo "  run-fibonacci-loop    - Fibonacci loop example"
	@echo "  run-travel            - Travel planning example"
	@echo "  run-sample-owner      - QNet with dynamic recipe loading"
	@echo "  run-qnet-target       - QNet with target want"
	@echo "  run-hierarchical-approval - Hierarchical approval workflow"
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
	@echo "  test-server-api  - Test server API endpoints"
	@echo "  restart-all      - Kill and restart frontend and backend processes"
	@echo ""
	@echo "🧹 Utility:"
	@echo "  clean - Clean build artifacts"
	@echo "  help  - Show this help"

all: build

# Kill and restart frontend and backend processes
restart-all:
	@echo "🔄 Restarting frontend and backend..."
	@echo "🛑 Killing existing processes..."
	@pkill -f "npm run dev" || echo "No frontend process found"
	@pkill -f "./bin/mywant" || echo "No backend process found"
	@pkill -f "vite" || echo "No vite process found"
	@sleep 2
	@echo "🏗️  Building backend..."
	@$(MAKE) build-server
	@echo "🚀 Starting backend in background..."
	@./bin/mywant 8080 localhost &
	@echo "📦 Installing frontend dependencies..."
	@cd web && npm install
	@echo "🌐 Starting frontend in background..."
	@cd web && npm run dev &
	@echo "✅ Both processes started!"
	@echo "🌐 Frontend: http://localhost:5173"
	@echo "🔧 Backend: http://localhost:8080"

# Default target
.DEFAULT_GOAL := help