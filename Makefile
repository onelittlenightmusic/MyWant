.PHONY: qnet clean run-demo_qnet run-prime run-fibonacci run-fibonacci-loop run-sample-owner

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

clean:
	rm -f qnet

all: qnet