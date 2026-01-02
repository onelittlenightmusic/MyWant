# Gmail MCP Troubleshooting Guide

## Quick Reference

**"MCP agent returned no result" エラーが出た時:**

```bash
# 最速の修復方法
make fix-mcp

# 詳細な診断が必要な場合
make troubleshoot-mcp
```

## エラーの原因

### 1. Gooseプロセスのクラッシュ
- **症状**: タイムアウト、応答なし
- **原因**: 前回のGooseプロセスが残っている
- **修復**: `pkill -9 -f "goose run"`

### 2. Gmail MCP認証切れ
- **症状**: "authentication failed", "no result"
- **原因**: OAuth トークンの有効期限切れ
- **修復**: `goose configure` → Gmail extension を再設定

### 3. NPXキャッシュ破損
- **症状**: MCP サーバーが起動しない
- **原因**: `~/.npm/_npx` のキャッシュ問題
- **修復**: `rm -rf ~/.npm/_npx`

### 4. 設定ファイルの問題
- **症状**: Goose が Gmail extension を検出しない
- **原因**: `~/.config/goose/config.yaml` の形式エラー
- **修復**: config.yaml を検証

## スクリプト詳細

### `fix-gmail-mcp.sh` - クイック修復

自動実行内容:
1. Goose/MCP プロセスを全て kill
2. NPX キャッシュをクリア
3. Goose セッションをクリア
4. **Gmail MCP トークン更新 & 再認証**
   - **4-1. トークンリフレッシュ**: `npx @gongrzhe/server-gmail-autoauth-mcp auth` を実行
   - **4-2. 完全な再認証**: Gmail MCP サーバーをバックグラウンド起動
   - ブラウザが自動的に開く
   - OAuth 認証フローで Google アカウント認証
   - 認証完了後、サーバー自動停止
5. MyWant サーバーを再起動

使用方法:
```bash
./scripts/fix-gmail-mcp.sh
# または
make fix-mcp
```

**重要**:
- **トークンリフレッシュ**: 既存トークンを更新（既に認証済みの場合）
- **新規認証**: トークンがない/期限切れの場合はブラウザで再認証
- Gmail MCP サーバーは `nohup` + `< /dev/null` でバックグラウンド実行
- `suspended (tty input)` の問題を回避
- ログファイル:
  - トークンリフレッシュ: `/tmp/gmail-mcp-refresh.log`
  - 完全な認証: `/tmp/gmail-mcp-auth.log`

### `troubleshoot-gmail-mcp.sh` - 詳細診断

診断内容:
1. 実行中プロセスの確認・クリーンアップ
2. Goose 設定ファイルの検証
3. Gmail MCP パッケージの確認
4. Goose 基本動作テスト
5. Gmail MCP 統合テスト
6. MyWant サーバーログの確認
7. 推奨される修復方法の提示

使用方法:
```bash
./scripts/troubleshoot-gmail-mcp.sh
# または
make troubleshoot-mcp
```

## 手動トラブルシューティング

### ステップ1: プロセス確認
```bash
# Gooseプロセス確認
ps aux | grep "goose run"

# MCP サーバープロセス確認
ps aux | grep "server-gmail-autoauth-mcp"

# すべて kill
pkill -9 -f "goose run"
pkill -9 -f "server-gmail-autoauth-mcp"
```

### ステップ2: 設定確認
```bash
# Gmail extension が有効か確認
cat ~/.config/goose/config.yaml | grep -A 10 "gmail:"

# 正しい形式:
# extensions:
#   gmail:
#     name: "Gmail MCP"
#     type: stdio
#     cmd: npx
#     args:
#       - "@gongrzhe/server-gmail-autoauth-mcp"
#     enabled: true
#     timeout: 300
```

### ステップ3: Goose 動作テスト
```bash
# 基本テスト
echo "What is 2+2?" | goose run -i -

# Gmail MCP テスト
echo "Use Gmail MCP to search for unread emails" | goose run -i -
```

### ステップ4: ログ確認
```bash
# MyWant サーバーログ
tail -f ./logs/mywant-backend.log | grep -i "mcp\|goose\|gmail"

# エラー検索
grep -i "error\|failed" ./logs/mywant-backend.log | grep -i "mcp\|goose"
```

## よくある問題と解決方法

### 問題1: "Goose process timeout"
```bash
# タイムアウト値を増やす
# ~/.config/goose/config.yaml で:
extensions:
  gmail:
    timeout: 600  # 300 → 600 に変更
```

### 問題2: "JSON parse error"
- **原因**: Goose の出力形式が変わった
- **確認**: Goose のバージョン確認 `goose --version`
- **対処**: 最新版にアップデート `brew upgrade goose` (macOS)

### 問題3: "Gmail authentication required"
```bash
# Gmail 再認証
goose configure
# → "Modify Extension" を選択
# → "gmail" を選択
# → 認証フローに従う
```

### 問題4: "No MCP servers found"
```bash
# MCP パッケージの再インストール
rm -rf ~/.npm/_npx
npx @gongrzhe/server-gmail-autoauth-mcp --help

# config.yaml を確認
cat ~/.config/goose/config.yaml
```

## デバッグ時の便利なコマンド

```bash
# Gooseプロセスの監視
watch -n 1 'ps aux | grep goose'

# リアルタイムログ
tail -f ./logs/mywant-backend.log | grep --color -E "ERROR|WARN|MCP|Goose"

# NPX キャッシュの確認
ls -lh ~/.npm/_npx/

# Goose セッション履歴
ls -lh ~/.config/goose/sessions/
```

## 予防策

### 定期的なメンテナンス

```bash
# 週1回の推奨メンテナンス
pkill -f "goose run"                    # 古いプロセスをクリア
rm -rf ~/.config/goose/sessions/*       # セッションをクリア
rm -rf ~/.npm/_npx                      # NPX キャッシュをクリア
```

### 開発時のベストプラクティス

1. **サーバー再起動時は Gmail MCP もリセット**
   ```bash
   make fix-mcp && make restart-all
   ```

2. **エラー時はログを確認してから修復**
   ```bash
   tail -50 ./logs/mywant-backend.log
   make fix-mcp
   ```

3. **長期間使わない場合は認証を確認**
   ```bash
   # 1週間以上使っていない場合
   goose configure  # Gmail extension を確認
   ```

## サポート

問題が解決しない場合:
1. `make troubleshoot-mcp` の出力を保存
2. `./logs/mywant-backend.log` の最新50行を保存
3. Goose のバージョン情報を確認: `goose --version`
4. Issue を作成してログを添付
