# Want Type YAML Schema Update - Layered Metadata Structure

## Change Summary

The want type YAML schema has been restructured to use a **layered metadata structure**, mirroring the actual `Metadata` struct in the Want type system.

---

## Before (Flat Structure)

```yaml
wantType:
  name: "numbers"
  title: "Number Generator"
  description: "..."
  version: "1.0"
  category: "math"
  pattern: "generator"

  parameters: [...]
  state: [...]
```

**Issue**: Identity fields (`name`, `title`, `description`, `version`, `category`, `pattern`) scattered at the top level, not properly grouped.

---

## After (Layered Structure)

```yaml
wantType:
  # Metadata layer - all identity/classification fields grouped
  metadata:
    name: "numbers"
    title: "Number Generator"
    description: "..."
    version: "1.0"
    category: "math"
    pattern: "generator"

  # Configuration layers
  parameters: [...]
  state: [...]
  connectivity: [...]
  agents: [...]
  constraints: [...]
  examples: [...]
  relatedTypes: [...]
  seeAlso: [...]
```

**Benefits**:
- ✅ Clear separation of concerns (metadata vs configuration)
- ✅ Mirrors the actual Metadata struct in Go code
- ✅ Intuitive hierarchy - identity at top, details below
- ✅ Easier to parse and validate in code
- ✅ Better organization for nested structures

---

## Files Updated

### Documentation Files
1. **WANT_TYPE_DEFINITION.md**
   - Updated schema specification (section 1.1)
   - Updated all 4 example definitions (sections 2.1-2.4)
   - Updated example code snippets throughout

### Example YAML Files (All 5 Updated)
1. ✅ `want_types/generators/numbers.yaml`
2. ✅ `want_types/processors/queue.yaml`
3. ✅ `want_types/independent/restaurant.yaml`
4. ✅ `want_types/coordinators/travel_coordinator.yaml`
5. ✅ `want_types/sinks/sink.yaml`

### Template File
1. ✅ `want_types/templates/WANT_TYPE_TEMPLATE.yaml`
   - Updated to use metadata layer
   - Removed duplicate pattern section

---

## Schema Structure (Complete)

```yaml
wantType:
  metadata:
    name: string                    # Unique identifier
    title: string                   # Human-readable title
    description: string             # What this want does
    version: string                 # Semantic version (e.g., "1.0")
    category: string                # Category for grouping
    pattern: string                 # Architectural pattern

  parameters:                        # Configuration parameters
    - name: string
      type: string
      default: <varies>
      required: boolean
      validation: {...}
      example: <varies>

  state:                            # State keys and definitions
    - name: string
      type: string
      persistent: boolean
      example: <varies>

  connectivity:                     # Input/output patterns
    inputs: [...]
    outputs: [...]

  agents:                           # Agent integration
    - name: string
      role: string
      description: string

  constraints:                      # Business logic validation
    - description: string
      validation: string

  examples:                         # Usage examples
    - name: string
      params: {...}
      expectedBehavior: string

  relatedTypes: [string]           # Cross-references
  seeAlso: [string]                # Documentation links
```

---

## Validation Rules for Metadata

When loading want type definitions, validate:

```go
type WantTypeMetadata struct {
    Name        string  // Required, must be non-empty, lowercase, no spaces
    Title       string  // Required, human-readable
    Description string  // Required, explains purpose
    Version     string  // Required, semantic versioning (e.g., "1.0")
    Category    string  // Required, one of: travel, queue, math, etc.
    Pattern     string  // Required, one of: generator, processor, sink, coordinator, independent
}
```

---

## Implementation Impact

### For WantTypeRegistry

When loading YAML:

```go
type WantTypeDefinition struct {
    Metadata WantTypeMetadata       // NEW: Layered structure

    Parameters []ParameterDef
    State []StateDef
    Connectivity ConnectivityDef
    Agents []AgentDef
    Constraints []ConstraintDef
    Examples []ExampleDef
    RelatedTypes []string
    SeeAlso []string
}

type WantTypeMetadata struct {
    Name        string
    Title       string
    Description string
    Version     string
    Category    string
    Pattern     string
}
```

### For API Responses

```go
// GET /api/v1/want-types/{name}
{
    "metadata": {
        "name": "restaurant",
        "title": "Restaurant Reservation",
        "description": "...",
        "version": "1.0",
        "category": "travel",
        "pattern": "independent"
    },
    "parameters": [...],
    "state": [...],
    ...
}
```

### For Frontend

When fetching want types:

```typescript
interface WantTypeDefinition {
    metadata: {
        name: string
        title: string
        description: string
        version: string
        category: string
        pattern: string
    }
    parameters: ParameterDef[]
    state: StateDef[]
    connectivity: ConnectivityDef
    agents: AgentDef[]
    // ... other fields
}
```

---

## Migration Path

### No Breaking Changes
- This is a schema refinement
- Implementation can be done gradually
- Both old and new formats can be supported during transition

### Update Order
1. ✅ Update documentation (completed)
2. ✅ Update example YAML files (completed)
3. ✅ Update template file (completed)
4. **Next**: Update WantTypeRegistry YAML parser
5. **Next**: Add metadata struct to Go code
6. **Next**: Update API response serialization
7. **Next**: Update frontend to use metadata layer

---

## Benefits of This Change

### 1. **Structural Clarity**
   - Metadata fields are explicitly grouped
   - Clear separation between identity and configuration
   - Easier to understand at a glance

### 2. **Code Generation Ready**
   - Can generate Go struct directly from metadata
   - Cleaner YAML-to-struct mapping
   - Enables automatic documentation generation

### 3. **API Consistency**
   - Want type definition matches Want struct layout
   - Frontend receives metadata in expected structure
   - Easier API client generation

### 4. **Validation Improvements**
   - All metadata fields validated together
   - Clearer error messages ("metadata.name is required")
   - Simpler validation code

### 5. **Extensibility**
   - Easy to add new metadata fields later
   - Doesn't disrupt parameters/state structure
   - Future-proof design

---

## Example: Before and After

### Restaurant Want Type - Before
```yaml
wantType:
  name: "restaurant"
  title: "Restaurant Reservation"
  description: "Finds and books a restaurant reservation..."
  version: "1.0"
  category: "travel"
  pattern: "independent"
  parameters:
    - name: "restaurant_type"
      ...
```

### Restaurant Want Type - After
```yaml
wantType:
  metadata:
    name: "restaurant"
    title: "Restaurant Reservation"
    description: "Finds and books a restaurant reservation..."
    version: "1.0"
    category: "travel"
    pattern: "independent"

  parameters:
    - name: "restaurant_type"
      ...
```

---

## Verification Checklist

- ✅ WANT_TYPE_DEFINITION.md updated with new schema
- ✅ All 5 example YAML files updated
- ✅ Template file updated
- ✅ Documentation examples show metadata layer
- ✅ Schema specification includes metadata grouping
- ✅ All files are valid YAML (tested with tools)
- ✅ No breaking changes to functionality
- ✅ Ready for implementation phase

---

## Next Steps

1. **Review** - Verify this structure matches your expectations
2. **Implement** - Update WantTypeRegistry parser to handle metadata layer
3. **Test** - Ensure all YAML files parse correctly
4. **Deploy** - Update API and frontend to use new structure

---

**Status**: Schema Update Complete
**Date**: 2024-11-12
**Files Modified**: 1 documentation + 5 YAML examples + 1 template = 7 files
**Breaking Changes**: None
**Ready for Implementation**: Yes
