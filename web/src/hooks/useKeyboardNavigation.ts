import { useInputActions } from './useInputActions';

interface UseKeyboardNavigationProps {
  itemCount: number;
  currentIndex: number;
  onNavigate: (index: number) => void;
  enabled?: boolean;
}

/**
 * Hook for flat-list navigation via keyboard (arrow keys / Home / End) and
 * Gamepad (D-pad / left analog stick).
 *
 * Selects the next/previous item and scrolls it into the center of the viewport.
 */
export const useKeyboardNavigation = ({
  itemCount,
  currentIndex,
  onNavigate,
  enabled = true
}: UseKeyboardNavigationProps) => {
  useInputActions({
    enabled: enabled && itemCount > 0,
    onNavigate: (dir) => {
      let newIndex = currentIndex;

      switch (dir) {
        case 'down':
        case 'right':
          newIndex = currentIndex === -1 ? 0 : Math.min(currentIndex + 1, itemCount - 1);
          break;
        case 'up':
        case 'left':
          if (currentIndex > 0) newIndex = currentIndex - 1;
          else return;
          break;
        case 'home':
          newIndex = 0;
          break;
        case 'end':
          newIndex = itemCount - 1;
          break;
        default:
          return;
      }

      onNavigate(newIndex);

      requestAnimationFrame(() => {
        setTimeout(() => {
          const el = document.querySelector('[data-keyboard-nav-selected="true"]');
          if (el instanceof HTMLElement) {
            el.focus();
            el.scrollIntoView({ behavior: 'smooth', block: 'center' });
          }
        }, 0);
      });
    },
  });
};
