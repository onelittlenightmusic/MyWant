# Playwright MCP App Server

Playwright MCP App Server は MyWant の **replay want type** 向けの MCP サーバーです。
ブラウザ操作を記録し、Playwright `.spec.ts` スクリプトとして保存します。

## セットアップ

```bash
cd mcp/playwright-app
npm install
npx playwright install chromium
npm run build
```

## 動作確認

```bash
node dist/server.js   # stdio transport で起動 (MCP client 接続待ち)
```

## 提供する MCP ツール

| ツール名 | 説明 | 入力 | 出力 |
|---|---|---|---|
| `start_recording` | ブラウザセッション開始・スクリーンショット配信開始 | `target_url?` | `{ session_id, ui_url }` |
| `stop_recording` | セッション停止・スクリプト生成 | `session_id` | `{ script }` |

## 環境変数

| 変数名 | デフォルト | 説明 |
|---|---|---|
| `PLAYWRIGHT_WS_PORT` | `9321` | WebSocket サーバーポート（スクリーンショット配信） |
| `PLAYWRIGHT_HTTP_PORT` | `9322` | HTTP サーバーポート（ブラウザ UI 提供） |

## アーキテクチャ

```
Go MonitorAgent (playwright_record_monitor)
  │
  ├─ start_recording tool call (MCP/stdio)
  │    → Chromium 起動
  │    → WebSocket サーバー起動 (WS_PORT)
  │    → HTTP サーバー (HTML UI) 起動 (HTTP_PORT)
  │    → { session_id, ui_url } を返す
  │
  └─ stop_recording tool call (MCP/stdio)
       → スクリーンショット停止
       → Playwright actions → .spec.ts 生成
       → ブラウザ終了
       → { script } を返す

Frontend iframe → http://localhost:9322/?session=xxx
  │
  └─ WebSocket ws://localhost:9321/?session=xxx
       ↑ スクリーンショット (base64 PNG 200ms間隔)
       ↓ マウスクリック/キー入力イベント
```
