package client

import "fmt"

// HierarchicalState mirrors the server's hierarchicalState type.
type HierarchicalState struct {
	FinalResult any            `json:"final_result,omitempty"`
	Current     map[string]any `json:"current,omitempty"`
	Goal        map[string]any `json:"goal,omitempty"`
	Plan        map[string]any `json:"plan,omitempty"`
}

// WantStateSnapshot is a state snapshot for a single Want.
type WantStateSnapshot struct {
	WantID   string            `json:"want_id"`
	WantName string            `json:"want_name"`
	State    HierarchicalState `json:"state"`
}

// StatesListResponse is the response for GET /api/v1/states.
type StatesListResponse struct {
	Wants       []WantStateSnapshot `json:"wants"`
	GlobalState map[string]any      `json:"global_state,omitempty"`
	Total       int                 `json:"total"`
}

// StateSearchResult is a single field match in the cross-want state search.
type StateSearchResult struct {
	WantID   string `json:"want_id,omitempty"`
	WantName string `json:"want_name,omitempty"`
	Field    string `json:"field"`
	Value    any    `json:"value"`
	Label    string `json:"label"`
	Source   string `json:"source"`
}

// StateSearchResponse is the response for GET /api/v1/states/search.
type StateSearchResponse struct {
	Field   string              `json:"field"`
	Results []StateSearchResult `json:"results"`
	Total   int                 `json:"total"`
}

// ListStates retrieves state snapshots for all wants.
// Set includeGlobal=false to exclude global state.
// Set ancestorID to scope results to descendants of a specific want.
// Set label to filter by state label ("current", "goal", "plan").
func (c *Client) ListStates(ancestorID, label string, includeGlobal bool) (*StatesListResponse, error) {
	path := "/api/v1/states"
	params := []string{}
	if ancestorID != "" {
		params = append(params, fmt.Sprintf("ancestor_id=%s", ancestorID))
	}
	if label != "" {
		params = append(params, fmt.Sprintf("label=%s", label))
	}
	if !includeGlobal {
		params = append(params, "include_global=false")
	}
	if len(params) > 0 {
		path += "?" + params[0]
		for i := 1; i < len(params); i++ {
			path += "&" + params[i]
		}
	}

	var result StatesListResponse
	if err := c.Request("GET", path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SearchStates searches all wants for a specific state field.
func (c *Client) SearchStates(field, ancestorID string, includeGlobal bool) (*StateSearchResponse, error) {
	path := fmt.Sprintf("/api/v1/states/search?field=%s", field)
	if ancestorID != "" {
		path += fmt.Sprintf("&ancestor_id=%s", ancestorID)
	}
	if !includeGlobal {
		path += "&include_global=false"
	}

	var result StateSearchResponse
	if err := c.Request("GET", path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetWantState retrieves the full state snapshot for a single want.
func (c *Client) GetWantState(wantID string) (*WantStateSnapshot, error) {
	var result WantStateSnapshot
	if err := c.Request("GET", fmt.Sprintf("/api/v1/states/%s", wantID), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateWantState merges the given key-value map into a want's state.
func (c *Client) UpdateWantState(wantID string, updates map[string]any) (*WantStateSnapshot, error) {
	var result WantStateSnapshot
	if err := c.Request("PUT", fmt.Sprintf("/api/v1/states/%s", wantID), updates, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SetWantStateKey sets a single key in a want's state.
func (c *Client) SetWantStateKey(wantID, key string, value any) error {
	return c.Request("PUT", fmt.Sprintf("/api/v1/states/%s/%s", wantID, key), value, nil)
}

// DeleteWantStateKey removes a single key from a want's state.
func (c *Client) DeleteWantStateKey(wantID, key string) error {
	return c.Request("DELETE", fmt.Sprintf("/api/v1/states/%s/%s", wantID, key), nil, nil)
}

// SetGlobalStateKey sets a single key in the global state.
func (c *Client) SetGlobalStateKey(key string, value any) error {
	return c.Request("PUT", fmt.Sprintf("/api/v1/global-state/%s", key), value, nil)
}

// DeleteGlobalStateKey removes a single key from the global state.
func (c *Client) DeleteGlobalStateKey(key string) error {
	return c.Request("DELETE", fmt.Sprintf("/api/v1/global-state/%s", key), nil, nil)
}
