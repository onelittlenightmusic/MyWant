# MyWant MCP Server

MyWant APIをMCP (Model Context Protocol) ツールとして公開するサーバーです。Claude Code等のMCP対応クライアントからMyWantのWant TypeやRecipeを参照できます。

## ビルド

```sh
cd mcp/server
make build
```

## 利用可能なツール

| ツール名 | 説明 |
|---|---|
| `list_want_types` | 登録済みの全Want Typeを一覧表示 |
| `get_want_type` | 指定したWant Typeの詳細（パラメータ、State、エージェント）を取得 |
| `search_want_types` | カテゴリやパターンでWant Typeを検索 |
| `list_recipes` | 登録済みの全Recipeを一覧表示 |
| `get_recipe` | 指定したRecipeの詳細（Wants、パラメータ、メタデータ）を取得 |

## 接続設定

### 前提条件

MyWantサーバーが起動していること:

```sh
./bin/mywant start -D
```

### Claude Code (`.mcp.json`)

プロジェクトルートの `.mcp.json` に以下を追加:

```json
{
  "mcpServers": {
    "mywant": {
      "command": "./mcp/server/mywant-mcp-server",
      "args": ["-api-url", "http://localhost:8080"]
    }
  }
}
```

### 起動オプション

| フラグ | デフォルト | 説明 |
|---|---|---|
| `-api-url` | `http://localhost:8080` | MyWant APIのベースURL |

## 通信方式

stdio (JSON-RPC) で通信します。MCP クライアントがプロセスを起動し、stdin/stdout 経由でやり取りします。
