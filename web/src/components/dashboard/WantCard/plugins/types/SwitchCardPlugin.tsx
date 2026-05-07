import React, { useState, useEffect } from 'react';
import { WantCardPluginProps, registerWantCardPlugin } from '../registry';
import { useInputActions } from '@/hooks/useInputActions';

const SwitchContentSection: React.FC<WantCardPluginProps> = ({
  want, isChild, isControl, isFocused, isInnerFocused, onExitInnerFocus,
}) => {
  const serverOn = want.state?.current?.on === true;
  const label = (want.state?.current?.label as string)
    || (want.spec?.params?.label as string)
    || 'Switch';

  const [localOn, setLocalOn] = useState(serverOn);
  const [pending, setPending] = useState(false);

  useEffect(() => { setLocalOn(serverOn); }, [serverOn]);

  const handleToggle = async () => {
    if (pending) return;
    const next = !localOn;
    setLocalOn(next);
    setPending(true);

    const id = want.metadata?.id;
    if (!id) { setPending(false); return; }
    try {
      await fetch(`/api/v1/webhooks/${id}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action: next ? 'on' : 'off' }),
      });
    } catch (err) {
      console.error('[SwitchCard] toggle failed:', err);
      setLocalOn(!next);
    } finally {
      setPending(false);
    }
  };

  // Gamepad/keyboard inner focus: A→toggle, B→exit
  useInputActions({
    enabled: !!isInnerFocused,
    captureInput: true,
    ignoreWhenInputFocused: false,
    onConfirm: handleToggle,
    onCancel: onExitInnerFocus,
  });

  const compact = isChild || (isControl && !isFocused);

  return (
    <div
      className={`${compact ? 'mt-2' : 'mt-4'} flex flex-col items-center gap-2`}
      onMouseDown={(e) => e.stopPropagation()}
      onTouchStart={(e) => e.stopPropagation()}
    >
      <div className={isInnerFocused ? 'ring-2 ring-yellow-400 ring-offset-1 rounded-full' : undefined}>
        <button
          onClick={(e) => { e.stopPropagation(); handleToggle(); }}
          onMouseDown={(e) => e.stopPropagation()}
          disabled={pending}
          className="relative focus:outline-none"
          aria-label={label}
          style={{ opacity: pending ? 0.7 : 1 }}
        >
          <div
            style={{
              width: compact ? 44 : 56,
              height: compact ? 24 : 30,
              borderRadius: 999,
              background: localOn
                ? 'linear-gradient(135deg, #22c55e, #16a34a)'
                : 'linear-gradient(135deg, #6b7280, #4b5563)',
              boxShadow: localOn
                ? '0 0 8px rgba(34,197,94,0.4), inset 0 1px 2px rgba(0,0,0,0.15)'
                : 'inset 0 1px 3px rgba(0,0,0,0.25)',
              position: 'relative',
              transition: 'background 0.2s ease',
            }}
          >
            <div
              style={{
                position: 'absolute',
                top: compact ? 3 : 4,
                left: localOn ? (compact ? 23 : 29) : (compact ? 3 : 4),
                width: compact ? 18 : 22,
                height: compact ? 18 : 22,
                borderRadius: '50%',
                background: '#ffffff',
                boxShadow: '0 1px 4px rgba(0,0,0,0.3)',
                transition: 'left 0.18s ease',
              }}
            />
          </div>
        </button>
      </div>
      <div className="text-[10px] text-gray-400 dark:text-gray-500 font-medium tracking-wide">
        {localOn ? 'ON' : 'OFF'}
      </div>
    </div>
  );
};

registerWantCardPlugin({
  types: ['switch'],
  ContentSection: SwitchContentSection,
});
