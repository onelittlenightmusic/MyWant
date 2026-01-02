# Recipe Format Guide

This directory contains recipe YAML files that define reusable component templates for the MyWant functional chain programming system.

## What is a Recipe?

A recipe is a configuration template that defines:
- **Parameters**: Input variables that can be customized when using the recipe
- **Wants**: Processing units that execute in a chain
- **Coordinator**: Optional orchestrating want that combines independent wants
- **Result**: Optional specification for how to compute results
- **Example**: Optional one-click deployment configuration for frontend UI integration

## Recipe Structure

### Complete Example

```yaml
recipe:
  # Metadata - identifies and documents the recipe
  metadata:
    name: "Recipe Name"
    description: "Brief description of what this recipe does"
    custom_type: "type-identifier"
    version: "1.0.0"

  # Parameters - customizable values used in wants
  parameters:
    count: 100
    rate: 10.5
    display_name: "Default Display Name"

  # Wants - processing units in the chain
  wants:
    # Independent want (no dependencies)
    - metadata:
        type: "queue"
        labels:
          role: "processor"
      spec:
        params:
          service_time: 0.1
        # using connects to other wants via labels
        using:
          - role: "source"

  # Optional coordinator want for independent wants
  coordinator:
    type: "travel_coordinator"
    params:
      display_name: display_name
    using:
      - role: "scheduler"

  # Optional result specification
  result:
    - want_name: "queue"
      stat_name: ".total_processed"
      description: "Total items processed"

  # Optional example deployment configuration for one-click recipe deployment
  example:
    wants:
      - metadata:
          name: "queue-system-demo"
          type: "owner"
          labels:
            recipe: "Queue System"
        spec:
          params:
            count: 1000
            rate: 10.0
            service_time: 0.1
```

## Key Components

### Metadata Section

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Unique recipe identifier |
| `description` | No | Human-readable description |
| `custom_type` | Yes | Custom type classification for the recipe |
| `version` | No | Recipe version (e.g., "1.0.0") |

### Parameters Section

Define reusable parameters that can be referenced in want specs:

```yaml
parameters:
  count: 1000           # Number parameter
  rate: 10.5            # Float parameter
  display_name: "Name"  # String parameter
```

Reference parameters using their name:
```yaml
wants:
  - spec:
      params:
        item_count: count     # References the "count" parameter
        item_rate: rate       # References the "rate" parameter
```

### Wants Section

Array of want definitions with three components:

#### 1. Metadata
```yaml
metadata:
  type: "want-type"     # e.g., "queue", "sink", "source"
  labels:               # Labels for connectivity
    role: "processor"
    category: "queue-processor"
```

#### 2. Spec
```yaml
spec:
  params:               # Want-specific parameters
    service_time: 0.1
  using:               # Connectivity selectors
    - role: "source"   # Connect to want with role="source"
    - category: "producer"
```

#### 3. Using (Optional)
Specifies dependencies on other wants via label matching:

```yaml
using:
  - role: "processor"        # Connect to want with this label
  - category: "source"       # Multiple conditions work
```

### Coordinator (Optional)

Orchestrates independent wants that don't have `using` selectors:

```yaml
coordinator:
  type: "travel_coordinator"
  params:
    display_name: "My Coordinator"
  using:
    - role: "scheduler"  # Connect to all scheduler wants
```

### Result (Optional)

Specifies how to compute and display results:

```yaml
result:
  - want_name: "queue"
    stat_name: ".total_processed"
    description: "Queue statistics"
```

### Example (Optional)

Provides one-click deployment configuration for the frontend UI. Enables instant recipe deployment with pre-configured parameters:

```yaml
example:
  wants:
    - metadata:
        name: "recipe-demo"           # Unique name for this example deployment
        type: "owner"                 # Type must be "owner" to load the recipe
        labels:
          recipe: "Recipe Display Name"  # Matches recipe metadata.name or custom_type
      spec:
        params:                       # Pre-configured parameter values
          param1: "value1"
          param2: "value2"
```

**How it works:**
- The `example.wants` array contains one or more want configurations
- Each want typically has `type: "owner"` and references the recipe via labels
- The `params` section contains pre-configured values matching recipe parameters
- Frontend can POST `example.wants` directly to `/api/v1/wants` for one-click deployment
- Users can modify parameters before deployment or use defaults as-is

## Recipe Types

### 1. Independent Wants (Travel Planning)
Wants without `using` selectors execute in parallel. A coordinator combines them:

```yaml
wants:
  - metadata:
      type: restaurant
      labels:
        role: scheduler
    # No using - independent
  - metadata:
      type: hotel
      labels:
        role: scheduler

coordinator:
  type: travel_coordinator
  using:
    - role: scheduler
```

### 2. Dependent Wants (Pipeline)
Wants with `using` selectors form processing pipelines:

```yaml
wants:
  # Source
  - metadata:
      type: sequence
      labels:
        role: source

  # Processor depends on source
  - metadata:
      type: queue
      labels:
        role: processor
    spec:
      using:
        - role: source

  # Sink depends on processor
  - metadata:
      type: sink
      labels:
        role: collector
    spec:
      using:
        - role: processor
```

## Available Recipe Files

All recipes include an `example` field for one-click frontend deployment:

| File | Type | Description | Example Deploy |
|------|------|-------------|-----------------|
| `queue-system.yaml` | Dependent | Queue processing pipeline | ✓ |
| `fibonacci-sequence.yaml` | Dependent | Fibonacci number generation | ✓ |
| `fibonacci-pipeline.yaml` | Dependent | Fibonacci with pipeline pattern | ✓ |
| `prime-sieve.yaml` | Dependent | Prime number generation | ✓ |
| `qnet-pipeline.yaml` | Dependent | QNet queue network | ✓ |
| `travel-agent.yaml` | Independent | Full agent-enabled travel system | ✓ |
| `dynamic-travel-change.yaml` | Dynamic | Travel with real-time changes | ✓ |
| `approval-level-1.yaml` | Approval | Multi-level approval workflows | ✓ |
| `approval-level-2.yaml` | Approval | Advanced approval workflows | ✓ |

## Using Recipes

### One-Click Deployment

All recipes include pre-configured examples for instant deployment from the frontend UI:

1. **View Recipe Details**: Frontend displays the recipe with a "Deploy" button
2. **Click Deploy**: Frontend sends `recipe.example.wants` to `/api/v1/wants`
3. **Instant Execution**: Recipe deploys immediately with pre-configured parameters
4. **Customize (Optional)**: Users can modify parameters before deployment

Example deployment request:
```bash
curl -X POST http://localhost:8080/api/v1/wants \
  -H "Content-Type: application/json" \
  -d '{
    "yaml": "wants:\n  - metadata:\n      name: \"travel-itinerary-demo\"\n      type: \"owner\"\n      labels:\n        recipe: \"Travel Itinerary\"\n    spec:\n      params:\n        restaurant_type: \"fine dining\"\n        hotel_type: \"luxury\""
  }'
```

### Via Config File

Reference a recipe in your config file:

```yaml
wants:
  - metadata:
      type: "owner"
      labels:
        recipe: "Travel Itinerary"
    spec:
      params:
        prefix: "travel"
        display_name: "One Day Trip"
```

### Via API

Create a recipe using the REST API:

```bash
curl -X POST http://localhost:8080/api/v1/recipes \
  -H "Content-Type: application/json" \
  -d @my-recipe.json
```

See [RECIPE_API_TESTING_GUIDE.md](../RECIPE_API_TESTING_GUIDE.md) for complete API documentation.

## Best Practices

### 1. Use Descriptive Names
```yaml
metadata:
  name: "travel-itinerary-v2"  # ✅ Good
  # name: "recipe1"            # ❌ Avoid
```

### 2. Use Semantic Versioning
```yaml
metadata:
  version: "2.1.0"  # Major.Minor.Patch
```

### 3. Label Conventions
- `role`: Primary function (scheduler, processor, collector)
- `category`: Classification (queue-producer, approver)
- `stage`: Pipeline stage (input, processing, output)

```yaml
labels:
  role: "processor"        # What does it do?
  category: "queue-processor"
  stage: "processing"
```

### 4. Parameter Naming
Use clear names for parameters:

```yaml
parameters:
  queue_service_time: 0.1   # ✅ Clear
  rate_limit: 100            # ✅ Clear
  # s: 0.1                    # ❌ Cryptic
```

### 5. Organize Dependencies
Define wants in logical order:

```yaml
wants:
  # Independent wants first
  - metadata:
      type: "restaurant"
      labels:
        role: "scheduler"

  # Dependent wants after
  - metadata:
      type: "coordinator"
      using:
        - role: "scheduler"
```

## OpenAPI Specification

The Recipe API follows the OpenAPI specification. See the spec directory for:
- Complete API endpoint definitions
- Request/response schemas
- Error codes and descriptions

Location: `spec/openapi.yaml` (if present)

## Testing Recipes

Run the recipe API test suite:

```bash
make test-recipe-api
```

This tests:
- Recipe creation via API
- Recipe loading from YAML files
- Recipe with wants and parameters
- Recipe validation

## Creating a New Recipe

### Step 1: Plan Your Recipe

Define:
- What processing units (wants) do you need?
- Are they independent or dependent?
- What parameters should be configurable?

### Step 2: Create the File

Create a new YAML file in this directory:

```bash
recipes/my-new-recipe.yaml
```

### Step 3: Define Structure

```yaml
recipe:
  metadata:
    name: "my-new-recipe"
    description: "Description of what it does"
    custom_type: "my-type"
    version: "1.0.0"

  parameters:
    param1: value1

  wants:
    # Define your wants here

  # Add example for one-click deployment
  example:
    wants:
      - metadata:
          name: "my-new-recipe-demo"
          type: "owner"
          labels:
            recipe: "my-new-recipe"
        spec:
          params:
            param1: value1
```

### Step 4: Test

```bash
# Verify the recipe loads
make run-server
curl http://localhost:8080/api/v1/recipes/my-new-recipe
```

## Common Recipe Patterns

### Producer → Consumer

```yaml
wants:
  - metadata:
      type: "sequence"
      labels:
        role: "producer"

  - metadata:
      type: "processor"
      spec:
        using:
          - role: "producer"
```

### Multiple Independent Coordinators

```yaml
wants:
  - metadata:
      type: "restaurant"
      labels:
        role: "service"

  - metadata:
      type: "hotel"
      labels:
        role: "service"

  - metadata:
      type: "coordinator"
      spec:
        using:
          - role: "service"
```

### Pipeline with Filtering

```yaml
wants:
  - metadata:
      type: "source"
      labels:
        role: "producer"

  - metadata:
      type: "filter"
      labels:
        role: "processor"
      spec:
        using:
          - role: "producer"

  - metadata:
      type: "sink"
      labels:
        role: "consumer"
      spec:
        using:
          - role: "processor"
```

## Troubleshooting

### Recipe Not Loading

Check:
1. YAML syntax is valid
2. `metadata.name` is unique
3. `metadata.custom_type` is set
4. File is in `recipes/` directory

### Wants Not Connecting

Verify:
1. `using` labels match want `labels`
2. Label case matches exactly
3. Want with matching label exists

### Parameters Not Substituting

Ensure:
1. Parameter is defined in `parameters` section
2. Reference uses exact parameter name
3. Parameter value type matches usage

## References

- [CLAUDE.md](../CLAUDE.md) - Architecture overview
- [RECIPE_API_TESTING_GUIDE.md](../RECIPE_API_TESTING_GUIDE.md) - API documentation
- OpenAPI spec - Complete API definitions

## Questions?

Refer to existing recipes in this directory for examples of common patterns and configurations.
