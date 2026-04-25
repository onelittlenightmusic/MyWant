package client

import "fmt"

// ShowGUIWant opens the sidebar for a specific want in the web GUI.
func (c *Client) ShowGUIWant(wantID, tab string, statusFilter, searchQuery string) error {
	updates := map[string]any{
		"source":                  "cli",
		"sidebar_open":            true,
		"sidebar_want_id":         wantID,
		"sidebar_active_tab":      tab,
		"dashboard_status_filter": statusFilter,
		"dashboard_search_query":  searchQuery,
	}
	return c.Request("PUT", "/api/v1/gui/state", updates, nil)
}

// ShowGUIDashboard navigates the web GUI to the dashboard view with optional filters.
func (c *Client) ShowGUIDashboard(statusFilter, searchQuery string) error {
	updates := map[string]any{
		"source":                  "cli",
		"sidebar_open":            false,
		"sidebar_want_id":         "",
		"sidebar_active_tab":      "",
		"dashboard_status_filter": statusFilter,
		"dashboard_search_query":  searchQuery,
	}
	return c.Request("PUT", "/api/v1/gui/state", updates, nil)
}

// GetGUIState retrieves the current GUI state from /api/v1/gui/state.
func (c *Client) GetGUIState() (*WantStateSnapshot, error) {
	var result WantStateSnapshot
	if err := c.Request("GET", "/api/v1/gui/state", nil, &result); err != nil {
		return nil, fmt.Errorf("failed to get GUI state: %w", err)
	}
	return &result, nil
}
