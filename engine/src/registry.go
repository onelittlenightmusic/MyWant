package mywant

import (
	"reflect"
)

// Global registries for Go implementations of Want types
var (
	typeImplementationRegistry   = make(map[string]reflect.Type)
	localsImplementationRegistry = make(map[string]reflect.Type)
)

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
		// Case 2: Struct embedding (Want)
		field.Set(reflect.ValueOf(*baseWant))
	}
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
