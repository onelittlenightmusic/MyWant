import React, { useState, useEffect, useRef, useCallback } from 'react';
import { WantCardPluginProps, registerWantCardPlugin } from '../registry';
import { useInputActions } from '@/hooks/useInputActions';
import { NumberSliderInput } from '@/components/common/NumberSliderInput';

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
      className={`${(isChild || (isControl && !isFocused)) ? 'mt-2' : 'mt-4'}`}
      onPointerEnter={() => onSliderActiveChange?.(true)}
      onPointerLeave={() => onSliderActiveChange?.(false)}
      onMouseDown={(e) => e.stopPropagation()}
      onTouchStart={(e) => e.stopPropagation()}
      onTouchMove={(e) => e.stopPropagation()}
    >
      <NumberSliderInput
        value={localValue}
        min={sliderMin}
        max={sliderMax}
        step={sliderStep}
        onChange={handleMouseChange}
        isDirty={isDirty}
        label={sliderTargetParam || 'value'}
        stopPropagation
      />
    </div>
  );
};

registerWantCardPlugin({
  types: ['slider'],
  ContentSection: SliderContentSection,
});
