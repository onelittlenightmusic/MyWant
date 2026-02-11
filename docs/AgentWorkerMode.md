# Agent Worker Mode

## 概要

`mywant start --worker`を使用すると、スタンドアロンのAgent Serviceをworkerモードで起動できます。このモードは**完全にステートレス**で、メインサーバーからのリクエストに応じてエージェントを実行するだけです。

## アーキテクチャ

```
┌─────────────────────────────┐         ┌─────────────────────────────┐
│   メインサーバー (8080)      │         │  Agent Service (8081)       │
├─────────────────────────────┤         ├─────────────────────────────┤
│ - ChainBuilder              │         │ - AgentRegistry             │
│ - Want管理                   │◄───────►│ - 登録済みAgents            │
│ - State管理                  │  HTTP   │ - HTTPエンドポイント         │
│ - YAML設定管理               │         │                             │
│ - WebhookExecutor           │         │ ステートレス:               │
│   └─ ServiceURL: :8081      │         │ - Want保存なし              │
└─────────────────────────────┘         │ - State保存なし             │
                                        │ - YAML管理なし              │
                                        └─────────────────────────────┘
```

## 起動方法

### 1. 通常モード（メインサーバー）

```bash
# メインサーバーを起動
./bin/mywant start --port 8080

# または、バックグラウンドで起動
./bin/mywant start --port 8080 -D
```

### 2. Workerモード（Agent Service）

```bash
# Agent Serviceをworkerモードで起動
./bin/mywant start --worker --port 8081

# または、バックグラウンドで起動
./bin/mywant start --worker --port 8081 -D
```

### 3. 両方を起動（分散構成）

```bash
# ターミナル1: メインサーバー
./bin/mywant start --port 8080 -D

# ターミナル2: Agent Service
./bin/mywant start --worker --port 8081 -D
```

## API エンドポイント

Agent Service (worker mode) が提供するエンドポイント:

### DoAgent実行
```bash
POST /api/v1/agent-service/execute

# リクエスト例
{
  "want_id": "flight-booking-001",
  "agent_name": "agent_flight_api",
  "want_state": {
    "departure": "NRT",
    "arrival": "LAX",
    "date": "2026-03-15"
  }
}

# レスポンス例
{
  "status": "completed",
  "state_updates": {
    "booking_id": "FL-12345",
    "status": "confirmed"
  },
  "execution_time_ms": 150
}
```

### MonitorAgent実行
```bash
POST /api/v1/agent-service/monitor/execute

# リクエスト例
{
  "want_id": "flight-booking-001",
  "agent_name": "monitor_flight_api",
  "callback_url": "http://localhost:8080/api/v1/agents/webhook/callback",
  "want_state": {
    "booking_id": "FL-12345"
  }
}

# レスポンス例
{
  "status": "completed",
  "state_updates_count": 2,
  "execution_time_ms": 200
}
```

### ヘルスチェック
```bash
GET /health

# レスポンス例
{
  "status": "healthy",
  "mode": "worker",
  "agents_count": 16,
  "capabilities": 12
}
```

### Agent一覧
```bash
GET /api/v1/agents

# レスポンス例
{
  "agents": [
    {
      "name": "agent_flight_api",
      "type": "do",
      "capabilities": ["flight_api_agency"]
    },
    ...
  ],
  "count": 16
}
```

### Capability一覧
```bash
GET /api/v1/capabilities

# レスポンス例
{
  "capabilities": [
    {
      "name": "flight_api_agency",
      "gives": ["create_flight", "cancel_flight"]
    },
    ...
  ],
  "count": 12
}
```

## 設定例

### メインサーバー側の設定

Agentの実行モードをwebhookに設定し、Agent ServiceのURLを指定:

```yaml
# config.yaml
agents:
  - name: flight_booking_agent
    type: do
    execution:
      mode: webhook
      webhook:
        service_url: "http://localhost:8081"  # Agent ServiceのURL
        callback_url: "http://localhost:8080/api/v1/agents/webhook/callback"
        auth_token: "${WEBHOOK_AUTH_TOKEN}"
        timeout_ms: 30000
```

### 環境変数

```bash
# 認証トークン（両方で同じ値を設定）
export WEBHOOK_AUTH_TOKEN="your-secret-token"

# 開発モードではトークンなしでも動作
# unset WEBHOOK_AUTH_TOKEN
```

## Worker Modeの特徴

### ✅ 含まれるもの
- AgentRegistryの初期化
- YAMLからのagent/capability読み込み
- コードベースのagent登録（execution_command, mcp_tools等）
- Agent Service HTTPエンドポイント
- 認証（Bearer token）
- ヘルスチェックとモニタリングエンドポイント

### ❌ 含まれないもの（メインサーバーの責務）
- ChainBuilder
- Want管理
- State管理
- YAML設定管理
- Webフロントエンド
- Reconciliationループ

## 利点

1. **スケーラビリティ**: Agent Serviceを複数起動可能
2. **分離**: メインサーバーとAgent実行環境を分離
3. **柔軟性**: 異なるマシン/ネットワークで実行可能
4. **軽量**: ステートレスなので起動が高速
5. **言語非依存**: HTTP経由なので、将来的に他言語での実装も可能

## ログ

### メインサーバーのログ
```bash
tail -f ~/.mywant/server.log
```

### Agent Serviceのログ
```bash
tail -f ~/.mywant/agent-service.log
```

## トラブルシューティング

### Agent Serviceに接続できない

1. Agent Serviceが起動しているか確認:
```bash
curl http://localhost:8081/health
```

2. ポートが正しいか確認

3. ファイアウォール設定を確認

### 認証エラー

```bash
# 環境変数を確認
echo $WEBHOOK_AUTH_TOKEN

# 開発モードでは認証を無効化
unset WEBHOOK_AUTH_TOKEN
```

### Agent not found

Agent Serviceのログで登録されたAgentを確認:
```bash
grep "Registered agent" ~/.mywant/agent-service.log
```

## 次のステップ

- Phase 4 (オプション): ドキュメントと実例の追加
- Agent Serviceのクラスタリング
- ロードバランシング
- メトリクス収集（Prometheus等）
