# Coding Rule Violations - Comprehensive Fix Plan

## Executive Summary

This document outlines a comprehensive plan to fix all coding rule violations found in the MyWant codebase. The violations range from critical concurrency issues to missing test coverage and documentation gaps.

## Critical Issues (Priority 1 - Must Fix)

### 1. Mutex Copying Violations (🚨 CRITICAL)

**Problem**: The `Want` struct contains `sync.RWMutex` and is being passed by value throughout the codebase, causing mutex copying which breaks thread safety.

**Files Affected**:
- `src/declarative.go` (47 violations)
- `src/owner_types.go` (3 violations)
- `src/recipe_loader.go` (4 violations)
- `src/recipe_loader_generic.go` (4 violations)

**Root Cause**:
```go
type Want struct {
    // ... other fields
    agentStateMutex sync.RWMutex `json:"-" yaml:"-"`
}
```

**Fix Strategy**:
1. **Immediate Fix**: Always pass `Want` by pointer (`*Want`) instead of by value
2. **Long-term Fix**: Consider moving mutex to separate concurrent-safe wrapper

**Implementation Plan**:

#### Phase 1: Function Signature Updates
- [ ] Update all functions to accept `*Want` instead of `Want`
- [ ] Update method receivers to use pointer receivers
- [ ] Update range loops to use pointers

#### Phase 2: Data Structure Updates
- [ ] Update slices and maps to store `*Want` instead of `Want`
- [ ] Update Config struct to use `[]*Want` instead of `[]Want`
- [ ] Update all constructors and factories

#### Phase 3: Method Call Updates
- [ ] Update all function calls to pass `&want` instead of `want`
- [ ] Update assignments to use pointers
- [ ] Update comparisons and equality checks

**Estimated Effort**: 2-3 days, 58+ locations to fix

### 2. Missing Test Infrastructure (🚨 CRITICAL)

**Problem**: Zero test coverage across the entire codebase.

**Fix Strategy**:
1. Create basic test infrastructure
2. Add unit tests for critical components
3. Add integration tests for main workflows

**Implementation Plan**:

#### Phase 1: Test Infrastructure
- [ ] Create `src/declarative_test.go`
- [ ] Create `src/chain/chain_test.go`
- [ ] Create `cmd/server/main_test.go`
- [ ] Set up test data directory with sample configs
- [ ] Add benchmarks for performance-critical code

#### Phase 2: Core Component Tests
- [ ] `Want` struct methods (state, parameters, history)
- [ ] `ChainBuilder` functionality (build, execute, reconcile)
- [ ] Recipe loading and processing
- [ ] Configuration validation

#### Phase 3: Integration Tests
- [ ] End-to-end recipe execution
- [ ] Server API endpoints
- [ ] Config file loading and validation
- [ ] Memory reconciliation

**Estimated Effort**: 1-2 weeks

## High Priority Issues (Priority 2)

### 3. Syntax Errors in Archive Files

**Problem**: Several files in `archive/` have syntax errors preventing compilation.

**Files Affected**:
- `archive/closure_channel.go:36` - expected ';', found '<-'
- `archive/pathtest1.go:32` - missing ',' in composite literal
- `archive/test_ubuntu.go:10,20` - syntax errors

**Fix Strategy**:
1. **Option A**: Fix syntax errors if files are needed
2. **Option B**: Move to separate archive repository
3. **Option C**: Delete if no longer needed

**Recommended**: Move to separate archive repository to keep main codebase clean.

### 4. Missing Documentation

**Problem**: Many public functions and types lack Go doc comments.

**Fix Strategy**:
1. Add documentation for all exported types and functions
2. Add package-level documentation
3. Update README with API documentation

**Priority Order**:
1. Core types (`Want`, `Config`, `ChainBuilder`)
2. Public interfaces and methods
3. Package documentation
4. Complex internal functions

## Medium Priority Issues (Priority 3)

### 5. Code Style and Formatting

**Problem**: Inconsistent formatting and style issues.

**Current Status**: ✅ Partially fixed by `make fmt`

**Remaining Issues**:
- [ ] Add golangci-lint configuration
- [ ] Fix any remaining linter warnings
- [ ] Ensure consistent error handling patterns
- [ ] Standardize logging format

### 6. Error Handling Improvements

**Problem**: Inconsistent error handling and missing error context.

**Fix Strategy**:
1. Wrap errors with context using `fmt.Errorf`
2. Add validation for all user inputs
3. Improve error messages with actionable information

## Implementation Roadmap

### Week 1: Critical Fixes
- **Days 1-3**: Fix mutex copying violations
- **Days 4-5**: Create basic test infrastructure

### Week 2: Test Coverage & Documentation
- **Days 1-3**: Add comprehensive unit tests
- **Days 4-5**: Add missing documentation

### Week 3: Quality & Polish
- **Days 1-2**: Fix archive syntax errors
- **Days 3-4**: Implement golangci-lint configuration
- **Days 5**: Integration testing and validation

## Detailed Fix Plans

### Mutex Copying Fix - Detailed Steps

1. **Update Config struct**:
```go
// Before
type Config struct {
    Wants []Want `json:"wants" yaml:"wants"`
}

// After
type Config struct {
    Wants []*Want `json:"wants" yaml:"wants"`
}
```

2. **Update function signatures** (58 locations):
```go
// Before
func (cb *ChainBuilder) addWant(wantConfig Want)
func (cb *ChainBuilder) wantsEqual(a, b Want) bool

// After
func (cb *ChainBuilder) addWant(wantConfig *Want)
func (cb *ChainBuilder) wantsEqual(a, b *Want) bool
```

3. **Update range loops**:
```go
// Before
for _, want := range cb.config.Wants {
    cb.addDynamicWantUnsafe(want)
}

// After
for _, want := range cb.config.Wants {
    cb.addDynamicWantUnsafe(want) // want is already *Want
}
```

### Test Infrastructure - Detailed Steps

1. **Create test file structure**:
```
src/
├── declarative_test.go
├── chain/
│   └── chain_test.go
├── testdata/
│   ├── valid-config.yaml
│   ├── invalid-config.yaml
│   └── sample-recipe.yaml
└── ...
```

2. **Basic test template**:
```go
func TestWantStateManagement(t *testing.T) {
    want := &Want{
        Metadata: Metadata{Name: "test-want", Type: "test"},
        Spec:     WantSpec{Params: make(map[string]interface{})},
    }

    want.StoreState("key", "value")

    value, exists := want.GetState("key")
    assert.True(t, exists)
    assert.Equal(t, "value", value)
}
```

### Golangci-lint Configuration

Create `.golangci.yml`:
```yaml
linters-settings:
  govet:
    check-shadowing: true
  gocyclo:
    min-complexity: 15
  goconst:
    min-len: 2
    min-occurrences: 2

linters:
  enable:
    - bodyclose
    - deadcode
    - depguard
    - goconst
    - gocyclo
    - gofmt
    - goimports
    - gosec
    - gosimple
    - govet
    - ineffassign
    - misspell
    - staticcheck
    - structcheck
    - typecheck
    - unconvert
    - unparam
    - unused
    - varcheck

issues:
  exclude-rules:
    - path: archive/
      linters:
        - govet
        - gofmt
```

## Validation Plan

### Pre-Fix Validation
- [ ] Run `make vet` to baseline current violations
- [ ] Run `make test` to confirm no tests exist
- [ ] Document current build/run status

### Post-Fix Validation
- [ ] `make check` must pass cleanly
- [ ] All demo programs must run successfully
- [ ] Test coverage must be >80% for critical components
- [ ] All public APIs must have documentation

## Risk Assessment

### High Risk
- **Mutex copying fixes**: Could introduce regressions if not done carefully
- **Config struct changes**: May break YAML serialization/deserialization

### Medium Risk
- **Large-scale refactoring**: May introduce subtle bugs
- **Test additions**: May reveal existing bugs

### Mitigation Strategies
1. **Incremental changes**: Fix one violation type at a time
2. **Comprehensive testing**: Test each change thoroughly
3. **Backup and branch**: Use git branches for each major change
4. **Demo validation**: Ensure all demo programs work after each change

## Success Criteria

✅ **All go vet warnings resolved**
✅ **Test coverage >80% for core components**
✅ **All public APIs documented**
✅ **Clean golangci-lint run**
✅ **All demo programs functional**
✅ **Build process includes quality checks**

## Maintenance Plan

### Ongoing Quality Assurance
1. **Pre-commit hooks**: Run `make check` before each commit
2. **CI/CD integration**: Automated quality checks on PR
3. **Regular reviews**: Monthly code quality reviews
4. **Documentation updates**: Keep docs current with code changes

This plan ensures the MyWant codebase meets professional open source standards while maintaining functionality and enabling future development.