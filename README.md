# GoChain

A Go library implementing functional chain programming patterns with channels, supporting both imperative chain building and declarative configuration-based approaches for flexible stream processing and pipeline architectures.

## Features

- **Declarative Configuration**: Define complex processing topologies using YAML/JSON configurations
- **Dynamic Node Addition**: Add nodes to running chains at runtime
- **Memory Reconciliation**: Persistent state management across chain executions
- **Label-based Connectivity**: Flexible node connections using label selectors
- **Multi-stream Processing**: Support for parallel processing streams with combiners

## Quick Start

### YAML Configuration

Define your processing chain in `config-qnet.yaml`:

```yaml
nodes:
  # Source node generating data
  - metadata:
      name: gen-primary
      type: sequence
      labels:
        role: source
        stream: primary
    spec:
      params:
        rate: 3.0
        count: 10000

  # Processing node
  - metadata:
      name: queue-primary
      type: queue
      labels:
        role: processor
        stage: first
    spec:
      params:
        service_time: 0.5
      inputs:
        - role: source
          stream: primary

  # Terminal node
  - metadata:
      name: collector-end
      type: sink
      labels:
        role: terminal
    spec:
      inputs:
        - role: processor
```

### Running Examples

```sh
# Run YAML-based queueing network
go run example_yaml.go declarative.go

# Run with dynamic node addition
go run demo_qnet_owner.go declarative.go owner_types.go dynamic_chain.go
```

### Dynamic Node Addition

```go
// Add nodes at runtime
builder.AddDynamicNode(Node{
    Metadata: Metadata{
        Name: "dynamic-processor",
        Type: "queue",
        Labels: map[string]string{"role": "processor"},
    },
    Spec: NodeSpec{
        Params: map[string]interface{}{"service_time": 0.4},
        Inputs: []InputSelector{{"role": "source"}},
    },
})
```

### Memory Reconciliation

The system automatically manages persistent state:
- Node states and statistics survive restarts
- Configuration recovery from memory dumps
- Dynamic topology preservation