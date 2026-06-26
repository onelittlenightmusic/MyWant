package types

import (
	"context"
	"fmt"
	"sort"
	"strings"

	. "mywant/engine/core"
)

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
		approved, _ := reaction["approved"].(bool)
		if !approved {
			webFormMonitorCleanupQueue(want)
			want.StoreLog("[WEB-FORM] rejected — resetting to waiting")
			return false, nil
		}
		if err := webFormMonitorSubmit(ctx, want); err != nil {
			want.StoreLog("[WEB-FORM] submit failed: %v", err)
			return false, nil
		}
		want.SetCurrent("plan_snapshot", webFormBuildPlanSnapshot(want))
		webFormMonitorCleanupQueue(want)
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
		want.StoreLog("[WEB-FORM] field=%s getPlan=(%v,%v)", sd.Name, v, ok)
		if !ok || s == "" {
			return false
		}
	}
	return hasPlan
}

// webFormMonitorSubmit calls POST /api/v1/web-wants/{name}/launch with plan field values.
func webFormMonitorSubmit(_ context.Context, want *Want) error {
	hc := want.GetHTTPClient()
	if hc == nil {
		return fmt.Errorf("no HTTP client available")
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
		"cdp_host":     want.GetStringParam("debug_chrome_host", "localhost"),
		"cdp_port":     want.GetStringParam("debug_chrome_port", "9222"),
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
