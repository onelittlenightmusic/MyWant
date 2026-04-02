# Whim Thinker 設計ドキュメント

## 概要

Whim（思いつきメモ）を受け取り、LLM との対話を通じて子 Want へ分解していく自律エージェントの設計。

ユーザーは「やりたいこと」を自然言語で書くだけでよい。Thinker が質問し、回答を受けて具体的な子 Want を自動生成する。

---

## アーキテクチャ

### Want 構成

```
whim レシピ（yaml/recipes/whim.yaml）
  ├─ whim-target Want  （custom_type、ユーザーが書いた want テキストを保持）
  └─ whim_thinker Want （新規 Want 型、LLM 対話と子 Want 追加を担う）
       ↓ requires: [whim_thinking]
       whim_think_agent（新規 ThinkAgent）
            ↓ サブプロセス呼び出し
            tools/whim_thinker.py（Anthropic SDK で LLM 呼び出し）
```

### 既存アーキテクチャとの対応

| 要素 | itinerary の例 | Whim の例 |
|---|---|---|
| Want 型 | `itinerary` | `whim_thinker`（新規） |
| requires | `opa_llm_planning` | `whim_thinking`（新規） |
| ThinkAgent | `opa_llm_thinker` | `whim_think_agent`（新規） |
| LLM 呼び出し | `opa-llm-planner` サブプロセス | `whim_thinker.py` サブプロセス |
| 子 Want 追加 | DispatchThinker 経由 | 直接 `AddChildWant` |

---

## ステートマシン

```
analyzing        ← want テキストを LLM に送り、質問を生成
    ↓
awaiting_input   ← ユーザーに質問を提示（WantStatusWaitingUserAction）
    ↓（ユーザーが回答）
decomposing      ← 回答を LLM に送り、子 Want 候補を生成
    ↓
awaiting_selection ← 候補をユーザーに提示（WantStatusWaitingUserAction）
    ↓（ユーザーが選択）
dispatching      ← 選択された候補を AddChildWant で追加
    ↓
monitoring       ← 子 Want の進捗を監視
    ↓（子 Want 完了 or 新情報）
re_analyzing     ← 次フェーズを LLM に再プラン依頼
```

---

## サブプロセス入出力仕様

`opa-llm-planner` と同じ JSON stdin/stdout 方式。`ANTHROPIC_API_KEY` を環境変数から引き継ぐ。

### Phase 1 — 質問生成（analyze）

```json
// Input（stdin）
{
  "phase": "analyze",
  "want_text": "家族でシリコンバレーに移住したい",
  "conversation_history": []
}

// Output（stdout）
{
  "phase": "questions",
  "questions": [
    {
      "id": "q1",
      "text": "仕事はどうしますか？",
      "options": ["現地就職", "リモート継続", "未定"]
    },
    {
      "id": "q2",
      "text": "子供の教育は？",
      "options": ["現地公立", "日本人学校", "インター"]
    }
  ]
}
```

### Phase 2 — 子 Want 候補生成（decompose）

```json
// Input（stdin）
{
  "phase": "decompose",
  "want_text": "家族でシリコンバレーに移住したい",
  "conversation_history": [...],
  "answers": {"q1": "現地就職", "q2": "現地公立"}
}

// Output（stdout）
{
  "phase": "breakdown",
  "breakdown": [
    {
      "name": "visa-research",
      "type": "knowledge",
      "description": "O-1A / H-1B ビザの要件調査",
      "params": {"topic": "US work visa for tech engineers"}
    },
    {
      "name": "cost-of-living",
      "type": "knowledge",
      "description": "Bay Area 4人家族の生活費調査",
      "params": {"topic": "Bay Area cost of living family of 4"}
    },
    {
      "name": "school-district",
      "type": "knowledge",
      "description": "Cupertino / Palo Alto 学区の調査",
      "params": {"topic": "Silicon Valley public school districts"}
    }
  ]
}
```

---

## 状態フィールド一覧

| キー | ラベル | 内容 |
|---|---|---|
| `want_text` | goal | 元の want パラメータ |
| `phase` | current | 現在フェーズ |
| `conversation_history` | current | LLM との会話履歴（再プラン時に文脈として渡す） |
| `pending_questions` | current | ユーザーに提示中の質問リスト |
| `user_answers` | current | ユーザーの回答 |
| `proposed_breakdown` | current | LLM が提案した子 Want 候補 |
| `selected_breakdown` | current | ユーザーが選択した候補 |
| `dispatched_children` | current | 追加済み子 Want の ID マップ |

---

## 新規ファイル一覧

| ファイル | 内容 |
|---|---|
| `engine/types/whim_types.go` | `whim_thinker` Want 型の定義 |
| `engine/types/agent_whim_thinker.go` | `whim_think_agent` ThinkAgent の実装 |
| `yaml/want_types/system/whim_thinker.yaml` | want type 定義（requires を含む） |
| `yaml/agents/agent-whim-thinker.yaml` | エージェント定義 |
| `tools/whim_thinker.py` | LLM 呼び出しサブプロセス（Anthropic SDK） |

## 変更ファイル一覧

| ファイル | 変更内容 |
|---|---|
| `yaml/recipes/whim.yaml` | `wants:` に `whim_thinker` を追加 |

---

## 実装コード骨格

### `engine/types/whim_types.go`

```go
package types

import . "mywant/engine/core"

func init() {
    RegisterWantImplementation[WhimThinkerWant, WhimThinkerLocals]("whim_thinker")
}

type WhimThinkerLocals struct {
    Phase string `mywant:"internal,phase"`
}

type WhimThinkerWant struct{ Want }

func (w *WhimThinkerWant) GetLocals() *WhimThinkerLocals {
    return CheckLocalsInitialized[WhimThinkerLocals](&w.Want)
}

func (w *WhimThinkerWant) Initialize() {
    w.SetGoal("want_text", w.GetStringParam("want", ""))
    w.SetCurrent("phase", "analyzing")
}

func (w *WhimThinkerWant) IsAchieved() bool { return false } // 常駐

func (w *WhimThinkerWant) Progress() {} // whim_think_agent が全ロジックを担う
```

### `engine/types/agent_whim_thinker.go`

```go
package types

import (
    "context"
    "encoding/json"
    "os"
    "os/exec"

    . "mywant/engine/core"
)

func init() {
    RegisterThinkAgentType("whim_think_agent", []Capability{
        {Name: "whim_thinking", Gives: []string{"whim_thinking"},
         Description: "Decomposes a whim into child wants via LLM conversation"},
    }, whimThinkFn)
}

func whimThinkFn(ctx context.Context, want *Want) error {
    phase := GetCurrent(want, "phase", "analyzing")

    switch phase {
    case "analyzing":
        input := map[string]any{
            "phase":                "analyze",
            "want_text":           GetGoal(want, "want_text", ""),
            "conversation_history": []any{},
        }
        output, err := callWhimThinkerScript(ctx, input)
        if err != nil {
            want.DirectLog("[WhimThinker] ERROR: %v", err)
            return err
        }
        want.SetCurrent("pending_questions", output["questions"])
        want.SetCurrent("phase", "awaiting_input")
        want.SetStatus(WantStatusWaitingUserAction)

    case "awaiting_input":
        answers, ok := want.GetCurrent("user_answers")
        if !ok || answers == nil {
            return nil // まだ回答がない
        }
        want.SetCurrent("phase", "decomposing")

    case "decomposing":
        input := map[string]any{
            "phase":                "decompose",
            "want_text":           GetGoal(want, "want_text", ""),
            "conversation_history": GetCurrent(want, "conversation_history", []any{}),
            "answers":             GetCurrent(want, "user_answers", map[string]any{}),
        }
        output, err := callWhimThinkerScript(ctx, input)
        if err != nil {
            want.DirectLog("[WhimThinker] ERROR: %v", err)
            return err
        }
        want.SetCurrent("proposed_breakdown", output["breakdown"])
        want.SetCurrent("phase", "awaiting_selection")
        want.SetStatus(WantStatusWaitingUserAction)

    case "awaiting_selection":
        selected, ok := want.GetCurrent("selected_breakdown")
        if !ok || selected == nil {
            return nil // まだ選択がない
        }
        want.SetCurrent("phase", "dispatching")

    case "dispatching":
        selected := GetCurrent(want, "selected_breakdown", []any{})
        items, _ := selected.([]any)
        for _, item := range items {
            child := buildChildWantFromBreakdown(item)
            if err := want.AddChildWant(child); err != nil {
                want.DirectLog("[WhimThinker] AddChildWant ERROR: %v", err)
            }
        }
        want.SetCurrent("phase", "monitoring")
        want.SetStatus(WantStatusReaching)
    }

    return nil
}

func callWhimThinkerScript(ctx context.Context, input map[string]any) (map[string]any, error) {
    inputJSON, _ := json.Marshal(input)

    cmd := exec.CommandContext(ctx, "python3", "tools/whim_thinker.py")
    cmd.Env = os.Environ() // ANTHROPIC_API_KEY を引き継ぐ
    cmd.Stdin = strings.NewReader(string(inputJSON))

    out, err := cmd.Output()
    if err != nil {
        return nil, err
    }

    var result map[string]any
    if err := json.Unmarshal(out, &result); err != nil {
        return nil, err
    }
    return result, nil
}

func buildChildWantFromBreakdown(item any) *Want {
    m, _ := item.(map[string]any)
    return &Want{
        Metadata: Metadata{
            ID:   GenerateUUID(),
            Name: m["name"].(string),
            Type: m["type"].(string),
        },
        Spec: WantSpec{
            Params: m["params"].(map[string]any),
        },
    }
}
```

### `yaml/recipes/whim.yaml`（変更後）

```yaml
recipe:
  metadata:
    name: whim
    custom_type: whim-target
    description: "Whim - 思いつきメモ。やりたいことを自然言語で書いておく場所。"
    version: "1.1.0"

  parameters:
    want: ""

  parameter_descriptions:
    want: "やりたいことの自然言語メモ"

  wants:
    - metadata:
        type: whim_thinker
        labels:
          role: thinker
      spec:
        params:
          want: want  # whim の want パラメータをそのまま渡す
```

---

## Draft システムとの再利用関係

Draft（`InteractionManager`）との比較。

| レイヤー | Draft | Whim Thinker | 再利用 |
|---|---|---|---|
| `Recommendation` 型 | ✅ | ✅ | ✅ そのまま |
| `ConversationMessage` 型 | ✅ | ✅ | ✅ そのまま |
| `InteractBubble` UI | ✅ | ✅ | ✅ そのまま |
| `DraftWantCard` UI パターン | ✅ | ✅ | ✅ 参考に |
| `InteractionManager.SendMessage()` | ✅ | ❌ | ❌ GooseExecutor 依存 |
| `generateRecommendations()` | ✅ | ❌ | ❌ サーバー側専用 |
| セッション管理（SessionCache） | ✅ | ❌ | ❌ want.state で代替 |
| デプロイ（AddWantsAsync） | ✅ | ❌ | ❌ AddChildWant で代替 |

**実質的な再利用率: 型・UI で約 30〜40%**

LLM 呼び出しは `opa_llm_thinker` と同じサブプロセス方式（`tools/whim_thinker.py`）を採用することで、サーバーとの疎結合を維持する。

---

## 実装ロードマップ

```
Step 1: whim_types.go + whim_thinker.yaml
        → whim_thinker Want が動く骨格を作る

Step 2: agent_whim_thinker.go（LLM なし・固定質問でテスト）
        → フェーズ遷移と AddChildWant が動くことを確認

Step 3: tools/whim_thinker.py + Anthropic SDK
        → 動的な質問生成・分解を LLM に委譲

Step 4: yaml/recipes/whim.yaml に whim_thinker を追加
        → Whim デプロイで Thinker が自動起動することを確認

Step 5: 再プラン（monitoring → re_analyzing）
        → 子 Want 完了後の次フェーズ提案
```

Step 2 まで完成すれば「質問に答えると子 Want が生える」体験が動作確認できる。

---

## 関連ドキュメント

- [Whim ユーザーストーリー](whim.md)
- [Want System](want-system.md)
- [Agent System](agent-system.md)
- [Want Developer Guide](WantDeveloperGuide.md)
