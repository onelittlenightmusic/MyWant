package main

import (
    "fmt"
    types "mywant/engine/cmd/types"
    mywant "mywant/engine/src"
)

func main() {
    // Simple inline config
    wants := []*mywant.Want{
        {
            Metadata: mywant.Metadata{
                Name: "test_gen",
                Type: "qnet numbers",
                Labels: map[string]string{"role": "source"},
            },
            Spec: mywant.WantSpec{
                Params: map[string]any{
                    "count": 5,
                    "rate": 1.0,
                    "deterministic": true,
                },
            },
        },
    }

    config := &mywant.Config{Wants: wants}

    fmt.Println("Creating ChainBuilder...")
    builder := mywant.NewChainBuilder(*config)
    types.RegisterQNetWantTypes(builder)

    fmt.Println("\n=== Before Execute ===")
    states := builder.GetAllWantStates()
    for name, state := range states {
        fmt.Printf("%s: status=%s\n", name, state.Status)
    }

    fmt.Println("\nCalling builder.Execute()...")
    builder.Execute()

    fmt.Println("\n=== After Execute ===")
    states = builder.GetAllWantStates()
    for name, state := range states {
        fmt.Printf("%s: status=%s, state=%+v\n", name, state.Status, state.State)
    }
}
