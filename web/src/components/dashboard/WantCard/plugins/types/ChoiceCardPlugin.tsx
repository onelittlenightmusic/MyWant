import React, { useState, useEffect } from 'react';
import { classNames } from '@/utils/helpers';
import { WantCardPluginProps, registerWantCardPlugin } from '../registry';
import styles from '../../../WantCard.module.css';

const ChoiceContentSection: React.FC<WantCardPluginProps> = ({
  want, isChild, isControl, isFocused,
}) => {
  const choiceSelected = want.state?.current?.selected;
  const choices = Array.isArray(want.state?.current?.choices) ? want.state.current.choices : [];
  const choiceTargetParam = (want.state?.current?.target_param as string) || '';

  const [localValue, setLocalValue] = useState(choiceSelected);

  useEffect(() => { setLocalValue(choiceSelected); }, [choiceSelected]);

  const handleChange = async (newValue: any) => {
    setLocalValue(newValue);
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
    <div className={`${(isChild || (isControl && !isFocused)) ? 'mt-2' : 'mt-4'} space-y-1`}>
      <div className="flex items-center justify-between text-[9px] text-gray-500 dark:text-gray-400 mb-1">
        <span className="font-medium truncate mr-2" title={choiceTargetParam}>
          {choiceTargetParam || 'Selection'}
        </span>
      </div>
      <select
        value={
          localValue === undefined || localValue === null
            ? ''
            : typeof localValue === 'object'
            ? JSON.stringify(localValue)
            : String(localValue)
        }
        onChange={(e) => {
          const val = e.target.value;
          try { handleChange(JSON.parse(val)); } catch { handleChange(val); }
        }}
        onClick={(e) => e.stopPropagation()}
        onMouseDown={(e) => e.stopPropagation()}
        className={classNames(
          'w-full appearance-none border border-gray-300 dark:border-gray-600 rounded',
          'bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100',
          'focus:outline-none focus:ring-1 focus:ring-blue-500',
          styles.compactSelect,
        )}
      >
        <option value="" disabled>Select an option...</option>
        {choices.map((choice: any, idx: number) => {
          const label =
            typeof choice === 'object'
              ? choice.room && choice.date && choice.time
                ? `${choice.room} (${choice.date} ${choice.time})`
                : choice.label || choice.name || choice.room || JSON.stringify(choice)
              : String(choice);
          const value = typeof choice === 'object' ? JSON.stringify(choice) : choice;
          return <option key={idx} value={value}>{label}</option>;
        })}
      </select>
    </div>
  );
};

registerWantCardPlugin({
  types: ['choice'],
  ContentSection: ChoiceContentSection,
});
