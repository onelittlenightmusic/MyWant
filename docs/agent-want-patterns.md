# Agent / Want 実装パターン集

MyWant の既存コードから得られた実装パターンの知見をまとめる。
新しい Want type や Agent を追加する際のリファレンスとして使用すること。

---

## 1. Want 実装の 3 パターン

### 1-A. パッシブ Coordinator (ThinkAgent 委譲型)

**代表例:** `PythonThinkerWant`, `OpaLLMPlannerWant`

Want 自体はロジックを持たず、ThinkAgent がバックグラウンドで全処理を担う。

```go
type PythonThinkerWant struct {
    Want
}

func (o *PythonThinkerWant) Initialize() {
    // パラメータを state にコピーするだけ
    if goal, ok := o.Spec.Params["goal"]; ok {
        o.SetGoal("goal", goal)
    }
    o.SetCurrent("python_script", o.GetStringParam("python_script", ""))
}

func (o *PythonThinkerWant) IsAchieved() bool {
    return false  // 無限稼働
}

func (o *PythonThinkerWant) Progress() {}  // no-op
```

**特徴:**
- `Progress()` は no-op
- `IsAchieved()` は常に `false`（親から Stop されるまで動き続ける）
- ThinkAgent が `Requires` 経由で自動起動し、2s ごとに state を更新

---

### 1-B. フェーズ State Machine (DoAgent 呼出型)

**代表例:** `ExecutionResultWant`, `ReminderWant`

`Progress()` でフェーズを遷移させ、必要なタイミングで `ExecuteAgents()` を呼ぶ。

```go
func (e *ExecutionResultWant) Progress() {
    phase := GetCurrent(e, "phase", ExecutionPhaseInitial)
    switch phase {
    case ExecutionPhaseInitial:
        e.SetCurrent("phase", ExecutionPhaseExecuting)
        e.SetCurrent("status", "running")
        // DoAgent を同期実行
        if err := e.ExecuteAgents(); err != nil {
            e.SetStatus(WantStatusModuleError)
        }
    case ExecutionPhaseExecuting:
        // agent_result から結果を取得して state に反映
        result := e.GetCurrent("agent_result", nil)
        e.processResult(result)
    }
}

func (e *ExecutionResultWant) IsAchieved() bool {
    return GetCurrent(e, "completed", false)
}
```

**特徴:**
- DoAgent は `Progress()` 内で `ExecuteAgents()` を呼んで同期実行
- フェーズ変数 (`phase`) で状態機械を制御
- `IsAchieved()` は state の `completed` フラグで判定

---

### 1-C. Dispatch Coordinator (動的 Want 生成型)

**代表例:** `ItineraryWant`

ThinkAgent が生成した `directions` を `InterpretDirections()` で解釈し、子 Want を動的に生成する。

```go
func (o *ItineraryWant) Progress() {
    // ThinkAgent が plan state に書いた directions を解釈
    InterpretDirections(&o.Want)

    // directions が空 = 全タスク完了
    dirs := o.GetAllPlan()
    o.SetAchieved(len(dirs["directions"].([]any)) == 0)
}
```

**特徴:**
- 子 Want の生成は `DispatchThinker` が担当（`AddChildWant` の呼出は Want の外側から）
- `direction_map` パラメータで direction 名 → Want type のマッピングを定義
- 完了判定: directions リストが空になったとき

---

## 2. Agent の 3 タイプ

### 2-A. DoAgent (同期・一回実行)

```go
// 登録
RegisterDoAgentType("my_agent", []Capability{Cap("my_capability")},
    func(ctx context.Context, want *Want) error {
        // 処理
        want.SetCurrent("result", "done")
        return nil
    },
)
```

| 項目 | 値 |
|------|-----|
| 実行タイミング | `Progress()` 内で `ExecuteAgents()` 呼出時 |
| 実行回数 | 明示的に呼ばれるたびに1回 |
| 停止条件 | 関数が return したら完了 |
| エラー処理 | `return error` → `WantStatusModuleError` |

---

### 2-B. ThinkAgent (バックグラウンド・無限ループ)

```go
// 登録
RegisterThinkAgentType("my_thinker", []Capability{Cap("my_thinking")},
    func(ctx context.Context, want *Want) error {
        // 2s ごとに呼ばれる
        goal := want.GetAllGoal()
        current := want.GetAllCurrent()
        // ... 計算 ...
        want.SetPlan("directions", []string{"action1"})
        return nil
    },
)
```

| 項目 | 値 |
|------|-----|
| 実行タイミング | `StartBackgroundAgents()` で自動起動 |
| 実行間隔 | 2秒 (デフォルト) |
| 停止条件 | なし（Want が Stop されるまで無限稼働） |
| ログ | `want.DirectLog()` を使用（`StoreLog()` は exec cycle 外で無効） |

---

### 2-C. MonitorAgent (バックグラウンド・停止可能)

```go
// 登録
RegisterMonitorAgentType("my_monitor", []Capability{Cap("my_monitoring")},
    func(ctx context.Context, want *Want) (bool, error) {
        // 5s ごとに呼ばれる
        status := checkExternalResource()
        want.SetCurrent("status", status)
        shouldStop := status == "complete"
        return shouldStop, nil  // true を返すと polling 停止
    },
)
```

| 項目 | 値 |
|------|-----|
| 実行タイミング | `StartBackgroundAgents()` で自動起動 |
| 実行間隔 | 5秒 (デフォルト) |
| 停止条件 | 第1戻り値が `true` のとき |

---

## 3. Requires → Agent 解決フロー

YAML の `requires:` リストが、実行時にどうエージェントに繋がるかの仕組み。

```
YAML定義
  requires: ["my_capability"]
        ↓
StartProgressionLoop() 起動時
  → StartBackgroundAgents()
      → agentRegistry.FindAgentsByGives("my_capability")
          → Think/Monitor: バックグラウンド goroutine 起動
          → Do: 無視 (Progress内でExecuteAgents()が担当)

Progress() 内
  → ExecuteAgents()
      → agentRegistry.FindAgentsByGives("my_capability")
          → Do: 同期実行
          → Think/Monitor: 無視 (既に起動済み)
```

**ポイント:**
- `requires:` に書いた capability 名でエージェントが検索される
- `Capability.Gives` フィールドで「このエージェントが何を提供するか」を宣言
- YAML の `agents:` フィールドは**ドキュメント専用**（実行には使われない）

---

## 4. State I/O コントラクト

### 4-A. ラベル体系

全ての state key に**ラベル**が必要。ラベルなしの書込みは警告が出る。

| ラベル | 用途 | API |
|--------|------|-----|
| `goal` | 達成したい目標状態 | `SetGoal()` / `GetAllGoal()` |
| `current` | 現在の観測状態 | `SetCurrent()` / `GetAllCurrent()` |
| `plan` | 中間計画・directions | `SetPlan()` / `GetAllPlan()` |
| `internal` | 内部管理用（外部非公開） | `SetInternal()` |

### 4-B. 親子間の state アクセス

```go
// 子→親の読み取り
parentGoal    := want.GetParentAllGoal()
parentCurrent := want.GetParentAllCurrent()

// 子→親への書込み
want.MergeParentState(map[string]any{"cost": 1200})
```

### 4-C. 変更検知パターン (ShouldRunAgent)

外部プロセス（Python/OPA等）の無駄な再実行を防ぐ。

```go
// 入力のハッシュを計算し、前回と同じなら skip
shouldRun, inputHash := ShouldRunAgent(want, "my_input_hash", goalAll, currentAll)
if !shouldRun {
    return nil
}

// ... 外部処理 ...

// ハッシュを保存（次回のデデュップ用）
want.SetCurrent("my_input_hash", inputHash)
```

**注意:** `hashKey` 自身を入力に含めると循環依存になるので除外すること。

---

## 5. スクリプト系エージェントの共通実装パターン

Python thinker / OPA thinker / execution_result が共有するパターン。

```go
func myScriptThink(ctx context.Context, want *Want) error {
    // 1. 入力収集
    goalAll    := want.GetAllGoal()
    currentAll := want.GetAllCurrent()

    // 2. 変更検知
    shouldRun, hash := ShouldRunAgent(want, "script_input_hash", goalAll, currentAll)
    if !shouldRun {
        return nil
    }

    // 3. 一時ファイル書出
    tmpDir, _ := os.MkdirTemp("", "my-thinker-*")
    defer os.RemoveAll(tmpDir)

    goalBytes, _    := json.Marshal(goalAll)
    currentBytes, _ := json.Marshal(currentAll)
    goalPath    := filepath.Join(tmpDir, "goal.json")
    currentPath := filepath.Join(tmpDir, "current.json")
    os.WriteFile(goalPath, goalBytes, 0600)
    os.WriteFile(currentPath, currentBytes, 0600)

    // 4. 外部プロセス実行
    cmd := exec.CommandContext(ctx, "my-script")
    cmd.Env = append(os.Environ(),
        "MYWANT_GOAL_FILE="+goalPath,
        "MYWANT_CURRENT_FILE="+currentPath,
    )
    stdout, err := cmd.Output()
    if err != nil {
        want.DirectLog("[MY-THINKER] ERROR: %v", err)
        return err
    }

    // 5. JSON 出力パース
    var output map[string]any
    json.Unmarshal(stdout, &output)

    // 6. state 書戻し
    if dirs, ok := output["directions"].([]any); ok {
        want.SetPlan("directions", dirs)
    }
    if updates, ok := output["current_updates"].(map[string]any); ok {
        for k, v := range updates {
            want.SetCurrent(k, v)
        }
    }

    // 7. ハッシュ保存
    want.SetCurrent("script_input_hash", hash)
    return nil
}
```

### スクリプトへの入力コントラクト (環境変数)

| 環境変数 | 内容 |
|---------|------|
| `MYWANT_GOAL_FILE` | goal state の JSON ファイルパス |
| `MYWANT_CURRENT_FILE` | current state の JSON ファイルパス |
| `MYWANT_PLAN_FILE` | plan state の JSON ファイルパス |
| `MYWANT_INPUT_FILE` | `{goal, current, plan}` 結合 JSON のファイルパス |

### スクリプトの出力コントラクト (stdout JSON)

```json
{
  "result": <any>,
  "directions": ["action1", "action2"],
  "current_updates": {"key": "value"},
  "should_stop": false
}
```

| フィールド | 対象エージェント | 意味 |
|-----------|----------------|------|
| `result` | Think | `plan.result` に格納 |
| `directions` | Think | `plan.directions` に格納 |
| `current_updates` | Think / Do / Monitor | current state に反映 |
| `should_stop` | Monitor のみ | `true` で polling 停止 |

---

## 6. バックグラウンドエージェントでのログ出力

`want.StoreLog()` は `inExecCycle = true` のときのみ動作する。
ThinkAgent/MonitorAgent のコールバック内（特に goroutine 内部）では必ず `DirectLog()` を使うこと。

```go
// NG: ThinkAgent 内では inExecCycle が false の場合があり、ログが消える
want.StoreLog("[MY-THINKER] done")

// OK: BeginProgressCycle() のラップ外でも確実に出力される
want.DirectLog("[MY-THINKER] done")
```

`ThinkingAgent` の実行ループは自動的に `BeginProgressCycle()` / `EndProgressCycle()` で囲むため、
ticker コールバック内では `StoreLog()` も使用できる。ただし goroutine を別途立ち上げた場合は `DirectLog()` を使うこと。

---

## 7. Want type の登録フロー (参考)

```
init() で呼ぶ
  RegisterWantImplementation[MyWant, MyLocals]("my_type")
    → typeImplementationRegistry["my_type"] = reflect.TypeOf(MyWant)

サーバ起動時
  WantTypeLoader.LoadAllWantTypes()
    → YAML 読込 → WantTypeDefinition
  ChainBuilder.StoreWantTypeDefinition(def)
    → typeImplementationRegistry に "my_type" があれば
      → createGenericFactory("my_type") でファクトリ登録

Want 生成時
  ChainBuilder.createWantFunction(want)
    → registry["my_type"](metadata, spec)
      → reflect.New(MyWant) → initializeInstance (Want フィールド注入)
      → SetWantTypeDefinition(def)  // state 初期値を適用
      → SetAgentRegistry(registry)
```
