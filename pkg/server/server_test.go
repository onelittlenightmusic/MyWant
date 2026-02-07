package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	_ "mywant/engine/cmd/types"

	"github.com/stretchr/testify/assert"
)

// Helper to create a test server instance
func setupTestServer() *Server {
	config := Config{Port: 0, Host: "localhost", Debug: true}
	// Create dummy directories if they don't exist to avoid loader errors
	os.MkdirAll("yaml/want_types", 0755)
	os.MkdirAll("yaml/recipes", 0755)

	server := New(config)
	server.setupRoutes()

	return server
}

func TestHealthCheck(t *testing.T) {
	s := setupTestServer()

	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "healthy", resp["status"])
	assert.Equal(t, "mywant", resp["server"])
}

func TestCreateWantAPI(t *testing.T) {
	s := setupTestServer()

	// Valid payload
	payload := map[string]any{
		"wants": []map[string]any{
			{
				"metadata": map[string]string{
					"name": "test-want",
					"type": "qnet queue",
				},
				"spec": map[string]any{
					"params": map[string]any{
						"service_time": 0.1,
					},
				},
			},
		},
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "/api/v1/wants", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "created", resp["status"])
	assert.Equal(t, float64(1), resp["wants"])
}

func TestInvalidWantType(t *testing.T) {
	s := setupTestServer()

	// Invalid type payload
	payload := map[string]any{
		"wants": []map[string]any{
			{
				"metadata": map[string]string{
					"name": "bad-want",
					"type": "non-existent-type",
				},
				"spec": map[string]any{
					"params": map[string]any{},
				},
			},
		},
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "/api/v1/wants", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	// Should fail validation
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
