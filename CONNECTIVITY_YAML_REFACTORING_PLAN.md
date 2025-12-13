# ConnectivityMetadata YAML定義・登録システム リファクタリング計画

## 目的

現在、`ConnectivityMetadata`（入出力の数、説明など）がGoコード内の`NewWantWithLocals`呼び出しで直接定義されています。これをYAML仕様ファイルで定義し、Want登録時に自動的に設定される仕組みを構築します。

**利点:**
- 設定をYAMLで一元管理（DRY原則）
- Goコードがシンプルになる
- Want仕様をコード以外で変更可能
- 自動バリデーション

## 現状

### 例：CoordinatorWantの場合

`engine/cmd/types/coordinator_types.go`:
```go
func NewCoordinatorWant(metadata Metadata, spec WantSpec) Executable {
    want := NewWantWithLocals(
        metadata,
        spec,
        nil,
        ConnectivityMetadata{
            RequiredInputs:  -1,
            RequiredOutputs: 0,
            MaxInputs:       -1,
            MaxOutputs:      0,
            WantType:        coordinatorType,
            Description:     fmt.Sprintf("Generic coordinator want (%s)", coordinatorType),
        },
        coordinatorType,
    )
    // ...
}
```

### 対象ファイル

すべての`engine/cmd/types/*_types.go`ファイルで同様の処理が存在：
- `evidence_types.go` → `EvidenceWant`
- `description_types.go` → `DescriptionWant`
- `coordinator_types.go` → `CoordinatorWant`
- `travel_types.go` → `RestaurantWant`, `HotelWant`, `BuffetWant`, `TravelCoordinatorWant`
- その他のWant実装

## 設計

### 1. YAML仕様構造の拡張

各Want型のYAMLファイルに`usageLimit`セクションを追加：

`want_types/coordinators/coordinator.yaml`:
```yaml
wantType:
  metadata:
    name: "coordinator"
    # ... 既存フィールド

  usageLimit:                      # ← 新規追加（providers/usersで入出力を区別）
    providers:                      # 入力接続の制限
      min: -1                       # 最小入力数（-1=チェックなし）
      max: -1                       # 最大入力数（-1=無制限）
    users:                          # 出力接続の制限
      min: 0
      max: -1
    description: "Generic coordinator want"

  parameters:
    # ... 既存パラメータ

  state:
    # ... 既存ステート定義
```

**デフォルト値:**
- 未指定フィールドは`0`に設定（チェックなし）
- 必要な制限だけYAMLで記載

```yaml
# 例1: ジェネレータ（入力不要）
usageLimit:
  providers:
    min: 0          # 入力不要

# 例2: 計算ロジック（入力1つ必須、出力1つ）
usageLimit:
  providers:
    min: 1
    max: 1
  users:
    min: 1
    max: 1
```

### 2. YAMLロード時の処理

新しいYAML構造に対応する`UsageLimitSpec`型を作成：

```go
type UsageLimitSpec struct {
    Providers struct {
        Min int `json:"min" yaml:"min"`
        Max int `json:"max" yaml:"max"`
    } `json:"providers" yaml:"providers"`
    Users struct {
        Min int `json:"min" yaml:"min"`
        Max int `json:"max" yaml:"max"`
    } `json:"users" yaml:"users"`
    Description string `json:"description" yaml:"description"`
}

// ConnectivityMetadataに変換
func (u *UsageLimitSpec) ToConnectivityMetadata(wantType string) ConnectivityMetadata {
    return ConnectivityMetadata{
        RequiredInputs:  u.Providers.Min,
        MaxInputs:       u.Providers.Max,
        RequiredOutputs: u.Users.Min,
        MaxOutputs:      u.Users.Max,
        WantType:        wantType,
        Description:     u.Description,
    }
}

type WantTypeDefinition struct {
    Metadata   WantMetadata     `json:"metadata" yaml:"metadata"`
    UsageLimit *UsageLimitSpec  `json:"usageLimit" yaml:"usageLimit"`
    Parameters []ParamDef       `json:"parameters" yaml:"parameters"`
    State      []StateDef       `json:"state" yaml:"state"`
    // ...
}
```

### 3. Want登録メカニズムの改善

#### 3.1 `ChainBuilder.RegisterWantType()`の拡張

現在:
```go
func (cb *ChainBuilder) RegisterWantType(wantType string, factory WantFactory) {
    cb.wantFactories[wantType] = factory
}
```

改善後:
```go
type WantTypeRegistry struct {
    Factory           WantFactory
    Connectivity      *ConnectivityMetadata
    YAMLSpec          *WantTypeDefinition
}

func (cb *ChainBuilder) RegisterWantType(wantType string, factory WantFactory, connectivity *ConnectivityMetadata) {
    cb.wantRegistries[wantType] = &WantTypeRegistry{
        Factory:      factory,
        Connectivity: connectivity,
    }
}

func (cb *ChainBuilder) RegisterWantTypeFromYAML(wantType string, factory WantFactory, yamlDefPath string) error {
    // YAMLファイルをロード
    // ConnectivitySpecをConnectivityMetadataに変換
    // レジストリに登録
}
```

#### 3.2 NewWantWithLocalsの改善

現在:
```go
func NewWantWithLocals(
    metadata Metadata,
    spec WantSpec,
    locals WantLocals,
    connectivityMeta ConnectivityMetadata,
    wantType string,
) *Want
```

改善後:
```go
func NewWantWithLocals(
    metadata Metadata,
    spec WantSpec,
    locals WantLocals,
    connectivityMeta *ConnectivityMetadata, // nil可能
    wantType string,
) *Want {
    want := &Want{
        Metadata: metadata,
        Spec:     spec,
    }
    want.Init()

    if locals != nil {
        want.Locals = locals
    }

    want.WantType = wantType

    // connectivityMetaがnilの場合は、登録時に設定されるべき
    if connectivityMeta != nil {
        want.ConnectivityMetadata = *connectivityMeta
    }

    return want
}
```

#### 3.3 Want生成時のメタデータ適用

`ChainBuilder.createWant()`またはFactory関数呼び出し時に、登録済みの`ConnectivityMetadata`を適用：

```go
func (cb *ChainBuilder) createWant(metadata Metadata, spec WantSpec) (Executable, error) {
    factory, exists := cb.wantRegistries[metadata.Type]
    if !exists {
        return nil, fmt.Errorf("want type %s not registered", metadata.Type)
    }

    // Factory関数を呼び出す
    executable := factory.Factory(metadata, spec)

    // ConnectivityMetadataをWantに適用
    if want, ok := executable.(*Want); ok && factory.Connectivity != nil {
        want.ConnectivityMetadata = *factory.Connectivity
    }

    return executable, nil
}
```

### 4. Goコードの簡素化

改善後、各Factory関数は`ConnectivityMetadata`を指定する必要がなくなります：

**Before:**
```go
func NewCoordinatorWant(metadata Metadata, spec WantSpec) Executable {
    want := NewWantWithLocals(
        metadata,
        spec,
        nil,
        ConnectivityMetadata{
            RequiredInputs:  -1,
            RequiredOutputs: 0,
            MaxInputs:       -1,
            MaxOutputs:      0,
            WantType:        metadata.Type,
            Description:     "Generic coordinator want",
        },
        metadata.Type,
    )
    // ... 処理
    return &CoordinatorWant{ Want: *want }
}
```

**After:**
```go
func NewCoordinatorWant(metadata Metadata, spec WantSpec) Executable {
    want := NewWantWithLocals(
        metadata,
        spec,
        nil,
        nil, // ConnectivityMetadataはYAMLから適用
        metadata.Type,
    )
    // ... 処理
    return &CoordinatorWant{ Want: *want }
}
```

登録時:
```go
func RegisterCoordinatorWantTypes(builder *ChainBuilder) {
    builder.RegisterWantTypeFromYAML(
        "coordinator",
        NewCoordinatorWant,
        "want_types/coordinators/coordinator.yaml",
    )
}
```

## 実装ステップ

### Phase 1: 基盤整備

1. **Step 1.1**: `ConnectivitySpec`型をcore systemに定義
   - Location: `engine/src/declarative.go`
   - File: Define new struct alongside `ConnectivityMetadata`

2. **Step 1.2**: `WantTypeRegistry`構造体を定義
   - Location: `engine/src/chain_builder.go`
   - Holds Factory + ConnectivityMetadata + YAMLSpec

3. **Step 1.3**: `ChainBuilder.wantFactories`を`wantRegistries`に置き換える準備
   - Backward compatibility layer: 古い`wantFactories`を保持しながら新システムに移行

### Phase 2: YAMLサポート実装

4. **Step 2.1**: YAML仕様ローダーに`usageLimit`セクション対応を追加
   - Location: `engine/src/recipe_loader_generic.go`
   - Parse `usageLimit` from YAML

5. **Step 2.2**: `UsageLimitSpec` → `ConnectivityMetadata`変換関数を実装
   - Location: `engine/src/declarative.go`
   - Implement `ToConnectivityMetadata()` method

6. **Step 2.3**: `RegisterWantTypeFromYAML()`関数を実装
   - Loads YAML file
   - Parses usageLimit spec
   - Converts to ConnectivityMetadata via `ToConnectivityMetadata()`
   - Registers in chain builder

### Phase 3: Want定義の更新

7. **Step 3.1**: 全Want型のYAMLファイルを更新
   - Add `usageLimit` section to each YAML file
   - デフォルト値（0）のフィールドは省略可能
   - Files:
     - `want_types/coordinators/coordinator.yaml`
     - `want_types/independent/evidence.yaml`
     - `want_types/independent/description.yaml`
     - All travel, queue, and other want types

8. **Step 3.2**: NewWantWithLocals()を修正
   - Make `connectivityMeta` parameter nullable/optional
   - Location: `engine/src/want.go`

### Phase 4: Factory関数の簡素化

9. **Step 4.1**: 各Factory関数から`ConnectivityMetadata`定義を削除
   - Update all `*_types.go` files
   - Remove hardcoded ConnectivityMetadata structs

10. **Step 4.2**: 各Factory関数をシンプルに統一
    - All factories now: `return &XxxWant{*NewWantWithLocals(metadata, spec, locals, nil, metadata.Type)}`

### Phase 5: 登録メカニズムの更新

11. **Step 5.1**: `RegisterXxxWantTypes()`関数を更新
    - Use new `RegisterWantTypeFromYAML()` API
    - Files:
      - `engine/cmd/types/coordinator_types.go`
      - `engine/cmd/types/travel_types.go`
      - `engine/cmd/types/*_types.go`

12. **Step 5.2**: `ChainBuilder.createWant()`を更新
    - Apply registered ConnectivityMetadata to created wants
    - Location: `engine/src/chain_builder.go`

### Phase 6: テスト・バリデーション

13. **Step 6.1**: Unit tests for new registration system
    - Test YAML loading
    - Test ConnectivityMetadata application

14. **Step 6.2**: Integration tests
    - Verify all want types work with new system
    - Test existing recipes and configs still work

## 重要な考慮事項

### Backward Compatibility

- 既存の`RegisterWantType()`は廃止せず、新しい`RegisterWantTypeFromYAML()`と並行して動作
- 段階的な移行を許可
- 古いコードと新しいコードが共存可能

### ConnectivityMetadata の適用タイミング

Want生成の流れ:
```
ChainBuilder.createWant()
  ↓
WantFactory (NewCoordinatorWant等) を呼び出し
  ↓
NewWantWithLocals() (ConnectivityMetaはnil)
  ↓
Want作成完了
  ↓
ChainBuilder.createWant() が ConnectivityMetadata を適用
```

### Type-specific Handling

一部のWant型では、`ConnectivityMetadata`が動的に決定される場合がある（例：approval levelに基づく）:
- YAMLでデフォルト値を定義
- Runtime時にパラメータに基づいて上書きする機構も必要

## ファイル一覧

### 修正対象ファイル

**Core:**
- `engine/src/declarative.go` - ConnectivitySpec型追加
- `engine/src/want.go` - NewWantWithLocals()修正
- `engine/src/chain_builder.go` - レジストリ機構実装

**Want登録:**
- `engine/cmd/types/coordinator_types.go`
- `engine/cmd/types/travel_types.go`
- `engine/cmd/types/approval_types.go`
- `engine/cmd/types/qnet_types.go`
- `engine/cmd/types/fibonacci_types.go`
- その他全Want実装ファイル

**YAML仕様:**
- `want_types/coordinators/coordinator.yaml`
- `want_types/independent/evidence.yaml`
- `want_types/independent/description.yaml`
- `want_types/travel/*.yaml`
- `want_types/queuing/*.yaml`
- その他全Want仕様ファイル

**ローダー:**
- `engine/src/recipe_loader_generic.go` - YAMLパース対応

## 推定範囲

- **Core変更**: 3ファイル × 2-3時間 = 6-9時間
- **Factory関数簡素化**: 15+ files × 30分 = 7.5時間以上
- **YAML仕様更新**: 30+ files × 15分 = 7.5時間以上
- **テスト**: 2-4時間
- **統合テスト**: 2-3時間

**合計**: 25-35時間

## 利点

1. **シングルソース・オブ・トゥルース**: Want仕様がYAMLに集約
2. **コード削減**: Factory関数がシンプルになる（~100行削減 per file）
3. **保守性向上**: 仕様変更時、YAMLのみ修正すれば十分
4. **自動バリデーション**: YAMLローダーで仕様チェック可能
5. **ドキュメンテーション**: YAMLが要件定義書になる

## リスク

1. **移行期間の複雑性**: 古いシステムと新しいシステムの並行動作
2. **YAMLパースエラーのハンドリング**: want作成失敗時の処理
3. **パフォーマンス**: YAMLロードが実行パスに入る場合の影響

## 次のステップ

ユーザーの承認後：
1. Phase 1-2を実装（基盤整備）
2. 小規模な1つのWant型（例：EvidenceWant）でパイロット実施
3. フィードバック に基づいて設計調整
4. 全Want型へ展開
