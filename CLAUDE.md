# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

GoChain is a Go library implementing functional chain programming patterns with channels. The project now supports both imperative chain building and declarative configuration-based approaches, enabling flexible stream processing and pipeline architectures.

## Core Architecture

### Main Components

- **chain/chain.go**: Primary chain library with `C_chain` struct and functional chain operations
- **chain.go**: Legacy/alternative implementation of chain functionality  
- **chain_qnet.go**: Original imperative example demonstrating queueing network simulation
- **declarative.go**: New declarative configuration system with JSON/YAML support
- **example_declarative.go**: JSON-based configuration example
- **example_yaml.go**: YAML-based configuration example

### Key Types and Concepts

#### Declarative API
- `Metadata`: Node identification with `Name`, `Type`, `Labels`, and `Connectivity`
- `NodeSpec`: Node configuration with `Params`, legacy `Inputs`, and new `Paths`
- `Config`: Complete network configuration with array of `Node`s
- `ChainBuilder`: Builds and executes chains from declarative configuration

#### Path Management System
- `PathInfo`: Connection information for a single path (channel, name, active status)
- `Paths`: Manages all input and output connections for a node
- `ConnectivityMetadata`: Defines node connectivity requirements and constraints
- `EnhancedBaseNode`: Interface for path-aware nodes with connectivity validation
- `PathSpec`: Path configuration in YAML/JSON with selectors and activity
- `PathsSpec`: Complete path specifications for inputs and outputs

### Chain Function Signatures

Functions added to chains follow these patterns:
- Start functions: `func(Chan) bool`
- Processing functions: `func(Chan, Chan) bool` (input, output channels)
- End functions: `func(Chan) bool`

## Development Commands

### Module Initialization
```sh
go mod init gochain
go mod tidy
```

### Building and Running Examples

#### Legacy Imperative Examples
```sh
# Run original queueing network example
go run chain_qnet.go

# Run other examples
go run closure_test.go
go run interface_test.go
```

#### Declarative Configuration Examples
```sh
# Run JSON-based configuration
go run example_declarative.go declarative.go

# Run YAML-based configuration  
go run example_yaml.go declarative.go
```

### Configuration Files
- `config.json`: JSON format configuration example
- `config.yaml`: YAML format configuration example
- `config_clean.yaml`: Clean YAML format with optional inputs

## Code Patterns

### Declarative Configuration Usage

#### JSON Configuration
```go
config, err := loadConfigFromJSON("config.json")
builder := NewChainBuilder(config)
builder.Build()
builder.Execute()
```

#### YAML Configuration
```go
config, err := loadConfigFromYAML("config.yaml")
builder := NewChainBuilder(config)
builder.Build()
builder.Execute()
```

#### Queueing Network Configuration (config-qnet.yaml)
```yaml
nodes:
  # Primary packet generator
  - metadata:
      name: gen-primary
      type: sequence
      labels:
        role: source
        stream: primary
        priority: high
    spec:
      params:
        rate: 3.0
        count: 10000

  # Main queue processing primary stream
  - metadata:
      name: queue-primary
      type: queue
      labels:
        role: processor
        stage: first
        stream: primary
        path: main
    spec:
      params:
        service_time: 0.5
      inputs:
        - role: source
          stream: primary

  # Stream combiner
  - metadata:
      name: combiner-main
      type: combiner
      labels:
        role: merger
        operation: combine
        stage: second
    spec:
      params: {}
      inputs:
        - role: processor
          stage: first

  # Final collector
  - metadata:
      name: collector-end
      type: sink
      labels:
        role: terminal
        stage: end
    spec:
      params: {}
      inputs:
        - role: processor
          stage: final
```

### Dynamic Node Addition

Nodes can be added to a running chain dynamically using the `AddDynamicNode` and `AddDynamicNodes` methods:

```go
// Add a single dynamic node
builder.AddDynamicNode(Node{
    Metadata: Metadata{
        Name: "dynamic-processor",
        Type: "queue",
        Labels: map[string]string{
            "role": "processor",
            "stage": "dynamic",
        },
    },
    Spec: NodeSpec{
        Params: map[string]interface{}{
            "service_time": 0.4,
        },
        Inputs: []InputSelector{{
            "role": "source",
        }},
    },
})

// Add multiple dynamic nodes
builder.AddDynamicNodes([]Node{node1, node2, node3})
```

Dynamic nodes are automatically connected based on their label selectors and integrate seamlessly with the existing chain topology.

### Memory Reconciliation

The system supports memory reconciliation for persistent state management across chain executions:

```go
// Memory is automatically loaded from YAML files at startup
// and saved during chain execution for state persistence

// Example memory configuration structure:
// - Node states and statistics
// - Connection topology
// - Processing parameters
// - Runtime metrics
```

Memory reconciliation enables:
- **State Persistence**: Node states survive chain restarts
- **Configuration Recovery**: Automatic reload of previous configurations
- **Statistics Continuity**: Processing metrics maintained across executions
- **Dynamic Topology**: Preserved connections for dynamically added nodes

## Dependencies

- Go 1.24.5+
- `gopkg.in/yaml.v3` for YAML configuration support

## Important Notes

### Legacy API
- The library uses Go channels extensively for communication between chain stages
- Functions in chains should return `true` when processing is complete, `false` to continue
- The `chain.Run()` function must be called to execute the constructed chains
- Channel buffer sizes are typically set to 10 for intermediate stages

### Declarative API
- Nodes are connected via label-based selectors, eliminating order dependencies
- Configuration supports both JSON and YAML formats with same struct definitions
- `inputs` field is optional - omit for source nodes (generators)
- Label matching allows flexible topology definition without hardcoded node names
- ChainBuilder handles the complexity of wiring connections based on selectors

### File Structure
- Use `declarative.go` when working with configuration-based chains
- Legacy examples demonstrate the original imperative API
- Both approaches can coexist and serve different use cases