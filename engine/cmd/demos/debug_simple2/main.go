package main

import (
	"fmt"
	_ "mywant/engine/cmd/types"
	mywant "mywant/engine/src"
	"time"
)

func main() {
	wants := []*mywant.Want{
		{
			Metadata: mywant.Metadata{
				Name:   "test_gen",
				Type:   "qnet numbers",
				Labels: map[string]string{"role": "source"},
			},
			Spec: mywant.WantSpec{
				Params: map[string]any{
					"count":         3,
					"rate":          1.0,
					"deterministic": true,
				},
			},
		},
	}

	config := &mywant.Config{Wants: wants}
	builder := mywant.NewChainBuilder(*config)
	

	// Execute in background goroutine
	go builder.Execute()

	// Wait and check status
	for i := 0; i < 3; i++ {
		time.Sleep(1 * time.Second)
		states := builder.GetAllWantStates()
		for name, state := range states {
			fmt.Printf("[%d] %s: status=%s\n", i, name, state.Status)
		}
	}
}
