package client

import (
	"fmt"
)

// CreateSession creates a new interaction session
func (c *Client) CreateSession() (*InteractCreateResponse, error) {
	var response InteractCreateResponse
	err := c.Request("POST", "/api/v1/interact", nil, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	return &response, nil
}

// SendMessage sends a message to an interaction session and receives recommendations
func (c *Client) SendMessage(sessionID string, req InteractMessageRequest) (*InteractMessageResponse, error) {
	var response InteractMessageResponse
	path := fmt.Sprintf("/api/v1/interact/%s", sessionID)
	err := c.Request("POST", path, req, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}
	return &response, nil
}

// DeleteSession deletes an interaction session
func (c *Client) DeleteSession(sessionID string) error {
	path := fmt.Sprintf("/api/v1/interact/%s", sessionID)
	err := c.Request("DELETE", path, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

// DeployRecommendation deploys a recommendation from a session
func (c *Client) DeployRecommendation(sessionID string, req InteractDeployRequest) (*InteractDeployResponse, error) {
	var response InteractDeployResponse
	path := fmt.Sprintf("/api/v1/interact/%s/deploy", sessionID)
	err := c.Request("POST", path, req, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy recommendation: %w", err)
	}
	return &response, nil
}
