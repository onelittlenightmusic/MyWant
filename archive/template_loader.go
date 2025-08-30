package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/template"
	"bytes"
	"strconv"
	"gopkg.in/yaml.v3"
)


// TemplateParameter defines a configurable parameter for templates
type TemplateParameter struct {
	Name        string      `yaml:"name"`
	Type        string      `yaml:"type"`
	Default     interface{} `yaml:"default"`
	Description string      `yaml:"description"`
}

// WantTemplate defines a template for creating child wants (legacy format)
type WantTemplate struct {
	Metadata struct {
		Name   string            `yaml:"name"`
		Type   string            `yaml:"type"`
		Labels map[string]string `yaml:"labels"`
	} `yaml:"metadata"`
	Spec struct {
		Params map[string]interface{}   `yaml:"params"`
		Using []map[string]string      `yaml:"using,omitempty"`
	} `yaml:"spec"`
	// Store type tag information separately
	TypeHints map[string]string `yaml:"-"` // param_name -> type_tag
}

// DRYWantSpec defines minimal want specification in DRY format
type DRYWantSpec struct {
	Name   string                    `yaml:"name"`
	Type   string                    `yaml:"type"`
	Labels map[string]string         `yaml:"labels,omitempty"`
	Params map[string]interface{}    `yaml:"params,omitempty"`
	Using []map[string]string       `yaml:"using,omitempty"`
	// Store type tag information separately
	TypeHints map[string]string `yaml:"-"` // param_name -> type_tag
}

// DRYTemplateDefaults defines common defaults for all wants in a template
type DRYTemplateDefaults struct {
	Metadata struct {
		Labels map[string]string `yaml:"labels,omitempty"`
	} `yaml:"metadata,omitempty"`
	Spec struct {
		Params map[string]interface{} `yaml:"params,omitempty"`
	} `yaml:"spec,omitempty"`
}

// TemplateResult defines how to fetch a result from child wants
type TemplateResult struct {
	Want     string `yaml:"want"`     // Name pattern or label selector for the child want
	StatName string `yaml:"statName"` // Name of the statistic to fetch (e.g., "AverageWaitTime", "TotalProcessed")
}

// ChildTemplate defines a complete template for creating child wants
type ChildTemplate struct {
	Description string              `yaml:"description"`
	
	// Legacy parameter format support
	Parameters  []TemplateParameter `yaml:"parameters,omitempty"`
	
	// New minimized parameter format support
	Params      map[string]interface{} `yaml:"params,omitempty"`
	
	Result      *TemplateResult     `yaml:"result,omitempty"` // Optional result fetching configuration
	
	// Legacy format support
	Children    []WantTemplate      `yaml:"children,omitempty"`
	
	// New DRY format support  
	Defaults    *DRYTemplateDefaults `yaml:"defaults,omitempty"`
	Wants       []DRYWantSpec        `yaml:"wants,omitempty"`
}

// TemplateConfig holds all available templates
type TemplateConfig struct {
	Templates map[string]ChildTemplate `yaml:"templates"`
}

// TemplateLoader manages loading and instantiating want templates
type TemplateLoader struct {
	templates   map[string]ChildTemplate
	templateDir string
}

// NewTemplateLoader creates a new template loader
func NewTemplateLoader(templateDir string) *TemplateLoader {
	if templateDir == "" {
		templateDir = "templates"
	}
	return &TemplateLoader{
		templates:   make(map[string]ChildTemplate),
		templateDir: templateDir,
	}
}

// LoadTemplates loads all template files from the template directory
func (tl *TemplateLoader) LoadTemplates() error {
	if _, err := os.Stat(tl.templateDir); os.IsNotExist(err) {
		fmt.Printf("[TEMPLATE] Template directory %s does not exist, using hardcoded templates\n", tl.templateDir)
		return tl.loadDefaultTemplates()
	}

	fmt.Printf("[TEMPLATE] Loading templates from directory: %s\n", tl.templateDir)
	err := filepath.Walk(tl.templateDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if filepath.Ext(path) == ".yaml" || filepath.Ext(path) == ".yml" {
			fmt.Printf("[TEMPLATE] Loading template file: %s\n", path)
			return tl.loadTemplateFile(path)
		}
		return nil
	})
	
	// Show final template count
	fmt.Printf("[TEMPLATE] Total templates loaded: %d\n", len(tl.templates))
	for name := range tl.templates {
		fmt.Printf("[TEMPLATE] Available template: %s\n", name)
	}
	
	return err
}

// loadTemplateFile loads a single template file with type tag preservation
func (tl *TemplateLoader) loadTemplateFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read template file %s: %w", filename, err)
	}

	// Parse with low-level YAML to preserve type tags
	var rootNode yaml.Node
	if err := yaml.Unmarshal(data, &rootNode); err != nil {
		return fmt.Errorf("failed to parse template file %s: %w", filename, err)
	}

	// Convert to TemplateConfig while preserving type information
	config, err := tl.parseTemplateConfigWithTypeTags(&rootNode)
	if err != nil {
		return fmt.Errorf("failed to process template config %s: %w", filename, err)
	}

	for name, template := range config.Templates {
		tl.templates[name] = template
		fmt.Printf("[TEMPLATE] Loaded template: %s\n", name)
		
		// Debug: Show first child template params
		if len(template.Children) > 0 {
			fmt.Printf("[TEMPLATE-PARAMS] First child params: %+v\n", template.Children[0].Spec.Params)
		}
	}

	return nil
}

// parseTemplateConfigWithTypeTags parses TemplateConfig while preserving type tag information
func (tl *TemplateLoader) parseTemplateConfigWithTypeTags(rootNode *yaml.Node) (TemplateConfig, error) {
	// First extract type hints from the YAML structure
	typeHints := make(map[string]map[string]string) // template_name -> param_name -> type_tag
	tl.extractTypeHints(rootNode, typeHints)
	
	// Decode to TemplateConfig struct (this loses type tags but gets the structure)
	var config TemplateConfig
	if err := rootNode.Decode(&config); err != nil {
		return TemplateConfig{}, err
	}
	
	// Apply the extracted type hints to both legacy and DRY templates
	for templateName, template := range config.Templates {
		// Handle legacy Children templates
		for i := range template.Children {
			if config.Templates[templateName].Children[i].TypeHints == nil {
				config.Templates[templateName].Children[i].TypeHints = make(map[string]string)
			}
			
			// Copy global type hints (we use global for simplicity)
			if globalHints, exists := typeHints["global"]; exists {
				for paramName, typeTag := range globalHints {
					config.Templates[templateName].Children[i].TypeHints[paramName] = typeTag
				}
			}
		}
		
		// Handle DRY Wants templates
		for i := range template.Wants {
			if config.Templates[templateName].Wants[i].TypeHints == nil {
				config.Templates[templateName].Wants[i].TypeHints = make(map[string]string)
			}
			
			// Copy global type hints (we use global for simplicity)
			if globalHints, exists := typeHints["global"]; exists {
				for paramName, typeTag := range globalHints {
					config.Templates[templateName].Wants[i].TypeHints[paramName] = typeTag
				}
			}
		}
	}

	return config, nil
}

// extractTypeHints recursively extracts type tag information from YAML nodes
func (tl *TemplateLoader) extractTypeHints(node *yaml.Node, typeHints map[string]map[string]string) {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			tl.extractTypeHints(child, typeHints)
		}
	case yaml.MappingNode:
		// Check if this looks like a template parameters section
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]
			
			// Look for parameter wants with type tags
			if key.Value == "params" && value.Kind == yaml.MappingNode {
				tl.extractParamTypeHints(value, typeHints)
			} else {
				tl.extractTypeHints(value, typeHints)
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			tl.extractTypeHints(child, typeHints)
		}
	}
}

// extractParamTypeHints extracts type hints from a params section
func (tl *TemplateLoader) extractParamTypeHints(paramsNode *yaml.Node, typeHints map[string]map[string]string) {
	for i := 0; i < len(paramsNode.Content); i += 2 {
		paramName := paramsNode.Content[i].Value
		paramValue := paramsNode.Content[i+1]
		
		// Check if this parameter has a type tag
		if paramValue.Tag != "" && (paramValue.Tag == "!int" || paramValue.Tag == "!float" || paramValue.Tag == "!bool") {
			fmt.Printf("[TYPE-HINT] Found %s with type %s\n", paramName, paramValue.Tag)
			// Store the type hint - we'll need to associate it with the right template
			// For now, store globally (we can refine this later)
			if typeHints["global"] == nil {
				typeHints["global"] = make(map[string]string)
			}
			typeHints["global"][paramName] = paramValue.Tag
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// loadDefaultTemplates provides fallback hardcoded templates
func (tl *TemplateLoader) loadDefaultTemplates() error {
	defaultTemplate := ChildTemplate{
		Description: "Default number processing pipeline",
		Parameters: []TemplateParameter{
			{Name: "count", Type: "int", Default: 1000, Description: "Number of items to generate"},
			{Name: "rate", Type: "float", Default: 10.0, Description: "Generation rate per second"},
			{Name: "service_time", Type: "float", Default: 0.1, Description: "Queue processing time"},
		},
		Result: &TemplateResult{
			Want:     "{{.targetName}}-queue",
			StatName: "AverageWaitTime",
		},
		Children: []WantTemplate{
			{
				Metadata: struct {
					Name   string            `yaml:"name"`
					Type   string            `yaml:"type"`
					Labels map[string]string `yaml:"labels"`
				}{
					Name: "{{.targetName}}-generator",
					Type: "sequence",
					Labels: map[string]string{
						"role":     "source",
						"owner":    "child",
						"category": "producer",
					},
				},
				Spec: struct {
					Params map[string]interface{} `yaml:"params"`
					Using []map[string]string    `yaml:"using,omitempty"`
				}{
					Params: map[string]interface{}{
						"count": "{{.count}}",
						"rate":  "{{.rate}}",
					},
				},
			},
			{
				Metadata: struct {
					Name   string            `yaml:"name"`
					Type   string            `yaml:"type"`
					Labels map[string]string `yaml:"labels"`
				}{
					Name: "{{.targetName}}-queue",
					Type: "queue",
					Labels: map[string]string{
						"role":     "processor",
						"owner":    "child",
						"category": "queue",
					},
				},
				Spec: struct {
					Params map[string]interface{} `yaml:"params"`
					Using []map[string]string    `yaml:"using,omitempty"`
				}{
					Params: map[string]interface{}{
						"service_time": "{{.service_time}}",
					},
					Using: []map[string]string{
						{"category": "producer"},
					},
				},
			},
			{
				Metadata: struct {
					Name   string            `yaml:"name"`
					Type   string            `yaml:"type"`
					Labels map[string]string `yaml:"labels"`
				}{
					Name: "{{.targetName}}-sink",
					Type: "sink",
					Labels: map[string]string{
						"role":     "collector",
						"category": "display",
					},
				},
				Spec: struct {
					Params map[string]interface{} `yaml:"params"`
					Using []map[string]string    `yaml:"using,omitempty"`
				}{
					Params: map[string]interface{}{
						"display_format": "Number: %d",
					},
					Using: []map[string]string{
						{"role": "processor"},
					},
				},
			},
		},
	}

	tl.templates["wait time in queue system"] = defaultTemplate
	fmt.Printf("[TEMPLATE] Loaded default template: wait time in queue system\n")
	return nil
}

// GetTemplate returns a template by name
func (tl *TemplateLoader) GetTemplate(name string) (ChildTemplate, error) {
	template, exists := tl.templates[name]
	if !exists {
		return ChildTemplate{}, fmt.Errorf("template %s not found", name)
	}
	fmt.Printf("[TEMPLATE-SOURCE] Using template '%s' from: %v\n", name, len(template.Children))
	return template, nil
}

// ListTemplates returns all available template names
func (tl *TemplateLoader) ListTemplates() []string {
	names := make([]string, 0, len(tl.templates))
	for name := range tl.templates {
		names = append(names, name)
	}
	return names
}

// InstantiateTemplate creates actual Want instances from a template
func (tl *TemplateLoader) InstantiateTemplate(templateName string, targetName string, params map[string]interface{}) ([]Want, error) {
	childTemplate, err := tl.GetTemplate(templateName)
	if err != nil {
		return nil, err
	}

	// Merge default parameters with provided parameters
	templateParams := make(map[string]interface{})
	templateParams["targetName"] = targetName

	// Set default values - support both legacy and new parameter formats
	if len(childTemplate.Parameters) > 0 {
		// Legacy format: parameters array
		for _, param := range childTemplate.Parameters {
			templateParams[param.Name] = param.Default
		}
	} else if childTemplate.Params != nil {
		// New format: params map
		for paramName, defaultValue := range childTemplate.Params {
			templateParams[paramName] = defaultValue
		}
	}

	// Override with provided parameters
	for key, value := range params {
		templateParams[key] = value
	}

	var wants []Want
	
	// Check if this is a DRY template (has Wants field) or legacy template (has Children field)
	if len(childTemplate.Wants) > 0 {
		// New DRY template format
		for _, dryWantSpec := range childTemplate.Wants {
			want, err := tl.instantiateDRYWant(dryWantSpec, childTemplate.Defaults, templateParams, targetName)
			if err != nil {
				return nil, fmt.Errorf("failed to instantiate DRY want template: %w", err)
			}
			wants = append(wants, want)
		}
	} else {
		// Legacy template format
		for _, wantTemplate := range childTemplate.Children {
			want, err := tl.instantiateWantFromTemplate(wantTemplate, templateParams, targetName)
			if err != nil {
				return nil, fmt.Errorf("failed to instantiate want template: %w", err)
			}
			wants = append(wants, want)
		}
	}

	return wants, nil
}

// instantiateDRYWant creates a single Want from a DRY want spec merged with defaults
func (tl *TemplateLoader) instantiateDRYWant(dryWant DRYWantSpec, defaults *DRYTemplateDefaults, params map[string]interface{}, targetName string) (Want, error) {
	// Merge defaults with want-specific values to create a complete WantTemplate
	mergedTemplate := tl.mergeDRYDefaults(dryWant, defaults, targetName)
	
	// Now use the existing instantiation logic
	return tl.instantiateWantFromTemplate(mergedTemplate, params, targetName)
}

// mergeDRYDefaults merges DRY template defaults with individual want specifications
func (tl *TemplateLoader) mergeDRYDefaults(dryWant DRYWantSpec, defaults *DRYTemplateDefaults, targetName string) WantTemplate {
	// Create a complete WantTemplate by merging defaults with the DRY want spec
	wantTemplate := WantTemplate{
		Metadata: struct {
			Name   string            `yaml:"name"`
			Type   string            `yaml:"type"`
			Labels map[string]string `yaml:"labels"`
		}{
			Name: dryWant.Name,
			Type: dryWant.Type,
			Labels: make(map[string]string),
		},
		Spec: struct {
			Params map[string]interface{}   `yaml:"params"`
			Using []map[string]string      `yaml:"using,omitempty"`
		}{
			Params: make(map[string]interface{}),
			Using: dryWant.Using, // Copy using directly
		},
		TypeHints: make(map[string]string),
	}
	
	// Merge default labels first, then override with node-specific labels
	if defaults != nil && defaults.Metadata.Labels != nil {
		for key, value := range defaults.Metadata.Labels {
			wantTemplate.Metadata.Labels[key] = value
		}
	}
	
	// Override with node-specific labels
	if dryWant.Labels != nil {
		for key, value := range dryWant.Labels {
			wantTemplate.Metadata.Labels[key] = value
		}
	}
	
	// Merge default params first, then override with node-specific params
	if defaults != nil && defaults.Spec.Params != nil {
		for key, value := range defaults.Spec.Params {
			wantTemplate.Spec.Params[key] = value
		}
	}
	
	// Override with node-specific params
	if dryWant.Params != nil {
		for key, value := range dryWant.Params {
			wantTemplate.Spec.Params[key] = value
		}
	}
	
	// Copy type hints from DRY node
	if dryWant.TypeHints != nil {
		for key, value := range dryWant.TypeHints {
			wantTemplate.TypeHints[key] = value
		}
	}
	
	fmt.Printf("[DRY-MERGE] Merged want '%s' with defaults, final params: %+v\n", dryWant.Name, wantTemplate.Spec.Params)
	
	return wantTemplate
}

// instantiateWantFromTemplate creates a single Want from a WantTemplate with type tag support
func (tl *TemplateLoader) instantiateWantFromTemplate(wantTemplate WantTemplate, params map[string]interface{}, targetName string) (Want, error) {
	// Convert template to YAML for processing
	templateBytes, err := yaml.Marshal(wantTemplate)
	if err != nil {
		return Want{}, fmt.Errorf("failed to marshal want template: %w", err)
	}
	
	fmt.Printf("[TEMPLATE-DEBUG] Raw template YAML:\n%s\n", string(templateBytes))

	// Apply template parameters using Go text/template
	tmpl, err := template.New("node").Parse(string(templateBytes))
	if err != nil {
		return Want{}, fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return Want{}, fmt.Errorf("failed to execute template: %w", err)
	}

	// Parse with type hints from the original template
	instantiatedTemplate, err := tl.parseTemplateWithTypeHints(buf.Bytes(), wantTemplate.TypeHints)
	if err != nil {
		return Want{}, fmt.Errorf("failed to parse instantiated template: %w", err)
	}

	// Create the actual Want with owner references
	want := Want{
		Metadata: Metadata{
			Name:   instantiatedTemplate.Metadata.Name,
			Type:   instantiatedTemplate.Metadata.Type,
			Labels: instantiatedTemplate.Metadata.Labels,
			OwnerReferences: []OwnerReference{
				{
					APIVersion:         "gochain/v1",
					Kind:               "Want",
					Name:               targetName,
					Controller:         true,
					BlockOwnerDeletion: true,
				},
			},
		},
		Spec: WantSpec{
			Params: instantiatedTemplate.Spec.Params, // Now contains properly typed values!
			Using: instantiatedTemplate.Spec.Using,
		},
		Stats:  WantStats{},
		Status: WantStatusIdle,
		State:  make(map[string]interface{}),
	}

	return want, nil
}

// parseTemplateWithTypeHints parses YAML and applies type conversion based on stored hints
func (tl *TemplateLoader) parseTemplateWithTypeHints(data []byte, typeHints map[string]string) (WantTemplate, error) {
	// First parse normally
	var wantTemplate WantTemplate
	if err := yaml.Unmarshal(data, &wantTemplate); err != nil {
		return WantTemplate{}, err
	}

	// Apply type conversions based on hints
	if typeHints != nil {
		tl.applyTypeConversions(wantTemplate.Spec.Params, typeHints)
	}

	return wantTemplate, nil
}

// applyTypeConversions converts parameter values based on type hints
func (tl *TemplateLoader) applyTypeConversions(params map[string]interface{}, typeHints map[string]string) {
	for paramName, value := range params {
		if typeHint, exists := typeHints[paramName]; exists {
			if strValue, ok := value.(string); ok {
				converted, err := tl.convertWithTypeHint(strValue, typeHint)
				if err != nil {
					fmt.Printf("[TYPE-CONVERSION] Failed to convert %s=%s to %s: %v\n", paramName, strValue, typeHint, err)
				} else {
					fmt.Printf("[TYPE-CONVERSION] Converted %s: %s (%s) -> %v (%T)\n", paramName, strValue, typeHint, converted, converted)
					params[paramName] = converted
				}
			}
		}
	}
}

// convertWithTypeHint converts a string value based on the type hint
func (tl *TemplateLoader) convertWithTypeHint(value, typeHint string) (interface{}, error) {
	switch typeHint {
	case "!int":
		return strconv.Atoi(value)
	case "!float":
		return strconv.ParseFloat(value, 64)
	case "!bool":
		return strconv.ParseBool(value)
	default:
		return value, nil
	}
}

// processYAMLNodeForTypes recursively processes YAML nodes to resolve type tags
func (tl *TemplateLoader) processYAMLNodeForTypes(node *yaml.Node) error {
	switch node.Kind {
	case yaml.DocumentNode:
		// Process document content
		for _, child := range node.Content {
			if err := tl.processYAMLNodeForTypes(child); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		// Process key-value pairs
		for i := 0; i < len(node.Content); i += 2 {
			value := node.Content[i+1]
			
			// Process the value node (which may have type tags)
			if err := tl.processYAMLNodeForTypes(value); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		// Process array elements
		for _, child := range node.Content {
			if err := tl.processYAMLNodeForTypes(child); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		// Handle type-tagged scalar values
		if err := tl.resolveScalarTypeTag(node); err != nil {
			return err
		}
	}
	return nil
}

// resolveScalarTypeTag converts type-tagged values to their proper types
func (tl *TemplateLoader) resolveScalarTypeTag(node *yaml.Node) error {
	switch node.Tag {
	case "!int":
		if value, err := strconv.Atoi(node.Value); err == nil {
			fmt.Printf("[TYPE-TAG] Converting !int '%s' to %d\n", node.Value, value)
			node.Tag = "tag:yaml.org,2002:int"
			node.Value = fmt.Sprintf("%d", value)
		} else {
			return fmt.Errorf("cannot convert '%s' to int: %w", node.Value, err)
		}
	case "!float":
		if value, err := strconv.ParseFloat(node.Value, 64); err == nil {
			fmt.Printf("[TYPE-TAG] Converting !float '%s' to %g\n", node.Value, value)
			node.Tag = "tag:yaml.org,2002:float"
			node.Value = fmt.Sprintf("%g", value)
		} else {
			return fmt.Errorf("cannot convert '%s' to float: %w", node.Value, err)
		}
	case "!bool":
		if value, err := strconv.ParseBool(node.Value); err == nil {
			fmt.Printf("[TYPE-TAG] Converting !bool '%s' to %t\n", node.Value, value)
			node.Tag = "tag:yaml.org,2002:bool"
			node.Value = fmt.Sprintf("%t", value)
		} else {
			return fmt.Errorf("cannot convert '%s' to bool: %w", node.Value, err)
		}
	}
	return nil
}

// GetTemplateResult fetches a result value from child nodes based on template configuration
func (tl *TemplateLoader) GetTemplateResult(templateName string, targetName string, wants []Want) (interface{}, error) {
	childTemplate, err := tl.GetTemplate(templateName)
	if err != nil {
		return nil, err
	}

	if childTemplate.Result == nil {
		return nil, fmt.Errorf("template %s does not define a result configuration", templateName)
	}

	// Find the target want based on the result configuration
	var targetWant *Want
	for i := range wants {
		want := &wants[i]
		
		// Check if this want matches the result configuration
		if tl.matchesResultWant(want, childTemplate.Result.Want, targetName) {
			targetWant = want
			break
		}
	}

	if targetWant == nil {
		return nil, fmt.Errorf("no want found matching result selector '%s' for template %s", childTemplate.Result.Want, templateName)
	}

	// Extract the requested statistic from the want
	return tl.extractWantStat(targetWant, childTemplate.Result.StatName)
}

// matchesResultWant checks if a want matches the result want selector
func (tl *TemplateLoader) matchesResultWant(want *Want, wantSelector string, targetName string) bool {
	// Replace template variables in the selector
	tmpl, err := template.New("selector").Parse(wantSelector)
	if err != nil {
		return false
	}
	
	params := map[string]interface{}{
		"targetName": targetName,
	}
	
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return false
	}
	
	resolvedSelector := buf.String()
	
	// Check if it matches the want name exactly
	if want.Metadata.Name == resolvedSelector {
		return true
	}
	
	// Check if it matches based on labels (category, role, etc.)
	for key, value := range want.Metadata.Labels {
		if key == resolvedSelector || value == resolvedSelector {
			return true
		}
	}
	
	return false
}

// extractWantStat extracts a specific statistic from a want
func (tl *TemplateLoader) extractWantStat(want *Want, statName string) (interface{}, error) {
	switch statName {
	case "AverageWaitTime", "averagewaittime":
		return want.Stats.AverageWaitTime, nil
	case "TotalProcessed", "totalprocessed":
		return want.Stats.TotalProcessed, nil
	case "TotalWaitTime", "totalwaittime":
		return want.Stats.TotalWaitTime, nil
	default:
		return nil, fmt.Errorf("unknown stat name: %s", statName)
	}
}