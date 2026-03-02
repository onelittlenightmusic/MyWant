package planner

# ─── Phase 1: Make initial bookings ───────────────────────────────────────────

# Book hotel when required and not yet reserved
missing[action] {
	input.goal.trip.require_hotel
	not input.current.hotel_reserved
	action := "reserve_hotel"
}

# Book dinner when required and not yet reserved
missing[action] {
	input.goal.trip.require_dinner
	not input.current.dinner_reserved
	action := "reserve_dinner"
}

# ─── Phase 2: Budget enforcement ──────────────────────────────────────────────
# Only trigger after both bookings are done so we can see the full cost picture.

# Reduce hotel cost when total exceeds budget and hotel not yet rebooked cheaper
missing[action] {
	input.current.hotel_reserved
	input.current.dinner_reserved
	input.current.hotel_cost > 0
	input.current.dinner_cost > 0
	input.current.hotel_cost + input.current.dinner_cost > input.goal.budget_target
	not input.current.hotel_reduced
	action := "reduce_hotel_cost"
}

# Reduce dinner cost if still over budget after hotel was rebooked cheaper
missing[action] {
	input.current.hotel_reserved
	input.current.dinner_reserved
	input.current.hotel_cost > 0
	input.current.dinner_cost > 0
	input.current.hotel_cost + input.current.dinner_cost > input.goal.budget_target
	input.current.hotel_reduced
	not input.current.dinner_reduced
	action := "reduce_dinner_cost"
}
