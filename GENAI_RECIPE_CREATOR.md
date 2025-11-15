# GenAI Recipe Creator

## Overview

The GenAI Recipe Creator is a command-line tool that uses Google's Gemini AI to generate recipe definitions from natural language descriptions. It automatically reads the recipe format documentation and creates recipes that conform to the MyWant system specification.

**Features:**
- 🤖 Natural language to recipe conversion using Gemini AI
- 📖 Reads recipe format from `recipes/README.md` for context
- ✅ Validates recipes against MyWant specification
- 📤 Automatically registers recipes with the backend server
- 💬 Interactive mode for creating multiple recipes
- 🔧 Command-line mode for automation

## Prerequisites

### 1. Gemini API Key

Get a Gemini API key from [Google AI Studio](https://aistudio.google.com/):

```bash
export GEMINI_API_KEY="your-api-key-here"
```

### 2. Backend Server Running

Ensure the MyWant backend server is running:

```bash
make restart-all
```

### 3. Build GenAI Runner

Build the genai-runner application:

```bash
cd genai-runner
go mod tidy
go build -o genai-runner ./
```

## Usage

### Command-Line Mode

Generate a single recipe from a natural language description:

```bash
cd genai-runner
./genai-runner -request "Create a queue processing pipeline with 3 stages"
```

### Command-Line Options

```bash
./genai-runner -help

Flags:
  -interactive
        Interactive mode - prompts for user input
  -name string
        Optional: Override recipe name from request
  -readme string
        Path to recipes/README.md for context (default "../recipes/README.md")
  -request string
        Natural language description of the recipe to create
  -server string
        Backend server URL (default "http://localhost:8080")
```

### Examples

#### Example 1: Travel Planning Recipe

```bash
./genai-runner -request "Create a travel planning recipe that includes restaurant reservation, hotel booking, and buffet breakfast with a travel coordinator"
```

Expected output:
```
✅ Generated Recipe:
════════════════════════════════════════════════════════════════════════
recipe:
  metadata:
    name: travel-planning-recipe
    description: Travel planning with restaurant, hotel, and buffet coordination
    custom_type: travel-planning
    version: 1.0.0
  parameters:
    restaurant_type: fine-dining
    hotel_type: luxury
  wants:
    - metadata:
        type: restaurant
        labels:
          role: scheduler
      spec:
        params:
          restaurant_type: restaurant_type
    ...

📤 Registering recipe with backend server...
✅ Recipe successfully registered!
   Recipe ID: travel-planning-recipe
   Server: http://localhost:8080
   Endpoint: http://localhost:8080/api/v1/recipes/travel-planning-recipe
```

#### Example 2: Fibonacci Generator

```bash
./genai-runner -request "Create a fibonacci number generator that produces 10 fibonacci numbers"
```

#### Example 3: Custom Name

```bash
./genai-runner -request "Create a simple queue processing system" -name my-custom-queue
```

### Interactive Mode

For creating multiple recipes interactively:

```bash
./genai-runner -interactive
```

In interactive mode:
1. Describe the recipe you want
2. Review the generated recipe
3. Choose whether to register it
4. Repeat or exit

Example session:

```
🧠 GenAI Recipe Creator - Interactive Mode
════════════════════════════════════════════════════════════════════════

This tool creates recipe definitions from natural language descriptions.
Recipes are automatically registered with the MyWant backend server.

────────────────────────────────────────────────────────────────────────
📝 Describe the recipe you want to create (or 'quit' to exit):
> Create a fibonacci sequence generator

🤖 Generating recipe from your request...
🔄 Calling Gemini API...

✅ Generated Recipe:
────────────────────────────────────────────────────────────────────────
recipe:
  metadata:
    name: fibonacci-sequence-generator
    ...

📤 Register this recipe? (yes/no): yes
✅ Recipe registered! ID: fibonacci-sequence-generator

────────────────────────────────────────────────────────────────────────
📝 Describe the recipe you want to create (or 'quit' to exit):
> quit

👋 Goodbye!
```

## How It Works

### 1. Recipe Format Context

The tool reads `recipes/README.md` to understand:
- Recipe structure and format
- Want types available
- Parameter conventions
- Label patterns for connectivity
- Best practices and examples

### 2. Gemini Prompt Generation

Creates a detailed prompt that includes:
- Full recipe format documentation
- User's natural language request
- Instructions for JSON output
- Examples of valid recipes

### 3. Recipe Parsing

Extracts and validates JSON from Gemini response:
- Validates required fields (name, custom_type, version)
- Ensures proper structure
- Adds defaults if missing

### 4. Server Registration

Submits recipe to backend API:
- POST to `/api/v1/recipes`
- Returns HTTP 201 Created on success
- Provides recipe ID for future reference

## Recipe Format Generated

The tool generates recipes in this structure:

```json
{
  "recipe": {
    "metadata": {
      "name": "recipe-name",
      "description": "What the recipe does",
      "custom_type": "category",
      "version": "1.0.0"
    },
    "parameters": {
      "param1": "default-value",
      "param2": 100
    },
    "wants": [
      {
        "metadata": {
          "type": "want-type",
          "labels": {
            "role": "label-value"
          }
        },
        "spec": {
          "params": {
            "key": "value"
          }
        }
      }
    ]
  }
}
```

## Customization

### Using Custom Server URL

```bash
./genai-runner -request "Create a queue" -server http://example.com:8080
```

### Using Custom README Path

```bash
./genai-runner -request "Create a queue" -readme /path/to/recipes/README.md
```

### Overriding Recipe Name

If Gemini generates a name you don't like:

```bash
./genai-runner -request "Create a queue" -name my-queue
```

## Troubleshooting

### Error: "GEMINI_API_KEY environment variable not set"

Set your API key:

```bash
export GEMINI_API_KEY="your-key-here"
```

### Error: "failed to connect to server"

Ensure backend server is running:

```bash
make restart-all
```

### Error: "failed to parse recipe"

The Gemini response may not be valid JSON. Try:
- Simplifying your request
- Being more specific about want types
- Checking server logs

### Error: "server returned status 409"

A recipe with that name already exists. Use:

```bash
./genai-runner -request "..." -name unique-name
```

Or delete the existing recipe:

```bash
curl -X DELETE http://localhost:8080/api/v1/recipes/recipe-name
```

## Best Practices

### 1. Clear Descriptions

❌ **Bad**: "Create a recipe"
✅ **Good**: "Create a queue processing pipeline that generates data, processes it through a queue, and collects results"

### 2. Specific Want Types

❌ **Bad**: "Create something with processing stages"
✅ **Good**: "Create a recipe with sequence generator, queue processor, and sink collector connected via labels"

### 3. Include Parameters

❌ **Bad**: "Create a travel recipe"
✅ **Good**: "Create a travel recipe with restaurant, hotel, and buffet that takes parameters for restaurant_type, hotel_type, and buffet_type"

### 4. Define Connectivity

❌ **Bad**: "Create multiple stages"
✅ **Good**: "Create independent wants (restaurant, hotel, buffet) coordinated by a travel_coordinator"

## Examples by Use Case

### Queue Processing

```bash
./genai-runner -request "Create a queue processing pipeline with a sequence generator, queue processor with service time 0.1, and sink collector connected via labels" -name queue-pipeline
```

### Travel Planning

```bash
./genai-runner -request "Create a travel itinerary recipe with independent wants for restaurant reservation, hotel booking, and buffet breakfast, coordinated by a travel_coordinator that combines all schedules" -name travel-itinerary
```

### Fibonacci Generation

```bash
./genai-runner -request "Create a fibonacci number generator pipeline that generates fibonacci numbers sequentially and collects the results" -name fibonacci-gen
```

### Multi-Stage Processing

```bash
./genai-runner -request "Create a 3-stage data processing pipeline: data source, filter/transform, and result collector with dependencies between stages" -name data-pipeline
```

## Integration with Makefile

Add a make target to your Makefile:

```makefile
genai-create-recipe:
	@cd genai-runner && go build -o genai-runner ./
	@echo "🤖 GenAI Recipe Creator ready"
	@echo "Usage: cd genai-runner && ./genai-runner -request \"your recipe description\""
```

Then run:

```bash
make genai-create-recipe
cd genai-runner
./genai-runner -request "Create a queue"
```

## API Response

After successful registration, you receive:

```json
{
  "id": "recipe-name",
  "message": "Recipe created successfully"
}
```

The recipe is immediately available at:

```
http://localhost:8080/api/v1/recipes/recipe-name
```

## Advanced Usage

### Batch Recipe Creation

Create multiple recipes from a script:

```bash
#!/bin/bash
recipes=(
  "Create a queue processing pipeline"
  "Create a fibonacci generator"
  "Create a travel planning recipe"
)

for recipe in "${recipes[@]}"; do
  cd genai-runner
  ./genai-runner -request "$recipe"
  cd ..
  sleep 2  # Rate limit
done
```

### Conditional Registration

Generate without registering:

```bash
# Generate and review (but the tool auto-registers)
./genai-runner -request "Create a queue" -server http://localhost:9999
```

Then manually register if satisfied.

## Architecture

```
User Request (natural language)
         ↓
    [GenAI Runner]
         ↓
   Reads recipes/README.md
         ↓
   Calls Gemini API
         ↓
   Parses JSON response
         ↓
   Validates recipe
         ↓
   Registers with backend API
         ↓
   Returns recipe ID
```

## Environment Variables

| Variable | Required | Default | Purpose |
|----------|----------|---------|---------|
| `GEMINI_API_KEY` | Yes | - | Google Gemini API key |

## Performance

- **Recipe Generation**: 2-5 seconds (depends on Gemini API)
- **Recipe Registration**: <1 second (local network)
- **Total Time**: 3-6 seconds per recipe

## Limitations

1. **Model Accuracy**: Gemini may not always generate perfect recipes
   - Review generated recipes before using
   - Adjust prompt if results are unsatisfactory

2. **Want Types**: Limited to types defined in `recipes/README.md`
   - Gemini may suggest non-existent types
   - Review recipes for validity

3. **Complex Topologies**: Very complex dependencies may not be generated correctly
   - For complex recipes, consider manual creation
   - Use simpler descriptions

## Next Steps

1. **Create Custom Recipes**: Start with simple recipes first
2. **Review Generated Recipes**: Always review before production use
3. **Refine Prompts**: Learn what descriptions work best
4. **Build Library**: Create a library of common recipes
5. **Automate Workflows**: Integrate into your deployment pipeline

## Support

- Check `recipes/README.md` for recipe format details
- Review `RECIPE_API_TESTING_GUIDE.md` for API specifics
- Check server logs: `tail -f logs/mywant-backend.log`
- Test generated recipes with `make test-recipe-api`

---

**Version**: 1.0.0
**Last Updated**: November 11, 2025
**Status**: Ready for Use
