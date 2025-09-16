.PHONY: clean build test-build run-example run-qnet run-prime run-fibonacci run-fibonacci-loop run-travel run-sample-owner run-sample-owner-config run-qnet-target run-travel-target run-qnet-using-recipe run-travel-recipe run-queue-system-recipe run-qnet-recipe run-prime-recipe run-fibonacci-recipe run-notification-demo build-server run-server

# Build the mywant library
build:
	go build ./src/...

# Test that module builds correctly  
test-build:
	go mod tidy && go build ./src/...

run-qnet:
	go run cmd/demos/demo_qnet.go cmd/demos/qnet_types.go config/config-qnet.yaml

run-prime:
	go run cmd/demos/demo_prime.go cmd/demos/prime_types.go

run-fibonacci:
	go run cmd/demos/demo_fibonacci.go cmd/demos/fibonacci_types.go

run-fibonacci-loop:
	go run cmd/demos/demo_fibonacci_loop.go cmd/demos/fibonacci_loop_types.go config/config-fibonacci-loop.yaml

run-sample-owner:
	go run cmd/demos/demo_qnet_owner.go cmd/demos/qnet_types.go

run-sample-owner-config:
	go run cmd/demos/demo_qnet_owner.go cmd/demos/qnet_types.go config/config-sample-owner.yaml

run-qnet-target:
	go run cmd/demos/demo_qnet_owner.go cmd/demos/qnet_types.go config/config-qnet-target.yaml

run-travel-target:
	go run cmd/demos/demo_travel_target.go cmd/demos/travel_types.go config/config-travel-target.yaml

run-travel:
	go run cmd/demos/demo_travel.go cmd/demos/travel_types.go config/config-travel.yaml

run-qnet-using-recipe:
	go run cmd/demos/demo_qnet_using_recipe.go cmd/demos/qnet_types.go config/config-fibonacci-using-recipe.yaml

run-travel-recipe:
	go run cmd/demos/demo_travel_recipe.go cmd/demos/travel_types.go

run-queue-system-recipe:
	go run cmd/demos/demo_queue_system_recipe.go cmd/demos/qnet_types.go

run-qnet-recipe:
	go run cmd/demos/demo_qnet_recipe.go cmd/demos/qnet_types.go

run-prime-recipe:
	go run cmd/demos/demo_prime_recipe.go cmd/demos/prime_types.go

run-fibonacci-recipe:
	go run cmd/demos/demo_fibonacci_recipe.go cmd/demos/fibonacci_types.go

run-notification-demo:
	go run cmd/demos/demo_notification_system.go cmd/demos/qnet_types.go config/config-notification-demo.yaml

run-target-notifications:
	go run cmd/demos/demo_target_notifications.go cmd/demos/qnet_types.go config/config-target-notification-test.yaml

run-parameter-history-test:
	go run cmd/demos/demo_parameter_history.go cmd/demos/qnet_types.go

run-qnet-with-params:
	go run cmd/demos/demo_qnet_with_params.go cmd/demos/qnet_types.go config/config-qnet.yaml

# Build the mywant server binary
build-server:
	go build -o bin/mywant cmd/server/*.go

# Run the mywant server
run-server: build-server
	./bin/mywant 8080 localhost

clean:
	rm -f qnet bin/mywant
	go clean

all: build