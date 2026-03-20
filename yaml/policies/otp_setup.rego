package planner

# OTP Setup Policy
#
# Orchestrates the full OpenTripPlanner setup with optional parallel GTFS download:
#   1a. download_osm   — download OSM PBF data (idempotent, skips if file exists)
#   1b. download_gtfs  — download GTFS zip (only dispatched when gtfs_downloaded=false)
#   2.  build_graph    — build OTP graph; waits for osm_downloaded AND gtfs_downloaded
#   3.  start_server   — start OTP routing server
#
# input.current flags (set via direction_map "sets" when each child Want achieves):
#   osm_downloaded   - true once OSM data is downloaded
#   gtfs_downloaded  - true once GTFS data is downloaded (pre-set to true if no gtfs_url)
#   graph_built      - true once OTP graph.obj has been built
#   server_running   - true once OTP server is up and healthy

import future.keywords.if

# ── Conditions ────────────────────────────────────────────────────────────────

already_done if input.current.server_running

# Both data sources ready (gtfs_downloaded is pre-set to true when no GTFS URL is configured)
data_ready if {
    input.current.osm_downloaded
    input.current.gtfs_downloaded
}

# ─── Step 1a: Download OSM ───────────────────────────────────────────────────

_download_actions["download_osm"] {
    not already_done
    not input.current.osm_downloaded
}

# ─── Step 1b: Download GTFS (parallel with OSM, skipped if gtfs_downloaded=true) ─

_download_actions["download_gtfs"] {
    not already_done
    not input.current.gtfs_downloaded
}

# ─── Step 2: Build Graph ─────────────────────────────────────────────────────

_build_actions["build_graph"] {
    not already_done
    data_ready
    not input.current.graph_built
}

# ─── Step 3: Start Server ─────────────────────────────────────────────────────

_serve_actions["start_server"] {
    not already_done
    input.current.graph_built
    not input.current.server_running
}

# ─── Aggregate ────────────────────────────────────────────────────────────────

missing[action] {
    action := (_download_actions | _build_actions | _serve_actions)[_]
}
