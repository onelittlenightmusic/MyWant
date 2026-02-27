package mywant

import "fmt"

// RegisterWantType allows registering custom want types
func (cb *ChainBuilder) RegisterWantType(wantType string, factory WantFactory) {
	cb.registry[wantType] = factory
}

// RegisterWantTypeFromYAML registers a want type and loads connectivity metadata from YAML
func (cb *ChainBuilder) RegisterWantTypeFromYAML(wantType string, factory WantFactory, yamlDefPath string) error {
	// Load YAML definition
	def, err := LoadWantTypeDefinition(yamlDefPath)
	if err != nil {
		return fmt.Errorf("failed to load want type definition from %s: %w", yamlDefPath, err)
	}

	// Register the factory
	cb.registry[wantType] = factory

	// Store want type definition for later use during want creation
	if cb.wantTypeDefinitions == nil {
		cb.wantTypeDefinitions = make(map[string]*WantTypeDefinition)
	}
	cb.wantTypeDefinitions[wantType] = def

	// Store connectivity metadata for later use during want creation
	if cb.connectivityRegistry == nil {
		cb.connectivityRegistry = make(map[string]ConnectivityMetadata)
	}

	// Use connect field if available, then require, otherwise fall back to usageLimit
	if def.Connect != nil {
		// Convert RequireSpec to ConnectivityMetadata
		cb.connectivityRegistry[wantType] = def.Connect.ToConnectivityMetadata(wantType)
	} else if def.Require != nil {
		// Convert RequireSpec to ConnectivityMetadata
		cb.connectivityRegistry[wantType] = def.Require.ToConnectivityMetadata(wantType)
	} else if def.UsageLimit != nil {
		// Legacy support for UsageLimit
		cb.connectivityRegistry[wantType] = def.UsageLimit.ToConnectivityMetadata(wantType)
	}

	return nil
}

// StoreWantTypeDefinition stores a want type definition without registering a factory
// Used when definitions are already registered separately and we just need to make them available for state initialization
// Also registers aliases for special naming patterns (e.g., "queue" -> "qnet queue")
func (cb *ChainBuilder) StoreWantTypeDefinition(def *WantTypeDefinition) {
	if def == nil {
		return
	}

	// Store want type definition for later use during want creation
	if cb.wantTypeDefinitions == nil {
		cb.wantTypeDefinitions = make(map[string]*WantTypeDefinition)
	}

	wantType := def.Metadata.Name
	cb.wantTypeDefinitions[wantType] = def

	// Automatically register factory if a Go implementation exists in the registry
	if _, ok := typeImplementationRegistry[wantType]; ok {
		if _, alreadyRegistered := cb.registry[wantType]; !alreadyRegistered {
			cb.RegisterWantType(wantType, createGenericFactory(wantType))
		}
	}

	// Store connectivity metadata for later use during want creation
	if cb.connectivityRegistry == nil {
		cb.connectivityRegistry = make(map[string]ConnectivityMetadata)
	}

	// Use connect field if available, then require, otherwise fall back to usageLimit
	metadata := func() ConnectivityMetadata {
		if def.Connect != nil {
			// Convert RequireSpec to ConnectivityMetadata
			return def.Connect.ToConnectivityMetadata(wantType)
		} else if def.Require != nil {
			// Convert RequireSpec to ConnectivityMetadata
			return def.Require.ToConnectivityMetadata(wantType)
		} else if def.UsageLimit != nil {
			// Legacy support for UsageLimit
			return def.UsageLimit.ToConnectivityMetadata(wantType)
		}
		return ConnectivityMetadata{}
	}()
	cb.connectivityRegistry[wantType] = metadata

	// Register aliases for special naming patterns
	// This handles naming mismatches between YAML definitions and code registrations
	var aliases []string
	switch wantType {
	case "queue":
		// Queue type can be referenced as "qnet queue" in code
		aliases = []string{"qnet queue"}
	case "combiner":
		// Combiner type can be referenced as "qnet combiner" in code
		aliases = []string{"qnet combiner"}
	case "numbers":
		// Numbers type can be referenced as "qnet numbers" in code
		aliases = []string{"qnet numbers"}
	}

	// Store definitions and metadata under all aliases
	for _, alias := range aliases {
		cb.wantTypeDefinitions[alias] = def
		cb.connectivityRegistry[alias] = metadata

		// Also register factory for alias if it exists
		if _, ok := typeImplementationRegistry[wantType]; ok {
			if _, alreadyRegistered := cb.registry[alias]; !alreadyRegistered {
				cb.RegisterWantType(alias, createGenericFactory(wantType))
			}
		}
	}
}

// GetWantTypeDefinition retrieves a want type definition by name
func (cb *ChainBuilder) GetWantTypeDefinition(wantType string) *WantTypeDefinition {
	if cb.wantTypeDefinitions == nil {
		return nil
	}
	return cb.wantTypeDefinitions[wantType]
}

func (cb *ChainBuilder) SetAgentRegistry(registry *AgentRegistry) {
	cb.agentRegistry = registry
}

func (cb *ChainBuilder) GetAgentRegistry() *AgentRegistry {
	return cb.agentRegistry
}

func (cb *ChainBuilder) SetCustomTargetRegistry(registry *CustomTargetTypeRegistry) {
	cb.customRegistry = registry
}

func (cb *ChainBuilder) SetConfigInternal(config Config) {
	cb.config = config
}

func (cb *ChainBuilder) SetServerMode(isServer bool) {
	cb.isServerMode = isServer
}

// SetHTTPClient sets the HTTP client for internal API calls
func (cb *ChainBuilder) SetHTTPClient(client *HTTPClient) {
	cb.httpClient = client
}

// GetHTTPClient returns the HTTP client for internal API calls
func (cb *ChainBuilder) GetHTTPClient() *HTTPClient {
	return cb.httpClient
}

func (cb *ChainBuilder) createWantFunction(want *Want) (any, error) {
	wantType := want.Metadata.Type

	// Check if it's a custom type first
	if cb.customRegistry != nil && cb.customRegistry.IsCustomType(wantType) {
		return cb.createCustomTargetWant(want)
	}

	// Fall back to standard type registration
	factory, exists := cb.registry[wantType]
	if !exists {
		// List available types for better error message
		availableTypes := make([]string, 0, len(cb.registry))
		for typeName := range cb.registry {
			availableTypes = append(availableTypes, typeName)
		}
		customTypes := make([]string, 0)
		if cb.customRegistry != nil {
			customTypes = cb.customRegistry.ListTypes()
		}

		return nil, fmt.Errorf("Unknown want type: '%s'. Available standard types: %v. Available custom types: %v",
			wantType, availableTypes, customTypes)
	}

	factoryResult := factory(want.Metadata, want.Spec)

	// Extract *Want from the Progressable result via reflection
	// All factories now return Progressable implementations that embed Want
	var wantPtr *Want
	if w, err := extractWantFromProgressable(factoryResult); err == nil {
		wantPtr = w
	} else {
		return nil, fmt.Errorf("factory returned Progressable but could not extract Want: %v", err)
	}

	// Automatically set want type definition if available
	// This initializes ProvidedStateFields and sets initial state values
	if wantPtr != nil && cb.wantTypeDefinitions != nil {
		if typeDef, exists := cb.wantTypeDefinitions[wantType]; exists {
			wantPtr.SetWantTypeDefinition(typeDef)
		}
	}

	if cb.agentRegistry != nil && wantPtr != nil {
		wantPtr.SetAgentRegistry(cb.agentRegistry)
	}

	// Automatically wrap with OwnerAwareWant if the want has owner references This enables parent-child coordination via subscription events
	if len(want.Metadata.OwnerReferences) > 0 {
		return NewOwnerAwareWant(factoryResult, want.Metadata, wantPtr), nil
	}

	return factoryResult, nil
}

// TestCreateWantFunction tests want type creation without side effects (exported for validation)
func (cb *ChainBuilder) TestCreateWantFunction(want *Want) (any, error) {
	return cb.createWantFunction(want)
}

func (cb *ChainBuilder) createCustomTargetWant(want *Want) (any, error) {
	config, exists := cb.customRegistry.Get(want.Metadata.Type)
	if !exists {
		availableTypes := cb.customRegistry.ListTypes()
		return nil, fmt.Errorf("custom type '%s' not found in registry. Available: %v", want.Metadata.Type, availableTypes)
	}

	// Merge custom type defaults with user-provided spec
	mergedSpec := cb.mergeWithCustomDefaults(want.Spec, config)
	target := config.CreateTargetFunc(want.Metadata, mergedSpec)
	target.SetBuilder(cb)
	recipeLoader := NewGenericRecipeLoader("recipes")
	target.SetRecipeLoader(recipeLoader)

	// Set the custom_target want type definition to enable state fields
	// Custom targets are based on the custom_target want type, so we set that definition
	if cb.wantTypeDefinitions != nil {
		if typeDef, exists := cb.wantTypeDefinitions["custom_target"]; exists {
			target.Want.SetWantTypeDefinition(typeDef)
		}
	}

	// Automatically wrap with OwnerAwareWant if the custom target has owner references This enables parent-child coordination via subscription events (critical for nested targets)
	var wantInstance any = target
	if len(want.Metadata.OwnerReferences) > 0 {
		wantInstance = NewOwnerAwareWant(wantInstance, want.Metadata, &target.Want)
	}

	return wantInstance, nil
}

// mergeWithCustomDefaults merges user spec with custom type defaults
func (cb *ChainBuilder) mergeWithCustomDefaults(spec WantSpec, config CustomTargetTypeConfig) WantSpec {
	merged := spec
	if merged.Params == nil {
		merged.Params = make(map[string]any)
	}

	// Merge default parameters (user params take precedence)
	for key, defaultValue := range config.DefaultParams {
		if _, exists := merged.Params[key]; !exists {
			merged.Params[key] = defaultValue
		}
	}

	// Recipe is no longer used - custom types are distinguished by metadata.name

	return merged
}
