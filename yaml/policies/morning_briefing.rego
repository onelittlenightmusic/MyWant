package planner

# Morning Briefing Policy
#
# Trigger timing and daily reset are controlled by the Want's "when" spec.
# This policy controls the full fetch-and-post flow:
#   1. fetch_weather  — fetch today's weather
#   2. fetch_transit  — fetch today's transit route
#   3. post_slack     — compose and post the briefing (only after 1+2 are done)
#
# input.current flags (reset on each `when`-triggered restart via Initialize):
#   weather_done   - true once weather child Want has achieved
#   transit_done   - true once transit child Want has achieved
#   briefing_done  - true once post_slack child Want has achieved (via Sets)

import future.keywords.if

# ── Conditions ────────────────────────────────────────────────────────────────

already_done if input.current.briefing_done

# ─── Weather ──────────────────────────────────────────────────────────────────

_weather_actions["fetch_weather"] {
    not already_done
    not input.current.weather_done
}

# ─── Transit ──────────────────────────────────────────────────────────────────

_transit_actions["fetch_transit"] {
    not already_done
    not input.current.transit_done
}

# ─── Slack ────────────────────────────────────────────────────────────────────

_slack_actions["post_slack"] {
    not already_done
    input.current.weather_done
    input.current.transit_done
}

# ─── Aggregate ────────────────────────────────────────────────────────────────

missing[action] {
    action := (_weather_actions | _transit_actions | _slack_actions)[_]
}
