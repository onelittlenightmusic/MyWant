package commands

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// captureOutput captures stdout during function execution
func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestListWantsCmd(t *testing.T) {
	// 1. Mock Server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/wants", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{
			"wants": [
				{"metadata": {"id": "1", "name": "want-1", "type": "target"}, "status": "running"},
				{"metadata": {"id": "2", "name": "want-2", "type": "queue"}, "status": "idle"}
			]
		}`)
	}))
	defer ts.Close()

	// 2. Set Config
	viper.Set("server", ts.URL)

	// 3. Execute
	output := captureOutput(func() {
		listWantsCmd.Run(listWantsCmd, []string{})
	})

	// 4. Verify Output
	assert.Contains(t, output, "want-1")
	assert.Contains(t, output, "target")
	assert.Contains(t, output, "running")
	assert.Contains(t, output, "want-2")
}

func TestGetWantCmd(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/wants/123", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{
			"metadata": {"id": "123", "name": "target-want", "type": "target"},
			"spec": {"params": {"count": 99}},
			"status": "completed"
		}`)
	}))
	defer ts.Close()

	viper.Set("server", ts.URL)

	output := captureOutput(func() {
		getWantCmd.Run(getWantCmd, []string{"123"})
	})

	assert.Contains(t, output, "ID: 123")
	assert.Contains(t, output, "Name: target-want")
	assert.Contains(t, output, "Status: completed")
	assert.Contains(t, output, "count: 99")
}
