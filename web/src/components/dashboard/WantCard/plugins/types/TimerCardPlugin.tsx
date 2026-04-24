import React, { useState, useEffect, useRef } from 'react';
import { WantCardPluginProps, registerWantCardPlugin } from '../registry';

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
  want, isChild, isControl, isFocused,
}) => {
  const timerEvery = (want.state?.current?.every as string) || '';
  const timerAt = (want.state?.current?.at as string) || '';
  const timerTargetParam = (want.state?.current?.target_param as string) || '';

  const [localEvery, setLocalEvery] = useState(timerEvery);
  const [localAt, setLocalAt] = useState(timerAt);
  const everyDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const atDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => { setLocalEvery(timerEvery); }, [timerEvery]);
  useEffect(() => { setLocalAt(timerAt); }, [timerAt]);

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

  const handleEveryChange = (value: string) => {
    setLocalEvery(value);
    updateState('every', value, everyDebounceRef);
  };

  const handleAtChange = (value: string) => {
    setLocalAt(value);
    updateState('at', value, atDebounceRef);
  };

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
          <circle cx={CX} cy={CY} r={R_FACE} fill="none" stroke="#e5e7eb" strokeWidth="1.5" />
          {/* Active arc */}
          <path
            d={`M ${arcX0} ${arcY0} A ${R_FACE} ${R_FACE} 0 ${ARC_DEG > 180 ? 1 : 0} 1 ${arcX1} ${arcY1}`}
            fill="none" stroke="#d1d5db" strokeWidth="2" strokeLinecap="round"
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
                onClick={(e) => { e.stopPropagation(); handleEveryChange(preset); }}>
                <circle cx={tx} cy={ty} r={10} fill="transparent" />
                <circle cx={tx} cy={ty} r={isSel ? 5.5 : 3.5}
                  fill={isSel ? '#3b82f6' : '#9ca3af'}
                  style={{ transition: 'all 0.2s' }} />
                <text x={lx} y={ly} textAnchor="middle" dominantBaseline="central"
                  fontSize="9.5" fontFamily="monospace"
                  fontWeight={isSel ? 'bold' : 'normal'}
                  fill={isSel ? '#3b82f6' : '#6b7280'}>
                  {preset}
                </text>
              </g>
            );
          })}
          {/* Hand */}
          {selectedIdx >= 0 && (
            <line x1={CX} y1={CY} x2={handX} y2={handY}
              stroke="#3b82f6" strokeWidth="2" strokeLinecap="round"
              style={{ transition: 'all 0.25s ease' }} />
          )}
          {/* Center pivot */}
          <circle cx={CX} cy={CY} r="3.5" fill="#3b82f6" />
        </svg>

        {/* Value display */}
        <div className="flex flex-col items-start gap-0.5 flex-1">
          <span className="text-[10px] text-gray-400 dark:text-gray-500 font-mono truncate leading-none"
            title={timerTargetParam}>
            {timerTargetParam || 'timer'}
          </span>
          <span className="text-xl font-mono font-bold text-blue-500 dark:text-blue-400 leading-tight">
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
