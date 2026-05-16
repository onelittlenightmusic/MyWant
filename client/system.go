package client

import (
	"encoding/json"
	"fmt"
)

// ListWantTypes retrieves available want types
func (c *Client) ListWantTypes() ([]WantType, error) {
	var result WantTypeListResponse
	err := c.Request("GET", "/api/v1/want-types", nil, &result)
	if err != nil {
		return nil, err
	}
	return result.WantTypes, nil
}

// GetWantType retrieves a specific want type definition
func (c *Client) GetWantType(name string) (*map[string]any, error) {
	var result map[string]any
	err := c.Request("GET", fmt.Sprintf("/api/v1/want-types/%s", name), nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// RegisterWantType sends raw YAML to POST /api/v1/want-types for hot-reload registration.
func (c *Client) RegisterWantType(data []byte) (map[string]any, error) {
	raw, err := c.RawRequest("POST", "/api/v1/want-types", data, "application/x-yaml")
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return result, nil
}

// UpdateWantType sends raw YAML to PUT /api/v1/want-types/{name} to replace an existing YAML-only type.
func (c *Client) UpdateWantType(name string, data []byte) (map[string]any, error) {
	raw, err := c.RawRequest("PUT", fmt.Sprintf("/api/v1/want-types/%s", name), data, "application/x-yaml")
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return result, nil
}

// DeleteWantType sends DELETE /api/v1/want-types/{name} to remove a YAML-only want type.
func (c *Client) DeleteWantType(name string) (map[string]any, error) {
	raw, err := c.RawRequest("DELETE", fmt.Sprintf("/api/v1/want-types/%s", name), nil, "application/json")
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return result, nil
}

// ReloadWantTypes calls POST /api/v1/want-types/reload to re-scan ~/.mywant/custom-types/ without restart.
func (c *Client) ReloadWantTypes() (map[string]any, error) {
	raw, err := c.RawRequest("POST", "/api/v1/want-types/reload", nil, "application/json")
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return result, nil
}

// GetWantTypeExamples retrieves examples for a specific want type
func (c *Client) GetWantTypeExamples(name string) (*map[string]any, error) {
	var result map[string]any
	err := c.Request("GET", fmt.Sprintf("/api/v1/want-types/%s/examples", name), nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetLogs retrieves system logs
func (c *Client) GetLogs() (*APILogsResponse, error) {
	var result APILogsResponse
	err := c.Request("GET", "/api/v1/logs", nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
