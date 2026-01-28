# Keyboard Shortcuts & MCP Testing Guide

This document outlines the keyboard shortcuts available in the application and provides guidance on how to use them for automated testing via Chrome MCP.

## Environment Details
- **Frontend URL:** `http://localhost:8080`
- **Backend API:** `http://localhost:8080` (Proxied via `/api` on frontend)

## Application Shortcuts

## Dashboard Modes

The dashboard operates in different modes that affect shortcut availability:

- **Normal Mode**: Default mode with all shortcuts enabled
- **Select Mode** (`Shift + S`): Batch operations enabled, all shortcuts available including `Ctrl+A` for select all
- **Add Mode** (New Want form open): **All dashboard shortcuts are DISABLED** - only form-specific shortcuts work
- **Summary Mode**: Summary panel open, all dashboard shortcuts remain enabled

### Global (Dashboard)
**Note**: All dashboard shortcuts below are **disabled** when in Add Mode (New Want form is open).

| Key | Action | Description |
| :--- | :--- | :--- |
| `a` | Add Want | Opens the "New Want" form sidebar. Disabled in Add Mode. |
| `s` | Toggle Summary | Opens/Closes the Summary sidebar. Disabled in Add Mode. |
| `Shift + S` | Toggle Select Mode | Enters/Exits batch selection mode. Disabled in Add Mode. |
| `Ctrl + A` (or `Cmd + A`) | Select All | In Select Mode: selects all visible wants for batch operations. Disabled in Add Mode. |
| `q` | Focus Suggestion Input | Focuses the Suggestion textbox to enter natural language requests. Disabled in Add Mode. |
| `Arrow Keys` | Navigate Wants | Navigate through want cards. Disabled in Add Mode. |
| `Escape` | Close/Cancel | Unfocuses Suggestion input if focused, otherwise closes open sidebars (Form, Summary, Details) or exits Select Mode. |

### Want Form (Sidebar)
| Key | Action | Description |
| :--- | :--- | :--- |
| `/` | Focus Search | Focuses the "Type/Recipe Selector" search input. |
| `ArrowDown` | Next Section | Moves focus to the next form section (e.g., from Name to Parameters). |
| `ArrowUp` | Previous Section | Moves focus to the previous form section. |
| `Enter` | Submit | Submits the form (when focused on an input). |

### Type/Recipe Selector (Dropdown)
| Key | Action | Description |
| :--- | :--- | :--- |
| `ArrowDown` / `ArrowUp` | Navigate | Moves selection up/down in the list. |
| `Enter` | Select | Confirms the currently highlighted item. |
| `Delete` / `Backspace` | Clear | Clears the current selection (when collapsed/selected). |

### Form Sections (Parameters, Labels, etc.)
| Key | Action | Description |
| :--- | :--- | :--- |
| `ArrowRight` | Expand & Focus | Expands the section and moves focus to the first input field. |
| `ArrowLeft` | Collapse | Collapses the section. |
| `Escape` | Exit Section | Moves focus from an input field back to the section header. |

---

## MCP Automated Testing

Using keyboard shortcuts in MCP is often faster and more reliable than selector-based interactions for complex UI sequences.

### Use Case: Add Want -> Search "Command" -> Deploy

**Goal:** Open the "Add Want" sidebar, search for the "Command" want type, configure a parameter, and deploy it.

**Step-by-Step Sequence:**

1.  **Open Form:** Press `a` to open the sidebar.
2.  **Search:** Press `/` to focus the search bar.
3.  **Type Query:** Input the string "Command".
4.  **Select Type:**
    *   Wait briefly for results.
    *   Press `ArrowDown` to highlight the first result.
    *   Press `Enter` to select it.
5.  **Navigate to Parameters:**
    *   Press `ArrowDown` (Focus moves to Name).
    *   Press `ArrowDown` (Focus moves to Parameters Header).
6.  **Edit Parameter:**
    *   Press `ArrowRight` (Expands section and focuses first parameter input).
    *   Type the parameter value (e.g., `echo "Hello World"`).
7.  **Deploy:** Press `Enter` to submit the form.

### MCP Implementation Example

To execute this sequence using the `press_key` and `fill` tools in MCP:

```javascript
// 1. Open the form
await use_mcp_tool("press_key", { key: "a" });

// 2. Focus search
await use_mcp_tool("press_key", { key: "/" });

// 3. Type "Command" into the search field
// Note: Since focus is already set by '/', we can potentially use 'press_key' for typing characters
// or use 'fill' if we can target the active element.
// For pure keyboard simulation without selectors, we can send keys:
await use_mcp_tool("press_key", { key: "C" });
await use_mcp_tool("press_key", { key: "o" });
await use_mcp_tool("press_key", { key: "m" });
await use_mcp_tool("press_key", { key: "m" });
await use_mcp_tool("press_key", { key: "a" });
await use_mcp_tool("press_key", { key: "n" });
await use_mcp_tool("press_key", { key: "d" });

// Alternatively, if 'fill' is preferred and we know the UID of the focused element (search input):
// await use_mcp_tool("fill", { uid: "search-input-uid", value: "Command" });

// 4. Select the first result
await use_mcp_tool("press_key", { key: "ArrowDown" });
await use_mcp_tool("press_key", { key: "Enter" });

// 5. Navigate to Parameters section
// Focus is now likely on the Type Selector or Name input.
// Press ArrowDown to move to Name
await use_mcp_tool("press_key", { key: "ArrowDown" });
// Press ArrowDown to move to Parameters Header
await use_mcp_tool("press_key", { key: "ArrowDown" });

// 6. Enter Parameter
// Expand section and focus first input
await use_mcp_tool("press_key", { key: "ArrowRight" });

// Type the command (e.g., 'ls -la')
// Again, can use individual keys or 'fill' if UID is known.
// Using keys for "ls":
await use_mcp_tool("press_key", { key: "l" });
await use_mcp_tool("press_key", { key: "s" });

// 7. Deploy
await use_mcp_tool("press_key", { key: "Enter" });
```

### Best Practices for MCP Shortcuts

1.  **Wait for Animations:** UI transitions (like sidebar opening) take time. While `press_key` is instant, the UI might lag. It's safe to add small delays or checks (e.g., `wait_for`) between critical steps.
2.  **Verify State:** After major actions (like opening the form), verify the UI state (e.g., check for "New Want" text) before proceeding.
3.  **Use `press_key` for Navigation:** Rely on `Arrow` keys and `Enter` for navigation instead of trying to find dynamic UIDs for every list item.
4.  **Use `fill` for Text:** For long text inputs, finding the element UID and using `fill` is much faster and less error-prone than simulating individual keystrokes.

---

## Automated Test Scripts

The repository includes two Playwright-based scripts in the `test/` directory to verify the deployment flow.

### Prerequisites

Ensure you have Playwright installed in the project root:
```bash
npm install -D playwright
npx playwright install chromium
```

### 1. UI-based Deployment Test
This script simulates a standard user interaction using mouse clicks and explicit locators.

- **File:** `test/deploy_execution_result.mjs`
- **Action:** Navigates the dashboard, searches for "Command", expands sections manually, and fills the form.
- **Run:**
  ```bash
  node test/deploy_execution_result.mjs
  ```

### 2. Shortcut-based Deployment Test
This script simulates a "power user" flow relying entirely on keyboard shortcuts as described in this guide.

- **File:** `test/deploy_via_shortcuts.mjs`
- **Action:** Uses `a`, `/`, `ArrowDown`, `ArrowRight`, and `Enter` to complete the deployment without clicking buttons.
- **Run:**
  ```bash
  node test/deploy_via_shortcuts.mjs
  ```
