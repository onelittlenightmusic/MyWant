import { useEffect } from 'react';

interface UseEscapeKeyProps {
  onEscape: () => void;
  enabled?: boolean;
}

/**
 * Hook for handling ESC key press to close modals/sidebars and deselect items
 *
 * @param onEscape - Callback when ESC key is pressed
 * @param enabled - Whether ESC key handling is enabled (default: true)
 */
export const useEscapeKey = ({
  onEscape,
  enabled = true
}: UseEscapeKeyProps) => {
  useEffect(() => {
    if (!enabled) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      // Don't intercept if user is typing in an input
      const target = e.target as HTMLElement;
      const isInputElement =
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable;

      if (isInputElement) return;

      if (e.key === 'Escape') {
        e.preventDefault();
        onEscape();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onEscape, enabled]);
};
