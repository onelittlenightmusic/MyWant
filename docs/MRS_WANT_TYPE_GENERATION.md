# MRS Want Type 自動生成ルール

machine-readable skill (MRS) の `SKILL.md` から want type YAML を自動生成するための仕様。

---

## 全体フロー

```
SKILL.md
  ├── frontmatter (name, description, compatibility, metadata)
  ├── パラメータ表 (入力仕様)
  ├── 出力 JSON スキーマ
  └── 使用例

        ↓ 生成ルール適用

want_type_YAML
  ├── metadata (name, title, description, pattern, labels)
  ├── parameters
  ├── state
  │     ├── [固定] MRS 実行スキャフォールド
  │     ├── [派生] 入力パラメータのミラー
  │     ├── [固定] 実行ステータス (status / error / summary)
  │     └── [派生] JSON 出力フィールド (fetchFrom)
  ├── onInitialize
  ├── requires: [do_mrs_agent]
  └── examples
```

---

## 1. SKILL.md フォーマット仕様

SKILL.md の記述ルールは **[SKILL_MD_RULE.md](SKILL_MD_RULE.md)** で定義されている。
以下はこの生成ルール文書内で参照するための要点のみ示す。

### 入力として使う SKILL.md の要素

| セクション | 機械生成に使う情報 |
|---|---|
| ディレクトリ名 or frontmatter `name` | `metadata.name` の導出、`skill_path` の導出 |
| 本文冒頭 or frontmatter `description` | `metadata.description` |
| `## 実行特性` `実行モデル` + `外部影響` | `child-role` の完全機械的決定 |
| frontmatter `metadata.type-name` | `metadata.name` (優先) |
| frontmatter `metadata.category` | `metadata.category` |
| frontmatter `metadata.final-result-field` | `finalResultField` |
| `## パラメータ` テーブル | `parameters[]` + state 層2 + `skill_json_arg` テンプレート |
| `## 出力フィールド` テーブル | state 層4 (`fetchFrom` / `onFetchData`) |
| `## 使用例` | `examples[]` |

### 型マッピング (SKILL.md → YAML)

| SKILL.md 型 | YAML type |
|---|---|
| `string` | `string` |
| `number` / `float` | `float64` |
| `integer` / `int` | `int` |
| `boolean` / `bool` | `bool` |
| `object` | `object` |
| `array` / `[]T` | `[]string` または `[]interface{}` |

---

## 2. 生成ルール

### 2.1 metadata

| 生成フィールド | ソース | ルール |
|---|---|---|
| `name` | frontmatter `name` | ハイフン区切りを除去してスラッグ化: `mywant-foo-plugin` → `foo` (最初と最後のセグメントを除く) または明示的な命名規則に従う |
| `title` | frontmatter `name` | タイトルケースで人間可読に変換 |
| `description` | frontmatter `description` | そのままコピー |
| `version` | `'1.0'` | 固定 (SKILL.md に version があればそれを使用) |
| `category` | frontmatter `metadata.category` または description から推論 | 例: "golf" → `smartgolf`、"travel" → `travel` |
| `pattern` | `independent` | MRS 型は常に independent |
| `labels.child-role` | GCP ルールで決定 (§3 参照) | 通常は省略 |

### 2.2 parameters

入力パラメータ表から1対1で生成:

```yaml
parameters:
  - name: <SKILL.md フィールド名>
    description: <SKILL.md 説明>
    type: <型マッピング>
    required: <true/false>
    # default はあれば追加
    # defaultGlobalParameterFrom は必要な場合のみ追加 (§4 参照)
```

### 2.3 state フィールド

state は **4層**で構成される。

#### 層1: MRS 実行スキャフォールド (固定・常に生成)

```yaml
state:
  - name: skill_timeout_seconds
    description: Max seconds to wait for the skill script to complete (default 120)
    type: int
    label: goal          # 設定値 → goal
    persistent: false
    initialValue: 120

  - name: skill_path
    description: Path to the MRS skill script (set by onInitialize)
    type: string
    label: current
    persistent: false
    initialValue: ""

  - name: skill_json_arg
    description: Single JSON string argument built from params via ${field} interpolation.
    type: string
    label: current
    persistent: false
    initialValue: ""

  - name: mrs_raw_output
    description: Raw JSON output from the MRS skill (source for fetchFrom fields)
    type: object
    label: current
    persistent: false
    initialValue: null
```

#### 層2: 入力パラメータのミラー (各入力パラメータから生成)

```yaml
  - name: <param_name>
    description: <param description> (copied from param)
    type: <param type>
    label: current
    persistent: false    # 再実行時にパラメータから再構築されるため false
    initialValue: <型に応じたゼロ値>
```

#### 層3: 実行ステータス (固定・常に生成)

```yaml
  - name: status
    description: Execution status (pending, done, failed)
    type: string
    label: current
    persistent: true
    initialValue: "pending"

  - name: error
    description: Error message if execution failed
    type: string
    label: current
    persistent: true
    initialValue: ""

  - name: summary
    description: Summary of execution result
    type: string
    label: current
    persistent: true
    initialValue: ""
```

#### 層4: 出力フィールド (JSON スキーマから生成)

出力 JSON の各リーフフィールドについて:

```yaml
  - name: <field_name>
    description: <SKILL.md 説明>
    type: string        # スキーマに型情報がなければ string
    label: current
    persistent: true
    initialValue: ""
    fetchFrom: mrs_raw_output
    onFetchData: "<dot.path.to.field>"  # ネストパスをドット区切りで
```

> **ルール**: `status` / `error` は層3が提供するためスキップ。入力パラメータと同名のトップレベルフィールドは**層2が担当するためスキップ**。

### 2.4 finalResultField

最終結果フィールドの選択優先順:

1. SKILL.md に `metadata.final-result-field` が明示されていればそれ
2. 出力 JSON に `summary` キーがあれば `summary`
3. 出力 JSON に `result` キーがあれば `result`
4. 最もトップレベルに近く、文字列型の出力フィールド
5. なければ `summary` (層3の固定フィールド)

### 2.5 onInitialize

```yaml
onInitialize:
  current:
    skill_path: "~/.mywant/custom-types/<plugin-dir>/main.py"
    skill_json_arg: '<入力パラメータを列挙したJSONテンプレート>'
```

`skill_json_arg` の構築ルール:
- 各入力パラメータを `"<name>": "${<name>}"` として列挙
- ネスト構造が必要な場合 (SKILL.md の入力例に基づく): ネスト構造を再現
- 例: `'{"room":"${room}","date":"${date}","time":"${time}"}'`

### 2.6 requires

`## 実行特性` の `実行モデル` によって使用エージェントが決まる:

```yaml
# 実行モデル: foreground
requires:
  - do_mrs_agent

# 実行モデル: background
requires:
  - monitor_mrs_agent
```

### 2.7 examples

SKILL.md の使用例セクションから生成:

```yaml
examples:
  - name: <使用例名>
    description: <説明>
    want:
      metadata:
        name: <name>
        type: <generated_type_name>
      spec:
        params:
          <入力パラメータを列挙>
```

---

## 3. GCP ルール (Governance / Child-role / Parent 状態管理)

want type が他の want と協調動作する場合、ガバナンスルールに従ってラベルを設定する必要がある。

### 3.1 child-role の定義と決定

child-role は「この want がどのように動作し、親に何を書き込むか」で決まる。
**実行モデル**と**親への書き込み内容**の2軸で判定する。

#### ロール定義

| child-role | 実行モデル | 親への書き込み | 典型例 |
|---|---|---|---|
| `doer` | 同期的・1回実行。トリガーされて処理し、`current` 結果を書く | `LabelCurrent` フィールド | MRS スクリプト実行、予約処理、API コール |
| `thinker` | バックグラウンド常駐。定期的に計算し、`plan` を書く | `LabelPlan` フィールド | 予算計算、スケジューリング、OPA プランナー |
| `monitor` | 非同期ポーリング。自律的に外部状態を監視し、`current` を更新 | `LabelCurrent` フィールド | 外部サービス監視、センサー読み取り、定期チェック |
| `admin` | 任意 | 全ラベル | 管理・調整用の特殊 want |
| 省略 | — | 不要 | スタンドアロン動作 (親なし) |

#### `doer` vs `monitor` の違い

```
doer:    外部から「今やれ」とトリガーされる。1回実行して結果を返す。
         例: 「この時間帯を予約して」「この URL にポストして」

monitor: 自ら定期的に動き続ける。外部状態の変化を検知して current を更新。
         例: 「フライトのステータスを5分おきに確認し続けて」
```

`do_mrs_agent` (MRS) は基本的に **doer** — スクリプトを1回実行して結果を書く。

#### 自動決定ロジック

[SKILL_MD_RULE.md](SKILL_MD_RULE.md) の `## 実行特性` セクションから機械的に決定する:

```
実行モデル: foreground → child-role: doer    / requires: do_mrs_agent
実行モデル: background → child-role: monitor / requires: monitor_mrs_agent
```

frontmatter の `lifecycle` フィールドは不要。`## 実行特性` の `実行モデル` だけで一意に決まる。

### 3.2 state label の決定

| フィールド種別 | label | 理由 |
|---|---|---|
| 設定・制限値 (skill_timeout_seconds) | `goal` | want が「目指す」ゴール設定値 |
| 実行スキャフォールド (skill_path, skill_json_arg, mrs_raw_output) | `current` | 実行中の一時状態 |
| 入力パラメータのミラー | `current` | 現在の実行コンテキスト |
| 実行結果・出力値 | `current` | エージェントが書き込む観測値 |
| ステータス・エラー | `current` | 観測された実行状態 |
| 計画・スケジュール | `plan` | thinker が書き込む計画値 |
| 内部フラグ (thinker 管理用) | `internal` | 親から隠蔽すべき内部状態 |

### 3.3 persistent の決定

| フィールド種別 | persistent | 理由 |
|---|---|---|
| skill_path, skill_json_arg | `false` | `onInitialize` で毎回再設定 |
| mrs_raw_output | `false` | エージェント実行後に再設定される一時バッファ |
| 入力パラメータのミラー | `false` | 起動時に `params` から再構築 |
| status, error | `true` | 失敗状態を再起動後も保持 |
| 出力フィールド (fetchFrom) | `true` | 確認画面のデータ等、取得後に保持 |
| summary | `true` | 最終結果は保持 |

### 3.4 親 coordinator の recipe.state に必要なラベル

MRS 型を coordinator の子として使う場合、recipe の `state` セクションに対応する `label` を定義しておくこと。

```yaml
# recipe.yaml
state:
  - name: <child_type>_final_result
    type: string
    label: current    # doer の結果を受け取る
  - name: target_budgets
    type: map
    label: plan       # thinker の計画値を受け取る
```

ガバナンスエンジンは子 want の `child-role` と親の `state label` の組み合わせで書き込みを許可/拒否する:

| child-role | 書き込み可能な親の label |
|---|---|
| `doer` | `current` |
| `thinker` | `plan` |
| `admin` | `current`, `plan`, `goal`, `internal` |
| 省略 (`unknown`) | **すべて拒否** |

---

## 4. globalParameter 連携 (オプション)

他の want (例: `choice`) が設定した global parameter をデフォルト値として使いたい場合:

### パターン A: 直接参照 (`defaultGlobalParameter`)

```yaml
parameters:
  - name: time
    type: string
    required: false
    defaultGlobalParameter: selected_slot    # グローバルパラメータ名をハードコード
```

**用途**: global parameter 名が固定で変わらない場合。

### パターン B: 間接参照 (`defaultGlobalParameterFrom`) ← 推奨

```yaml
parameters:
  - name: time
    type: string
    required: false
    defaultGlobalParameterFrom: time_global_param  # 別パラメータの値をキーとして使う

  - name: time_global_param
    description: "Global parameter key to look up for default time"
    type: string
    required: false
    default: "selected_slot"    # デフォルトは selected_slot を参照
```

**用途**: global parameter 名をユーザーが差し替えられるようにしたい場合。`time_global_param` に別のキーを渡せば任意の global parameter を参照できる。

### 解決の優先順位

1. `params` に明示的に渡された値 (最優先)
2. YAML `default` 値
3. `defaultGlobalParameter` / `defaultGlobalParameterFrom` で解決した global parameter 値 (最低優先)

---

## 5. 完全な変換例

### 入力: SKILL.md (抜粋)

```markdown
---
name: mywant-smartgolf-book-plugin
description: SmartGolf の指定部屋・日付・時間帯の予約確認画面まで進める。
metadata:
  output-format: json
---

### パラメータ

| フィールド | 型 | 必須 | 説明 |
|---|---|---|---|
| `room` | string | ✓ | 部屋名 |
| `date` | string | ✓ | 日付 (YYYY-MM-DD) |
| `time` | string |   | 時刻 (HH:MM) |

## 出力JSON形式

```json
{
  "status": "ready_to_confirm",
  "confirmation": {
    "reservation_datetime": "2026-04-12 20:00",
    "service": "中野新橋店",
    "payment": "クレジットカード"
  }
}
```
```

### 出力: want type YAML (生成結果)

```yaml
wantType:
  metadata:
    name: smartgolf_book               # "mywant-smartgolf-book-plugin" から派生
    title: SmartGolf Book
    description: |
      SmartGolf の指定部屋・日付・時間帯の予約確認画面まで進める。
    version: '1.0'
    category: smartgolf
    pattern: independent
    # child-role: 省略 (スタンドアロン動作)

  finalResultField: summary            # 出力JSONに summary なし → 固定フィールド使用

  parameters:
    # --- 入力パラメータ (SKILL.md パラメータ表から) ---
    - name: room
      description: "部屋名"
      type: string
      required: true
    - name: date
      description: "日付 (YYYY-MM-DD)"
      type: string
      required: true
    - name: time
      description: "時刻 (HH:MM)"
      type: string
      required: false
      defaultGlobalParameterFrom: time_global_param  # グローバル連携 (パターンB)
    - name: time_global_param
      description: "Global parameter key for default time"
      type: string
      required: false
      default: "selected_slot"

  state:
    # --- 層1: MRS 実行スキャフォールド (固定) ---
    - name: skill_timeout_seconds
      type: int
      label: goal
      persistent: false
      initialValue: 120

    - name: skill_path
      type: string
      label: current
      persistent: false
      initialValue: ""

    - name: skill_json_arg
      type: string
      label: current
      persistent: false
      initialValue: ""

    - name: mrs_raw_output
      type: object
      label: current
      persistent: false
      initialValue: null

    # --- 層2: 入力パラメータのミラー ---
    - name: room
      type: string
      label: current
      persistent: false
      initialValue: ""

    - name: date
      type: string
      label: current
      persistent: false
      initialValue: ""

    - name: time
      type: string
      label: current
      persistent: false
      initialValue: ""

    # --- 層3: 実行ステータス (固定) ---
    - name: status
      type: string
      label: current
      persistent: true
      initialValue: "pending"

    - name: error
      type: string
      label: current
      persistent: true
      initialValue: ""

    - name: summary
      type: string
      label: current
      persistent: true
      initialValue: ""

    # --- 層4: 出力フィールド (confirmation.* から派生) ---
    - name: reservation_datetime
      type: string
      label: current
      persistent: true
      initialValue: ""
      fetchFrom: mrs_raw_output
      onFetchData: "confirmation.reservation_datetime"

    - name: service
      type: string
      label: current
      persistent: true
      initialValue: ""
      fetchFrom: mrs_raw_output
      onFetchData: "confirmation.service"

    - name: payment
      type: string
      label: current
      persistent: true
      initialValue: ""
      fetchFrom: mrs_raw_output
      onFetchData: "confirmation.payment"

  onInitialize:
    current:
      skill_path: "~/.mywant/custom-types/mywant-smartgolf-book-plugin/main.py"
      skill_json_arg: '{"room":"${room}","date":"${date}","time":"${time}"}'

  requires:
    - do_mrs_agent

  examples:
    - name: Book SmartGolf Room
      description: 指定部屋・時間の予約確認画面へ進む
      want:
        metadata:
          name: book_room02
          type: smartgolf_book
        spec:
          params:
            room: "中野新橋店/打席予約(Room02)"
            date: "2026-04-13"
            time: "20:00"
```

---

## 6. 自動生成ツールへの入力仕様 (将来対応)

自動生成ツール (`mywant types generate --from-skill SKILL.md`) に渡すべき追加情報:

| オプション | 説明 | 例 |
|---|---|---|
| `--plugin-dir` | プラグインディレクトリ名 | `mywant-smartgolf-book-plugin` |
| `--type-name` | 生成する type 名 (省略時は frontmatter name から自動導出) | `smartgolf_book` |
| `--category` | カテゴリ (省略時は description から推論) | `smartgolf` |
| `--child-role` | child-role ラベル (省略時は §3.1 ヒューリスティックで自動判定) | `doer` |
| `--final-result-field` | finalResultField の明示指定 | `summary` |
| `--global-param-link` | `param:global_key` 形式で globalParameter 連携を指定 | `time:selected_slot` |

---

## 7. チェックリスト

YAML 生成後の確認事項:

- [ ] `pattern: independent` が設定されている
- [ ] 全 state フィールドに `label:` が設定されている
- [ ] child-role が正しく判定されている
  - `実行モデル: foreground` → `child-role: doer` + `requires: [do_mrs_agent]`
  - `実行モデル: background` → `child-role: monitor` + `requires: [monitor_mrs_agent]`
- [ ] 親コーディネーターへの書き込みが必要な場合、`labels.child-role` が設定されている
- [ ] `skill_json_arg` のテンプレートが全入力パラメータを網羅している
- [ ] `fetchFrom: mrs_raw_output` + `onFetchData` のパスが出力 JSON スキーマと一致している
- [ ] `finalResultField` に指定したフィールドが state に定義されている
- [ ] `requires` が `実行モデル` に対応している (`foreground` → `do_mrs_agent`、`background` → `monitor_mrs_agent`)
- [ ] examples が少なくとも1件含まれている
- [ ] 親 recipe を使う場合、recipe.state に対応する `label` が設定されている
