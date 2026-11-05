package client

// story.go — a character's story (GET /api/v1/characters/{id}/story), as
// want-spec's StoryList.

import (
	"net/url"
	"strconv"

	ws "github.com/onelittlenightmusic/want-spec"
)

// StoryQuery is what a story read asks for; empty fields are the server's
// defaults (the last 24h, every kind, the last 500).
type StoryQuery struct {
	Since string // RFC 3339, or a duration back from now (1h)
	Until string // RFC 3339
	Kinds string // comma-separated
	Limit int
}

// GetStory is a character's events, oldest first.
func (c *Client) GetStory(character string, q StoryQuery) ([]ws.StoryEvent, error) {
	v := url.Values{}
	if q.Since != "" {
		v.Set("since", q.Since)
	}
	if q.Until != "" {
		v.Set("until", q.Until)
	}
	if q.Kinds != "" {
		v.Set("kind", q.Kinds)
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	path := "/api/v1/characters/" + url.PathEscape(character) + "/story"
	if len(v) > 0 {
		path += "?" + v.Encode()
	}
	var out ws.StoryList
	if err := c.Request("GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out.Events, nil
}

// CharacterRef is a character as GET /api/v1/characters lists it.
type CharacterRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListCharacterRefs is the characters, by id and name.
func (c *Client) ListCharacterRefs() ([]CharacterRef, error) {
	var out struct {
		Characters []CharacterRef `json:"characters"`
	}
	if err := c.Request("GET", "/api/v1/characters", nil, &out); err != nil {
		return nil, err
	}
	return out.Characters, nil
}

// GetDigests is a character's days, one digest each, oldest first: since and
// until are YYYY-MM-DD or days back (7d); empty, the server's defaults (the
// last 7 days to today).
func (c *Client) GetDigests(character, since, until string) ([]ws.StoryDigest, error) {
	v := url.Values{}
	if since != "" {
		v.Set("since", since)
	}
	if until != "" {
		v.Set("until", until)
	}
	path := "/api/v1/characters/" + url.PathEscape(character) + "/digests"
	if len(v) > 0 {
		path += "?" + v.Encode()
	}
	var out struct {
		Digests []ws.StoryDigest `json:"digests"`
	}
	if err := c.Request("GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out.Digests, nil
}

// PatternAnswer is one way a kind of question was answered, and how it went.
type PatternAnswer struct {
	Shape       string         `json:"shape" yaml:"shape"`
	Count       int            `json:"count" yaml:"count"`
	Outcomes    map[string]int `json:"outcomes" yaml:"outcomes"`
	SuccessRate float64        `json:"successRate" yaml:"successRate"`
	Example     *ws.PlanSpec   `json:"example,omitempty" yaml:"example,omitempty"`
}

// PatternQA is the want type's worked example a kind of question is.
type PatternQA struct {
	Type     string `json:"type" yaml:"type"`
	Request  string `json:"request" yaml:"request"`
	Answer   string `json:"answer" yaml:"answer"`
	Template string `json:"template" yaml:"template"`
}

// Pattern is a kind of question (GET /api/v1/patterns).
type Pattern struct {
	Pattern  string          `json:"pattern" yaml:"pattern"`
	Declared *PatternQA      `json:"declared,omitempty" yaml:"declared,omitempty"`
	Match    string          `json:"match,omitempty" yaml:"match,omitempty"`
	Count    int             `json:"count" yaml:"count"`
	Answers  []PatternAnswer `json:"answers" yaml:"answers"`
	LastAt   string          `json:"lastAt" yaml:"lastAt"`
}

// GetPatterns is the kinds of question asked and how each was best answered:
// those question is one of (else the most like it), or all; one character's
// answers, or everyone's.
func (c *Client) GetPatterns(question, character string, limit int) ([]Pattern, error) {
	v := url.Values{}
	if question != "" {
		v.Set("q", question)
	}
	if character != "" {
		v.Set("character", character)
	}
	if limit > 0 {
		v.Set("limit", strconv.Itoa(limit))
	}
	path := "/api/v1/patterns"
	if len(v) > 0 {
		path += "?" + v.Encode()
	}
	var out struct {
		Patterns []Pattern `json:"patterns"`
	}
	if err := c.Request("GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out.Patterns, nil
}
