package mywant

import "fmt"

// want_error.go — config/module error handling, governance warnings, and param validation

// ModuleErrorPanic is a sentinel type used to immediately terminate Progress() execution
// When SetModuleErrorAndExit() is called, it panics with this type
// StartProgressionLoop() recovers from this panic and handles cleanup gracefully
type ModuleErrorPanic struct {
	Component string
	Message   string
}

func (e ModuleErrorPanic) Error() string {
	return fmt.Sprintf("module error [%s]: %s", e.Component, e.Message)
}

// ConfigErrorPanic is a sentinel type used to immediately terminate Progress() execution
// When SetConfigErrorAndExit() is called, it panics with this type
type ConfigErrorPanic struct {
	Field   string
	Message string
}

func (e ConfigErrorPanic) Error() string {
	return fmt.Sprintf("config error [%s]: %s", e.Field, e.Message)
}

// SetConfigError marks the want as having a configuration error
// ConfigError means the input values or spec are invalid and processing cannot continue
// The want can be recovered by updating the configuration (params/spec)
// Usage in Initialize():
//
//	if locals.Topic == "" {
//	    return w.SetConfigError("topic", "Missing required parameter 'topic'")
//	}
func (w *Want) SetConfigError(field string, message string) error {
	w.storeStateMulti(map[string]any{
		"config_error_field":   field,
		"config_error_message": message,
		"error":                message,
	})
	w.StoreLog("CONFIG_ERROR: %s - %s", field, message)
	w.SetStatus(WantStatusConfigError)
	return fmt.Errorf("config error [%s]: %s", field, message)
}

// setGovernanceWarning records a governance or label access violation as a warning.
// The want continues running — governance rules are best-effort, not enforced hard stops.
// Status transitions: reaching → reaching_with_warning, achieved → achieved_with_warning.
func (w *Want) setGovernanceWarning(component string, message string) {
	w.storeStateMulti(map[string]any{
		"governance_warning_component": component,
		"governance_warning_message":   message,
	})
	w.StoreLog("[GOVERNANCE_WARNING] %s - %s", component, message)
	switch w.Status {
	case WantStatusReaching, WantStatusReachingWithWarning:
		w.SetStatus(WantStatusReachingWithWarning)
	case WantStatusAchieved, WantStatusAchievedWithWarning:
		w.SetStatus(WantStatusAchievedWithWarning)
	}
}

// SetModuleError marks the want as having a module/implementation error
// ModuleError means there's an issue with the want type implementation itself
// (e.g., GetState failure, type cast failure, nil pointer dereference in framework code)
// This typically requires code changes to fix, not configuration changes
// Usage in Progress():
//
//	if locals == nil {
//	    return w.SetModuleError("GetLocals", "Failed to access type-specific locals")
//	}
func (w *Want) SetModuleError(component string, message string) error {
	w.storeStateMulti(map[string]any{
		"module_error_component": component,
		"module_error_message":   message,
		"error":                  message,
	})
	w.StoreLog("MODULE_ERROR: %s - %s", component, message)
	w.SetStatus(WantStatusModuleError)
	return fmt.Errorf("module error [%s]: %s", component, message)
}

// SetModuleErrorAndExit sets module error and immediately terminates Progress() execution
// This uses panic/recover to exit the goroutine cleanly from deep call stacks
// StartProgressionLoop() will recover from this panic and handle cleanup
// Use this when you want to immediately stop execution without returning through the call stack
//
// Usage in Progress():
//
//	locals := w.GetLocals()
//	if locals == nil {
//	    w.SetModuleErrorAndExit("Locals", "Failed to access type-specific locals")
//	    // Code after this line will NOT execute
//	}
func (w *Want) SetModuleErrorAndExit(component string, message string) {
	w.storeStateMulti(map[string]any{
		"module_error_component": component,
		"module_error_message":   message,
		"error":                  message,
	})
	w.StoreLog("MODULE_ERROR: %s - %s (exiting immediately)", component, message)
	w.SetStatus(WantStatusModuleError)
	panic(ModuleErrorPanic{Component: component, Message: message})
}

// SetConfigErrorAndExit sets config error and immediately terminates Progress() execution
// Similar to SetModuleErrorAndExit but for configuration errors
// StartProgressionLoop() will recover and keep the want in ConfigError state waiting for config update
func (w *Want) SetConfigErrorAndExit(field string, message string) {
	w.storeStateMulti(map[string]any{
		"config_error_field":   field,
		"config_error_message": message,
		"error":                message,
	})
	w.StoreLog("CONFIG_ERROR: %s - %s (exiting immediately)", field, message)
	w.SetStatus(WantStatusConfigError)
	panic(ConfigErrorPanic{Field: field, Message: message})
}

// ClearConfigError clears config error state and transitions to Idle
// Called when user updates the configuration that caused the error
func (w *Want) ClearConfigError() {
	if w.Status != WantStatusConfigError {
		return
	}

	// Clear error-related state
	w.storeStateMulti(map[string]any{
		"config_error_field":   nil,
		"config_error_message": nil,
		"error":                nil,
	})

	// Transition back to idle for re-execution
	w.SetStatus(WantStatusIdle)
	w.StoreLog("Config error cleared, transitioning to idle")
}

// IsRecoverableError returns true if the current status is a recoverable error state
// ConfigError is recoverable (by updating config), ModuleError is not
func (w *Want) IsRecoverableError() bool {
	return w.Status == WantStatusConfigError
}

// IsErrorState returns true if the want is in any error state
func (w *Want) IsErrorState() bool {
	return w.Status == WantStatusConfigError ||
		w.Status == WantStatusModuleError ||
		w.Status == WantStatusFailed
}

// ValidateRequiredParams validates that required parameters are present and returns error if any are missing
// Returns nil if all required params are present, otherwise calls SetConfigError and returns error
// Usage in Initialize():
//
//	if err := w.ValidateRequiredParams("topic", "output_path"); err != nil {
//	    return // Want status already set to ConfigError
//	}
func (w *Want) ValidateRequiredParams(paramNames ...string) error {
	for _, paramName := range paramNames {
		value, exists := w.Spec.Params[paramName]
		if !exists {
			return w.SetConfigError(paramName, fmt.Sprintf("Missing required parameter '%s'", paramName))
		}
		// Check for empty string values
		if strVal, ok := value.(string); ok && strVal == "" {
			return w.SetConfigError(paramName, fmt.Sprintf("Required parameter '%s' cannot be empty", paramName))
		}
	}
	return nil
}

// ValidateParamFormat validates a parameter against a validation function
// Usage in Initialize():
//
//	if err := w.ValidateParamFormat("event_time", func(v any) error {
//	    if str, ok := v.(string); ok {
//	        _, err := time.Parse(time.RFC3339, str)
//	        return err
//	    }
//	    return fmt.Errorf("must be a string")
//	}); err != nil {
//	    return
//	}
func (w *Want) ValidateParamFormat(paramName string, validator func(any) error) error {
	value, exists := w.Spec.Params[paramName]
	if !exists {
		return nil // Skip validation for non-existent params (use ValidateRequiredParams for required check)
	}
	if err := validator(value); err != nil {
		return w.SetConfigError(paramName, fmt.Sprintf("Invalid format for parameter '%s': %v", paramName, err))
	}
	return nil
}

// CheckLocalsInitialized checks if Locals is properly initialized and immediately
// terminates the goroutine via panic if not. The panic is recovered by
// StartProgressionLoop() which handles cleanup gracefully.
// Usage in Progress():
//
//	locals := CheckLocalsInitialized[MyLocals](w)
//	// No nil check needed - if Locals is invalid, execution stops immediately
func CheckLocalsInitialized[T any](w *Want) *T {
	if w.Locals == nil {
		w.SetModuleErrorAndExit("Locals", "Locals not initialized - Initialize() may not have been called")
	}
	locals, ok := w.Locals.(*T)
	if !ok {
		w.SetModuleErrorAndExit("Locals", fmt.Sprintf("Failed to cast Locals to expected type %T", (*T)(nil)))
	}
	return locals
}
