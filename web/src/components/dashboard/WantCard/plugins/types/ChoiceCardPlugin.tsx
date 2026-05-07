import React, { useState, useEffect, useRef } from 'react';
import { classNames } from '@/utils/helpers';
import { WantCardPluginProps, registerWantCardPlugin } from '../registry';
import { useInputActions } from '@/hooks/useInputActions';
import styles from '../../../WantCard.module.css';

const ChoiceContentSection: React.FC<WantCardPluginProps> = ({
  want, isChild, isControl, isFocused, isInnerFocused, onExitInnerFocus,
}) => {
  const choiceSelected = want.state?.current?.selected;
  const choices = Array.isArray(want.state?.current?.choices) ? want.state.current.choices : [];
  const choiceTargetParam = (want.state?.current?.target_param as string) || '';

  const [localValue, setLocalValue] = useState(choiceSelected);
  const [isDropdownOpen, setIsDropdownOpen] = useState(false);
  const [highlightedIndex, setHighlightedIndex] = useState(0);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);

  // Keep highlighted item centered in the dropdown list
  useEffect(() => {
    itemRefs.current[highlightedIndex]?.scrollIntoView({ block: 'center', behavior: 'smooth' });
  }, [highlightedIndex]);

  useEffect(() => { setLocalValue(choiceSelected); }, [choiceSelected]);

  // Close dropdown when inner focus is lost
  useEffect(() => {
    if (!isInnerFocused) setIsDropdownOpen(false);
  }, [isInnerFocused]);

  // Sync highlighted index to currently selected option when opening
  useEffect(() => {
    if (isDropdownOpen && choices.length > 0) {
      const idx = choices.findIndex((c: any) => {
        const v = typeof c === 'object' ? JSON.stringify(c) : String(c);
        const lv = localValue === undefined || localValue === null ? '' : typeof localValue === 'object' ? JSON.stringify(localValue) : String(localValue);
        return v === lv;
      });
      setHighlightedIndex(idx >= 0 ? idx : 0);
    }
  }, [isDropdownOpen]);

  const handleSelect = async (choice: any) => {
    const newValue = choice;
    setLocalValue(newValue);
    setIsDropdownOpen(false);
    onExitInnerFocus?.();
    const id = want.metadata?.id;
    if (!id) return;
    try {
      await fetch(`/api/v1/states/${id}/selected`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(newValue),
      });
    } catch (err) {
      console.error('[ChoiceCard] state update failed:', err);
    }
  };

  const getChoiceLabel = (choice: any): string => {
    if (typeof choice === 'object') {
      if (choice.room && choice.date && choice.time) return `${choice.room} (${choice.date} ${choice.time})`;
      return choice.label || choice.name || choice.room || JSON.stringify(choice);
    }
    return String(choice);
  };

  const getChoiceValue = (choice: any): string =>
    typeof choice === 'object' ? JSON.stringify(choice) : String(choice);

  // Inner focus: A→open dropdown, B→exit
  useInputActions({
    enabled: !!isInnerFocused && !isDropdownOpen,
    captureInput: true,
    ignoreWhenInputFocused: false,
    onConfirm: () => setIsDropdownOpen(true),
    onCancel: onExitInnerFocus,
  });

  // Dropdown open: Up/Down navigate, A/Enter confirm, B/Escape close+exit
  useInputActions({
    enabled: !!isInnerFocused && isDropdownOpen,
    captureInput: true,
    ignoreWhenInputFocused: false,
    onNavigate: (dir) => {
      if (dir === 'up') setHighlightedIndex(i => Math.max(0, i - 1));
      else if (dir === 'down') setHighlightedIndex(i => Math.min(choices.length - 1, i + 1));
    },
    onConfirm: () => {
      if (choices[highlightedIndex] !== undefined) handleSelect(choices[highlightedIndex]);
    },
    onCancel: () => {
      setIsDropdownOpen(false);
      onExitInnerFocus?.();
    },
  });

  const currentLabel = localValue === undefined || localValue === null
    ? 'Select an option...'
    : getChoiceLabel(localValue);

  return (
    <div className={`${(isChild || (isControl && !isFocused)) ? 'mt-2' : 'mt-4'} space-y-1`}>
      <div className="flex items-center justify-between text-[9px] text-gray-500 dark:text-gray-400 mb-1">
        <span className="font-medium truncate mr-2" title={choiceTargetParam}>
          {choiceTargetParam || 'Selection'}
        </span>
      </div>

      {/* Custom dropdown trigger */}
      <div className="relative" ref={dropdownRef}>
        <button
          type="button"
          onClick={(e) => { e.stopPropagation(); setIsDropdownOpen(v => !v); }}
          onMouseDown={(e) => e.stopPropagation()}
          className={classNames(
            'w-full text-left appearance-none border rounded px-2 py-1',
            'bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 text-xs',
            'focus:outline-none',
            isInnerFocused && !isDropdownOpen
              ? 'ring-2 ring-yellow-400 border-yellow-400'
              : 'border-gray-300 dark:border-gray-600',
            styles.compactSelect,
          )}
        >
          <span className={localValue === undefined || localValue === null ? 'text-gray-400' : undefined}>
            {currentLabel}
          </span>
          <span className="float-right text-gray-400">▾</span>
        </button>

        {/* Dropdown list */}
        {isDropdownOpen && choices.length > 0 && (
          <div className="absolute z-50 w-full mt-0.5 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded shadow-lg max-h-40 overflow-y-auto">
            {choices.map((choice: any, idx: number) => (
              <button
                key={idx}
                ref={el => { itemRefs.current[idx] = el; }}
                type="button"
                onClick={(e) => { e.stopPropagation(); handleSelect(choice); }}
                onMouseEnter={() => setHighlightedIndex(idx)}
                className={classNames(
                  'w-full text-left px-2 py-1.5 text-xs',
                  idx === highlightedIndex
                    ? 'bg-yellow-400 text-gray-900'
                    : 'text-gray-900 dark:text-gray-100 hover:bg-gray-100 dark:hover:bg-gray-700',
                )}
              >
                {getChoiceLabel(choice)}
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};

registerWantCardPlugin({
  types: ['choice'],
  ContentSection: ChoiceContentSection,
});
