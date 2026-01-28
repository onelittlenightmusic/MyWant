package client

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListWants(t *testing.T) {
	// 1. Mock Server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/wants", r.URL.Path)
		assert.Equal(t, "GET", r.Method)

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{
			"execution_id": "exec-1",
			"wants": [
				{"metadata": {"id": "w1", "name": "want-1", "type": "target"}, "status": "running"},
				{"metadata": {"id": "w2", "name": "want-2", "type": "queue"}, "status": "idle"}
			]
		}`)
	}))
	defer ts.Close()

	// 2. Client Setup
	c := NewClient(ts.URL)

	// 3. Execute
	resp, err := c.ListWants("", nil, nil)

	// 4. Assert
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "exec-1", resp.ExecutionID)
	assert.Len(t, resp.Wants, 2)
	assert.Equal(t, "want-1", resp.Wants[0].Metadata.Name)
}

func TestGetWant(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/wants/test-id", r.URL.Path)
		assert.Equal(t, "GET", r.Method)

		if r.URL.Query().Get("connectivityMetadata") == "true" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{
				"metadata": {"id": "test-id", "name": "test-want", "type": "target"},
				"spec": {"params": {"count": 10}},
				"status": "active",
				"connectivity_metadata": {
					"required_inputs": 1,
					"required_outputs": 1,
					"want_type": "target",
					"description": "Test target"
				}
			}`)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{
			"metadata": {"id": "test-id", "name": "test-want", "type": "target"},
			"spec": {"params": {"count": 10}},
			"status": "active"
		}`)
	}))
	defer ts.Close()

	c := NewClient(ts.URL)

	// Test without connectivity metadata
	want, err := c.GetWant("test-id", false)
	assert.NoError(t, err)
	assert.Equal(t, "test-id", want.Metadata.ID)
	assert.Equal(t, float64(10), want.Spec.Params["count"])
	assert.Nil(t, want.ConnectivityMetadata)

	// Test with connectivity metadata
	want, err = c.GetWant("test-id", true)
	assert.NoError(t, err)
	assert.NotNil(t, want.ConnectivityMetadata)
	assert.Equal(t, 1, want.ConnectivityMetadata.RequiredInputs)
}

func TestCreateWant(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/wants", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, `{
			"id": "exec-new",
			"status": "created",
			"wants": 1,
			"want_ids": ["w-new"],
			"message": "created"
		}`)
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	config := Config{
		Wants: []*Want{
			{Metadata: Metadata{Name: "new", Type: "queue"}},
		},
	}
	resp, err := c.CreateWant(config)

	assert.NoError(t, err)
	assert.Equal(t, "exec-new", resp.ID)
	assert.Equal(t, 1, resp.Wants)
}
