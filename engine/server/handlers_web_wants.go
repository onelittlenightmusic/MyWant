package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/mux"
	mywant "mywant/engine/core"
	"mywant/engine/types"
)

// WebWantElement describes a single interactive element captured by the inspector.
type WebWantElement struct {
	Role     string `json:"role"`
	Name     string `json:"name"`
	Selector string `json:"selector,omitempty"`
	FieldKey string `json:"field_key,omitempty"` // ASCII param key for textbox inputs (empty for buttons)
	HtmlName string `json:"html_name,omitempty"` // HTML name attribute of the element (e.g. "q" for Google search)
}

// createWebWantRequest is the body for POST /api/v1/web-wants/create.
type createWebWantRequest struct {
	Name        string                      `json:"name"`
	Title       string                      `json:"title,omitempty"`
	URL         string                      `json:"url"`
	Hostname    string                      `json:"hostname,omitempty"`
	Elements    []WebWantElement            `json:"elements"`
	AllData     map[string][]WebWantElement `json:"all_data,omitempty"`
	URLTemplate string                      `json:"url_template,omitempty"`
}

var validTypeName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var reName = regexp.MustCompile(`\[name=["']?([^"'\]]+)["']?\]`)
var reID = regexp.MustCompile(`\[id=["']?([^"'\]]+)["']?\]`)
var reNonASCII = regexp.MustCompile(`[^a-zA-Z0-9_]`)

// isInputRole returns true for roles that accept text input.
func isInputRole(role string) bool {
	r := strings.ToLower(strings.TrimSpace(role))
	switch r {
	case "textbox", "searchbox", "combobox", "spinbutton", "slider", "input":
		return true
	}
	return false
}

// fieldKeyFromSelector derives an ASCII field key from a CSS selector.
func fieldKeyFromSelector(sel string) string {
	if m := reName.FindStringSubmatch(sel); len(m) > 1 {
		return sanitizeFieldKey(m[1])
	}
	if strings.HasPrefix(sel, "#") {
		return sanitizeFieldKey(strings.TrimPrefix(sel, "#"))
	}
	if m := reID.FindStringSubmatch(sel); len(m) > 1 {
		return sanitizeFieldKey(m[1])
	}
	return ""
}

func sanitizeFieldKey(s string) string {
	r := reNonASCII.ReplaceAllString(s, "_")
	r = strings.ToLower(r)
	if len(r) == 0 {
		return ""
	}
	if r[0] >= '0' && r[0] <= '9' {
		r = "f_" + r
	}
	return r
}

// enrichElements assigns FieldKey to every captured element.
// Input roles  → field key derived from selector/name (e.g. "email").
// Button roles → "click_" + sanitized element name (e.g. "click_login").
func enrichElements(elements []WebWantElement) []WebWantElement {
	usedKeys := map[string]bool{}
	result := make([]WebWantElement, len(elements))
	autoInputIdx := 0
	autoButtonIdx := 0
	for i, el := range elements {
		enriched := el
		if isInputRole(el.Role) {
			key := sanitizeFieldKey(el.Name)
			if key == "" {
				key = fieldKeyFromSelector(el.Selector)
			}
			if key == "" {
				autoInputIdx++
				key = fmt.Sprintf("field_%d", autoInputIdx)
			}
			base := key
			for n := 2; usedKeys[key]; n++ {
				key = fmt.Sprintf("%s_%d", base, n)
			}
			usedKeys[key] = true
			enriched.FieldKey = key
		} else {
			// Button / link / other clickable: derive key from element name
			key := ""
			if el.Name != "" {
				key = "click_" + sanitizeFieldKey(el.Name)
			}
			if key == "" || key == "click_" {
				autoButtonIdx++
				key = fmt.Sprintf("click_btn_%d", autoButtonIdx)
			}
			base := key
			for n := 2; usedKeys[key]; n++ {
				key = fmt.Sprintf("%s_%d", base, n)
			}
			usedKeys[key] = true
			enriched.FieldKey = key
		}
		result[i] = enriched
	}
	return result
}

// createWebWant handles POST /api/v1/web-wants/create
func (s *Server) createWebWant(w http.ResponseWriter, r *http.Request) {
	var req createWebWantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	name := strings.ToLower(strings.ReplaceAll(req.Name, " ", "_"))
	name = strings.ReplaceAll(name, "-", "_")
	if !validTypeName.MatchString(name) {
		http.Error(w, "invalid name: must match [a-z][a-z0-9_-]{0,63}", http.StatusBadRequest)
		return
	}
	if req.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}

	title := req.Title
	if title == "" {
		title = req.Name
	}

	hostname := req.Hostname
	if hostname == "" && req.URL != "" {
		u := req.URL
		if i := strings.Index(u, "://"); i >= 0 {
			u = u[i+3:]
		}
		if i := strings.Index(u, "/"); i >= 0 {
			u = u[:i]
		}
		hostname = u
	}

	elements := req.Elements
	if elements == nil && req.AllData != nil {
		if v, ok := req.AllData[hostname]; ok {
			elements = v
		}
	}

	// Enrich elements with field_key for textbox-like roles
	elements = enrichElements(elements)

	dir := filepath.Join(mywant.UserCustomTypesDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		http.Error(w, "failed to create directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Write elements.json (includes field_key for textboxes)
	elemJSON, _ := json.MarshalIndent(map[string][]WebWantElement{hostname: elements}, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "elements.json"), elemJSON, 0o644); err != nil {
		http.Error(w, "failed to write elements.json: "+err.Error(), http.StatusInternalServerError)
		return
	}

	yamlContent := buildWebWantYAML(name, title, req.URL, hostname, req.URLTemplate, elements)
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(yamlContent), 0o644); err != nil {
		http.Error(w, "failed to write YAML: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(filepath.Join(dir, "main.py"), []byte(buildWebWantMainPy(req.URL, elements)), 0o755); err != nil {
		http.Error(w, "failed to write main.py: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(buildWebWantSkillMd(name, title, req.URL, elements)), 0o644); err != nil {
		http.Error(w, "failed to write SKILL.md: "+err.Error(), http.StatusInternalServerError)
		return
	}

	loaded, warnings := s.reloadUserCustomTypesAndSync()
	s.globalBuilder.LogAPIOperation("POST", "/web-wants/create", name, "created", loaded, "", "")

	s.JSONResponse(w, http.StatusCreated, map[string]any{
		"name":     name,
		"dir":      dir,
		"loaded":   loaded,
		"warnings": warnings,
		"message":  fmt.Sprintf("web want type %q created successfully", name),
	})
}

func buildWebWantYAML(name, title, url, hostname, urlTemplate string, elements []WebWantElement) string {
	var elemComments strings.Builder
	var inputStateFields strings.Builder
	var buttonStateFields strings.Builder

	for _, el := range elements {
		if isInputRole(el.Role) {
			elemComments.WriteString(fmt.Sprintf("    # - [input] %s  selector: %s\n", el.Name, el.Selector))
			if el.FieldKey != "" {
				subType := "text"
				if el.Role == "combobox" {
					subType = "select"
				}
				inputStateFields.WriteString(fmt.Sprintf(`
    - name: %s
      description: "Value for \"%s\" (%s)"
      type: string
      subType: %s
      label: plan
      persistent: true
      initialValue: ""
`, el.FieldKey, el.Name, el.Role, subType))
			}
		} else {
			elemComments.WriteString(fmt.Sprintf("    # - [button] %s  selector: %s\n", el.Name, el.Selector))
			if el.FieldKey != "" {
				buttonStateFields.WriteString(fmt.Sprintf(`
    - name: %s
      description: "Click \"%s\" (%s) — set by the want after form submission"
      type: bool
      label: current
      persistent: true
      initialValue: false
`, el.FieldKey, el.Name, el.Role))
			}
		}
	}

	elementStateBlock := ""
	if inputStateFields.Len() > 0 {
		elementStateBlock += "\n    # — form field values —" + inputStateFields.String()
	}
	if buttonStateFields.Len() > 0 {
		elementStateBlock += "\n    # — buttons —" + buttonStateFields.String()
	}

	// Rewrite url-template placeholders from HTML element names to field_keys.
	// e.g. {{plan.q}} → {{plan.search}} when the user named the element "Search"
	rewrittenTemplate := urlTemplate
	for _, el := range elements {
		if !isInputRole(el.Role) || el.FieldKey == "" {
			continue
		}
		// Prefer the explicit html_name; fall back to extracting from selector.
		htmlName := el.HtmlName
		if htmlName == "" {
			if m := reName.FindStringSubmatch(el.Selector); len(m) > 1 {
				htmlName = m[1]
			}
		}
		if htmlName != "" && htmlName != el.FieldKey {
			rewrittenTemplate = strings.ReplaceAll(rewrittenTemplate,
				"{{plan."+htmlName+"}}", "{{plan."+el.FieldKey+"}}")
		}
	}

	urlTemplateLabel := ""
	if rewrittenTemplate != "" {
		urlTemplateLabel = fmt.Sprintf("\n      url-template: %q", rewrittenTemplate)
	}

	return fmt.Sprintf(`wantType:
  metadata:
    name: %s
    title: %s
    description: |
      Opens %s in an existing Chrome browser.  Set plan state fields to
      auto-fill form elements.  Pre-configured elements for %s:
%s    version: '1.0'
    category: web
    pattern: independent
    labels:
      category-icon: "Globe"
      category-bg-light: "linear-gradient(160deg, #bfdbfe 0%%, #ddd6fe 100%%)"
      category-bg-dark:  "linear-gradient(160deg, #1e3a5f 0%%, #2d1b69 100%%)"
      source-url: %q%s

  parameters:
    - name: target_url
      description: URL to open (defaults to the captured site)
      type: string
      required: false
      default: %q

    - name: debug_chrome_host
      description: Hostname of Chrome with --remote-debugging-port
      type: string
      required: false
      default: "localhost"

    - name: debug_chrome_port
      description: Remote debugging port
      type: string
      required: false
      default: "9222"

  state:
    - name: status
      description: Current status (idle / active / done)
      type: string
      label: current
      persistent: true
      initialValue: "idle"

    - name: active_element
      description: Currently focused element name
      type: string
      label: current
      persistent: true
      initialValue: ""

    - name: phase
      description: "Submission phase: waiting / ready / done"
      type: string
      label: current
      persistent: true
      initialValue: "waiting"

    - name: reaction_queue_id
      description: Reaction queue ID for user approval
      type: string
      label: current
      persistent: true
      initialValue: ""

    - name: user_reaction
      description: User reaction result from approval UI
      type: object
      label: current
      persistent: true
      initialValue: {}

    - name: plan_snapshot
      description: Snapshot of plan field values at last submission
      type: string
      label: current
      persistent: true
      initialValue: ""

    - name: pending_device_action
      description: Open-URL action pushed to connected devices (cleared after use)
      type: object
      label: current
      persistent: false
%s
  requires:
    - reminder_monitoring
    - web_form_monitoring

  finalResultField: status
`, name, title, url, hostname, elemComments.String(), url, urlTemplateLabel, url, elementStateBlock)
}

// launchWebWant handles POST /api/v1/web-wants/{name}/launch
func (s *Server) launchWebWant(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		if vars := mux.Vars(r); vars != nil {
			name = vars["name"]
		}
	}
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	dir := filepath.Join(mywant.UserCustomTypesDir(), name)
	elemFile := filepath.Join(dir, "elements.json")
	data, err := os.ReadFile(elemFile)
	if err != nil {
		http.Error(w, "elements.json not found for: "+name, http.StatusNotFound)
		return
	}

	var allElems map[string][]WebWantElement
	if err := json.Unmarshal(data, &allElems); err != nil {
		http.Error(w, "failed to parse elements.json: "+err.Error(), http.StatusBadRequest)
		return
	}

	var body struct {
		TargetURL    string            `json:"target_url"`
		CDPHost      string            `json:"cdp_host"`
		CDPPort      string            `json:"cdp_port"`
		FieldValues  map[string]string `json:"field_values,omitempty"`
		NavigateOnly bool              `json:"navigate_only,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	cdpHost := body.CDPHost
	if cdpHost == "" {
		cdpHost = "localhost"
	}
	cdpPort := body.CDPPort
	if cdpPort == "" {
		cdpPort = "9222"
	}

	targetURL := body.TargetURL
	if targetURL == "" {
		for hostname := range allElems {
			targetURL = "https://" + hostname
			break
		}
	}

	var elements []WebWantElement
	for _, elems := range allElems {
		elements = append(elements, elems...)
	}
	if len(elements) == 0 {
		http.Error(w, "no elements found in elements.json", http.StatusBadRequest)
		return
	}

	cdpURL := fmt.Sprintf("http://%s:%s", cdpHost, cdpPort)

	navElems := make([]types.WebNavElement, len(elements))
	for i, el := range elements {
		navElems[i] = types.WebNavElement{
			Role:     el.Role,
			Name:     el.Name,
			Selector: el.Selector,
			FieldKey: el.FieldKey,
		}
	}

	// navigate_only: just open the URL in Chrome without filling or overlay (url-template mode).
	if body.NavigateOnly {
		go func() {
			if err := types.NavigateTab(context.Background(), cdpURL, targetURL); err != nil {
				log.Printf("[WEB-WANT] navigate error for %s: %v", name, err)
			}
		}()
		s.JSONResponse(w, http.StatusAccepted, map[string]any{
			"ok":      true,
			"url":     targetURL,
			"mode":    "navigate",
			"message": fmt.Sprintf("navigating to %s (background)", targetURL),
		})
		return
	}

	// Auto-fill mode when field_values are provided; run in background and return immediately.
	if len(body.FieldValues) > 0 {
		go func() {
			if err := types.FillAndSubmitForm(context.Background(), cdpURL, targetURL, navElems, body.FieldValues); err != nil {
				log.Printf("[WEB-WANT] fill form error for %s: %v", name, err)
			}
		}()
		s.JSONResponse(w, http.StatusAccepted, map[string]any{
			"ok":      true,
			"url":     targetURL,
			"mode":    "fill",
			"fields":  len(body.FieldValues),
			"message": fmt.Sprintf("filling %d field(s) on %s (background)", len(body.FieldValues), name),
		})
		return
	}

	if err := types.OpenWebNavigator(r.Context(), cdpURL, targetURL, navElems); err != nil {
		http.Error(w, "failed to launch navigator: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.JSONResponse(w, http.StatusOK, map[string]any{
		"ok":       true,
		"url":      targetURL,
		"mode":     "nav",
		"elements": len(elements),
		"message":  fmt.Sprintf("launched %s with %d elements", name, len(elements)),
	})
}

// navCallback is a no-op endpoint consumed by the navigation overlay's "Done" post.
func (s *Server) navCallback(w http.ResponseWriter, _ *http.Request) {
	s.JSONResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

// suggestNameRequest is the body for POST /api/v1/web-wants/suggest-name.
type suggestNameRequest struct {
	HTML string `json:"html"`
}

// suggestNameResponse always returns HTTP 200 — this is a fail-open, best-effort
// suggestion; callers should just check Name for truthiness and never branch on
// HTTP status.
type suggestNameResponse struct {
	Name  string `json:"name"`
	Error string `json:"error,omitempty"`
}

// suggestElementName asks the lightweight one-shot `claude --print` helper for
// a short name describing an inspector element's surrounding HTML. Invoked by
// the Web Inspector overlay (injected into the page being inspected, hence a
// cross-origin fetch — see corsMiddleware) speculatively while its naming
// dialog is open and not yet focused by the user.
func (s *Server) suggestElementName(w http.ResponseWriter, r *http.Request) {
	var req suggestNameRequest
	if err := DecodeRequest(r, &req); err != nil || req.HTML == "" {
		s.JSONResponse(w, http.StatusOK, suggestNameResponse{Error: "html is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	name, err := types.SuggestElementName(ctx, req.HTML)
	if err != nil {
		s.JSONResponse(w, http.StatusOK, suggestNameResponse{Error: err.Error()})
		return
	}
	s.JSONResponse(w, http.StatusOK, suggestNameResponse{Name: name})
}

func buildWebWantMainPy(url string, elements []WebWantElement) string {
	elemJSON, _ := json.Marshal(elements)

	return fmt.Sprintf(`#!/usr/bin/env python3
"""Web want: auto-fills form fields from parameters, or opens a navigation overlay."""
import json, os, sys, urllib.request, urllib.error

MYWANT_API = os.environ.get("MYWANT_URL", "http://localhost:8080")
TARGET_URL = %q
ELEMENTS   = %s
WANT_NAME  = os.path.basename(os.path.dirname(os.path.abspath(__file__)))

_SYSTEM_PARAMS = {"target_url", "debug_chrome_host", "debug_chrome_port"}


def report(p, m=""):
    print(json.dumps({"_progress": p, "_message": m}), flush=True)


def call_launch(payload_dict):
    payload = json.dumps(payload_dict).encode()
    req = urllib.request.Request(
        f"{MYWANT_API}/api/v1/web-wants/{WANT_NAME}/launch",
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.loads(r.read())


def main():
    raw = sys.argv[1] if len(sys.argv) > 1 else "{}"
    arg = json.loads(raw or "{}")
    target_url = arg.get("target_url", TARGET_URL)
    cdp_host   = arg.get("debug_chrome_host", "localhost")
    cdp_port   = arg.get("debug_chrome_port", "9222")

    # Collect field values for input elements that have a field_key in params
    field_values = {}
    for el in ELEMENTS:
        fk = el.get("field_key") or ""
        if fk and fk in arg and str(arg[fk]).strip():
            field_values[fk] = str(arg[fk])

    launch_payload = {
        "target_url": target_url,
        "cdp_host":   cdp_host,
        "cdp_port":   cdp_port,
    }
    if field_values:
        launch_payload["field_values"] = field_values
        report(20, f"auto-filling {len(field_values)} field(s) on {target_url}")
    else:
        report(20, "launching browser navigation overlay")

    try:
        result = call_launch(launch_payload)
        report(100, result.get("message", "done"))
        print(json.dumps({"status": "active", "url": target_url,
                          "mode": result.get("mode", "nav")}), flush=True)
    except Exception as e:
        print(json.dumps({"status": "error", "error": str(e)}), flush=True)
        sys.exit(1)


if __name__ == "__main__":
    main()
`, url, string(elemJSON))
}

func buildWebWantSkillMd(name, title, url string, elements []WebWantElement) string {
	var elemList strings.Builder
	for _, el := range elements {
		fkNote := ""
		if el.FieldKey != "" {
			fkNote = fmt.Sprintf(" → state field `%s` (plan)", el.FieldKey)
		}
		elemList.WriteString(fmt.Sprintf("- **%s** (%s)%s\n", el.Name, el.Role, fkNote))
	}

	return fmt.Sprintf(`# %s

Web want type for **%s**.

Set plan state fields to auto-fill form elements on launch.
Without plan state values, opens an interactive navigation overlay.

## Elements

%s
## State Fields (plan)

Set these plan state fields to auto-fill the form:

| Field | Element | Role |
|-------|---------|------|
`, title, url, elemList.String()) + buildParamTable(elements) + "\n"
}

func buildParamTable(elements []WebWantElement) string {
	var sb strings.Builder
	for _, el := range elements {
		if el.FieldKey != "" {
			sb.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", el.FieldKey, el.Name, el.Role))
		}
	}
	return sb.String()
}
