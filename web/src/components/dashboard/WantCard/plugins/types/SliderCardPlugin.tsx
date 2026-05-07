import React, { useState, useEffect, useRef, useCallback } from 'react';
import { WantCardPluginProps, registerWantCardPlugin } from '../registry';
import { useInputActions } from '@/hooks/useInputActions';

const SliderContentSection: React.FC<WantCardPluginProps> = ({
  want, isChild, isControl, isFocused, isInnerFocused, onExitInnerFocus, onSliderActiveChange,
}) => {
  const sliderValue = typeof want.state?.current?.value === 'number' ? want.state.current.value : 0;
  const sliderMin = typeof want.state?.current?.min === 'number' ? want.state.current.min : 0;
  const sliderMax = typeof want.state?.current?.max === 'number' ? want.state.current.max : 100;
  const sliderStep = typeof want.state?.current?.step === 'number' ? want.state.current.step : 1;
  const sliderTargetParam = (want.state?.current?.target_param as string) || '';

  const [localValue, setLocalValue] = useState(sliderValue);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Value captured when inner focus starts — used to revert on cancel
  const committedRef = useRef(sliderValue);

  // Sync from server unless inner-focused (would override in-progress navigation)
  useEffect(() => {
    if (!isInnerFocused) setLocalValue(sliderValue);
  }, [sliderValue, isInnerFocused]);

  // Capture the committed value when entering inner focus
  useEffect(() => {
    if (isInnerFocused) committedRef.current = localValue;
  }, [isInnerFocused]);

  const commitToApi = useCallback(async (value: number) => {
    const id = want.metadata?.id;
    if (!id) return;
    try {
      await fetch(`/api/v1/states/${id}/value`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(value),
      });
    } catch (err) {
      console.error('[SliderCard] state update failed:', err);
    }
  }, [want.metadata?.id]);

  // Mouse drag: update locally and commit immediately via debounce
  const handleMouseChange = useCallback((newValue: number) => {
    const clamped = Math.min(sliderMax, Math.max(sliderMin, newValue));
    setLocalValue(clamped);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => commitToApi(clamped), 150);
  }, [sliderMin, sliderMax, commitToApi]);

  const isDirty = !!isInnerFocused && localValue !== committedRef.current;

  // Inner focus: left/right→adjust, Enter/A→confirm, Escape/B→revert+exit
  useInputActions({
    enabled: !!isInnerFocused,
    captureInput: true,
    ignoreWhenInputFocused: false,
    onNavigate: (dir) => {
      if (dir === 'left') setLocalValue(v => Math.max(sliderMin, v - sliderStep));
      else if (dir === 'right') setLocalValue(v => Math.min(sliderMax, v + sliderStep));
    },
    onConfirm: () => {
      commitToApi(localValue);
      onExitInnerFocus?.();
    },
    onCancel: () => {
      setLocalValue(committedRef.current);
      onExitInnerFocus?.();
    },
  });

  return (
    <div
      className={`${(isChild || (isControl && !isFocused)) ? 'mt-2' : 'mt-4'} space-y-1`}
      onPointerEnter={() => onSliderActiveChange?.(true)}
      onPointerLeave={() => onSliderActiveChange?.(false)}
      onMouseDown={(e) => e.stopPropagation()}
      onTouchStart={(e) => e.stopPropagation()}
      onTouchMove={(e) => e.stopPropagation()}
    >
      {/* Label + value row with OK? badge */}
      <div className="relative flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
        <span className="font-medium truncate mr-2" title={sliderTargetParam}>
          {sliderTargetParam || 'value'}
        </span>
        <span className={`font-mono tabular-nums text-sm font-semibold ${isDirty ? 'text-yellow-700 dark:text-yellow-400' : 'text-gray-800 dark:text-gray-200'}`}>
          {localValue}
        </span>
        {isDirty && (
          <div className="absolute right-0 -top-5 text-[10px] font-medium text-yellow-700 bg-yellow-100 px-1.5 py-0.5 rounded shadow-sm border border-yellow-200 pointer-events-none">
            OK?
          </div>
        )}
      </div>
      <input
        type="range"
        min={sliderMin}
        max={sliderMax}
        step={sliderStep}
        value={localValue}
        onChange={(e) => handleMouseChange(Number(e.target.value))}
        onClick={(e) => e.stopPropagation()}
        className={`w-full h-2 rounded-lg appearance-none cursor-pointer ${isDirty ? 'accent-yellow-500 bg-yellow-100 dark:bg-yellow-900/30' : 'accent-blue-500 bg-gray-200 dark:bg-gray-700'}`}
      />
      <div className="flex justify-between text-[10px] text-gray-400 dark:text-gray-500">
        <span>{sliderMin}</span>
        <span>{sliderMax}</span>
      </div>
    </div>
  );
};

registerWantCardPlugin({
  types: ['slider'],
  ContentSection: SliderContentSection,
});
