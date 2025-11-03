# Frontend Sample: Hierarchical Approval Workflow

This document provides a complete sample for deploying a hierarchical approval workflow via the MyWant API.

## Overview

The hierarchical approval workflow includes:
- **Evidence Want**: Provides evidence data for approval processes
- **Description Want**: Provides description/details for approval requests
- **Level 1 Approval Target**: Creates Level 1 Coordinator and Level 2 Approval as children
  - **Level 1 Coordinator**: Processes Level 1 approval
  - **Level 2 Approval Target**: Creates Level 2 Coordinator as child
    - **Level 2 Coordinator**: Processes Level 2 approval

## Deployment Steps

### Quick Start: Using YAML File (Recommended)

The easiest way to deploy is using the frontend sample YAML file:

```bash
curl -X POST "http://localhost:8080/api/v1/wants" \
  -H "Content-Type: application/yaml" \
  --data-binary @FRONTEND_SAMPLE_HIERARCHICAL_APPROVAL.yaml
```

### Step 1: Deploy Hierarchical Approval via API (JSON Format)

If you prefer JSON, send a POST request with the complete approval workflow:

```bash
curl -X POST "http://localhost:8080/api/v1/wants" \
  -H "Content-Type: application/json" \
  -d '{
  "wants": [
    {
      "metadata": {
        "name": "evidence",
        "type": "evidence",
        "labels": {
          "role": "evidence-provider",
          "category": "approval-data",
          "approval_id": "approval-001"
        }
      },
      "spec": {
        "params": {
          "evidence_type": "document",
          "approval_id": "approval-001"
        }
      }
    },
    {
      "metadata": {
        "name": "description",
        "type": "description",
        "labels": {
          "role": "description-provider",
          "category": "approval-data",
          "approval_id": "approval-001"
        }
      },
      "spec": {
        "params": {
          "description_format": "Request for approval: %s",
          "approval_id": "approval-001"
        }
      }
    },
    {
      "metadata": {
        "name": "level1_approval",
        "type": "level 1 approval",
        "labels": {
          "role": "approval-target",
          "approval_level": "1"
        }
      },
      "spec": {
        "params": {
          "approval_id": "approval-001",
          "coordinator_type": "level1",
          "level2_authority": "senior_manager"
        }
      }
    }
  ]
}'
```

### IMPORTANT: Evidence and Description are Required

The Level 1 Approval target **does NOT contain** evidence and description wants. These must be deployed separately as top-level wants because:

1. **Shared Data**: Evidence and description are used by both Level 1 and Level 2 coordinators
2. **Multi-Output Broadcasting**: The Evidence and Description wants use `MaxOutputs: -1` to broadcast the same data to multiple consumers
3. **Simplified Configuration**: Avoids duplication when multiple approval levels are needed

**You must deploy all three components together:**
- ✅ Evidence want
- ✅ Description want
- ✅ Level 1 Approval target (which automatically creates Level 2 Approval as a child)

## Response

The API will respond with:

```json
{
  "want_ids": [
    "want-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
    "want-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
    "want-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  ]
}
```

## Hierarchical Structure Created

```
evidence (Evidence Provider)
├─ Provides evidence data to both levels

description (Description Provider)
├─ Provides description to both levels

level1_approval (Level 1 Approval Target)
├── level1_approval-level1_coordinator-1 (Level 1 Coordinator)
│   └─ Processes: evidence + description → Level 1 approval decision
│
└── level1_approval-level 2 approval-2 (Level 2 Approval Target)
    └── level1_approval-level 2 approval-2-level2_coordinator-1 (Level 2 Coordinator)
        └─ Processes: evidence + description → Level 2 final approval decision
```

## Want Details

### Evidence Want
- **Type**: `evidence`
- **Purpose**: Provides supporting evidence for approval process
- **MaxOutputs**: Unlimited (broadcasts to multiple consumers)
- **Output**: Sends `ApprovalData` containing evidence information

```json
{
  "metadata": {
    "name": "evidence",
    "type": "evidence"
  },
  "spec": {
    "params": {
      "evidence_type": "document",
      "approval_id": "approval-001"
    }
  }
}
```

### Description Want
- **Type**: `description`
- **Purpose**: Provides description/details for the approval request
- **MaxOutputs**: Unlimited (broadcasts to multiple consumers)
- **Output**: Sends `ApprovalData` containing description information

```json
{
  "metadata": {
    "name": "description",
    "type": "description"
  },
  "spec": {
    "params": {
      "description_format": "Request for approval: %s",
      "approval_id": "approval-001"
    }
  }
}
```

### Level 1 Approval Target
- **Type**: `level 1 approval` (custom target type)
- **Purpose**: Orchestrates Level 1 and Level 2 approval process
- **Children Created**:
  - `level1_approval-level1_coordinator-1` (from `approval-level-1.yaml` recipe)
  - `level1_approval-level 2 approval-2` (nested target, also from recipe)

```json
{
  "metadata": {
    "name": "level1_approval",
    "type": "level 1 approval",
    "labels": {
      "role": "approval-target",
      "approval_level": "1"
    }
  },
  "spec": {
    "params": {
      "approval_id": "approval-001",
      "coordinator_type": "level1",
      "level2_authority": "senior_manager"
    }
  }
}
```

## Execution Flow

1. **Initialization**
   - Config loads evidence, description, and level1_approval wants
   - Level 1 Approval Target detects custom type and loads `approval-level-1.yaml` recipe

2. **Child Creation**
   - Level 1 recipe creates:
     - Level 1 Coordinator (standard want)
     - Level 2 Approval Target (custom target type, creates own children)

3. **Data Provider Execution**
   - Evidence want broadcasts evidence data to all coordinators
   - Description want broadcasts description to all coordinators

4. **Level 1 Approval**
   - Level 1 Coordinator receives evidence + description
   - Processes and approves (simulated)
   - Sends completion event to parent (level1_approval)

5. **Level 2 Approval Creation**
   - Level 2 Approval Target detects custom type
   - Loads `approval-level-2.yaml` recipe
   - Creates Level 2 Coordinator

6. **Level 2 Approval**
   - Level 2 Coordinator receives evidence + description (via multi-output broadcast)
   - Processes and approves (simulated)
   - Sends completion event to Level 2 Approval Target

7. **Hierarchical Completion**
   - Level 2 Approval Target receives Level 2 Coordinator completion
   - Sends its own completion event to Level 1 Approval (parent)
   - Level 1 Approval receives both child completions
   - Completes the hierarchical workflow

## Key Features

### Multi-Output Broadcasting
The Evidence and Description wants use `SendPacketMulti()` to broadcast the same data to multiple consumers (both Level 1 and Level 2 coordinators).

**Implementation**: `Want.SendPacketMulti(packet interface{}, outputs []Chan) error`

This allows:
- Single data providers to serve multiple consumers
- Eliminates need for duplicate data sources
- Reduced configuration complexity

### Owner-Based Parent-Child Coordination
- Child wants are automatically wrapped with `OwnerAwareWant`
- When a child completes, it emits `OwnerCompletionEvent`
- Parent tracks completion via subscription system
- Nested targets properly propagate completion events up the hierarchy

### Custom Target Types with Recipes
- `level 1 approval` and `level 2 approval` are custom target types
- Each target loads its recipe (`approval-level-1.yaml`, `approval-level-2.yaml`)
- Recipes define child wants and coordinators
- Nested targets can have their own children and recipes

## Monitoring

### Check Want Status
```bash
curl "http://localhost:8080/api/v1/wants" | jq '.wants[] | {name: .metadata.name, status: .status, type: .metadata.type}'
```

### Check Specific Want
```bash
curl "http://localhost:8080/api/v1/wants/{want-id}" | jq '.'
```

### Check Want State History
```bash
curl "http://localhost:8080/api/v1/wants/{want-id}" | jq '.state'
```

## Testing with curl

### Complete Test Sequence

```bash
# 1. Create the hierarchical approval workflow
RESPONSE=$(curl -s -X POST "http://localhost:8080/api/v1/wants" \
  -H "Content-Type: application/json" \
  -d '{
  "wants": [
    {
      "metadata": {
        "name": "evidence",
        "type": "evidence",
        "labels": {
          "role": "evidence-provider",
          "category": "approval-data",
          "approval_id": "approval-001"
        }
      },
      "spec": {
        "params": {
          "evidence_type": "document",
          "approval_id": "approval-001"
        }
      }
    },
    {
      "metadata": {
        "name": "description",
        "type": "description",
        "labels": {
          "role": "description-provider",
          "category": "approval-data",
          "approval_id": "approval-001"
        }
      },
      "spec": {
        "params": {
          "description_format": "Request for approval: %s",
          "approval_id": "approval-001"
        }
      }
    },
    {
      "metadata": {
        "name": "level1_approval",
        "type": "level 1 approval",
        "labels": {
          "role": "approval-target",
          "approval_level": "1"
        }
      },
      "spec": {
        "params": {
          "approval_id": "approval-001",
          "coordinator_type": "level1",
          "level2_authority": "senior_manager"
        }
      }
    }
  ]
}')

echo "Created wants:"
echo "$RESPONSE" | jq '.want_ids'

# 2. Wait for execution to complete
echo "Waiting for execution..."
sleep 3

# 3. Check all wants
echo -e "\nAll wants:"
curl -s "http://localhost:8080/api/v1/wants" | jq '.wants[] | {name: .metadata.name, status: .status, type: .metadata.type}'

# 4. Check Level 1 approval status
echo -e "\nLevel 1 Approval State:"
LEVEL1_ID=$(curl -s "http://localhost:8080/api/v1/wants" | jq -r '.wants[] | select(.metadata.name == "level1_approval") | .metadata.id')
curl -s "http://localhost:8080/api/v1/wants/$LEVEL1_ID" | jq '.state'
```

## Notes

- Evidence and Description wants use `MaxOutputs: -1` (unlimited) to support multi-output broadcasting
- All child wants automatically receive owner references pointing to their parent
- The hierarchical structure is created dynamically based on recipes
- Completion events propagate up the hierarchy automatically via the subscription system
