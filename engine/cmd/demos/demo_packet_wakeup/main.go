package main

import (
	"fmt"
	"log"
	_ "mywant/engine/cmd/types"
	mywant "mywant/engine/src"
	"os"
)

func main() {
	fmt.Println("🔄 Packet Wake-up Test Server")
	fmt.Println("==============================")
	fmt.Println("This demo tests automatic wake-up of completed wants when receiving packets.")
	fmt.Println()
	configFile := "yaml/config/config-packet-test.yaml"
	if len(os.Args) > 1 {
		configFile = os.Args[1]
	}

	// Load config
	config, err := mywant.LoadConfigFromYAML(configFile)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	fmt.Printf("📝 Loaded %d wants from configuration\n", len(config.Wants))
	builder := mywant.NewChainBuilder(config)

	// Register want types

	// Execute
	fmt.Println("\n🚀 Executing simple 3-want pipeline (gen → queue → sink)")
	fmt.Println("   Wants will complete, then monitor for packets")
	fmt.Println("   Server will keep running - use API to update parameters")
	fmt.Println("   API: http://localhost:8080/api/v1/wants")
	fmt.Println()

	builder.Execute()

	fmt.Println("\n✅ Server running! Watch for [MONITOR] and [TRIGGER:PACKET] messages")

	// Keep running forever (server mode)
	select {}
}
