# MyWant GUI コンセプト

## 設計思想

### 1. あらゆる操作をキーボード・ゲームパッドで完結できる

テキスト入力中を除き、すべての操作はキーボードショートカットまたはゲームパッドで完結できる。マウス操作は必須ではない。

- モーダル・オーバーレイ・確認ダイアログは必ず Escape / B ボタンで閉じられる
- フォーカスが当たっている要素は Enter / A ボタンで実行できる
- 選択肢がある画面では十字キー / D-pad で選択肢を移動できる

### 2. キーボードとゲームパッドの操作は一対一で対応している

| キーボード | ゲームパッド | 意味 |
| :--- | :--- | :--- |
| Arrow keys | D-pad | 移動・ナビゲーション |
| Enter | A ボタン（短押し） | 決定・実行 |
| Escape | B ボタン | キャンセル・閉じる |
| Shift + Arrow | D-pad（A 長押し中） | ドラッグ移動 |
| A 長押し（500ms） | A 長押し（500ms） | 特殊モード開始（ドラッグ等） |
| Tab / Shift+Tab | — | フォーム内の項目移動 |
| 左右矢印 | D-pad 左右 | 二択選択（Yes/No など）の切り替え |

### 3. 現在地が一目でわかる視覚表現

**デフォルト（未フォーカス）** → モノトーン（グレー系）で控えめに表示

**フォーカス中** → 背景色が薄い青色に変化 ＋ sky-400 の枠線（3px）が表示される

- フォーカスリングの色は画面全体で **sky-400（薄水色）** に統一している
- 枠線の太さは 3px。視認性のため `outline` を使用（`box-shadow` は `overflow: hidden` でクリップされるため不適）
- 選択肢グリッドで現在フォーカスがある要素だけ `ring-2 ring-sky-400` が表示される
- セクションヘッダーボタン・パラメータエリアは **デフォルトがグレー**、フォーカス時のみ **blue-100 系** に変化する
- Add ボタン（追加確定）は **デフォルトがダークグレー**、フォーカス時のみ **blue-950（暗い紺）** に変化する

---

## 画面別操作ガイド

### ダッシュボード（カード一覧）

| 操作 | キーボード | ゲームパッド |
| :--- | :--- | :--- |
| Want カードにフォーカス | Arrow keys | D-pad |
| クイックアクション表示 | Enter | A ボタン（短押し） |
| カードドラッグモード開始 | Shift 押し続ける | A 長押し（500ms） |
| ドラッグ中の移動 | Arrow keys（Shift 保持） | D-pad（A 保持） |
| ドラッグ確定 | Shift 離す | A ボタン離す |
| ドラッグキャンセル | Escape | B ボタン |
| Add Want フォーム開く | `a` キー | — |
| サマリーサイドバー開閉 | `s` キー | — |
| セレクトモード切替 | Shift + S | — |
| 全選択（セレクトモード中） | Ctrl/Cmd + A | — |

**ドラッグモード中の視覚表現:**
- ドラッグ中のカードは `opacity-40` になる
- ドロップ可能なセル: 青いオーバーレイ
- ドロップ不可なセル（競合）: 赤いオーバーレイ
- フィールドマッチ推奨が存在するセル: 黄色いヒントバッジ

---

### クイックアクションオーバーレイ（カード上）

Want カードにフォーカス中に Enter / A を押すと 3×2 のグリッドが表示される。

| 配置 | アクション |
| :--- | :--- |
| [Start/Stop] [Restart] [Edit] | 左上 / 中上 / 右上 |
| [Suspend/Resume] [Close] [Delete] | 左下 / 中下 / 右下 |

操作:
- Arrow keys / D-pad: グリッド内を移動（sky-400 枠線で現在位置を表示）
- Enter / A: フォーカス中のアクションを実行
- Escape / B: オーバーレイを閉じる

---

### Add Want サイドバー

#### フォーム Tab 順序（4要素サイクル）

```
Change（Want type 再選択） → Parameters（セクションヘッダー） → Want Name → Add ボタン → (先頭に戻る)
```

- Shift+Tab で逆順移動
- Tab フォーカスはサイドバー内に閉じ込められており、ヘッダーメニューなど外部にはフォーカスが出ない
- FormYamlToggle・Filter ボタン等のユーティリティは `tabIndex={-1}` でサイクルから除外

#### Want Type 選択フェーズ（インベントリピッカー）

- Arrow keys / D-pad: カテゴリ順・名前順のグリッドナビゲーション
- Enter / A: フォーカス中の Want type を選択
- `/` キー: 検索ボックスにフォーカス

#### パラメータ入力フェーズ

- Tab: セクションヘッダーで「Parameters → Name」にジャンプ（内部フィールドはスキップ）
- Arrow Up/Down / D-pad 上下: セクション間を移動
- ArrowRight: パラメータセクションを展開して最初のフィールドにフォーカス
- ArrowLeft: パラメータセクションを折りたたむ
- パラメータ内部フィールドの移動は上下キーで行う（Tab は使わない）

#### Add ボタン

- 色: デフォルトはダークグレー（`bg-gray-700`）— 未フォーカス時は目立たせない
- フォーカス時: 暗い紺（`bg-blue-950`）に変化 ＋ 3px の sky-400 アウトライン
- Want type 未選択時: グレーアウト（disabled）

#### Advanced セクション（折りたたみ）

以下は Advanced セクション内に格納される:
- Scheduling（実行スケジュール）
- Labels（ラベル）
- Dependencies（依存関係）
- Owner（オーナー Want の参照）
- Load Example（サンプルデータ読み込み）

---

### 削除確認ダイアログ（ConfirmationBubble）

| 操作 | キーボード | ゲームパッド |
| :--- | :--- | :--- |
| No（キャンセル）にフォーカス | ← キー | D-pad 左 |
| Yes（確認）にフォーカス | → キー | D-pad 右 |
| フォーカス中の選択を実行 | Enter | A ボタン |
| キャンセル | Escape / N キー | B ボタン |
| 確認 | Y キー | — |

- 初期フォーカスは **No（キャンセル）** 側に置く（誤操作防止）
- フォーカス中のボタンに sky-400 の枠線（3px）

---

### フィールドマッチ推奨（FieldMatchBubble）

カード間の出力→入力の接続推奨が表示されるオーバーレイ。

- Arrow Up/Down / D-pad 上下: 推奨候補（Apply ボタン）間を移動
- Enter / A: フォーカス中の Apply を実行
- Escape / B: オーバーレイを閉じる
- フォーカス中の行は `bg-blue-50 ring-1 ring-blue-400/50` でハイライト

---

## カラーパレット（フォーカス・選択表現）

| 用途 | デフォルト | フォーカス時 |
| :--- | :--- | :--- |
| フォーカスリング（全体統一） | — | sky-400 (#38bdf8)、3px `outline` |
| セクションヘッダーボタン | `bg-gray-100 dark:bg-gray-800` | `bg-blue-100 dark:bg-blue-900/25` |
| Want Name 入力エリア | `bg-gray-100 dark:bg-gray-800` | テキスト入力は `bg-blue-100 dark:bg-blue-900/30`（グローバル CSS） |
| Add ボタン（追加確定） | `bg-gray-700` | `bg-blue-950`（暗い紺） |
| 編集確定ボタン（Update） | `bg-indigo-600/90` | — |
| Deploy ボタン | `bg-purple-600/90` | — |
| 選択中の行ハイライト | — | `bg-blue-50 ring-1 ring-blue-400/50` |
| 危険操作ボタン（Delete） | `bg-rose-700/90` | — |
| 停止ボタン | `bg-red-600/90` | — |
| 開始ボタン | `bg-green-600/90` | — |
| サスペンドボタン | `bg-amber-500/90` | — |
| 非活性状態 | `bg-gray-400/30 grayscale opacity-50` | — |

---

## 排他的入力キャプチャ（captureInput モード）

モーダル的なオーバーレイ（クイックアクション、確認ダイアログ、FieldMatchBubble、カンバスドラッグ中）では `captureInput: true` モードになる。このモード中は:

- ゲームパッドのすべての入力がオーバーレイに集中する（背面の操作をブロック）
- Escape / B で必ずそのレイヤーを閉じられる
- 閉じることで元のコンテキストに戻る

captureInput の重ね順は後から開いたものが優先される。

---

## 実装上の注意点

### フォーカス時の背景色変化（`.sidebar-section-btn` / `.sidebar-add-btn`）

セクションヘッダーボタンと Add ボタンは、**デフォルトをモノトーン（グレー）にしてフォーカス時のみ青色に変化させる**ため、専用 CSS クラスを使用している。

- `.sidebar-section-btn` — セクションヘッダーボタン（Parameters・Labels・Dependencies・Scheduling）に付与。`:focus` / `:focus-visible` で `background-color: blue-100` を `!important` で上書き。
- `.sidebar-add-btn` — Add ボタン（create モード）に付与。フォーカス時 `background-color: blue-950` を適用。

Tailwind の `focus:bg-*` 修飾子ではなく CSS クラスで管理する理由: `button:focus-visible { outline: none !important }` のグローバルリセット規則が先に適用されるため、`@layer base` 内で `!important` 付きのクラスを定義することでオーバーライドしている（specificity の問題を `!important` で解決）。

### `overflow: hidden` とフォーカスリング

`box-shadow` ベースの `ring-*` はコンテナの `overflow: hidden` にクリップされる。サイドバーのヘッダーボタンなど `overflow: hidden` 内にあるボタンには `outline` を使用し、`.sidebar-focus-ring` CSS クラスで `!important` を付与して `button:focus-visible { outline: none !important }` のグローバルリセットをオーバーライドする。

### フォーカストラップ

RightSidebar は `document.addEventListener('keydown', trap)` でフォーカストラップを実装。ただし `e.defaultPrevented` が true（コンポーネント側で既に処理済み）の場合はトラップをスキップし、二重処理を防ぐ。

### deferred confirm（A ボタンの遅延確認）

A ボタンは**押した瞬間ではなく離した瞬間**に `confirm` イベントを発火する。これにより「A 長押し + D-pad」のコード操作（ドラッグ移動等）が A の即時実行で中断されるバグを防ぐ。

- 短押し（< 500ms）→ 離したとき `confirm` 発火
- 長押し（≥ 500ms）→ タイマー発火時に `confirm-long` 発火、離しても `confirm` は発火しない
