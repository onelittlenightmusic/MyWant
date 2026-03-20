package planner

# input.goal    = mergeOPAInput(GetAllGoal())    → {"trip": {...}, "budget_target": N, ...}
# input.current = mergeOPAInput(GetAllCurrent()) → {"hotel_reserved": bool, ..., "opa_input_hash": "..."}

goal    := input.goal
current := input.current

# Total cost = sum of all values in current.costs (keyed by direction name).
# Populated by the itinerary from direction_map.cost_field entries, so adding new
# cost types requires no rego changes.
total_cost := sum([to_number(v) | v := current.costs[_]])

over_budget {
	current.hotel_reserved
	current.dinner_reserved
	total_cost > to_number(goal.budget_target)
}

# ─── Hotel ────────────────────────────────────────────────────────────────────

_hotel_actions["reserve_hotel"] {
	goal.trip.require_hotel
	not current.hotel_reserved
}

_hotel_actions["reduce_hotel_cost"] {
	over_budget
	not current.hotel_reduced
}

# ─── Dinner ───────────────────────────────────────────────────────────────────

_dinner_actions["reserve_dinner"] {
	goal.trip.require_dinner
	not current.dinner_reserved
}

_dinner_actions["reduce_dinner_cost"] {
	over_budget
	current.hotel_reduced
	not current.dinner_reduced
}

# ─── Aggregate ────────────────────────────────────────────────────────────────

missing[action] {
	action := (_hotel_actions | _dinner_actions)[_]
}
