package planner

# input.goal    = mergeOPAInput(GetAllGoal())
# input.current = mergeOPAInput(GetAllCurrent())
#
# This policy extends travel_budget.rego with hotel_expert capability awareness.
# When input.current.available_capabilities contains "hotel_expert" and the goal
# prefers luxury, it plans reserve_hotel_luxury instead of reserve_hotel_budget.

goal    := input.goal
current := input.current

total_cost := sum([to_number(v) | v := current.costs[_]])

# helper: true if "hotel_expert" is in available_capabilities
has_hotel_expert {
    current.available_capabilities[_] == "hotel_expert"
}

# ─── Phase 1: Hotel booking with capability-aware routing ─────────────────────

# Luxury hotel when hotel_expert is available and goal prefers luxury
missing[action] {
    goal.trip.require_hotel
    not current.hotel_reserved
    goal.trip.prefer_luxury
    has_hotel_expert
    action := "reserve_hotel_luxury"
}

# Budget/standard hotel otherwise (no hotel_expert or no luxury preference)
missing[action] {
    goal.trip.require_hotel
    not current.hotel_reserved
    not (goal.trip.prefer_luxury; has_hotel_expert)
    action := "reserve_hotel_budget"
}

# Book dinner when required and not yet reserved
missing[action] {
    goal.trip.require_dinner
    not current.dinner_reserved
    action := "reserve_dinner"
}

# ─── Phase 2: Budget enforcement ──────────────────────────────────────────────

missing[action] {
    current.hotel_reserved
    current.dinner_reserved
    total_cost > to_number(goal.budget_target)
    not current.hotel_reduced
    action := "reduce_hotel_cost"
}

missing[action] {
    current.hotel_reserved
    current.dinner_reserved
    total_cost > to_number(goal.budget_target)
    current.hotel_reduced
    not current.dinner_reduced
    action := "reduce_dinner_cost"
}
