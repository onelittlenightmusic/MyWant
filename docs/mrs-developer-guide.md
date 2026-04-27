# MRS (Machine-Readable Skill) 開発ガイド

このドキュメントは、MyWant のスキルプラグイン（MRS: Machine-Readable Skill）を開発し、それを実行するための Want Type を自動生成するための完全なガイドです。

---

## 1. MRS とは

MRS (Machine-Readable Skill) は、Python や Shell などの外部スクリプトを MyWant のエコシステムに統合するための仕組みです。`SKILL.md` という標準化された Markdown ファイルに仕様を記述することで、MyWant が解釈可能な **Want Type YAML を完全に機械的に生成**できます。

### 全体フロー

```
[ スキル開発 ]
SKILL.md + main.py (スクリプト本体)
        ↓
[ Want Type 生成 ] (mywant types generate --from-skill)
want_type_YAML (GPCルール適用済み)
        ↓
[ Want 実行 ]
DoMRS / MonitorMRS エージェントがスクリプトを実行
```

---

## 2. SKILL.md 記述ルール

`SKILL.md` は、スキルの入力、出力、実行特性を定義します。

### セクション構造

```markdown
## 実行特性       (必須) -> ロールとエージェントの決定
## パラメータ     (必須) -> 入力仕様テーブル
## 出力フィールド (必須) -> 出力仕様テーブル (JSONパス付き)
## 使用例         (推奨) -> テストおよび examples 生成
## エラー         (任意)
```

### 2.1 実行特性

スキルの実行モデル（フォアグラウンド/バックグラウンド）を定義します。

| 実行モデル | 意味 | child-role | 使用エージェント |
|:---|:---|:---|:---|
| `foreground` | 1回実行して完了する (同期) | `doer` | `do_mrs_agent` |
| `background` | 定期的に監視を続ける (非同期) | `monitor` | `monitor_mrs_agent` |

### 2.2 パラメータ

入力パラメータの仕様をテーブル形式で記述します。

| フィールド | 型 | 必須 | デフォルト | 説明 |
|:---|:---|:---:|:---|:---|
| `room` | string | ✓ | — | 部屋名 |
| `date` | string | ✓ | — | 日付（YYYY-MM-DD） |
| `time` | string | — | (グローバルパラメータ: selected_slot) | 時刻 |

**型マッピング**: `string`, `int`, `float`, `bool`, `object`, `array`

**グローバルパラメータ連携**:
- `(グローバルパラメータ: key)` : 直接指定
- `(グローバルパラメータ from: key_param)` : パラメータ経由の間接指定

### 2.3 出力フィールド

スクリプトが出力する JSON の各フィールドを定義します。

| フィールド名 | 型 | JSONパス | 永続化 | 説明 |
|:---|:---|:---|:---:|:---|
| `reservation_id` | string | `confirmation.id` | true | 予約ID |
| `status` | string | `status` | true | 内部ステータス |

- **JSONパス**: 出力 JSON 内のドット区切りパス。
- **永続化**: `true` に設定すると、取得した値が Want の State に保持されます。

---

## 3. 自動生成と変換ルール

自動生成ツールを実行すると、`SKILL.md` に基づいた Want Type YAML が `yaml/want_types/` に生成されます。

### 生成コマンド

```bash
./bin/mywant types generate --from-skill <plugin-dir>/SKILL.md
```

### 生成される State の 4層構造

生成された YAML には、GPC ルールに基づいた以下の State が定義されます。

1. **MRS 実行スキャフォールド (固定)**: `skill_path`, `skill_json_arg`, `mrs_raw_output` など。
2. **パラメータミラー**: 入力パラメータと同期する State。
3. **実行ステータス (固定)**: `status`, `error`, `summary`。
4. **出力フィールド**: `SKILL.md` の「出力フィールド」表から派生。

---

## 4. GPC ルールとガバナンス

MRS から生成された Want Type は、MyWant の **GPC (Goal -> Plan -> Current)** 統治モデルに自動的に適合します。

### ラベルの割り当て

- **Goal**: `skill_timeout_seconds` などの設定値。
- **Current**: 実行結果、パラメータミラー、ステータス、生出力など（エージェントが書き込む事実）。
- **Plan**: スケジュールや計画（Thinker が書き込む指示）。

### 子ロール (child-role) と親への書き込み

MRS Want が Coordinator (親) の子として動作する場合、親の State への書き込み権限は `child-role` によって制御されます。

| child-role | 書込可能な親のラベル | 生成条件 |
|:---|:---:|:---|
| `doer` | `current` | 実行モデル: `foreground` |
| `monitor` | `current` | 実行モデル: `background` |

---

## 5. スクリプトの I/O コントラクト

実行スクリプト（`main.py` 等）は、以下の規約に従う必要があります。

### 入力
引数として単一の JSON 文字列を受け取ります。
```bash
python3 main.py '{"room": "Room01", "date": "2026-04-12"}'
```

### 出力
標準出力 (stdout) に JSON を出力します。
```json
{
  "status": "success",
  "confirmation": {
    "id": "RES-123",
    "datetime": "2026-04-12 10:00"
  }
}
```

---

## 6. チェックリスト

- [ ] `## 実行特性` セクションで `実行モデル` を指定したか
- [ ] パラメータの `型` と `必須` チェックは正しいか
- [ ] `JSONパス` はスクリプトの出力構造と一致しているか
- [ ] `final-result-field` は適切か（省略時は最初の永続化フィールド）
- [ ] `## 使用例` に入出力のサンプルを含めたか
