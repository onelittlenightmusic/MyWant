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
		mywant.RegisterDoAgent("rpg_hook_agent", manageRPGHook)
	})
}

// manageRPGHook registers or unregisters a filtered lifecycle rule, driven by
// the rpg_hook_plan state field ("register"/"unregister").
func manageRPGHook(_ context.Context, want *mywant.Want) error {
	plan := mywant.GetPlan(want, "rpg_hook_plan", "")
	switch plan {
	case "register":
		return rpgHookRegister(want)
	case "unregister":
		return rpgHookUnregister(want)
	}
	return nil
}

func rpgHookRegister(want *mywant.Want) error {
	existing := mywant.GetCurrent(want, "hook_rule_id", "")
	if existing != "" {
		want.StoreLog("[RPG-HOOK] already registered as %s", existing)
		want.SetCurrent("status", "running")
		return nil
	}

	event := mywant.GetCurrent(want, "rpg_event", "want_created")
	match := mywant.LifecycleRuleMatch{
		Type:  mywant.GetCurrent(want, "rpg_match_type", ""),
		Owner: mywant.GetCurrent(want, "rpg_match_owner", ""),
		Name:  mywant.GetCurrent(want, "rpg_match_name", ""),
	}
	targetURL := mywant.GetCurrent(want, "rpg_target_url", "")
	if targetURL == "" {
		return fmt.Errorf("rpg_target_url is required")
	}

	metadata := buildRPGMetadata(want)
	mywantURL := mywant.GetCurrent(want, "mywant_url", "http://localhost:8080")

	var ruleID string

	// Try in-process registration first (when this agent runs inside mywant itself)
	if mywant.RegisterLifecycleRule != nil {
		ruleID = mywant.RegisterLifecycleRule(event, targetURL, match, metadata)
		want.StoreLog("[RPG-HOOK] registered in-process rule %s", ruleID)
	} else {
		// Fallback: HTTP
		var err error
		ruleID, err = postLifecycleRuleHTTP(mywantURL, event, match, targetURL, metadata)
		if err != nil {
			return fmt.Errorf("failed to register lifecycle rule: %w", err)
		}
		want.StoreLog("[RPG-HOOK] registered via HTTP rule %s", ruleID)
	}

	want.SetCurrent("hook_rule_id", ruleID)
	want.SetCurrent("status", "running")
	return nil
}

func rpgHookUnregister(want *mywant.Want) error {
	ruleID := mywant.GetCurrent(want, "hook_rule_id", "")
	if ruleID == "" {
		want.SetCurrent("status", "stopped")
		return nil
	}

	mywantURL := mywant.GetCurrent(want, "mywant_url", "http://localhost:8080")

	if mywant.UnregisterLifecycleRule != nil {
		mywant.UnregisterLifecycleRule(ruleID)
		want.StoreLog("[RPG-HOOK] unregistered in-process rule %s", ruleID)
	} else {
		if err := deleteLifecycleRuleHTTP(mywantURL, ruleID); err != nil {
			want.StoreLog("[RPG-HOOK] HTTP unregister failed for %s: %v", ruleID, err)
		} else {
			want.StoreLog("[RPG-HOOK] unregistered via HTTP rule %s", ruleID)
		}
	}

	want.SetCurrent("hook_rule_id", "")
	want.SetCurrent("status", "stopped")
	return nil
}

func buildRPGMetadata(want *mywant.Want) map[string]any {
	meta := map[string]any{}
	if v := mywant.GetCurrent(want, "rpg_activate", ""); v != "" {
		meta["activate"] = v
	}
	if v := mywant.GetCurrent(want, "rpg_skip_if_achievement", ""); v != "" {
		meta["skip_if_achievement"] = v
	}
	if v := mywant.GetCurrent(want, "rpg_require_achievements", ""); v != "" {
		var list []string
		if err := json.Unmarshal([]byte(v), &list); err == nil {
			meta["require_achievements"] = list
		}
	}
	if v := mywant.GetCurrent(want, "rpg_run", ""); v != "" {
		var cmds []string
		if err := json.Unmarshal([]byte(v), &cmds); err == nil {
			meta["run"] = cmds
		}
	}
	return meta
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
