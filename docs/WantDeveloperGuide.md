# Want Developer Guide

このガイドは、Want型を拡張する（カスタムWantタイプを作成する）開発者向けのドキュメントです。

## 必須インターフェース

Want型を拡張する場合、以下のインターフェースを実装する必要があります。

### Progressable インターフェース

```go
type Progressable interface {
    // Progress はWantの実行メインループです
    // BeginProgressCycle() と EndProgressCycle() のペアで囲まれた exec cycle 内で呼ばれます
    Progress()
}
```

**責務：**
- 実行状態を進める
- 子どもWantの作成・管理
- パケットの受信・送信
- 状態の更新（StoreState/MergeState経由）

**重要：** Progress()は複数の exec cycle にわたって呼ばれる可能性があります。状態管理に注意してください。

### その他の重要なメソッド

```go
// Initialize は exec cycle 開始前に呼ばれます
Initialize()

// IsAchieved は Want が完了したかを判定します
IsAchieved() bool

// EndProgressCycle は exec cycle 終了時に呼ばれます
// 状態変更の永続化などはここで行います
EndProgressCycle()
```

## 状態所有権ルール（CRITICAL）

> **各WantはそのStateとStatusの唯一の所有者です。**
> Want Aのために動くAgent（またはあらゆるコード）が、別のWant BのStateやStatusを直接変更することは違反です。

### なぜこのルールがあるか

すべてのWantは独自のgoroutineで実行されます。外部goroutineから別WantのStateに書き込むと、ロック機構が機能せずデータ競合・ステータス不整合・履歴破損が発生します。

### 具体的なルール

 | コンテキスト | 許可 | 禁止 |
 |:------------|:-----|:-----|
 | `Progress()` / `Initialize()` | `b.StoreState(...)`, `b.SetStatus(...)` をレシーバに対して呼ぶ | `otherWant.StoreState(...)`, `otherWant.SetStatus(...)` |
| `Progress()` / `Initialize()` | `b.AddChildWant(...)` (Parentとして自分を登録) | `cb.AddWant(...)` を直接呼んで子を増やす行為 |
 | Agent `Exec(ctx, want)` | `want.StoreStateForAgent(...)` を渡された`want`に対して呼ぶ | `cb.GetWants()`で別のwantを取得してそのstateに書く |
 | ThinkAgent `ThinkFunc` | `want.StoreState(...)`（自分自身）と `want.MergeParentState(...)` | 兄弟WantやChildWantのstateに直接書く |

+## エージェント登録ルール
+
+**`Initialize()` などのコード内で `AddThinkingAgent()` や `AddMonitoringAgent()` を直接呼び出すことは原則禁止です。**
+
+### 正しい実装パターン
+
+*   **YAMLの `requires` を使用**: 必要なCapabilityを `requires` に記載することで、フレームワークが適切にエージェントをアサイン・起動します。
+*   **Targetの例外**: `Target` 型（およびそれを拡張した型）のように、動的に子Wantを管理するコア実装において、YAMLで定義できない特殊なシステムエージェント（`DispatchThinkerAgent` など）が必要な場合に限り、`Initialize()` での直接登録を許可します。
+
 ## 子Want作成の階層ルール

 **子Want（葉ノードや中間ノード）が直接別の子Wantを作成することは禁止されています。**

+
+### 正しい実装パターン
+
+1.  **中間Want (e.g. Itinerary)** は、次に実行すべきアクション（スペック）を自身のステート、または `MergeParentState()` を通じて親（Target）のステートに書き込みます。
+2.  **親Want (Target)** に登録された `DispatchThinkerAgent` が、そのステートを監視します。
+3.  **DispatchThinkerAgent** が、親に対して `AddChildWant()` を呼び出し、新しい子Wantを作成します。
+
+これにより、Wantの生成責任が常に上位の階層（Target/Recipe）に集約され、実行グラフの複雑化と管理不能な拡散を防ぎます。
+
 ---

 ## 状態管理API
### 他Wantのキャンセルが必要な場合の正しいパターン

WantのStatusを外から変えたい場合は**間接シグナル**を使います：

```go
// ✅ 正しい: フラグを書いてRestartWantで相手goroutineに処理させる
for _, w := range cb.GetWants() {
    if w.Metadata.ID == targetID {
        w.StoreState("_cancel_requested", true) // フラグ書き込みのみ
        break
    }
}
cb.RestartWant(targetID) // 相手のgoroutineを起こす → 相手自身がSetStatus(Cancelled)を呼ぶ
```

```go
// ❌ 禁止: 外部goroutineから直接Statusを変更する
for _, w := range cb.GetWants() {
    if w.Metadata.ID == targetID {
        w.SetStatus(WantStatusCancelled) // データ競合・所有権違反
    }
}
```

---

## 状態管理API

Want内の状態は以下のメソッドで管理します。**これらのメソッドの選択は重要です。**

### StoreState(key string, value any)

**用途：** 確定値（最終値）を設定する

**動作：**
```
State[key] = value              // State に直接書き込み（即座に反映）
pendingStateChanges[key] = value // pendingStateChanges にも追加
```

**特徴：**
- State に直接書き込むため、即座に GetState() から見えます
- pendingStateChanges にも追加されるため、EndProgressCycle() で履歴に記録されます
- **後からの上書きを安心して受けられる設計です**

**使用例：**
```go
// 最終的に達成した状態
t.StoreState("achieving_percentage", 100.0)

// 完了したと判定した時
t.StoreState("completion_status", "done")

// 確定的な結果値
t.StoreState("result", "success")
```

### MergeState(updates map[string]any)

**用途：** 非同期操作からの一時的な状態更新（複数の更新を「マージ」する）

**動作：**
```
pendingStateChanges[key] = value  // pendingStateChanges にのみ追加
```

**特徴：**
- State には**直接書き込まない**
- pendingStateChanges に追加するだけ
- GetState() は pendingStateChanges を優先して読みます（最新値は見えます）
- **EndProgressCycle() で pendingStateChanges が State に反映されるまで確定しません**
- **後から同じキーで StoreState() が呼ばれると上書きされます**

**使用例：**
```go
// 非同期イベントハンドラから複数の値を更新
coordinator.MergeState(map[string]any{
    "evidence_0":  evidence,
    "evidence_1":  evidence,
})

// その後、Progress() で確定値を StoreState() で設定
coordinator.StoreState("final_result", finalValue)
```

### ⚠️ MergeState の危険性

**実際のバグ例：**
```
Cycle 1: Progress()
  - allComplete = false
  - StoreState("achieving_percentage", 75%)
  - pendingStateChanges["achieving_percentage"] = 75%
  - EndProgressCycle() で State に 75% が反映

OnEvent: すべての子どもが完了
  - MergeState({"achieving_percentage": 100})
  - pendingStateChanges["achieving_percentage"] = 100 に変更

Cycle 2: Progress() 再実行
  - allComplete = false（古いデータで再評価）
  - StoreState("achieving_percentage", 75%)
  - pendingStateChanges["achieving_percentage"] = 75% に上書き ❌
  - EndProgressCycle() で State に 75% が反映（100% が失われた！）
```

**解決策：** 最終値は必ず **StoreState()** で設定し、MergeState() で上書きされないようにします。

## ロギングAPI

### StoreLog(format string, args ...any)

**用途：** Want実行ログを記録

**動作：**
- ログを fmt 形式で出力
- Want の履歴に記録される

**使用例：**
```go
t.StoreLog("[TARGET] Starting execution\n")
t.StoreLog("[TARGET] Child %s completed with status: %s\n", childName, status)
```

## exec cycle のライフサイクル

理解すべき重要な流れです：

```
1. BeginProgressCycle()
   - inExecCycle = true
   - pendingStateChanges をリセット

2. Progress() [複数回呼ぶ可能性あり]
   - MergeState() で値を pendingStateChanges に追加
   - StoreState() で値を State と pendingStateChanges に追加
   - GetState() は pendingStateChanges を優先して読み取る

3. EndProgressCycle()
   - CRITICAL: Status == WantStatusAchieved ならば achieving_percentage = 100 を強制
   - pendingStateChanges の値を State に反映（履歴に記録）
   - inExecCycle = false

重要：
- Progress() 内で MergeState() で設定した値は EndProgressCycle() まで確定しません
- 同じ exec cycle 内で同じキーに対して先に MergeState()、後に StoreState() を呼ぶと、
  StoreState() の値が最終的に State に反映されます
```

## achieving_percentage の管理（重要！）

achieving_percentage は Want の進捗を示す重要な指標です。

**ルール：**
1. **Status == WantStatusAchieved の場合、achieving_percentage は必ず 100.0**
2. SetStatus(WantStatusAchieved) を呼ぶと、SetStatus() 内で自動的に achieving_percentage = 100.0 が StoreState() される
3. achieving_percentage の設定は **StoreState() で**（MergeState() では**ない**）

**実装例：**
```go
func (t *CustomTarget) Progress() {
    // allComplete を判定
    if allComplete {
        // こうする：SetStatus() が achieving_percentage = 100 を自動設定
        t.SetStatus(WantStatusAchieved)

        // こうしない：
        // t.StoreState("achieving_percentage", 100.0)
        // t.SetStatus(WantStatusAchieved)
        // （二重設定になり、オーバーヘッド）

        return
    }

    // 進捗中の場合は百分率を計算して設定
    percentage := (completedCount * 100.0) / float64(totalCount)
    t.StoreState("achieving_percentage", percentage)
}
```

## GetState の使用法

### GetState(key string) (any, bool)

```go
value, exists := t.GetState("some_key")
if exists {
    // 値を使う
}
```

**順序：**
1. pendingStateChanges から取得（最新の pending 更新）
2. なければ State から取得（確定済みの値）

## よくある間違い

### ❌ MergeState で最終値を設定

```go
if allComplete {
    t.MergeState(Dict{"achieving_percentage": 100.0})  // 危険！
}
```

→ 後の Progress() で 75% に上書きされる可能性

### ✅ StoreState で最終値を設定

```go
if allComplete {
    t.StoreState("achieving_percentage", 100.0)  // 安全
}
```

→ State に直接書き込まれるため、上書きされない

### ❌ 非同期コンテキストで MergeState の値を確定と思う

```go
onEvent() {
    t.MergeState(Dict{"result": "done"})
}

// 別の exec cycle で
progress() {
    if !completed {
        t.StoreState("result", "pending")  // MergeState の値が上書きされた！
    }
}
```

### ✅ 最終値は常に StoreState で設定

```go
onEvent() {
    if allComplete {
        t.SetStatus(WantStatusAchieved)  // achieving_percentage = 100 は自動設定
    }
}
```

## パケット駆動設計の理解

Want の実行は**パケット駆動**で設計されています：

```go
// ユーザーがパケットを送信
coordinator.Provide(someData)

// ChainBuilder が受け取り、Receiver want を起動
// → Progress() が呼ばれる
```

**重要：** イベント駆動（パケットなし）では Progress() を再トリガーできません。
その場合は SetStatus() で状態を設定する必要があります。

## Progress の再開可能実装パターン（Resumable Progress）

### 原則

`Progress()` はいつでも中断される可能性があります（サーバー停止、goroutineの再スケジュール、エラーなど）。
再度 `Progress()` が呼ばれたとき、**中断前の途中状態から再開できる**実装が求められます。

### 基本ルール

> **欲しい値が State になければ、それを作るために必要な値を再帰的に確認する。**

明示的なステップ変数（`_step` など）は使いません。
**各値の存在そのものが再開ポイント**になります。

1. 最終的に欲しい値（例: `c`）が State にあれば完了
2. なければ `c` の依存値（例: `a`, `b`）を確認する
3. 依存値がなければ、それを取得・計算して State に保存する
4. すべての依存値が揃ったら `c` を計算して State に保存する

### 実装テンプレート（`c = a + b` の例）

```go
func (w *MyWant) Progress() {
    // 最終値 c が揃っていれば完了
    c, hasC := w.GetCurrent("c")
    if hasC {
        w.SetStatus(WantStatusAchieved)
        return
    }

    // c がない → a を確認
    a, hasA := w.GetCurrent("a")
    if !hasA {
        // a がない → 取得する
        fetched, err := fetchA()
        if err != nil {
            w.SetStatus(WantStatusFailed)
            return
        }
        w.SetCurrent("a", fetched)
        a = fetched
    }

    // b を確認
    b, hasB := w.GetCurrent("b")
    if !hasB {
        // b がない → 取得する
        fetched, err := fetchB()
        if err != nil {
            w.SetStatus(WantStatusFailed)
            return
        }
        w.SetCurrent("b", fetched)
        b = fetched
    }

    // a と b が揃った → c を計算して保存
    _ = c // 未使用変数の回避
    w.SetCurrent("c", a.(int) + b.(int))
    w.SetStatus(WantStatusAchieved)
}
```

**再開時の動作:**

| 中断タイミング | 再開時の挙動 |
|--------------|------------|
| `a` 取得前 | `a` の取得から再開 |
| `a` 取得後・`b` 取得前 | `a` はスキップ、`b` の取得から再開 |
| `b` 取得後・`c` 計算前 | `a`, `b` はスキップ、`c` の計算から再開 |
| `c` 保存後 | 冒頭で `c` を検出し即完了 |

### より複雑な依存関係

依存関係が深い場合も同じパターンを入れ子で適用します。

```go
func (w *MyWant) Progress() {
    // 最終値: summary
    if _, ok := w.GetCurrent("summary"); ok {
        w.SetStatus(WantStatusAchieved)
        return
    }

    // summary の依存: user_profile
    userProfile, hasProfile := w.GetCurrent("user_profile")
    if !hasProfile {
        // user_profile の依存: user_id
        userID, hasID := w.GetCurrent("user_id")
        if !hasID {
            id, err := resolveUserID()
            if err != nil { w.SetStatus(WantStatusFailed); return }
            w.SetCurrent("user_id", id)
            userID = id
        }
        profile, err := fetchProfile(userID)
        if err != nil { w.SetStatus(WantStatusFailed); return }
        w.SetCurrent("user_profile", profile)
        userProfile = profile
    }

    // summary の依存: recent_activity
    activity, hasActivity := w.GetCurrent("recent_activity")
    if !hasActivity {
        fetched, err := fetchActivity()
        if err != nil { w.SetStatus(WantStatusFailed); return }
        w.SetCurrent("recent_activity", fetched)
        activity = fetched
    }

    // すべて揃った → summary を生成
    w.SetCurrent("summary", buildSummary(userProfile, activity))
    w.SetStatus(WantStatusAchieved)
}
```

### アンチパターン

```go
// ❌ 悪い例: State を読まず毎回最初から処理する
func (w *MyWant) Progress() {
    a := fetchA()   // 再開時も毎回実行される（副作用・無駄なAPI呼び出し）
    b := fetchB()
    w.SetCurrent("c", a + b)
    w.SetStatus(WantStatusAchieved)
}
```

```go
// ❌ 悪い例: _step で手続き的に管理する
func (w *MyWant) Progress() {
    step, _ := w.GetInternal("_step")
    switch step {
    case "": fetchA(); w.SetInternal("_step", "got_a")   // 値ではなくステップを記録
    case "got_a": fetchB(); w.SetInternal("_step", "got_b")
    case "got_b": computeC(); w.SetStatus(WantStatusAchieved)
    }
    // → 依存関係が増えるほど複雑化し、値の再利用ができない
}
```

### チェックリスト（再開可能実装）

- [ ] 最終的に欲しい値を先に State から確認している
- [ ] 値がなければ、その依存値を再帰的に確認している
- [ ] 各値の取得・計算後に必ず State に保存している（次回はスキップされる）
- [ ] `_step` などの手続き的なフェーズ変数を使っていない
- [ ] 同じ `Progress()` が複数回呼ばれても正しく動作する（冪等性）
- [ ] 外部API呼び出しなど副作用のある処理は、結果を State に保存してから先に進む

---

## チェックリスト：カスタムWant型の開発

新しいWant型を作成する場合のチェックリスト：

- [ ] Progressable インターフェースを実装した
- [ ] Progress() メソッドを実装した
- [ ] 完了状態を判定して SetStatus(WantStatusAchieved) を呼ぶ
- [ ] achieving_percentage は StoreState() で設定している（MergeState()ではない）
- [ ] 状態更新は適切に StoreState()/MergeState() を使い分けている
- [ ] OnEvent ハンドラでは確定値のみを設定（또는 SetStatus() に任せる）
- [ ] ロギングを適切に追加した（StoreLog()）
- [ ] exec cycle 外での状態更新を避けている
- [ ] **【所有権】自Wantのみ StoreState()/SetStatus() を呼んでいる（他Wantのstateを直接変更していない）**
- [ ] **【所有権】Agent Exec内では want.StoreStateForAgent() を使っている（渡された want のみに書く）**
- [ ] **【所有権】他Wantへのキャンセル指示は _cancel_requested フラグ + RestartWant() 経由にしている**

## よくある間違いとトラブルシューティング

WantやAgentの開発において、特に状態管理（State）に関連して陥りやすい落とし穴をまとめました。

### 1. GCPラベル未定義によるサイレントな失敗（重要）

`SetCurrent` や `GetGoal` などのGCP専用メソッドを使用する場合、**YAML定義で正しいラベルが付与されていないと、操作はサイレントに無視されます。**

*   **症状:** Agentが `SetCurrent` を呼んでいるのにステートが更新されない、または `GetCurrent` が常に `nil` を返す。
*   **原因:** YAMLの `state:` セクションで `label: current` などが設定されていない。
*   **対策（外部公開フィールド）:** YAMLを確認し、アクセスしようとしているフィールドに適切なラベルを付けてください。
    ```yaml
    # ✅ 正しい例
    state:
      - name: actual_cost
        type: float64
        label: current  # これがないと SetCurrent/GetCurrent は動作しない
    ```

*   **対策（内部ステートマシン用フィールド）:** APIやUIに公開不要な内部状態（例: ステートマシンのフェーズ）は `CreateInternal` を使って `Initialize()` 内で動的に登録できます。YAMLへの定義は不要です。
    ```go
    // ✅ Initialize() 内でステートマシンの初期フェーズを登録
    func (f *FlightWant) Initialize() {
        // ...
        // YAMLに定義不要。内部フィールドを動的に生成してラベル登録する
        f.CreateInternal("_flight_phase", PhaseInitial)
    }
    ```
    `CreateInternal` は冪等です。すでに値が存在する場合は上書きしません。`StateLabels` に `internal` ラベルを登録したうえで値を保存するので、その後 `SetInternal` / `GetInternal` が正常に動作します。

### 2. フィールド名の不一致（タイポ）

Goのコード内では文字列キーでステートにアクセスするため、YAMLで定義した名前と1文字でも異なると動作しません。

*   **症状:** ログには「成功」と出ているが、UIや他のWantから値が見えない。
*   **原因:** YAMLでは `reminder_phase` と定義しているが、Goコードで `phase` と書いていた。
*   **対策:** キー名を定数化するか、YAMLとコードを注意深く突き合わせてください。

### 3. YAML定義がないフィールドの不可視性

`StoreState` などの低レベルメソッドを使えば、YAMLに定義のないフィールドにも書き込みは可能ですが、**外部（API、UI、コマンドライン）からは一切見えなくなります。**

*   **症状:** `DebugLog` では値が入っていることを確認できるが、`mywant wants get` の出力に現れない。
*   **原因:** YAMLの `state:` セクションにフィールド名が列挙されていない。
*   **対策:** APIやUIに公開したい動的なフィールドは、必ずYAMLの `state:` に追加してください。

### 4. リスト・マップの上書き問題

複数のソースから情報を集約する場合（例：複数のプランナーからの提案、複数の子Wantからのコスト集計）、`StoreState` を使うと前のデータが消えてしまいます。

*   **症状:** 最後に完了したWantのコストしか残っていない。複数の提案をしたはずなのに、最後の1つしか表示されない。
*   **原因:** `StoreState` でリストやマップ全体を毎回上書きしている。
*   **対策:** 
    *   **Map（辞書型）への追記:** `MergeState` または `MergeParentState` を使用してください。
    *   **List（配列型）への追記:** `SuggestParent`（重複排除機能付き）を使用してください。

### 5. 親Wantへの完了伝搬

`Target`（レシピ）の下で動くプランナーWant（例：`Itinerary`）が完了しても、親のステータスが自動的に完了するわけではありません。

*   **症状:** 子Wantはすべて `achieved` なのに、親の `Target` がずっと `reaching (50%)` のまま。
*   **原因:** 子Wantが親の `goal_achieved` フラグを更新していない。
*   **対策:** プランナーの最終ステップで親のステートを更新してください。
    ```go
    // Itineraryプランニング完了時
    w.MergeParentState(map[string]any{"goal_achieved": true})
    ```

## State ラベル 読み書き権限表

Wantのステート管理には5種類のラベルがあり、**誰が読み書きできるかが厳密に決まっています**。
ラベルに対応したAPI以外での読み書きはサイレントに無視されます。

### 権限表

| ラベル | YAML `label:` | 書ける主体 | 読める主体 | SyncLocalsState の扱い | 主な用途 |
|--------|--------------|-----------|-----------|------------------------|---------|
| **goal** | `goal` | `Initialize()` のみ | Progress() / Agent | 読み込みのみ (state→struct) | 外部から与えられた目標値。一度セットしたら変更しない |
| **current** | `current` | Progress() / **外部エージェント** / MergeParentState | Progress() / Agent / 親Want | 読み込みのみ (state→struct) | 現在の観測可能な状態。**外部から更新される値はここ** |
| **plan** | `plan` | **ThinkAgent** / **DoAgent**（実行可否フラグなど） | Progress() | 読み込みのみ (state→struct) | ThinkAgent が出力する計画、またはDoAgentが設定するアクションフラグ |
| **internal** | Locals struct tag | **Progress() 経由の Locals のみ** | Progress() のみ | 読み書き両方 (state↔struct) | Progress() が排他的に管理するステートマシンの内部状態 |
| **predefined** | — (YAML 不要) | フレームワーク / Progress() | 誰でも | 対象外 | `achieving_percentage`, `final_result` などシステム予約フィールド |

### internal ラベルの制約（重要）

`internal` は `SyncLocalsState(false)`（Progress後）が **Locals の全フィールドを state に上書き** するため、
Locals に含めたフィールドは「Progress() が唯一の書き込み主体」になります。

```
Progress()実行前: SyncLocalsState(state → Locals)  ← 外部の最新値を読み込む
Progress()実行
Progress()実行後: SyncLocalsState(Locals → state)  ← Locals の値で state を上書き ⚠️
```

**外部エージェントが `SetInternal` で書いても、次の Progress サイクルで Locals の古い値に上書きされます。**
これが `internal` を外部エージェントが使えない理由です。

### どのラベルを使うべきか

```
外部エージェント（DoAgent / ThinkAgent）が書く値
  → label: current  （YAML に定義、SetCurrent / GetCurrent を使用）

Progress() だけが読み書きする内部状態
  → Locals struct のフィールドに mywant:"internal,key_name" タグで定義

別 Want から MergeParentState で書かれる値（例: コスト集計）
  → label: current  （Locals には絶対に含めない）

外部から一度だけ与えられる目標値
  → label: goal  （Initialize() で SetGoal）

ThinkAgent が出力する計画、またはDoAgentが設定する実行フラグ
  → label: plan  （SetPlan / GetPlan）
```

### DoAgent のステートアクセス原則

DoAgent（`RegisterDoAgent` で登録される関数）は以下のルールに従う必要があります：

- **読み取り**: `GetCurrent` / `GetPlan` / `GetState[T]` を使用
- **書き込み**: **`SetCurrent` のみ**（エージェントが生成・更新するすべての値）
- **`SetPlan` も許可**: 次サイクルのアクションフラグ（`execute_booking`, `flight_action` など）
- **`SetInternal` は使用禁止**: internal は Progress()・Locals 専用。DoAgentが書くとSyncLocalsStateに上書きされる

```
✅ 正しい DoAgent の書き方:
  want.SetCurrent("status", "completed")
  want.SetCurrent("result", data)
  want.SetPlan("flight_action", "cancel_flight")   // アクション指示

❌ 禁止:
  want.SetInternal("_cache", value)   // → SetCurrent に変更
```

### ラベル違反の自動検出（module_error）

`SetGoal` / `SetCurrent` / `SetPlan` / `SetInternal` に対して、対応するラベルで登録されていないキーを渡すと：

1. ログに `[FAILED] SetCurrent("my_key") dropped: ...` が記録される（値は保存されない）
2. **2サイクル目以降**に違反があると Want のステータスが **`module_error`** になる

これにより、ラベル未登録のキーへの書き込みが本番環境でも確実に検出されます。
初回サイクル（Initialize 直後）のみスキップされます。

### API 早見表

| API | ラベル要件 | 主な呼び出し元 |
|-----|-----------|--------------|
| `want.SetGoal(key, val)` | YAML `label: goal` | `Initialize()` |
| `want.GetGoal(key)` | YAML `label: goal` | Progress() / Agent |
| `want.SetCurrent(key, val)` | YAML `label: current` | Progress() / **DoAgent** / ThinkAgent |
| `want.GetCurrent(key)` | YAML `label: current` | Progress() / **DoAgent** / ThinkAgent |
| `want.SetPlan(key, val)` | YAML `label: plan` | ThinkAgent / **DoAgent**（アクションフラグ） |
| `want.GetPlan(key)` | YAML `label: plan` | Progress() |
| `want.SetInternal(key, val)` | Locals struct tag のみ | Progress()（Locals 経由） |
| `want.GetInternal(key)` | Locals struct tag のみ | Progress()（Locals 経由） |
| `want.SetPredefined(key, val)` | 不要 | Progress() / フレームワーク |
| `want.StoreState(key, val)` | 不要（ラベルチェックなし） | フレームワーク内部のみ（非推奨） |
| `GetState[T](want, key, def)` | 不要（ラベルチェックなし） | ユーティリティ（`ShouldRunAgent` など） |
| `want.MergeParentState(map)` | 親側の定義に依存 | Agent / Progress() |
| `want.GetParentState(key)` | 親側の定義に依存 | Progress() |

## 参考資料

- [want-system.md](want-system.md) - Want システムの全体像
- [agent-system.md](agent-system.md) - Agent システム
- [target_execution_flow.md](target_execution_flow.md) - Target実行フロー
