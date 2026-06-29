package server

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
)

// hashJSON returns a 16-char hex digest of v marshalled as JSON.
// Used to compute content-based ETags.
func hashJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h)[:16]
}

// checkETagValue sets the given etagValue (must already include surrounding
// quotes, e.g. `"abc123"`) as the ETag response header, then returns true
// when the client's If-None-Match matches exactly — meaning the caller should
// write http.StatusNotModified and return.
func checkETagValue(w http.ResponseWriter, r *http.Request, etagValue string) bool {
	w.Header().Set("ETag", etagValue)
	return r.Header.Get("If-None-Match") == etagValue
}

// checkETag hashes v as JSON, wraps it in quotes to form an ETag value, and
// delegates to checkETagValue. Convenience wrapper for content-hash ETags.
func checkETag(w http.ResponseWriter, r *http.Request, v any) bool {
	return checkETagValue(w, r, `"`+hashJSON(v)+`"`)
}
