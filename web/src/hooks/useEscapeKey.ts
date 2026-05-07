import { useInputActions } from './useInputActions';

interface UseEscapeKeyProps {
  onEscape: () => void;
  enabled?: boolean;
}

/**
 * Calls `onEscape` when the Escape key is pressed or the Gamepad B button is
 * pressed.  Ignored when an input element is focused or focus is inside a
 * sidebar.
 */
export const useEscapeKey = ({
  onEscape,
  enabled = true
}: UseEscapeKeyProps) => {
  useInputActions({
    enabled,
    onCancel: onEscape,
  });
};
