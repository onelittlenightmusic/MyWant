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
# yaml/recipes/data-pipeline.yaml
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

#### Recipe Storage Locations

レシピファイルは2箇所から読み込まれます:

| 場所 | パス | 用途 |
|------|------|------|
| ビルトイン | `yaml/recipes/` | リポジトリ同梱のサンプル・共有レシピ |
| ユーザー保存 | `~/.mywant/recipes/` | "Save as Recipe" で保存したレシピ |

サーバー起動時に両方のディレクトリを自動的にスキャンし、レシピレジストリに登録します。

#### Save as Recipe

デプロイ済みの Want を再利用可能なレシピとして保存できます。

**ダッシュボード (UI) から保存:**

1. Want の詳細サイドバーを開く（target want のみ有効）
2. "Save Recipe" ボタンをクリック
3. 名前・説明・バージョン・カテゴリを入力して保存

保存先: `~/.mywant/recipes/{recipe-name}.yaml`

**CLI から保存:**

```bash
./bin/mywant recipes from-want <WANT_ID> --name "my-recipe"
# Short: ./bin/mywant r fw <WANT_ID> -n "my-recipe"
```

**保存されるYAML構造:**

```yaml
recipe:
  metadata:
    name: my-recipe
    description: "Optional description"
    version: "1.0.0"
    category: general        # general / approval / travel / mathematics / queue
    custom_type: ""          # オプション: カスタム型名
  wants:
    - metadata:
        name: child-want-name
        type: want-type
        labels: {}
      spec:
        params: {...}
        requires: [...]
  state:
    - name: field_name       # 子 Want の capabilities から自動検出
      description: ""
      type: ""
  parameters: {}
```

**起動時の自動ロード:**

サーバーは起動時に以下の順序でレシピをロードします:

```
1. yaml/recipes/          → ビルトインレシピ (ScanAndRegisterCustomTypes)
2. ~/.mywant/recipes/     → ユーザー保存レシピ (ScanAndRegisterCustomTypes)
```

`~/.mywant/recipes/` が存在しない場合はスキップされます（エラーにはなりません）。

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
want.BeginProgressCycle()
want.StageStateChange(map[string]interface{}{
    "batch_size": 1000,
    "processed_count": 5000,
    "status": "processing",
})
want.EndProgressCycle()  // Commits all staged changes atomically

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
  path: "yaml/recipes/etl-pipeline.yaml"
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
want.BeginProgressCycle()
want.StageStateChange(map[string]interface{}{
    "processed_count": count,
    "status": "active",
    "metrics": map[string]interface{}{
        "throughput": tps,
        "error_rate": errorRate,
    },
})
want.EndProgressCycle()  // Atomic commit
```

MyWant's declarative want system transforms complex distributed processing into simple, readable configuration that expresses **what you want to achieve** rather than **how to achieve it**.