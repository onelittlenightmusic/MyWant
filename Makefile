.PHONY: clean build test-build run-example run-qnet run-prime run-prime-recipe run-fibonacci run-fibonacci-recipe run-fibonacci-loop run-sample-owner run-sample-owner-config run-sample-owner-high-throughput run-sample-owner-dry run-sample-owner-input run-qnet-target run-travel-target run-travel run-travel-recipe run-queue-system-recipe run-qnet-recipe run-qnet-using-recipe

# Build the mywant library
build:
	go build .

# Test that module builds correctly  
test-build:
	go mod tidy && go build .

run-qnet:
	go run cmd/demos/demo_qnet.go cmd/demos/qnet_types.go config/config-qnet.yaml

run-prime:
	go run cmd/demos/demo_prime.go cmd/demos/prime_types.go

run-prime-recipe:
	go run cmd/demos/demo_prime_recipe.go cmd/demos/prime_types.go config/config-prime-recipe.yaml

run-fibonacci:
	go run cmd/demos/demo_fibonacci.go cmd/demos/fibonacci_types.go

run-fibonacci-recipe:
	go run cmd/demos/demo_fibonacci_recipe.go cmd/demos/fibonacci_types.go config/config-fibonacci-recipe.yaml

run-fibonacci-loop:
	go run cmd/demos/demo_fibonacci_loop.go cmd/demos/fibonacci_loop_types.go config/config-fibonacci-loop.yaml

run-sample-owner:
	go run cmd/demos/demo_qnet_owner.go cmd/demos/qnet_types.go

run-sample-owner-config:
	go run cmd/demos/demo_qnet_owner.go cmd/demos/qnet_types.go config/config-sample-owner.yaml

run-sample-owner-high-throughput:
	go run cmd/demos/demo_qnet_owner.go cmd/demos/qnet_types.go config/config-sample-owner-high-throughput.yaml

run-sample-owner-dry:
	go run cmd/demos/demo_qnet_owner.go cmd/demos/qnet_types.go config/config-sample-owner-dry.yaml

run-sample-owner-input:
	go run cmd/demos/demo_qnet_owner.go cmd/demos/qnet_types.go config/config-sample-owner-input.yaml

run-qnet-target:
	go run cmd/demos/demo_qnet_owner.go cmd/demos/qnet_types.go config/config-qnet.yaml

run-travel-target:
	go run cmd/demos/demo_travel_target.go cmd/demos/travel_types.go config/config-travel-target.yaml

run-travel:
	go run cmd/demos/demo_travel.go cmd/demos/travel_types.go config/config-travel.yaml

run-travel-recipe:
	go run cmd/demos/demo_travel_recipe.go cmd/demos/travel_types.go config/config-travel-recipe.yaml

run-queue-system-recipe:
	go run cmd/demos/demo_queue_system_recipe.go cmd/demos/qnet_types.go config/config-queue-system-recipe.yaml

run-qnet-recipe:
	go run cmd/demos/demo_qnet_recipe.go cmd/demos/qnet_types.go config/config-qnet-recipe.yaml

run-qnet-using-recipe:
	go run cmd/demos/demo_qnet_using_recipe.go cmd/demos/qnet_types.go config/config-qnet-using-recipe.yaml

# Run the example project
run-example:
	cd ../mywant-example && go run .

clean:
	rm -f qnet
	go clean

all: build