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
    amount, _ := want.GetGoal("amount")
    currency, _ := want.GetGoal("currency")

    // Make Stripe API call
    chargeID, err := stripeAPI.CreateCharge(amount, currency)
    if err != nil {
        return fmt.Errorf("stripe charge failed: %w", err)
    }

    // Set results in Current state
    want.SetCurrent("charge_id", chargeID)
    want.SetCurrent("status", "charged")
    want.SetCurrent("processed_at", time.Now().Format(time.RFC3339))
    want.SetCurrent("gateway", "stripe")

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

            // Update current state
            want.SetCurrent("cpu_usage", metrics.CPU)
            want.SetCurrent("memory_usage", metrics.Memory)
            want.SetCurrent("disk_usage", metrics.Disk)
            want.SetCurrent("status", getHealthStatus(metrics))
            want.SetCurrent("last_check", time.Now().Format(time.RFC3339))

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
    want.SetCurrent("stage", "data_prep")
    want.SetCurrent("status", "processing")

    // Prepare data
    processedData := prepareData()

    // Stage 2: Trigger next requirement
    want.SetCurrent("stage", "validation")
    want.SetCurrent("prepared_data", processedData)
    
    // Set a plan for the next agent
    want.SetPlan("validate_data", true)

    return nil
}
```

### 2. Conditional Agent Execution

Agents that execute based on state conditions:

```go
func (r *AgentRegistry) conditionalAgent(ctx context.Context, want *Want) error {
    // Check condition from want goals
    priority, exists := want.GetGoal("priority")
    if !exists {
        return fmt.Errorf("priority not set")
    }

    switch priority.(string) {
    case "high":
        want.SetCurrent("processing_tier", "premium")
        want.SetCurrent("sla_hours", 4)
        want.SetCurrent("assigned_team", "senior")
    case "normal":
        want.SetCurrent("processing_tier", "standard")
        want.SetCurrent("sla_hours", 24)
        want.SetCurrent("assigned_team", "general")
    default:
        want.SetCurrent("processing_tier", "basic")
        want.SetCurrent("sla_hours", 72)
        want.SetCurrent("assigned_team", "junior")
    }

    return nil
}
```

### 3. Error Recovery Agents

Agents specifically for handling failures:

```go
func (r *AgentRegistry) errorRecoveryAgent(ctx context.Context, want *Want) error {
    // Check for error state in Current
    if errorMsg, exists := want.GetCurrent("error"); exists {
        retryCount, _ := want.SetInternal("retry_count") // Internal state for retries
        count := 0
        if retryCount != nil {
            count = retryCount.(int)
        }

        if count < 3 {
            // Attempt recovery
            want.SetCurrent("error", nil)
            want.SetCurrent("status", "retrying")
            want.SetInternal("retry_count", count + 1)
            want.SetCurrent("retry_at", time.Now().Format(time.RFC3339))
        } else {
            // Mark as failed
            want.SetCurrent("status", "failed")
            want.SetCurrent("final_error", errorMsg)
            want.SetCurrent("failed_at", time.Now().Format(time.RFC3339))
        }
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
// ✅ Good: Atomic property setters
want.SetCurrent("order_id", orderID)
want.SetCurrent("status", "confirmed")
want.SetCurrent("confirmed_at", time.Now())
want.SetCurrent("confirmation_email", email)

// ❌ Avoid: Batching with legacy methods
// want.StageStateChange(...)
// want.CommitStateChanges()
```

### 3. Error Handling Patterns

```go
func (r *AgentRegistry) robustAgent(ctx context.Context, want *Want) error {
    // Set processing state
    want.SetCurrent("status", "processing")

    defer func() {
        if r := recover(); r != nil {
            // Handle panic
            want.SetCurrent("status", "error")
            want.SetCurrent("error", fmt.Sprintf("panic: %v", r))
            want.SetCurrent("error_at", time.Now())
        }
    }()

    // Main logic with timeout
    select {
    case <-ctx.Done():
        want.SetCurrent("status", "cancelled")
        return ctx.Err()
    case result := <-doWork():
        want.SetCurrent("status", "completed")
        want.SetCurrent("result", result)
        want.SetCurrent("completed_at", time.Now())
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
    endpoint, _ := want.GetGoal("api_endpoint")
    method, _ := want.GetGoal("method")
    payload, _ := want.GetGoal("payload")

    client := &http.Client{Timeout: 30 * time.Second}

    var req *http.Request
    var err error

    if method.(string) == "POST" || method.(string) == "PUT" {
        jsonPayload, _ := json.Marshal(payload)
        req, err = http.NewRequestWithContext(ctx, method.(string), endpoint.(string), bytes.NewBuffer(jsonPayload))
        req.Header.Set("Content-Type", "application/json")
    } else {
        req, err = http.NewRequestWithContext(ctx, method.(string), endpoint.(string), nil)
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

    want.SetCurrent("api_response", string(body))
    want.SetCurrent("status_code", resp.StatusCode)
    want.SetCurrent("headers", resp.Header)
    want.SetCurrent("called_at", time.Now().Format(time.RFC3339))

    return nil
}
```

### 2. Database Operation Agent

```go
func (r *AgentRegistry) databaseAgent(ctx context.Context, want *Want) error {
    query, _ := want.GetGoal("query")
    params, _ := want.GetGoal("params")

    db := getDBConnection()

    rows, err := db.QueryContext(ctx, query.(string), params.([]interface{})...)
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

    want.SetCurrent("query_results", results)
    want.SetCurrent("row_count", len(results))
    want.SetCurrent("executed_at", time.Now().Format(time.RFC3339))

    return nil
}
```

### 3. File Processing Agent

```go
func (r *AgentRegistry) fileProcessingAgent(ctx context.Context, want *Want) error {
    filePath, _ := want.GetGoal("file_path")
    operation, _ := want.GetGoal("operation")

    file, err := os.Open(filePath.(string))
    if err != nil {
        return fmt.Errorf("file open failed: %w", err)
    }
    defer file.Close()

    var result map[string]interface{}

    switch operation.(string) {
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

    want.SetCurrent("file_path", filePath)
    want.SetCurrent("processed_at", time.Now().Format(time.RFC3339))
    
    for k, v := range result {
        want.SetCurrent(k, v)
    }

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
        StateLabels: map[string]StateLabel{
            "hotel_type": LabelGoal,
            "check_in":   LabelGoal,
            "reservation_id": LabelCurrent,
            "status":         LabelCurrent,
        },
        State: make(map[string]interface{}),
    }
    
    // Set input goals
    want.SetGoal("hotel_type", "luxury")
    want.SetGoal("check_in", "2025-01-01")

    // Create registry and agent
    registry := NewAgentRegistry()
    ctx := context.Background()

    // Execute agent
    err := registry.hotelReservationAction(ctx, want)

    // Assert results from Current state
    assert.NoError(t, err)
    resID, _ := want.GetCurrent("reservation_id")
    status, _ := want.GetCurrent("status")
    assert.Equal(t, "HTL-12345", resID)
    assert.Equal(t, "confirmed", status)
}
```

This examples guide provides practical patterns and real-world implementations for building robust agent systems in MyWant.