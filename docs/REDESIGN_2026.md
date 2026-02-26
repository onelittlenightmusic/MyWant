# MyWant 再設計提案 2026

> テスト実行結果: **全テスト PASS** (engine/core, engine/core/chain, engine/core/pubsub, engine/server)
> 分析対象: ~19,700 LOC (Go) + 50+ YAML configs + 11 docs

---

## エグゼクティブサマリー

現在のMyWantは「何をしたいかをYAMLで宣言し、自律エージェントが実現する」という**優れたビジョン**を持つ。しかし実装は機能の追加とともに有機的に成長し、いくつかの構造的負債を抱えている。

**コアの問題:** `ChainBuilder` が3500行超の**God Object**になっており、6つ以上のMutexが混在する「ロックのスープ」状態。これがスケーラビリティとメンテナンス性の根本的なボトルネックになっている。

このドキュメントは、ビジョンを維持しながら**2026年のベストプラクティス**で再設計した場合の提案を示す。

---

## 1. 現在のアーキテクチャの問題点

### 1.1 ChainBuilder God Object

```go
// 現在: 1つの構造体が全てを担う (chain_builder.go ~3500行)
type ChainBuilder struct {
    wants                map[string]*runtimeWant   // Want管理
    registry             map[string]WantFactory    // 型ファクトリ
    agentRegistry        *AgentRegistry            // エージェント管理
    channels             map[string]chain.Chan      // チャネル管理
    pubsub               pubsub.PubSub             // メッセージング
    labelRegistry        map[string]map[string]bool // ラベル管理
    apiLogs              []APILogEntry              // ログ

    // 6つ以上のMutex — "ロックのスープ"
    reconcileMutex     sync.RWMutex
    channelMutex       sync.RWMutex
    controlMutex       sync.RWMutex
    completedFlagsMutex sync.RWMutex
    pubsubMutex        sync.RWMutex
    labelRegistryMutex sync.RWMutex
    apiLogsMutex       sync.RWMutex

    // DEPRECATEDフィールドが残存
    addWantsChan    chan []*Want   // DEPRECATED
    deleteWantsChan chan []string  // DEPRECATED
}
```

### 1.2 型安全性の欠如

```go
// 現在: anyだらけでランタイムエラーのリスク
type Dict = map[string]any
State    map[string]any
Params   map[string]any
function any  // runtimeWantのfunction フィールド
```

### 1.3 状態管理の問題

- **無制限の履歴成長**: StateHistoryEntryがリングバッファなし
- **YAML永続化**: 毎秒全体をシリアライズ (GlobalStatsInterval = 1s)
- **トランザクションなし**: 部分的な状態更新の失敗時に不整合が起きうる

### 1.4 エージェント実行の不一致

3つの実行モード(Local/Webhook/RPC)がそれぞれ異なるタイムアウト・リトライセマンティクスを持ち、推論が困難。

---

## 2. 再設計の原則

1. **単一責任**: 各コンポーネントは1つのことだけを行う
2. **チャネルファースト**: MutexよりGoroutine + Channelで並行性を管理
3. **型安全**: コンパイル時にエラーを検出
4. **イベントソーシング**: 状態変化を不変なイベントとして記録
5. **アクターモデル**: Want = Actorとして隔離と耐障害性を実現
6. **観測可能性**: OpenTelemetry統合をファーストクラスで

---

## 3. 提案するアーキテクチャ

### 3.1 全体像

```
┌─────────────────────────────────────────────────────────────┐
│                    MyWant Runtime                            │
│                                                             │
│  ┌──────────┐    ┌──────────┐    ┌──────────┐             │
│  │  Want    │    │  Want    │    │  Want    │             │
│  │  Actor   │◄──►│  Actor   │◄──►│  Actor   │             │
│  └────┬─────┘    └────┬─────┘    └────┬─────┘             │
│       │               │               │                    │
│  ┌────▼───────────────▼───────────────▼──────────────┐    │
│  │            Event Bus (typed, async)                │    │
│  └────┬───────────────┬───────────────┬──────────────┘    │
│       │               │               │                    │
│  ┌────▼──────┐  ┌─────▼─────┐  ┌─────▼──────┐           │
│  │  Want     │  │  Agent    │  │  State     │           │
│  │ Registry  │  │ Scheduler │  │  Store     │           │
│  └───────────┘  └───────────┘  └────────────┘           │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐  │
│  │              Observability Layer                     │  │
│  │     (OpenTelemetry: Traces + Metrics + Logs)         │  │
│  └─────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

### 3.2 コンポーネント分解

ChainBuilderを**5つの専門コンポーネント**に分解する:

```
ChainBuilder (現在: 3500行) → 分解後:
├── WantRegistry        (Want CRUD + ファクトリ管理)
├── ConnectivityGraph   (ラベルセレクタ → グラフ解決)
├── ExecutionScheduler  (Goroutineライフサイクル)
├── StateStore          (永続化 + イベントログ)
└── AgentOrchestrator   (エージェントライフサイクル)
```

---

## 4. 詳細設計

### 4.1 型安全なWant DSL

```go
// 現在: map[string]any だらけ
type Want struct {
    State  map[string]any
    Params map[string]any
}

// 提案: ジェネリクス + 型付きDSL (Go 1.21+)
type Want[S StateSpec, P ParamsSpec] struct {
    Metadata WantMetadata
    Spec     TypedSpec[P]
    State    TypedState[S]
    Status   WantStatus
}

// ユーザー定義の型付き状態
type FlightBookingState struct {
    BookingID string    `yaml:"booking_id" state:"required"`
    Status    string    `yaml:"status"`
    FlightAt  time.Time `yaml:"flight_at"`
    Cost      float64   `yaml:"cost"`
}

type FlightBookingParams struct {
    Origin      string `yaml:"origin" param:"required"`
    Destination string `yaml:"destination" param:"required"`
    Date        string `yaml:"date"`
    MaxBudget   float64 `yaml:"max_budget"`
}

// 型安全なファクトリ登録
registry.Register(WantFactory[FlightBookingState, FlightBookingParams]{
    Type: "flight_booking",
    New:  func(p FlightBookingParams) Progressable[FlightBookingState] {
        return &FlightBookingWant{params: p}
    },
})
```

### 4.2 アクターモデルによるWant実行

```go
// 提案: Want = Actor with mailbox
type WantActor struct {
    id       string
    mailbox  chan Message       // 受信メッセージキュー (バッファ付き)
    state    atomic.Value      // lock-freeな状態アクセス
    children []*WantActor      // 子アクター (supervision tree)
    parent   *WantActor        // 親アクター
    ctx      context.Context
    cancel   context.CancelFunc
}

// メッセージ型
type Message interface{ messageTag() }

type (
    PacketMessage   struct{ Payload any }
    StopMessage     struct{ Reason string }
    SuspendMessage  struct{}
    ResumeMessage   struct{}
    AgentResultMsg  struct{ Key string; Value any }
    ParamChangeMsg  struct{ Key string; Value any }
)

// アクター実行ループ — Mutexなし
func (a *WantActor) run() {
    for {
        select {
        case msg := <-a.mailbox:
            a.handle(msg)
        case <-a.ctx.Done():
            a.cleanup()
            return
        }
    }
}
```

**メリット:**
- ロック競合ゼロ: メッセージパッシングのみ
- 隔離: アクターがクラッシュしても他に影響しない
- Supervision tree: 子アクターの障害を親が検知・再起動
- テスト容易性: メッセージを注入してテスト

### 4.3 型付きイベントバス

```go
// 現在: 緩やかに型付けられたPubSub
type StateNotification struct {
    StateValue any // 何でも入る
}

// 提案: 型安全なイベントバス
type Event[T any] struct {
    ID        ulid.ULID
    Source    WantID
    Timestamp time.Time
    Payload   T
}

// イベント型の例
type PacketSent[T any]   struct{ Packet T }
type WantCompleted       struct{ WantID string; Status Status }
type AgentExecuted       struct{ AgentName string; Duration time.Duration; Error error }
type StateChanged[T any] struct{ Key string; OldValue T; NewValue T }

// 型付き購読
bus.Subscribe[PacketSent[FlightData]](func(e Event[PacketSent[FlightData]]) {
    // コンパイル時に型チェック済み
    fmt.Println(e.Payload.Packet.Origin)
})
```

### 4.4 イベントソーシングによる状態管理

```go
// 現在: ミュータブルな状態 + YAML永続化
type Want struct {
    State map[string]any // 直接変更
}

// 提案: イベントログ + Projection
type StateStore interface {
    // コマンド
    Append(ctx context.Context, wantID string, event StateEvent) error

    // クエリ
    CurrentState(ctx context.Context, wantID string) (WantState, error)
    StateAt(ctx context.Context, wantID string, t time.Time) (WantState, error)
    History(ctx context.Context, wantID string, opts HistoryOptions) ([]StateEvent, error)
}

type StateEvent struct {
    ID        ulid.ULID
    WantID    string
    AgentName string    // 誰が変更したか
    Timestamp time.Time
    Changes   map[string]StateChange  // Keyごとの新旧値
}

type StateChange struct {
    OldValue any
    NewValue any
}

// 永続化バックエンド (差し替え可能)
type StorageBackend interface {
    Append(event StateEvent) error
    ReadEvents(wantID string, from ulid.ULID) ([]StateEvent, error)
    Snapshot(wantID string) (WantState, ulid.ULID, error) // スナップショット + 最終イベントID
}

// 実装例
var _ StorageBackend = &SQLiteBackend{}   // 本番: SQLite
var _ StorageBackend = &InMemoryBackend{} // テスト: In-memory
var _ StorageBackend = &YAMLBackend{}     // 移行期: 後方互換
```

**メリット:**
- **監査ログ**: 誰が何をいつ変更したか完全記録
- **タイムトラベルデバッグ**: 任意の時点の状態を再現
- **バウンドされた履歴**: スナップショット + 差分で効率的
- **競合解決**: イベントの順序が明確

### 4.5 ConnectivityGraph — ラベルセレクタ改善

```go
// 現在: O(n)線形スキャン
// labelToUsers: map[string][]string  // "key=value" -> wants

// 提案: DAGベースの依存グラフ
type ConnectivityGraph struct {
    nodes map[WantID]*GraphNode
    edges []*Edge  // source -> target + label selector

    // インデックス (高速ルックアップ)
    labelIndex *InvertedIndex // label -> []WantID
    typeIndex  map[WantType][]WantID
}

type GraphNode struct {
    WantID     WantID
    Labels     Labels
    WantType   WantType
    InEdges    []*Edge  // 入力接続
    OutEdges   []*Edge  // 出力接続
}

// トポロジカルソートで実行順序決定
func (g *ConnectivityGraph) ExecutionOrder() ([][]WantID, error) {
    // Kahnのアルゴリズム
    // 返値: 並列実行可能なグループのリスト
}

// 循環依存の検出
func (g *ConnectivityGraph) DetectCycles() []Cycle {
    // DFS with coloring
}
```

### 4.6 統一エージェント実行モデル

```go
// 現在: Local/Webhook/RPCが異なるセマンティクス
// 提案: 統一インターフェース + プラガブルトランスポート

type AgentTransport interface {
    Execute(ctx context.Context, req AgentRequest) (AgentResponse, error)
    Name() string
}

type AgentRequest struct {
    AgentName  string
    WantID     string
    WantState  WantState  // コピー (not pointer)
    Parameters map[string]any
    Timeout    time.Duration
    RetryPolicy RetryPolicy
}

type AgentResponse struct {
    StateUpdates map[string]any
    Logs         []string
    Metadata     map[string]any
}

// トランスポート実装
var _ AgentTransport = &LocalTransport{}    // In-process
var _ AgentTransport = &WebhookTransport{}  // HTTP
var _ AgentTransport = &RPCTransport{}      // gRPC
var _ AgentTransport = &WASMTransport{}     // WASM (新規: tetratelabs/wazero使用)

// 統一リトライポリシー
type RetryPolicy struct {
    MaxAttempts     int
    InitialInterval time.Duration
    MaxInterval     time.Duration
    Multiplier      float64  // 指数バックオフ
    Jitter          bool
}
```

**WASMトランスポートの追加 (wazeroは既にdependency):**
```go
// エージェントをWASMプラグインとして実行
// - エージェントをホットスワップ可能 (サーバー再起動不要)
// - 言語非依存 (Go/Rust/Python → WASM)
// - サンドボックス実行 (セキュリティ)
```

### 4.7 観測可能性レイヤー (OpenTelemetry)

```go
// 現在: log.Printf + 独自ログ構造
// 提案: OpenTelemetry統合

type ObservabilityConfig struct {
    TraceExporter  string  // "otlp", "jaeger", "stdout"
    MetricExporter string  // "prometheus", "otlp"
    LogExporter    string  // "otlp", "stdout"
    ServiceName    string
    SampleRate     float64
}

// Wantライフサイクルの自動計装
func (a *WantActor) run() {
    ctx, span := tracer.Start(a.ctx, "want.execution",
        trace.WithAttributes(
            attribute.String("want.id", a.id),
            attribute.String("want.type", a.wantType),
        ),
    )
    defer span.End()
    // ...
}

// メトリクス (Prometheus形式)
var (
    wantExecutionDuration = histogram("want_execution_duration_seconds",
        []string{"want_type", "status"})
    agentExecutionTotal = counter("agent_execution_total",
        []string{"agent_name", "mode", "status"})
    wantStateTransitions = counter("want_state_transitions_total",
        []string{"want_type", "from_status", "to_status"})
    activeWants = gauge("want_active_count",
        []string{"want_type", "status"})
)
```

---

## 5. YAMLスキーマの進化

### 5.1 型スキーマ定義の改善

```yaml
# 現在: 型定義が散在し、GoとYAMLが手動同期
# yaml/want_types/*.yaml と engine/types/*.go が別々

# 提案: 単一の真実のソース (OpenAPI/JSON Schema)
# yaml/schemas/flight_booking.schema.yaml

$schema: "https://json-schema.org/draft/2020-12"
$id: "mywant/want-types/flight_booking"
title: "Flight Booking Want"
description: "Books a flight between two airports"

properties:
  params:
    type: object
    required: [origin, destination, date]
    properties:
      origin:
        type: string
        pattern: "^[A-Z]{3}$"
        description: "IATA airport code"
      destination:
        type: string
        pattern: "^[A-Z]{3}$"
      date:
        type: string
        format: date
      max_budget:
        type: number
        minimum: 0

  state:
    type: object
    properties:
      booking_id:
        type: string
        readOnly: true  # エージェントのみが書き込み可
      status:
        type: string
        enum: [pending, confirmed, cancelled, failed]
      cost:
        type: number
        minimum: 0

  connectivity:
    inputs:
      min: 0
      max: 1
      accepts: ["itinerary_coordinator"]
    outputs:
      min: 0
      max: 1
      provides: "flight_booking_result"
```

### 5.2 コード生成パイプライン

```
yaml/schemas/*.schema.yaml
    ↓ (go generate)
engine/types/generated/*.go  (型定義)
    ↓ (コンパイル)
型安全なWant実装
```

```go
//go:generate mywant-codegen -schemas ../yaml/schemas/ -out ./generated/

// 生成されるコード例:
// engine/types/generated/flight_booking_want.go

type FlightBookingParams struct {
    Origin      string  `yaml:"origin" validate:"required,len=3"`
    Destination string  `yaml:"destination" validate:"required,len=3"`
    Date        string  `yaml:"date" validate:"required,datetime=2006-01-02"`
    MaxBudget   float64 `yaml:"max_budget" validate:"min=0"`
}

type FlightBookingState struct {
    BookingID string  `yaml:"booking_id"`
    Status    string  `yaml:"status" validate:"oneof=pending confirmed cancelled failed"`
    Cost      float64 `yaml:"cost" validate:"min=0"`
}

func init() {
    // 自動登録
    RegisterWantType[FlightBookingState, FlightBookingParams]("flight_booking")
}
```

---

## 6. 移行戦略

段階的移行で**後方互換性**を維持しながら改善する:

### Phase 1: 内部整理 (リスク低)
- [ ] ChainBuilderをサブパッケージに分解 (外部APIは変えない)
- [ ] DEPRECATEDフィールドの削除 (`addWantsChan`, `deleteWantsChan`)
- [ ] StateHistoryをリングバッファに置換 (デフォルト1000エントリ上限)
- [ ] MutexをSingleflightパターンで削減

### Phase 2: 型安全化 (リスク中)
- [ ] `map[string]any` → ジェネリクス型 (Go 1.21+)
- [ ] スキーマ → コード生成パイプライン構築
- [ ] バリデーションレイヤー追加 (YAMLロード時)

### Phase 3: 実行エンジン刷新 (リスク高)
- [ ] GoroutineモデルからActorモデルへ移行
- [ ] イベントソーシングのStateStore実装 (SQLiteバックエンド)
- [ ] 統一エージェント実行インターフェース

### Phase 4: 観測可能性 (リスク低)
- [ ] OpenTelemetry統合
- [ ] Prometheusメトリクスエンドポイント
- [ ] 構造化ログ (zerolog/slog)

---

## 7. 新規追加すべき機能

### 7.1 Wantテンプレート + パラメータ継承

```yaml
# 現在: レシピは固定パラメータ
# 提案: Helmのような階層的パラメータ継承

values.yaml:
  global:
    max_budget: 5000
    currency: JPY

travel-recipe.yaml:
  wants:
    - type: flight_booking
      params:
        max_budget: "{{ .global.max_budget * 0.4 }}"  # テンプレート式
```

### 7.2 Want-to-Want 通信の型付け

```yaml
# 現在: パケットの型が不明
# 提案: 明示的な型付きチャネル

wants:
  - name: flight-search
    outputs:
      - name: results
        type: "FlightSearchResult"  # 型付き出力

  - name: flight-filter
    inputs:
      - from: flight-search.results
        type: "FlightSearchResult"  # 型チェック
```

### 7.3 サーキットブレーカー + バックプレッシャー

```yaml
# エージェント障害耐性
agents:
  - name: flight_api_agency
    resilience:
      circuit_breaker:
        failure_threshold: 5
        recovery_timeout: 30s
      rate_limit:
        requests_per_second: 10
      backpressure:
        max_queue_depth: 100
        strategy: "drop_oldest"  # or "block", "drop_newest"
```

### 7.4 Want条件式の改善

```yaml
# 現在: 条件式は限定的
# 提案: CEL (Common Expression Language) 統合

wants:
  - name: hotel-booking
    spec:
      when: |
        flight.status == "confirmed" &&
        budget.hotel_allocation > hotel.min_price * 1.1
      requires:
        - hotel_agency
      retry:
        on_condition: "hotel.status == 'price_changed'"
        max_retries: 3
```

---

## 8. 実装優先度マトリクス

| 改善項目 | 影響度 | 実装コスト | 優先度 |
|---------|--------|-----------|--------|
| StateHistory上限追加 | 高 | 低 | **P0** |
| DEPRECATEDフィールド削除 | 中 | 低 | **P0** |
| ChainBuilder分解 | 高 | 中 | **P1** |
| 型安全なWant登録 | 高 | 中 | **P1** |
| SQLite StateStore | 高 | 中 | **P1** |
| OpenTelemetry統合 | 中 | 低 | **P1** |
| アクターモデル移行 | 高 | 高 | **P2** |
| WASMエージェント | 中 | 高 | **P2** |
| コード生成パイプライン | 高 | 高 | **P2** |
| CEL条件式 | 中 | 中 | **P3** |

---

## 9. 設計の判断と根拠

### なぜアクターモデルか
Goのgoroutineはアクターモデルと相性が良い。現在のgoroutine-per-wantモデルはすでにアクターに近いが、Mutexで保護された共有状態へのアクセスが混在している。純粋なメッセージパッシングに移行することで:
- デッドロックのリスク排除
- より明確な所有権モデル
- 単体テストのしやすさ (メッセージを注入するだけ)

### なぜイベントソーシングか
Wantは「達成したい状態への道筋」を表す。その道筋を事後に再現・デバッグできることは、この種のシステムでは本質的に重要。現在のYAMLスナップショット方式では「なぜこの状態になったか」が分からない。

### なぜSQLiteか (YAMLファイルではなく)
- **ACID保証**: トランザクション内での一貫した書き込み
- **クエリ**: 状態の集計・フィルタが簡単
- **パフォーマンス**: YAMLシリアライズより100倍以上高速
- **ゼロ依存**: SQLiteは組み込みで外部サービス不要
- `modernc.org/sqlite` でCGO不要のGo実装あり

### なぜジェネリクスか
Go 1.21の `map[string]any` からジェネリクスへの移行で:
- ランタイムパニックがコンパイルエラーに変わる
- IDEの補完・リファクタリングが機能する
- ゼロコストの型チェック

---

## 結論

MyWantは**宣言的ワークフロー + 自律エージェント**という2026年に最も重要になるアーキテクチャパターンを先取りしたシステム。コアのビジョンは正しく、必要なのは実装の構造的整理だ。

最も重要な3つの改善:

1. **ChainBuilderの分解** → 各コンポーネントが理解しやすく、テストしやすくなる
2. **SQLite StateStore** → 状態管理の信頼性と観測可能性が劇的に向上する
3. **アクターモデル** → スケーラビリティとデッドロックフリーな並行性を実現する

これらはすべて段階的に移行可能であり、既存のYAML設定との後方互換性を維持できる。
