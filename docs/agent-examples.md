# MyWant Agent System Examples

This document provides practical examples and patterns for implementing agents in the MyWant system.

## Table of Contents

- [Basic Agent Patterns](#basic-agent-patterns)
- [Real-World Examples](#real-world-examples)
- [Advanced Patterns](#advanced-patterns)
- [Best Practices](#best-practices)
- [Common Use Cases](#common-use-cases)

## Basic Agent Patterns

### 1. Simple DoAgent Pattern

**Use Case**: Making external API calls to perform actions.

```yaml
# yaml/capabilities/capability-payment.yaml
capabilities:
  - name: payment_processing
    gives:
      - payment_charge
      - payment_refund
```

```yaml
# yaml/agents/agent-payment.yaml
agents:
  - name: stripe_payment_agent
    type: do
    capabilities:
      - payment_processing
    uses:
      - stripe_api
    config:
      api_timeout: 30
      retry_count: 3
```

```go
// Implementation
func (r *AgentRegistry) stripePaymentAction(ctx context.Context, want *Want) error {
    amount, _ := want.GetState("amount")
    currency, _ := want.GetState("currency")

    // Make Stripe API call
    chargeID, err := stripeAPI.CreateCharge(amount, currency)
    if err != nil {
        return fmt.Errorf("stripe charge failed: %w", err)
    }

    // Stage all results
    want.StageStateChange(map[string]interface{}{
        "charge_id": chargeID,
        "status": "charged",
        "processed_at": time.Now().Format(time.RFC3339),
        "gateway": "stripe",
    })

    want.CommitStateChanges()
    return nil
}
```

### 2. Monitoring Agent Pattern

**Use Case**: Continuous monitoring of external resources.

```yaml
# yaml/capabilities/capability-monitoring.yaml
capabilities:
  - name: system_monitoring
    gives:
      - health_check
      - performance_monitoring
```

```yaml
# yaml/agents/agent-monitor.yaml
agents:
  - name: health_monitor
    type: monitor
    capabilities:
      - system_monitoring
    config:
      check_interval: 60
      alert_threshold: 0.9
```

```go
// Implementation with periodic monitoring
func (r *AgentRegistry) healthMonitorAction(ctx context.Context, want *Want) error {
    ticker := time.NewTicker(60 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            // Check system health
            metrics := checkSystemHealth()

            want.StageStateChange(map[string]interface{}{
                "cpu_usage": metrics.CPU,
                "memory_usage": metrics.Memory,
                "disk_usage": metrics.Disk,
                "status": getHealthStatus(metrics),
                "last_check": time.Now().Format(time.RFC3339),
            })

            want.CommitStateChanges()

            // Exit if unhealthy
            if metrics.CPU > 0.9 {
                return fmt.Errorf("system critical: CPU usage %f", metrics.CPU)
            }
        }
    }
}
```

## Real-World Examples

### E-Commerce Order Processing

Complete example showing multiple agents working together:

```yaml
# yaml/capabilities/capability-ecommerce.yaml
capabilities:
  - name: inventory_management
    gives:
      - inventory_check
      - inventory_reserve
  - name: payment_processing
    gives:
      - payment_charge
  - name: shipping_management
    gives:
      - shipping_label
      - tracking_number
```

```yaml
# yaml/agents/agent-ecommerce.yaml
agents:
  - name: inventory_agent
    type: do
    capabilities:
      - inventory_management
    priority: 90  # High priority for inventory

  - name: payment_agent
    type: do
    capabilities:
      - payment_processing
    priority: 80

  - name: shipping_agent
    type: do
    capabilities:
      - shipping_management
    priority: 70

  - name: order_monitor
    type: monitor
    capabilities:
      - inventory_management
      - payment_processing
      - shipping_management
    priority: 50
```

```yaml
# yaml/config/config-order-processing.yaml
wants:
  - metadata:
      name: customer-order-12345
      type: order
      labels:
        customer_id: "cust_789"
        order_value: "high"
    spec:
      params:
        items:
          - sku: "LAPTOP001"
            quantity: 1
            price: 1299.99
        customer_email: "customer@example.com"
        shipping_address: "123 Main St, City, State"
      requires:
        - inventory_check
        - inventory_reserve
        - payment_charge
        - shipping_label
```

### Infrastructure Management

Example for managing cloud resources:

```yaml
# yaml/capabilities/capability-cloud.yaml
capabilities:
  - name: aws_ec2_management
    gives:
      - instance_launch
      - instance_terminate
      - instance_monitoring
  - name: database_management
    gives:
      - database_backup
      - database_restore
```

```yaml
# yaml/agents/agent-cloud.yaml
agents:
  - name: ec2_provisioner
    type: do
    capabilities:
      - aws_ec2_management
    config:
      aws_region: "us-west-2"
      instance_type: "t3.medium"

  - name: ec2_monitor
    type: monitor
    capabilities:
      - aws_ec2_management
    config:
      monitoring_interval: 300

  - name: rds_backup_agent
    type: do
    capabilities:
      - database_management
    config:
      backup_retention: 7
```

## Advanced Patterns

### 1. Multi-Stage Agent Pipeline

Agents that trigger other agents in sequence:

```go
func (r *AgentRegistry) pipelineInitiatorAction(ctx context.Context, want *Want) error {
    // Stage 1: Data preparation
    want.StageStateChange(map[string]interface{}{
        "stage": "data_prep",
        "status": "processing",
    })
    want.CommitStateChanges()

    // Prepare data
    processedData := prepareData()

    // Stage 2: Trigger next requirement
    want.StageStateChange(map[string]interface{}{
        "stage": "validation",
        "prepared_data": processedData,
        "next_requirement": "data_validation",
    })
    want.CommitStateChanges()

    return nil
}
```

### 2. Conditional Agent Execution

Agents that execute based on state conditions:

```go
func (r *AgentRegistry) conditionalAgent(ctx context.Context, want *Want) error {
    // Check condition from want state
    priority, exists := want.GetState("priority")
    if !exists {
        return fmt.Errorf("priority not set")
    }

    var action map[string]interface{}

    switch priority.(string) {
    case "high":
        action = map[string]interface{}{
            "processing_tier": "premium",
            "sla_hours": 4,
            "assigned_team": "senior",
        }
    case "normal":
        action = map[string]interface{}{
            "processing_tier": "standard",
            "sla_hours": 24,
            "assigned_team": "general",
        }
    default:
        action = map[string]interface{}{
            "processing_tier": "basic",
            "sla_hours": 72,
            "assigned_team": "junior",
        }
    }

    want.StageStateChange(action)
    want.CommitStateChanges()

    return nil
}
```

### 3. Error Recovery Agents

Agents specifically for handling failures:

```go
func (r *AgentRegistry) errorRecoveryAgent(ctx context.Context, want *Want) error {
    // Check for error state
    if errorMsg, exists := want.GetState("error"); exists {
        retryCount, _ := want.GetState("retry_count")
        count := 0
        if retryCount != nil {
            count = retryCount.(int)
        }

        if count < 3 {
            // Attempt recovery
            want.StageStateChange(map[string]interface{}{
                "error": nil,
                "status": "retrying",
                "retry_count": count + 1,
                "retry_at": time.Now().Format(time.RFC3339),
            })
        } else {
            // Mark as failed
            want.StageStateChange(map[string]interface{}{
                "status": "failed",
                "final_error": errorMsg,
                "failed_at": time.Now().Format(time.RFC3339),
            })
        }

        want.CommitStateChanges()
    }

    return nil
}
```

## Best Practices

### 1. Agent Naming Conventions

```yaml
# Good naming patterns
agents:
  - name: stripe_payment_processor    # Service_Function_Type
  - name: aws_ec2_monitor            # Provider_Service_Type
  - name: order_validation_agent     # Domain_Function_Agent
  - name: email_notification_sender  # Channel_Function_Action
```

### 2. State Management Patterns

```go
// ✅ Good: Atomic updates with related data
want.StageStateChange(map[string]interface{}{
    "order_id": orderID,
    "status": "confirmed",
    "confirmed_at": time.Now(),
    "confirmation_email": email,
})
want.CommitStateChanges()

// ❌ Avoid: Multiple separate commits
want.StageStateChange("order_id", orderID)
want.CommitStateChanges()
want.StageStateChange("status", "confirmed")
want.CommitStateChanges()
```

### 3. Error Handling Patterns

```go
func (r *AgentRegistry) robustAgent(ctx context.Context, want *Want) error {
    // Set processing state
    want.StageStateChange("status", "processing")
    want.CommitStateChanges()

    defer func() {
        if r := recover(); r != nil {
            // Handle panic
            want.StageStateChange(map[string]interface{}{
                "status": "error",
                "error": fmt.Sprintf("panic: %v", r),
                "error_at": time.Now(),
            })
            want.CommitStateChanges()
        }
    }()

    // Main logic with timeout
    select {
    case <-ctx.Done():
        want.StageStateChange("status", "cancelled")
        want.CommitStateChanges()
        return ctx.Err()
    case result := <-doWork():
        want.StageStateChange(map[string]interface{}{
            "status": "completed",
            "result": result,
            "completed_at": time.Now(),
        })
        want.CommitStateChanges()
        return nil
    }
}
```

### 4. Configuration Management

```yaml
# ✅ Good: Comprehensive configuration
agents:
  - name: api_client_agent
    type: do
    capabilities:
      - external_api
    config:
      base_url: "https://api.example.com"
      timeout: 30
      retry_count: 3
      retry_backoff: "exponential"
      rate_limit: 100
    tags: ["external", "api", "production"]
    version: "2.1.0"
    enabled: true
    priority: 70

# ❌ Avoid: Minimal configuration
agents:
  - name: api_agent
    type: do
    capabilities:
      - external_api
```

## Common Use Cases

### 1. API Integration Agent

```go
func (r *AgentRegistry) apiIntegrationAgent(ctx context.Context, want *Want) error {
    endpoint := want.Spec.Params["api_endpoint"].(string)
    method := want.Spec.Params["method"].(string)
    payload := want.Spec.Params["payload"]

    client := &http.Client{Timeout: 30 * time.Second}

    var req *http.Request
    var err error

    if method == "POST" || method == "PUT" {
        jsonPayload, _ := json.Marshal(payload)
        req, err = http.NewRequestWithContext(ctx, method, endpoint, bytes.NewBuffer(jsonPayload))
        req.Header.Set("Content-Type", "application/json")
    } else {
        req, err = http.NewRequestWithContext(ctx, method, endpoint, nil)
    }

    if err != nil {
        return fmt.Errorf("request creation failed: %w", err)
    }

    resp, err := client.Do(req)
    if err != nil {
        return fmt.Errorf("api call failed: %w", err)
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return fmt.Errorf("response read failed: %w", err)
    }

    want.StageStateChange(map[string]interface{}{
        "api_response": string(body),
        "status_code": resp.StatusCode,
        "headers": resp.Header,
        "called_at": time.Now().Format(time.RFC3339),
    })
    want.CommitStateChanges()

    return nil
}
```

### 2. Database Operation Agent

```go
func (r *AgentRegistry) databaseAgent(ctx context.Context, want *Want) error {
    query := want.Spec.Params["query"].(string)
    params := want.Spec.Params["params"].([]interface{})

    db := getDBConnection()

    rows, err := db.QueryContext(ctx, query, params...)
    if err != nil {
        return fmt.Errorf("query failed: %w", err)
    }
    defer rows.Close()

    var results []map[string]interface{}
    columns, _ := rows.Columns()

    for rows.Next() {
        values := make([]interface{}, len(columns))
        valuePtrs := make([]interface{}, len(columns))
        for i := range values {
            valuePtrs[i] = &values[i]
        }

        rows.Scan(valuePtrs...)

        row := make(map[string]interface{})
        for i, col := range columns {
            row[col] = values[i]
        }
        results = append(results, row)
    }

    want.StageStateChange(map[string]interface{}{
        "query_results": results,
        "row_count": len(results),
        "executed_at": time.Now().Format(time.RFC3339),
    })
    want.CommitStateChanges()

    return nil
}
```

### 3. File Processing Agent

```go
func (r *AgentRegistry) fileProcessingAgent(ctx context.Context, want *Want) error {
    filePath := want.Spec.Params["file_path"].(string)
    operation := want.Spec.Params["operation"].(string)

    file, err := os.Open(filePath)
    if err != nil {
        return fmt.Errorf("file open failed: %w", err)
    }
    defer file.Close()

    var result map[string]interface{}

    switch operation {
    case "word_count":
        scanner := bufio.NewScanner(file)
        wordCount := 0
        lineCount := 0

        for scanner.Scan() {
            lineCount++
            words := strings.Fields(scanner.Text())
            wordCount += len(words)
        }

        result = map[string]interface{}{
            "word_count": wordCount,
            "line_count": lineCount,
            "operation": "word_count",
        }

    case "hash":
        hasher := sha256.New()
        if _, err := io.Copy(hasher, file); err != nil {
            return fmt.Errorf("hashing failed: %w", err)
        }

        result = map[string]interface{}{
            "sha256": fmt.Sprintf("%x", hasher.Sum(nil)),
            "operation": "hash",
        }
    }

    result["file_path"] = filePath
    result["processed_at"] = time.Now().Format(time.RFC3339)

    want.StageStateChange(result)
    want.CommitStateChanges()

    return nil
}
```

## Testing Agents

### Unit Testing Pattern

```go
func TestHotelReservationAgent(t *testing.T) {
    // Create test want
    want := &Want{
        Metadata: Metadata{Name: "test-hotel"},
        Spec: WantSpec{
            Params: map[string]interface{}{
                "hotel_type": "luxury",
                "check_in": "2025-01-01",
            },
        },
        State: make(map[string]interface{}),
    }

    // Create registry and agent
    registry := NewAgentRegistry()
    ctx := context.Background()

    // Execute agent
    err := registry.hotelReservationAction(ctx, want)

    // Assert results
    assert.NoError(t, err)
    assert.Equal(t, "HTL-12345", want.GetState("reservation_id"))
    assert.Equal(t, "confirmed", want.GetState("status"))
}
```

This examples guide provides practical patterns and real-world implementations for building robust agent systems in MyWant.