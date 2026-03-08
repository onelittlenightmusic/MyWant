package planner

# input.goal    = mergeOPAInput(GetAllGoal())    → {"trip": {...}, "budget_target": N, ...}
# input.current = mergeOPAInput(GetAllCurrent()) → {"hotel_reserved": bool, ..., "opa_input_hash": "..."}

goal    := input.goal
current := input.current

# Total cost = sum of all values in current.costs map
total_cost := sum([to_number(v) | v := current.costs[_]])

# ─── Phase 1: Make initial bookings ───────────────────────────────────────────

# Book hotel when required and not yet reserved
missing[action] {
	goal.trip.require_hotel
	not current.hotel_reserved
	action := "reserve_hotel"
}

# Book dinner when required and not yet reserved
missing[action] {
	goal.trip.require_dinner
	not current.dinner_reserved
	action := "reserve_dinner"
}

# ─── Phase 2: Budget enforcement ──────────────────────────────────────────────
# Only trigger after both bookings are done so we can see the full cost picture.

# Reduce hotel cost when total exceeds budget and hotel not yet rebooked cheaper
missing[action] {
	current.hotel_reserved
	current.dinner_reserved
	total_cost > to_number(goal.budget_target)
	not current.hotel_reduced
	action := "reduce_hotel_cost"
}

# Reduce dinner cost if still over budget after hotel was rebooked cheaper
missing[action] {
	current.hotel_reserved
	current.dinner_reserved
	total_cost > to_number(goal.budget_target)
	current.hotel_reduced
	not current.dinner_reduced
	action := "reduce_dinner_cost"
}
