# Want.Locals Interface Refactoring Plan

## 目的

現在、各Want型（FlightWant、RestaurantWant等）は個別のコンストラクタ関数（NewFlightWant、NewRestaurantWant等）を持っています。これらを統一し、共通の`New()`コンストラクタに変更することで、コード重複を削減し、メンテナンス性を向上させます。

## 現状分析

### 現在の構造

```go
// Flight型の例
type FlightWant struct {
    Want
    FlightType         string
    Duration           time.Duration
    DepartureDate      string
    monitoringStartTime time.Time
    monitoringActive   bool
}

func NewFlightWant(metadata Metadata, spec WantSpec) interface{} {
    flight := &FlightWant{
        Want: Want{},
        FlightType: "economy",
        // ... その他の初期化
    }
    flight.Init(metadata, spec)
    flight.FlightType = flight.GetStringParam("flight_type", "economy")
    // ... パラメータ設定
    return flight
}
```

### 問題点

1. **コード重複**: 各Want型のコンストラクタがほぼ同じパターンを繰り返す
2. **拡張の手間**: 新しいWant型を追加するたびに新しいコンストラクタを作成が必要
3. **メンテナンス**: 共通ロジックの変更がすべてのコンストラクタに波及

## 提案されたアーキテクチャ

### 1. Want.Localsインターフェースの定義

```go
// want.go内
type Want struct {
    Metadata Metadata
    Spec WantSpec
    Status WantStatus
    State map[string]interface{}
    paths Paths

    // 新規追加: 型固有のローカル状態を保持
    Locals interface{} // FlightWantLocals, RestaurantWantLocals等を実装するInterface

    // ... その他のフィールド
}

// Want型固有のローカル状態を保持するインターフェース
type WantLocals interface {
    InitLocals(spec WantSpec)           // パラメータを読み込んでLocalsを初期化
    GetConnectivityMetadata() ConnectivityMetadata // 接続性メタデータを提供
    GetWantType() string                // Want型を返す
}
```

### 2. 型固有のLocals構造体

```go
// flight_types.go
type FlightWantLocals struct {
    FlightType         string
    Duration           time.Duration
    DepartureDate      string
    monitoringStartTime time.Time
    monitoringDuration time.Duration
    monitoringActive   bool
    lastLogTime        time.Time
}

func (f *FlightWantLocals) InitLocals(spec WantSpec) {
    // WantSpec.Paramsからパラメータを読み込む
    // (注: spec自体にアクセスはできないので、Want経由でGetIntParam等を使う)
    if spec.Params != nil {
        if v, ok := spec.Params["flight_type"].(string); ok {
            f.FlightType = v
        }
        if v, ok := spec.Params["duration_hours"].(float64); ok {
            f.Duration = time.Duration(v * float64(time.Hour))
        }
    }
}

func (f *FlightWantLocals) GetWantType() string {
    return "flight"
}

func (f *FlightWantLocals) GetConnectivityMetadata() ConnectivityMetadata {
    return ConnectivityMetadata{
        RequiredInputs:  0,
        RequiredOutputs: 1,
        MaxInputs:       1,
        MaxOutputs:      1,
        WantType:        "flight",
        Description:     "Flight booking scheduling want",
    }
}
```

### 3. 統一されたNew()コンストラクタ

```go
// want.go内 - 共通コンストラクタファクトリ
type WantFactory func(metadata Metadata, spec WantSpec) WantLocals

func New(wantType string, metadata Metadata, spec WantSpec, localsFactory WantFactory) *Want {
    want := &Want{
        Metadata: metadata,
        Spec: spec,
        Status: WantStatusIdle,
        State: make(map[string]interface{}),
        paths: Paths{
            In:  []PathInfo{},
            Out: []PathInfo{},
        },
    }

    // 型固有のLocalsを作成・初期化
    locals := localsFactory(metadata, spec)
    locals.InitLocals(spec)
    want.Locals = locals

    // 共通メタデータを設定
    want.WantType = locals.GetWantType()
    want.ConnectivityMetadata = locals.GetConnectivityMetadata()

    return want
}

// または、より簡潔に:
func (w *Want) Init(metadata Metadata, spec WantSpec, localsFactory WantFactory) {
    w.Metadata = metadata
    w.Spec = spec
    w.Status = WantStatusIdle
    w.State = make(map[string]interface{})
    w.paths = Paths{
        In:  []PathInfo{},
        Out: []PathInfo{},
    }

    if localsFactory != nil {
        locals := localsFactory(metadata, spec)
        locals.InitLocals(spec)
        w.Locals = locals
        w.WantType = locals.GetWantType()
        w.ConnectivityMetadata = locals.GetConnectivityMetadata()
    }
}
```

### 4. 登録時の使用方法

```go
// chain_builder.go内
func RegisterFlightWantTypes(builder *ChainBuilder) {
    builder.RegisterWantType("flight", func(metadata Metadata, spec WantSpec) interface{} {
        want := &Want{}
        want.Init(metadata, spec, func(m Metadata, s WantSpec) WantLocals {
            return &FlightWantLocals{
                FlightType:        "economy",
                Duration:          12 * time.Hour,
                DepartureDate:     "2024-01-01",
                monitoringActive:  false,
                monitoringDuration: 30 * time.Second,
            }
        })
        return want
    })
}
```

### より良い実装: Locals生成関数を外部化

```go
// flight_types.go
func NewFlightWantLocals() *FlightWantLocals {
    return &FlightWantLocals{
        FlightType:        "economy",
        Duration:          12 * time.Hour,
        DepartureDate:     "2024-01-01",
        monitoringActive:  false,
        monitoringDuration: 30 * time.Second,
    }
}

// chain_builder.go内
func RegisterFlightWantTypes(builder *ChainBuilder) {
    builder.RegisterWantType("flight", func(metadata Metadata, spec WantSpec) interface{} {
        want := &Want{}
        want.Init(metadata, spec, func(m Metadata, s WantSpec) WantLocals {
            return NewFlightWantLocals()
        })
        return want
    })
}
```

## 実装段階

### Phase 1: Foundation (2-3日)

1. Want.Localsインターフェースを定義（want.go）
2. WantLocalsインターフェースの仕様を決定
3. InitLocalsメソッドの共通パターンを定義

### Phase 2: Flight型の試験的リファクタリング (1-2日)

1. FlightWantLocals構造体を定義（flight_types.go）
2. InitLocals、GetConnectivityMetadata等を実装
3. NewFlightWant関数をInit()ベースに変更
4. Exec()メソッドでFlightWantLocals（w.Locals.(*FlightWantLocals)）にアクセス

### Phase 3: 他のWant型への適用 (3-5日)

1. RestaurantWantLocals等を同じパターンで実装
2. 各Exec()メソッドでLocalsフィールドにアクセスするよう更新
3. テスト実行と検証

### Phase 4: Want.Initの統一 (1日)

1. Want.InitメソッドをlocalsFactory対応に変更
2. 既存のコンストラクタ関数を新スタイルに統一

### Phase 5: テストと最適化 (2-3日)

1. 全テスト実行
2. パフォーマンス確認
3. ドキュメント更新

## メリット

1. **コード削減**: 各Want型のコンストラクタの重複コードを排除
2. **拡張性向上**: 新しいWant型追加時のボイラープレートコード削減
3. **メンテナンス性**: 共通ロジック（Init、コネクティビティメタデータ設定）が1箇所に集約
4. **型安全性**: Localsインターフェースにより、各型固有のデータにアクセスする際も型チェック可能

## デメリット・リスク

1. **型アサーション**: Exec()でLocalsにアクセスする際、`w.Locals.(*FlightWantLocals)`のように型アサーションが必要
   - ソリューション: ヘルパーメソッドを提供: `flight := w.GetLocals(FlightWantLocals{})`

2. **移行期間**: すべてのWant型を同時に変更する必要がある
   - ソリューション: 段階的に1つずつ移行

3. **リフレクション使用**: 型アサーション回避のためにリフレクションを使うと性能低下
   - ソリューション: パフォーマンスベンチマークを実施

## 実装上の注意点

### 型アサーション の簡潔化

```go
// 現在（冗長）
func (f *FlightWant) Exec() bool {
    flightWant := f.Locals.(*FlightWantLocals)
    flightWant.Duration // アクセス
}

// 改善案: ヘルパーメソッド
func (w *Want) GetLocals() interface{} {
    return w.Locals
}

// または、より型安全に:
func GetFlightLocals(w *Want) (*FlightWantLocals, bool) {
    locals, ok := w.Locals.(*FlightWantLocals)
    return locals, ok
}
```

### パラメータ初期化の共通化

InitLocalsメソッドで各型が独自の初期化をするため、以下のヘルパーが有用:

```go
// agent_types.go内に追加
func (w *Want) GetParamString(key string, defaultValue string) string {
    if w.Spec.Params != nil {
        if v, ok := w.Spec.Params[key].(string); ok {
            return v
        }
    }
    return defaultValue
}

// flight_types.go内
func (f *FlightWantLocals) InitLocals(spec WantSpec, want *Want) {
    f.FlightType = want.GetParamString("flight_type", "economy")
    // ...
}
```

## 予想される コード削減量

- **前**: 70個のNewXxxWant関数 × 平均15行 = 1,050行
- **後**: 1つのNew関数 + 各Locals実装（実質フィールド定義のみ）
- **削減**: 約700-800行のコンストラクタボイラープレート

## 後方互換性

- **破壊的変更**: Exec()メソッドの実装方式が変更される
- **移行戦略**:
  1. 既存NewXxxWant関数をDeprecatedマークして、廃止予定を通知
  2. 新規コードはLocalsベースで記述
  3. 段階的に既存Exec()メソッドを更新

## まとめ

このリファクタリングにより、MyWantフレームワークはより統一された、拡張性の高いWant型システムに進化します。コンストラクタの重複を排除しながら、各Want型固有のデータを適切に管理できる設計です。
