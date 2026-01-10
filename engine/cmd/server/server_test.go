package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	types "mywant/engine/cmd/types"

	"github.com/stretchr/testify/assert"
)

// Helper to create a test server instance
func setupTestServer() *Server {
	config := ServerConfig{Port: 0, Host: "localhost", Debug: true}
	server := NewServer(config)
	server.setupRoutes()
	
	// Register built-in types for testing (normally done in Start)
	types.RegisterQNetWantTypes(server.globalBuilder)
	
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
	assert.Equal(t, float64(1), resp["wants"]) // JSON unmarshals numbers as float64
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
