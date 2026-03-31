# YAML-Only Scriptable Want Type 設計

Go コードを書かずに YAML だけで新しい Want type を定義・実行できる仕組み。
`inlineAgents` フィールドにスクリプトを埋め込むことで、Think/Do/Monitor エージェントを宣言できる。

---

## 概要

### 現状の課題

新しい Want type を追加するには必ず Go コードが必要:

```
1. engine/types/my_type.go に RegisterWantImplementation[T,L]("my_type") を書く
2. Initialize() / IsAchieved() / Progress() を実装
3. エージェント関数を RegisterDoAgentType 等で登録
4. make restart-all でリビルド
```

### 解決策

YAML の `inlineAgents:` にスクリプトを書くだけで実行可能な Want type を定義できる。

```yaml
inlineAgents:
  - name: my_thinker
    type: think
    runtime: rego
    script: |
      package mywant.my_type
      directions contains "action1" if { input.current.value > 10 }
```

---

## YAML フォーマット

### 完全な例

```yaml
wantType:
  metadata:
    name: disk_health_check
    title: Disk Health Check & Cleanup
    description: |
      Rego でポリシー判定、Shell でクリーンアップ実行、Python でディスク使用量を監視。
      Go コード不要の YAML-only Want type の例。
    version: "1.0"
    category: automation
    pattern: independent

  parameters:
    - name: threshold_percent
      description: クリーンアップを起動するディスク使用率のしきい値
      type: int
      default: 80
    - name: cleanup_path
      description: クリーンアップ対象のパス
      type: string
      required: true

  state:
    - name: disk_usage_percent
      description: 現在のディスク使用率 (%)
      type: int
      label: current
      persistent: true
      initialValue: 0
    - name: cleanup_result
      description: 最後のクリーンアップ実行結果
      type: string
      label: current
      persistent: true
      initialValue: ""
    - name: directions
      description: Rego が生成した実行指示
      type: object
      label: plan
      persistent: true
      initialValue: []
    - name: think_input_hash
      description: ThinkAgent の変更検知用ハッシュ
      type: string
      label: current
      persistent: false
      initialValue: ""

  # ===========================
  # NEW: インラインエージェント定義
  # ===========================
  inlineAgents:

    # Think エージェント: Rego でポリシー判定
    - name: disk_policy_thinker
      type: think              # think | do | monitor
      runtime: rego            # rego | shell | python
      interval: 5              # 実行間隔 (秒)。省略時: think=2, monitor=5
      script: |
        package mywant.disk_health_check

        import rego.v1

        default directions := []

        directions contains "cleanup_temp" if {
          input.current.disk_usage_percent > input.goal.threshold_percent
        }

        directions contains "cleanup_logs" if {
          input.current.disk_usage_percent > 95
        }

    # Do エージェント: Shell でクリーンアップ実行
    - name: disk_cleanup_executor
      type: do
      runtime: shell
      script: |
        #!/bin/bash
        CLEANUP_PATH=$(jq -r '.cleanup_path' "$MYWANT_CURRENT_FILE")
        find "$CLEANUP_PATH" -name "*.tmp" -mtime +7 -delete 2>&1
        FREED=$(du -sh "$CLEANUP_PATH" | cut -f1)
        echo "{\"current_updates\": {\"cleanup_result\": \"freed $FREED\"}}"

    # Monitor エージェント: Python でディスク使用量を監視
    - name: disk_space_monitor
      type: monitor
      runtime: python
      interval: 30
      script: |
        import json, os, shutil
        current = json.load(open(os.environ["MYWANT_CURRENT_FILE"]))
        path = current.get("cleanup_path", "/")
        usage = shutil.disk_usage(path)
        used_pct = int((usage.used / usage.total) * 100)
        should_stop = used_pct < 50
        print(json.dumps({
            "current_updates": {"disk_usage_percent": used_pct},
            "should_stop": should_stop
        }))

  # ===========================
  # NEW: 宣言的達成条件
  # ===========================
  achievedWhen:
    field: disk_usage_percent    # 評価する state フィールド名
    operator: "<"                # ==, !=, >, >=, <, <=
    value: 80
```

---

## 各フィールドの仕様

### `inlineAgents[].type`

| 値 | 実行モデル | 間隔 | 停止条件 |
|---|---------|------|---------|
| `think` | バックグラウンド goroutine + ticker | `interval` 秒 (デフォルト 2s) | なし（無限稼働） |
| `do` | `Progress()` から `ExecuteAgents()` で同期呼出 | - | 1回実行 |
| `monitor` | バックグラウンド polling | `interval` 秒 (デフォルト 5s) | `"should_stop": true` |

### `inlineAgents[].runtime`

| 値 | 実行方式 | 対応 type | 依存 |
|---|---------|---------|------|
| `rego` | 埋め込み OPA ライブラリで評価 | `think` のみ | なし (Go 組み込み) |
| `shell` | `/bin/bash -c <script>` | `do`, `monitor` | bash |
| `python` | `python3 <tmpfile>` | `think`, `do`, `monitor` | python3 |
| `ansible` | `ansible-playbook --inventory localhost, --connection local` | `do`, `monitor` | 下記参照 |

#### `runtime: ansible` の事前準備

```bash
pip install ansible
ansible-galaxy collection install community.docker  # docker_compose を使う場合
```

Ansible が見つからない場合、Want は以下のエラーで失敗します:

```
ansible-playbook not found in PATH.
  Install Ansible and required collections:
    pip install ansible
    ansible-galaxy collection install community.docker
  Then restart the mywant server.
```

#### Ansible スクリプトの I/O コントラクト

Ansible playbook は他のランタイムと同じ環境変数でステートファイルを受け取る。
出力は stdout ではなく `$MYWANT_OUTPUT_FILE` に JSON を書き込む:

```yaml
- name: Write output
  ansible.builtin.copy:
    content: '{"current_updates": {"my_field": "value"}}'
    dest: "{{ lookup('env', 'MYWANT_OUTPUT_FILE') }}"
```

`$MYWANT_OUTPUT_FILE` が書き込まれない場合は state 更新なし (playbook 成功扱い)。

`launch_env_*` 形式の current state キーは自動的に実環境変数に展開される
(例: `launch_env_OTP_DATA_DIR` → `OTP_DATA_DIR=value`)。
docker-compose ファイルの `${VAR}` 置換がそのまま機能する。

### `inlineAgents[].scriptFile`

`script:` の代わりに外部ファイルを参照できる。長いスクリプトの可読性向上に有効。
パスはサーバの起動ディレクトリからの相対パス、または絶対パス。

```yaml
inlineAgents:
  - name: my_agent
    type: do
    runtime: shell
    scriptFile: yaml/scripts/my_script.sh   # script: の代わりに指定
```

### `achievedWhen`

`ScriptableWant.IsAchieved()` の動作を宣言的に定義する。省略時は `state["achieved"] == true` にフォールバック。

```yaml
achievedWhen:
  field: my_state_field   # current-label の state key
  operator: ">="          # ==, !=, >, >=, <, <=
  value: 100
```

#### 特殊値

| 値 | 解決内容 |
|---|---------|
| `"$today"` | 実行時の日付 `YYYY-MM-DD` に動的解決 |

```yaml
# 例: 今日の天気データが取得済みなら達成
achievedWhen:
  field: weather_date
  operator: "=="
  value: "$today"
```

### `onAchieved`

`IsAchieved()` が true を返した Progress() ティックで実行されるライフサイクルフック。
`onInitialize` / `onDelete` と同じ構造。達成済みのとき `ExecuteAgents()` は呼ばれない。

```yaml
onAchieved:
  mergeParent:               # 親 Want に state を伝播
    weather_done: true
    weather_text: "${weather_text}"   # ${varName} で current state から補間
```

### `LifecycleHookDef.mergeParent`

`onInitialize` / `onDelete` / `onAchieved` いずれのフックでも使用可能。
指定したキーと値を `MergeParentState()` で親 Want に伝播する。
値は `${varName}` プレースホルダで current state から補間できる。

```yaml
onAchieved:
  mergeParent:
    task_done: true
    task_result: "${result_field}"
```

---

## スクリプト I/O コントラクト

### 入力 (スクリプトへ渡される環境変数)

| 環境変数 | 内容 |
|---------|------|
| `MYWANT_GOAL_FILE` | goal state の JSON ファイルパス |
| `MYWANT_CURRENT_FILE` | current state の JSON ファイルパス |
| `MYWANT_PLAN_FILE` | plan state の JSON ファイルパス |
| `MYWANT_INPUT_FILE` | `{goal, current, plan}` 結合 JSON のファイルパス |

### 出力 (スクリプトが stdout に出力する JSON)

```json
{
  "result": <any>,
  "directions": ["action1", "action2"],
  "current_updates": {"key": "value"},
  "should_stop": false
}
```

| フィールド | 対象 type | 意味 |
|-----------|---------|------|
| `result` | think | `plan.result` に格納 |
| `directions` | think | `plan.directions` に格納 |
| `current_updates` | 全て | current-labeled state を更新 |
| `should_stop` | monitor | `true` で polling 停止 |

### Rego スクリプトの入力 (OPA input オブジェクト)

```json
{
  "goal":    { /* goal state の全フィールド */ },
  "current": { /* current state の全フィールド */ }
}
```

Rego スクリプトで定義すべきルール:
- `directions` (set or array) — 実行する action 名のリスト
- `result` (object, optional) — plan.result に格納される任意データ

---

## Go 実装の構成

### 新規ファイル

```
engine/core/
  inline_agent_def.go       # InlineAgentDef / AchievedWhenDef 構造体
  scriptable_want.go        # ScriptableWant (汎用 Progressable) + ファクトリ
  script_runtime.go         # ScriptRuntime インターフェース + 共通ヘルパー
  script_runtime_shell.go   # Shell ランタイム
  script_runtime_python.go  # Python ランタイム
  script_runtime_rego.go    # Rego ランタイム (embedded OPA)
```

### 既存ファイルの修正

```
engine/core/want_type_loader.go      # WantTypeDefinition に InlineAgents/AchievedWhen 追加
engine/core/chain_builder_registry.go  # StoreWantTypeDefinition でインライン型を自動登録
engine/go.mod                         # github.com/open-policy-agent/opa 追加
```

### 既存ファイルの修正なし (再利用)

```
engine/core/registry.go      # RegisterDoAgentType 等をそのまま利用
engine/core/think_agent.go   # ThinkingAgent ticker ループをそのまま利用
engine/core/want_agent.go    # StartBackgroundAgents/ExecuteAgents をそのまま利用
```

---

## 内部動作フロー

### 起動時（サーバ起動 / `make restart-all`）

```
WantTypeLoader.LoadAllWantTypes()
  → yaml/want_types/ の YAML ファイルを読込
  → WantTypeDefinition に InlineAgents をパース

ChainBuilder.StoreWantTypeDefinition(def)
  → Go 実装なし && len(InlineAgents) > 0 の場合:
      → registerInlineAgents(def)
          → 各 InlineAgentDef について:
              → capName = "{type_name}__{agent_name}" を生成
              → runtime = resolveRuntime(ia.Runtime)
              → RegisterThinkAgentType / RegisterDoAgentType / RegisterMonitorAgentType
              → def.Requires に capName を追加
      → cb.RegisterWantType(typeName, createScriptableFactory(def))
```

### Want 実行時

```
ChainBuilder.createWantFunction(want)
  → registry["disk_health_check"](metadata, spec)
      → ScriptableWant{} を生成
      → SetWantTypeDefinition(def)  // state 初期値を適用
      → SetAgentRegistry(registry)

StartProgressionLoop()
  → progressable.Initialize()
      → パラメータを state にコピー
  → StartBackgroundAgents()
      → Spec.Requires = ["disk_health_check__disk_policy_thinker", ...]
      → Think agent → ThinkingAgent goroutine 起動 (5s interval)
      → Monitor agent → PollingAgent goroutine 起動 (30s interval)

Progress() ループ
  → ExecuteAgents()
      → Do agent → disk_cleanup_executor を同期実行

IsAchieved()
  → achievedWhen.field = "disk_usage_percent"
  → state["disk_usage_percent"] < 80 なら true
```

---

## 使用例

### ファイルの配置

```
yaml/want_types/
  custom/                  # カスタム want types 用ディレクトリ (新設)
    disk_health_check.yaml
    my_custom_type.yaml
```

### デプロイ

```yaml
# yaml/config/my-config.yaml
wants:
  - metadata:
      name: check-disk-1
      type: disk_health_check
    spec:
      params:
        threshold_percent: 75
        cleanup_path: /tmp
```

```bash
./bin/mywant wants create -f yaml/config/my-config.yaml
./bin/mywant wants get check-disk-1
```

---

## 既存 Want type との違い

| 項目 | 既存 (Go 実装) | YAML-Only (inlineAgents) |
|-----|-------------|------------------------|
| 実装場所 | `engine/types/*.go` | `yaml/want_types/*.yaml` |
| リビルド | 必要 (`make restart-all`) | 不要 (サーバ再起動のみ) |
| ロジック記述 | Go コード | Rego / Shell / Python |
| 複雑な状態機械 | 可能 | 限定的 (`achievedWhen` のみ) |
| パフォーマンス | 高い | Rego は高速、Shell/Python はプロセス起動コスト |
| デバッグ | Go デバッガ使用可 | ログ (`DirectLog`) + JSON ファイル確認 |

---

## 注意事項

1. **current_updates の更新制限**: スクリプトから更新できるのは `label: current` のフィールドのみ。
   `label: goal` や `label: plan` のフィールドへの書込みはスキップされる。

2. **変更検知**: think agent は `think_input_hash` state フィールドで入力変化を検知し、変化がなければスクリプトを再実行しない。
   このフィールドを state に定義しておくこと（`label: current`, `persistent: false`）。

3. **Rego パッケージ名**: `package mywant.<type_name>` の命名規則に従うこと。
   `directions` ルールは set (順不同) でも array でも受け付ける。

4. **Shell スクリプトのセキュリティ**: Shell スクリプトはサーバプロセスの権限で実行される。
   信頼できる環境でのみ使用すること。

5. **Python コマンド**: デフォルトは `python3`。`MYWANT_PYTHON_CMD` 環境変数または
   サーバ設定で変更できる（将来実装予定）。

6. **Ansible runtime の前提条件**: `runtime: ansible` を使う Want type をデプロイする前に
   以下をインストールすること。未インストールの場合、Want は即座に `module_error` 状態になる。
   ```bash
   pip install ansible
   ansible-galaxy collection install community.docker  # docker_compose 操作時
   ```
   インストール後はサーバの再起動が必要。

7. **Ansible の出力プロトコル**: shell/python ランタイムと異なり、Ansible playbook は
   stdout ではなく `$MYWANT_OUTPUT_FILE` に JSON を書き込む。
   `ansible.builtin.copy` タスクで書き込むのが定番パターン。
