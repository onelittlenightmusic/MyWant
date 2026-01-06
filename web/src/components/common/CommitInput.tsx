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
export interface CommitInputHandle {
  commit: () => void;
  getValue: () => string;
  focus: () => void;
}

export const CommitInput = React.forwardRef<CommitInputHandle, CommitInputProps>(({
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
  const inputRef = useRef<HTMLInputElement>(null);

  // Sync local value when prop value changes (unless focused)
  useEffect(() => {
    if (!isFocused) {
      setLocalValue(String(value));
    }
  }, [value, isFocused]);

  const hasChanges = localValue !== String(value);

  // Expose methods to parent via ref
  React.useImperativeHandle(ref, () => ({
    commit: () => {
      if (hasChanges) {
        onChange(localValue);
      }
    },
    getValue: () => localValue,
    focus: () => {
      inputRef.current?.focus();
    }
  }));

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
      inputRef.current?.blur();
    }

    if (onKeyDown) {
      onKeyDown(e);
    }
  };

  const handleBlur = (e: React.FocusEvent<HTMLInputElement>) => {
    setIsFocused(false);
    if (onBlur) {
      onBlur(e);
    }
  };

  const handleFocus = () => {
    setIsFocused(true);
  };

  return (
    <div className="relative flex-1">
      <input
        {...props}
        ref={inputRef}
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
