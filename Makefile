.PHONY: clean build test-build run-example run-qnet run-prime run-fibonacci run-fibonacci-loop run-travel run-sample-owner run-sample-owner-config run-qnet-target run-travel-target run-qnet-using-recipe

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
	go run cmd/demos/demo_qnet_using_recipe.go cmd/demos/qnet_types.go config/config-qnet-using-recipe.yaml

clean:
	rm -f qnet
	go clean

all: build