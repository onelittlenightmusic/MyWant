# SKILL.md 記述ルール (Machine-Readable Skill Specification)

このドキュメントは、machine-readable skill (MRS) の `SKILL.md` を記述するためのルール仕様。
このルールに従って書かれた `SKILL.md` からは、want type YAML が**完全に機械的に生成**できる。

自動生成ツール: `mywant types generate --from-skill <plugin-dir>/SKILL.md`

---

## SKILL.md の構造

```
SKILL.md
  ├── [任意] YAML frontmatter  ← 明示したい場合のみ。なくても自動推定される
  ├── [必須] ## 実行特性       ← child-role と使用エージェントを決定する
  ├── [必須] ## パラメータ     ← 入力仕様テーブル
  ├── [必須] ## 出力フィールド ← 出力仕様テーブル (JSONパス付き)
  ├── [推奨] ## 使用例
  └── [任意] ## エラー
```

frontmatter は**不要**。ドキュメント本文のセクション構造から機械的に情報を取得する。

---

## 1. `## 実行特性` セクション (必須)

child-role と使用エージェントを機械的に決定するためのセクション。
`実行モデル` の1項目のみ記述する。

### フォーマット

```markdown
## 実行特性

| 項目 | 値 | 説明 |
|---|---|---|
| 実行モデル | `foreground` | トリガーされて1回実行し完了する |
```

### `実行モデル` の値と生成ルールの対応

| 値 | 意味 | child-role | 使用エージェント | 親への書き込み先 |
|---|---|---|---|---|
| `foreground` | 外部からトリガーされ、1回実行して完了する | `doer` | `do_mrs_agent` | `label: current` |
| `background` | 自律的にバックグラウンドで繰り返し実行される | `monitor` | `monitor_mrs_agent` | `label: current` |

`実行モデル` の値だけで child-role と使用エージェントが一意に決まる。

---

## 2. `## パラメータ` セクション (必須)

入力パラメータの仕様を記述するテーブル。

### フォーマット

```markdown
## パラメータ

| フィールド | 型 | 必須 | デフォルト | 説明 |
|---|---|---|---|---|
| `room` | string | ✓ | — | 部屋名（例: 中野新橋店/打席予約(Room02)） |
| `date` | string | ✓ | — | 日付（YYYY-MM-DD形式、JST） |
| `time` | string | — | (グローバルパラメータ from: time_global_param) | 時刻（HH:MM形式、JST） |
| `time_global_param` | string | — | `selected_slot` | time のデフォルト参照先となるグローバルパラメータキー |
```

### カラム仕様

| カラム | 必須 | 内容 |
|---|---|---|
| `フィールド` | ✓ | バッククォートで囲んだパラメータ名 |
| `型` | ✓ | `string` / `int` / `float` / `bool` / `object` / `array` |
| `必須` | ✓ | `✓` = `required: true`、空または `—` = `required: false` |
| `デフォルト` | — | リテラル値、または後述のグローバルパラメータ記法 |
| `説明` | ✓ | パラメータの説明 |

### 型マッピング (SKILL.md → YAML)

| SKILL.md 型 | YAML type |
|---|---|
| `string` | `string` |
| `number` / `float` | `float64` |
| `integer` / `int` | `int` |
| `boolean` / `bool` | `bool` |
| `object` | `object` |
| `array` / `[]T` | `[]string` または `[]interface{}` |

### グローバルパラメータ連携

デフォルト欄に以下の記法を使うと `defaultGlobalParameter` / `defaultGlobalParameterFrom` が自動生成される:

| デフォルト欄の記法 | 生成されるYAMLフィールド |
|---|---|
| `(グローバルパラメータ: selected_slot)` | `defaultGlobalParameter: selected_slot` |
| `(グローバルパラメータ from: time_global_param)` | `defaultGlobalParameterFrom: time_global_param` |

---

## 3. `## 出力フィールド` セクション (必須)

MRS スクリプトが出力する JSON の各フィールドを記述するテーブル。
このテーブルから `fetchFrom` / `onFetchData` が自動生成される。

### フォーマット

```markdown
## 出力フィールド

| フィールド名 | 型 | JSONパス | 永続化 | 説明 |
|---|---|---|---|---|
| `reservation_datetime` | string | `confirmation.reservation_datetime` | true | 予約日時テキスト |
| `service`              | string | `confirmation.service`              | true | 店舗名 |
| `payment`              | string | `confirmation.payment`              | true | 支払い方法 |
```

### カラム仕様

| カラム | 必須 | 内容 |
|---|---|---|
| `フィールド名` | ✓ | バッククォートで囲んだ state フィールド名 |
| `型` | ✓ | `string` / `int` / `float` / `bool` / `object` |
| `JSONパス` | ✓ | MRS 出力 JSON のドット区切りパス → `onFetchData:` に使用 |
| `永続化` | ✓ | `true` / `false` → `persistent:` に使用 |
| `説明` | ✓ | フィールドの説明 |

### 変換ルール

各行は以下の state フィールドに変換される:

```yaml
- name: <フィールド名>
  description: <説明>
  type: <型>
  label: current
  persistent: <永続化>
  initialValue: <型のゼロ値>
  fetchFrom: mrs_raw_output
  onFetchData: "<JSONパス>"
```

**特例**: `status` / `error` は固定スキャフォールドが提供するためスキップ。

---

## 4. `## 使用例` セクション (推奨)

`examples:` セクションの want パラメータを導出するために使用する。

````markdown
## 使用例

### 基本: 日時指定で予約確認画面へ

```bash
python3 main.py '{"room": "中野新橋店/打席予約(Room02)", "date": "2026-04-13", "time": "20:00"}'
```

出力:
```json
{
  "status": "ready_to_confirm",
  "confirmation": {
    "reservation_datetime": "2026-04-13 20:00",
    "service": "中野新橋店",
    "payment": "クレジットカード"
  }
}
```
````

---

## 5. frontmatter (任意)

frontmatter は**なくてよい**。存在する場合は明示値が自動推定より優先される。

```yaml
---
name: mywant-smartgolf-book-plugin   # 省略時: ディレクトリ名から取得
description: |                        # 省略時: 本文最初の段落
  SmartGolf の指定部屋・日付・時間帯の予約確認画面まで進める。

compatibility:
  python: ">=3.10"
  requires:
    - playwright (sync_api)

metadata:
  type-name: smartgolf_book           # 省略時: name からスラッグ変換
  category: smartgolf                 # 省略時: description から推論
  final-result-field: summary         # 省略時: 出力フィールドテーブルの最初の persistent:true フィールド
---
```

### frontmatter がない場合の自動推定

| 情報 | 推定ソース |
|---|---|
| `name` | プラグインディレクトリ名 |
| `description` | SKILL.md 本文の最初の段落 |
| `metadata.type-name` | `name` のスラッグ変換 (`mywant-foo-plugin` → `foo`) |
| `metadata.category` | `name` または `description` のキーワードから推論 |
| `metadata.final-result-field` | `## 出力フィールド` テーブルの最初の `永続化: true` フィールド |

---

## 6. 完全な変換マップ

| SKILL.md の情報源 | → want type YAML | 変換ルール |
|---|---|---|
| ディレクトリ名 or frontmatter `name` | `metadata.name` | スラッグ変換。frontmatter `metadata.type-name` 優先 |
| ディレクトリ名 or frontmatter `name` | `onInitialize.current.skill_path` | `~/.mywant/custom-types/{name}/main.py` |
| 本文冒頭 or frontmatter `description` | `metadata.description` | frontmatter 優先 |
| frontmatter `metadata.category` | `metadata.category` | 省略時は推論 |
| frontmatter `metadata.final-result-field` | `finalResultField` | 省略時はテーブルから選択 |
| `## 実行特性` `実行モデル` | `metadata.labels.child-role` + `requires` | §1 の決定ロジック |
| `## パラメータ` テーブル各行 | `parameters[]` | 型・必須・デフォルト |
| `## パラメータ` テーブル各行 | `state[]` (層2: パラメータミラー) | `label: current`, `persistent: false` |
| `## パラメータ` テーブルのフィールド名一覧 | `onInitialize.current.skill_json_arg` | `'{"p1":"${p1}","p2":"${p2}",...}'` |
| `## 出力フィールド` テーブル各行 | `state[]` (層4: 出力フィールド) | `fetchFrom: mrs_raw_output` + `onFetchData` |
| — (固定生成) | `state[]` (層1: スキャフォールド) | skill_timeout_seconds, skill_path, skill_json_arg, mrs_raw_output |
| — (固定生成) | `state[]` (層3: ステータス) | status, error, summary |
| `## 実行特性` `実行モデル` | `requires` | `foreground` → `[do_mrs_agent]`、`background` → `[monitor_mrs_agent]` |
| — (固定) | `metadata.pattern` | `independent` |

---

## 7. 準拠チェックリスト

- [ ] `## 実行特性` セクションが存在する
- [ ] `実行モデル` が `foreground` / `background` のいずれかである
- [ ] `## パラメータ` セクションが存在し、全行に `フィールド` / `型` / `必須` / `説明` カラムがある
- [ ] `## 出力フィールド` セクションが存在し、全行に `フィールド名` / `型` / `JSONパス` / `永続化` / `説明` カラムがある
- [ ] `JSONパス` がドット区切りで MRS の実際の出力と一致している
- [ ] `final-result-field` に指定したフィールドが `## 出力フィールド` テーブルに存在する (省略時は自動選択)
- [ ] `## 使用例` セクションに少なくとも1件の入出力例がある

---

## 8. 準拠サンプル (smartgolf_book)

```markdown
## 実行特性

| 項目 | 値 | 説明 |
|---|---|---|
| 実行モデル | `foreground` | トリガーされて1回実行し完了する |

## パラメータ

| フィールド | 型 | 必須 | デフォルト | 説明 |
|---|---|---|---|---|
| `room` | string | ✓ | — | 部屋名（例: 中野新橋店/打席予約(Room02)） |
| `date` | string | ✓ | — | 日付（YYYY-MM-DD形式、JST） |
| `time` | string | — | (グローバルパラメータ from: time_global_param) | 時刻（HH:MM形式、JST） |
| `time_global_param` | string | — | `selected_slot` | time のデフォルト参照先となるグローバルパラメータキー |

## 出力フィールド

| フィールド名 | 型 | JSONパス | 永続化 | 説明 |
|---|---|---|---|---|
| `reservation_datetime` | string | `confirmation.reservation_datetime` | true | 予約日時テキスト |
| `service`              | string | `confirmation.service`              | true | 店舗名 |
| `payment`              | string | `confirmation.payment`              | true | 支払い方法 |

## 使用例

### 基本: 日時指定で予約確認画面へ

```bash
python3 main.py '{"room": "中野新橋店/打席予約(Room02)", "date": "2026-04-13", "time": "20:00"}'
```

出力:
```json
{
  "status": "ready_to_confirm",
  "confirmation": {
    "reservation_datetime": "2026-04-13 20:00",
    "service": "中野新橋店",
    "payment": "クレジットカード"
  }
}
```

## エラー

```json
{ "error": "Time slot not found: 2026-04-12 20:00" }
```
```
