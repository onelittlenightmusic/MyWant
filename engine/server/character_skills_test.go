package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// The skills resource (skills.go): put by name as YAML (what the GUI sends
// when it starts), listed in their digits' order, kept beside the config,
// created once, deleted.
func TestSkillsCRUD(t *testing.T) {
	dir := t.TempDir()
	s := &Server{config: Config{ConfigPath: filepath.Join(dir, "config.yaml")}}
	skills = &skillRegistry{}
	put := func(name, body string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/v1/character-skills/"+name, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/yaml")
		s.putSkill(rec, mux.SetURLVars(req, map[string]string{"name": name}))
		return rec.Code
	}
	if c := put("connect", "apiVersion: mywant.io/v1\nkind: CharacterSkill\nmetadata:\n  name: connect\nspec:\n  label: 接続\n  digit: 2\n  description: つなぐ\n"); c != http.StatusOK {
		t.Fatalf("put connect: %d", c)
	}
	if c := put("throw", "spec:\n  label: 投げる\n  digit: 1\n"); c != http.StatusOK {
		t.Fatalf("put throw (named by its path): %d", c)
	}

	skills = &skillRegistry{} // as a restarted backend: read back from the file
	rec := httptest.NewRecorder()
	s.listSkills(rec, httptest.NewRequest(http.MethodGet, "/api/v1/character-skills", nil))
	var got struct{ Skills []CharacterSkill }
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Skills) != 2 || got.Skills[0].Metadata.Name != "throw" || got.Skills[1].Spec.Description != "つなぐ" || got.Skills[0].Kind != "CharacterSkill" {
		t.Fatalf("skills = %+v", got.Skills)
	}

	rec = httptest.NewRecorder()
	s.getSkill(rec, mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/api/v1/character-skills/connect", nil), map[string]string{"name": "connect"}))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "接続") {
		t.Errorf("get: %d %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	s.deleteSkill(rec, mux.SetURLVars(httptest.NewRequest(http.MethodDelete, "/api/v1/character-skills/throw", nil), map[string]string{"name": "throw"}))
	if rec.Code != http.StatusOK {
		t.Errorf("delete: %d", rec.Code)
	}
	if _, ok := skills.get("throw"); ok {
		t.Error("still there after delete")
	}
}
