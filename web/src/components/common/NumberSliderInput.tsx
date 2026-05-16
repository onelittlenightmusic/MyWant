import React from 'react';
import { classNames } from '@/utils/helpers';

export interface NumberSliderInputProps {
  value: number;
  min: number;
  max: number;
  step?: number;
  onChange: (value: number) => void;
  isDirty?: boolean;
  /** Label on the left of the value row. Omit when the caller shows the name elsewhere. */
  label?: string;
  /** Stop click/change event propagation (needed inside want cards) */
  stopPropagation?: boolean;
  className?: string;
}

export const NumberSliderInput: React.FC<NumberSliderInputProps> = ({
  value, min, max, step = 1, onChange,
  isDirty = false, label, stopPropagation = false, className,
}) => {
  const stop = (e: React.SyntheticEvent) => { if (stopPropagation) e.stopPropagation(); };

  return (
    <div className={classNames('space-y-1', className)}>
      <div className="relative flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
        {label !== undefined && (
          <span className="font-medium truncate mr-2">{label}</span>
        )}
        <span className={classNames(
          'font-mono tabular-nums text-sm font-semibold',
          label === undefined ? 'ml-auto' : '',
          isDirty ? 'text-yellow-700 dark:text-yellow-400' : 'text-gray-800 dark:text-gray-200',
        )}>
          {value}
        </span>
        {isDirty && (
          <div className="absolute right-0 -top-5 text-[10px] font-medium text-yellow-700 bg-yellow-100 px-1.5 py-0.5 rounded shadow-sm border border-yellow-200 pointer-events-none">
            OK?
          </div>
        )}
      </div>
      <input
        type="range"
        min={min}
        max={max}
        step={step}
        value={value}
        onChange={(e) => { stop(e); onChange(Number(e.target.value)); }}
        onClick={stop}
        className={classNames(
          'w-full h-2 rounded-lg appearance-none cursor-pointer accent-sky-500',
          isDirty ? 'bg-sky-50 dark:bg-sky-900/20' : 'bg-gray-200 dark:bg-gray-700',
        )}
      />
      <div className="flex justify-between text-[10px] text-gray-400 dark:text-gray-500">
        <span>{min}</span>
        <span>{max}</span>
      </div>
    </div>
  );
};
