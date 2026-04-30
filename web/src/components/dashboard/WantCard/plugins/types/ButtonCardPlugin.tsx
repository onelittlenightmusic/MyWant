import React, { useState, useEffect } from 'react';
import { WantCardPluginProps, registerWantCardPlugin } from '../registry';

const ButtonContentSection: React.FC<WantCardPluginProps> = ({
  want, isChild, isControl, isFocused,
}) => {
  const pressedCount = typeof want.state?.current?.pressed_count === 'number'
    ? want.state.current.pressed_count : 0;
  const label = (want.state?.current?.label as string)
    || (want.spec?.params?.label as string)
    || 'Push';

  const [isPressed, setIsPressed] = useState(false);
  const [localCount, setLocalCount] = useState(pressedCount);

  useEffect(() => { setLocalCount(prev => Math.max(prev, pressedCount)); }, [pressedCount]);

  const handlePress = async () => {
    if (isPressed) return;
    setIsPressed(true);
    setLocalCount(c => c + 1);
    setTimeout(() => setIsPressed(false), 150);

    const id = want.metadata?.id;
    if (!id) return;
    try {
      await fetch(`/api/v1/webhooks/${id}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action: 'press' }),
      });
    } catch (err) {
      console.error('[ButtonCard] press event failed:', err);
    }
  };

  const compact = isChild || (isControl && !isFocused);
  const size = compact ? 44 : 54;
  const bezelPad = compact ? 4 : 6;

  return (
    <div
      className={`${compact ? 'mt-2' : 'mt-4'} flex flex-col items-center gap-1.5`}
      onMouseDown={(e) => e.stopPropagation()}
      onTouchStart={(e) => e.stopPropagation()}
    >
      <div style={{
        width: size + bezelPad * 2,
        height: size + bezelPad * 2,
        borderRadius: '50%',
        background: 'radial-gradient(circle at 45% 35%, #374151, #111827)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        boxShadow: '0 4px 12px rgba(0,0,0,0.4), inset 0 1px 2px rgba(255,255,255,0.06)',
        padding: bezelPad,
      }}>
        <button
          onClick={(e) => { e.stopPropagation(); handlePress(); }}
          onMouseDown={(e) => e.stopPropagation()}
          className="rounded-full bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 font-semibold cursor-pointer select-none"
          style={{
            width: size,
            height: size,
            fontSize: compact ? 9 : 10,
            letterSpacing: '0.03em',
            boxShadow: isPressed
              ? 'inset 0 2px 4px rgba(0,0,0,0.18)'
              : '0 2px 0 rgba(0,0,0,0.12), 0 1px 3px rgba(0,0,0,0.08)',
            transform: isPressed ? 'translateY(2px)' : 'translateY(0)',
            transition: 'transform 0.08s ease, box-shadow 0.08s ease',
          }}
        >
          {label}
        </button>
      </div>

      <div className="flex items-center gap-1 text-[10px] text-gray-400 dark:text-gray-500 tabular-nums">
        <span>×</span>
        <span>{localCount}</span>
      </div>
    </div>
  );
};

registerWantCardPlugin({
  types: ['button'],
  ContentSection: ButtonContentSection,
});
