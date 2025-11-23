import { useEffect } from 'react';

export interface HierarchicalItem {
  id: string;
  parentId?: string;
}

interface UseHierarchicalKeyboardNavigationProps<T extends HierarchicalItem> {
  items: T[];
  currentItem: T | null;
  onNavigate: (item: T) => void;
  enabled?: boolean;
}

/**
 * Hook for hierarchical keyboard navigation with arrow keys
 * - Arrow Up/Down: Navigate between items at same hierarchy level
 * - Arrow Left: Move to parent item (if current item has parent)
 * - Arrow Right: Move to first child item (if current item has children)
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
        case 'ArrowDown':
          e.preventDefault();
          nextItem = getNextSibling(items, currentItem);
          shouldNavigate = !!nextItem;
          break;

        case 'ArrowUp':
          e.preventDefault();
          nextItem = getPreviousSibling(items, currentItem);
          shouldNavigate = !!nextItem;
          break;

        case 'ArrowRight':
          e.preventDefault();
          if (currentItem) {
            // Try to move to first child
            nextItem = getFirstChild(items, currentItem);
            if (!nextItem) {
              // If no children, move to next sibling at same level
              nextItem = getNextSibling(items, currentItem);
            }
          } else if (items.length > 0) {
            // No current item, start with first item
            nextItem = items[0];
          }
          shouldNavigate = !!nextItem;
          break;

        case 'ArrowLeft':
          e.preventDefault();
          if (currentItem) {
            // Try to move to parent
            nextItem = getParent(items, currentItem);
            if (!nextItem) {
              // If no parent, move to previous sibling at same level
              nextItem = getPreviousSibling(items, currentItem);
            }
          }
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
  }, [items, currentItem, onNavigate, enabled]);
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
