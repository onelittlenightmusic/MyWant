package client

// ListAgents retrieves all agents
func (c *Client) ListAgents() (map[string][]Agent, error) {
	var result map[string][]Agent
	err := c.Request("GET", "/api/v1/agents", nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
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
