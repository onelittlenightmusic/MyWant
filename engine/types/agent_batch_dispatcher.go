package types

import (
	"context"
	"encoding/json"
	"fmt"

	. "mywant/engine/core"
)

const parallelizeDispatchCapability = "parallelize_dispatch"

func init() {
	RegisterWithInit(func() {
		RegisterThinkAgentType(
			"parallelize_dispatcher_agent",
			[]Capability{{Name: parallelizeDispatchCapability, Gives: []string{parallelizeDispatchCapability}, Description: "Dispatches sibling wants for each entry in child_params list"}},
			parallelizeDispatcherThink,
		)
	})
}

func parallelizeDispatcherThink(ctx context.Context, want *Want) error {
	dispatchCount := GetCurrent(want, "dispatch_count", 0)

	// Phase 1: propose dispatch once
	if !GetCurrent(want, "all_dispatched", false) {
		childParams := parseChildParams(want)
		childType := GetCurrent(want, "child_type", "")
		if childType == "" || len(childParams) == 0 {
			want.SetCurrent("all_dispatched", true)
			want.SetCurrent("dispatch_count", 0)
			want.SetCurrent("siblings_done", true)
			return nil
		}

		requests := make([]DispatchRequest, 0, len(childParams))
		for i, params := range childParams {
			requests = append(requests, DispatchRequest{
				Direction:   fmt.Sprintf("item-%d", i),
				RequesterID: want.Metadata.ID,
				Type:        childType,
				Params:      params,
			})
		}
		want.ProposeDispatch(requests)
		want.SetCurrent("all_dispatched", true)
		want.SetCurrent("dispatch_count", len(childParams))
		dispatchCount = len(childParams)
		want.StoreLog("[Parallelize] Proposed dispatch for %d %s wants", len(childParams), childType)
		return nil
	}

	// Phase 2: stay alive until all dispatched siblings complete
	if dispatchCount == 0 {
		want.SetCurrent("siblings_done", true)
		return nil
	}

	rawDispatched, ok := want.GetParentState("_dispatched_directions")
	if !ok || rawDispatched == nil {
		return nil
	}

	doneCount := 0
	switch dm := rawDispatched.(type) {
	case map[string]string:
		for _, v := range dm {
			if v == "DONE" {
				doneCount++
			}
		}
	case map[string]any:
		for _, v := range dm {
			if id, ok := v.(string); ok && id == "DONE" {
				doneCount++
			}
		}
	default:
		return nil
	}
	want.StoreLog("[Parallelize] %d/%d siblings done", doneCount, dispatchCount)

	if doneCount >= dispatchCount {
		want.SetCurrent("siblings_done", true)
		want.StoreLog("[Parallelize] All %d siblings completed", dispatchCount)
	}
	return nil
}

// parseChildParams parses child_params from want state.
// Accepts []map[string]any directly, or a JSON array string.
func parseChildParams(want *Want) []map[string]any {
	raw := GetCurrent[any](want, "child_params", nil)
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []map[string]any:
		return v
	case []any:
		result := make([]map[string]any, 0, len(v))
		for _, item := range v {
			switch m := item.(type) {
			case map[string]any:
				result = append(result, m)
			case map[any]any:
				converted := make(map[string]any, len(m))
				for k, val := range m {
					converted[fmt.Sprintf("%v", k)] = val
				}
				result = append(result, converted)
			}
		}
		return result
	case string:
		var arr []map[string]any
		if err := json.Unmarshal([]byte(v), &arr); err == nil {
			return arr
		}
	}
	return nil
}
