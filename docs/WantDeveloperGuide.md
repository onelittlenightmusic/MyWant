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

## 参考資料

- [want-system.md](want-system.md) - Want システムの全体像
- [agent-system.md](agent-system.md) - Agent システム
- [target_execution_flow.md](target_execution_flow.md) - Target実行フロー
