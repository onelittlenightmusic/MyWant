import React, { useState, useEffect, useRef, KeyboardEvent } from 'react';
import { classNames } from '@/utils/helpers';

interface CommitInputProps extends Omit<React.InputHTMLAttributes<HTMLInputElement>, 'onChange'> {
  value: string | number;
  onChange: (value: string) => void;
  hint?: string;
}

/**
 * An input component that only commits its value when the user presses Enter.
 * Highlights in yellow when there are uncommitted changes.
 */
export const CommitInput = React.forwardRef<HTMLInputElement, CommitInputProps>(({
  value,
  onChange,
  className,
  onKeyDown,
  onBlur,
  hint = 'Enter to commit',
  ...props
}, ref) => {
  const [localValue, setLocalValue] = useState<string>(String(value));
  const [isFocused, setIsFocused] = useState(false);
  const internalRef = useRef<HTMLInputElement>(null);

  // Sync local value when prop value changes (unless focused)
  useEffect(() => {
    if (!isFocused) {
      setLocalValue(String(value));
    }
  }, [value, isFocused]);

  const hasChanges = localValue !== String(value);

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      if (hasChanges) {
        e.preventDefault();
        onChange(localValue);
      }
      // If no changes, let it bubble up (e.g. for form submission)
    } else if (e.key === 'Escape') {
      e.preventDefault();
      setLocalValue(String(value));
      internalRef.current?.blur();
    }

    if (onKeyDown) {
      onKeyDown(e);
    }
  };

  const handleBlur = (e: React.FocusEvent<HTMLInputElement>) => {
    setIsFocused(false);
    // Optional: revert on blur? The user said "Enter to confirm", 
    // so maybe we should keep the yellow state until Enter or Escape?
    // But usually blur reverts or commits.
    // Given the requirement "Enterで確定", let's keep it yellow even on blur 
    // if it has changes, but revert if requested. 
    // Actually, let's just let it stay yellow.
    
    if (onBlur) {
      onBlur(e);
    }
  };

  const handleFocus = () => {
    setIsFocused(true);
  };

  // Merge refs
  const setRefs = (node: HTMLInputElement | null) => {
    (internalRef as React.MutableRefObject<HTMLInputElement | null>).current = node;
    if (typeof ref === 'function') {
      ref(node);
    } else if (ref) {
      (ref as React.MutableRefObject<HTMLInputElement | null>).current = node;
    }
  };

  return (
    <div className="relative flex-1">
      <input
        {...props}
        ref={setRefs}
        value={localValue}
        onChange={(e) => setLocalValue(e.target.value)}
        onKeyDown={handleKeyDown}
        onFocus={handleFocus}
        onBlur={handleBlur}
        className={classNames(
          'w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 transition-colors text-sm',
          hasChanges 
            ? 'bg-yellow-50 border-yellow-400 focus:ring-yellow-400' 
            : 'bg-white border-gray-300 focus:ring-blue-500 focus:border-transparent',
          className
        )}
      />
      {hasChanges && (
        <div className="absolute right-2 -top-5 text-[10px] font-medium text-yellow-700 bg-yellow-100 px-1.5 py-0.5 rounded shadow-sm border border-yellow-200 pointer-events-none animate-in fade-in slide-in-from-bottom-1">
          {hint}
        </div>
      )}
    </div>
  );
});

CommitInput.displayName = 'CommitInput';
