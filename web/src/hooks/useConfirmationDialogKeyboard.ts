import { useEffect } from 'react';

interface UseConfirmationDialogKeyboardProps {
  isVisible: boolean;
  onConfirm: () => void;
  onCancel: () => void;
  loading?: boolean;
  enabled?: boolean;
}

/**
 * Hook for handling keyboard shortcuts in confirmation dialogs and notifications
 *
 * Supported keyboard shortcuts:
 * - y/Y: Confirm action
 * - n/N: Cancel action
 * - Delete: Cancel action
 * - Escape: Cancel action
 *
 * Shortcuts are disabled when:
 * - Dialog/notification is not visible (isVisible = false)
 * - Loading state is active (prevents duplicate submissions)
 * - User is typing in INPUT, TEXTAREA, or contentEditable elements
 * - Hook is explicitly disabled (enabled = false)
 *
 * @param isVisible - Whether the dialog/notification is currently visible
 * @param onConfirm - Callback function for confirm action (triggered by y/Y)
 * @param onCancel - Callback function for cancel action (triggered by n/N/Delete/Escape)
 * @param loading - Whether an async operation is in progress (default: false)
 * @param enabled - Whether keyboard shortcuts are enabled (default: true)
 *
 * @example
 * ```tsx
 * useConfirmationDialogKeyboard({
 *   isVisible: showConfirmation,
 *   onConfirm: handleConfirm,
 *   onCancel: handleCancel,
 *   loading: isSubmitting,
 *   enabled: true
 * });
 * ```
 */
export const useConfirmationDialogKeyboard = ({
  isVisible,
  onConfirm,
  onCancel,
  loading = false,
  enabled = true
}: UseConfirmationDialogKeyboardProps) => {
  useEffect(() => {
    // Early exit if disabled, not visible, or loading
    if (!enabled || !isVisible || loading) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      // Don't intercept if user is typing in an input element
      const target = e.target as HTMLElement;
      const isInputElement =
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable;

      if (isInputElement) return;

      // Normalize key to lowercase for case-insensitive matching
      const key = e.key.toLowerCase();

      // Handle confirm shortcut (y/Y)
      if (key === 'y') {
        e.preventDefault();
        onConfirm();
        return;
      }

      // Handle cancel shortcuts (n/N, Delete, Escape)
      if (key === 'n' || e.key === 'Delete' || e.key === 'Escape') {
        e.preventDefault();
        onCancel();
        return;
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isVisible, onConfirm, onCancel, loading, enabled]);
};
