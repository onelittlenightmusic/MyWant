import React, { useState, useEffect, useRef } from 'react';
import { WantCardPluginProps, registerWantCardPlugin } from '../registry';
import { useInputActions } from '@/hooks/useInputActions';
import {
  EVERY_PRESETS, TimerMode, parseAt, formatAt,
} from '@/components/forms/timerUtils';
import { TimerEveryDial } from '@/components/forms/TimerEveryDial';
import { TimerClockFace } from '@/components/forms/TimerClockFace';

const TimerContentSection: React.FC<WantCardPluginProps> = ({
  want, isChild, isControl, isFocused, isInnerFocused, onExitInnerFocus,
}) => {
  const stateEvery        = (want.state?.current?.every         as string) || '';
  const stateAt           = (want.state?.current?.at            as string) || '';
  const stateTimerMode    = (want.state?.current?.timer_mode    as string) || 'every';
  const stateAtRecurrence = (want.state?.current?.at_recurrence as string) || '';
  const stateAtWeekday    = (want.state?.current?.at_weekday    as string) || '';
  const timerTargetParam  = (want.state?.current?.target_param  as string) || '';

  const [mode, setMode]               = useState<TimerMode>(stateTimerMode === 'at' ? 'at' : 'every');
  const [localEvery, setLocalEvery]   = useState(stateEvery);
  const [atH24, setAtH24]             = useState(() => parseAt(stateAt).h24);
  const [atMin, setAtMin]             = useState(() => parseAt(stateAt).min);
  const [atRecurrence, setAtRecurrence] = useState(stateAtRecurrence);
  const [atWeekday, setAtWeekday]     = useState(stateAtWeekday);

  const everyDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const committedRef     = useRef(stateEvery);

  useEffect(() => { if (!isInnerFocused) setLocalEvery(stateEvery); }, [stateEvery, isInnerFocused]);
  useEffect(() => { const p = parseAt(stateAt); setAtH24(p.h24); setAtMin(p.min); }, [stateAt]);
  useEffect(() => { setAtRecurrence(stateAtRecurrence); }, [stateAtRecurrence]);
  useEffect(() => { setAtWeekday(stateAtWeekday); }, [stateAtWeekday]);
  useEffect(() => { setMode(stateTimerMode === 'at' ? 'at' : 'every'); }, [stateTimerMode]);
  useEffect(() => { if (isInnerFocused) committedRef.current = localEvery; }, [isInnerFocused, localEvery]);

  const updateStateKey = (key: string, value: string, debounceRef?: React.MutableRefObject<ReturnType<typeof setTimeout> | null>) => {
    const doUpdate = async () => {
      const id = want.metadata?.id;
      if (!id) return;
      try {
        await fetch(`/api/v1/states/${id}/${key}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(value),
        });
      } catch (err) {
        console.error(`[TimerCard] ${key} update failed:`, err);
      }
    };
    if (debounceRef) {
      if (debounceRef.current) clearTimeout(debounceRef.current);
      debounceRef.current = setTimeout(doUpdate, 400);
    } else {
      doUpdate();
    }
  };

  const handleModeSwitch = (newMode: TimerMode) => {
    setMode(newMode);
    updateStateKey('timer_mode', newMode);
  };

  const isDirty = !!isInnerFocused && localEvery !== committedRef.current;

  useInputActions({
    enabled: !!isInnerFocused && mode === 'every',
    captureInput: true,
    ignoreWhenInputFocused: false,
    onNavigate: (dir) => {
      const idx = EVERY_PRESETS.indexOf(localEvery);
      if (dir === 'up' || dir === 'right') {
        setLocalEvery(EVERY_PRESETS[Math.min(EVERY_PRESETS.length - 1, idx < 0 ? 0 : idx + 1)]);
      } else {
        setLocalEvery(EVERY_PRESETS[Math.max(0, idx < 0 ? EVERY_PRESETS.length - 1 : idx - 1)]);
      }
    },
    onConfirm: () => { updateStateKey('every', localEvery); onExitInnerFocus?.(); },
    onCancel:  () => { setLocalEvery(committedRef.current); onExitInnerFocus?.(); },
  });

  return (
    <div className={`${(isChild || (isControl && !isFocused)) ? 'mt-1' : 'mt-2'} space-y-2`}>

      {/* Mode toggle */}
      <div className="flex gap-0.5 p-0.5 bg-gray-100 dark:bg-gray-800 rounded-lg"
        onClick={(e) => e.stopPropagation()}
        onMouseDown={(e) => e.stopPropagation()}
      >
        {(['every', 'at'] as TimerMode[]).map((m) => (
          <button key={m}
            onClick={(e) => { e.stopPropagation(); handleModeSwitch(m); }}
            onMouseDown={(e) => e.stopPropagation()}
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
          every={localEvery}
          isDirty={isDirty}
          stopPropagation
          onSelect={(preset) => {
            setLocalEvery(preset);
            if (isInnerFocused) onExitInnerFocus?.();
            updateStateKey('every', preset, everyDebounceRef);
          }}
          rightSlot={
            <>
              {isDirty && (
                <div className="absolute right-0 -top-5 text-[10px] font-medium text-yellow-700 bg-yellow-100 px-1.5 py-0.5 rounded shadow-sm border border-yellow-200 pointer-events-none">
                  OK?
                </div>
              )}
              <span className="text-[10px] text-gray-400 dark:text-gray-500 font-mono truncate leading-none"
                title={timerTargetParam}>
                {timerTargetParam || 'timer'}
              </span>
              <span className={`text-xl font-mono font-bold leading-tight ${isDirty ? 'text-yellow-500 dark:text-yellow-400' : 'text-blue-500 dark:text-blue-400'}`}
                style={{ transition: 'color 0.2s' }}>
                {localEvery || '--'}
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
          stopPropagation
          onAtChange={(at) => {
            const p = parseAt(at);
            setAtH24(p.h24);
            setAtMin(p.min);
            updateStateKey('at', at);
          }}
          onRecurrenceChange={(r) => {
            setAtRecurrence(r);
            updateStateKey('at_recurrence', r);
          }}
          onAtWeekdayChange={(wd) => {
            setAtWeekday(wd);
            updateStateKey('at_weekday', wd);
          }}
          rightExtra={
            <span className="text-[10px] text-gray-400 dark:text-gray-500 font-mono truncate leading-none"
              title={timerTargetParam}>
              {timerTargetParam || 'timer'}
            </span>
          }
        />
      )}
    </div>
  );
};

registerWantCardPlugin({
  types: ['timer'],
  ContentSection: TimerContentSection,
});
