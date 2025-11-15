# Recipe API Testing Guide

## Overview

The MyWant backend server provides a complete REST API for recipe management. This guide covers how to use the Recipe API and run the comprehensive test suite.

## Recipe API Endpoints

### Base URL
```
http://localhost:8080/api/v1/recipes
```

### Endpoints

#### 1. Create Recipe
**POST** `/api/v1/recipes`

Creates a new recipe in the system.

**Request:**
```bash
curl -X POST http://localhost:8080/api/v1/recipes \
  -H "Content-Type: application/json" \
  -d '{
    "recipe": {
      "metadata": {
        "name": "my-recipe",
        "description": "My recipe description",
        "custom_type": "custom-type",
        "version": "1.0.0"
      },
      "parameters": {
        "param1": "value1",
        "param2": 100
      },
      "wants": [
        {
          "metadata": {
            "type": "queue",
            "labels": {
              "role": "processor"
            }
          },
          "spec": {
            "params": {
              "service_time": 0.1
            }
          }
        }
      ]
    }
  }'
```

**Response (201 Created):**
```json
{
  "id": "my-recipe",
  "message": "Recipe created successfully"
}
```

#### 2. List Recipes
**GET** `/api/v1/recipes`

Lists all recipes in the system.

**Request:**
```bash
curl http://localhost:8080/api/v1/recipes
```

**Response (200 OK):**
```json
{
  "recipes": [
    {
      "id": "travel-itinerary",
      "recipe": { ... },
      "created_at": "2025-11-11T10:30:00Z"
    },
    ...
  ]
}
```

#### 3. Get Specific Recipe
**GET** `/api/v1/recipes/{id}`

Gets a specific recipe by ID.

**Request:**
```bash
curl http://localhost:8080/api/v1/recipes/travel-itinerary
```

**Response (200 OK):**
```json
{
  "id": "travel-itinerary",
  "recipe": {
    "metadata": { ... },
    "parameters": { ... },
    "wants": [ ... ]
  }
}
```

#### 4. Update Recipe
**PUT** `/api/v1/recipes/{id}`

Updates an existing recipe.

**Request:**
```bash
curl -X PUT http://localhost:8080/api/v1/recipes/my-recipe \
  -H "Content-Type: application/json" \
  -d '{
    "recipe": {
      "metadata": {
        "name": "my-recipe",
        "description": "Updated description",
        "custom_type": "custom-type",
        "version": "2.0.0"
      },
      "parameters": { ... },
      "wants": [ ... ]
    }
  }'
```

**Response (200 OK):**
```json
{
  "id": "my-recipe",
  "message": "Recipe updated successfully"
}
```

#### 5. Delete Recipe
**DELETE** `/api/v1/recipes/{id}`

Deletes a recipe from the system.

**Request:**
```bash
curl -X DELETE http://localhost:8080/api/v1/recipes/my-recipe
```

**Response (204 No Content):**
```
(empty response)
```

## Test Suite

### Prerequisites

Before running tests, ensure:
1. **Server is running** on `http://localhost:8080`
   ```bash
   make run-server
   ```

2. **Recipes directory exists** at `recipes/`
   - The test verifies YAML recipe files are available

### Running Tests

#### Quick Start
```bash
make test-recipe-api
```

#### What the Tests Cover

The test suite runs 6 comprehensive tests:

1. **Create Recipe** - Tests creating a new recipe via API
   - Validates request/response format
   - Checks HTTP 201 (Created) status
   - Verifies recipe is registered

2. **List Recipes** - Tests listing all recipes
   - Validates JSON response format
   - Counts available recipes
   - Checks HTTP 200 (OK) status

3. **Get Recipe** - Tests retrieving a specific recipe
   - Gets recipe ID from list
   - Retrieves specific recipe
   - Validates response structure

4. **Load Recipe from YAML** - Tests YAML recipe files
   - Verifies `recipes/travel-itinerary.yaml` exists
   - Confirms recipes are loadable from files

5. **Update Recipe** - Tests updating an existing recipe
   - Creates test recipe
   - Updates its metadata
   - Validates HTTP 200 (OK) status

6. **Delete Recipe** - Tests deleting a recipe
   - Creates test recipe
   - Deletes it via API
   - Validates HTTP 204 (No Content) status

### Test Output

The test suite produces a summary report:

```
========================================================================
TEST SUMMARY
========================================================================

Total Tests: 6
✅ Passed: 6
❌ Failed: 0
⏱️  Total Duration: 0.45s

🎉 All tests passed!
========================================================================
```

## Recipe Format Reference

### Recipe Structure

```yaml
recipe:
  # Recipe metadata
  metadata:
    name: "Recipe Name"
    description: "Recipe description"
    custom_type: "type-name"
    version: "1.0.0"

  # Parameters available for substitution
  parameters:
    param1: value1
    param2: 100
    param3: 10.5

  # Array of wants to create
  wants:
    - metadata:
        type: "want-type"
        labels:
          role: "label-value"
      spec:
        params:
          param_name: value
        using:
          - role: "dependency-label"

  # Optional coordinator want
  coordinator:
    type: "coordinator-type"
    params:
      display_name: "Coordinator Name"
    using:
      - role: "scheduler"

  # Optional result specification
  result:
    - want_name: "want-name"
      stat_name: ".stat-path"
      description: "Result description"
```

### Parameter Substitution

Parameters are referenced by name in want specs:

```yaml
parameters:
  count: 100
  rate: 10.5

wants:
  - spec:
      params:
        item_count: count      # References "count" parameter
        item_rate: rate        # References "rate" parameter
```

## Example Recipes

### Travel Itinerary Recipe
Located at: `recipes/travel-itinerary.yaml`

Creates independent wants for travel planning:
- Restaurant reservation
- Hotel booking
- Buffet reservation
- Travel coordinator to combine schedules

### Queue System Recipe
Located at: `recipes/queue-system.yaml`

Creates dependent wants for queue processing:
- Sequence generator (source)
- Queue processor
- Sink collector

## Error Handling

### Common HTTP Status Codes

| Code | Meaning | Example |
|------|---------|---------|
| 200 | OK | Recipe retrieved/updated successfully |
| 201 | Created | Recipe created successfully |
| 204 | No Content | Recipe deleted successfully |
| 400 | Bad Request | Invalid recipe format or missing name |
| 404 | Not Found | Recipe doesn't exist |
| 409 | Conflict | Recipe name already exists |

### Example Error Response

```json
{
  "error": "Recipe name is required"
}
```

## Integration with Wants API

Recipes can be used to create wants by referencing them in configurations:

```yaml
# config/config-travel-recipe.yaml
wants:
  - metadata:
      type: "owner"
      labels:
        recipe: "Travel Itinerary"
    spec:
      params:
        prefix: "travel"
        display_name: "One Day Travel"
```

## Advanced Usage

### Using Environment Variables

The server connects to external services via environment variables:

```bash
# Set Flight API server URL
export FLIGHT_API_URL=http://localhost:8081

# Set LLM server URL
export GPT_BASE_URL=http://localhost:11434

make run-server
```

### Recipe Versioning

Keep track of recipe versions in metadata:

```json
{
  "metadata": {
    "name": "my-recipe",
    "version": "2.0.0"  // Increment when making changes
  }
}
```

## Troubleshooting

### Server Not Responding

```bash
# Check if server is running
curl http://localhost:8080/health

# Start server if needed
make run-server
```

### Recipe Creation Fails

Verify recipe structure:
- `metadata.name` is required and must be unique
- `parameters` should be a map of string to any value
- `wants` should be an array of want objects

### Test Timeout

If tests hang, check:
1. Server is responding to health check
2. Network connectivity to localhost:8080
3. Server logs for errors: `tail -f logs/mywant-backend.log`

## Running Multiple Tests

Run all API tests in sequence:

```bash
make test-recipe-api
make test-concurrent-deploy
make test-llm-api
```

Or run all tests at once:

```bash
make test-all-runs
```

## API Rate Limits

The recipe API has no built-in rate limiting. For production, consider:
- Adding rate limiting middleware
- Implementing request throttling
- Using API gateways

## Security Considerations

1. **Recipe Validation** - Recipes are validated before creation
2. **Name Uniqueness** - Recipe names must be unique (409 Conflict if duplicate)
3. **Input Sanitization** - JSON/YAML parsing prevents injection attacks
4. **CORS Support** - API allows cross-origin requests

## Next Steps

1. Create custom recipes for your use cases
2. Integrate recipe creation into your workflow
3. Monitor recipe performance in production
4. Track recipe versions and changes

## Support

For issues or questions:
1. Check server logs: `logs/mywant-backend.log`
2. Review CLAUDE.md for architecture details
3. Examine existing recipes in `recipes/` directory
4. Run diagnostic: `make test-recipe-api`

