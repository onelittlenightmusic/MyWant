package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	mywant "mywant/engine/src"

	"gopkg.in/yaml.v3"
)

func (s *Server) validateWant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var buf bytes.Buffer
	io.Copy(&buf, r.Body)
	data := buf.Bytes()

	result := ValidationResult{
		Valid:       true,
		FatalErrors: make([]ValidationError, 0),
		Warnings:    make([]ValidationWarning, 0),
		ValidatedAt: time.Now().Format(time.RFC3339),
	}

	var config mywant.Config
	var parseErr error

	if r.Header.Get("Content-Type") == "application/yaml" || r.Header.Get("Content-Type") == "text/yaml" {
		parseErr = yaml.Unmarshal(data, &config)
	} else {
		parseErr = json.Unmarshal(data, &config)
	}

	if parseErr != nil || len(config.Wants) == 0 {
		var newWant *mywant.Want
		if r.Header.Get("Content-Type") == "application/yaml" || r.Header.Get("Content-Type") == "text/yaml" {
			parseErr = yaml.Unmarshal(data, &newWant)
		} else {
			parseErr = json.Unmarshal(data, &newWant)
		}

		if parseErr != nil || newWant == nil {
			result.Valid = false
			result.FatalErrors = append(result.FatalErrors, ValidationError{
				ErrorType: "syntax",
				Message:   "Invalid YAML/JSON syntax",
				Details:   fmt.Sprintf("%v", parseErr),
			})
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(result)
			return
		}
		config = mywant.Config{Wants: []*mywant.Want{newWant}}
	}

	result.WantCount = len(config.Wants)
	s.collectFatalErrors(&config, &result)

	if result.Valid {
		s.collectWarnings(&config, &result)
	}

	statusCode := http.StatusOK
	if !result.Valid {
		statusCode = http.StatusBadRequest
	}

	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(result)
}

func (s *Server) validateWantTypes(config mywant.Config) error {
	for _, want := range config.Wants {
		wantType := want.Metadata.Type
		hasRecipe := want.Spec.Recipe != ""

		if wantType == "" && !hasRecipe {
			return fmt.Errorf("want '%s' must have either a type or a recipe specified", want.Metadata.Name)
		}

		if hasRecipe && wantType == "" {
			continue
		}
		testWant := &mywant.Want{
			Metadata: mywant.Metadata{
				Name: want.Metadata.Name,
				Type: wantType,
			},
			Spec: mywant.WantSpec{
				Params: make(map[string]any),
			},
		}

		_, err := s.globalBuilder.TestCreateWantFunction(testWant)
		if err != nil {
			return fmt.Errorf("invalid want type '%s' in want '%s': %v", wantType, want.Metadata.Name, err)
		}
	}
	return nil
}

func (s *Server) validateWantSpec(config mywant.Config) error {
	for _, want := range config.Wants {
		for i, selector := range want.Spec.Using {
			for key := range selector {
				if key == "" {
					return fmt.Errorf("want '%s': using[%d] has empty selector key", want.Metadata.Name, i)
				}
			}
		}
		for key := range want.Metadata.Labels {
			if key == "" {
				return fmt.Errorf("want '%s': labels has empty key", want.Metadata.Name)
			}
		}
	}
	return nil
}

func (s *Server) collectFatalErrors(config *mywant.Config, result *ValidationResult) {
	for _, want := range config.Wants {
		wantName := want.Metadata.Name
		if wantName == "" {
			wantName = want.Metadata.Type
		}

		if want.Spec.Recipe == "" {
			if err := s.validateWantType(want); err != nil {
				result.Valid = false
				result.FatalErrors = append(result.FatalErrors, ValidationError{
					WantName:  wantName,
					ErrorType: "want_type",
					Field:     "metadata.type",
					Message:   fmt.Sprintf("Want type '%s' does not exist", want.Metadata.Type),
					Details:   err.Error(),
				})
			}
		}

		if want.Spec.Recipe != "" {
			if err := s.validateRecipeExists(want.Spec.Recipe); err != nil {
				result.Valid = false
				result.FatalErrors = append(result.FatalErrors, ValidationError{
					WantName:  wantName,
					ErrorType: "recipe",
					Field:     "spec.recipe",
					Message:   fmt.Sprintf("Recipe file '%s' does not exist", want.Spec.Recipe),
					Details:   err.Error(),
				})
			}
		}

		if err := s.validateSelectors(want); err != nil {
			result.Valid = false
			result.FatalErrors = append(result.FatalErrors, ValidationError{
				WantName:  wantName,
				ErrorType: "selector",
				Field:     "spec.using or metadata.labels",
				Message:   "Empty keys in selectors or labels",
				Details:   err.Error(),
			})
		}

		if want.Spec.Recipe == "" {
			if errs := s.validateRequiredParameters(want); len(errs) > 0 {
				result.Valid = false
				for _, err := range errs {
					result.FatalErrors = append(result.FatalErrors, err)
				}
			}
		}
	}
}

func (s *Server) validateWantType(want *mywant.Want) error {
	wantType := want.Metadata.Type
	if wantType == "" {
		return fmt.Errorf("want type is empty")
	}
	testWant := &mywant.Want{
		Metadata: mywant.Metadata{Name: want.Metadata.Name, Type: wantType},
		Spec:     mywant.WantSpec{Params: make(map[string]any)},
	}
	_, err := s.globalBuilder.TestCreateWantFunction(testWant)
	return err
}

func (s *Server) validateRecipeExists(recipePath string) error {
	fullPath := recipePath
	if !strings.HasPrefix(recipePath, "/") {
		fullPath = fmt.Sprintf("%s/%s", mywant.RecipesDir, recipePath)
	}
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return fmt.Errorf("recipe file not found: %s", fullPath)
	}
	return nil
}

func (s *Server) validateSelectors(want *mywant.Want) error {
	for i, selector := range want.Spec.Using {
		for key := range selector {
			if key == "" {
				return fmt.Errorf("using[%d] has empty selector key", i)
			}
		}
	}
	for key := range want.Metadata.Labels {
		if key == "" {
			return fmt.Errorf("labels has empty key")
		}
	}
	return nil
}

func (s *Server) validateRequiredParameters(want *mywant.Want) []ValidationError {
	errors := make([]ValidationError, 0)
	if s.wantTypeLoader == nil {
		return errors
	}
	typeDef := s.wantTypeLoader.GetDefinition(want.Metadata.Type)
	if typeDef == nil {
		return errors
	}
	for _, paramDef := range typeDef.Parameters {
		if paramDef.Required {
			if _, exists := want.Spec.Params[paramDef.Name]; !exists {
				errors = append(errors, ValidationError{
					WantName:  want.Metadata.Name,
					ErrorType: "parameter",
					Field:     fmt.Sprintf("spec.params.%s", paramDef.Name),
					Message:   fmt.Sprintf("Required parameter '%s' is missing", paramDef.Name),
					Details:   paramDef.Description,
				})
			}
		}
	}
	return errors
}

func (s *Server) collectWarnings(config *mywant.Config, result *ValidationResult) {
	for _, want := range config.Wants {
		if warnings := s.checkDependencySatisfaction(want); len(warnings) > 0 {
			result.Warnings = append(result.Warnings, warnings...)
		}
		if warnings := s.checkConnectivityRequirements(want); len(warnings) > 0 {
			result.Warnings = append(result.Warnings, warnings...)
		}
		if want.Spec.Recipe == "" {
			if warnings := s.checkParameterTypes(want); len(warnings) > 0 {
				result.Warnings = append(result.Warnings, warnings...)
			}
		}
	}
}

func (s *Server) checkDependencySatisfaction(want *mywant.Want) []ValidationWarning {
	warnings := make([]ValidationWarning, 0)
	if len(want.Spec.Using) == 0 {
		return warnings
	}
	deployedWants := s.globalBuilder.GetAllWantStates()
	for i, selector := range want.Spec.Using {
		matched := false
		for _, deployed := range deployedWants {
			if s.matchesSelector(deployed.Metadata.Labels, selector) {
				matched = true
				break
			}
		}
		if !matched {
			warnings = append(warnings, ValidationWarning{
				WantName:    want.Metadata.Name,
				WarningType: "dependency",
				Field:       fmt.Sprintf("spec.using[%d]", i),
				Message:     fmt.Sprintf("No deployed wants match selector: %v", selector),
				Suggestion:  "Deploy wants with matching labels",
			})
		}
	}
	return warnings
}

func (s *Server) matchesSelector(labels map[string]string, selector map[string]string) bool {
	for key, value := range selector {
		if labels[key] != value {
			return false
		}
	}
	return true
}

func (s *Server) checkConnectivityRequirements(want *mywant.Want) []ValidationWarning {
	warnings := make([]ValidationWarning, 0)
	if s.wantTypeLoader == nil {
		return warnings
	}
	typeDef := s.wantTypeLoader.GetDefinition(want.Metadata.Type)
	if typeDef == nil || typeDef.Require == nil {
		return warnings
	}
	connMeta := typeDef.Require.ToConnectivityMetadata(want.Metadata.Type)
	deployedWants := s.globalBuilder.GetAllWantStates()

	inputCount := 0
	for _, selector := range want.Spec.Using {
		for _, deployed := range deployedWants {
			if s.matchesSelector(deployed.Metadata.Labels, selector) {
				inputCount++
				break
			}
		}
	}

	outputCount := 0
	for _, deployed := range deployedWants {
		for _, deployedSelector := range deployed.Spec.Using {
			if s.matchesSelector(want.Metadata.Labels, deployedSelector) {
				outputCount++
				break
			}
		}
	}

	if connMeta.RequiredInputs > 0 && inputCount < connMeta.RequiredInputs {
		warnings = append(warnings, ValidationWarning{
			WantName:    want.Metadata.Name,
			WarningType: "connectivity",
			Field:       "spec.using",
			Message:     fmt.Sprintf("Requires %d input connection(s), but may only have %d", connMeta.RequiredInputs, inputCount),
		})
	}
	if connMeta.RequiredOutputs > 0 && outputCount < connMeta.RequiredOutputs {
		warnings = append(warnings, ValidationWarning{
			WantName:    want.Metadata.Name,
			WarningType: "connectivity",
			Field:       "metadata.labels",
			Message:     fmt.Sprintf("Requires %d output connection(s), but may only have %d", connMeta.RequiredOutputs, outputCount),
		})
	}
	return warnings
}

func (s *Server) checkParameterTypes(want *mywant.Want) []ValidationWarning {
	warnings := make([]ValidationWarning, 0)
	if s.wantTypeLoader == nil {
		return warnings
	}
	typeDef := s.wantTypeLoader.GetDefinition(want.Metadata.Type)
	if typeDef == nil {
		return warnings
	}
	paramDefs := make(map[string]*mywant.ParameterDef)
	for i := range typeDef.Parameters {
		paramDefs[typeDef.Parameters[i].Name] = &typeDef.Parameters[i]
	}
	for paramName, paramValue := range want.Spec.Params {
		paramDef, exists := paramDefs[paramName]
		if !exists {
			continue
		}
		expectedType := paramDef.Type
		actualType := s.getGoType(paramValue)
		if !s.isTypeCompatible(expectedType, actualType) {
			warnings = append(warnings, ValidationWarning{
				WantName:    want.Metadata.Name,
				WarningType: "parameter_type",
				Field:       fmt.Sprintf("spec.params.%s", paramName),
				Message:     fmt.Sprintf("Parameter type mismatch: expected %s, got %s", expectedType, actualType),
			})
		}
	}
	return warnings
}

func (s *Server) getGoType(value any) string {
	if value == nil {
		return "nil"
	}
	switch value.(type) {
	case bool:
		return "bool"
	case float64:
		return "float64"
	case string:
		return "string"
	case []any:
		return "[]any"
	case map[string]any:
		return "map[string]any"
	default:
		return fmt.Sprintf("%T", value)
	}
}

func (s *Server) isTypeCompatible(expected, actual string) bool {
	if expected == actual {
		return true
	}
	numericTypes := map[string]bool{"int": true, "int64": true, "float64": true}
	if numericTypes[expected] && numericTypes[actual] {
		return true
	}
	if expected == "int" && actual == "float64" {
		return true
	}
	return false
}
