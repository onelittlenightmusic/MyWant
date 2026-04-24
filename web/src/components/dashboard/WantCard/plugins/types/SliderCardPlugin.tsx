import React, { useState, useEffect, useRef } from 'react';
import { WantCardPluginProps, registerWantCardPlugin } from '../registry';

const SliderContentSection: React.FC<WantCardPluginProps> = ({
  want, isChild, isControl, isFocused, onSliderActiveChange,
}) => {
  const sliderValue = typeof want.state?.current?.value === 'number' ? want.state.current.value : 0;
  const sliderMin = typeof want.state?.current?.min === 'number' ? want.state.current.min : 0;
  const sliderMax = typeof want.state?.current?.max === 'number' ? want.state.current.max : 100;
  const sliderStep = typeof want.state?.current?.step === 'number' ? want.state.current.step : 1;
  const sliderTargetParam = (want.state?.current?.target_param as string) || '';

  const [localValue, setLocalValue] = useState(sliderValue);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => { setLocalValue(sliderValue); }, [sliderValue]);

  const handleChange = (newValue: number) => {
    setLocalValue(newValue);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(async () => {
      const id = want.metadata?.id;
      if (!id) return;
      try {
        await fetch(`/api/v1/states/${id}/value`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(newValue),
        });
      } catch (err) {
        console.error('[SliderCard] state update failed:', err);
      }
    }, 150);
  };

  return (
    <div
      className={`${(isChild || (isControl && !isFocused)) ? 'mt-2' : 'mt-4'} space-y-1`}
      onPointerEnter={() => onSliderActiveChange?.(true)}
      onPointerLeave={() => onSliderActiveChange?.(false)}
      onMouseDown={(e) => e.stopPropagation()}
      onTouchStart={(e) => e.stopPropagation()}
      onTouchMove={(e) => e.stopPropagation()}
    >
      <div className="flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
        <span className="font-medium truncate mr-2" title={sliderTargetParam}>
          {sliderTargetParam || 'value'}
        </span>
        <span className="font-mono tabular-nums text-sm font-semibold text-gray-800 dark:text-gray-200">
          {localValue}
        </span>
      </div>
      <input
        type="range"
        min={sliderMin}
        max={sliderMax}
        step={sliderStep}
        value={localValue}
        onChange={(e) => handleChange(Number(e.target.value))}
        onClick={(e) => e.stopPropagation()}
        className="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded-lg appearance-none cursor-pointer accent-blue-500"
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
