import React from 'react';
import { TimerMode, parseAt } from './timerUtils';
import { TimerEveryDial } from './TimerEveryDial';
import { TimerClockFace } from './TimerClockFace';

export { EVERY_PRESETS } from './timerUtils';
export type { TimerMode } from './timerUtils';

export interface TimerPickerContentProps {
  mode: TimerMode;
  every: string;
  at: string;
  atRecurrence: string;
  atWeekday?: string;
  onModeChange: (mode: TimerMode) => void;
  onEveryChange: (every: string) => void;
  onAtChange: (at: string) => void;
  onAtRecurrenceChange: (r: string) => void;
  onAtWeekdayChange?: (wd: string) => void;
}

export const TimerPickerContent: React.FC<TimerPickerContentProps> = ({
  mode, every, at, atRecurrence, atWeekday = '',
  onModeChange, onEveryChange, onAtChange, onAtRecurrenceChange, onAtWeekdayChange,
}) => {
  const { h24: atH24, min: atMin } = parseAt(at);

  return (
    <div className="space-y-2">
      <div className="flex gap-0.5 p-0.5 bg-gray-100 dark:bg-gray-800 rounded-lg">
        {(['every', 'at'] as TimerMode[]).map((m) => (
          <button key={m} type="button" onClick={() => onModeChange(m)}
            className={`flex-1 text-[10px] font-mono font-semibold py-0.5 rounded-md transition-all ${
              mode === m
                ? 'bg-white dark:bg-gray-700 text-blue-600 dark:text-blue-400 shadow-sm'
                : 'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300'
            }`}
          >
            {m}
          </button>
        ))}
      </div>

      {mode === 'every' ? (
        <TimerEveryDial
          every={every}
          onSelect={onEveryChange}
          rightSlot={
            <>
              <span className="text-[10px] text-gray-400 dark:text-gray-500 font-mono">interval</span>
              <span className="text-xl font-mono font-bold leading-tight text-blue-500 dark:text-blue-400">
                {every || '--'}
              </span>
            </>
          }
        />
      ) : (
        <TimerClockFace
          atH24={atH24}
          atMin={atMin}
          atRecurrence={atRecurrence}
          atWeekday={atWeekday}
          onAtChange={onAtChange}
          onRecurrenceChange={onAtRecurrenceChange}
          onAtWeekdayChange={onAtWeekdayChange}
        />
      )}
    </div>
  );
};
