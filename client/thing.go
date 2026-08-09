package client

import (
	"fmt"
	"net/url"
)

// Memo is the user's catalog of remembered input values, keyed by subtype
// (destination, hotel, command, ...). The server records where each value came
// from (events) and how often it is used (stats), and lets values be labelled -
// constellations are a facade over labels with a "constellation/<name>" key.

// ThingEvent is one provenance entry: a value was recorded or used.
type ThingEvent struct {
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

// ThingDefinition is a name given to a value, and who gave it.
type ThingDefinition struct {
	Catalog string `json:"catalog"`
	Subtype string `json:"subtype"`
	Name    string `json:"name"`
	Value   any    `json:"value,omitempty"`
	At      string `json:"at"`

	CharacterID   string `json:"characterId,omitempty"`
	CharacterName string `json:"characterName,omitempty"`

	WantID   string `json:"wantId,omitempty"`
	WantType string `json:"wantType,omitempty"`
}

// Thing is one remembered value, whole: what it is, who named it, how often it
// is used, which wants name it now, and where it sits on the board. The server
// assembles all of it, so a client never has to fetch the pieces separately.
type Thing struct {
	ID      string `json:"id"` // "<catalog>::<value>"
	Catalog string `json:"catalog"`
	Subtype string `json:"subtype"`
	Value   string `json:"value"`

	Icon  string `json:"icon"`
	Color string `json:"color"`

	Definitions []ThingDefinition `json:"definitions,omitempty"`
	Stats       *MemoStat         `json:"stats,omitempty"`
	WantIDs     []string          `json:"wantIDs,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// GetThings returns every remembered value, whole.
func (c *Client) GetThings() ([]Thing, error) {
	var result struct {
		Things []Thing `json:"things"`
	}
	if err := c.Request("GET", "/api/v1/things", nil, &result); err != nil {
		return nil, err
	}
	return result.Things, nil
}

// GetThingsByCatalog folds the list back into catalog → values, which is both
// the shape PutThings takes and the shape a caller wants when it only cares
// which values exist.
func (c *Client) GetThingsByCatalog() (map[string][]string, error) {
	things, err := c.GetThings()
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, t := range things {
		out[t.Catalog] = append(out[t.Catalog], t.Value)
	}
	return out, nil
}

// PutThings replaces the whole memo catalog.
func (c *Client) PutThings(memo map[string][]string) error {
	return c.Request("PUT", "/api/v1/things", memo, nil)
}

// GetThingSuggestions returns the values recorded for one subtype, newest first.
func (c *Client) GetThingSuggestions(subtype string, limit int) ([]string, error) {
	path := fmt.Sprintf("/api/v1/things/suggestions/%s", url.PathEscape(subtype))
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

// GetThingEvents returns memo provenance events, newest first. When catalog and
// value are both set the log is narrowed to that one value.
func (c *Client) GetThingEvents(catalog, value string, limit int) ([]ThingEvent, error) {
	query := url.Values{}
	if limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", limit))
	}
	if catalog != "" && value != "" {
		query.Set("catalog", catalog)
		query.Set("value", value)
	}
	path := "/api/v1/things/events"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}

	var result struct {
		Events []ThingEvent `json:"events"`
	}
	if err := c.Request("GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result.Events, nil
}

// GetThingStats returns per-value usage counts keyed by catalog, then value.
func (c *Client) GetThingStats() (map[string]map[string]MemoStat, error) {
	var result struct {
		Stats map[string]map[string]MemoStat `json:"stats"`
	}
	if err := c.Request("GET", "/api/v1/things/stats", nil, &result); err != nil {
		return nil, err
	}
	return result.Stats, nil
}

// GetThingLabels returns every label attached to memo values, keyed by value id.
func (c *Client) GetThingLabels() (map[string]map[string]string, error) {
	var result struct {
		Labels map[string]map[string]string `json:"labels"`
	}
	if err := c.Request("GET", "/api/v1/things/labels", nil, &result); err != nil {
		return nil, err
	}
	return result.Labels, nil
}

// SetThingLabel attaches key=value to a memo value, identified as "<catalog>::<value>".
func (c *Client) SetThingLabel(valueID, key, value string) error {
	body := map[string]string{"value_id": valueID, "key": key, "value": value}
	return c.Request("POST", "/api/v1/things/labels", body, nil)
}

// RemoveThingLabel detaches a label key from a memo value.
func (c *Client) RemoveThingLabel(valueID, key string) error {
	body := map[string]string{"value_id": valueID, "key": key}
	return c.Request("POST", "/api/v1/things/labels/remove", body, nil)
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
