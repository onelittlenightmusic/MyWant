package client

import (
	"encoding/json"
	"fmt"
)

// ListWants retrieves all wants from the server, optionally filtered by type and labels
func (c *Client) ListWants(wantType string, labels []string) (*APIDumpResponse, error) {
	var result APIDumpResponse
	path := "/api/v1/wants"

	// Build query parameters
	params := []string{}
	if wantType != "" {
		params = append(params, fmt.Sprintf("type=%s", wantType))
	}
	for _, label := range labels {
		params = append(params, fmt.Sprintf("label=%s", label))
	}

	if len(params) > 0 {
		path += "?" + fmt.Sprintf("%s", params[0])
		for i := 1; i < len(params); i++ {
			path += "&" + params[i]
		}
	}

	err := c.Request("GET", path, nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetWant retrieves a specific want by ID
func (c *Client) GetWant(id string, connectivityMetadata bool) (*Want, error) {
	var result Want
	path := fmt.Sprintf("/api/v1/wants/%s", id)
	if connectivityMetadata {
		path += "?connectivityMetadata=true"
	}
	err := c.Request("GET", path, nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// CreateWant creates a new want execution from config
func (c *Client) CreateWant(config Config) (*CreateWantResponse, error) {
	var result CreateWantResponse
	err := c.Request("POST", "/api/v1/wants", config, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteWant deletes a want by ID
func (c *Client) DeleteWant(id string) error {
	return c.Request("DELETE", fmt.Sprintf("/api/v1/wants/%s", id), nil, nil)
}

// BatchOperationRequest is the payload for batch operations
type BatchOperationRequest struct {
	IDs []string `json:"ids"`
}

// SuspendWants suspends multiple wants
func (c *Client) SuspendWants(ids []string) error {
	return c.Request("POST", "/api/v1/wants/suspend", BatchOperationRequest{IDs: ids}, nil)
}

// ResumeWants resumes multiple wants
func (c *Client) ResumeWants(ids []string) error {
	return c.Request("POST", "/api/v1/wants/resume", BatchOperationRequest{IDs: ids}, nil)
}

// StopWants stops multiple wants
func (c *Client) StopWants(ids []string) error {
	return c.Request("POST", "/api/v1/wants/stop", BatchOperationRequest{IDs: ids}, nil)
}

// StartWants starts multiple wants
func (c *Client) StartWants(ids []string) error {
	return c.Request("POST", "/api/v1/wants/start", BatchOperationRequest{IDs: ids}, nil)
}

// ExportWants exports all wants as YAML
func (c *Client) ExportWants() ([]byte, error) {
	return c.RawRequest("POST", "/api/v1/wants/export", nil, "application/json")
}

// ImportWants imports wants from YAML
func (c *Client) ImportWants(yamlData []byte) (*ImportWantsResponse, error) {
	var result ImportWantsResponse
	respData, err := c.RawRequest("POST", "/api/v1/wants/import", yamlData, "application/yaml")
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(respData, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &result, nil
}
