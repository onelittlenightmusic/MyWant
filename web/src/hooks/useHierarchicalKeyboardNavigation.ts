import { useEffect } from 'react';

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
 * Hook for hierarchical keyboard navigation with arrow keys and space
 * - Arrow Right: If parent expanded, move to first child; else move to next sibling or next top-level
 * - Arrow Left: Move to previous sibling; if first child, move to parent; if top-level, move to previous top-level
 * - Arrow Up: Move to previous top-level item
 * - Arrow Down: Move to next top-level item
 * - Space: Toggle expand/collapse on current parent item (only works on parents with children)
 *
 * @param items - All items in flat list
 * @param currentItem - Currently selected item
 * @param onNavigate - Callback when navigation occurs with new item
 * @param onToggleExpand - Callback to toggle expand/collapse
 * @param expandedItems - Set of expanded parent IDs for checking expansion state
 * @param enabled - Whether keyboard navigation is enabled (default: true)
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
  useEffect(() => {
    if (!enabled || items.length === 0) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      // Don't intercept if user is typing in an input
      const target = e.target as HTMLElement;
      const isInputElement =
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable;

      if (isInputElement) return;

      // Don't intercept if focus is inside a sidebar
      if (target.closest('[data-sidebar="true"]')) return;

      let nextItem: T | null = null;
      let shouldNavigate = false;

      // Determine the reference item for navigation (current or last selected)
      let refItem = currentItem;
      if (!refItem && lastSelectedItemId && items.length > 0) {
        refItem = items.find(item => item.id === lastSelectedItemId) || null;
      }

      switch (e.key) {
        case 'ArrowRight':
          if (typeof e.preventDefault === 'function') e.preventDefault();
          if (refItem) {
            if (!refItem.parentId) {
              // Current item is top-level (parent)
              const hasChildren = items.some(item => item.parentId === refItem!.id);
              const isExpanded = expandedItems?.has(refItem!.id);

              if (hasChildren && isExpanded) {
                // Parent is expanded, move to first child
                nextItem = getFirstChild(items, refItem);
              } else {
                // Parent is not expanded or has no children, move to next top-level
                nextItem = getNextTopLevel(items, refItem);
              }
            } else {
              // Current item is a child, move to next sibling
              nextItem = getNextSibling(items, refItem);
              if (!nextItem) {
                // No next sibling, move to next parent's sibling
                const parent = getParent(items, refItem);
                if (parent) {
                  nextItem = getNextSibling(items, parent);
                }
              }
            }
          } else if (items.length > 0) {
            // No current item, start with first item
            nextItem = items[0];
          }
          shouldNavigate = !!nextItem;
          break;

        case 'ArrowLeft':
          if (typeof e.preventDefault === 'function') e.preventDefault();
          if (refItem) {
            if (refItem.parentId) {
              // Current item is a child
              nextItem = getPreviousSibling(items, refItem);
              if (!nextItem) {
                // No previous sibling, move to parent
                nextItem = getParent(items, refItem);
              }
            } else {
              // Current item is top-level, move to previous top-level item
              nextItem = getPreviousTopLevel(items, refItem);
            }
          } else if (items.length > 0) {
            // No current item, start with first item (or could be last)
            nextItem = items[0];
          }
          shouldNavigate = !!nextItem;
          break;

        case 'ArrowDown':
          if (typeof e.preventDefault === 'function') e.preventDefault();
          // Down arrow: navigate to next top-level want
          nextItem = getNextTopLevel(items, refItem);
          if (!nextItem && !refItem && items.length > 0) {
            // Fallback for first press
            nextItem = getTopLevelItems(items)[0];
          }
          shouldNavigate = !!nextItem;
          break;

        case 'ArrowUp':
          if (typeof e.preventDefault === 'function') e.preventDefault();
          // Up arrow: navigate to previous top-level want
          nextItem = getPreviousTopLevel(items, refItem);
          if (!nextItem && !refItem && items.length > 0) {
            // Fallback for first press
            const topLevel = getTopLevelItems(items);
            nextItem = topLevel[topLevel.length - 1];
          }
          shouldNavigate = !!nextItem;
          break;

        case ' ':
          e.preventDefault();
          // Space: Toggle selection if onSelect provided, else toggle expand/collapse
          if (currentItem) {
            if (onSelect) {
              onSelect(currentItem.id);
            } else {
              // Check if current item is a parent (has children)
              const hasChildren = items.some(item => item.parentId === currentItem.id);
              if (hasChildren && onToggleExpand) {
                onToggleExpand(currentItem.id);
              }
            }
          }
          return;  // Don't navigate, just toggle

        case 'Home':
          e.preventDefault();
          if (items.length > 0) {
            nextItem = items[0];
            shouldNavigate = true;
          }
          break;

        case 'End':
          e.preventDefault();
          if (items.length > 0) {
            nextItem = items[items.length - 1];
            shouldNavigate = true;
          }
          break;

        default:
          return;
      }

      if (shouldNavigate && nextItem) {
        onNavigate(nextItem);

        // Use requestAnimationFrame and a small timeout to ensure React has fully updated the DOM
        // before attempting to scroll. This prevents animation artifacts.
        requestAnimationFrame(() => {
          setTimeout(() => {
            const selectedElement = document.querySelector('[data-keyboard-nav-selected="true"]');
            if (selectedElement && selectedElement instanceof HTMLElement) {
              selectedElement.scrollIntoView({ behavior: 'smooth', block: 'center' });
            }
          }, 0);
        });
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [items, currentItem, onNavigate, onToggleExpand, expandedItems, lastSelectedItemId, enabled]);
};

// Helper functions for hierarchical navigation

/**
 * Get all siblings of an item (items at same parent level)
 */
function getSiblings<T extends HierarchicalItem>(items: T[], item: T | null): T[] {
  if (!item) return [];
  const parentId = item.parentId;
  return items.filter(i => i.parentId === parentId);
}

/**
 * Get the next sibling item
 */
function getNextSibling<T extends HierarchicalItem>(items: T[], item: T | null): T | null {
  if (!item) return items.length > 0 ? items[0] : null;

  const siblings = getSiblings(items, item);
  if (siblings.length === 0) return null;

  const currentIndex = siblings.findIndex(s => s.id === item.id);
  if (currentIndex < siblings.length - 1) {
    return siblings[currentIndex + 1];
  }
  return null;
}

/**
 * Get the previous sibling item
 */
function getPreviousSibling<T extends HierarchicalItem>(items: T[], item: T | null): T | null {
  if (!item) return null;

  const siblings = getSiblings(items, item);
  if (siblings.length === 0) return null;

  const currentIndex = siblings.findIndex(s => s.id === item.id);
  if (currentIndex > 0) {
    return siblings[currentIndex - 1];
  }
  return null;
}

/**
 * Get the first child of an item
 */
function getFirstChild<T extends HierarchicalItem>(items: T[], item: T): T | null {
  return items.find(i => i.parentId === item.id) || null;
}

/**
 * Get the parent of an item
 */
function getParent<T extends HierarchicalItem>(items: T[], item: T): T | null {
  if (!item.parentId) return null;
  return items.find(i => i.id === item.parentId) || null;
}

/**
 * Get all top-level items (items without a parent)
 */
function getTopLevelItems<T extends HierarchicalItem>(items: T[]): T[] {
  return items.filter(i => !i.parentId);
}

/**
 * Get the next top-level item
 */
function getNextTopLevel<T extends HierarchicalItem>(items: T[], item: T | null): T | null {
  if (!item) return null;

  // If current item is not top-level, find its parent
  let topLevelItem = item;
  if (item.parentId) {
    topLevelItem = getParent(items, item) || item;
  }

  const topLevelItems = getTopLevelItems(items);
  const currentIndex = topLevelItems.findIndex(i => i.id === topLevelItem.id);

  if (currentIndex < topLevelItems.length - 1) {
    return topLevelItems[currentIndex + 1];
  }
  return null;
}

/**
 * Get the previous top-level item
 */
function getPreviousTopLevel<T extends HierarchicalItem>(items: T[], item: T | null): T | null {
  if (!item) return null;

  // If current item is not top-level, find its parent
  let topLevelItem = item;
  if (item.parentId) {
    topLevelItem = getParent(items, item) || item;
  }

  const topLevelItems = getTopLevelItems(items);
  const currentIndex = topLevelItems.findIndex(i => i.id === topLevelItem.id);

  if (currentIndex > 0) {
    return topLevelItems[currentIndex - 1];
  }
  return null;
}
