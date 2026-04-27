# エージェント・デプロイメント・ガイド

このドキュメントは、MyWant エージェントの実行モード（通信方式）とデプロイ形態（配置構成）についてのガイドです。

---

## 1. 概要

MyWant は、小規模なローカル実行から、複数のワーカープロセスに分散した大規模な実行まで柔軟に対応可能です。これは、エージェントの「実行モード（Executor）」と「Workerモード」の組み合わせにより実現されます。

### デプロイメント・バリエーション

| パターン | 通信方式 | プロセス構成 | 特徴 |
|:---|:---|:---|:---|
| **Local (デフォルト)** | 関数呼び出し | 単一プロセス | 低レイテンシ、設定不要 |
| **Separate Worker** | Webhook | Server + Worker | プロセス分離、独立したスケーリング |
| **External Service** | Webhook | Server + 他言語サービス | Python/Node.js等での実装が可能 |
| **Cloud/Docker** | Webhook | Server + Containers | 高度な分離とリソース制限 |

---

## 2. 実行モード (Execution Modes)

エージェントの `execution.mode` フィールドで指定します。

### 2.1 Local Mode
同一プロセス内の Go 関数として実行します。
```yaml
execution:
  mode: local
```

### 2.2 Webhook Mode
外部の HTTP エンドポイントへリクエストを送信して実行します。
```yaml
execution:
  mode: webhook
  webhook:
    service_url: "http://localhost:8081"
    callback_url: "http://localhost:8080/api/v1/agents/webhook/callback"
```

---

## 3. Worker モード (Standalone Agent Service)

`mywant start --worker` フラグを使用すると、エージェント実行に特化したステートレスなサービスを起動できます。

### アーキテクチャ
- **Main Server**: Want、State、Recipe、Reconciliation を管理。
- **Worker (Agent Service)**: エージェントの実装を保持し、リクエストに応じて実行するのみ。

### 起動コマンド
```bash
# サーバー (Port 8080)
./bin/mywant start --port 8080

# ワーカー (Port 8081)
./bin/mywant start --worker --port 8081
```

---

## 4. 構成例：分散実行

メインサーバーから、Worker プロセスのエージェントを呼び出す設定例です。

### サーバー側の設定 (config.yaml)
```yaml
agents:
  - name: heavy_task_agent
    type: do
    execution:
      mode: webhook
      webhook:
        service_url: "http://worker-node:8081"
        callback_url: "http://main-server:8080/api/v1/agents/webhook/callback"
```

### ワーカー側の役割
ワーカーは起動時に自身の `yaml/agents/` をスキャンし、`heavy_task_agent` を登録します。サーバーからの POST リクエストを受けると、ローカルで処理を実行し結果を返します。

---

## 5. セキュリティ

外部（Webhook/Worker）との通信には、Bearer トークンによる認証が必要です。

### 設定方法
サーバーとワーカーの両方で同じ環境変数を設定します。
```bash
export WEBHOOK_AUTH_TOKEN="your-secret-token"
```

### 通信の保護
- 本番環境では `https` を使用してください。
- ファイアウォールでワーカーへのアクセスをサーバー IP に限定することを推奨します。

---

## 6. トラブルシューティング

- **接続エラー**: サーバーからワーカーの `service_url` に到達できるか確認してください。
- **認証エラー**: `WEBHOOK_AUTH_TOKEN` が一致しているか確認してください。
- **Agent Not Found**: ワーカー側に該当するエージェント YAML が存在するか確認してください。
