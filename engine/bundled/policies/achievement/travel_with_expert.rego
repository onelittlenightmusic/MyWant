package planner

# Achievement-based discount routing policy.
#
# When input.current.available_capabilities contains "hotel_discount" (unlocked
# by earning the "割引の達人" achievement), the planner routes to reserve_hotel_discounted
# (80–150 USD) instead of reserve_hotel_standard (250–400 USD).
# With a budget target of 200 USD:
#   - Without hotel_discount → standard hotel exceeds budget → budget NOT achieved
#   - With    hotel_discount → discounted hotel stays under budget → budget achieved

goal    := input.goal
current := input.current

total_cost := sum([to_number(v) | v := current.costs[_]])

# helper: true if "hotel_discount" capability is unlocked via achievement
has_hotel_discount {
    current.available_capabilities[_] == "hotel_discount"
}

# ─── Phase 1: Hotel booking with discount capability routing ──────────────────

# Discounted hotel when hotel_discount capability is available
missing[action] {
    goal.trip.require_hotel
    not current.hotel_reserved
    has_hotel_discount
    action := "reserve_hotel_discounted"
}

# Standard hotel when no discount capability
missing[action] {
    goal.trip.require_hotel
    not current.hotel_reserved
    not has_hotel_discount
    action := "reserve_hotel_standard"
}
