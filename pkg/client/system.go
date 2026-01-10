package client

// ListWantTypes retrieves available want types
func (c *Client) ListWantTypes() ([]WantType, error) {
	var result WantTypeListResponse
	err := c.Request("GET", "/api/v1/want-types", nil, &result)
	if err != nil {
		return nil, err
	}
	return result.WantTypes, nil
}

// QueryLLM sends a query to the LLM
func (c *Client) QueryLLM(message, model string) (*LLMResponse, error) {
	req := LLMRequest{
		Message: message,
		Model:   model,
	}
	var result LLMResponse
	err := c.Request("POST", "/api/v1/llm/query", req, &result)
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
