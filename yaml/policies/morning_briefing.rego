package planner

# Morning Briefing Policy
#
# Trigger timing is controlled by the Want's "when" spec (e.g. at: "07:30", every: "day").
# This policy only decides whether the message should be composed given the current state.
#
# input.goal:
#   weather_city  - city name for weather query
#   routes        - array of {name, origin, destination, arrive_by, days}
#
# input.current (updated each cycle by MorningBriefingWant.Progress):
#   today          - "YYYY-MM-DD" of today
#   outgoing_date  - "YYYY-MM-DD" when the message was already composed
#   weather_date   - "YYYY-MM-DD" when weather was last fetched
#   transit_date_N - "YYYY-MM-DD" when transit route N was last fetched

import future.keywords.if
import future.keywords.in

# ── Weekday helpers ───────────────────────────────────────────────────────────

now_ns := time.now_ns()
now_weekday := time.weekday([now_ns, "Asia/Tokyo"])
weekday_map := {"Sunday": "sun", "Monday": "mon", "Tuesday": "tue", "Wednesday": "wed", "Thursday": "thu", "Friday": "fri", "Saturday": "sat"}
today_wd := weekday_map[now_weekday]

# ── Conditions ────────────────────────────────────────────────────────────────

already_composed_today if input.current.outgoing_date == input.current.today

weather_fetched_today if input.current.weather_date == input.current.today

transit_fetched_today(i) if {
    key := sprintf("transit_date_%d", [i])
    object.get(input.current, key, "") == input.current.today
}

route_is_today(i) if {
    route := input.goal.routes[i]
    days := split(route.days, ",")
    d := days[_]
    trim_space(d) == today_wd
}

route_is_today(i) if {
    route := input.goal.routes[i]
    not route.days
    today_wd in ["mon", "tue", "wed", "thu", "fri"]
}

all_transit_ready if {
    count([i |
        route := input.goal.routes[i]
        route_is_today(i)
        not transit_fetched_today(i)
    ]) == 0
}

all_transit_ready if {
    not input.goal.routes
}

# ── Actions ───────────────────────────────────────────────────────────────────

missing[action] if {
    not already_composed_today
    weather_fetched_today
    all_transit_ready
    action := "post_slack"
}
