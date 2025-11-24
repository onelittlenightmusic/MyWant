import { useEffect } from 'react';

export interface HierarchicalItem {
  id: string;
  parentId?: string;
}

interface UseHierarchicalKeyboardNavigationProps<T extends HierarchicalItem> {
  items: T[];
  currentItem: T | null;
  onNavigate: (item: T) => void;
  onExpandParent?: (itemId: string) => void;
  onToggleExpand?: (itemId: string) => void;
  expandedItems?: Set<string>;
  enabled?: boolean;
}

/**
 * Hook for hierarchical keyboard navigation with arrow keys
 * - Arrow Left/Right: Navigate between items at same hierarchy level
 * - Arrow Up: Move to parent item (if current item has parent)
 * - Arrow Down: Move to first child item (if current item has children)
 *
 * @param items - All items in flat list
 * @param currentItem - Currently selected item
 * @param onNavigate - Callback when navigation occurs with new item
 * @param enabled - Whether keyboard navigation is enabled (default: true)
 */
export const useHierarchicalKeyboardNavigation = <T extends HierarchicalItem>({
  items,
  currentItem,
  onNavigate,
  onExpandParent,
  onToggleExpand,
  expandedItems,
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

      let nextItem: T | null = null;
      let shouldNavigate = false;

      switch (e.key) {
        case 'ArrowRight':
          e.preventDefault();
          // Right arrow: navigate within hierarchy (previously Down)
          if (currentItem) {
            // Check if current item is a parent with children
            const hasChildren = items.some(item => item.parentId === currentItem.id);

            if (hasChildren) {
              // If parent is not expanded, expand it
              const isExpanded = expandedItems?.has(currentItem.id);
              if (!isExpanded && onExpandParent) {
                onExpandParent(currentItem.id);
                // Don't navigate, just expand
                shouldNavigate = false;
              } else {
                // Parent is already expanded, move to first child
                nextItem = getFirstChild(items, currentItem);
                shouldNavigate = !!nextItem;
              }
            } else {
              // Current item has no children, move to next sibling at same level
              nextItem = getNextSibling(items, currentItem);
              shouldNavigate = !!nextItem;
            }
          } else if (items.length > 0) {
            // No current item, start with first item
            nextItem = items[0];
            shouldNavigate = true;
          }
          break;

        case 'ArrowLeft':
          e.preventDefault();
          // Left arrow: minimize expanded parent, navigate to previous sibling, or navigate up
          if (currentItem) {
            // Check if current item is an expanded parent (has children and is expanded)
            const hasChildren = items.some(item => item.parentId === currentItem.id);
            const isExpanded = expandedItems?.has(currentItem.id);

            if (hasChildren && isExpanded) {
              // Current item is an expanded parent, toggle/collapse it
              if (onToggleExpand) {
                onToggleExpand(currentItem.id);
              }
              shouldNavigate = false;
            } else if (currentItem.parentId) {
              // Current item is a child, try to navigate to previous sibling first
              nextItem = getPreviousSibling(items, currentItem);
              if (!nextItem) {
                // No previous sibling, move to parent
                nextItem = getParent(items, currentItem);
              }
              shouldNavigate = !!nextItem;
            } else {
              // Current item is a top-level want, move to previous top-level
              nextItem = getPreviousTopLevel(items, currentItem);
              shouldNavigate = !!nextItem;
            }
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
          // Up arrow: navigate to previous top-level want (previously Left)
          nextItem = getPreviousTopLevel(items, currentItem);
          shouldNavigate = !!nextItem;
          break;

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
  }, [items, currentItem, onNavigate, onExpandParent, onToggleExpand, expandedItems, enabled]);
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
