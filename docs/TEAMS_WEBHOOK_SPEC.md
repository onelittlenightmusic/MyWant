# Webhook 設定ガイド (Teams / Slack)

外部サービスからのメッセージを `/api/v1/webhooks/{want-id}` で受信し、Want stateに保存します。Teams・Slack・汎用JSONペイロードを自動判定して処理します。

## Teams Webhook のセットアップ

Teams からのメッセージ受信には2つの方式があります。

| | Outgoing Webhook (方式A) | Power Automate (方式B) |
|---|---|---|
| @mention | 必須 | 不要 |
| 対象メッセージ | @mention付きのみ | 全メッセージ |
| HMAC署名検証 | あり | なし |
| ライセンス | Teams標準機能 | Power Automate必要 |

### 1. MyWant側の準備

1. サーバー起動 & Want をデプロイ:

   ```bash
   make restart-all
   ./bin/mywant wants create -f yaml/config/config-teams-webhook.yaml
   ```

2. Want の State から `webhook_url` を確認:

   ```bash
   ./bin/mywant wants get {want-id}
   ```

   State内に以下のようなパスが表示されます:
   ```
   webhook_url ... /api/v1/webhooks/{want-id}
   ```

   > ダッシュボードのWant DetailでState値をマウスオーバーし、右のコピーアイコンからコピーすると便利です。

### 2. ngrok でトンネルを起動

TeamsからのWebhookはHTTPSが必須のため、開発環境では ngrok を使います。

```bash
ngrok http 8080
```

表示されるURLをメモします（例: `https://xxxx.ngrok-free.app`）。

### 3. Teams側の設定

#### 方式A: Outgoing Webhook (@mention必須)

> 公式ドキュメント: [Create an Outgoing Webhook - Microsoft Learn](https://learn.microsoft.com/en-us/microsoftteams/platform/webhooks-and-connectors/how-to/add-outgoing-webhook)

1. **Teams** 左ペインからチームを選択 → **•••** → **Manage team**
2. **Apps** タブ → **Create an outgoing webhook**
3. **Callback URL** に、ngrokのURL + ステップ1でコピーした `webhook_url` を結合して入力:
   ```
   https://xxxx.ngrok-free.app/api/v1/webhooks/{want-id}
   ```
4. **Name** (= @mention名) と **Description** を入力
5. **Create** → 表示される **Security token** をWantの `webhook_secret` パラメータに設定

#### 方式B: Power Automate (@mention不要)

> 公式ドキュメント: [Microsoft Teams connectors - Power Automate](https://learn.microsoft.com/en-us/connectors/teams/)

1. [Power Automate](https://make.powerautomate.com/) でフローを新規作成
2. トリガー: **When a new channel message is added** → チーム・チャネルを指定
3. アクション: **HTTP** → POST に、ngrokのURL + ステップ1でコピーした `webhook_url` を結合して入力:
   ```
   https://xxxx.ngrok-free.app/api/v1/webhooks/{want-id}
   ```
4. Body:
   ```json
   {
     "type": "message",
     "text": "@{triggerOutputs()?['body/body/plainTextContent']}",
     "from": {"name": "@{triggerOutputs()?['body/from/user/displayName']}"},
     "channelId": "msteams",
     "timestamp": "@{triggerOutputs()?['body/createdDateTime']}"
   }
   ```

### 4. 動作確認

1. Teamsチャネルでメッセージを投稿する
   - 方式A: `@MyWantBot Hello from Teams!` と @mention 付きで投稿
   - 方式B: 任意のメッセージを投稿
2. WantのStateを確認:

   ```bash
   ./bin/mywant wants get {want-id}
   ```

3. `teams_latest_message` に投稿したメッセージが反映されていればOK:

   ```
   teams_latest_message ... {"sender":"Test User","text":"Hello from Teams!","timestamp":"...","channel_id":"..."}
   ```

   > ダッシュボードのWant Detailからも `teams_latest_message` の値をマウスオーバーで確認できます。

### YAML Config

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
        webhook_secret: "${TEAMS_WEBHOOK_SECRET}"
        channel_filter: ""
      capabilities:
        - teams_webhook_monitoring
```

---

## Slack Webhook のセットアップ

### 1. Slack App の作成

1. [Slack API](https://api.slack.com/apps) → **Create New App** → **From scratch**
2. App名とワークスペースを選択して **Create App**
3. 左メニュー **Basic Information** → **App Credentials** → **Signing Secret** をコピーしておく

### 2. MyWant側の準備

1. YAML設定の `signing_secret` にステップ1で取得した Signing Secret を設定:

   ```yaml
   # yaml/config/config-slack-webhook.yaml
   wants:
     - metadata:
         name: slack-inbox
         type: slack webhook
       spec:
         params:
           signing_secret: "取得したSigning Secret"
   ```

2. サーバー起動 & Want をデプロイ:

   ```bash
   make restart-all
   ./bin/mywant wants create -f yaml/config/config-slack-webhook.yaml
   ```

3. Want の State から `webhook_url` を確認:

   ```bash
   ./bin/mywant wants get {want-id}
   ```

   State内に以下のようなパスが表示されます:
   ```
   webhook_url ... /api/v1/webhooks/{want-id}
   ```

   > ダッシュボードのWant DetailでState値をマウスオーバーし、右のコピーアイコンからコピーすると便利です。

### 3. ngrok でトンネルを起動

SlackからのWebhookはHTTPSが必須のため、開発環境では ngrok を使います。

```bash
ngrok http 8080
```

表示されるURLをメモします（例: `https://xxxx.ngrok-free.app`）。

### 4. Slack App の Event Subscriptions を設定

1. [Slack API](https://api.slack.com/apps) で作成したAppを開く
2. 左メニュー **Event Subscriptions** → **Enable Events** をON
3. **Request URL** に、ngrokのURL + ステップ2でコピーした `webhook_url` を結合して入力:
   ```
   https://xxxx.ngrok-free.app/api/v1/webhooks/{want-id}
   ```
   - SlackがURL verification challengeを送信し、MyWantが自動応答します
   - 「**Verified**」と表示されればOK
4. **Subscribe to bot events** で受信したいイベントを追加:
   - `message.channels` — パブリックチャネルのメッセージ
   - `message.groups` — プライベートチャネルのメッセージ (任意)
   - `message.im` — ダイレクトメッセージ (任意)
5. **Save Changes**

### 5. Bot Token Scopes の設定 & インストール

1. 左メニュー **OAuth & Permissions** → **Scopes** → **Bot Token Scopes** で以下を追加:
   - `channels:history` — パブリックチャネルのメッセージ読み取り
   - `groups:history` — プライベートチャネル (任意)
   - `im:history` — DM (任意)
2. 左メニュー **Install App** → **Install to Workspace** → 権限を確認して **Allow**
3. ボットをチャネルに招待: `/invite @YourAppName`

### 6. 動作確認

1. ボットを招待したSlackチャネルで、任意のメッセージを投稿する（例: `Hello from Slack!`）
2. WantのStateを確認:

   ```bash
   ./bin/mywant wants get {want-id}
   ```

3. `slack_latest_message` に投稿したメッセージが反映されていればOK:

   ```
   slack_latest_message ... {"sender":"U01ABCDEF","text":"Hello from Slack!","timestamp":"...","channel_id":"C01ABCDEF23"}
   ```

   > ダッシュボードのWant Detailからも `slack_latest_message` の値をマウスオーバーで確認できます。

### YAML Config

```yaml
wants:
  - metadata:
      name: slack-inbox
      type: slack webhook
      labels:
        category: communication
        source: slack
    spec:
      params:
        signing_secret: "${SLACK_SIGNING_SECRET}"
        channel_filter: ""
      capabilities:
        - slack_webhook_monitoring
```

---

## Pipeline で下流 Want と連携

メッセージを受信し、下流のWantで処理する例:

```yaml
wants:
  - metadata:
      name: slack-inbox
      type: slack webhook
      labels:
        role: source
    spec:
      params:
        signing_secret: ""
      capabilities:
        - slack_webhook_monitoring

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

メッセージ受信時、`Progress()` が `Provide()` で最新メッセージを下流に送信します。

---

## Monitoring and Debugging

```bash
# Webhook endpoint 一覧
curl http://localhost:8080/api/v1/webhooks

# Want state 確認
./bin/mywant wants get {want-id}

# Teams: 複数メッセージ送信テスト
for i in $(seq 1 5); do
  curl -s -X POST http://localhost:8080/api/v1/webhooks/{want-id} \
    -H "Content-Type: application/json" \
    -d "{\"type\":\"message\",\"text\":\"Message $i\",\"from\":{\"name\":\"Tester\"},\"channelId\":\"msteams\"}"
done

# Slack: 複数メッセージ送信テスト
for i in $(seq 1 5); do
  curl -s -X POST http://localhost:8080/api/v1/webhooks/{want-id} \
    -H "Content-Type: application/json" \
    -d "{\"type\":\"event_callback\",\"event\":{\"type\":\"message\",\"user\":\"U01TEST\",\"text\":\"Message $i\",\"channel\":\"C01TEST\",\"ts\":\"123456789$i.000000\"}}"
done

# ログ確認
./bin/mywant logs
```

---

## リファレンス

### Parameters

| | Teams | Slack |
|---|---|---|
| シークレット | `webhook_secret` (Base64エンコード済み) | `signing_secret` (平文) |
| チャネルフィルタ | `channel_filter` | `channel_filter` |

### State Fields

TeamsとSlackでstate keyのプレフィックスが異なります (`teams_` / `slack_`)。

| Field | Teams | Slack | Type |
|---|---|---|---|
| status | `teams_webhook_status` | `slack_webhook_status` | string (`active`/`stopped`/`error`) |
| latest message | `teams_latest_message` | `slack_latest_message` | object |
| messages | `teams_messages` | `slack_messages` | []object (FIFO, 最大20件) |
| message count | `teams_message_count` | `slack_message_count` | int |

メッセージ構造 (共通):

```json
{
  "sender": "User Name or User ID",
  "text": "メッセージ本文",
  "timestamp": "2026-02-09T12:00:00Z",
  "channel_id": "channel identifier"
}
```

> Teamsでは `sender` はユーザー表示名、Slackでは `sender` はユーザーID (`U01ABCDEF`)。

### 署名検証の比較

| | Teams | Slack |
|---|---|---|
| ヘッダー | `Authorization: HMAC <base64>` | `X-Slack-Signature: v0=<hex>` + `X-Slack-Request-Timestamp` |
| アルゴリズム | HMAC-SHA256 | HMAC-SHA256 |
| シークレット | Base64デコード → バイト列 | Signing Secretをそのまま使用 |
| ベース文字列 | リクエストボディ全体 | `v0:{timestamp}:{body}` |
| 出力形式 | Base64 | `v0=` + hex |
| リプレイ攻撃防止 | なし | 5分以内のタイムスタンプチェック |

### API Endpoints

**POST /api/v1/webhooks/{want-id}** — ペイロード受信。自動判定で Teams/Slack/汎用 に振り分け。

| Status Code | 説明 |
|---|---|
| `200 OK` | 受信成功 |
| `400 Bad Request` | JSONパース失敗 |
| `401 Unauthorized` | 署名検証失敗 |
| `404 Not Found` | Want IDが存在しない |

**GET /api/v1/webhooks** — Webhook受信可能なWant一覧 (Teams + Slack)。
