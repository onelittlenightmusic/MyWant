package client

import (
	"encoding/json"
	"fmt"
)

// ListWants retrieves all wants from the server, optionally filtered by type, labels, and using selectors.
func (c *Client) ListWants(wantType string, labels []string, using []string, includeCancelled bool, includeSystem bool) (*APIDumpResponse, error) {
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
	for _, u := range using {
		params = append(params, fmt.Sprintf("using=%s", u))
	}
	if includeCancelled {
		params = append(params, "includeCancelled=true")
	}
	if includeSystem {
		params = append(params, "includeSystemWants=true")
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

// RestartWants restarts multiple wants (stop then start)
func (c *Client) RestartWants(ids []string) error {
	return c.Request("POST", "/api/v1/wants/restart", BatchOperationRequest{IDs: ids}, nil)
}

// ResolveWantID resolves a want name or ID to a want ID.
// If nameOrID already matches a want's metadata.id, it is returned as-is.
// Otherwise the wants list is searched for a want whose metadata.name matches.
func (c *Client) ResolveWantID(nameOrID string) (string, error) {
	resp, err := c.ListWants("", nil, nil, false, false)
	if err != nil {
		return "", err
	}
	for _, w := range resp.Wants {
		if w.Metadata.ID == nameOrID || w.Metadata.Name == nameOrID {
			return w.Metadata.ID, nil
		}
	}
	return "", fmt.Errorf("want not found: %q", nameOrID)
}

// FieldMatchRec mirrors a recommendation returned by the field-match-recommendations API.
type FieldMatchRec struct {
	Score        float64 `json:"score"`
	Description  string  `json:"description"`
	ExposeAction *struct {
		WantID    string `json:"want_id"`
		WantName  string `json:"want_name"`
		FieldName string `json:"field_name"`
		GlobalKey string `json:"global_key"`
	} `json:"expose_action,omitempty"`
	ImportAction *struct {
		WantID    string `json:"want_id"`
		WantName  string `json:"want_name"`
		GlobalKey string `json:"global_key"`
		LocalKey  string `json:"local_key"`
	} `json:"import_action,omitempty"`
}

// ConnectWants fetches field-match recommendations between sourceID and targetID
// (trying both role orderings) and applies the highest-scoring recommendation.
func (c *Client) ConnectWants(sourceID, targetID string) (*FieldMatchRec, error) {
	const labels = "current,plan,goal"
	type recResp struct {
		Recommendations []FieldMatchRec `json:"recommendations"`
	}

	// Try source→target then target→source; pick whichever returns recommendations.
	pairs := [][2]string{{sourceID, targetID}, {targetID, sourceID}}
	var chosen *FieldMatchRec
	var chosenSrc, chosenTgt string
	for _, pair := range pairs {
		src, tgt := pair[0], pair[1]
		var resp recResp
		path := "/api/v1/wants/field-match-recommendations?source_id=" + src + "&target_id=" + tgt + "&exposed_labels=" + labels
		if err := c.Request("GET", path, nil, &resp); err != nil {
			continue
		}
		if len(resp.Recommendations) > 0 {
			r := resp.Recommendations[0]
			chosen = &r
			chosenSrc, chosenTgt = src, tgt
			break
		}
	}
	if chosen == nil {
		return nil, fmt.Errorf("no field-match recommendations found between the two wants")
	}

	body := map[string]any{
		"source_id": chosenSrc,
		"target_id": chosenTgt,
	}
	if chosen.ExposeAction != nil {
		body["expose_action"] = chosen.ExposeAction
	}
	if chosen.ImportAction != nil {
		body["import_action"] = chosen.ImportAction
	}
	if err := c.Request("POST", "/api/v1/wants/field-match-recommendations/apply", body, nil); err != nil {
		return nil, fmt.Errorf("apply recommendation: %w", err)
	}
	return chosen, nil
}

// DisconnectWants removes the expose/import link between two wants.
// It inspects both wants to find which is provider and which is consumer,
// then deletes the matching expose entry from the provider.
func (c *Client) DisconnectWants(idA, idB string) (string, error) {
	wantA, err := c.GetWant(idA, false)
	if err != nil {
		return "", err
	}
	wantB, err := c.GetWant(idB, false)
	if err != nil {
		return "", err
	}

	// Find a global key that A exposes and B imports (or vice versa).
	providerID, globalKey := findExposeImportMatch(wantA, wantB)
	if providerID == "" {
		// Try the reverse direction.
		providerID, globalKey = findExposeImportMatch(wantB, wantA)
	}
	if providerID == "" {
		return "", fmt.Errorf("no expose/import link found between the two wants")
	}

	label := "expose/" + globalKey
	if err := c.Request("DELETE", "/api/v1/wants/"+providerID+"/relations", map[string]any{"label": label}, nil); err != nil {
		return "", fmt.Errorf("delete relation: %w", err)
	}
	return label, nil
}

// findExposeImportMatch returns (providerID, globalKey) if provider exposes a key
// that consumer imports; empty strings if no match.
func findExposeImportMatch(provider, consumer *Want) (string, string) {
	for _, exp := range provider.Spec.Exposes {
		if _, ok := consumer.Spec.Imports[exp.As]; ok {
			return provider.Metadata.ID, exp.As
		}
	}
	return "", ""
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
