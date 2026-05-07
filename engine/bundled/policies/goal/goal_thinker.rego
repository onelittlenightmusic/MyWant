package planner

# Goal Thinker Policy
#
# input.goal.goal_text  = user's original goal (string, label: goal)
# input.goal.targets    = approved breakdown items (array, label: goal)
#   Each item: {name, type, description, params}
# input.current.<name>_done = true when that sub-goal's child want has completed (via Sets)
# input.current.available_capabilities = ["visa_expert", ...] (computed from achievements)
#
# For each target in input.goal.targets, output its name as an action if not yet done.
# OPA replans automatically whenever current state changes (hash detection in opaLLMThinkerThink).

import future.keywords.if

missing[action] if {
    target := input.goal.targets[_]
    done_key := sprintf("%s_done", [target.name])
    not input.current[done_key]
    action := target.name
}
