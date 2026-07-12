#!/usr/bin/env bash
# ============================================================
# SmartGolf Zone Scenario Deployment Script
# Monitor | Thinker | Doer ゾーン検証シナリオ
#
# 構成:
#   PARENT: custom_target (smartgolf-booking-target)
#   ├── MONITOR  : smartgolf_check_reserved  ← 既存予約確認
#   ├── THINKER  : smartgolf_list_available  ← 空き枠一覧取得
#   ├── THINKER  : choice                   ← 枠選択UI
#   └── DOER     : smartgolf_book           ← 予約実行
# ============================================================

set -e

SERVER="${MYWANT_SERVER:-http://localhost:8080}"
API="$SERVER/api/v1"

echo "=== SmartGolf Zone Scenario Deployment ==="
echo "Server: $SERVER"
echo ""

# ── Step 1: Create parent target ──────────────────────────────
echo "[1/2] Creating parent want: smartgolf-booking-target ..."

PARENT_RESP=$(curl -s -X POST "$API/wants" \
  -H "Content-Type: application/json" \
  -d '[{
    "metadata": {
      "name": "smartgolf-booking-target",
      "type": "custom_target",
      "labels": {}
    },
    "spec": {
      "params": {
        "target_value": "SmartGolfの次回予約を管理する"
      }
    }
  }]')

echo "Response: $PARENT_RESP"

PARENT_ID=$(echo "$PARENT_RESP" | python3 -c "
import json, sys
d = json.load(sys.stdin)
ids = d.get('want_ids', [])
if ids:
    print(ids[0])
else:
    print('')
")

if [ -z "$PARENT_ID" ]; then
  echo "ERROR: Failed to create parent want."
  echo "Response was: $PARENT_RESP"
  exit 1
fi

echo "✓ Parent created: $PARENT_ID"
echo ""

# ── Step 2: Create 4 child wants ─────────────────────────────
echo "[2/2] Creating child wants (monitor / thinker×2 / doer) ..."

TOMORROW=$(python3 -c "
from datetime import date, timedelta
print((date.today() + timedelta(days=1)).strftime('%Y-%m-%d'))
")

CHILDREN_RESP=$(curl -s -X POST "$API/wants" \
  -H "Content-Type: application/json" \
  -d "[
    {
      \"metadata\": {
        \"name\": \"smartgolf-check\",
        \"type\": \"smartgolf_check_reserved\",
        \"labels\": {\"child-role\": \"monitor\"},
        \"ownerReferences\": [{
          \"id\": \"$PARENT_ID\",
          \"name\": \"smartgolf-booking-target\",
          \"kind\": \"Want\",
          \"controller\": true
        }]
      },
      \"spec\": {
        \"exposes\": [
          {\"currentState\": \"is_reserved\",  \"asGoal\": \"is_reserved\"},
          {\"currentState\": \"next_datetime\", \"asGoal\": \"next_reservation_datetime\"},
          {\"currentState\": \"next_store\",    \"asGoal\": \"next_reservation_store\"}
        ]
      }
    },
    {
      \"metadata\": {
        \"name\": \"smartgolf-list\",
        \"type\": \"smartgolf_list_available\",
        \"labels\": {\"child-role\": \"thinker\"},
        \"ownerReferences\": [{
          \"id\": \"$PARENT_ID\",
          \"name\": \"smartgolf-booking-target\",
          \"kind\": \"Want\",
          \"controller\": true
        }]
      },
      \"spec\": {
        \"exposes\": [
          {\"currentState\": \"smartgolf_all_available_times\", \"asGlobalParam\": \"smartgolf_all_available_times\"}
        ]
      }
    },
    {
      \"metadata\": {
        \"name\": \"smartgolf-slot-choice\",
        \"type\": \"choice\",
        \"labels\": {\"child-role\": \"thinker\"},
        \"ownerReferences\": [{
          \"id\": \"$PARENT_ID\",
          \"name\": \"smartgolf-booking-target\",
          \"kind\": \"Want\",
          \"controller\": true
        }]
      },
      \"spec\": {
        \"imports\": {
          \"smartgolf_all_available_times\": \"choices\"
        },
        \"exposes\": [
          {\"currentState\": \"selected\", \"asGoal\": \"selected_slot\"}
        ]
      }
    },
    {
      \"metadata\": {
        \"name\": \"smartgolf-book\",
        \"type\": \"smartgolf_book\",
        \"labels\": {\"child-role\": \"doer\"},
        \"ownerReferences\": [{
          \"id\": \"$PARENT_ID\",
          \"name\": \"smartgolf-booking-target\",
          \"kind\": \"Want\",
          \"controller\": true
        }]
      },
      \"spec\": {
        \"params\": {
          \"room\": \"中野新橋店/打席予約(Room02)\",
          \"date\": \"$TOMORROW\",
          \"time\": \"20:00\"
        }
      }
    }
  ]")

echo "Response: $CHILDREN_RESP"

CHILD_COUNT=$(echo "$CHILDREN_RESP" | python3 -c "
import json, sys
d = json.load(sys.stdin)
print(len(d.get('want_ids', [])))
")

echo ""
echo "=== Deployment Complete ==="
echo "Parent ID : $PARENT_ID"
echo "Children  : $CHILD_COUNT wants created"
echo ""
echo "Child IDs:"
echo "$CHILDREN_RESP" | python3 -c "
import json, sys
d = json.load(sys.stdin)
for wid in d.get('want_ids', []):
    print('  -', wid)
"
echo ""
echo "Zone layout:"
echo "  👁 Monitor  : smartgolf-check         (smartgolf_check_reserved)"
echo "  💡 Thinker  : smartgolf-list          (smartgolf_list_available)"
echo "  💡 Thinker  : smartgolf-slot-choice   (choice)"
echo "  ⚡ Doer     : smartgolf-book          (smartgolf_book)"
echo ""
echo "Next steps:"
echo "  1. Open GUI dashboard and click 'smartgolf-booking-target'"
echo "  2. Verify Monitor/Thinker/Doer zones display correctly"
echo "  3. Wait for monitor (check) and thinker (list) to achieve"
echo "  4. Use choice to select a booking slot"
echo "  5. Update smartgolf-book params (room/date/time) and trigger"
