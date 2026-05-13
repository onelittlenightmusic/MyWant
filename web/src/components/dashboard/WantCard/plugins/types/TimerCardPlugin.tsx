import React, { useState, useEffect, useRef } from 'react';
import { WantCardPluginProps, registerWantCardPlugin } from '../registry';
import { useInputActions } from '@/hooks/useInputActions';

// ── Every mode ────────────────────────────────────────────────────────────────

const EVERY_PRESETS = ['10s', '30s', '1m', '5m', '10m', '30m', '1h', '6h', '1d'];

const toSeconds = (s: string) => {
  if (s.endsWith('d')) return parseInt(s) * 86400;
  if (s.endsWith('h')) return parseInt(s) * 3600;
  if (s.endsWith('m')) return parseInt(s) * 60;
  if (s.endsWith('s')) return parseInt(s);
  return parseInt(s);
};

const START_DEG = -70;
const ARC_DEG = 270;
const LOG_VALS = EVERY_PRESETS.map((p) => Math.log(toSeconds(p)));
const LOG_MIN = LOG_VALS[0];
const LOG_RANGE = LOG_VALS[LOG_VALS.length - 1] - LOG_MIN;
const angleForEvery = (i: number) =>
  (START_DEG + ((LOG_VALS[i] - LOG_MIN) / LOG_RANGE) * ARC_DEG) * (Math.PI / 180);

const CX = 70, CY = 70, R_FACE = 56, R_TICK_E = 42, R_LABEL_E = 57;

// ── At mode ───────────────────────────────────────────────────────────────────

const HOUR_RING = [12, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11];
const MINUTE_RING = [0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55];
const WEEKDAYS = ['Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa', 'Su'];
const WEEKDAY_VALS = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'];

const R_AT_TICK = 44;
const R_AT_LABEL = 57;

const clockPos = (i: number, count: number, r: number) => {
  const angle = ((i / count) * 360 - 90) * (Math.PI / 180);
  return { x: CX + r * Math.cos(angle), y: CY + r * Math.sin(angle) };
};

const parseAt = (at: string) => {
  const parts = (at || '').split(':').map(Number);
  const h24 = isNaN(parts[0]) ? 9 : Math.max(0, Math.min(23, parts[0]));
  const min = isNaN(parts[1]) ? 0 : Math.max(0, Math.min(59, parts[1]));
  return { h24, min };
};

const formatAt = (h24: number, min: number) =>
  `${String(h24).padStart(2, '0')}:${String(min).padStart(2, '0')}`;

const to12h = (h24: number) => ({ isPM: h24 >= 12, h12: h24 % 12 === 0 ? 12 : h24 % 12 });

// ── Component ─────────────────────────────────────────────────────────────────

type TimerMode = 'every' | 'at';
type ClockMode = 'hour' | 'minute';

const TimerContentSection: React.FC<WantCardPluginProps> = ({
  want, isChild, isControl, isFocused, isInnerFocused, onExitInnerFocus,
}) => {
  const stateEvery = (want.state?.current?.every as string) || '';
  const stateAt = (want.state?.current?.at as string) || '';
  const stateTimerMode = (want.state?.current?.timer_mode as string) || 'every';
  const stateAtRecurrence = (want.state?.current?.at_recurrence as string) || '';
  const stateAtWeekday = (want.state?.current?.at_weekday as string) || '';
  const timerTargetParam = (want.state?.current?.target_param as string) || '';

  const [mode, setMode] = useState<TimerMode>(stateTimerMode === 'at' ? 'at' : 'every');
  const [localEvery, setLocalEvery] = useState(stateEvery);

  const parsedAt = parseAt(stateAt);
  const [atH24, setAtH24] = useState(parsedAt.h24);
  const [atMin, setAtMin] = useState(parsedAt.min);
  const { isPM: initIsPM } = to12h(parsedAt.h24);
  const [isPM, setIsPM] = useState(initIsPM);
  const [clockMode, setClockMode] = useState<ClockMode>('hour');
  const [atRecurrence, setAtRecurrence] = useState(stateAtRecurrence);
  const [atWeekday, setAtWeekday] = useState(stateAtWeekday);

  const everyDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const committedRef = useRef(stateEvery);

  useEffect(() => {
    if (!isInnerFocused) setLocalEvery(stateEvery);
  }, [stateEvery, isInnerFocused]);

  useEffect(() => {
    const p = parseAt(stateAt);
    setAtH24(p.h24);
    setAtMin(p.min);
    setIsPM(to12h(p.h24).isPM);
  }, [stateAt]);

  useEffect(() => { setAtRecurrence(stateAtRecurrence); }, [stateAtRecurrence]);
  useEffect(() => { setAtWeekday(stateAtWeekday); }, [stateAtWeekday]);
  useEffect(() => { setMode(stateTimerMode === 'at' ? 'at' : 'every'); }, [stateTimerMode]);
  useEffect(() => { if (isInnerFocused) committedRef.current = localEvery; }, [isInnerFocused]);

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

  const handleEveryClick = (preset: string) => {
    setLocalEvery(preset);
    if (isInnerFocused) onExitInnerFocus?.();
    updateStateKey('every', preset, everyDebounceRef);
  };

  const handleHourSelect = (idx: number) => {
    const h12 = HOUR_RING[idx];
    const newH24 = (h12 % 12) + (isPM ? 12 : 0);
    setAtH24(newH24);
    updateStateKey('at', formatAt(newH24, atMin));
    setClockMode('minute');
  };

  const handleMinuteSelect = (idx: number) => {
    const newMin = MINUTE_RING[idx];
    setAtMin(newMin);
    updateStateKey('at', formatAt(atH24, newMin));
  };

  const handleAmPmToggle = (newIsPM: boolean) => {
    if (newIsPM === isPM) return;
    setIsPM(newIsPM);
    const newH24 = (atH24 % 12) + (newIsPM ? 12 : 0);
    setAtH24(newH24);
    updateStateKey('at', formatAt(newH24, atMin));
  };

  const handleRecurrenceSelect = (r: string) => {
    const newR = atRecurrence === r ? '' : r;
    setAtRecurrence(newR);
    updateStateKey('at_recurrence', newR);
    if (newR !== 'week') {
      setAtWeekday('');
      updateStateKey('at_weekday', '');
    }
  };

  const handleWeekdaySelect = (wv: string) => {
    const newWd = atWeekday === wv ? '' : wv;
    setAtWeekday(newWd);
    updateStateKey('at_weekday', newWd);
  };

  // Every mode keyboard nav
  const isDirty = !!isInnerFocused && localEvery !== committedRef.current;
  useInputActions({
    enabled: !!isInnerFocused && mode === 'every',
    captureInput: true,
    ignoreWhenInputFocused: false,
    onNavigate: (dir) => {
      const idx = EVERY_PRESETS.indexOf(localEvery);
      if (dir === 'up' || dir === 'right') {
        const next = idx < 0 ? 0 : Math.min(EVERY_PRESETS.length - 1, idx + 1);
        setLocalEvery(EVERY_PRESETS[next]);
      } else {
        const next = idx < 0 ? EVERY_PRESETS.length - 1 : Math.max(0, idx - 1);
        setLocalEvery(EVERY_PRESETS[next]);
      }
    },
    onConfirm: () => { updateStateKey('every', localEvery); onExitInnerFocus?.(); },
    onCancel: () => { setLocalEvery(committedRef.current); onExitInnerFocus?.(); },
  });

  // Every mode geometry
  const selectedIdx = EVERY_PRESETS.indexOf(localEvery);
  const handAngle = selectedIdx >= 0 ? angleForEvery(selectedIdx) : START_DEG * (Math.PI / 180);
  const handX = CX + R_TICK_E * Math.cos(handAngle);
  const handY = CY + R_TICK_E * Math.sin(handAngle);
  const a0 = angleForEvery(0);
  const a1 = angleForEvery(EVERY_PRESETS.length - 1);
  const arcX0 = CX + R_FACE * Math.cos(a0), arcY0 = CY + R_FACE * Math.sin(a0);
  const arcX1 = CX + R_FACE * Math.cos(a1), arcY1 = CY + R_FACE * Math.sin(a1);

  // At mode geometry
  const { h12: displayH12 } = to12h(atH24);
  const atHourRingIdx = HOUR_RING.indexOf(displayH12);
  const roundedMin = Math.round(atMin / 5) * 5 % 60;
  const atMinRingIdx = MINUTE_RING.indexOf(roundedMin);
  const hourHandPos = clockPos(atHourRingIdx >= 0 ? atHourRingIdx : 0, 12, R_AT_TICK);
  const minuteHandPos = clockPos(atMinRingIdx >= 0 ? atMinRingIdx : 0, 12, R_AT_TICK);

  return (
    <div className={`${(isChild || (isControl && !isFocused)) ? 'mt-1' : 'mt-2'} space-y-2`}>

      {/* Mode toggle */}
      <div
        className="flex gap-0.5 p-0.5 bg-gray-100 dark:bg-gray-800 rounded-lg"
        onClick={(e) => e.stopPropagation()}
        onMouseDown={(e) => e.stopPropagation()}
      >
        {(['every', 'at'] as TimerMode[]).map((m) => (
          <button
            key={m}
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
        /* ── Every mode ── */
        <div className="flex items-center gap-2">
          <svg width="140" height="140" viewBox="0 0 140 140" className="flex-shrink-0">
            <circle cx={CX} cy={CY} r={R_FACE} fill="none" stroke={isDirty ? '#fbbf24' : '#e5e7eb'} strokeWidth="1.5" style={{ transition: 'stroke 0.2s' }} />
            <path
              d={`M ${arcX0} ${arcY0} A ${R_FACE} ${R_FACE} 0 ${ARC_DEG > 180 ? 1 : 0} 1 ${arcX1} ${arcY1}`}
              fill="none" stroke={isDirty ? '#fde68a' : '#d1d5db'} strokeWidth="2" strokeLinecap="round"
              style={{ transition: 'stroke 0.2s' }}
            />
            <text x={CX} y={CY - R_LABEL_E - 1} textAnchor="middle" dominantBaseline="auto"
              fontSize="7" fontFamily="monospace" fill="#9ca3af">0</text>
            <line x1={CX} y1={CY - R_FACE + 3} x2={CX} y2={CY - R_FACE + 8}
              stroke="#d1d5db" strokeWidth="1.5" strokeLinecap="round" />
            {EVERY_PRESETS.map((preset, idx) => {
              const angle = angleForEvery(idx);
              const tx = CX + R_TICK_E * Math.cos(angle);
              const ty = CY + R_TICK_E * Math.sin(angle);
              const lx = CX + R_LABEL_E * Math.cos(angle);
              const ly = CY + R_LABEL_E * Math.sin(angle);
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
            {selectedIdx >= 0 && (
              <line x1={CX} y1={CY} x2={handX} y2={handY}
                stroke={isDirty ? '#f59e0b' : '#3b82f6'} strokeWidth="2" strokeLinecap="round"
                style={{ transition: 'all 0.25s ease' }} />
            )}
            <circle cx={CX} cy={CY} r="3.5" fill={isDirty ? '#f59e0b' : '#3b82f6'} style={{ transition: 'fill 0.2s' }} />
          </svg>

          <div className="relative flex flex-col items-start gap-0.5 flex-1">
            {isDirty && (
              <div className="absolute right-0 -top-5 text-[10px] font-medium text-yellow-700 bg-yellow-100 px-1.5 py-0.5 rounded shadow-sm border border-yellow-200 pointer-events-none">
                OK?
              </div>
            )}
            <span className="text-[10px] text-gray-400 dark:text-gray-500 font-mono truncate leading-none" title={timerTargetParam}>
              {timerTargetParam || 'timer'}
            </span>
            <span className={`text-xl font-mono font-bold leading-tight ${isDirty ? 'text-yellow-500 dark:text-yellow-400' : 'text-blue-500 dark:text-blue-400'}`}
              style={{ transition: 'color 0.2s' }}>
              {localEvery || '--'}
            </span>
          </div>
        </div>
      ) : (
        /* ── At mode ── */
        <div className="space-y-2">
          <div className="flex items-start gap-2">
            {/* Clock face */}
            <svg width="140" height="140" viewBox="0 0 140 140" className="flex-shrink-0">
              <circle cx={CX} cy={CY} r={R_FACE} fill="none" stroke="#e5e7eb" strokeWidth="1.5" />

              {(clockMode === 'hour' ? HOUR_RING : MINUTE_RING).map((val, idx) => {
                const tickPos = clockPos(idx, 12, R_AT_TICK);
                const labelPos = clockPos(idx, 12, R_AT_LABEL);
                const isSel = clockMode === 'hour' ? val === displayH12 : val === roundedMin;
                const label = clockMode === 'hour' ? String(val) : String(val).padStart(2, '0');
                return (
                  <g key={`${clockMode}-${val}`} style={{ cursor: 'pointer' }}
                    onClick={(e) => {
                      e.stopPropagation();
                      if (clockMode === 'hour') handleHourSelect(idx);
                      else handleMinuteSelect(idx);
                    }}>
                    <circle cx={tickPos.x} cy={tickPos.y} r={11} fill="transparent" />
                    <circle cx={tickPos.x} cy={tickPos.y} r={isSel ? 5.5 : 3.5}
                      fill={isSel ? '#3b82f6' : '#9ca3af'}
                      style={{ transition: 'all 0.2s' }} />
                    <text x={labelPos.x} y={labelPos.y} textAnchor="middle" dominantBaseline="central"
                      fontSize={clockMode === 'hour' ? '9.5' : '8.5'} fontFamily="monospace"
                      fontWeight={isSel ? 'bold' : 'normal'}
                      fill={isSel ? '#3b82f6' : '#6b7280'}>
                      {label}
                    </text>
                  </g>
                );
              })}

              {/* Hands */}
              {clockMode === 'hour' ? (
                <line x1={CX} y1={CY} x2={hourHandPos.x} y2={hourHandPos.y}
                  stroke="#3b82f6" strokeWidth="2" strokeLinecap="round"
                  style={{ transition: 'all 0.25s ease' }} />
              ) : (
                <>
                  {/* Faded hour hand */}
                  <line
                    x1={CX} y1={CY}
                    x2={CX + (hourHandPos.x - CX) * 0.65}
                    y2={CY + (hourHandPos.y - CY) * 0.65}
                    stroke="#9ca3af" strokeWidth="1.5" strokeLinecap="round" />
                  {/* Minute hand */}
                  <line x1={CX} y1={CY} x2={minuteHandPos.x} y2={minuteHandPos.y}
                    stroke="#3b82f6" strokeWidth="2" strokeLinecap="round"
                    style={{ transition: 'all 0.25s ease' }} />
                </>
              )}
              <circle cx={CX} cy={CY} r="3.5" fill="#3b82f6" />
            </svg>

            {/* Right side controls */}
            <div className="flex flex-col gap-1.5 flex-1 pt-1">
              {/* Digital time display */}
              <div className="text-center">
                <span className="text-lg font-mono font-bold text-blue-500 dark:text-blue-400">
                  {`${String(displayH12).padStart(2, '0')}:${String(atMin).padStart(2, '0')}`}
                </span>
              </div>

              {/* H / M toggle */}
              <div
                className="flex gap-0.5 p-0.5 bg-gray-100 dark:bg-gray-800 rounded"
                onClick={(e) => e.stopPropagation()}
                onMouseDown={(e) => e.stopPropagation()}
              >
                {(['hour', 'minute'] as ClockMode[]).map((cm) => (
                  <button key={cm}
                    onClick={(e) => { e.stopPropagation(); setClockMode(cm); }}
                    onMouseDown={(e) => e.stopPropagation()}
                    className={`flex-1 text-[10px] font-mono py-0.5 rounded transition-all ${
                      clockMode === cm
                        ? 'bg-white dark:bg-gray-700 text-blue-600 dark:text-blue-400 shadow-sm'
                        : 'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300'
                    }`}
                  >
                    {cm === 'hour' ? 'H' : 'M'}
                  </button>
                ))}
              </div>

              {/* AM / PM toggle */}
              <div
                className="flex gap-0.5 p-0.5 bg-gray-100 dark:bg-gray-800 rounded"
                onClick={(e) => e.stopPropagation()}
                onMouseDown={(e) => e.stopPropagation()}
              >
                {(['AM', 'PM'] as const).map((ampm) => (
                  <button key={ampm}
                    onClick={(e) => { e.stopPropagation(); handleAmPmToggle(ampm === 'PM'); }}
                    onMouseDown={(e) => e.stopPropagation()}
                    className={`flex-1 text-[10px] font-mono py-0.5 rounded transition-all ${
                      (ampm === 'PM') === isPM
                        ? 'bg-white dark:bg-gray-700 text-blue-600 dark:text-blue-400 shadow-sm'
                        : 'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300'
                    }`}
                  >
                    {ampm}
                  </button>
                ))}
              </div>

              <span className="text-[10px] text-gray-400 dark:text-gray-500 font-mono truncate leading-none" title={timerTargetParam}>
                {timerTargetParam || 'timer'}
              </span>
            </div>
          </div>

          {/* Recurrence selector */}
          <div
            className="space-y-1"
            onClick={(e) => e.stopPropagation()}
            onMouseDown={(e) => e.stopPropagation()}
          >
            <div className="flex gap-1">
              {[{ label: 'every day', val: 'day' }, { label: 'every week', val: 'week' }].map(({ label, val }) => (
                <button key={val}
                  onClick={(e) => { e.stopPropagation(); handleRecurrenceSelect(val); }}
                  onMouseDown={(e) => e.stopPropagation()}
                  className={`flex-1 text-[10px] font-mono py-0.5 px-1 border rounded transition-all ${
                    atRecurrence === val
                      ? 'border-blue-400 bg-blue-50 dark:bg-blue-900/30 text-blue-600 dark:text-blue-400'
                      : 'border-gray-300 dark:border-gray-600 text-gray-500 dark:text-gray-400 hover:border-gray-400'
                  }`}
                >
                  {label}
                </button>
              ))}
            </div>

            {atRecurrence === 'week' && (
              <div className="flex gap-0.5">
                {WEEKDAYS.map((day, i) => (
                  <button key={day}
                    onClick={(e) => { e.stopPropagation(); handleWeekdaySelect(WEEKDAY_VALS[i]); }}
                    onMouseDown={(e) => e.stopPropagation()}
                    className={`flex-1 text-[9px] font-mono py-0.5 border rounded transition-all ${
                      atWeekday === WEEKDAY_VALS[i]
                        ? 'border-blue-400 bg-blue-50 dark:bg-blue-900/30 text-blue-600 dark:text-blue-400'
                        : 'border-gray-200 dark:border-gray-700 text-gray-400 dark:text-gray-500 hover:border-gray-400'
                    }`}
                  >
                    {day}
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
};

registerWantCardPlugin({
  types: ['timer'],
  ContentSection: TimerContentSection,
});
