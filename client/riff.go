package client

import "strconv"

// RiffProposal mirrors the server's RiffProposal: one generated absurd wiring
// between a named thing and an effect toy.
type RiffProposal struct {
	Text         string `json:"text"`
	TriggerKind  string `json:"triggerKind"`
	TriggerName  string `json:"triggerName"`
	ReactionType string `json:"reactionType"`
}

// GetRiffs fetches up to n generated riff proposals.
func (c *Client) GetRiffs(n int) ([]RiffProposal, error) {
	var result struct {
		Proposals []RiffProposal `json:"proposals"`
	}
	path := "/api/v1/riff"
	if n > 0 {
		path += "?n=" + strconv.Itoa(n)
	}
	if err := c.Request("GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result.Proposals, nil
}

// DeployRiff deploys a proposal's reaction want, straight from its structure.
func (c *Client) DeployRiff(p RiffProposal) (map[string]any, error) {
	var result map[string]any
	if err := c.Request("POST", "/api/v1/riff/deploy", p, &result); err != nil {
		return nil, err
	}
	return result, nil
}
