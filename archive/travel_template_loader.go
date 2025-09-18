package main

// LoadTravelTemplate loads and processes a travel template file using generic loader
func LoadTravelTemplate(templatePath string, params map[string]interface{}) (*GenericTemplateConfig, error) {
	loader := NewGenericTemplateLoader("")
	return loader.LoadTemplate(templatePath, params)
}

// LoadConfigFromTemplate loads configuration from a travel template
func LoadConfigFromTemplate(templatePath string, params map[string]interface{}) (Config, error) {
	loader := NewGenericTemplateLoader("")
	return loader.LoadConfigFromTemplate(templatePath, params)
}

// ValidateTemplate checks if the template file exists and is valid
func ValidateTemplate(templatePath string) error {
	loader := NewGenericTemplateLoader("")
	return loader.ValidateTemplate(templatePath)
}

// GetTemplateParameters extracts available parameters from template
func GetTemplateParameters(templatePath string) (map[string]interface{}, error) {
	loader := NewGenericTemplateLoader("")
	return loader.GetTemplateParameters(templatePath)
}

// Legacy types for backward compatibility
type TemplateConfig = GenericTemplateConfig
type TemplateMetadata = GenericTemplateMetadata
type TravelTemplate = GenericTemplate
