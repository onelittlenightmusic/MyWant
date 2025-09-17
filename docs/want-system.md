# MyWant Want System Documentation

## Overview

The MyWant Want System provides a **declarative configuration framework** for functional chain programming with channels. Wants are the fundamental processing units that define "what you want to achieve" rather than "how to achieve it", enabling a highly flexible and composable system architecture.

## Table of Contents

- [Want Structure & Declarative Configuration](#want-structure--declarative-configuration)
- [Connection Patterns & State Management](#connection-patterns--state-management)
- [Configuration Examples & Best Practices](#configuration-examples--best-practices)

## Want Structure & Declarative Configuration

### What is a Want?

A **Want** represents a declarative specification of a desired outcome. Instead of imperative "do this, then do that" programming, wants express "I want this result" and let the system determine how to achieve it.

```yaml
# Declarative: "I want a hotel booking"
- metadata:
    name: luxury-hotel-booking
    type: hotel
  spec:
    params:
      hotel_type: luxury
      check_in: "2025-09-20"
```

### Complete Want Structure

```yaml
wants:
  - metadata:                    # Want identification and classification
      name: "unique-want-name"   # Unique identifier within config
      type: "processing-type"    # Determines implementation to use
      labels:                    # Key-value pairs for selection/grouping
        role: "processor"
        category: "data-analysis"

    spec:                        # Desired state configuration
      params:                    # Want-specific parameters
        processing_mode: "batch"
        batch_size: 1000
      using:                     # Input connections from other wants
        - role: "data-source"    # Label selector - connects to any want with role=data-source
      requires:                  # Required capabilities (triggers agent execution)
        - "data_validation"
        - "error_handling"

    status: "running"            # Current execution status (idle/running/completed/failed)
    state:                       # Current runtime state (key-value pairs)
      processed_count: 1500
      last_update: "2025-09-18T10:30:00Z"
```

### Declarative Configuration Philosophy

MyWant embraces **declarative configuration** - you describe the **desired end state** rather than the steps to achieve it:

**❌ Imperative (Traditional)**:
```go
func processData() {
    data := readInput()
    cleaned := cleanData(data)
    validated := validateData(cleaned)
    writeOutput(validated)
}
```

**✅ Declarative (MyWant)**:
```yaml
wants:
  - metadata: {name: "data-cleaner", type: "cleaner"}
    spec: {params: {cleaning_rules: "standard"}}

  - metadata: {name: "data-validator", type: "validator"}
    spec:
      params: {validation_schema: "v2"}
      using: [{type: "cleaner"}]
```

### Recipe System

**Recipes** provide reusable component templates:

```yaml
# recipes/data-pipeline.yaml
recipe:
  parameters:
    input_format: "csv"
    batch_size: 1000

  wants:
    - type: "data_reader"
      params: {format: input_format, batch_size: batch_size}
    - type: "data_validator"
      using: [{type: "data_reader"}]
```

## Connection Patterns & State Management

### Label-Based Connection Patterns

MyWant uses **label selectors** for flexible connectivity:

```yaml
# Sequential Pipeline
wants:
  - metadata: {name: "generator", type: "source", labels: {role: "producer"}}

  - metadata: {name: "processor", type: "transform", labels: {role: "processor"}}
    spec:
      using: [{role: "producer"}]  # Sequential dependency

  - metadata: {name: "sink", type: "destination"}
    spec:
      using: [{role: "processor"}] # Final stage
```

```yaml
# Fan-Out Pattern
wants:
  - metadata: {name: "source", type: "generator", labels: {role: "producer"}}

  - metadata: {name: "analyzer", type: "analyzer"}
    spec:
      using: [{role: "producer"}]  # Both connect to same source

  - metadata: {name: "enricher", type: "enricher"}
    spec:
      using: [{role: "producer"}]  # Fan-out from source
```

### State Management

MyWant uses **dual-layer state management**:

```go
// Batched state updates (recommended)
want.BeginExecCycle()
want.StageStateChange(map[string]interface{}{
    "batch_size": 1000,
    "processed_count": 5000,
    "status": "processing",
})
want.EndExecCycle()  // Commits all staged changes atomically

// Agent state updates
want.StageStateChange("reservation_id", "HTL-12345")
want.StageStateChange("status", "confirmed")
want.CommitStateChanges()  // Atomic commit
```

### State History & Subscriptions

```yaml
# State subscriptions between wants
spec:
  stateSubscriptions:
    - wantName: "payment-processor"
      stateKeys: ["status", "transaction_id"]
      conditions: ["status == 'completed'"]
```

## Configuration Examples & Best Practices

### Configuration Examples

```yaml
# Simple Data Processing Pipeline
wants:
  - metadata: {name: "csv-reader", type: "file_reader", labels: {role: "source"}}
    spec:
      params: {file_path: "/data/input.csv", batch_size: 1000}

  - metadata: {name: "data-cleaner", type: "data_cleaner", labels: {role: "processor"}}
    spec:
      params: {cleaning_rules: ["trim_whitespace", "remove_duplicates"]}
      using: [{role: "source"}]

  - metadata: {name: "json-writer", type: "file_writer"}
    spec:
      params: {file_path: "/data/output.json", format: "json"}
      using: [{role: "processor"}]
```

```yaml
# Recipe-Based Configuration
recipe:
  path: "recipes/etl-pipeline.yaml"
  parameters:
    input_file: "/data/sales-2024.csv"
    output_file: "/data/processed-sales.json"
    validation_level: "strict"
    chunk_size: 5000
```

```yaml
# Agent Integration
wants:
  - metadata: {name: "hotel-booking", type: "hotel"}
    spec:
      params: {hotel_type: "luxury", check_in: "2025-09-20"}
      requires: ["hotel_reservation", "payment_processing"]  # Triggers agent execution
```

### Best Practices

```yaml
# ✅ Good naming and labeling
metadata:
  name: "user-data-processor"     # kebab-case, descriptive
  labels:
    role: "processor"             # Functional role
    layer: "transformation"       # Architecture layer
    domain: "user-management"     # Business domain

# ✅ Well-structured parameters
spec:
  params:
    batch_size: 1000             # Operational parameters
    processing_mode: "strict"    # Business parameters
    database_url: "${DATABASE_URL}"  # Environment variables

# ✅ Flexible connections
spec:
  using:
    - role: "data-source"        # Role-based connection
    - layer: "processing"        # Layer-based connection
      status: "healthy"          # Multi-criteria selection
```

```go
// ✅ Efficient state management
want.BeginExecCycle()
want.StageStateChange(map[string]interface{}{
    "processed_count": count,
    "status": "active",
    "metrics": map[string]interface{}{
        "throughput": tps,
        "error_rate": errorRate,
    },
})
want.EndExecCycle()  // Atomic commit
```

MyWant's declarative want system transforms complex distributed processing into simple, readable configuration that expresses **what you want to achieve** rather than **how to achieve it**.