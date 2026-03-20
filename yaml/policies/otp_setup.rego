package planner

# OTP Setup Policy
#
# Orchestrates the full OpenTripPlanner setup:
#   1a. download_osm        — download OSM PBF data (idempotent)
#   1b. download_gtfs       — download GTFS feeds (parallel with OSM, skipped if no feeds)
#   1c. write_build_config  — write build-config.json (after GTFS downloaded, skipped if no feeds)
#   2.  build_graph         — build OTP graph (waits for OSM + GTFS + build config)
#   3.  start_server        — start OTP routing server
#
# input.current flags:
#   osm_downloaded        - true once OSM data is downloaded
#   gtfs_downloaded       - true once GTFS data is downloaded (pre-set to true if no feeds)
#   build_config_written  - true once build-config.json is written (pre-set to true if no feeds)
#   graph_built           - true once OTP graph.obj has been built
#   server_running        - true once OTP server is up and healthy

import future.keywords.if

# ── Conditions ────────────────────────────────────────────────────────────────

already_done if input.current.server_running

# Both data sources and build config ready
ready_to_build if {
    input.current.osm_downloaded
    input.current.gtfs_downloaded
    input.current.build_config_written
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

# ─── Step 1c: Write build-config.json (after GTFS downloaded) ────────────────

_config_actions["write_build_config"] {
    not already_done
    input.current.gtfs_downloaded
    not input.current.build_config_written
}

# ─── Step 2: Build Graph ─────────────────────────────────────────────────────

_build_actions["build_graph"] {
    not already_done
    ready_to_build
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
    action := (_download_actions | _config_actions | _build_actions | _serve_actions)[_]
}
