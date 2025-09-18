# Coding Rules - MyWant Project

Welcome to the MyWant project! This document outlines the coding standards and best practices for contributing to this Go library that implements functional chain programming patterns with channels.

## Table of Contents

- [General Principles](#general-principles)
- [Go Code Style](#go-code-style)
- [Project Architecture](#project-architecture)
- [Testing Guidelines](#testing-guidelines)
- [Documentation Standards](#documentation-standards)
- [Git Workflow](#git-workflow)
- [Performance Considerations](#performance-considerations)

## General Principles

### 1. Simplicity and Clarity
- Write code that is easy to read and understand
- Prefer simple solutions over complex ones
- Use descriptive names for variables, functions, and types
- Avoid premature optimization

### 2. Consistency
- Follow existing code patterns in the project
- Use consistent naming conventions throughout
- Maintain consistent file organization

### 3. Functional Programming Approach
- Embrace immutability where possible
- Prefer pure functions with no side effects
- Use channels for communication between goroutines
- Implement chain patterns for data processing

## Go Code Style

### 1. Formatting
- Use `gofmt` or `goimports` for automatic formatting
- Use tabs for indentation (Go standard)
- Keep lines under 120 characters when reasonable

### 2. Naming Conventions
- Use `camelCase` for private functions and variables
- Use `PascalCase` for public functions, types, and variables
- Use descriptive names that explain the purpose
- Avoid abbreviations unless they're widely understood

```go
// Good
func processQueueMessages(queue *Queue) error {
    return nil
}

// Bad
func procQMsg(q *Queue) error {
    return nil
}
```

### 3. Error Handling
- Always handle errors explicitly
- Return errors as the last return value
- Use descriptive error messages
- Wrap errors with context when appropriate

```go
// Good
func loadConfig(filename string) (*Config, error) {
    data, err := os.ReadFile(filename)
    if err != nil {
        return nil, fmt.Errorf("failed to read config file %s: %w", filename, err)
    }
    // ... process data
    return config, nil
}
```

### 4. Interface Design
- Keep interfaces small and focused
- Define interfaces at the point of use
- Use composition over inheritance

### 5. Concurrency
- Use channels for communication between goroutines
- Avoid shared state when possible
- Use context.Context for cancellation and timeouts
- Always close channels when done

## Project Architecture

### 1. File Organization
- Keep related functionality in the same file
- Use descriptive filenames that reflect content
- Follow the established patterns:
  - `*_types.go` for want type implementations
  - `declarative.go` for core configuration system
  - `recipe_loader*.go` for recipe processing

### 2. Configuration System
- **Config files** (`config/`) are the primary user interface
- **Recipe files** (`recipes/`) provide reusable components
- **Demo programs** (`cmd/demos/`) serve as entry points
- Always validate configuration before processing

### 3. Want System Design
- Implement wants as independent, composable units
- Use label-based selectors for flexible connectivity
- Support both independent and dependent want patterns
- Provide clear registration functions for want types

```go
// Example want type registration
func RegisterMyWantTypes(builder *ChainBuilder) {
    builder.RegisterWantType("my_type", func() Want {
        return &MyWant{}
    })
}
```

### 4. Recipe System
- Use YAML for configuration with parameter substitution
- Keep recipes simple and reusable
- Provide clear parameter documentation
- Support both generic and owner-based loading

## Testing Guidelines

### 1. Test Coverage
- Write unit tests for all public functions
- Test error conditions and edge cases
- Aim for at least 80% test coverage

### 2. Test Structure
- Use table-driven tests for multiple scenarios
- Keep tests focused and independent
- Use descriptive test names

```go
func TestConfigLoader(t *testing.T) {
    tests := []struct {
        name     string
        filename string
        want     *Config
        wantErr  bool
    }{
        {
            name:     "valid config",
            filename: "testdata/valid-config.yaml",
            want:     &Config{},
            wantErr:  false,
        },
        // ... more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := loadConfigFromYAML(tt.filename)
            if (err != nil) != tt.wantErr {
                t.Errorf("loadConfigFromYAML() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            // ... assert results
        })
    }
}
```

### 3. Integration Tests
- Test complete workflows with real configurations
- Use the demo programs as integration test examples
- Test recipe loading and parameter substitution

## Documentation Standards

### 1. Code Comments
- Document all public functions and types
- Use Go doc comment format
- Explain the "why" not just the "what"
- Keep comments up to date with code changes

```go
// ChainBuilder constructs and manages execution chains from declarative configurations.
// It provides registration for want types and handles dynamic node addition during execution.
type ChainBuilder struct {
    config *Config
    chain  *chain.Chain
}

// Execute starts the chain execution and blocks until completion or error.
// It returns an error if any want fails during execution.
func (cb *ChainBuilder) Execute() error {
    // Implementation...
}
```

### 2. README Updates
- Keep README.md current with project changes
- Update examples when adding new features
- Document new make targets and commands

### 3. Configuration Documentation
- Document all configuration options
- Provide examples for common use cases
- Explain the relationship between configs and recipes

## Git Workflow

### 1. Commit Messages
- Use clear, descriptive commit messages
- Start with a verb in imperative mood
- Keep the first line under 50 characters
- Provide details in the body if needed

```
Add support for dynamic want connections

- Implement label-based selector matching
- Add validation for circular dependencies
- Update documentation with examples
```

### 2. Branch Management
- Create feature branches for new development
- Keep commits atomic and focused
- Rebase before merging to maintain clean history

### 3. Pull Requests
- Provide clear description of changes
- Include test cases for new functionality
- Update documentation as needed
- Ensure all tests pass before requesting review

## Performance Considerations

### 1. Memory Management
- Avoid memory leaks in long-running processes
- Use object pools for frequently allocated objects
- Profile memory usage for large datasets

### 2. Concurrency Performance
- Use buffered channels appropriately
- Avoid unnecessary goroutine creation
- Monitor channel blocking and deadlocks

### 3. Configuration Loading
- Cache loaded configurations when appropriate
- Validate configurations early to fail fast
- Use streaming for large configuration files

## Contributing Guidelines

### 1. Before Contributing
- Read through existing code to understand patterns
- Check for existing issues or discussions
- Run tests to ensure everything works

### 2. Making Changes
- Follow the coding standards outlined above
- Add tests for new functionality
- Update documentation as needed
- Run linters and formatters

### 3. Submitting Changes
- Create clear, focused pull requests
- Provide detailed descriptions
- Respond to review feedback promptly
- Ensure CI passes before requesting final review

## Tools and Commands

### Development Commands
```bash
# Format code
go fmt ./...
goimports -w .

# Run tests
go test ./...
go test -race ./...
go test -cover ./...

# Run linters
golangci-lint run

# Build examples
make run-travel-recipe
make run-queue-system-recipe
```

### Code Quality
- Use `golangci-lint` for comprehensive linting
- Run tests with race detection enabled
- Check test coverage regularly
- Profile performance-critical code

---

Thank you for contributing to MyWant! Following these guidelines helps maintain code quality and makes the project more accessible to all contributors.