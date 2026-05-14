import React, {
  useState, useRef, useCallback, useEffect, useLayoutEffect, forwardRef,
} from 'react';
import { ChevronDown } from 'lucide-react';
import { useInputActions } from '@/hooks/useInputActions';
import { classNames } from '@/utils/helpers';

export interface SelectOption {
  value: string;
  label?: string;
}

export interface SelectInputProps {
  value: string;
  onChange: (value: string) => void;
  options: SelectOption[];
  placeholder?: string;
  className?: string;
  disabled?: boolean;
  /** Use semi-transparent background so card background icons show through */
  transparent?: boolean;
  /** Pass-through: fires when trigger is focused (closed state). */
  onFocus?: (e: React.FocusEvent) => void;
  /** Pass-through: fires when focus leaves the entire widget. */
  onBlur?: (e: React.FocusEvent) => void;
  /**
   * Pass-through: fires on keydown while the dropdown is CLOSED.
   * Parents use this for between-item arrow-key navigation.
   */
  onKeyDown?: (e: React.KeyboardEvent) => void;
}

export interface SelectInputHandle {
  focus: () => void;
}

/**
 * Custom dropdown with full keyboard + gamepad support.
 *
 * Keyboard (all handled directly in onKeyDown — no captureInput race condition):
 *   Closed  Enter / Space        → open
 *   Closed  ArrowUp/Down         → delegated to parent onKeyDown
 *   Open    ArrowUp/Down         → navigate highlighted option
 *   Open    Enter / Space        → confirm highlighted option → close
 *   Open    Escape               → cancel → close (no change)
 *
 * Gamepad (two captureGamepad hooks — no keyboard interference):
 *   Closed + trigger focused  A  → open
 *   Open                      D-pad Up/Down → navigate
 *   Open                      A  → confirm → close
 *   Open                      B  → cancel → close
 */
export const SelectInput = forwardRef<SelectInputHandle, SelectInputProps>(({
  value,
  onChange,
  options,
  placeholder = '— select —',
  className,
  disabled = false,
  transparent = false,
  onFocus,
  onBlur,
  onKeyDown,
}, ref) => {
  const [isOpen, setIsOpen] = useState(false);
  const [highlightedIndex, setHighlightedIndex] = useState(0);
  const [isTriggerFocused, setIsTriggerFocused] = useState(false);

  const triggerRef = useRef<HTMLButtonElement>(null);
  const listRef = useRef<HTMLUListElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  // Stable refs so gamepad callbacks always see latest values without stale closures
  const highlightedIndexRef = useRef(highlightedIndex);
  highlightedIndexRef.current = highlightedIndex;
  const optionsRef = useRef(options);
  optionsRef.current = options;

  React.useImperativeHandle(ref, () => ({
    focus: () => triggerRef.current?.focus(),
  }));

  const open = useCallback(() => {
    if (disabled) return;
    const idx = options.findIndex(o => o.value === value);
    setHighlightedIndex(idx >= 0 ? idx : 0);
    setIsOpen(true);
  }, [disabled, options, value]);

  const close = useCallback(() => {
    setIsOpen(false);
    // Return focus to trigger after closing
    triggerRef.current?.focus();
  }, []);

  const confirm = useCallback((index: number) => {
    const opt = optionsRef.current[index];
    if (opt) onChange(opt.value);
    setIsOpen(false);
    triggerRef.current?.focus();
  }, [onChange]);

  // Scroll highlighted option into view
  useLayoutEffect(() => {
    if (!isOpen) return;
    const item = listRef.current?.children[highlightedIndex] as HTMLElement | undefined;
    item?.scrollIntoView({ block: 'nearest' });
  }, [isOpen, highlightedIndex]);

  // Close on outside click
  useEffect(() => {
    if (!isOpen) return;
    const handleMouseDown = (e: MouseEvent) => {
      if (!containerRef.current?.contains(e.target as Node)) close();
    };
    document.addEventListener('mousedown', handleMouseDown);
    return () => document.removeEventListener('mousedown', handleMouseDown);
  }, [isOpen, close]);

  // ── Gamepad: closed + trigger focused → A opens ──────────────────────────
  // captureGamepad so D-pad doesn't also move want-card focus while trigger is focused.
  useInputActions({
    enabled: !isOpen && isTriggerFocused,
    captureGamepad: true,
    ignoreWhenInputFocused: false,
    ignoreWhenInSidebar: false,
    onConfirm: open,
  });

  // ── Gamepad: open → navigate / confirm / cancel ───────────────────────────
  // captureGamepad so D-pad presses don't also reach the parent's between-item handler.
  useInputActions({
    enabled: isOpen,
    captureGamepad: true,
    ignoreWhenInputFocused: false,
    ignoreWhenInSidebar: false,
    onNavigate: (dir) => {
      if (dir === 'up') {
        setHighlightedIndex(i => (i > 0 ? i - 1 : optionsRef.current.length - 1));
      } else if (dir === 'down') {
        setHighlightedIndex(i => (i < optionsRef.current.length - 1 ? i + 1 : 0));
      }
    },
    onTabForward: () => {
      setHighlightedIndex(i => (i < optionsRef.current.length - 1 ? i + 1 : 0));
    },
    onTabBackward: () => {
      setHighlightedIndex(i => (i > 0 ? i - 1 : optionsRef.current.length - 1));
    },
    onConfirm: () => confirm(highlightedIndexRef.current),
    onCancel: close,
  });

  // ── Keyboard: all handled directly, no captureInput to avoid React event race ──
  // React's bubble-phase synthetic events and native captureInput window listeners
  // interfere when both try to handle the same key (e.g. Enter closes then reopens).
  // Handling everything here in onKeyDown is reliable and avoids that race.
  const handleTriggerKeyDown = useCallback((e: React.KeyboardEvent<HTMLButtonElement>) => {
    if (isOpen) {
      switch (e.key) {
        case 'ArrowUp':
          e.preventDefault();
          setHighlightedIndex(i => (i > 0 ? i - 1 : optionsRef.current.length - 1));
          return;
        case 'ArrowDown':
          e.preventDefault();
          setHighlightedIndex(i => (i < optionsRef.current.length - 1 ? i + 1 : 0));
          return;
        case 'Enter':
        case ' ':
          e.preventDefault();
          confirm(highlightedIndexRef.current);
          return;
        case 'Escape':
          e.preventDefault();
          close();
          return;
      }
      // Don't delegate to parent while open — all keys are ours
      return;
    }

    // Closed state
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      open();
      return;
    }
    // Delegate everything else (ArrowUp/Down etc.) to parent for between-item nav
    onKeyDown?.(e);
  }, [isOpen, open, confirm, close, onKeyDown]);

  const currentLabel = options.find(o => o.value === value)?.label ?? value;

  return (
    <div ref={containerRef} className={classNames('relative', className ?? '')}>
      {/* Trigger */}
      <button
        ref={triggerRef}
        type="button"
        disabled={disabled}
        onClick={() => (isOpen ? close() : open())}
        onKeyDown={handleTriggerKeyDown}
        onFocus={(e) => {
          setIsTriggerFocused(true);
          onFocus?.(e);
        }}
        onBlur={(e) => {
          setIsTriggerFocused(false);
          if (!containerRef.current?.contains(e.relatedTarget as Node)) {
            onBlur?.(e);
          }
        }}
        className={classNames(
          'w-full flex items-center justify-between gap-2',
          'px-3 py-2 rounded-md border text-sm text-left',
          'transition-colors focus:outline-none focus:ring-2',
          isOpen
            ? `border-blue-400 ring-2 ring-blue-400 ${transparent ? 'bg-white/80 dark:bg-gray-800/70' : 'bg-white dark:bg-gray-800'}`
            : `${transparent ? 'border-gray-200/70 dark:border-gray-600/60 bg-white/70 dark:bg-gray-800/60' : 'border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800'}`,
          'text-gray-900 dark:text-gray-100',
          'hover:border-blue-400 dark:hover:border-blue-500',
          'focus:ring-blue-400 focus:border-blue-400',
          disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer',
        )}
        aria-haspopup="listbox"
        aria-expanded={isOpen}
      >
        <span className={currentLabel ? '' : 'text-gray-400 dark:text-gray-500'}>
          {currentLabel || placeholder}
        </span>
        <ChevronDown
          className={classNames(
            'w-4 h-4 flex-shrink-0 text-gray-400 transition-transform duration-150',
            isOpen ? 'rotate-180' : '',
          )}
        />
      </button>

      {/* Dropdown list */}
      {isOpen && (
        <ul
          ref={listRef}
          role="listbox"
          className={classNames(
            'absolute z-50 mt-1 w-full max-h-60 overflow-y-auto',
            'rounded-md border border-gray-200 dark:border-gray-600',
            'bg-white dark:bg-gray-800 shadow-lg',
            'py-1',
          )}
        >
          {options.map((opt, i) => (
            <li
              key={opt.value}
              role="option"
              aria-selected={opt.value === value}
              onMouseEnter={() => setHighlightedIndex(i)}
              onMouseDown={(e) => { e.preventDefault(); confirm(i); }}
              className={classNames(
                'px-3 py-1.5 text-sm cursor-pointer select-none',
                i === highlightedIndex
                  ? 'bg-blue-500 text-white'
                  : opt.value === value
                    ? 'bg-blue-50 dark:bg-blue-900/20 text-blue-700 dark:text-blue-300'
                    : 'text-gray-900 dark:text-gray-100 hover:bg-gray-100 dark:hover:bg-gray-700',
              )}
            >
              {opt.label ?? opt.value}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
});

SelectInput.displayName = 'SelectInput';
