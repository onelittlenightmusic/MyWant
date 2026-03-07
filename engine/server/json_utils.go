package server

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"gopkg.in/yaml.v3"
)

// generateWantID generates a unique ID for a want
func generateWantID() string {
	uuid := make([]byte, 16)
	rand.Read(uuid)
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	return fmt.Sprintf("want-%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}

// JSONResponse sends a JSON response with the given status code
func (s *Server) JSONResponse(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			log.Printf("[ERROR] Failed to encode JSON response: %v", err)
		}
	}
}

// JSONError sends a JSON error response and logs the error
func (s *Server) JSONError(w http.ResponseWriter, r *http.Request, statusCode int, message string, details string) {
	s.globalBuilder.LogAPIOperation(r.Method, r.URL.Path, "", "error", statusCode, message, details)
	s.logError(r, statusCode, message, details, "", "")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{
		"error":   message,
		"details": details,
	})
}

// DecodeRequest decodes the request body into the given target, supporting both JSON and YAML
func DecodeRequest(r *http.Request, target any) error {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "yaml") || strings.Contains(contentType, "text/yaml") {
		return yaml.Unmarshal(data, target)
	}
	return json.Unmarshal(data, target)
}
