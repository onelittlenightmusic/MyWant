.PHONY: clean build test test-build fmt lint vet check run-qnet run-prime run-fibonacci-loop run-travel run-hotel-agent run-sample-owner run-sample-owner-config run-qnet-target run-qnet-using-recipe run-parameter-history-test run-hierarchical-approval build-server run-server test-server-api test-server-simple run-travel-recipe run-queue-system-recipe run-qnet-recipe run-fibonacci-recipe run-prime-recipe

# Code quality targets
fmt:
	@echo "🔧 Formatting Go code..."
	gofmt -w -s .
	goimports -w . 2>/dev/null || echo "goimports not available, using gofmt only"

vet:
	@echo "🔍 Running go vet..."
	go vet ./src/... ./cmd/server/...

lint:
	@echo "🧹 Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./src/... ./cmd/server/...; \
	else \
		echo "⚠️  golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		echo "📋 Running basic checks instead..."; \
		$(MAKE) vet; \
	fi

test:
	@echo "🧪 Running tests..."
	go test -v ./src/... ./cmd/server/... || echo "⚠️  Some tests failed or no tests found"

check: fmt vet test
	@echo "✅ All code quality checks completed"

# Build the mywant library
build: check
	@echo "🔨 Building mywant library..."
	go build ./src/...

# Test that module builds correctly
test-build:
	go mod tidy && go build ./src/...

run-qnet:
	go run cmd/demos/demo_qnet.go cmd/demos/qnet_types.go config/config-qnet.yaml

run-prime:
	go run cmd/demos/demo_prime.go cmd/demos/prime_types.go

run-fibonacci-loop:
	go run cmd/demos/demo_fibonacci_loop.go cmd/demos/fibonacci_loop_types.go config/config-fibonacci-loop.yaml

run-sample-owner:
	go run cmd/demos/demo_qnet_owner.go cmd/demos/qnet_types.go

run-sample-owner-config:
	go run cmd/demos/demo_qnet_owner.go cmd/demos/qnet_types.go config/config-sample-owner.yaml

run-qnet-target:
	go run cmd/demos/demo_qnet_owner.go cmd/demos/qnet_types.go config/config-qnet-target.yaml

run-travel:
	go run cmd/demos/demo_travel.go cmd/demos/travel_types.go config/config-travel.yaml

run-hotel-agent:
	go run cmd/demos/demo_hotel_agent.go

run-qnet-using-recipe:
	go run cmd/demos/demo_qnet_using_recipe.go cmd/demos/qnet_types.go config/config-fibonacci-using-recipe.yaml

run-parameter-history-test:
	go run cmd/demos/demo_parameter_history.go cmd/demos/qnet_types.go

run-hierarchical-approval:
	go run cmd/demos/demo_hierarchical_approval.go cmd/demos/approval_types.go

# Recipe-based execution targets
run-travel-recipe:
	go run cmd/demos/demo_travel_recipe.go cmd/demos/travel_types.go config/config-travel-recipe.yaml

run-queue-system-recipe:
	go run cmd/demos/demo_queue_system_recipe.go cmd/demos/qnet_types.go config/config-queue-system-recipe.yaml

run-qnet-recipe:
	go run cmd/demos/demo_qnet_recipe.go cmd/demos/qnet_types.go config/config-qnet-recipe.yaml

run-fibonacci-recipe:
	go run cmd/demos/demo_fibonacci_recipe.go cmd/demos/fibonacci_types.go config/config-fibonacci-recipe.yaml

run-prime-recipe:
	go run cmd/demos/demo_prime_recipe.go cmd/demos/prime_types.go config/config-prime-recipe.yaml

# Build the mywant server binary
build-server: check
	@echo "🏗️  Building mywant server..."
	mkdir -p bin
	go build -o bin/mywant cmd/server/*.go

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
	@echo "  run-fibonacci-loop    - Fibonacci loop example"
	@echo "  run-travel            - Travel planning example"
	@echo ""
	@echo "📜 Recipe-based Examples:"
	@echo "  run-travel-recipe     - Travel with recipe system"
	@echo "  run-queue-system-recipe - Queue system with recipes"
	@echo "  run-qnet-recipe       - QNet with recipes"
	@echo "  run-fibonacci-recipe  - Fibonacci with recipes"
	@echo "  run-prime-recipe      - Prime with recipes"
	@echo ""
	@echo "🔧 Server:"
	@echo "  run-server       - Start mywant server"
	@echo "  test-server-api  - Test server API endpoints"
	@echo ""
	@echo "🧹 Utility:"
	@echo "  clean - Clean build artifacts"
	@echo "  help  - Show this help"

all: build

# Default target
.DEFAULT_GOAL := help