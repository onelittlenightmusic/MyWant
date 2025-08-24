package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/template"
	"bytes"
	"gopkg.in/yaml.v3"
)

// TemplateParameter defines a configurable parameter for templates
type TemplateParameter struct {
	Name        string      `yaml:"name"`
	Type        string      `yaml:"type"`
	Default     interface{} `yaml:"default"`
	Description string      `yaml:"description"`
}

// NodeTemplate defines a template for creating child nodes
type NodeTemplate struct {
	Metadata struct {
		Name   string            `yaml:"name"`
		Type   string            `yaml:"type"`
		Labels map[string]string `yaml:"labels"`
	} `yaml:"metadata"`
	Spec struct {
		Params map[string]interface{}   `yaml:"params"`
		Inputs []map[string]string      `yaml:"inputs,omitempty"`
	} `yaml:"spec"`
}

// ChildTemplate defines a complete template for creating child nodes
type ChildTemplate struct {
	Description string              `yaml:"description"`
	Parameters  []TemplateParameter `yaml:"parameters"`
	Children    []NodeTemplate      `yaml:"children"`
}

// TemplateConfig holds all available templates
type TemplateConfig struct {
	Templates map[string]ChildTemplate `yaml:"templates"`
}

// TemplateLoader manages loading and instantiating node templates
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

	return filepath.Walk(tl.templateDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if filepath.Ext(path) == ".yaml" || filepath.Ext(path) == ".yml" {
			return tl.loadTemplateFile(path)
		}
		return nil
	})
}

// loadTemplateFile loads a single template file
func (tl *TemplateLoader) loadTemplateFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read template file %s: %w", filename, err)
	}

	var config TemplateConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse template file %s: %w", filename, err)
	}

	for name, template := range config.Templates {
		tl.templates[name] = template
		fmt.Printf("[TEMPLATE] Loaded template: %s\n", name)
	}

	return nil
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
		Children: []NodeTemplate{
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
					Inputs []map[string]string    `yaml:"inputs,omitempty"`
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
					Inputs []map[string]string    `yaml:"inputs,omitempty"`
				}{
					Params: map[string]interface{}{
						"service_time": "{{.service_time}}",
					},
					Inputs: []map[string]string{
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
					Inputs []map[string]string    `yaml:"inputs,omitempty"`
				}{
					Params: map[string]interface{}{
						"display_format": "Number: %d",
					},
					Inputs: []map[string]string{
						{"role": "processor"},
					},
				},
			},
		},
	}

	tl.templates["number-processing-pipeline"] = defaultTemplate
	fmt.Printf("[TEMPLATE] Loaded default template: number-processing-pipeline\n")
	return nil
}

// GetTemplate returns a template by name
func (tl *TemplateLoader) GetTemplate(name string) (ChildTemplate, error) {
	template, exists := tl.templates[name]
	if !exists {
		return ChildTemplate{}, fmt.Errorf("template %s not found", name)
	}
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

// InstantiateTemplate creates actual Node instances from a template
func (tl *TemplateLoader) InstantiateTemplate(templateName string, targetName string, params map[string]interface{}) ([]Node, error) {
	childTemplate, err := tl.GetTemplate(templateName)
	if err != nil {
		return nil, err
	}

	// Merge default parameters with provided parameters
	templateParams := make(map[string]interface{})
	templateParams["targetName"] = targetName

	// Set default values
	for _, param := range childTemplate.Parameters {
		templateParams[param.Name] = param.Default
	}

	// Override with provided parameters
	for key, value := range params {
		templateParams[key] = value
	}

	var nodes []Node
	for _, nodeTemplate := range childTemplate.Children {
		node, err := tl.instantiateNodeFromTemplate(nodeTemplate, templateParams, targetName)
		if err != nil {
			return nil, fmt.Errorf("failed to instantiate node template: %w", err)
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// instantiateNodeFromTemplate creates a single Node from a NodeTemplate
func (tl *TemplateLoader) instantiateNodeFromTemplate(nodeTemplate NodeTemplate, params map[string]interface{}, targetName string) (Node, error) {
	// Convert template to YAML for processing
	templateBytes, err := yaml.Marshal(nodeTemplate)
	if err != nil {
		return Node{}, fmt.Errorf("failed to marshal node template: %w", err)
	}

	// Apply template parameters
	tmpl, err := template.New("node").Parse(string(templateBytes))
	if err != nil {
		return Node{}, fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return Node{}, fmt.Errorf("failed to execute template: %w", err)
	}

	// Parse the instantiated template back to Node struct
	var instantiatedTemplate NodeTemplate
	if err := yaml.Unmarshal(buf.Bytes(), &instantiatedTemplate); err != nil {
		return Node{}, fmt.Errorf("failed to unmarshal instantiated template: %w", err)
	}

	// Create the actual Node with owner references
	node := Node{
		Metadata: Metadata{
			Name:   instantiatedTemplate.Metadata.Name,
			Type:   instantiatedTemplate.Metadata.Type,
			Labels: instantiatedTemplate.Metadata.Labels,
			OwnerReferences: []OwnerReference{
				{
					APIVersion:         "gochain/v1",
					Kind:               "Node",
					Name:               targetName,
					Controller:         true,
					BlockOwnerDeletion: true,
				},
			},
		},
		Spec: NodeSpec{
			Params: instantiatedTemplate.Spec.Params,
			Inputs: instantiatedTemplate.Spec.Inputs,
		},
		Stats:  NodeStats{},
		Status: NodeStatusIdle,
		State:  make(map[string]interface{}),
	}

	return node, nil
}