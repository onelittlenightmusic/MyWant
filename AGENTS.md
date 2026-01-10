# MyWant Agents & Capabilities

MyWantは、宣言的な「Want（やりたいこと）」を自律的なエージェント群が協力して解決するシステムです。各エージェントは特定の**Capability（能力）**を持ち、チェーンの一部として機能します。

## エージェントの基本概念

MyWantには2つの主要なエージェント・タイプが存在します：

1.  **DoAgent (アクション型)**: 特定のタスクを一度だけ実行し、結果をStateに書き込みます（例：航空券の予約）。
2.  **MonitorAgent (監視型)**: 外部の状態を継続的にポーリングし、変化を検知してWantを更新します（例：フライトの遅延監視）。

---

## エージェント・カタログ

現在システムに登録されている主要なエージェントの一覧です。これらは `want-cli agents list` で確認できます。

### ✈️ トラベル・ドメイン (Travel)

| エージェント名 | タイプ | 必要な能力 (Capabilities) | 役割 |
| :--- | :--- | :--- | :--- |
| `agent_flight_api` | Do | `flight_api_agency` | フライトの検索と予約実行。 |
| `monitor_flight_api` | Monitor | `flight_api_agency` | 予約済みフライトのステータス監視。 |
| `hotel_monitor` | Monitor | `hotel_agency` | ホテルの空き状況と予約の確認。 |
| `agent_premium` | Do | `hotel_agency` | プレミアム特典付きの宿泊予約。 |
| `restaurant_monitor` | Monitor | `restaurant_agency` | レストラン予約のステータス追跡。 |

### 🛠️ システム・ユーティリティ (System)

| エージェント名 | タイプ | 必要な能力 (Capabilities) | 役割 |
| :--- | :--- | :--- | :--- |
| `execution_command` | Do | `command_execution` | シェルコマンドの実行と標準出力のキャプチャ。 |
| `mcp_tools` | Do | `mcp_gmail` | Model Context Protocolを通じたGmail操作。 |
| `reminder_queue_manager` | Do | `reminder_queue_management` | リマインダーのライフサイクルとキュー管理。 |
| `user_reaction_monitor` | Monitor | `reminder_monitoring` | ユーザーからの承認/否認アクションを監視。 |

---

## Capabilities (能力定義)

エージェントが提供、あるいは要求するインターフェースの定義です。

*   **`flight_api_agency`**: モックまたは実際の航空会社APIへの接続。
*   **`reaction_auto_approval`**: 承認ワークフローの自動化ロジック。
*   **`command_execution`**: ローカル環境でのバイナリ/スクリプト実行。
*   **`mcp_gmail`**: 外部ツールをAIが利用するためのインターフェース。

---

## エージェントの定義方法 (YAML)

新しいエージェントは `agents/` ディレクトリにYAMLファイルを配置することで追加できます。

```yaml
# agents/agent-hotel-premium.yaml
name: "agent_premium"
type: "do"
capabilities:
  - "hotel_agency"
  - "premium_services"
description: "Handles luxury hotel bookings with automated room upgrades."
```

---

## ワークフロー・パターン

### 1. 監視と再実行 (Monitor & Retrigger)
`Flight` Wantが遅延を検知すると、`MonitorAgent`が状態を「delayed」に変更します。これにより上位の`Coordinator`がリトリガーされ、代わりの手段を再計算します。

### 2. サイレンサー (Silencer Pattern)
`Reminder`エージェントがユーザーの入力を待っている間、`Silencer`エージェントが「自動承認ポリシー」に基づいて反応を代行します。

---

## エージェントの管理コマンド

CLIを使用して、実行時のエージェントの状態を確認できます。

```bash
# 登録されている全エージェントを表示
./want-cli agents list

# 利用可能な能力の一覧を表示
./want-cli capabilities list

# 特定のWantでどのアージェントが動いたか履歴を確認
./want-cli wants get <WANT_ID> --history
```

---

## 開発者向けガイド

新しいエージェントをGoで実装する場合は、`engine/cmd/types/` 配下の既存実装（例：`agent_flight_api.go`）を参考にしてください。

1.  `mywant.BaseAgent` を埋め込んだ構造体を作成。
2.  `Exec` または `Monitor` メソッドを実装。
3.  `main.go` または `Server.registerDynamicAgents` でレジストリに登録。
