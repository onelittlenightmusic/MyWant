package client

import "fmt"

// ListAgents retrieves all agents
func (c *Client) ListAgents() (map[string][]Agent, error) {
	var result map[string][]Agent
	err := c.Request("GET", "/api/v1/agents", nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetAgent retrieves an agent by name
func (c *Client) GetAgent(name string) (*Agent, error) {
	var result Agent
	err := c.Request("GET", fmt.Sprintf("/api/v1/agents/%s", name), nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteAgent deletes an agent by name
func (c *Client) DeleteAgent(name string) error {
	return c.Request("DELETE", fmt.Sprintf("/api/v1/agents/%s", name), nil, nil)
}

// ListCapabilities retrieves all capabilities
func (c *Client) ListCapabilities() (map[string][]Capability, error) {
	var result map[string][]Capability
	err := c.Request("GET", "/api/v1/capabilities", nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetCapability retrieves a capability by name
func (c *Client) GetCapability(name string) (*Capability, error) {
	var result Capability
	err := c.Request("GET", fmt.Sprintf("/api/v1/capabilities/%s", name), nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteCapability deletes a capability by name
func (c *Client) DeleteCapability(name string) error {
	return c.Request("DELETE", fmt.Sprintf("/api/v1/capabilities/%s", name), nil, nil)
}
