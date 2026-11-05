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
