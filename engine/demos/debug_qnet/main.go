package main

import (
	"fmt"
	mywant "mywant/engine/core"
	_ "mywant/engine/types"
)

func main() {
	config := mywant.Config{
		Wants: []*mywant.Want{
			{
				Metadata: mywant.Metadata{
					Name:   "gen",
					Type:   "qnet numbers",
					Labels: map[string]string{"role": "source"},
				},
				Spec: mywant.WantSpec{
					Params: map[string]any{
						"count":         5,
						"rate":          1.0,
						"deterministic": true,
					},
				},
			},
			{
				Metadata: mywant.Metadata{
					Name:   "queue",
					Type:   "qnet queue",
					Labels: map[string]string{"role": "processor"},
				},
				Spec: mywant.WantSpec{
					Params: map[string]any{"service_time": 0.1},
					Using:  []map[string]string{{"role": "source"}},
				},
			},
		},
	}

	fmt.Println("Creating ChainBuilder...")
	builder := mywant.NewChainBuilder(config)

	fmt.Println("\nBefore reconciliation:")
	states := builder.GetAllWantStates()
	for name, state := range states {
		fmt.Printf("  %s: status=%s, inputs=%d, outputs=%d\n",
			name, state.Status, state.GetInCount(), state.GetOutCount())
	}
}
