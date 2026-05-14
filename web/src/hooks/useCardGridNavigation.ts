/**
 * 2D card-grid keyboard + gamepad navigation hook.
 *
 * Analogous to useHierarchicalKeyboardNavigation but for a flat grid layout.
 *
 * - Gamepad D-pad: captureGamepad:true so want-card navigation doesn't also fire.
 * - Keyboard: local onKeyDown returned as `gridProps` (captureGamepad blocks the
 *   document-level bubble handler, so keyboard is handled on the grid div itself).
 *
 * Usage:
 *   const { gridProps } = useCardGridNavigation({ count, cols, isActive, focusedIndex, setFocusedIndex, ... });
 *   <div {...gridProps} className="grid grid-cols-2 gap-2 outline-none"> ... </div>
 */
import { useRef, useEffect, useCallback } from 'react';
import { useInputActions } from './useInputActions';

export interface UseCardGridNavigationOptions {
  /** Total number of navigable items */
  count: number;
  /** Grid column count (default 2) */
  cols?: number;
  /** Whether this grid is currently the active tab / panel */
  isActive: boolean;
  /** Currently highlighted card index (-1 = none) */
  focusedIndex: number;
  setFocusedIndex: (i: number) => void;
  /** Called on Enter / A-button — focus the input at the given card index */
  onConfirm?: (i: number) => void;
  onTabForward?: () => void;
  onTabBackward?: () => void;
}

export interface UseCardGridNavigationResult {
  gridRef: React.RefObject<HTMLDivElement>;
  gridProps: {
    ref: React.RefObject<HTMLDivElement>;
    tabIndex: number;
    onKeyDown: (e: React.KeyboardEvent) => void;
  };
}

export function useCardGridNavigation({
  count,
  cols = 2,
  isActive,
  focusedIndex,
  setFocusedIndex,
  onConfirm,
  onTabForward,
  onTabBackward,
}: UseCardGridNavigationOptions): UseCardGridNavigationResult {
  const gridRef = useRef<HTMLDivElement>(null);
  const focusedIndexRef = useRef(focusedIndex);
  focusedIndexRef.current = focusedIndex;
  const countRef = useRef(count);
  countRef.current = count;
  const onConfirmRef = useRef(onConfirm);
  onConfirmRef.current = onConfirm;

  // Auto-focus grid div when a card is highlighted and no input is focused.
  useEffect(() => {
    if (focusedIndex >= 0) {
      const el = document.activeElement as HTMLElement | null;
      if (!el || (el.tagName !== 'INPUT' && el.tagName !== 'TEXTAREA' && el.tagName !== 'SELECT')) {
        gridRef.current?.focus();
      }
    }
  }, [focusedIndex]);

  // Gamepad exclusivity — prevents want-card handler from also firing D-pad.
  // captureGamepad:true means keyboard events are NOT handled here; we use the
  // local onKeyDown below for keyboard navigation instead.
  useInputActions({
    enabled: isActive && focusedIndex >= 0,
    captureGamepad: true,
    ignoreWhenInputFocused: true,
    ignoreWhenInSidebar: false,
    onTabForward,
    onTabBackward,
    onNavigate: (dir) => {
      const el = document.activeElement as HTMLElement | null;
      if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT')) {
        el.blur();
      }
      const i = focusedIndexRef.current;
      const total = countRef.current;
      const col = i % cols;
      const row = Math.floor(i / cols);
      if (dir === 'left' && i > 0) setFocusedIndex(i - 1);
      else if (dir === 'right' && i + 1 < total) setFocusedIndex(i + 1);
      else if (dir === 'up' && row > 0) setFocusedIndex(i - cols);
      else if (dir === 'down' && i + cols < total) setFocusedIndex(i + cols);
    },
    onConfirm: () => onConfirmRef.current?.(focusedIndexRef.current),
    onCancel: () => {
      const el = document.activeElement as HTMLElement | null;
      if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT')) {
        el.blur();
      } else {
        setFocusedIndex(-1);
      }
    },
  });

  // Keyboard handler for the grid div — mirrors the gamepad logic above.
  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (!isActive) return;
    const target = e.target as HTMLElement;
    const isInInput = target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT';

    if (isInInput) {
      if (e.key === 'Escape') {
        e.preventDefault();
        target.blur();
        gridRef.current?.focus();
      }
      return;
    }

    if (focusedIndexRef.current < 0) {
      if (e.key === 'ArrowDown' || e.key === 'ArrowRight') {
        e.preventDefault();
        setFocusedIndex(0);
      }
      return;
    }

    const i = focusedIndexRef.current;
    const total = countRef.current;
    const col = i % cols;
    const row = Math.floor(i / cols);

    switch (e.key) {
      case 'ArrowLeft':
        e.preventDefault(); e.stopPropagation();
        if (i > 0) setFocusedIndex(i - 1);
        break;
      case 'ArrowRight':
        e.preventDefault(); e.stopPropagation();
        if (i + 1 < total) setFocusedIndex(i + 1);
        break;
      case 'ArrowUp':
        e.preventDefault(); e.stopPropagation();
        if (row > 0) setFocusedIndex(i - cols);
        break;
      case 'ArrowDown':
        e.preventDefault(); e.stopPropagation();
        if (i + cols < total) setFocusedIndex(i + cols);
        break;
      case 'Enter':
        e.preventDefault(); e.stopPropagation();
        onConfirmRef.current?.(i);
        break;
      case 'Escape':
        e.preventDefault(); e.stopPropagation();
        setFocusedIndex(-1);
        break;
    }
  }, [isActive, cols, setFocusedIndex]); // refs are stable — no extra deps needed

  return {
    gridRef,
    gridProps: {
      ref: gridRef,
      tabIndex: -1,
      onKeyDown: handleKeyDown,
    },
  };
}
