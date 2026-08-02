package client

import (
	"fmt"
	"net/url"
)

// Memo is the user's catalog of remembered input values, keyed by subtype
// (destination, hotel, command, ...). The server records where each value came
// from (events) and how often it is used (stats), and lets values be labelled -
// constellations are a facade over labels with a "constellation/<name>" key.

// MemoEvent is one provenance entry: a value was recorded or used.
type MemoEvent struct {
	At      string `json:"at"` // RFC3339
	Catalog string `json:"catalog"`
	Subtype string `json:"subtype"`
	Value   string `json:"value"`
	Source  string `json:"source"`

	WantID   string `json:"wantId,omitempty"`
	WantType string `json:"wantType,omitempty"`

	CharacterID   string `json:"characterId,omitempty"`
	CharacterName string `json:"characterName,omitempty"`
}

// MemoStat is the usage summary of a single value.
type MemoStat struct {
	Count    int    `json:"count"`
	LastUsed string `json:"lastUsed"`
}

// Constellation is a named set of memo values or wants, stored as labels.
type Constellation struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"` // memo | want
	Members []string `json:"members"`
}

// GetMemo returns every subtype and its recorded values.
func (c *Client) GetMemo() (map[string][]string, error) {
	var result map[string][]string
	if err := c.Request("GET", "/api/v1/memo", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// PutMemo replaces the whole memo catalog.
func (c *Client) PutMemo(memo map[string][]string) error {
	return c.Request("PUT", "/api/v1/memo", memo, nil)
}

// GetMemoSuggestions returns the values recorded for one subtype, newest first.
func (c *Client) GetMemoSuggestions(subtype string, limit int) ([]string, error) {
	path := fmt.Sprintf("/api/v1/memo/suggestions/%s", url.PathEscape(subtype))
	if limit > 0 {
		path += fmt.Sprintf("?limit=%d", limit)
	}
	var result struct {
		Subtype     string   `json:"subtype"`
		Suggestions []string `json:"suggestions"`
	}
	if err := c.Request("GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result.Suggestions, nil
}

// GetMemoEvents returns memo provenance events, newest first. When catalog and
// value are both set the log is narrowed to that one value.
func (c *Client) GetMemoEvents(catalog, value string, limit int) ([]MemoEvent, error) {
	query := url.Values{}
	if limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", limit))
	}
	if catalog != "" && value != "" {
		query.Set("catalog", catalog)
		query.Set("value", value)
	}
	path := "/api/v1/memo/events"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}

	var result struct {
		Events []MemoEvent `json:"events"`
	}
	if err := c.Request("GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result.Events, nil
}

// GetMemoStats returns per-value usage counts keyed by catalog, then value.
func (c *Client) GetMemoStats() (map[string]map[string]MemoStat, error) {
	var result struct {
		Stats map[string]map[string]MemoStat `json:"stats"`
	}
	if err := c.Request("GET", "/api/v1/memo/stats", nil, &result); err != nil {
		return nil, err
	}
	return result.Stats, nil
}

// GetMemoLabels returns every label attached to memo values, keyed by value id.
func (c *Client) GetMemoLabels() (map[string]map[string]string, error) {
	var result struct {
		Labels map[string]map[string]string `json:"labels"`
	}
	if err := c.Request("GET", "/api/v1/memo/labels", nil, &result); err != nil {
		return nil, err
	}
	return result.Labels, nil
}

// SetMemoLabel attaches key=value to a memo value, identified as "<catalog>::<value>".
func (c *Client) SetMemoLabel(valueID, key, value string) error {
	body := map[string]string{"value_id": valueID, "key": key, "value": value}
	return c.Request("POST", "/api/v1/memo/labels", body, nil)
}

// RemoveMemoLabel detaches a label key from a memo value.
func (c *Client) RemoveMemoLabel(valueID, key string) error {
	body := map[string]string{"value_id": valueID, "key": key}
	return c.Request("POST", "/api/v1/memo/labels/remove", body, nil)
}

// GetConstellations lists constellations. kind is "memo", "want", or "" for both.
func (c *Client) GetConstellations(kind string) ([]Constellation, error) {
	path := "/api/v1/constellations"
	if kind != "" {
		path += "?kind=" + url.QueryEscape(kind)
	}
	var result struct {
		Constellations []Constellation `json:"groups"`
	}
	if err := c.Request("GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result.Constellations, nil
}

// CreateConstellation creates a constellation of memo values or wants.
func (c *Client) CreateConstellation(name, kind string, members []string) error {
	body := map[string]any{"name": name, "kind": kind, "members": members}
	return c.Request("POST", "/api/v1/constellations", body, nil)
}

// UpdateConstellation renames a constellation and/or replaces its members.
func (c *Client) UpdateConstellation(name, kind string, newName *string, members *[]string) error {
	body := map[string]any{"kind": kind}
	if newName != nil {
		body["name"] = *newName
	}
	if members != nil {
		body["members"] = *members
	}
	return c.Request("PUT", "/api/v1/constellations/"+url.PathEscape(name), body, nil)
}

// DeleteConstellation removes a constellation, leaving its members in place.
func (c *Client) DeleteConstellation(name, kind string) error {
	path := "/api/v1/constellations/" + url.PathEscape(name)
	if kind != "" {
		path += "?kind=" + url.QueryEscape(kind)
	}
	return c.Request("DELETE", path, nil, nil)
}
