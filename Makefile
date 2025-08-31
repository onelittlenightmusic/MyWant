.PHONY: qnet clean run-demo_qnet run-prime run-fibonacci run-fibonacci-loop run-sample-owner run-qnet-target run-travel-target run-travel run-travel-recipe run-queue-system-recipe run-qnet-recipe run-qnet-using-recipe

run-qnet:
	go run demo_qnet.go declarative.go qnet_types.go config/config-qnet.yaml

run-prime:
	go run demo_prime.go declarative.go prime_types.go

run-fibonacci:
	go run demo_fibonacci.go declarative.go fibonacci_types.go

run-fibonacci-loop:
	go run demo_fibonacci_loop.go declarative.go fibonacci_loop_types.go config/config-fibonacci-loop.yaml

run-sample-owner:
	go run demo_qnet_owner.go declarative.go owner_types.go qnet_types.go recipe_loader_generic.go

run-qnet-target:
	go run demo_qnet_owner.go declarative.go qnet_types.go owner_types.go recipe_loader_generic.go config/config-qnet.yaml

run-travel-target:
	go run demo_travel_target.go declarative.go travel_types.go qnet_types.go owner_types.go recipe_loader_generic.go config/config-travel-target.yaml

run-travel:
	go run demo_travel.go declarative.go travel_types.go config/config-travel.yaml

run-travel-recipe:
	go run demo_travel_recipe.go declarative.go travel_types.go recipe_loader_generic.go config/config-travel-recipe.yaml

run-queue-system-recipe:
	go run demo_queue_system_recipe.go declarative.go qnet_types.go recipe_loader_generic.go config/config-queue-system-recipe.yaml

run-qnet-recipe:
	go run demo_qnet_recipe.go declarative.go qnet_types.go recipe_loader_generic.go config/config-qnet-recipe.yaml

run-qnet-using-recipe:
	go run demo_qnet_using_recipe.go declarative.go qnet_types.go config/config-qnet-using-recipe.yaml

clean:
	rm -f qnet

all: qnet