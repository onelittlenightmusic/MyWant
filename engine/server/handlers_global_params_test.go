package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	mywant "mywant/engine/core"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetGlobalParamsForTest resets the global parameters state between tests.
func resetGlobalParamsForTest(t *testing.T) {
	t.Helper()
	// Load from a non-existent path to clear in-memory state and reset path
	mywant.LoadGlobalParameters(filepath.Join(t.TempDir(), "parameters.yaml"))
}

func TestGetGlobalParameters_Empty(t *testing.T) {
	resetGlobalParamsForTest(t)
	s := setupTestServer()

	req, _ := http.NewRequest("GET", "/api/v1/global-parameters", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["count"])
	assert.NotNil(t, resp["parameters"])
}

func TestGetGlobalParameters_WithData(t *testing.T) {
	resetGlobalParamsForTest(t)
	s := setupTestServer()

	// Prime the global parameters state
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.yaml")
	os.WriteFile(path, []byte("api_key: secret\nmax_retry: 3\n"), 0644)
	mywant.LoadGlobalParameters(path)

	req, _ := http.NewRequest("GET", "/api/v1/global-parameters", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(2), resp["count"])

	params, ok := resp["parameters"].(map[string]any)
	require.True(t, ok, "parameters should be a map")
	assert.Equal(t, "secret", params["api_key"])
}

func TestUpdateGlobalParameters_Success(t *testing.T) {
	resetGlobalParamsForTest(t)
	s := setupTestServer()

	body, _ := json.Marshal(map[string]any{
		"parameters": map[string]any{
			"host":    "localhost",
			"port":    9090,
			"enabled": true,
		},
	})

	req, _ := http.NewRequest("PUT", "/api/v1/global-parameters", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(3), resp["count"])

	params, ok := resp["parameters"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "localhost", params["host"])
	assert.Equal(t, true, params["enabled"])
}

func TestUpdateGlobalParameters_ReflectedInGet(t *testing.T) {
	resetGlobalParamsForTest(t)
	s := setupTestServer()

	// PUT new parameters
	putBody, _ := json.Marshal(map[string]any{
		"parameters": map[string]any{"greet": "hello"},
	})
	putReq, _ := http.NewRequest("PUT", "/api/v1/global-parameters", bytes.NewBuffer(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	s.router.ServeHTTP(httptest.NewRecorder(), putReq)

	// GET should return the updated value
	req, _ := http.NewRequest("GET", "/api/v1/global-parameters", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	params, ok := resp["parameters"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "hello", params["greet"])
}

func TestUpdateGlobalParameters_ReplacesExistingParams(t *testing.T) {
	resetGlobalParamsForTest(t)
	s := setupTestServer()

	// Load initial parameters
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.yaml")
	os.WriteFile(path, []byte("old_key: old_value\n"), 0644)
	mywant.LoadGlobalParameters(path)

	// PUT replaces all parameters
	putBody, _ := json.Marshal(map[string]any{
		"parameters": map[string]any{"new_key": "new_value"},
	})
	req, _ := http.NewRequest("PUT", "/api/v1/global-parameters", bytes.NewBuffer(putBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	params, ok := resp["parameters"].(map[string]any)
	require.True(t, ok)

	_, hasOldKey := params["old_key"]
	assert.False(t, hasOldKey, "old_key should be removed after replace")
	assert.Equal(t, "new_value", params["new_key"])
}

func TestUpdateGlobalParameters_PersistsToDisk(t *testing.T) {
	resetGlobalParamsForTest(t)
	s := setupTestServer()

	// Set path via LoadGlobalParameters (non-existent file is OK)
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.yaml")
	mywant.LoadGlobalParameters(path)

	// PUT parameters
	putBody, _ := json.Marshal(map[string]any{
		"parameters": map[string]any{"db": "postgres"},
	})
	req, _ := http.NewRequest("PUT", "/api/v1/global-parameters", bytes.NewBuffer(putBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	// File should exist on disk
	_, err := os.Stat(path)
	assert.NoError(t, err, "parameters.yaml should be created on disk")

	// Simulate restart: reload from file
	mywant.LoadGlobalParameters(path)
	v, ok := mywant.GetGlobalParameter("db")
	assert.True(t, ok)
	assert.Equal(t, "postgres", v)
}

func TestUpdateGlobalParameters_EmptyParams(t *testing.T) {
	resetGlobalParamsForTest(t)
	s := setupTestServer()

	body, _ := json.Marshal(map[string]any{"parameters": map[string]any{}})
	req, _ := http.NewRequest("PUT", "/api/v1/global-parameters", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["count"])
}

func TestUpdateGlobalParameters_InvalidBody(t *testing.T) {
	resetGlobalParamsForTest(t)
	s := setupTestServer()

	req, _ := http.NewRequest("PUT", "/api/v1/global-parameters", bytes.NewBufferString("{not valid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateGlobalParameters_NullParametersField(t *testing.T) {
	resetGlobalParamsForTest(t)
	s := setupTestServer()

	// parameters field is null — should be treated as empty
	body, _ := json.Marshal(map[string]any{"parameters": nil})
	req, _ := http.NewRequest("PUT", "/api/v1/global-parameters", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["count"])
}
