package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
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
	Name  string `json:"name" yaml:"name"`
	Kind  string `json:"kind" yaml:"kind"` // thing | want | mixed
	Color string `json:"color,omitempty" yaml:"color,omitempty"`
	Art   string `json:"art,omitempty" yaml:"art,omitempty"`
	// Capture: the hub's tile — a capture into it; "" is an order, a chain.
	Capture     string            `json:"capture,omitempty" yaml:"capture,omitempty"`
	CaptureAt   string            `json:"captureAt,omitempty" yaml:"captureAt,omitempty"`
	Members     []string          `json:"members" yaml:"members"`
	MemberKinds map[string]string `json:"memberKinds,omitempty" yaml:"memberKinds,omitempty"`
	// Labels are the constellation's own (not its members').
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
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
	ID      string `json:"id"` // the thing's own UUID — survives its catalog and name changing
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

// GetThingsByCatalog folds the list back into catalog → values, the shape a
// caller wants when it only cares which values exist.
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

// ThingEntry is a thing as stored: an identity, and what is true about it.
type ThingEntry struct {
	ID      string `json:"id"`
	Catalog string `json:"catalog"`
	Value   string `json:"value"`
}

// CreateThing remembers one value and returns it, identity included.
func (c *Client) CreateThing(catalog, value string) (ThingEntry, error) {
	var out ThingEntry
	body := map[string]string{"catalog": catalog, "value": value}
	if err := c.Request("POST", "/api/v1/things", body, &out); err != nil {
		return ThingEntry{}, err
	}
	return out, nil
}

// PatchThing changes what a thing is filed under, or what it is called,
// without changing which thing it is. Empty fields are left alone.
func (c *Client) PatchThing(id, catalog, value string) (ThingEntry, error) {
	body := map[string]string{}
	if catalog != "" {
		body["catalog"] = catalog
	}
	if value != "" {
		body["value"] = value
	}
	var out ThingEntry
	path := fmt.Sprintf("/api/v1/things/%s", url.PathEscape(id))
	if err := c.Request("PATCH", path, body, &out); err != nil {
		return ThingEntry{}, err
	}
	return out, nil
}

// DeleteThing forgets one thing by identity.
func (c *Client) DeleteThing(id string) error {
	return c.Request("DELETE", fmt.Sprintf("/api/v1/things/%s", url.PathEscape(id)), nil, nil)
}

// GetThing is one thing, whole (GET /things/{id}).
func (c *Client) GetThing(id string) (*Thing, error) {
	var out Thing
	if err := c.Request("GET", "/api/v1/things/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Removed is what a delete of several took, and what it found nothing of.
type Removed struct {
	Deleted []string `json:"deleted"`
	Missing []string `json:"missing"`
}

// DeleteThings forgets several things at once (DELETE /things?ids=).
func (c *Client) DeleteThings(ids []string) (Removed, error) {
	var out Removed
	err := c.Request("DELETE", "/api/v1/things?ids="+url.QueryEscape(strings.Join(ids, ",")), nil, &out)
	return out, err
}

// PatchThingLabels sets and removes several of a thing's labels in one request.
func (c *Client) PatchThingLabels(id string, set map[string]string, remove []string) (map[string]string, error) {
	var out struct {
		Labels map[string]string `json:"labels"`
	}
	body := map[string]any{"set": set, "remove": remove}
	if err := c.Request("PATCH", "/api/v1/things/"+url.PathEscape(id)+"/labels", body, &out); err != nil {
		return nil, err
	}
	return out.Labels, nil
}

// ExportThings is things as YAML (all, or those ids).
func (c *Client) ExportThings(ids []string) ([]byte, error) {
	path := "/api/v1/things/export"
	if len(ids) > 0 {
		path += "?ids=" + url.QueryEscape(strings.Join(ids, ","))
	}
	return c.RawRequest("POST", path, nil, "application/json")
}

// ImportThings reads things from YAML; the ids they have here.
func (c *Client) ImportThings(yamlData []byte) ([]string, error) {
	return importIDs(c, "/api/v1/things/import", yamlData)
}

func importIDs(c *Client, path string, yamlData []byte) ([]string, error) {
	raw, err := c.RawRequest("POST", path, yamlData, "application/yaml")
	if err != nil {
		return nil, err
	}
	var out struct {
		Imported []string `json:"imported"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%s", strings.TrimSpace(string(raw)))
	}
	return out.Imported, nil
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

// GetThingLabels returns every thing's labels, keyed by its id (from GET /things).
func (c *Client) GetThingLabels() (map[string]map[string]string, error) {
	things, err := c.GetThings()
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]string{}
	for _, t := range things {
		if len(t.Labels) > 0 {
			out[t.ID] = t.Labels
		}
	}
	return out, nil
}

// ResolveThingID is the id of the thing ref names: its id ("thg-…"), or
// "<catalog>::<value>".
func (c *Client) ResolveThingID(ref string) (string, error) {
	catalog, value, pair := strings.Cut(ref, "::")
	if !pair {
		return ref, nil
	}
	things, err := c.GetThings()
	if err != nil {
		return "", err
	}
	for _, t := range things {
		if t.Catalog == catalog && t.Value == value {
			return t.ID, nil
		}
	}
	return "", fmt.Errorf("no thing %q", ref)
}

// SetThingLabel attaches key=value to a thing (POST /things/{id}/labels); ref
// is its id or "<catalog>::<value>".
func (c *Client) SetThingLabel(ref, key, value string) error {
	id, err := c.ResolveThingID(ref)
	if err != nil {
		return err
	}
	body := map[string]string{"key": key, "value": value}
	return c.Request("POST", "/api/v1/things/"+url.PathEscape(id)+"/labels", body, nil)
}

// RemoveThingLabel detaches a label key from a thing (DELETE /things/{id}/labels/{key}).
func (c *Client) RemoveThingLabel(ref, key string) error {
	id, err := c.ResolveThingID(ref)
	if err != nil {
		return err
	}
	return c.Request("DELETE", "/api/v1/things/"+url.PathEscape(id)+"/labels/"+url.PathEscape(key), nil, nil)
}

// GetConstellations lists constellations. kind is "thing", "want", or "" for all.
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

// ConstellationChange is what a create or an update says; nil (or "" on
// create) leaves a field as it is.
type ConstellationChange struct {
	Name    *string   `json:"name,omitempty"`
	Members *[]string `json:"members,omitempty"`
	Color   *string   `json:"color,omitempty"`
	// Capture: the hub's tile id — a capture into it; "" makes it an order.
	Capture *string `json:"capture,omitempty"`
	// Labels replace its own labels when given; an empty map clears them.
	Labels *map[string]string `json:"labels,omitempty"`
}

// CreateConstellation makes a constellation of things or wants.
func (c *Client) CreateConstellation(name string, ch ConstellationChange) error {
	ch.Name = &name
	if ch.Members == nil {
		ch.Members = &[]string{}
	}
	return c.Request("POST", "/api/v1/constellations", ch, nil)
}

// GetConstellation is one constellation (GET /constellations/{name}).
func (c *Client) GetConstellation(name string) (*Constellation, error) {
	var out Constellation
	if err := c.Request("GET", "/api/v1/constellations/"+url.PathEscape(name), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateConstellation changes what it is given: its name, members, colour, hub.
func (c *Client) UpdateConstellation(name string, ch ConstellationChange) error {
	return c.Request("PATCH", "/api/v1/constellations/"+url.PathEscape(name), ch, nil)
}

// DeleteConstellations removes several at once, leaving their members in place.
func (c *Client) DeleteConstellations(names []string) (Removed, error) {
	var out Removed
	err := c.Request("DELETE", "/api/v1/constellations?names="+url.QueryEscape(strings.Join(names, ",")), nil, &out)
	return out, err
}

// ExportConstellations is constellations as YAML (all, or those names).
func (c *Client) ExportConstellations(names []string) ([]byte, error) {
	path := "/api/v1/constellations/export"
	if len(names) > 0 {
		path += "?names=" + url.QueryEscape(strings.Join(names, ","))
	}
	return c.RawRequest("POST", path, nil, "application/json")
}

// ImportConstellations reads constellations from YAML; their names.
func (c *Client) ImportConstellations(yamlData []byte) ([]string, error) {
	return importIDs(c, "/api/v1/constellations/import", yamlData)
}

// DeleteConstellation removes one constellation, leaving its members in place.
func (c *Client) DeleteConstellation(name string) error {
	return c.Request("DELETE", "/api/v1/constellations/"+url.PathEscape(name), nil, nil)
}
