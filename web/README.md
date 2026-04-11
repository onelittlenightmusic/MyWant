# MyWant Frontend Documentation

A modern React-based Single Page Application (SPA) dashboard for managing MyWant configurations and executions. This frontend provides a rich, interactive interface for monitoring autonomous agents and want-driven workflows.

## Table of Contents
1. [Overall Architecture](#overall-architecture)
2. [Screen Configuration](#screen-configuration)
3. [File and Directory Breakdown](#file-and-directory-breakdown)
4. [State Management](#state-management)
5. [Key Components](#key-components)
6. [Styling and UI Patterns](#styling-and-ui-patterns)

---

## Overall Architecture

The MyWant frontend is built with a modern, high-performance stack:

- **React 18**: Utilizing functional components and hooks.
- **TypeScript**: Ensuring type safety across the entire codebase.
- **Vite**: Providing lightning-fast build times and HMR.
- **Tailwind CSS**: Utility-first styling for consistent and responsive design.
- **Zustand**: Lightweight and scalable state management.
- **React Router 6**: For declarative routing.
- **CodeMirror 6**: Powering the advanced YAML editing experience.
- **Lucide React**: For a consistent and modern icon set.

### Core Philosophy
- **Real-time Visibility**: Data is polled and updated live to reflect the backend engine's state.
- **Interactivity**: Extensive support for drag-and-drop, hierarchical navigation, and contextual actions.
- **Responsive**: Adaptive layout supporting everything from mobile phones (iPhone) to large desktop monitors.
- **Compactness**: High information density through optimized typography and layout.

---

## Screen Configuration

The application is divided into several main functional areas:

### 1. Main Dashboard (`/dashboard`)
The central hub of the application.
- **Want Grid**: Displays all "Wants" as interactive cards.
- **Want Children Bubble**: An inline "speech bubble" that appears when a Target Want is selected, showing its hierarchical children.
- **Minimap**: A floating, high-level overview of the grid for quick navigation.
- **Batch Action Bar**: Appears during multi-select mode to perform operations on multiple Wants at once.

### 2. Sidebars (Right Panel)
A unified sidebar system managed with mutual exclusivity:
- **Want Details**: Deep dive into a specific Want's parameters, results, logs, and agent activity.
- **Memo (Global State)**: A shared "whiteboard" where Wants and users can store and view persistent global parameters and state.
- **Summary/Stats**: High-level statistics, labels, and filtering tools.
- **Want Form**: The creation and editing interface for new Wants and Whims.

### 3. Specialty Pages
- **Agents (`/agents`)**: Management and monitoring of autonomous agent workers.
- **Want Types (`/want-types`)**: Browser for available Want Type definitions and their schemas.
- **Recipes (`/recipes`)**: Management of reusable Want configuration templates.
- **Achievements (`/achievements`)**: Visual record of completed goals and milestones.
- **Logs (`/logs`)**: System-wide log viewer.

---

## File and Directory Breakdown

### `src/pages/` (Main View Controllers)
- `Dashboard.tsx`: Orchestrates the main grid, sidebar exclusivity, and global keyboard shortcuts.
- `AgentsPage.tsx`: Specialized view for agent worker status.
- `WantTypePage.tsx`: Catalog of capabilities available to the system.
- `RecipePage.tsx`: Interface for managing reusable YAML templates.
- `AchievementsPage.tsx`: Gallery of success states.

### `src/components/layout/` (The Frame)
- `Header.tsx`: Contains the main navigation, global search, "Ask" (agent interact), and primary action buttons.
- `Sidebar.tsx`: The left-hand navigation rail.
- `RightSidebar.tsx`: The flexible container for all side-panel content, including mobile "bottom sheet" behavior.
- `HeaderOverlay.tsx`: Contextual bars (like Batch Actions) that slide in from the top.

### `src/components/dashboard/` (Grid Logic)
- `WantGrid.tsx`: Manages the responsive grid layout and row-based bubble insertion.
- `WantCard/`: A modular set of components representing a single Want.
    - `WantCard.tsx`: The main card container with drag-and-drop and selection logic.
    - `WantCardContent.tsx`: The internal visual representation, including status-specific UI.
- `WantChildrenBubble.tsx`: Hierarchical visualization logic for child Wants.
- `WantMinimap.tsx`: Navigation tool for large grids.

### `src/components/sidebar/` (Panel Content)
- `WantDetailsSidebar.tsx`: The most complex sidebar, featuring multi-tab information about a single Want.
- `GlobalStateSidebar.tsx`: Implementation of the "Memo" feature.
- `DetailsSidebar.tsx`: A common layout component providing a consistent header and tabbed interface for all sidebars.

### `src/components/forms/` (Inputs)
- `WantForm.tsx`: Large, schema-aware form for creating/editing configurations.
- `YamlEditor.tsx`: Wrapper for CodeMirror with YAML-specific enhancements.

### `src/stores/` (Global State)
- `wantStore.ts`: Primary store for all Want data, status, and CRUD operations.
- `configStore.ts`: System-wide settings (e.g., header position).
- `uiStore.ts`: Ephemeral UI state like notifications.
- `wantHashCache.ts`: Optimization layer for ETag-based polling to minimize unnecessary re-renders.

---

## State Management

MyWant uses **Zustand** for state management, organized by domain:

| Store | Responsibility |
|-------|----------------|
| `wantStore` | Fetches and caches the list of Wants, handles start/stop/delete commands. |
| `agentStore` | Tracks active agent workers and their statistics. |
| `recipeStore` | Manages templates and custom types. |
| `configStore` | Persistent user preferences (synced with localStorage). |
| `uiStore` | Global UI states like the "Select Mode" and toast notifications. |

### Exclusivity Hook
`src/hooks/useRightSidebarExclusivity.ts` is a critical piece of logic that ensures the UI doesn't become cluttered by managing the transition between Details, Memo, Summary, and Form panels.

---

## Key Components

### WantCard
The primary visual unit of the application. It changes its appearance based on:
- **Status**: Colored borders and pulse animations (`pulseGlow`).
- **Type**: `user-control: true` cards appear as flat UI controls (sliders/toggles) when selected.
- **Selection**: Highlights and focus triangles indicate keyboard and mouse focus.
- **Progress**: A background progress bar tracks achievement percentage.

### WantChildrenBubble
A distinctive UI pattern that visualizes parent-child relationships.
- It appears directly below the row of the parent card.
- Supports recursive nesting (bubbles inside bubbles).
- Allows drag-and-drop targeting to "adopt" Wants into the hierarchy.

### RightSidebar
An adaptive container:
- **Desktop**: Slides in from the right, pushing or overlaying the content.
- **Mobile**: Transforms into a "Bottom Sheet" covering 50% of the viewport.
- **Non-blocking**: When configured, it allows the background grid to remain interactive while open.

---

## Styling and UI Patterns

### Standard Button Style
Most primary action buttons (Header, Sidebar Close, Refresh) follow a consistent pattern:
- `flex flex-col items-center justify-center`
- Icon size: `h-4 w-4`
- Label: `text-[9px] font-bold uppercase tracking-tighter`
- Height: `h-full` (flush with container edges)

### Responsive Breakpoints
- **Mobile (< 640px)**: 1-column grid, Bottom Sheet sidebars, compact buttons.
- **Tablet (< 1024px)**: 2-column grid, overlay sidebars.
- **Desktop (>= 1024px)**: 3-column grid, persistent/pushing sidebars, full labels.

### Interactive Feedback
- **Drag-and-Drop**: Visual "drop indicators" appear between cards. Targets (like Bubbles) highlight when hovered with a draggable item.
- **Blinking**: The Minimap and cards can "blink" to draw attention to newly selected or located items.
- **Keyboard Navigation**: Full support for arrow keys, Enter (select), and Escape (close/back).
