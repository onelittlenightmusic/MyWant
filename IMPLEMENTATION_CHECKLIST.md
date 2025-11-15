# Want Type Server Loading - Implementation Checklist

## Quick Overview

**Goal**: Load all 24 want type YAML definitions at server startup (like recipe loader)

**Current**: 5/24 YAML files exist (21% coverage)

**Missing**: 19 YAML files need to be created

**Pattern**: Follow GenericRecipeLoader design from `recipe_loader_generic.go`

---

## Phase 1: WantTypeLoader Implementation

### Create `engine/src/want_type_loader.go`

- [ ] Create file structure
- [ ] Implement struct:
  ```go
  type WantTypeLoader struct {
      directory string
      definitions map[string]*WantTypeDefinition
      byCategory map[string][]*WantTypeDefinition
      byPattern map[string][]*WantTypeDefinition
      mu sync.RWMutex
  }
  ```

- [ ] Implement LoadAllWantTypes()
  - [ ] Scan want_types/ directory recursively
  - [ ] Find all *.yaml files
  - [ ] Parse each YAML file
  - [ ] Validate structure
  - [ ] Build category index
  - [ ] Build pattern index

- [ ] Implement GetDefinition(name string)
  - [ ] Lookup by name
  - [ ] Return nil if not found

- [ ] Implement ListByCategory(cat string)
  - [ ] Return all types in category

- [ ] Implement ListByPattern(pat string)
  - [ ] Return all types with pattern

- [ ] Implement ValidateDefinition(def)
  - [ ] Check metadata fields non-empty
  - [ ] Check pattern is valid
  - [ ] Check parameters have type
  - [ ] Check state has type
  - [ ] Return detailed errors

- [ ] Add unit tests
  - [ ] Test loading single file
  - [ ] Test loading directory
  - [ ] Test indexing by category
  - [ ] Test indexing by pattern
  - [ ] Test validation
  - [ ] Test error cases

---

## Phase 2: Server Integration

### Modify `engine/cmd/server/main.go`

#### In NewServer() method:

- [ ] Import want_type_loader package
- [ ] Create WantTypeLoader instance
  ```go
  loader := want.NewWantTypeLoader("want_types")
  ```
- [ ] Call LoadAllWantTypes()
  ```go
  err := loader.LoadAllWantTypes()
  if err != nil {
      log.Fatalf("Failed to load want type definitions: %v", err)
  }
  ```
- [ ] Add to Server struct
  ```go
  server.wantTypeLoader = loader
  ```

#### In Start() method:

- [ ] Add validation check
  ```go
  // Warn about missing definitions
  for _, typeName := range getRegisteredWantTypeNames() {
      def := server.wantTypeLoader.GetDefinition(typeName)
      if def == nil {
          log.Warnf("No definition found for want type: %s", typeName)
      }
  }
  ```

#### Update Server struct:

- [ ] Add field
  ```go
  wantTypeLoader *WantTypeLoader
  ```

---

## Phase 3: API Endpoints

### Add to `engine/cmd/server/main.go` routes section

#### GET /api/v1/want-types

- [ ] Implement handler
- [ ] Support ?category=X filter
- [ ] Support ?pattern=X filter
- [ ] Return list of definitions
- [ ] Write tests

#### GET /api/v1/want-types/{name}

- [ ] Implement handler
- [ ] Return full definition
- [ ] Return 404 if not found
- [ ] Write tests

#### GET /api/v1/want-types/{name}/examples

- [ ] Implement handler
- [ ] Return examples array
- [ ] Return 404 if not found
- [ ] Write tests

#### Optional Future Endpoints

- [ ] POST /api/v1/want-types (register new type)
- [ ] PUT /api/v1/want-types/{name} (update type)
- [ ] DELETE /api/v1/want-types/{name} (delete type)

---

## Phase 4: Create Missing YAML Files

### Travel Domain (3 files)

- [ ] **hotel.yaml**
  - [ ] Extract parameters from HotelWant code
  - [ ] Document state keys
  - [ ] Define agents if used
  - [ ] Add 2+ examples
  - [ ] Validate YAML

- [ ] **buffet.yaml**
  - [ ] Extract parameters from BuffetWant code
  - [ ] Document state keys
  - [ ] Define agents if used
  - [ ] Add 2+ examples
  - [ ] Validate YAML

- [ ] **flight.yaml** (or flight_alt.yaml)
  - [ ] Extract parameters from FlightWant code
  - [ ] Document state keys
  - [ ] Define agents if used
  - [ ] Add 2+ examples
  - [ ] Validate YAML

### Queue Domain (2 files)

- [ ] **combiner.yaml**
  - [ ] Extract from Combiner type
  - [ ] Define connectivity (multiple inputs)
  - [ ] Add examples
  - [ ] Validate YAML

- [ ] **collector.yaml**
  - [ ] Extract from Collector type
  - [ ] Define as sink pattern
  - [ ] Add examples
  - [ ] Validate YAML

### Fibonacci Domain (3 files)

- [ ] **fibonacci_numbers.yaml**
  - [ ] Generator pattern
  - [ ] Extract parameters
  - [ ] Add examples

- [ ] **fibonacci_sequence.yaml**
  - [ ] Processor pattern
  - [ ] Extract parameters
  - [ ] Add examples

- [ ] **fibonacci_adder.yaml**
  - [ ] Processor pattern
  - [ ] Extract parameters
  - [ ] Add examples

### Fibonacci Loop Domain (3 files)

- [ ] **fibonacci_loop.yaml**
- [ ] **fibonacci_source_loop.yaml**
- [ ] **fibonacci_adder_loop.yaml**

### Prime Domain (3 files)

- [ ] **prime_numbers.yaml**
  - [ ] Generator pattern
  - [ ] Extract parameters
  - [ ] Add examples

- [ ] **prime_sieve.yaml**
  - [ ] Processor pattern
  - [ ] Extract parameters
  - [ ] Add examples

- [ ] **prime_sequence.yaml**
  - [ ] Processor pattern
  - [ ] Extract parameters
  - [ ] Add examples

### Approval Domain (2 files)

- [ ] **evidence.yaml**
  - [ ] Independent pattern
  - [ ] Extract parameters
  - [ ] Add examples

- [ ] **description.yaml**
  - [ ] Independent pattern
  - [ ] Extract parameters
  - [ ] Add examples

### System Domain (2 files)

- [ ] **owner.yaml**
  - [ ] System type
  - [ ] Document purpose
  - [ ] Add examples

- [ ] **custom_target.yaml**
  - [ ] System type
  - [ ] Document purpose
  - [ ] Add examples

### Create Directory Structure

- [ ] want_types/system/ (new)
- [ ] Move system types into it
- [ ] Update all filenames to be consistent

---

## Validation & Testing

### Unit Tests

- [ ] Test WantTypeLoader.LoadAllWantTypes()
- [ ] Test WantTypeLoader.GetDefinition()
- [ ] Test WantTypeLoader.ListByCategory()
- [ ] Test WantTypeLoader.ListByPattern()
- [ ] Test validation logic
- [ ] Test error handling

### Integration Tests

- [ ] Server starts with loaded want types
- [ ] No errors during initialization
- [ ] All 24 types have definitions
- [ ] API endpoints work correctly
- [ ] Filtering works (category, pattern)

### YAML Validation

- [ ] All YAML files are valid syntax
- [ ] All metadata fields non-empty
- [ ] All patterns are valid values
- [ ] All parameter types defined
- [ ] All state keys have types
- [ ] All connectivity valid

### Code Integration Tests

- [ ] WantTypeLoader initializes in main.go
- [ ] Definitions available for parameter validation
- [ ] API endpoints return correct data
- [ ] Error messages reference want type definitions

---

## Frontend Updates (Optional - Phase 5)

### Component Updates

- [ ] Fetch want types on page load
- [ ] Display want type selection
- [ ] Generate parameter forms from definitions
- [ ] Show parameter validation rules
- [ ] Display examples
- [ ] Show agent requirements
- [ ] Show related types

### API Integration

- [ ] Fetch from GET /api/v1/want-types
- [ ] Fetch from GET /api/v1/want-types/{name}
- [ ] Fetch from GET /api/v1/want-types/{name}/examples
- [ ] Handle 404 responses
- [ ] Cache definitions in state

---

## Documentation

- [ ] Update API documentation
- [ ] Update server startup documentation
- [ ] Add examples of API usage
- [ ] Document want type schema
- [ ] Create migration guide for existing configs
- [ ] Update README

---

## Final Verification

### Before Merging

- [ ] All 24 want types have YAML definitions
- [ ] All YAML files validate successfully
- [ ] Server starts without errors
- [ ] API endpoints work
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] No breaking changes to existing code
- [ ] Documentation updated
- [ ] Code reviewed

### After Deployment

- [ ] Server starts with want type loader
- [ ] No warnings about missing definitions
- [ ] Frontend can fetch definitions
- [ ] Frontend generates forms correctly
- [ ] No performance regressions
- [ ] Logging shows all types loaded

---

## Success Metrics

| Metric | Target | Status |
|--------|--------|--------|
| YAML Files Created | 24/24 | - |
| Loader Implementation | 100% | - |
| API Endpoints | 3/3 | - |
| Server Integration | Complete | - |
| Unit Test Coverage | 90%+ | - |
| Integration Tests | All Pass | - |
| Documentation | Complete | - |

---

## Priority Order (Recommended)

### Week 1
1. **Phase 1**: WantTypeLoader implementation (2 days)
2. **Phase 2**: Server integration (1 day)
3. **Phase 3**: API endpoints (1 day)
4. **Phase 4**: High-priority YAML files (2 days)
   - hotel.yaml, buffet.yaml
   - fibonacci_numbers.yaml, fibonacci_sequence.yaml
   - prime_numbers.yaml
   - Total: 5 critical files

### Week 2
5. **Phase 4**: Remaining YAML files (3-5 days)
   - All other types
   - Testing and validation

### Week 3
6. Testing and validation (1-2 days)
7. Frontend integration (optional)
8. Documentation updates

---

## File Locations

### Files to Create
- `engine/src/want_type_loader.go` (new)
- `want_types/{category}/{name}.yaml` (19 files)

### Files to Modify
- `engine/cmd/server/main.go` (add initialization and endpoints)

### Files to Reference
- `engine/src/recipe_loader_generic.go` (pattern to follow)
- Existing YAML files in `want_types/`

---

## Common Extraction Tasks (For Creating YAML)

When creating a new want type YAML, follow this process:

### 1. Find Want Type Implementation
```bash
grep -r "type [Name]Want struct" engine/cmd/types/
```

### 2. Extract Parameters
```bash
grep "spec.Params\[" engine/cmd/types/*_types.go
```

### 3. Extract State Keys
```bash
grep "StoreState\|GetState" engine/cmd/types/*_types.go
```

### 4. Check for Agents
```bash
grep "agentRegistry\|Agent" engine/cmd/types/*_types.go
```

### 5. Identify Connectivity
```bash
grep "ConnectivityMetadata\|InputLabel\|OutputLabel" engine/cmd/types/*_types.go
```

### 6. Use Template
```bash
cp want_types/templates/WANT_TYPE_TEMPLATE.yaml want_types/{pattern}/{name}.yaml
```

### 7. Fill in Extracted Info
- Update metadata
- Add parameters from step 2
- Add state from step 3
- Add agents from step 4
- Set connectivity from step 5

### 8. Validate
```bash
# Check YAML syntax
yaml-lint want_types/{pattern}/{name}.yaml

# Verify structure matches schema
# Check against WANT_TYPE_DEFINITION.md
```

---

## Timeline Summary

| Phase | Task | Time | Start | End |
|-------|------|------|-------|-----|
| 1 | WantTypeLoader | 1-2d | Week 1 Mon | Week 1 Tue |
| 2 | Server Integration | 1d | Week 1 Wed | Week 1 Wed |
| 3 | API Endpoints | 1d | Week 1 Wed | Week 1 Thu |
| 4a | High-Priority YAML | 2d | Week 1 Mon | Week 1 Tue |
| 4b | Remaining YAML | 3-5d | Week 1 Fri | Week 2 Fri |
| 5 | Testing & Polish | 1-2d | Week 3 | Week 3 |

**Can parallelize phases 1-3 with 4a → Completes in 1 week**

---

## Questions to Answer

1. **Which want types are critical for demos?**
   - Answer: Travel types (restaurant, hotel, buffet, flight, coordinator)
   - Priority: High

2. **Are there tests that require all want types to load?**
   - Check: Tests in engine/cmd/tests/

3. **What parameters do approval and owner types need?**
   - Check: approval_types.go, owner_types.go for Exec() code

4. **Should system types have separate category?**
   - Recommendation: Yes, want_types/system/

5. **Who reviews YAML definitions for accuracy?**
   - Recommendation: Original want type implementer

---

**Status**: Ready to implement
**Next Step**: Approval and start Phase 1
