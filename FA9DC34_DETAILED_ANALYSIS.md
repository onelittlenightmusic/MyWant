# fa9dc34 コミット詳細分析

## コミット情報
- **Hash**: fa9dc34f7d995021a3eaa8a4deb5d62ade658a7b
- **Date**: 2025-12-01 00:50:27 (+0900)
- **Author**: Hiro Osaki
- **Title**: `fix: Remove reconciliation trigger queue, rely on ticker-based reconciliation`

---

## 主な目的

**問題**: 動的トラベルプランナーのretrigger機構が正常に動作していない
- サーバー起動時にハングが発生
- Idleに設定されたwantsがRunningに遷移しない

**解決策**: reconciliation trigger queueを削除し、既存のticker-basedなreconciliation（100ms間隔）に統一

---

## 変更されたファイル (15ファイル)

### 1. **engine/src/chain_builder.go** (コア修正)

#### 削除内容:
```go
// 削除されたコード
anyWantRetriggered := false

// if anyWantRetriggered {
//     select {
//     case cb.reconcileTrigger <- &TriggerCommand{Type: "reconcile"}:
//         // OK
//     default:
//         // Channel full
//     }
// }
```

**理由**: reconciliation trigger queueが原因でサーバーハングが発生していた

#### 追加内容:
```go
// startPhase()に追加: Idle Coordinatorの診断ログ
for wantName, want := range cb.wants {
    if want.want.GetStatus() == WantStatusIdle && wantName == "dynamic-travel-coordinator-5" {
        InfoLog("[STARTPHASE] Found Idle coordinator, processing...\n")
        break
    }
}
```

**目的**: Coordinator状態遷移の検証

**新しい戦略**:
- Queue型のreconciliation triggerを廃止
- ticker-basedなreconciliation（100ms周期）に任せる
- `checkAndRetriggerCompletedWants()`で`SetStatus(WantStatusIdle)`を呼び出すと、次のreconciliation cycleで自動的に処理される

---

### 2. **engine/cmd/types/coordinator_types.go** (Retriggerロジック追加)

#### Retriggerの検出ロジック追加:

```go
// Retrigger検出: 完了後に新しいデータがあるか確認
completionKey := c.DataHandler.GetCompletionKey()
isCompleted, _ := c.GetStateBool(completionKey, false)
if isCompleted {
    hasNewData := false
    for i := 0; i < inCount; i++ {
        in, inChannelAvailable := c.GetInputChannel(i)
        if !inChannelAvailable {
            continue
        }
        select {
        case <-in:
            hasNewData = true
            break
        default:
        }
    }
    if hasNewData {
        // 状態をリセットしてretriggerを開始
        c.StoreState(completionKey, false)
        c.receivedFromIndex = make(map[int]bool)
        c.StoreLog(fmt.Sprintf("[RETRIGGER] Detected new data while completed, resetting state"))
    } else {
        // 新しいデータがない→完了を保つ
        return true
    }
}
```

**詳細**:
1. Coordinatorが完了した状態（`isCompleted == true`）をチェック
2. 全入力チャネルをnon-blockingで読む
3. 新しいデータが到着していたら:
   - completion状態をFalseにリセット
   - `receivedFromIndex`マップをクリア
   - 再度処理を開始
4. 新しいデータがなければ→完了のままreturn

#### ロギング追加:
```go
c.StoreLog(fmt.Sprintf("[RECV] Received data on channel %d: %+v", i, data))
```

---

### 3. **engine/src/want.go** (Retrigger通知インフラ)

#### SetStatus()に追加:
```go
if status == WantStatusIdle {
    InfoLog("[RETRIGGER:SETSTATUS] Setting '%s' to Idle (from %v)\n", n.Metadata.Name, oldStatus)
}

// ChainBuilderのcompleted flagを更新
cb := GetGlobalChainBuilder()
if cb != nil {
    cb.UpdateCompletedFlag(n.Metadata.Name, status)
}
```

**目的**:
- Idle状態遷移をログで追跡
- ChainBuilderに完了フラグを通知

#### 新しいメソッド追加:
```go
// NotifyRetriggerViaDataReceived()
// 完了したwantが依存wantに新しいデータを送った時に呼び出される
func (w *Want) NotifyRetriggerViaDataReceived(cb *ChainBuilder, sourceWantName string, payload interface{}) {
    // WantRetriggerEventを発行
    event := &WantRetriggerEvent{
        SourceWant:  sourceWantName,
        TargetWants: []string{w.Metadata.Name},
        Reason:      "completed_want_sent_data",
        Payload:     payload,
        Scope:       "local",
    }
    GetGlobalSubscriptionSystem().Emit(ctx, event)
}
```

**目的**: Subscription systemを通じたAsync retrigger通知

---

### 4. **engine/cmd/types/flight_types.go** (Rebooking時の修正)

#### 主な変更: Rebooking時の出力チャネル再取得

```go
// Rebooking後の出力チャネルを新しく取得（retrigger flow用）
rebookOut, rebookConnectionAvailable := f.GetFirstOutputChannel()
f.StoreLog(fmt.Sprintf("[REBOOK-CHAN] GetFirstOutputChannel: available=%v", rebookConnectionAvailable))

if rebookConnectionAvailable {
    rebookOut <- travelSchedule

    // Retrigger検出を開始（依存wantに新しいデータを通知）
    cb := GetGlobalChainBuilder()
    if cb != nil {
        f.StoreLog("[RETRIGGER] Triggering completed want retrigger check for dependencies")
        cb.TriggerCompletedWantRetriggerCheck()
    }
} else {
    f.StoreLog("[REBOOK-CHAN] ERROR: No output channel available!")
}
```

**重要**:
- `GetFirstOutputChannel()`を新しく呼び出す（キャッシュしない）
- Rebooking完了後に`TriggerCompletedWantRetriggerCheck()`を明示的に呼び出す
- このメソッドが`checkAndRetriggerCompletedWants()`をトリガー

#### コード整形修正:
- インデント修正（多くのタイプミスがあった）

---

### 5. **その他のWant型へのロギング追加**

**影響を受けたファイル**:
- `fibonacci_loop_types.go`
- `fibonacci_types.go`
- `prime_types.go`
- `qnet_types.go`
- `travel_types.go`

**追加内容**: 基本的な診断ロギング
- Want初期化時
- 重要な状態遷移時
- データ送受信時

---

### 6. **テスト・ドキュメント追加**

#### ドキュメント:
1. **test_results/README.md** (170行)
   - テスト結果のディレクトリ説明
   - テスト実行方法

2. **test_results/RETRIGGER_DEBUG_ANALYSIS.md** (145行)
   - Retrigger機構のデバッグ分析
   - 問題の根本原因を特定

3. **ASYNC_RETRIGGER_TEST_SUMMARY.md** (212行)
   - Async retrigger機構の実装概要

4. **FLIGHT_CODE_PATH_INVESTIGATION.md** (178行)
   - Flight rebooking のコードパス分析

#### テストシナリオ:
- **test_scenarios/dynamic_travel_retrigger_test.sh**
  - 動的トラベルretriggerの自動テスト
  - サーバー状態の監視
  - 結果のJSON出力

#### テスト結果:
- 複数の実行結果 (37個のテストケース)
- 各実行のJSONと詳細ログ
- 実行日時: 2025-11-30

---

## アーキテクチャの変化

### Before (問題のあった状態)

```
Flight Rebooking
    ↓
TriggerCompletedWantRetriggerCheck() [呼び出されない]
    ↓
checkAndRetriggerCompletedWants()
    ↓
SetStatus(WantStatusIdle)
    ↓
reconcileTrigger <- TriggerCommand  [Queue型]
    ↓
でも、queueがfullか何かで失敗...
    ↓
Coordinator: Idleのまま
    ↓
✗ 完了しない
```

### After (修正後)

```
Flight Rebooking
    ↓
TriggerCompletedWantRetriggerCheck()
    ↓
checkAndRetriggerCompletedWants()
    ↓
SetStatus(WantStatusIdle)
    ↓
"No trigger queuing anymore"
    ↓
Next reconciliation cycle (100ms interval)
    ↓
startPhase() で Idle wants を検出
    ↓
Coordinator: Idle → Running
    ↓
Input channels をnon-blocking readで確認
    ↓
新しいデータを検出 → 処理開始
    ↓
✓ 完了へ向かう
```

**キーポイント**:
- Trigger queueを完全に削除
- Ticker-basedなreconciliation（100ms周期）に完全に依存
- より単純で信頼性の高い設計

---

## 実装の流れ (Retrigger)

### 1. **Flight Rebooking検出**
   - `FlightWant.Exec()` でcancellationを検出
   - `tryAgentExecution()` で新しいflight scheduleを作成

### 2. **Rebooking後の出力**
   ```go
   rebookOut, _ := f.GetFirstOutputChannel()
   rebookOut <- travelSchedule  // Coordinatorへ送信
   ```

### 3. **Retrigger check開始**
   ```go
   cb.TriggerCompletedWantRetriggerCheck()
   ```
   - これが `checkAndRetriggerCompletedWants()` を直接呼び出す

### 4. **Coordinator状態のリセット**
   ```go
   runtimeWant.want.SetStatus(WantStatusIdle)
   ```
   - Coordinator: Completed → Idle

### 5. **次のreconciliation cycleで再実行**
   ```go
   // 100ms後のstartPhase()で自動実行
   if want.want.GetStatus() == WantStatusIdle {
       // ConnectivityがOKならこれを実行
       want.want.Exec()
   }
   ```

### 6. **Coordinatorの新しいデータ検出**
   - `Coordinator.Exec()` で入力チャネルをnon-blocking read
   - 新しいデータ（rebooking schedule）を検出
   - 処理を開始 → 完了 → 依存wantへ発信

---

## テスト状況

### テスト結果サマリー
```
Date: 2025-11-30
Test Runs: 37+

Current Status:
- ✓ Coordinator status transitions: Working (Idle → Running → Completing)
- ✗ Data payload updates: Not yet complete
  └─ Reason: Data from Flight rebooking not reaching final state
```

### 次のステップ
1. Coordinator retrigger後、完了後のデータペイロード更新
2. Flight rebookingからCoordinatorへのデータフロー確認
3. エンドツーエンドの動的retrigger検証

---

## コード品質の改善

### 1. **インデント修正**
   - `flight_types.go` で大量のインデント誤りを修正
   - コード可読性向上

### 2. **ロギングの体系化**
   - `[RETRIGGER]` プレフィックスで一貫性
   - 各Want型で同じロギング戦略

### 3. **ドキュメンテーション**
   - Retrigger機構の実装概要をMarkdownで説明
   - テスト方法を明確化
   - デバッグ分析を記録

---

## 重要な変更点

| 項目 | Before | After |
|------|--------|-------|
| Reconciliation Trigger方式 | Queue型（channel） | Ticker型（100ms） |
| Coordinator Retrigger | explicit trigger送信 | 自動（次のcycle） |
| エラーハンドリング | Queue full時の失敗 | No failure point |
| 複雑度 | 高い（queue + ticker） | 低い（ticker only） |
| 信頼性 | 低い（queue issues） | 高い（単純設計） |

---

## 結論

**fa9dc34は大規模なアーキテクチャ修正コミット**:
- ❌ Queue型のreconciliation trigger（失敗の原因）を削除
- ✅ シンプルで信頼性の高いticker-basedの設計に統一
- 🔧 Retrigger検出ロジックを各Want型に追加
- 📝 テストシナリオとドキュメントを充実

**の目的**:
動的トラベルプランナーの「Flight rebooking → Coordinator retrigger → 新しいschedule反映」という一連のフローを実装するための基礎作業
