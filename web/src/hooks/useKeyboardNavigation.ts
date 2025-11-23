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

        // Use multiple animation frames to ensure React has fully updated the DOM
        // and the browser has reflow/repaint cycle
        requestAnimationFrame(() => {
          requestAnimationFrame(() => {
            const selectedElement = document.querySelector('[data-keyboard-nav-selected="true"]');
            if (selectedElement && selectedElement instanceof HTMLElement) {
              // Use the native scrollIntoView method which handles both window and container scrolling
              selectedElement.scrollIntoView({ behavior: 'smooth', block: 'center' });
            }
          });
        });
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [itemCount, currentIndex, onNavigate, enabled]);
};
