# Teams Webhook Want Specification

## Overview

Teams Webhook Wantは、Microsoft Teams Outgoing Webhookからのメッセージを受信し、Want stateに保存するWant typeです。汎用的なWebhookエンドポイント `/api/v1/webhooks/{id}` を通じて、Teams以外のサービスからのWebhookも受信できます。

## Architecture

```
Teams Outgoing Webhook → POST /api/v1/webhooks/{want-id}
                              ↓
                         Main Server (handlers_webhook.go)
                              ↓ payload解析 + HMAC検証
                         FindWantByID → Want state更新
                              ↓
                         TeamsWebhookWant (Progress)
                           → 新着メッセージ検知 → Provide() で下流送信
                           → MonitorAgent: バッファ管理 + ヘルスチェック
```

## Lifecycle

```
┌───────────────────────────────────────────────────────────┐
│                    INITIALIZATION                          │
│  • Parse params (webhook_secret, channel_filter)          │
│  • Initialize state (status=active, messages=[], count=0) │
│  • Start background MonitorAgent (5s interval)            │
│  • Log webhook URL: POST /api/v1/webhooks/{want-id}      │
└───────────────────┬───────────────────────────────────────┘
                    ▼
┌───────────────────────────────────────────────────────────┐
│                   ACTIVE PHASE (50%)                       │
│  Webhook endpoint receives messages → state updated       │
│  Progress() detects new messages → Provide() to downstream│
│  MonitorAgent trims buffer, health checks                 │
└───────────────────┬───────────────────────────────────────┘
                    ▼
          teams_webhook_status = "stopped"
                    ▼
┌───────────────────────────────────────────────────────────┐
│                   STOPPED (100%)                           │
│  IsAchieved() = true                                      │
│  MonitorAgent stops                                       │
└───────────────────────────────────────────────────────────┘
```

## Parameters

### webhook_secret (optional)
- **Type**: `string`
- **Default**: `""`
- **Description**: Teams Outgoing Webhook設定時に取得するBase64エンコード済みシークレット。HMAC-SHA256署名検証に使用
- **Example**: `"dGVhbXMtc2VjcmV0LWtleQ=="`
- **Note**: 空の場合、署名検証はスキップされる

### channel_filter (optional)
- **Type**: `string`
- **Default**: `""`
- **Description**: 特定のTeamsチャネルIDのみ受信するフィルタ。空の場合は全チャネルを受信
- **Example**: `"19:abc123@thread.tacv2"`

## State Fields

| Field | Type | Description |
|-------|------|-------------|
| `webhook_url` | string | Webhookエンドポイント URL (`/api/v1/webhooks/{want-id}`) |
| `teams_webhook_status` | string | `active` / `stopped` / `error` |
| `webhook_secret` | string | HMAC検証用シークレット (params経由) |
| `teams_latest_message` | object | 最新の受信メッセージ |
| `teams_messages` | []object | 直近20件のメッセージバッファ (FIFO) |
| `teams_message_count` | int | 累計受信メッセージ数 |
| `channel_filter` | string | チャネルフィルタ (params経由) |
| `achieving_percentage` | int | 進捗率 (active: 50%, stopped: 100%) |

### teams_latest_message Structure

```json
{
  "sender": "User Name",
  "text": "メッセージ本文",
  "timestamp": "2026-02-08T12:00:00Z",
  "channel_id": "19:xxx@thread.tacv2"
}
```

## API Endpoints

### POST /api/v1/webhooks/{want-id}

Webhookペイロードを受信し、Want stateに保存する。

**Teams Payload** (`channelId == "msteams"` で判定):
- HMAC-SHA256署名検証 (`Authorization: HMAC <base64>`)
- 構造化パース → `teams_messages` / `teams_latest_message` / `teams_message_count` を更新
- Teams応答JSON返却

**Request (Teams形式):**
```json
{
  "type": "message",
  "text": "Hello from Teams",
  "from": {"name": "User Name"},
  "channelId": "msteams",
  "timestamp": "2026-02-08T12:00:00Z",
  "conversation": {"id": "19:xxx@thread.tacv2"}
}
```

**Response (Teams形式):**
```json
{
  "type": "message",
  "text": "Received"
}
```

**Generic Payload** (Teams以外):
- ペイロード全体を `webhook_payload` + `webhook_received_at` としてstateに保存

**Request (汎用):**
```json
{
  "event": "deployment",
  "status": "success",
  "details": "..."
}
```

**Response (汎用):**
```json
{
  "status": "received"
}
```

**Status Codes:**
- `200 OK`: ペイロード受信成功
- `400 Bad Request`: JSONパース失敗
- `401 Unauthorized`: HMAC署名検証失敗
- `404 Not Found`: Want IDが存在しない

### GET /api/v1/webhooks

Webhook受信可能なWant一覧を返却する。

**Response:**
```json
{
  "endpoints": [
    {
      "want_id": "abc-123",
      "want_name": "teams-inbox",
      "want_type": "teams webhook",
      "url": "/api/v1/webhooks/abc-123",
      "status": "active"
    }
  ],
  "count": 1
}
```

## Usage Examples

### Example 1: Basic Teams Inbox

**Scenario**: Teamsからのメッセージを受信して保存する

```yaml
wants:
  - metadata:
      name: teams-inbox
      type: teams webhook
      labels:
        category: communication
        source: teams
    spec:
      params:
        webhook_secret: ""
        channel_filter: ""
      capabilities:
        - teams_webhook_monitoring
```

**Setup:**
```bash
# 1. Start server
make restart-all

# 2. Deploy want
./mywant wants create -f yaml/config/config-teams-webhook.yaml

# 3. Get want ID
./mywant wants list

# 4. Test with curl
curl -X POST http://localhost:8080/api/v1/webhooks/{want-id} \
  -H "Content-Type: application/json" \
  -d '{
    "type": "message",
    "text": "Hello from Teams!",
    "from": {"name": "Test User"},
    "channelId": "msteams",
    "timestamp": "2026-02-08T12:00:00Z"
  }'

# 5. Verify state
./mywant wants get {want-id}
```

### Example 2: Secured Webhook with HMAC

**Scenario**: 本番環境でHMAC署名検証を有効にする

```yaml
wants:
  - metadata:
      name: secured-teams-inbox
      type: teams webhook
    spec:
      params:
        webhook_secret: "dGVhbXMtc2VjcmV0LWtleQ=="
        channel_filter: ""
      capabilities:
        - teams_webhook_monitoring
```

**HMAC付きリクエスト:**
```bash
# Generate HMAC signature
SECRET="dGVhbXMtc2VjcmV0LWtleQ=="
BODY='{"type":"message","text":"secure message","from":{"name":"User"},"channelId":"msteams"}'
SIGNATURE=$(echo -n "$BODY" | openssl dgst -sha256 -hmac "$(echo $SECRET | base64 -d)" -binary | base64)

curl -X POST http://localhost:8080/api/v1/webhooks/{want-id} \
  -H "Content-Type: application/json" \
  -H "Authorization: HMAC $SIGNATURE" \
  -d "$BODY"
```

### Example 3: Generic Webhook (non-Teams)

**Scenario**: CI/CDパイプラインの通知を受信する

```bash
# Any JSON payload (without channelId: "msteams") is stored generically
curl -X POST http://localhost:8080/api/v1/webhooks/{want-id} \
  -H "Content-Type: application/json" \
  -d '{
    "event": "build_complete",
    "project": "myapp",
    "status": "success",
    "commit": "abc123"
  }'
```

**Result state:**
```json
{
  "webhook_payload": {
    "event": "build_complete",
    "project": "myapp",
    "status": "success",
    "commit": "abc123"
  },
  "webhook_received_at": "2026-02-08T12:00:00Z"
}
```

### Example 4: Pipeline with Downstream Want

**Scenario**: Teamsメッセージを受信し、下流のWantで処理する

```yaml
wants:
  - metadata:
      name: teams-inbox
      type: teams webhook
      labels:
        role: source
    spec:
      params:
        webhook_secret: ""
      capabilities:
        - teams_webhook_monitoring

  - metadata:
      name: message-processor
      type: execution_result
      labels:
        role: processor
    spec:
      using:
        - label:
            role: source
```

Teamsメッセージ受信時、`Progress()` が `Provide()` で最新メッセージを下流に送信する。

## Teams連携の設定

MyWantのwebhookエンドポイントにTeamsメッセージを送信する方法は2つある。

### 共通: MyWant側の準備

Wantをデプロイし、Stateから `webhook_url` を取得する:

```bash
./mywant wants create -f yaml/config/config-teams-webhook.yaml
./mywant wants get {want-id}
# State内の "webhook_url" フィールド (例: /api/v1/webhooks/{want-id}) を確認
```

### Network Requirements

- Callback URLは **HTTPS必須**
- MyWantサーバーがインターネットから到達可能であること
- 開発時は [ngrok](https://ngrok.com/) 等のトンネリングツールを使用

```bash
# ngrokでローカルサーバーを公開
ngrok http 8080

# Callback URLの例:
# https://xxxx.ngrok-free.app + State の webhook_url 値
```

---

### 方式A: Outgoing Webhook (@mention必須)

> 公式ドキュメント: [Create an Outgoing Webhook - Microsoft Learn](https://learn.microsoft.com/en-us/microsoftteams/platform/webhooks-and-connectors/how-to/add-outgoing-webhook)

シンプルな設定で、チャネルで `@mention` されたメッセージのみを受信する。

**Teams側の設定:**

1. **Teams** 左ペインからチームを選択 → **•••** (more options) → **Manage team**
2. **Apps** タブ → **Create an outgoing webhook**
3. 以下を入力:
   - **Name**: 任意の名前 (e.g., "MyWant Bot") — チャネルでの @mention 名になる
   - **Callback URL**: `https://{your-server}` + Stateから取得した `webhook_url` の値
   - **Description**: 任意の説明
4. **Create** をクリック
5. 表示される **Security token** (HMAC) をコピーし、Want の `webhook_secret` パラメータに設定

> **Note**: Security tokenは作成時に一度だけ表示される。期限はなく、設定ごとに固有の値となる。

**使い方:**

チャネルで `@MyWant Bot メッセージ` のように @mention すると、Teams が Callback URL に POST リクエストを送信する。MyWant は5秒以内に応答する必要がある。

---

### 方式B: Power Automate (@mention不要)

> 公式ドキュメント: [Microsoft Teams connectors - Power Automate](https://learn.microsoft.com/en-us/connectors/teams/)

チャネルの全メッセージを自動転送する。@mention不要で、Power Automateライセンスが必要。

**Power Automate側の設定:**

1. [Power Automate](https://make.powerautomate.com/) でフローを新規作成
2. トリガー: **When a new channel message is added** を選択し、対象のチーム・チャネルを指定
3. アクション: **HTTP** を追加し、以下を設定:
   - **Method**: `POST`
   - **URI**: `https://{your-server}` + Stateから取得した `webhook_url` の値
   - **Headers**: `Content-Type: application/json`
   - **Body**: Teams メッセージの動的コンテンツをJSON形式で構成
     ```json
     {
       "type": "message",
       "text": "@{triggerOutputs()?['body/body/plainTextContent']}",
       "from": {"name": "@{triggerOutputs()?['body/from/user/displayName']}"},
       "channelId": "msteams",
       "timestamp": "@{triggerOutputs()?['body/createdDateTime']}"
     }
     ```
4. フローを保存・有効化

**特徴:**

| | Outgoing Webhook (方式A) | Power Automate (方式B) |
|---|---|---|
| @mention | 必須 | 不要 |
| 対象メッセージ | @mention付きのみ | チャネルの全メッセージ |
| HMAC署名検証 | あり | なし (Power Automate側で認証) |
| 設定の容易さ | 簡単 | Power Automateの知識が必要 |
| ライセンス | Teams標準機能 | Power Automateライセンスが必要 |

## Agent System

### monitor_teams_webhook (PollAgent)

- **Type**: Monitor (Poll-based)
- **Capability**: `teams_webhook_monitoring`
- **Interval**: 5秒
- **Role**:
  - メッセージバッファを20件にトリム
  - `teams_webhook_status` のヘルスチェック
  - `stopped` 状態で自動停止

### Agent Startup

MonitorAgentは `Initialize()` で自動起動される。`Progress()` でも再起動チェックを行い、サーバーリスタート後も自動復帰する。

```
Initialize() → StopAllBackgroundAgents() → startMonitoringAgent()
Progress()   → startMonitoringAgent() (if not running)
```

## Monitoring and Debugging

### Check webhook endpoints
```bash
curl http://localhost:8080/api/v1/webhooks
```

### Check want state
```bash
./mywant wants get {want-id}
```

### Send multiple test messages
```bash
for i in $(seq 1 5); do
  curl -s -X POST http://localhost:8080/api/v1/webhooks/{want-id} \
    -H "Content-Type: application/json" \
    -d "{\"type\":\"message\",\"text\":\"Message $i\",\"from\":{\"name\":\"Tester\"},\"channelId\":\"msteams\"}"
  echo ""
done
```

### Check logs
```bash
./mywant logs
```

## File Structure

| File | Description |
|------|-------------|
| `engine/cmd/types/teams_webhook_types.go` | Want type implementation |
| `engine/cmd/types/agent_monitor_teams_webhook.go` | PollAgent implementation |
| `pkg/server/handlers_webhook.go` | Webhook HTTP handlers |
| `pkg/server/handlers_setup.go` | Route registration |
| `yaml/agents/agent-teams-webhook.yaml` | Agent YAML definition |
| `yaml/config/config-teams-webhook.yaml` | Sample config |
| `yaml/want_types/system/teams_webhook.yaml` | Want type definition |

## Limitations and Future Work

### Current Limitations
- メッセージバッファはin-memory (サーバーリスタートで消失、ただしstateが永続化されている場合は復元)
- `channel_filter` はWant type内では未実装 (ハンドラ側でのフィルタリングは今後追加)
- Teams Adaptive Cardレスポンスは未対応

### Planned Enhancements
- [ ] チャネルフィルタリング (channel_filter param)
- [ ] Adaptive Card形式のレスポンス
- [ ] メッセージ検索 API
- [ ] メッセージごとのリアクション (reply) 機能
- [ ] Slack Webhook対応 (同じエンドポイントで)
- [ ] Webhook payload validation schema
