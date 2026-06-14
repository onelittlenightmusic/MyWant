package types

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	mywant "mywant/engine/core"
)

func init() {
	mywant.RegisterWithInit(func() {
		mywant.RegisterDoAgent("lifecycle_hook_agent", manageLifecycleHook)
	})
}

// hookRuleSpec defines one lifecycle rule inside a lifecycle_hook want.
// The metadata fields (activate, skip_if_achievement, etc.) are arbitrary
// key/value pairs forwarded as-is to the target URL — no RPG semantics here.
type hookRuleSpec struct {
	Event       string            `json:"event"`
	MatchType   string            `json:"match_type,omitempty"`
	MatchOwner  string            `json:"match_owner,omitempty"`
	MatchName   string            `json:"match_name,omitempty"`
	MatchLabels map[string]string `json:"match_labels,omitempty"`
	TargetURL   string            `json:"target_url,omitempty"` // per-rule override
	Metadata    map[string]any    `json:"metadata,omitempty"`   // forwarded verbatim to target
}

// manageLifecycleHook registers or unregisters lifecycle rules based on
// the hook_plan state field ("register" / "unregister").
func manageLifecycleHook(_ context.Context, want *mywant.Want) error {
	plan := mywant.GetPlan(want, "hook_plan", "")
	switch plan {
	case "register":
		return lifecycleHookRegister(want)
	case "unregister":
		return lifecycleHookUnregister(want)
	}
	return nil
}

func lifecycleHookRegister(want *mywant.Want) error {
	if existing := mywant.GetCurrent(want, "hook_rule_ids", ""); existing != "" && existing != "[]" {
		want.StoreLog("[LIFECYCLE-HOOK] already registered: %s", existing)
		want.SetCurrent("status", "running")
		return nil
	}

	rulesJSON := mywant.GetCurrent(want, "hook_rules", "")
	if rulesJSON == "" {
		return fmt.Errorf("hook_rules is required")
	}

	var specs []hookRuleSpec
	if err := json.Unmarshal([]byte(rulesJSON), &specs); err != nil {
		return fmt.Errorf("failed to parse hook_rules JSON: %w", err)
	}

	defaultTargetURL := mywant.GetCurrent(want, "hook_target_url", "")
	mywantURL := mywant.GetCurrent(want, "mywant_url", "http://localhost:8080")

	var ids []string
	for i, spec := range specs {
		targetURL := spec.TargetURL
		if targetURL == "" {
			targetURL = defaultTargetURL
		}
		if targetURL == "" {
			return fmt.Errorf("rule[%d]: target_url required (set per-rule or as hook_target_url param)", i)
		}

		match := mywant.LifecycleRuleMatch{
			Type:   spec.MatchType,
			Owner:  spec.MatchOwner,
			Name:   spec.MatchName,
			Labels: spec.MatchLabels,
		}

		var ruleID string
		if mywant.RegisterLifecycleRule != nil {
			ruleID = mywant.RegisterLifecycleRule(spec.Event, targetURL, match, spec.Metadata)
			want.StoreLog("[LIFECYCLE-HOOK] registered in-process rule[%d] %s (event=%s type=%s owner=%s)", i, ruleID, spec.Event, spec.MatchType, spec.MatchOwner)
		} else {
			var err error
			ruleID, err = postLifecycleRuleHTTP(mywantURL, spec.Event, match, targetURL, spec.Metadata)
			if err != nil {
				return fmt.Errorf("rule[%d]: %w", i, err)
			}
			want.StoreLog("[LIFECYCLE-HOOK] registered via HTTP rule[%d] %s", i, ruleID)
		}
		ids = append(ids, ruleID)
	}

	idsJSON, _ := json.Marshal(ids)
	want.SetCurrent("hook_rule_ids", string(idsJSON))
	want.SetCurrent("status", "running")
	return nil
}

func lifecycleHookUnregister(want *mywant.Want) error {
	mywantURL := mywant.GetCurrent(want, "mywant_url", "http://localhost:8080")

	idsJSON := mywant.GetCurrent(want, "hook_rule_ids", "")
	if idsJSON == "" || idsJSON == "[]" {
		want.SetCurrent("status", "stopped")
		return nil
	}

	var ids []string
	if err := json.Unmarshal([]byte(idsJSON), &ids); err == nil {
		for _, id := range ids {
			lifecycleUnregisterOne(want, mywantURL, id)
		}
	}
	want.SetCurrent("hook_rule_ids", "[]")
	want.SetCurrent("status", "stopped")
	return nil
}

func lifecycleUnregisterOne(want *mywant.Want, mywantURL, ruleID string) {
	if mywant.UnregisterLifecycleRule != nil {
		mywant.UnregisterLifecycleRule(ruleID)
		want.StoreLog("[LIFECYCLE-HOOK] unregistered in-process rule %s", ruleID)
	} else {
		if err := deleteLifecycleRuleHTTP(mywantURL, ruleID); err != nil {
			want.StoreLog("[LIFECYCLE-HOOK] HTTP unregister failed for %s: %v", ruleID, err)
		} else {
			want.StoreLog("[LIFECYCLE-HOOK] unregistered via HTTP rule %s", ruleID)
		}
	}
}

// ruleBody is the JSON body for POST /api/v1/lifecycle-webhooks/rules.
type ruleBody struct {
	Event     string                    `json:"event"`
	Match     mywant.LifecycleRuleMatch `json:"match"`
	TargetURL string                    `json:"target_url"`
	Metadata  map[string]any            `json:"metadata,omitempty"`
}

func postLifecycleRuleHTTP(mywantURL, event string, match mywant.LifecycleRuleMatch, targetURL string, metadata map[string]any) (string, error) {
	body, err := json.Marshal(ruleBody{
		Event:     event,
		Match:     match,
		TargetURL: targetURL,
		Metadata:  metadata,
	})
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(mywantURL+"/api/v1/lifecycle-webhooks/rules", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.ID, nil
}

func deleteLifecycleRuleHTTP(mywantURL, ruleID string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodDelete, mywantURL+"/api/v1/lifecycle-webhooks/rules/"+ruleID, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body) //nolint:errcheck
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
