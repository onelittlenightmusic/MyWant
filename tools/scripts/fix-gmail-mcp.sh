#!/bin/bash

# Quick Fix for Gmail MCP Issues
# One-command solution for "MCP agent returned no result"

echo "🔧 Quick Fix: Resetting Gmail MCP..."
echo ""

# Kill all Goose and MCP processes
echo "1. Killing stale processes..."
pkill -9 -f "goose run" 2>/dev/null || true
pkill -9 -f "server-gmail-autoauth-mcp" 2>/dev/null || true
sleep 1
echo "   ✓ Processes cleared"

# Clear NPX cache
echo "2. Clearing NPX cache..."
rm -rf ~/.npm/_npx 2>/dev/null || true
echo "   ✓ Cache cleared"

# Clear Goose sessions
echo "3. Clearing Goose sessions..."
rm -rf ~/.config/goose/sessions/* 2>/dev/null || true
echo "   ✓ Sessions cleared"

# Gmail MCP re-authentication
echo ""
echo "4. Gmail MCP トークン更新 & 再認証..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Initialize flag
SKIP_FULL_AUTH=false

# Step 4-1: Refresh token using auth command (with timeout)
echo "   🔄 既存トークンをリフレッシュ中（最大10秒）..."
echo ""

# Run auth command with timeout and stdin redirect to prevent hanging
timeout 10 npx @gongrzhe/server-gmail-autoauth-mcp auth < /dev/null > /tmp/gmail-mcp-refresh.log 2>&1 &
AUTH_PID=$!

# Wait for completion or timeout
wait $AUTH_PID 2>/dev/null
AUTH_RESULT=$?

if [ $AUTH_RESULT -eq 0 ]; then
    echo "   ✅ トークンのリフレッシュ成功"
    echo "   📄 詳細: cat /tmp/gmail-mcp-refresh.log"
    echo ""

    # If token refresh succeeded, we might not need full auth
    echo "   トークン更新完了。サーバー起動をスキップしますか？ (y/n)"
    read -p "   (既に認証済みなら y を推奨): " -n 1 -r
    echo ""
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "   ✓ 認証プロセスをスキップしました"
        SKIP_FULL_AUTH=true
    else
        echo "   完全な認証フローも実行します..."
        SKIP_FULL_AUTH=false
    fi
elif [ $AUTH_RESULT -eq 124 ]; then
    echo "   ⏱️  トークンリフレッシュがタイムアウトしました"
    echo "   📄 詳細: cat /tmp/gmail-mcp-refresh.log"
    echo ""
    echo "   新規認証フローを開始します..."
    echo ""
    SKIP_FULL_AUTH=false
else
    echo "   ⚠️  トークンリフレッシュ失敗（初回認証が必要かもしれません）"
    echo "   📄 詳細: cat /tmp/gmail-mcp-refresh.log"
    echo ""
    echo "   新規認証フローを開始します..."
    echo ""
    SKIP_FULL_AUTH=false
fi

# Step 4-2: Start full authentication flow in background (if needed)
if [ "$SKIP_FULL_AUTH" = false ]; then
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "   📋 Gmail MCP サーバーをバックグラウンドで起動します"
    echo "   ブラウザが自動的に開くので、認証を完了してください"
    echo ""

    # Start Gmail MCP server in background with stdin redirected
    echo "   🚀 Gmail MCP サーバー起動中..."
    nohup npx @gongrzhe/server-gmail-autoauth-mcp < /dev/null > /tmp/gmail-mcp-auth.log 2>&1 &
    MCP_PID=$!
    echo "   ✓ サーバー起動 (PID: $MCP_PID)"
    echo ""

    # Wait for browser to open
    echo "   ⏳ ブラウザが開くまで待機（最大10秒）..."
    sleep 10

    # Check if process is still running
    if ps -p $MCP_PID > /dev/null; then
        echo "   ✓ サーバー実行中"
        echo ""
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo ""
        echo "🌐 ブラウザで以下の操作を行ってください："
        echo "   1. Google アカウントでログイン"
        echo "   2. Gmail へのアクセス権限を承認"
        echo "   3. 「認証が完了しました」と表示されるまで待つ"
        echo ""
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo ""

        # Wait for user confirmation
        read -p "認証は完了しましたか？ (y/n): " -n 1 -r
        echo ""

        if [[ $REPLY =~ ^[Yy]$ ]]; then
            echo ""
            echo "   ✅ 認証完了！Gmail MCP サーバーを停止します..."
            kill $MCP_PID 2>/dev/null || true
            sleep 2
            echo "   ✓ サーバー停止完了"
        else
            echo ""
            echo "   ⚠️  認証が未完了の場合、以下で確認してください:"
            echo "      tail -f /tmp/gmail-mcp-auth.log"
            echo ""
            echo "   Gmail MCP サーバーを停止します..."
            kill $MCP_PID 2>/dev/null || true
            sleep 2
            echo "   ❌ 認証未完了。手動で再試行してください:"
            echo "      npx @gongrzhe/server-gmail-autoauth-mcp auth"
            exit 1
        fi
    else
        echo "   ❌ サーバー起動に失敗しました"
        echo "   ログを確認: cat /tmp/gmail-mcp-auth.log"
        exit 1
    fi
else
    echo ""
    echo "   ✓ 完全な認証プロセスをスキップしました"
    echo ""
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Restart MyWant server
echo "5. Restarting MyWant server..."
cd "$(git rev-parse --show-toplevel)"
make restart-all > /dev/null 2>&1 &
sleep 3
echo "   ✓ Server restarting"

echo ""
echo "✅ Gmail MCP reset complete!"
echo ""
echo "Wait 20 seconds for server startup, then test with:"
echo "  curl -X POST http://localhost:8080/api/v1/wants \\"
echo "    -H 'Content-Type: application/json' \\"
echo "    -d '{\"metadata\":{\"name\":\"test\",\"type\":\"gmail\"},\"spec\":{\"params\":{\"prompt\":\"is:unread\"}}}'"
echo ""
