.PHONY: qnet clean run-demo_qnet run-prime run-fibonacci run-fibonacci-loop run-sample-owner run-travel run-travel-template run-queue-system-template

run-qnet:
	go run demo_qnet.go declarative.go qnet_types.go config-qnet.yaml

run-prime:
	go run demo_prime.go declarative.go prime_types.go

run-fibonacci:
	go run demo_fibonacci.go declarative.go fibonacci_types.go

run-fibonacci-loop:
	go run demo_fibonacci_loop.go declarative.go fibonacci_loop_types.go config-fibonacci-loop.yaml

run-sample-owner:
	go run demo_qnet_owner.go declarative.go owner_types.go qnet_types.go template_loader.go

run-travel:
	go run demo_travel.go declarative.go travel_types.go config-travel.yaml

run-travel-template:
	go run demo_travel_template.go declarative.go travel_types.go template_loader_generic.go config-travel-template.yaml

run-queue-system-template:
	go run demo_queue_system_template.go declarative.go qnet_types.go template_loader_generic.go config-queue-system-template.yaml

clean:
	rm -f qnet

all: qnet