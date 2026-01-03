import { useEffect } from 'react';

interface UseKeyboardNavigationProps {
  itemCount: number;
  currentIndex: number;
  onNavigate: (index: number) => void;
  enabled?: boolean;
}

/**
 * Hook for keyboard navigation with arrow keys
 * Supports Arrow Up/Down for vertical navigation and Arrow Left/Right for grid navigation
 *
 * @param itemCount - Total number of items
 * @param currentIndex - Current selected item index (-1 if none selected)
 * @param onNavigate - Callback when navigation occurs with new index
 * @param enabled - Whether keyboard navigation is enabled (default: true)
 */
export const useKeyboardNavigation = ({
  itemCount,
  currentIndex,
  onNavigate,
  enabled = true
}: UseKeyboardNavigationProps) => {
  useEffect(() => {
    if (!enabled || itemCount === 0) return;

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

      let newIndex = currentIndex;
      let shouldNavigate = false;

      switch (e.key) {
        case 'ArrowDown':
          e.preventDefault();
          newIndex = currentIndex === -1 ? 0 : Math.min(currentIndex + 1, itemCount - 1);
          shouldNavigate = true;
          break;

        case 'ArrowUp':
          e.preventDefault();
          if (currentIndex > 0) {
            newIndex = currentIndex - 1;
            shouldNavigate = true;
          }
          break;

        case 'ArrowRight':
          e.preventDefault();
          newIndex = currentIndex === -1 ? 0 : Math.min(currentIndex + 1, itemCount - 1);
          shouldNavigate = true;
          break;

        case 'ArrowLeft':
          e.preventDefault();
          if (currentIndex > 0) {
            newIndex = currentIndex - 1;
            shouldNavigate = true;
          }
          break;

        case 'Home':
          e.preventDefault();
          if (itemCount > 0) {
            newIndex = 0;
            shouldNavigate = true;
          }
          break;

        case 'End':
          e.preventDefault();
          if (itemCount > 0) {
            newIndex = itemCount - 1;
            shouldNavigate = true;
          }
          break;

        default:
          return;
      }

      if (shouldNavigate) {
        onNavigate(newIndex);

        // Use requestAnimationFrame and a small timeout to ensure React has fully updated the DOM
        // before attempting to scroll. This prevents animation artifacts.
        requestAnimationFrame(() => {
          setTimeout(() => {
            const selectedElement = document.querySelector('[data-keyboard-nav-selected="true"]');
            if (selectedElement && selectedElement instanceof HTMLElement) {
              // Use 'center' to ensure the selected element is clearly visible in the center of the viewport
              selectedElement.scrollIntoView({ behavior: 'smooth', block: 'center' });
            }
          }, 0);
        });
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [itemCount, currentIndex, onNavigate, enabled]);
};
