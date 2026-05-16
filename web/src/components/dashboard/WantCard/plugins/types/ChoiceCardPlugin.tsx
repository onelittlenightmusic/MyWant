import React, { useState, useEffect, useRef } from 'react';
import { WantCardPluginProps, registerWantCardPlugin } from '../registry';
import { SelectInput, SelectInputHandle } from '@/components/common/SelectInput';

const getChoiceLabel = (choice: any): string => {
  if (typeof choice === 'object') {
    if (choice.room && choice.date && choice.time) return `${choice.room} (${choice.date} ${choice.time})`;
    return choice.label || choice.name || choice.room || JSON.stringify(choice);
  }
  return String(choice);
};

const getChoiceValue = (choice: any): string =>
  typeof choice === 'object' ? JSON.stringify(choice) : String(choice);

const ChoiceContentSection: React.FC<WantCardPluginProps> = ({
  want, isChild, isControl, isFocused, isInnerFocused, onExitInnerFocus,
}) => {
  const choiceSelected = want.state?.current?.selected;
  const choices = Array.isArray(want.state?.current?.choices) ? want.state.current.choices : [];
  const choiceTargetParam = (want.state?.current?.target_param as string) || '';

  const [localValue, setLocalValue] = useState(choiceSelected);
  const selectRef = useRef<SelectInputHandle>(null);

  useEffect(() => { setLocalValue(choiceSelected); }, [choiceSelected]);

  // Hand native focus to the trigger button when the want card enters inner-focus mode
  useEffect(() => {
    if (isInnerFocused) selectRef.current?.focus();
  }, [isInnerFocused]);

  const options = [
    { value: '', label: 'Select an option...' },
    ...choices.map((c: any) => ({ value: getChoiceValue(c), label: getChoiceLabel(c) })),
  ];

  const currentValueStr = localValue !== undefined && localValue !== null
    ? getChoiceValue(localValue) : '';

  const handleChange = async (val: string) => {
    const choice = choices.find((c: any) => getChoiceValue(c) === val);
    const newValue = choice ?? (val || null);
    setLocalValue(newValue);
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

  return (
    <div
      className={`${(isChild || (isControl && !isFocused)) ? 'mt-2' : 'mt-4'} space-y-1`}
      onMouseDown={(e) => e.stopPropagation()}
    >
      <div className="flex items-center justify-between text-[9px] text-gray-500 dark:text-gray-400 mb-1">
        <span className="font-medium truncate mr-2" title={choiceTargetParam}>
          {choiceTargetParam || 'Selection'}
        </span>
      </div>
      <SelectInput
        ref={selectRef}
        value={currentValueStr}
        onChange={handleChange}
        options={options}
        placeholder="Select an option..."
        compact
        onKeyDown={(e) => {
          // Escape on closed dropdown → exit inner focus
          if (e.key === 'Escape') onExitInnerFocus?.();
        }}
        onBlur={() => onExitInnerFocus?.()}
      />
    </div>
  );
};

registerWantCardPlugin({
  types: ['choice'],
  ContentSection: ChoiceContentSection,
});
