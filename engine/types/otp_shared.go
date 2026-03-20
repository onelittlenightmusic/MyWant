package types

import (
	"encoding/base64"
	"encoding/json"
	"os"
)

// gtfsFeed holds a single GTFS feed URL and target filename.
type gtfsFeed struct {
	URL      string `json:"url"`
	Filename string `json:"filename"`
}

// resolveGTFSFeeds parses GTFS feed configuration from params.
// Priority: gtfs_feeds (JSON array of {url, filename}) > legacy gtfs_url/gtfs_filename.
// Returns nil if no GTFS is configured.
func resolveGTFSFeeds(params map[string]any) []gtfsFeed {
	if feedsJSON, ok := params["gtfs_feeds"].(string); ok && feedsJSON != "" {
		var feeds []gtfsFeed
		if err := json.Unmarshal([]byte(feedsJSON), &feeds); err == nil && len(feeds) > 0 {
			return feeds
		}
	}
	// Legacy single-feed params.
	url, _ := params["gtfs_url"].(string)
	if url == "" {
		return nil
	}
	filename, _ := params["gtfs_filename"].(string)
	if filename == "" {
		filename = "gtfs.zip"
	}
	return []gtfsFeed{{URL: url, Filename: filename}}
}

// fileExists returns true if the given path exists on disk.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// base64Encode returns the standard base64 encoding of s.
func base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
