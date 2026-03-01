package client

// GlobalStateResponse is the response from GET /api/v1/global-state
type GlobalStateResponse struct {
	State     map[string]any `json:"state"`
	Timestamp string         `json:"timestamp"`
}

// GlobalParametersResponse is the response from GET/PUT /api/v1/global-parameters
type GlobalParametersResponse struct {
	Parameters map[string]any `json:"parameters"`
	Count      int            `json:"count"`
}

// GetGlobalState retrieves the current global state (memo).
func (c *Client) GetGlobalState() (*GlobalStateResponse, error) {
	var result GlobalStateResponse
	err := c.Request("GET", "/api/v1/global-state", nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteGlobalState clears all global state.
func (c *Client) DeleteGlobalState() error {
	return c.Request("DELETE", "/api/v1/global-state", nil, nil)
}

// GetGlobalParameters retrieves all global parameters.
func (c *Client) GetGlobalParameters() (*GlobalParametersResponse, error) {
	var result GlobalParametersResponse
	err := c.Request("GET", "/api/v1/global-parameters", nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateGlobalParameters replaces all global parameters and persists to disk.
func (c *Client) UpdateGlobalParameters(parameters map[string]any) (*GlobalParametersResponse, error) {
	body := map[string]any{"parameters": parameters}
	var result GlobalParametersResponse
	err := c.Request("PUT", "/api/v1/global-parameters", body, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
