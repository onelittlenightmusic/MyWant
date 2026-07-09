package types

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	. "mywant/engine/core"
)

var rePlanPlaceholder = regexp.MustCompile(`\{\{plan\.([a-zA-Z0-9_]+)\}\}`)

func init() {
	RegisterWithInit(func() {
		RegisterMonitorAgent("monitor_web_form_phase", monitorWebFormPhase)
	})
}

func monitorWebFormPhase(ctx context.Context, want *Want) (bool, error) {
	phase := ""
	if v, ok := want.GetCurrent("phase"); ok {
		phase, _ = v.(string)
	}
	if phase == "" {
		phase = "waiting"
	}

	switch phase {
	case "done":
		// Plan unchanged — stay in done
		if webFormBuildPlanSnapshot(want) == GetCurrent(want, "plan_snapshot", "") {
			return false, nil
		}
		// Plan changed — reset and immediately evaluate as "waiting"
		want.SetCurrent("phase", "waiting")
		want.SetCurrent("user_reaction", map[string]any{})
		fallthrough

	case "waiting":
		filled := webFormAllPlanFilled(want)
		if !filled {
			return false, nil
		}
		hc := want.GetHTTPClient()
		if hc == nil {
			return false, nil
		}
		queueID, err := webFormMonitorCreateQueue(hc)
		if err != nil {
			want.StoreLog("[WEB-FORM] failed to create reaction queue: %v", err)
			return false, nil
		}
		want.SetCurrent("reaction_queue_id", queueID)
		want.SetCurrent("phase", "ready")
		want.SetStatus(WantStatusWaitingUserAction)
		want.StoreLog("[WEB-FORM] awaiting approval (queue: %s)", queueID)

	case "ready":
		// Plan cleared → reset
		if !webFormAllPlanFilled(want) {
			webFormMonitorCleanupQueue(want)
			return false, nil
		}
		ur, _ := want.GetCurrent("user_reaction")
		reaction, _ := ur.(map[string]any)
		if len(reaction) == 0 {
			return false, nil
		}
		want.StoreLog("[TIMING] monitor picked up user_reaction — calling submit")
		approved, _ := reaction["approved"].(bool)
		if !approved {
			webFormMonitorCleanupQueue(want)
			want.SetStatus(WantStatusReaching)
			want.StoreLog("[WEB-FORM] rejected — resetting to waiting")
			return false, nil
		}
		if err := webFormMonitorSubmit(ctx, want); err != nil {
			want.StoreLog("[WEB-FORM] submit failed: %v", err)
			return false, nil
		}
		want.StoreLog("[TIMING] pending_device_action written to state")
		want.SetCurrent("plan_snapshot", webFormBuildPlanSnapshot(want))
		webFormMonitorCleanupQueue(want)
		want.SetStatus(WantStatusReaching)
		want.SetCurrent("phase", "done")
		want.StoreLog("[WEB-FORM] form submitted successfully")
		return false, nil
	}

	return false, nil
}

// webFormBuildPlanSnapshot returns a deterministic string of all non-system plan field values.
func webFormBuildPlanSnapshot(want *Want) string {
	typeDef := want.WantTypeDefinition
	if typeDef == nil {
		return ""
	}
	reserved := make(map[string]bool, len(SystemReservedStateFields()))
	for _, f := range SystemReservedStateFields() {
		reserved[f] = true
	}
	pairs := []string{}
	for _, sd := range typeDef.State {
		if reserved[sd.Name] {
			continue
		}
		label, exists := want.StateLabels[sd.Name]
		if !exists || label != LabelPlan {
			continue
		}
		if sd.Type == "bool" || sd.Type == "boolean" {
			continue
		}
		v, _ := want.GetPlan(sd.Name)
		s, _ := v.(string)
		pairs = append(pairs, sd.Name+"="+s)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

// webFormAllPlanFilled returns true when every non-boolean, non-system plan field has a non-empty value.
func webFormAllPlanFilled(want *Want) bool {
	typeDef := want.WantTypeDefinition
	if typeDef == nil {
		return false
	}
	reserved := make(map[string]bool, len(SystemReservedStateFields()))
	for _, f := range SystemReservedStateFields() {
		reserved[f] = true
	}
	hasPlan := false
	for _, sd := range typeDef.State {
		if reserved[sd.Name] {
			continue
		}
		label, exists := want.StateLabels[sd.Name]
		if !exists || label != LabelPlan {
			continue
		}
		if sd.Type == "bool" || sd.Type == "boolean" {
			continue
		}
		hasPlan = true
		v, ok := want.GetPlan(sd.Name)
		s, _ := v.(string)
		if !ok || s == "" {
			return false
		}
	}
	return hasPlan
}

// webFormMonitorSubmit calls POST /api/v1/web-wants/{name}/launch with plan field values.
// If the want type has a url-template label, the URL is built directly from plan fields
// (GET query param mode) and no CDP form filling is performed.
func webFormMonitorSubmit(_ context.Context, want *Want) error {
	hc := want.GetHTTPClient()
	if hc == nil {
		return fmt.Errorf("no HTTP client available")
	}

	urlTemplate := ""
	if want.WantTypeDefinition != nil {
		urlTemplate = want.WantTypeDefinition.Metadata.Labels["url-template"]
	}

	if urlTemplate != "" {
		builtURL := webFormBuildURL(urlTemplate, want)
		want.StoreLog("[WEB-FORM] URL-complete mode → open on device: %s", builtURL)
		want.SetCurrent("pending_device_action", map[string]any{
			"type": "open-url",
			"url":  builtURL,
		})
		return nil
	}

	fieldValues := map[string]string{}
	if typeDef := want.WantTypeDefinition; typeDef != nil {
		for _, sd := range typeDef.State {
			label, exists := want.StateLabels[sd.Name]
			if !exists || label != LabelPlan {
				continue
			}
			if sd.Type == "bool" || sd.Type == "boolean" {
				fieldValues[sd.Name] = "true"
				continue
			}
			v, ok := want.GetPlan(sd.Name)
			if s, _ := v.(string); ok && s != "" {
				fieldValues[sd.Name] = s
			}
		}
	}
	payload := map[string]any{
		"target_url":   want.GetStringParam("target_url", ""),
		"field_values": fieldValues,
	}

	path := fmt.Sprintf("/api/v1/web-wants/%s/launch", want.Metadata.Type)
	resp, err := hc.POST(path, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("launch API returned %d", resp.StatusCode)
	}
	return nil
}

// webFormBuildURL substitutes {{plan.fieldname}} placeholders in tmpl with URL-encoded plan values.
func webFormBuildURL(tmpl string, want *Want) string {
	return rePlanPlaceholder.ReplaceAllStringFunc(tmpl, func(match string) string {
		subs := rePlanPlaceholder.FindStringSubmatch(match)
		if len(subs) < 2 {
			return match
		}
		fieldName := subs[1]
		v, ok := want.GetPlan(fieldName)
		if !ok {
			want.StoreLog("[WEB-FORM] url-template: plan.%s not found", fieldName)
			return match
		}
		s, _ := v.(string)
		want.StoreLog("[WEB-FORM] url-template: plan.%s = %q", fieldName, s)
		return url.QueryEscape(s)
	})
}

func webFormMonitorCreateQueue(hc *HTTPClient) (string, error) {
	resp, err := hc.POST("/api/v1/reactions/", nil)
	if err != nil {
		return "", err
	}
	var result struct {
		QueueID string `json:"queue_id"`
	}
	if err := hc.DecodeJSON(resp, &result); err != nil {
		return "", err
	}
	return result.QueueID, nil
}

func webFormMonitorCleanupQueue(want *Want) {
	queueID := ""
	if v, ok := want.GetCurrent("reaction_queue_id"); ok {
		queueID, _ = v.(string)
	}
	if queueID != "" {
		if hc := want.GetHTTPClient(); hc != nil {
			path := fmt.Sprintf("/api/v1/reactions/%s", queueID)
			if resp, err := hc.DELETE(path); err == nil {
				resp.Body.Close()
			}
		}
	}
	want.SetCurrent("reaction_queue_id", "")
	want.SetCurrent("user_reaction", map[string]any{})
	want.SetCurrent("phase", "waiting")
}
