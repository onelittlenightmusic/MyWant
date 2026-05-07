import React, { useState, useEffect, useRef } from 'react';
import { WantCardPluginProps, registerWantCardPlugin } from '../registry';
import { useInputActions } from '@/hooks/useInputActions';

const PRESETS = ['1m', '5m', '10m', '30m', '1h', '6h', '1d'];

const toSeconds = (s: string) => {
  if (s.endsWith('d')) return parseInt(s) * 86400;
  if (s.endsWith('h')) return parseInt(s) * 3600;
  if (s.endsWith('m')) return parseInt(s) * 60;
  return parseInt(s);
};

const START_DEG = -70;
const ARC_DEG = 270;
const LOG_VALS = PRESETS.map((p) => Math.log(toSeconds(p)));
const LOG_MIN = LOG_VALS[0];
const LOG_RANGE = LOG_VALS[LOG_VALS.length - 1] - LOG_MIN;

const angleFor = (i: number) =>
  (START_DEG + ((LOG_VALS[i] - LOG_MIN) / LOG_RANGE) * ARC_DEG) * (Math.PI / 180);

const CX = 70, CY = 70, R_FACE = 56, R_TICK = 42, R_LABEL = 57;

const TimerContentSection: React.FC<WantCardPluginProps> = ({
  want, isChild, isControl, isFocused, isInnerFocused, onExitInnerFocus,
}) => {
  const timerEvery = (want.state?.current?.every as string) || '';
  const timerAt = (want.state?.current?.at as string) || '';
  const timerTargetParam = (want.state?.current?.target_param as string) || '';

  const [localEvery, setLocalEvery] = useState(timerEvery);
  const [localAt, setLocalAt] = useState(timerAt);
  const everyDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const atDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Value when inner focus started — used to revert on cancel
  const committedRef = useRef(timerEvery);

  // Sync from server unless inner-focused
  useEffect(() => {
    if (!isInnerFocused) setLocalEvery(timerEvery);
  }, [timerEvery, isInnerFocused]);

  useEffect(() => { setLocalAt(timerAt); }, [timerAt]);

  // Capture committed value when entering inner focus
  useEffect(() => {
    if (isInnerFocused) committedRef.current = localEvery;
  }, [isInnerFocused]);

  const updateState = (key: string, value: string, debounceRef: React.MutableRefObject<ReturnType<typeof setTimeout> | null>) => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(async () => {
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
    }, 400);
  };

  const commitEvery = async (value: string) => {
    const id = want.metadata?.id;
    if (!id) return;
    try {
      await fetch(`/api/v1/states/${id}/every`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(value),
      });
    } catch (err) {
      console.error('[TimerCard] every update failed:', err);
    }
  };

  // Click on preset: immediate commit (mouse/touch, no inner focus needed)
  const handleEveryClick = (preset: string) => {
    setLocalEvery(preset);
    if (isInnerFocused) onExitInnerFocus?.();
    updateState('every', preset, everyDebounceRef);
  };

  const handleAtChange = (value: string) => {
    setLocalAt(value);
    updateState('at', value, atDebounceRef);
  };

  const isDirty = !!isInnerFocused && localEvery !== committedRef.current;

  // Inner focus: up/down→cycle presets, Enter/A→confirm, Escape/B→revert+exit
  useInputActions({
    enabled: !!isInnerFocused,
    captureInput: true,
    ignoreWhenInputFocused: false,
    onNavigate: (dir) => {
      const idx = PRESETS.indexOf(localEvery);
      if (dir === 'up' || dir === 'right') {
        const next = idx < 0 ? 0 : Math.min(PRESETS.length - 1, idx + 1);
        setLocalEvery(PRESETS[next]);
      } else if (dir === 'down' || dir === 'left') {
        const next = idx < 0 ? PRESETS.length - 1 : Math.max(0, idx - 1);
        setLocalEvery(PRESETS[next]);
      }
    },
    onConfirm: () => {
      commitEvery(localEvery);
      onExitInnerFocus?.();
    },
    onCancel: () => {
      setLocalEvery(committedRef.current);
      onExitInnerFocus?.();
    },
  });

  const selectedIdx = PRESETS.indexOf(localEvery);
  const handAngle = selectedIdx >= 0 ? angleFor(selectedIdx) : START_DEG * (Math.PI / 180);
  const handX = CX + R_TICK * Math.cos(handAngle);
  const handY = CY + R_TICK * Math.sin(handAngle);

  const a0 = angleFor(0);
  const a1 = angleFor(PRESETS.length - 1);
  const arcX0 = CX + R_FACE * Math.cos(a0), arcY0 = CY + R_FACE * Math.sin(a0);
  const arcX1 = CX + R_FACE * Math.cos(a1), arcY1 = CY + R_FACE * Math.sin(a1);

  return (
    <div className={`${(isChild || (isControl && !isFocused)) ? 'mt-1' : 'mt-2'} space-y-1`}>
      <div className="flex items-center gap-2">
        {/* Clock dial */}
        <svg width="140" height="140" viewBox="0 0 140 140" className="flex-shrink-0">
          {/* Face */}
          <circle cx={CX} cy={CY} r={R_FACE} fill="none" stroke={isDirty ? '#fbbf24' : '#e5e7eb'} strokeWidth="1.5" style={{ transition: 'stroke 0.2s' }} />
          {/* Active arc */}
          <path
            d={`M ${arcX0} ${arcY0} A ${R_FACE} ${R_FACE} 0 ${ARC_DEG > 180 ? 1 : 0} 1 ${arcX1} ${arcY1}`}
            fill="none" stroke={isDirty ? '#fde68a' : '#d1d5db'} strokeWidth="2" strokeLinecap="round"
            style={{ transition: 'stroke 0.2s' }}
          />
          {/* 0 marker at top */}
          <text x={CX} y={CY - R_LABEL - 1} textAnchor="middle" dominantBaseline="auto"
            fontSize="7" fontFamily="monospace" fill="#9ca3af">0</text>
          <line x1={CX} y1={CY - R_FACE + 3} x2={CX} y2={CY - R_FACE + 8}
            stroke="#d1d5db" strokeWidth="1.5" strokeLinecap="round" />
          {/* Preset ticks + labels */}
          {PRESETS.map((preset, idx) => {
            const angle = angleFor(idx);
            const tx = CX + R_TICK * Math.cos(angle);
            const ty = CY + R_TICK * Math.sin(angle);
            const lx = CX + R_LABEL * Math.cos(angle);
            const ly = CY + R_LABEL * Math.sin(angle);
            const isSel = preset === localEvery;
            return (
              <g key={preset} style={{ cursor: 'pointer' }}
                onClick={(e) => { e.stopPropagation(); handleEveryClick(preset); }}>
                <circle cx={tx} cy={ty} r={10} fill="transparent" />
                <circle cx={tx} cy={ty} r={isSel ? 5.5 : 3.5}
                  fill={isSel ? (isDirty ? '#f59e0b' : '#3b82f6') : '#9ca3af'}
                  style={{ transition: 'all 0.2s' }} />
                <text x={lx} y={ly} textAnchor="middle" dominantBaseline="central"
                  fontSize="9.5" fontFamily="monospace"
                  fontWeight={isSel ? 'bold' : 'normal'}
                  fill={isSel ? (isDirty ? '#d97706' : '#3b82f6') : '#6b7280'}>
                  {preset}
                </text>
              </g>
            );
          })}
          {/* Hand */}
          {selectedIdx >= 0 && (
            <line x1={CX} y1={CY} x2={handX} y2={handY}
              stroke={isDirty ? '#f59e0b' : '#3b82f6'} strokeWidth="2" strokeLinecap="round"
              style={{ transition: 'all 0.25s ease' }} />
          )}
          {/* Center pivot */}
          <circle cx={CX} cy={CY} r="3.5" fill={isDirty ? '#f59e0b' : '#3b82f6'} style={{ transition: 'fill 0.2s' }} />
        </svg>

        {/* Value display with OK? badge */}
        <div className="relative flex flex-col items-start gap-0.5 flex-1">
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
          {localAt && (
            <span className="text-[10px] font-mono text-gray-500 dark:text-gray-400">
              @ {localAt}
            </span>
          )}
        </div>
      </div>

      {/* at input */}
      <input
        type="text"
        value={localAt}
        onChange={(e) => handleAtChange(e.target.value)}
        onClick={(e) => e.stopPropagation()}
        onMouseDown={(e) => e.stopPropagation()}
        placeholder="at (optional, e.g. 09:00)"
        className="w-full px-2 py-0.5 text-[10px] font-mono border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-1 focus:ring-blue-500"
      />
    </div>
  );
};

registerWantCardPlugin({
  types: ['timer'],
  ContentSection: TimerContentSection,
});
