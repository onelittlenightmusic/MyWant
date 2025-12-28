# Reminder Want API Specification

## Overview

The Reminder Want system provides scheduled notification functionality with optional user reaction requirements. Reminders can be:
- **One-time**: Single scheduled reminder at a specific time
- **Recurring**: Repeating reminders using `when` specifications

## Lifecycle

```
┌─────────────────────────────────────────────────────────────┐
│                    INITIALIZATION                           │
│  • Parse parameters (message, ahead, event_time)           │
│  • Calculate reaching_time = event_time - ahead            │
│  • Initialize reminder_phase to "waiting"                  │
│  • Generate reaction_id for tracking                       │
└────────────────────┬────────────────────────────────────────┘
                     ▼
┌─────────────────────────────────────────────────────────────┐
│                    WAITING PHASE                            │
│  Time < reaching_time → Scheduler monitors                 │
└────────────────────┬────────────────────────────────────────┘
                     ▼
          Time >= reaching_time
                     ▼
┌─────────────────────────────────────────────────────────────┐
│                   REACHING PHASE                            │
│  Notification active, optionally awaiting user reaction    │
├─────────────────────────────────────────────────────────────┤
│  If require_reaction = FALSE:                              │
│    • Auto-completes at event_time                          │
│    • → COMPLETED                                           │
├─────────────────────────────────────────────────────────────┤
│  If require_reaction = TRUE:                               │
│    • Awaits POST /api/v1/reactions/{reaction_id}           │
│    • If approved: → COMPLETED                              │
│    • If rejected: → FAILED                                 │
│    • If timeout (5min): → FAILED or COMPLETED              │
└─────────────────────────────────────────────────────────────┘
```

## Parameters

### message (required)
- **Type**: `string`
- **Description**: The reminder notification message
- **Example**: `"Team meeting in 10 minutes"`

### ahead (optional)
- **Type**: `string`
- **Default**: `"5 minutes"`
- **Format**: `\d+ (seconds|minutes|hours|days)`
- **Description**: Time before event_time to enter reaching phase
- **Behavior**:
  - `reaching_time = event_time - ahead`
  - Reminder enters "reaching" phase at reaching_time
  - Event completes at event_time

#### **Ahead Parameter Examples**

| ahead Value | Time Before Event | Use Case |
|------------|-------------------|----------|
| `"10 seconds"` | 10 seconds | Real-time alerts, time-sensitive ops |
| `"5 minutes"` | 5 minutes | Meetings, scheduled tasks (default) |
| `"30 minutes"` | 30 minutes | Important deadlines, prep time |
| `"1 hours"` | 1 hour | Day planning, major events |
| `"1 days"` | 1 day | Important milestones |

#### **Ahead Calculation Example**

```yaml
params:
  message: "Production deployment window starting"
  event_time: "2025-12-28T14:00:00Z"
  ahead: "30 minutes"

# Timeline:
# 13:30:00 UTC → reminder_phase transitions to "reaching"
# 13:30:00 - 14:00:00 UTC → Notification active (reaching phase)
# 14:00:00 UTC → reminder_phase transitions to "completed"
```

### event_time (optional if `when` provided)
- **Type**: `string` (RFC3339)
- **Format**: `YYYY-MM-DDTHH:MM:SSZ`
- **Description**: Scheduled event time
- **Example**: `"2025-12-28T14:00:00Z"`
- **Note**: Either `event_time` or `when` spec must be provided

### require_reaction (optional)
- **Type**: `boolean`
- **Default**: `false`
- **Description**: Whether user approval is required
- **Values**:
  - `false`: Auto-completes at event_time
  - `true`: Awaits user reaction via API

### reaction_type (optional)
- **Type**: `string`
- **Default**: `"internal"`
- **Options**: `"internal"`, `"slack"`, `"webhook"`
- **Description**: Reaction notification channel
- **Current**: Only "internal" (API-based) fully implemented

## State Fields

| Field | Type | Description |
|-------|------|-------------|
| `reminder_phase` | string | One of: waiting, reaching, completed, failed |
| `reaching_time` | RFC3339 | Calculated: event_time - ahead |
| `event_time` | RFC3339 | Scheduled event time |
| `reaction_id` | string | Unique ID for reaction tracking |
| `user_reaction` | object | User's response (approved/rejected) |
| `reaction_result` | string | "approved" or "rejected" |
| `timeout` | boolean | Whether reaction timed out |
| `auto_completed` | boolean | Whether auto-completed without reaction |
| `error_message` | string | Error details if failed |

## API Endpoints

### POST /api/v1/reactions/{reaction_id}

Submit user reaction to a reminder.

**Request Body:**
```json
{
  "approved": true,
  "comment": "Optional comment about this reaction"
}
```

**Response:**
```json
{
  "reaction_id": "reminder-deployment_approval-1735378000123",
  "approved": true,
  "timestamp": "2025-12-28T21:34:43+09:00"
}
```

**Status Codes:**
- `201 Created`: Reaction accepted
- `400 Bad Request`: Invalid request format
- `404 Not Found`: reaction_id not found

### GET /api/v1/reactions

List all pending reactions (debug endpoint).

**Response:**
```json
{
  "count": 2,
  "reactions": [
    {
      "reaction_id": "reminder-deployment_approval-1735378000123",
      "approved": true,
      "comment": "Approved in staging",
      "timestamp": "2025-12-28T21:34:43.208462+09:00"
    }
  ]
}
```

## Usage Examples

### Example 1: Simple Notification (No Reaction)

**Scenario**: Alert 10 minutes before a meeting

```yaml
wants:
  - metadata:
      name: meeting_alert
      type: reminder
    spec:
      params:
        message: "Q4 Planning Meeting starting soon"
        ahead: "10 minutes"
        event_time: "2025-12-29T10:00:00Z"
        require_reaction: false
```

**Timeline:**
```
09:50:00 UTC  → reminder_phase = "reaching" (notification sent)
09:50-10:00   → User sees notification
10:00:00 UTC  → reminder_phase = "completed" (auto-complete)
```

### Example 2: Approval Required

**Scenario**: Production deployment needs approval 5 minutes before

```yaml
wants:
  - metadata:
      name: prod_deployment_approval
      type: reminder
    spec:
      params:
        message: "Approve production deployment for feature-xyz?"
        ahead: "5 minutes"
        event_time: "2025-12-29T15:30:00Z"
        require_reaction: true
        reaction_type: "internal"
```

**Timeline:**
```
15:25:00 UTC  → reminder_phase = "reaching"
              → reaction_id = "reminder-prod_deployment_approval-..." generated
              → Waiting for: POST /api/v1/reactions/{reaction_id}
```

**User submits approval:**
```bash
curl -X POST http://localhost:8080/api/v1/reactions/reminder-prod_deployment_approval-123 \
  -H "Content-Type: application/json" \
  -d '{
    "approved": true,
    "comment": "Approved by DevOps team"
  }'
```

**Result:**
```
15:26:00 UTC  → MonitorAgent detects reaction
              → user_reaction state updated
              → reaction_result = "approved"
              → reminder_phase = "completed"
```

### Example 3: Advance Warning (Long Ahead)

**Scenario**: Notify 1 hour before system maintenance

```yaml
wants:
  - metadata:
      name: system_maintenance_alert
      type: reminder
    spec:
      params:
        message: "System maintenance in 1 hour - save your work"
        ahead: "1 hours"
        event_time: "2025-12-29T22:00:00Z"
        require_reaction: false
```

**Timeline:**
```
21:00:00 UTC  → reminder_phase = "reaching" (long notification window)
21:00-22:00   → Users can prepare
22:00:00 UTC  → reminder_phase = "completed"
```

### Example 4: Recurring Daily with Reaction

**Scenario**: Daily security audit report approval

```yaml
wants:
  - metadata:
      name: daily_audit_approval
      type: reminder
    spec:
      params:
        message: "Approve today's security audit report"
        ahead: "30 minutes"
        require_reaction: true
        reaction_type: "internal"
      when:
        - at: "5pm"
          every: "day"
```

**Timeline (Daily):**
```
16:30:00 UTC  → reminder_phase = "reaching" (every day at 4:30 PM)
16:30-17:05   → Waiting for approval (30min timeout)
17:05:00 UTC  → If approved: → "completed"
              → If rejected: → "failed"
              → If timeout: → "failed"
17:00:00 UTC  → Next day, new reminder instance created
```

### Example 5: Immediate Alert (Minimal Ahead)

**Scenario**: Emergency alert with minimal advance notice

```yaml
wants:
  - metadata:
      name: emergency_alert
      type: reminder
    spec:
      params:
        message: "CRITICAL: Database connection pool exhausted"
        ahead: "10 seconds"
        event_time: "2025-12-28T21:35:30Z"
        require_reaction: true
```

**Timeline:**
```
21:35:20 UTC  → reminder_phase = "reaching" (minimal lead time)
21:35:20-21:35:25 → Quick reaction window
21:35:30 UTC  → Timeout if no reaction
```

### Example 6: Complex Scenario - Multi-Step Approval

**Scenario**: Staged approval process with multiple reminders

```yaml
wants:
  # Initial reminder 1 hour before
  - metadata:
      name: deployment_approval_1h
      type: reminder
    spec:
      params:
        message: "Deployment window opens in 1 hour - begin review"
        ahead: "1 hours"
        event_time: "2025-12-29T10:00:00Z"
        require_reaction: false

  # Final approval 5 minutes before
  - metadata:
      name: deployment_approval_5m
      type: reminder
    spec:
      params:
        message: "Final approval required to proceed with deployment"
        ahead: "5 minutes"
        event_time: "2025-12-29T10:00:00Z"
        require_reaction: true
        reaction_type: "internal"
```

**Timeline:**
```
09:00:00 UTC  → First reminder (reaching) - review begins
              → reminder_phase = "completed" (auto-complete)

09:55:00 UTC  → Second reminder (reaching) - approval needed
              → Waiting for: POST /api/v1/reactions/{reaction_id}

10:00:00 UTC  → If approved: deployment proceeds → "completed"
              → If rejected/timeout: deployment blocked → "failed"
```

## State Transitions

### Happy Path (No Reaction)
```
waiting → reaching → completed
```

### Approval Path (Requires Reaction)
```
waiting → reaching → [awaiting reaction]
                          ↓
                   ┌─────────────────┐
                   ↓                 ↓
              approved           rejected/timeout
                   ↓                 ↓
              completed           failed
```

## Error Handling

### Missing Required Parameters
- **Error**: "Missing required parameter 'message'"
- **Result**: reminder_phase = "failed", error_message populated

### Invalid ahead Format
- **Error**: "Invalid ahead parameter 'invalid duration'"
- **Result**: reminder_phase = "failed"
- **Valid Formats**: `\d+ (second|minute|hour|day)s?`

### Missing event_time and when
- **Error**: "Either 'event_time' or 'when' spec must be provided"
- **Result**: reminder_phase = "failed"

### Reaction Timeout
- **Behavior**: If require_reaction=true and no reaction within 5 minutes
- **Result**: reminder_phase = "failed", timeout = true

## Best Practices

1. **Choose ahead based on importance**
   - Emergency: `"10 seconds"`
   - Normal tasks: `"5 minutes"`
   - Important events: `"30 minutes"`
   - Day planning: `"1 hours"` or `"1 days"`

2. **Use require_reaction for critical decisions**
   - Deployment approvals: `require_reaction: true`
   - FYI notifications: `require_reaction: false`

3. **Set realistic reaction timeouts**
   - Default: 5 minutes
   - For critical: Plan ahead with longer `ahead` value

4. **Use meaningful reaction_ids**
   - Auto-generated: `reminder-{name}-{timestamp}`
   - Use descriptive want names for easy tracking

5. **Leverage recurring reminders**
   - Use `when` spec for daily/weekly patterns
   - Reduces manual reminder creation overhead

## Integration Examples

### with Coordinator Want
```yaml
wants:
  # Approval reminder
  - metadata:
      name: approval_request
      type: reminder
    spec:
      params:
        message: "Waiting for approval"
        ahead: "5 minutes"
        event_time: "2025-12-29T10:00:00Z"
        require_reaction: true

  # Orchestrate based on reaction
  - metadata:
      name: approval_coordinator
      type: coordinator
    spec:
      params:
        coordinator_type: "sequential"
```

## Monitoring and Debugging

### List all pending reactions
```bash
curl http://localhost:8080/api/v1/reactions
```

### Get reminder status
```bash
curl http://localhost:8080/api/v1/wants/{want-id}
```

### Check state progression
```bash
# Monitor reminder_phase changes
# Check reaching_time vs event_time calculations
# Verify reaction_id tracking
```

## Limitations and Future Work

### Current Limitations
- ReactionQueue is in-memory (lost on server restart)
- Reaction notifications are API-based only
- Single MonitorAgent polls all reminders

### Planned Enhancements (Phase 2)
- [ ] Persistent reaction storage (DB)
- [ ] Slack integration (reaction_type: "slack")
- [ ] Webhook integration (reaction_type: "webhook")
- [ ] Reaction history tracking
- [ ] Multiple ahead values (progressive notifications)
- [ ] Snooze functionality
- [ ] Escalation policies on timeout
