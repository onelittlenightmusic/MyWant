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

#### Legacy Chain API
- `Tuple interface{}`: Generic data type that flows through chains
- `Chan chan Tuple`: Channel type for chain communication
- `C_chain struct`: Main chain object with methods:
  - `Start()`: Initialize chain with starting function
  - `Add()`: Add processing function to chain
  - `End()`: Terminate chain with ending function
  - `Merge()`: Combine two chains
  - `Split()`: Split chain into two parallel paths

#### Enhanced Declarative API
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

### Legacy Chain Usage (Imperative)
```go
var c chain.C_chain
c.Add(init_func(3.0, 1000))    // Start with data generator
c.Add(queue(0.5))              // Add processing stage
c.Add(queue(0.9))              // Add another stage
c.End(end_func)                // Terminate chain
chain.Run()                    // Execute the chain
```

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

#### Configuration Structure
```yaml
nodes:
  - metadata:
      name: gen-primary
      type: generator
      labels:
        role: primary-source
    spec:
      params:
        rate: 3.0
        count: 1000
      # inputs field is optional for source nodes
  
  - metadata:
      name: queue-1
      type: queue
      labels:
        stage: first
    spec:
      params:
        service_time: 0.5
      inputs:
        - role: primary-source  # Label-based selector
```

### Advanced Chain Operations
```go
// Merging chains (legacy)
c.Merge(c2, combine_func)

// Splitting chains (legacy)
c3 := c.Split(split_func)
```

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