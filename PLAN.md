# PubSub-First パケット配信システム改修プラン

## 問題の根本原因

`reconcileWants()` が呼ばれるたびに `generatePathsFromConnections()` が実行され、タイミングによっては:
1. Provider (Evidence/Description) が古いチャネルに `Provide()` で送信
2. 同時に新しいチャネルが生成される
3. Consumer (Coordinator) は新しいチャネルを監視
4. 古いチャネルに送信されたパケットは消失

## 解決方針

**PubSub-First アーキテクチャ**: 直接チャネルではなく PubSub を唯一の配信経路にする

## 実装計画

### Phase 1: PubSub を唯一の配信経路にする

**ファイル**: `engine/src/want.go`

#### 1.1 Provide() の修正

```go
func (n *Want) Provide(packet any) error {
    cb := GetGlobalChainBuilder()

    // PubSub が唯一の配信経路
    if cb != nil && cb.pubsub != nil && len(n.Metadata.Labels) > 0 {
        topic := serializeLabels(n.Metadata.Labels)
        msg := &pubsub.Message{
            Payload:   packet,
            Timestamp: time.Now(),
            Done:      false,
        }

        if err := cb.pubsub.Publish(topic, msg); err != nil {
            ErrorLog("[PubSub] Failed to publish: %v", err)
            return err
        }
        InfoLog("[PROVIDE] Want '%s' published to PubSub topic '%s'", n.Metadata.Name, topic)
    }

    // 直接チャネルは削除（PubSub のみ使用）
    return nil
}
```

#### 1.2 ProvideDone() の修正

同様に PubSub のみ使用するように変更。

### Phase 2: Consumer 側を PubSub サブスクリプションのみに

**ファイル**: `engine/src/chain_builder.go`

#### 2.1 generatePathsFromConnections() の簡素化

```go
func (cb *ChainBuilder) generatePathsFromConnections() map[string]Paths {
    pathMap := make(map[string]*Paths)
    for wantName := range cb.wants {
        pathMap[wantName] = &Paths{
            In:  []PathInfo{},
            Out: []PathInfo{},
        }
    }

    // 直接チャネル生成を削除
    // PubSub パスのみを追加
    cb.addPubSubPaths(pathMap)

    result := make(map[string]Paths)
    for wantName, pathsPtr := range pathMap {
        result[wantName] = *pathsPtr
    }
    return result
}
```

#### 2.2 addPubSubPaths() の強化

- すべての `using` セレクターに対して PubSub サブスクリプションを作成
- キャッシュからの自動リプレイを活用
- 直接パスの重複チェックを削除（直接パスがなくなるため）

### Phase 3: PubSub キャッシュの信頼性向上

**ファイル**: `engine/src/pubsub/inmemory.go`

#### 3.1 キャッシュサイズの拡大

```go
const (
    DefaultCacheSize    = 100  // 10 → 100
    DefaultChannelBuffer = 100
)
```

#### 3.2 サブスクリプション時の完全リプレイ保証

```go
func (ps *InMemoryPubSub) Subscribe(topic string) pubsub.Subscription {
    // サブスクリプション作成時にキャッシュ全体をリプレイ
    // ブロッキングリプレイでパケット消失を防止
}
```

### Phase 4: OutCount/InCount の整合性

**ファイル**: `engine/src/want.go`

#### 4.1 GetOutCount() の修正

PubSub トピックへのサブスクライバー数を返すように変更:

```go
func (n *Want) GetOutCount() int {
    cb := GetGlobalChainBuilder()
    if cb != nil && cb.pubsub != nil && len(n.Metadata.Labels) > 0 {
        topic := serializeLabels(n.Metadata.Labels)
        stats := cb.pubsub.GetStats(topic)
        return stats.SubscriberCount
    }
    return 0
}
```

### Phase 5: inputs_received の実装

**ファイル**: `engine/cmd/types/coordinator_types.go`

#### 5.1 Progress() でカウンター更新

```go
func (c *CoordinatorWant) Progress() {
    // データ受信時
    if !ok {
        // タイムアウト
    } else if done {
        // DONE シグナル
    } else {
        // データ受信
        inputsReceived, _ := c.GetStateInt("inputs_received", 0)
        c.StoreState("inputs_received", inputsReceived + 1)
        // ... 既存の処理
    }
}
```

## 実装順序

1. **Phase 3** - PubSub キャッシュ強化（基盤整備）
2. **Phase 1** - Provide() を PubSub-only に
3. **Phase 2** - パス生成から直接チャネルを削除
4. **Phase 4** - OutCount/InCount 整合性
5. **Phase 5** - inputs_received 実装

## テスト計画

1. `make test-concurrent-deploy` - 並行デプロイテスト
2. Level 2 Approval ネステッドレシピテスト
3. Travel レシピテスト（複数プロバイダー）

## リスクと対策

| リスク | 対策 |
|--------|------|
| PubSub キャッシュ溢れ | キャッシュサイズ拡大 + 警告ログ |
| サブスクリプション前のパケット消失 | キャッシュリプレイ保証 |
| 既存テストの破損 | 段階的移行 + テスト更新 |

## 成功基準

- [ ] Level 2 Approval Coordinator が achieving_percentage=100% に到達
- [ ] data_by_channel に Evidence と Description の両方が存在
- [ ] inputs_received が正しくカウントされる
- [ ] 既存の Travel/Queue テストがパス
