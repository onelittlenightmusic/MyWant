package client

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// A world is a named snapshot of every non-system want. Opening one saves the
// current world first, clears the canvas, and loads the target - so worlds are
// how a user keeps several unrelated setups side by side.

// World is one entry of the worlds list.
type World struct {
	Name        string `json:"name"`
	WantCount   int    `json:"want_count"`
	ModifiedAt  string `json:"modified_at"`
	Current     bool   `json:"current"`
	ThumbnailAt string `json:"thumbnail_at,omitempty"`
}

// ListWorlds returns every saved world, flagging the one currently open.
func (c *Client) ListWorlds() ([]World, error) {
	var worlds []World
	if err := c.Request("GET", "/api/v1/worlds", nil, &worlds); err != nil {
		return nil, err
	}
	return worlds, nil
}

// OpenWorld switches to a world, auto-saving the current one first.
func (c *Client) OpenWorld(name string) (map[string]any, error) {
	var result map[string]any
	path := fmt.Sprintf("/api/v1/worlds/%s/open", url.PathEscape(name))
	if err := c.Request("POST", path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// SaveWorld snapshots the running wants into a world without switching to it.
func (c *Client) SaveWorld(name string) error {
	path := fmt.Sprintf("/api/v1/worlds/%s/save", url.PathEscape(name))
	return c.Request("POST", path, nil, nil)
}

// ExportWorld downloads a world snapshot as YAML.
func (c *Client) ExportWorld(name string) ([]byte, error) {
	path := fmt.Sprintf("/api/v1/worlds/%s/export", url.PathEscape(name))
	return c.RawRequest("GET", path, nil, "application/json")
}

// ImportWorld uploads a wants YAML snapshot as a world.
func (c *Client) ImportWorld(name string, data []byte) (map[string]any, error) {
	path := fmt.Sprintf("/api/v1/worlds/%s/import", url.PathEscape(name))
	raw, err := c.RawRequest("POST", path, data, "application/x-yaml")
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return result, nil
}
