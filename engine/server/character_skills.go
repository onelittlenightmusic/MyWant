package server

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/gorilla/mux"
	ws "github.com/onelittlenightmusic/want-spec"
	"gopkg.in/yaml.v3"
)

// Character skills: what a character can do with the Y button on the board — 投げる,
// 接続, 型, キャプチャ, 攻撃 — defined by the GUI extension that gives the board
// them (mywant-guiex's skills/*.yaml), not by the engine. The extension's GUI
// server puts them here when it starts (PUT, one by one), so the engine can
// answer what each one is: the character sheet's 技能 tab shows a skill's
// description, and a skill's picture is drawn from it.
//
// Written as the rest of a want's world is, Kubernetes-flavoured:
//
//	apiVersion: mywant.io/v1
//	kind: CharacterSkill
//	metadata:
//	  name: throw
//	spec:
//	  label: 投げる
//	  digit: 1
//	  description: …
//
// Kept beside the config (character-skills.yaml), so a backend restarted while the GUI
// is not still knows them. A resource of its own, YAML or JSON:
//
//	GET    /api/v1/character-skills          {skills: [...]} in their digits' order
//	POST   /api/v1/character-skills          a new one (409 when its name is taken)
//	GET    /api/v1/character-skills/{name}
//	PUT    /api/v1/character-skills/{name}   made or replaced — what the GUI sends
//	DELETE /api/v1/character-skills/{name}
//
// The schema is want-spec's (spec/character-skill-spec.yaml, CharacterSkill):
// not a Machine-Readable Skill, which is what an agent runs.

// CharacterSkill and its spec are want-spec's (spec/character-skill-spec.yaml).
type (
	CharacterSkill     = ws.CharacterSkill
	CharacterSkillSpec = ws.CharacterSkillSpec
)

type skillRegistry struct {
	mu     sync.Mutex
	path   string
	skills map[string]CharacterSkill
	loaded bool
}

var skills = &skillRegistry{}

func (r *skillRegistry) load(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.loaded && r.path == path {
		return
	}
	r.path, r.loaded, r.skills = path, true, map[string]CharacterSkill{}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var list []CharacterSkill
	if yaml.Unmarshal(data, &list) == nil {
		for _, sk := range list {
			r.skills[sk.Metadata.Name] = sk
		}
	}
}

func (r *skillRegistry) put(sk CharacterSkill) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[sk.Metadata.Name] = sk
	return r.saveLocked()
}

func (r *skillRegistry) get(name string) (CharacterSkill, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sk, ok := r.skills[name]
	return sk, ok
}

func (r *skillRegistry) remove(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.skills[name]; !ok {
		return false
	}
	delete(r.skills, name)
	_ = r.saveLocked()
	return true
}

func (r *skillRegistry) list() []CharacterSkill {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]CharacterSkill, 0, len(r.skills))
	for _, sk := range r.skills {
		out = append(out, sk)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Spec.Digit != out[j].Spec.Digit {
			return out[i].Spec.Digit < out[j].Spec.Digit
		}
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out
}

func (r *skillRegistry) saveLocked() error {
	if r.path == "" {
		return nil
	}
	list := make([]CharacterSkill, 0, len(r.skills))
	for _, sk := range r.skills {
		list = append(list, sk)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Metadata.Name < list[j].Metadata.Name })
	data, err := yaml.Marshal(list)
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, data, 0o644)
}

func (s *Server) skillsPath() string {
	if s.config.ConfigPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(s.config.ConfigPath), "character-skills.yaml")
}

// decodeSkill reads a CharacterSkill from the request (YAML or JSON), named `name`
// when the path says one.
func decodeSkill(r *http.Request, name string) (CharacterSkill, error) {
	var sk CharacterSkill
	if err := DecodeRequest(r, &sk); err != nil {
		return sk, err
	}
	if sk.Kind != "" && sk.Kind != ws.CharacterSkillKind {
		return sk, fmt.Errorf("kind %q is not CharacterSkill", sk.Kind)
	}
	if name != "" {
		if sk.Metadata.Name != "" && sk.Metadata.Name != name {
			return sk, fmt.Errorf("metadata.name %q is not %q", sk.Metadata.Name, name)
		}
		sk.Metadata.Name = name
	}
	if sk.Metadata.Name == "" {
		return sk, errors.New("a CharacterSkill needs metadata.name")
	}
	sk.Kind = ws.CharacterSkillKind
	if sk.APIVersion == "" {
		sk.APIVersion = "mywant.io/v1"
	}
	return sk, nil
}

// GET /api/v1/character-skills
func (s *Server) listSkills(w http.ResponseWriter, r *http.Request) {
	skills.load(s.skillsPath())
	s.JSONResponse(w, http.StatusOK, map[string]any{"skills": skills.list()})
}

// POST /api/v1/character-skills
func (s *Server) createSkill(w http.ResponseWriter, r *http.Request) {
	skills.load(s.skillsPath())
	sk, err := decodeSkill(r, "")
	if err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid CharacterSkill", err.Error())
		return
	}
	if _, ok := skills.get(sk.Metadata.Name); ok {
		s.JSONError(w, r, http.StatusConflict, "a CharacterSkill of that name exists", sk.Metadata.Name)
		return
	}
	if err := skills.put(sk); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to keep the skill", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusCreated, sk)
}

// GET /api/v1/character-skills/{name}
func (s *Server) getSkill(w http.ResponseWriter, r *http.Request) {
	skills.load(s.skillsPath())
	sk, ok := skills.get(mux.Vars(r)["name"])
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "no such skill", mux.Vars(r)["name"])
		return
	}
	s.JSONResponse(w, http.StatusOK, sk)
}

// PUT /api/v1/character-skills/{name}
func (s *Server) putSkill(w http.ResponseWriter, r *http.Request) {
	skills.load(s.skillsPath())
	sk, err := decodeSkill(r, mux.Vars(r)["name"])
	if err != nil {
		s.JSONError(w, r, http.StatusBadRequest, "invalid CharacterSkill", err.Error())
		return
	}
	if err := skills.put(sk); err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "failed to keep the skill", err.Error())
		return
	}
	s.JSONResponse(w, http.StatusOK, sk)
}

// DELETE /api/v1/character-skills/{name}
func (s *Server) deleteSkill(w http.ResponseWriter, r *http.Request) {
	skills.load(s.skillsPath())
	name := mux.Vars(r)["name"]
	if !skills.remove(name) {
		s.JSONError(w, r, http.StatusNotFound, "no such skill", name)
		return
	}
	s.JSONResponse(w, http.StatusOK, map[string]any{"message": "removed", "name": name})
}
