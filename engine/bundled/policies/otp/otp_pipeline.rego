package planner

import future.keywords.contains
import future.keywords.if
import future.keywords.in
import future.keywords.every

current := input.current

# ── 全プロバイダー達成チェック（フィード名はRegoに書かない） ────────────────
# dispatch_thinker が provider_keys に ["osm_done", "gtfs_*_done", ...] をセットする。
# GTFSフィードを追加しても、Regoの変更は不要。
all_providers_done if {
    every k in current.provider_keys {
        current[k] == true
    }
}

# ── build_graph: 全プロバイダー完了後にgraph.objをビルド ─────────────────────
_build contains "build_graph" if {
    all_providers_done
    not current.graph_done
}

# ── start_server: サーバー起動まで常にmissingに残す ─────────────────────────
# 実際のブロック（graph完了待ち）はdispatch_thinkerのusing:[{direction:build_graph}]が担う。
_server contains "start_server" if {
    not current.server_running
}

missing contains action if {
    some action in (_build | _server)
}
