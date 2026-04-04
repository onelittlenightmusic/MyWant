# Goal Thinker 進化設計ドキュメント (Capability-Discovery Planning)

## 概要

ユーザーの「やりたいこと（Goal / Whim）」を、システムの現在の能力（Capability）と実績（Achievement）に基づいて、具体的で実行可能な子 Want へ動的に分解・増殖させていく自律エージェントの設計。

従来の `goal_thinker` が単なる「テキスト分割」だったのに対し、本設計では **「今何ができるか」を自ら発見し、手札を組み合わせてプランを立てる脳** へと進化させる。

---

## アーキテクチャ

### 構成要素

```
goal Want（yaml/want_types/system/goal.yaml）
  └─ goal_thinker エージェント（engine/types/agent_goal_thinker.go）
       ↓ 1. 能力の問い合わせ（Discovery）
       MyWant MCP Server（mcp/server/main.go）
            ├─ list_want_types
            ├─ list_capabilities
            └─ list_achievements
       ↓ 2. プランニング依頼（コンテキスト付き）
       tools/goal_thinker.py（LLM 呼び出し）
```

---

## 拡張されたステートマシン

従来の `decomposing` フェーズの前に、能力を把握するステップを追加する。

1.  **analyzing (Discovery):** 
    *   MCP サーバーを叩き、現在利用可能な Want 型、Capability、解除済み Achievement を取得。
    *   ユーザーの `goal_text` と合わせて LLM に送る。
2.  **questioning (Optional):** 
    *   LLM が「具体化のために情報が足りない」と判断した場合、ユーザーに質問を提示。
3.  **decomposing (Planning):** 
    *   現在の手札で実行可能な子 Want（`knowledge`, `reminder`, `visa_application` 等）のツリーを生成。
4.  **monitoring & evolution:** 
    *   子 Want の完了や、**新しい Achievement の解除** を検知した瞬間に、自動で `re_planning` をトリガーし、ツリーをさらに深く伸ばす。

---

## LLM への入力（Context）の拡張

`goal_thinker.py` に渡される JSON に、MCP から取得した「システムの手札」を統合する。

```json
{
  "phase": "plan",
  "goal_text": "家族でシリコンバレーに移住したい",
  "capabilities": {
    "available_wants": ["knowledge", "reminder", "itinerary", "visa_application"],
    "unlocked_achievements": ["passport_ready"],
    "active_capabilities": ["flight_api_access"]
  },
  "history": [...]
}
```

---

## 実装ロードマップ

### Step 1: MyWant MCP Server の拡張（完了）
*   `list_achievements`, `list_capabilities` ツールの追加。

### Step 2: `goal_thinker` エージェントの改造 (`engine/types/agent_goal_thinker.go`)
*   `goalThinkerDecompose` 関数内で MCP サーバーを呼び出し、コンテキストを取得するロジックを追加。
*   `monitoring` フェーズで、「Achievement の増分」を監視対象に加える。

### Step 3: `goal_thinker.py` のプロンプト更新
*   「渡された能力リストの中から、最適な Want 型を選択してプランを立てよ」という命令を追加。
*   実績が足りない場合は、「その実績を解除するための調査（Knowledge）」を先行させるロジック。

### Step 4: ライフサイクルとの統合
*   子 Want が完了（Achieved）した際に、親の `goal_thinker` が即座に反応して「次のステップ」を生み出す増殖サイクルの確立。

---

## 期待される挙動（whim.md シナリオの実現）

1.  **最初:** システムには `knowledge` しかない。Thinker は「ビザの調査」を生成。
2.  **中盤:** 調査完了により `visa_expert` 実績が解除。
3.  **増殖:** Thinker が MCP で `visa_expert` を発見。自動的に「ビザ申請書作成」という具体的タスクを親 Goal の下に生やす。
4.  **終盤:** 全てが連鎖し、トップダウンで描いた「シリコンバレー移住」が具体的な実行ログの集積として完遂される。

---

## 関連ドキュメント

- [Whim ユーザーストーリー](whim.md)
- [Want System](want-system.md)
- [Agent System](agent-system.md)
- [YAML Scriptable Want Type](yaml-scriptable-want-type.md)
