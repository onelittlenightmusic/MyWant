package main

import (
	"fmt"
	"time"

	_ "mywant/engine/cmd/types"
	mywant "mywant/engine/src"
)

func main() {
	fmt.Println("🧪 Parameter History Test Demo")
	fmt.Println("===============================")
	fmt.Println("This demo tests the parameterHistory functionality by")
	fmt.Println("creating a simple want and then updating its parameters")
	fmt.Println("multiple times to verify parameter history tracking.")
	config := mywant.Config{
		Wants: []*mywant.Want{
			{
				Metadata: mywant.Metadata{
					Name: "test-want",
					Type: "numbers",
					Labels: map[string]string{
						"role": "test",
					},
				},
				Spec: mywant.WantSpec{
					Params: map[string]any{
						"count": 100,
						"rate":  1.0,
					},
				},
				State: map[string]any{
					"initial_state": "initialized",
				},
			},
		},
	}
	builder := mywant.NewChainBuilder(config)

	fmt.Println("🔧 Creating test want...")
	testWant := config.Wants[0]

	fmt.Println("📝 Testing parameter updates...")

	// Update parameters multiple times to generate parameter history
	fmt.Println("  → Updating 'count' from 100 to 200")
	testWant.UpdateParameter("count", 200)

	time.Sleep(50 * time.Millisecond)

	fmt.Println("  → Updating 'rate' from 1.0 to 2.5")
	testWant.UpdateParameter("rate", 2.5)

	time.Sleep(50 * time.Millisecond)

	fmt.Println("  → Updating 'count' from 200 to 500")
	testWant.UpdateParameter("count", 500)

	time.Sleep(50 * time.Millisecond)

	fmt.Println("  → Updating 'rate' from 2.5 to 0.5")
	testWant.UpdateParameter("rate", 0.5)

	time.Sleep(50 * time.Millisecond)

	fmt.Println("  → Adding new parameter 'description'")
	testWant.UpdateParameter("description", "test parameter")
	fmt.Printf("📊 State before execution: %+v\n", testWant.State)

	// Now execute the chain
	fmt.Println("🚀 Starting chain execution...")
	builder.Execute()
	fmt.Printf("📊 State after execution: %+v\n", testWant.State)

	fmt.Println("✅ Parameter history test completed!")
	fmt.Println("")
	fmt.Println("Check the memory dump files to see the parameterHistory entries.")
}
