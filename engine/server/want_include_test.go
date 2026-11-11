package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	mywant "mywant/engine/core"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// One want on a fresh server, by its id.
func oneWant(t *testing.T) (*Server, string) {
	t.Helper()
	t.Setenv("MYWANT_MRS_SHARED", "0")
	t.Setenv("MYWANT_RUNNER", "off")
	s := patchTestServer(t)
	w := patchTestDo(t, s, "POST", "/api/v1/wants", []map[string]any{{
		"metadata": map[string]any{"name": "include-want", "type": "checklist"},
		"spec":     map[string]any{},
	}})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var id string
	require.Eventually(t, func() bool {
		for _, want := range s.globalBuilder.GetAllWantStates() {
			if want.Metadata.Name == "include-want" {
				id = want.Metadata.ID
				return true
			}
		}
		return false
	}, 3*time.Second, 20*time.Millisecond)
	return s, id
}

func TestAWantsPanelReadsItsAnswersAndTypeWithItInOneAsk(t *testing.T) {
	s, id := oneWant(t)

	get := func(path, etag string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest("GET", path, nil)
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)
		return w
	}

	w := get("/api/v1/wants/"+id+"?include=results,type", "")
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	inc, ok := resp["included"].(map[string]any)
	require.True(t, ok, "included is there")
	results, ok := inc["results"].(map[string]any)
	require.True(t, ok, "its answers, as /results gives them")
	assert.Contains(t, results, "data")
	typ, ok := inc["type"].(map[string]any)
	require.True(t, ok, "its type, as /want-types/{name} gives it")
	assert.Equal(t, "checklist", typ["metadata"].(map[string]any)["name"])

	// Asked again with what it had: nothing new.
	assert.Equal(t, http.StatusNotModified, get("/api/v1/wants/"+id+"?include=results,type", w.Header().Get("ETag")).Code)

	// Without ?include, as before: nothing more.
	w = get("/api/v1/wants/"+id, "")
	require.Equal(t, http.StatusOK, w.Code)
	resp = map[string]any{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotContains(t, resp, "included")
}

func TestABoardChangeCarriesTheChangedWantsAsTheBoardReadsThem(t *testing.T) {
	s, id := oneWant(t)

	data, hashes := s.boardChangeData([]string{id, "no-such-want"})
	require.Len(t, data, 1, "the want there, not the one gone")
	assert.Equal(t, id, data[0].Metadata.ID)
	assert.NotEmpty(t, data[0].Hash, "with its hash, to keep it by")
	assert.Equal(t, historyResults, data[0].HistoryScope, "its answers, not every history")
	want, _, _ := s.globalBuilder.FindWantByID(id)
	assert.Equal(t, mywant.CalculateWantHash(want), hashes[id], "and by the hash the want list's hashes give it")
}
