package client

import (
	"encoding/json"
	"fmt"
)

// ListWants retrieves all wants from the server
func (c *Client) ListWants() (*APIDumpResponse, error) {
	var result APIDumpResponse
	err := c.Request("GET", "/api/v1/wants", nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetWant retrieves a specific want by ID
func (c *Client) GetWant(id string) (*Want, error) {
	var result Want
	err := c.Request("GET", fmt.Sprintf("/api/v1/wants/%s", id), nil, &result)
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
