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

      switch (e.key) {
        case 'ArrowRight':
          e.preventDefault();
          // Right arrow behavior:
          // - If parent is expanded, move to first child
          // - If at a child, move to next sibling
          // - If at last child, move to next parent's sibling
          // - If at top-level non-expanded parent, move to next top-level item
          // - If no current item, restore last selected item or start with first item
          let itemForRight = currentItem;
          if (!itemForRight && lastSelectedItemId && items.length > 0) {
            // No current item but we have a last selected ID, restore it
            itemForRight = items.find(item => item.id === lastSelectedItemId) || null;
          }

          if (itemForRight) {
            if (!itemForRight.parentId) {
              // Current item is top-level (parent)
              const hasChildren = items.some(item => item.parentId === itemForRight.id);
              const isExpanded = expandedItems?.has(itemForRight.id);

              if (hasChildren && isExpanded) {
                // Parent is expanded, move to first child
                nextItem = getFirstChild(items, itemForRight);
              } else {
                // Parent is not expanded or has no children, move to next top-level
                nextItem = getNextTopLevel(items, itemForRight);
              }
            } else {
              // Current item is a child, move to next sibling
              nextItem = getNextSibling(items, itemForRight);
              if (!nextItem) {
                // No next sibling, move to next parent's sibling
                const parent = getParent(items, itemForRight);
                if (parent) {
                  nextItem = getNextSibling(items, parent);
                }
              }
            }
            shouldNavigate = !!nextItem;
          } else if (items.length > 0) {
            // No current item or last selected item, start with first item
            nextItem = items[0];
            shouldNavigate = true;
          }
          break;

        case 'ArrowLeft':
          e.preventDefault();
          // Left arrow behavior:
          // - If at a child with previous sibling, move to previous sibling
          // - If at first child or no previous sibling, move to parent
          // - If at top-level, move to previous top-level item
          // - If no current item, restore last selected item
          let itemForLeft = currentItem;
          if (!itemForLeft && lastSelectedItemId && items.length > 0) {
            // No current item but we have a last selected ID, restore it
            itemForLeft = items.find(item => item.id === lastSelectedItemId) || null;
          }

          if (itemForLeft) {
            if (itemForLeft.parentId) {
              // Current item is a child
              nextItem = getPreviousSibling(items, itemForLeft);
              if (!nextItem) {
                // No previous sibling, move to parent
                nextItem = getParent(items, itemForLeft);
              }
            } else {
              // Current item is top-level, move to previous top-level item
              nextItem = getPreviousTopLevel(items, itemForLeft);
            }
            shouldNavigate = !!nextItem;
          }
          break;

        case 'ArrowDown':
          e.preventDefault();
          // Down arrow: navigate to next top-level want (previously Right)
          nextItem = getNextTopLevel(items, currentItem);
          shouldNavigate = !!nextItem;
          break;

        case 'ArrowUp':
          e.preventDefault();
          // Up arrow: navigate to previous top-level want
          nextItem = getPreviousTopLevel(items, currentItem);
          shouldNavigate = !!nextItem;
          break;

        case ' ':
          e.preventDefault();
          // Space: Toggle expand/collapse on current parent item
          if (currentItem) {
            // Check if current item is a parent (has children)
            const hasChildren = items.some(item => item.parentId === currentItem.id);
            if (hasChildren && onToggleExpand) {
              onToggleExpand(currentItem.id);
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
