.PHONY: qnet clean run-demo_qnet run-prime run-fibonacci

run-qnet:
	go run demo_qnet.go declarative.go qnet_types.go config-qnet.yaml

run-prime:
	go run demo_prime.go declarative.go prime_types.go

run-fibonacci:
	go run demo_fibonacci.go declarative.go fibonacci_types.go

prime:
	go run prime.go declarative.go

fibonacci:
	go run fibonacci.go declarative.go
clean:
	rm -f qnet

all: qnet