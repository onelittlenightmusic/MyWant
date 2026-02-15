package mywant

import (
	"context"
	"reflect"
)

// Global registries for Go implementations of Want types
var (
	typeImplementationRegistry   = make(map[string]reflect.Type)
	localsImplementationRegistry = make(map[string]reflect.Type)
)

// Global registries for agent implementation functions
var (
	doActionRegistry      = make(map[string]func(context.Context, *Want) error)
	monitorActionRegistry = make(map[string]func(context.Context, *Want) error)
	pollActionRegistry    = make(map[string]PollFunc)
)

// RegisterDoAgent registers a DoAgent implementation logic by agent name.
func RegisterDoAgent(agentName string, action func(context.Context, *Want) error) {
	doActionRegistry[agentName] = action
}

// RegisterMonitorAgent registers a MonitorAgent implementation logic by agent name.
func RegisterMonitorAgent(agentName string, monitor func(context.Context, *Want) error) {
	monitorActionRegistry[agentName] = monitor
}

// RegisterPollAgent registers a PollAgent implementation logic by agent name.
func RegisterPollAgent(agentName string, poll PollFunc) {
	pollActionRegistry[agentName] = poll
}

// Global registry for agent implementation factories
var agentFactoryRegistry = make(map[string]func(registry *AgentRegistry))

// RegisterAgentImplementation registers an agent factory function that will be called
// when RegisterAllKnownAgentImplementations is invoked. Used in init() functions of
// agent implementation files, mirroring the pattern used by RegisterWantImplementation.
func RegisterAgentImplementation(name string, factory func(registry *AgentRegistry)) {
	agentFactoryRegistry[name] = factory
}

// Cap creates a Capability where Name == Gives[0] (most common pattern).
func Cap(name string) Capability {
	return Capability{Name: name, Gives: []string{name}}
}

// RegisterDoAgentType is a declarative API that registers a DoAgent in one call.
// It registers capabilities, creates the DoAgent, and wires it into the agent factory registry.
func RegisterDoAgentType(name string, capabilities []Capability, action func(context.Context, *Want) error) {
	RegisterDoAgent(name, action)

	capNames := make([]string, len(capabilities))
	for i, c := range capabilities {
		capNames[i] = c.Name
	}
	RegisterAgentImplementation(name, func(registry *AgentRegistry) {
		for _, c := range capabilities {
			registry.RegisterCapability(c)
		}
		agent := &DoAgent{
			BaseAgent: *NewBaseAgent(name, capNames, DoAgentType),
			Action:    action,
		}
		registry.RegisterAgent(agent)
	})
}

// RegisterMonitorAgentType is a declarative API that registers a MonitorAgent in one call.
// It registers capabilities, creates the MonitorAgent, and wires it into the agent factory registry.
func RegisterMonitorAgentType(name string, capabilities []Capability, monitor func(context.Context, *Want) error) {
	RegisterMonitorAgent(name, monitor)

	capNames := make([]string, len(capabilities))
	for i, c := range capabilities {
		capNames[i] = c.Name
	}
	RegisterAgentImplementation(name, func(registry *AgentRegistry) {
		for _, c := range capabilities {
			registry.RegisterCapability(c)
		}
		agent := &MonitorAgent{
			BaseAgent: *NewBaseAgent(name, capNames, MonitorAgentType),
			Monitor:   monitor,
		}
		registry.RegisterAgent(agent)
	})
}

// RegisterPollAgentType is a declarative API that registers a poll-based MonitorAgent in one call.
// Unlike RegisterMonitorAgentType, the poll function can signal stop via shouldStop=true.
func RegisterPollAgentType(name string, capabilities []Capability, poll PollFunc) {
	RegisterPollAgent(name, poll)

	capNames := make([]string, len(capabilities))
	for i, c := range capabilities {
		capNames[i] = c.Name
	}
	RegisterAgentImplementation(name, func(registry *AgentRegistry) {
		for _, c := range capabilities {
			registry.RegisterCapability(c)
		}
		agent := &PollAgent{
			BaseAgent: *NewBaseAgent(name, capNames, MonitorAgentType),
			Poll:      poll,
		}
		registry.RegisterAgent(agent)
	})
}

// RegisterAllKnownAgentImplementations calls all registered agent factory functions
// to register their agents with the provided AgentRegistry.
func RegisterAllKnownAgentImplementations(registry *AgentRegistry) {
	for _, factory := range agentFactoryRegistry {
		factory(registry)
	}
}

// RegisterWantImplementation registers a Go implementation (struct and locals) for a specific Want type name.
// Used in init() functions of specific Want type implementation files.
// T should be the struct that embeds Want (e.g., ReminderWant).
// L should be the locals struct (e.g., ReminderLocals).
func RegisterWantImplementation[T any, L any](typeName string) {
	typeImplementationRegistry[typeName] = reflect.TypeOf((*T)(nil)).Elem()
	localsImplementationRegistry[typeName] = reflect.TypeOf((*L)(nil)).Elem()
}

// createGenericFactory creates a WantFactory that instantiates and initializes a Want implementation using reflection.
func createGenericFactory(typeName string) WantFactory {
	return func(metadata Metadata, spec WantSpec) Progressable {
		tType, ok := typeImplementationRegistry[typeName]
		if !ok {
			return nil
		}

		// Instantiate the implementation struct (e.g., ReminderWant)
		// reflect.New returns a pointer to the type
		instancePtr := reflect.New(tType)

		// Instantiate the locals struct (e.g., ReminderLocals)
		var locals any
		if lType, ok := localsImplementationRegistry[typeName]; ok {
			localsPtr := reflect.New(lType)
			locals = localsPtr.Interface()
		}

		// Create the base Want using NewWantWithLocals
		baseWant := NewWantWithLocals(metadata, spec, locals, typeName)

		// Initialize the instance by injecting the base Want
		initializeInstance(instancePtr.Interface(), baseWant)

		// Type assertion to Progressable
		if progressable, ok := instancePtr.Interface().(Progressable); ok {
			return progressable
		}
		return nil
	}
}

// initializeInstance injects the base Want into the implementation struct via reflection.
// It handles both struct embedding (Want) and pointer embedding (*Want).
func initializeInstance(instance any, baseWant *Want) {
	val := reflect.ValueOf(instance)
	if val.Kind() != reflect.Ptr {
		return
	}
	elem := val.Elem()
	if elem.Kind() != reflect.Struct {
		return
	}

	// Find the field named "Want"
	field := elem.FieldByName("Want")
	if !field.IsValid() || !field.CanSet() {
		return
	}

	// Set the base Want according to the field type (pointer or struct)
	fieldType := field.Type()
	// Case 1: Pointer embedding (*Want)
	if fieldType.Kind() == reflect.Ptr && fieldType.Elem().Name() == "Want" {
		field.Set(reflect.ValueOf(baseWant))
	} else if fieldType.Kind() == reflect.Struct && fieldType.Name() == "Want" {
		// Case 2: Struct embedding (Want) - use Elem() to avoid copying the mutex
		field.Set(reflect.ValueOf(baseWant).Elem())
	}
}

// GetLocals provides a type-safe way to access the Locals field of a Want.
// L should be the specific locals struct type (e.g., ReminderLocals).
func GetLocals[L any](w *Want) *L {
	if w.Locals == nil {
		return nil
	}
	if locals, ok := w.Locals.(*L); ok {
		return locals
	}
	return nil
}

// RegisterAllKnownImplementations registers all Go implementations in the registry with the provided ChainBuilder.
// This is useful for demos or tests where YAML-based automatic registration via StoreWantTypeDefinition might not occur.
func (cb *ChainBuilder) RegisterAllKnownImplementations() {
	for typeName := range typeImplementationRegistry {
		// Only register if not already registered manually
		if _, exists := cb.registry[typeName]; !exists {
			cb.RegisterWantType(typeName, createGenericFactory(typeName))
		}
	}
}
