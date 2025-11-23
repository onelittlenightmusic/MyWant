import React, { useRef, useEffect, useState } from 'react';
import { Search, X } from 'lucide-react';
import { classNames } from '@/utils/helpers';

interface ExpandableSearchBarProps {
  placeholder?: string;
  value: string;
  onChange: (value: string) => void;
  onFocus?: () => void;
  onBlur?: () => void;
}

export const ExpandableSearchBar: React.FC<ExpandableSearchBarProps> = ({
  placeholder = 'Search wants...',
  value,
  onChange,
  onFocus,
  onBlur
}) => {
  const [isExpanded, setIsExpanded] = useState(false);
  const [isFocused, setIsFocused] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  // Expand when input has value
  useEffect(() => {
    if (value) {
      setIsExpanded(true);
    }
  }, [value]);

  const handleExpand = () => {
    setIsExpanded(true);
    setTimeout(() => inputRef.current?.focus(), 0);
  };

  const handleBlur = () => {
    setIsFocused(false);
    // Collapse only if no value
    if (!value) {
      setIsExpanded(false);
    }
    onBlur?.();
  };

  const handleFocus = () => {
    setIsFocused(true);
    onFocus?.();
  };

  const handleClear = () => {
    onChange('');
    setIsExpanded(false);
    inputRef.current?.blur();
  };

  return (
    <div className="flex items-center gap-2">
      {/* Search Icon Button (always visible) */}
      <button
        onClick={handleExpand}
        className={classNames(
          'p-2 rounded-full transition-all duration-200 flex-shrink-0',
          isExpanded || isFocused
            ? 'text-gray-900 bg-transparent'
            : 'text-gray-500 hover:text-gray-700 hover:bg-gray-100'
        )}
        title="Search"
      >
        <Search className="h-5 w-5" />
      </button>

      {/* Search Input (expands when clicked) */}
      <div
        className={classNames(
          'overflow-hidden transition-all duration-300 ease-in-out flex-shrink-0',
          isExpanded ? 'w-64' : 'w-0'
        )}
      >
        <input
          ref={inputRef}
          type="text"
          placeholder={placeholder}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onFocus={handleFocus}
          onBlur={handleBlur}
          className="w-full px-4 py-2 bg-gray-100 text-gray-900 placeholder-gray-500 rounded-full border-0 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:bg-white transition-colors"
        />
      </div>

      {/* Clear Button (visible when expanded and has value) */}
      {isExpanded && value && (
        <button
          onClick={handleClear}
          className="p-2 text-gray-500 hover:text-gray-700 hover:bg-gray-100 rounded-full transition-colors flex-shrink-0"
          title="Clear search"
        >
          <X className="h-4 w-4" />
        </button>
      )}
    </div>
  );
};
