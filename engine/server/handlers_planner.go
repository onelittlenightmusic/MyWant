package server

import (
	"encoding/json"
	"io"
	"net/http"

	ws "github.com/onelittlenightmusic/want-spec"
	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"

	"mywant/engine/planner"
)

// POST /api/v1/planner/recipe/{name}
//
// Derives the child wants for a registered recipe that has an "achieve:" section.
// Returns a PlannerResult with the auto-derived Recipe and a reasoning trace.
// Useful for previewing what wants will be created before deploying the recipe.
//
// Path parameter: name — the recipe name (metadata.name field)
//
// Optional JSON body may supply globalParams to pre-fill parameters.
func (s *Server) planFromRecipe(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	// Find the recipe by metadata.name across all registered recipes.
	var recipe *ws.GenericRecipe
	for _, r := range s.recipeRegistry.ListRecipes() {
		if r.Recipe.Metadata.Name == name {
			recipe = r
			break
		}
	}
	if recipe == nil {
		http.Error(w, "recipe not found: "+name, http.StatusNotFound)
		return
	}

	if len(recipe.Recipe.Achieve) == 0 {
		http.Error(w, "recipe "+name+" has no achieve section", http.StatusBadRequest)
		return
	}

	// Optional: read globalParams from request body.
	globalParams := map[string]any{}
	body, _ := io.ReadAll(r.Body)
	defer r.Body.Close()
	if len(body) > 0 {
		_ = json.Unmarshal(body, &globalParams)
	}

	defs := s.globalBuilder.AllWantTypeDefinitions()
	wsDefs := make(map[string]*ws.WantTypeDefinition, len(defs))
	for k, v := range defs {
		wsDefs[k] = v
	}
	idx := planner.BuildExposableIndexFromDefs(wsDefs)
	p := planner.New(idx, wsDefs)

	plan := &ws.WantTypePlan{
		Achieve:     recipe.Recipe.Achieve,
		Hints:       recipe.Recipe.Hints,
		Constraints: nil,
	}
	if recipe.Recipe.IsSatisfied != nil {
		mon := ws.PlanTarget{
			Type:        recipe.Recipe.IsSatisfied.Type,
			Name:        recipe.Recipe.IsSatisfied.Name,
			Description: "isSatisfied pre-check",
		}
		// Always set When — field defaults to "final_result" when omitted.
		cond := recipe.Recipe.IsSatisfied.When
		mon.When = &cond
		plan.Monitor = []ws.PlanTarget{mon}
	}

	result := p.PlanFromWantType(name, recipe.Recipe.Metadata.Category, plan, globalParams)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		http.Error(w, "failed to encode result: "+err.Error(), http.StatusInternalServerError)
	}
}

// POST /api/v1/planner/plan  (ad-hoc inline body)
//
// Accepts an inline WantTypePlan (YAML or JSON) and returns the derived PlannerResult.
// Useful for testing plan derivation without a registered recipe.
//
// Request body:
//
//	achieve:
//	  - type: smartgolf_book
//	isSatisfied:
//	  type: smartgolf_check_reserved
//	  when: {field: is_reserved, operator: "==", value: true}
//	hints:
//	  - for: smartgolf_book
//	    use: smartgolf_list_available
func (s *Server) planFromBody(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var plan ws.WantTypePlan
	if err := yaml.Unmarshal(body, &plan); err != nil {
		http.Error(w, "invalid WantTypePlan: "+err.Error(), http.StatusBadRequest)
		return
	}

	defs := s.globalBuilder.AllWantTypeDefinitions()
	wsDefs := make(map[string]*ws.WantTypeDefinition, len(defs))
	for k, v := range defs {
		wsDefs[k] = v
	}
	idx := planner.BuildExposableIndexFromDefs(wsDefs)
	p := planner.New(idx, wsDefs)

	result := p.PlanFromWantType("adhoc", "", &plan, nil)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		http.Error(w, "failed to encode result: "+err.Error(), http.StatusInternalServerError)
	}
}

// GET /api/v1/planner/index
//
// Returns the full exposable capability index.
func (s *Server) getPlannerIndex(w http.ResponseWriter, r *http.Request) {
	defs := s.globalBuilder.AllWantTypeDefinitions()
	wsDefs := make(map[string]*ws.WantTypeDefinition, len(defs))
	for k, v := range defs {
		wsDefs[k] = v
	}
	idx := planner.BuildExposableIndexFromDefs(wsDefs)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"fields": idx.All(),
		"count":  len(idx.All()),
	}); err != nil {
		http.Error(w, "failed to encode index: "+err.Error(), http.StatusInternalServerError)
	}
}
