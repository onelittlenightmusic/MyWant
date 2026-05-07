import { useInputActions } from './useInputActions';

export interface HierarchicalItem {
  id: string;
  parentId?: string;
}

interface UseHierarchicalKeyboardNavigationProps<T extends HierarchicalItem> {
  items: T[];
  currentItem: T | null;
  onNavigate: (item: T) => void;
  onToggleExpand?: (itemId: string) => void;
  onSelect?: (itemId: string) => void;
  expandedItems?: Set<string>;
  lastSelectedItemId?: string | null;
  enabled?: boolean;
}

/**
 * Hook for hierarchical keyboard / gamepad navigation.
 *
 * Arrow key / D-pad / left-stick mapping:
 *   Right  – if parent is expanded move to first child; else move to next top-level
 *   Left   – move to previous sibling; if first child, move to parent; if top-level, move to previous top-level
 *   Down   – next top-level item
 *   Up     – previous top-level item
 *   Space / Gamepad X – toggle expand/collapse (or call onSelect when provided)
 *   Home   – first item
 *   End    – last item
 */
export const useHierarchicalKeyboardNavigation = <T extends HierarchicalItem>({
  items,
  currentItem,
  onNavigate,
  onToggleExpand,
  onSelect,
  expandedItems,
  lastSelectedItemId,
  enabled = true
}: UseHierarchicalKeyboardNavigationProps<T>) => {
  useInputActions({
    enabled: enabled && items.length > 0,

    onNavigate: (dir) => {
      // Determine the reference item for navigation (current or last selected)
      let refItem: T | null = currentItem;
      if (!refItem && lastSelectedItemId) {
        refItem = items.find(i => i.id === lastSelectedItemId) ?? null;
      }

      // If no reference, default to first / last item
      if (!refItem) {
        if (dir === 'up') {
          onNavigate(items[items.length - 1]);
        } else {
          onNavigate(items[0]);
        }
        return;
      }

      let nextItem: T | null = null;

      switch (dir) {
        case 'right': {
          if (!refItem.parentId) {
            const hasChildren = items.some(i => i.parentId === refItem!.id);
            const isExpanded = expandedItems?.has(refItem!.id);
            nextItem = (hasChildren && isExpanded)
              ? getFirstChild(items, refItem)
              : getNextTopLevel(items, refItem);
          } else {
            nextItem = getNextSibling(items, refItem);
            if (!nextItem) {
              const parent = getParent(items, refItem);
              if (parent) nextItem = getNextSibling(items, parent);
            }
          }
          break;
        }
        case 'left': {
          if (refItem.parentId) {
            nextItem = getPreviousSibling(items, refItem) ?? getParent(items, refItem);
          } else {
            nextItem = getPreviousTopLevel(items, refItem);
          }
          break;
        }
        case 'down':
          nextItem = getNextTopLevel(items, refItem);
          break;
        case 'up':
          nextItem = getPreviousTopLevel(items, refItem);
          break;
        case 'home':
          nextItem = items[0] ?? null;
          break;
        case 'end':
          nextItem = items[items.length - 1] ?? null;
          break;
      }

      if (!nextItem) return;
      onNavigate(nextItem);

      const targetId = nextItem.id;
      requestAnimationFrame(() => {
        setTimeout(() => {
          const el = document.querySelector(`[data-keyboard-nav-id="${targetId}"]`);
          if (el instanceof HTMLElement) {
            el.focus();
            el.scrollIntoView({ behavior: 'smooth', block: 'center' });
          }
        }, 0);
      });
    },

    onToggle: () => {
      if (!currentItem) return;
      if (onSelect) {
        onSelect(currentItem.id);
      } else {
        const hasChildren = items.some(i => i.parentId === currentItem.id);
        if (hasChildren && onToggleExpand) {
          onToggleExpand(currentItem.id);
        }
      }
    },
  });
};

// ─── Hierarchy helpers ────────────────────────────────────────────────────────

function getSiblings<T extends HierarchicalItem>(items: T[], item: T): T[] {
  return items.filter(i => i.parentId === item.parentId);
}

function getNextSibling<T extends HierarchicalItem>(items: T[], item: T | null): T | null {
  if (!item) return items[0] ?? null;
  const siblings = getSiblings(items, item);
  const idx = siblings.findIndex(s => s.id === item.id);
  return idx < siblings.length - 1 ? siblings[idx + 1] : null;
}

function getPreviousSibling<T extends HierarchicalItem>(items: T[], item: T | null): T | null {
  if (!item) return null;
  const siblings = getSiblings(items, item);
  const idx = siblings.findIndex(s => s.id === item.id);
  return idx > 0 ? siblings[idx - 1] : null;
}

function getFirstChild<T extends HierarchicalItem>(items: T[], item: T): T | null {
  return items.find(i => i.parentId === item.id) ?? null;
}

function getParent<T extends HierarchicalItem>(items: T[], item: T): T | null {
  if (!item.parentId) return null;
  return items.find(i => i.id === item.parentId) ?? null;
}

function getTopLevelItems<T extends HierarchicalItem>(items: T[]): T[] {
  return items.filter(i => !i.parentId);
}

function getNextTopLevel<T extends HierarchicalItem>(items: T[], item: T | null): T | null {
  if (!item) return null;
  let ref = item.parentId ? (getParent(items, item) ?? item) : item;
  const topLevel = getTopLevelItems(items);
  const idx = topLevel.findIndex(i => i.id === ref.id);
  return idx < topLevel.length - 1 ? topLevel[idx + 1] : null;
}

function getPreviousTopLevel<T extends HierarchicalItem>(items: T[], item: T | null): T | null {
  if (!item) return null;
  let ref = item.parentId ? (getParent(items, item) ?? item) : item;
  const topLevel = getTopLevelItems(items);
  const idx = topLevel.findIndex(i => i.id === ref.id);
  return idx > 0 ? topLevel[idx - 1] : null;
}
