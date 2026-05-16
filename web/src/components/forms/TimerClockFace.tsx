import React, { useState, useEffect } from 'react';
import {
  CX, CY, R_FACE, R_AT_TICK, R_AT_LABEL,
  HOUR_RING, MINUTE_RING, WEEKDAYS, WEEKDAY_VALS,
  clockPos, formatAt, to12h,
  ClockMode,
} from './timerUtils';

export interface TimerClockFaceProps {
  atH24: number;
  atMin: number;
  atRecurrence: string;
  atWeekday?: string;
  onAtChange: (at: string) => void;
  onRecurrenceChange: (r: string) => void;
  onAtWeekdayChange?: (wd: string) => void;
  /** Stop click/mousedown propagation (needed inside want cards) */
  stopPropagation?: boolean;
  /** Extra content below the AM/PM toggle (e.g. timerTargetParam label) */
  rightExtra?: React.ReactNode;
}

export const TimerClockFace: React.FC<TimerClockFaceProps> = ({
  atH24, atMin, atRecurrence, atWeekday = '',
  onAtChange, onRecurrenceChange, onAtWeekdayChange,
  stopPropagation = false, rightExtra,
}) => {
  const [clockMode, setClockMode] = useState<ClockMode>('hour');
  const [isPM, setIsPM] = useState(to12h(atH24).isPM);

  useEffect(() => { setIsPM(to12h(atH24).isPM); }, [atH24]);

  const stop = (e: React.MouseEvent) => { if (stopPropagation) e.stopPropagation(); };
  const sp = stopPropagation
    ? { onClick: (e: React.MouseEvent) => e.stopPropagation(), onMouseDown: (e: React.MouseEvent) => e.stopPropagation() }
    : {};

  const { h12: displayH12 } = to12h(atH24);
  const atHourRingIdx = HOUR_RING.indexOf(displayH12);
  const roundedMin = Math.round(atMin / 5) * 5 % 60;
  const atMinRingIdx = MINUTE_RING.indexOf(roundedMin);
  const hourHandPos   = clockPos(atHourRingIdx >= 0 ? atHourRingIdx : 0, 12, R_AT_TICK);
  const minuteHandPos = clockPos(atMinRingIdx  >= 0 ? atMinRingIdx  : 0, 12, R_AT_TICK);

  const handleHourSelect = (idx: number) => {
    const h12 = HOUR_RING[idx];
    const newH24 = (h12 % 12) + (isPM ? 12 : 0);
    onAtChange(formatAt(newH24, atMin));
    setClockMode('minute');
  };

  const handleMinuteSelect = (idx: number) => {
    onAtChange(formatAt(atH24, MINUTE_RING[idx]));
  };

  const handleAmPmToggle = (newIsPM: boolean) => {
    if (newIsPM === isPM) return;
    setIsPM(newIsPM);
    onAtChange(formatAt((atH24 % 12) + (newIsPM ? 12 : 0), atMin));
  };

  const handleRecurrenceSelect = (r: string) => {
    const newR = atRecurrence === r ? '' : r;
    onRecurrenceChange(newR);
    if (newR !== 'week') onAtWeekdayChange?.('');
  };

  return (
    <div className="space-y-2">
      <div className="flex items-start gap-2">
        <svg width="140" height="140" viewBox="0 0 140 140" className="flex-shrink-0">
          <circle cx={CX} cy={CY} r={R_FACE} fill="none" stroke="#e5e7eb" strokeWidth="1.5" />
          {(clockMode === 'hour' ? HOUR_RING : MINUTE_RING).map((val, idx) => {
            const tickPos  = clockPos(idx, 12, R_AT_TICK);
            const labelPos = clockPos(idx, 12, R_AT_LABEL);
            const isSel = clockMode === 'hour' ? val === displayH12 : val === roundedMin;
            const label = clockMode === 'hour' ? String(val) : String(val).padStart(2, '0');
            return (
              <g key={`${clockMode}-${val}`} style={{ cursor: 'pointer' }}
                onClick={(e) => {
                  stop(e);
                  if (clockMode === 'hour') handleHourSelect(idx);
                  else handleMinuteSelect(idx);
                }}>
                <circle cx={tickPos.x} cy={tickPos.y} r={11} fill="transparent" />
                <circle cx={tickPos.x} cy={tickPos.y} r={isSel ? 5.5 : 3.5}
                  fill={isSel ? '#3b82f6' : '#9ca3af'} style={{ transition: 'all 0.2s' }} />
                <text x={labelPos.x} y={labelPos.y} textAnchor="middle" dominantBaseline="central"
                  fontSize={clockMode === 'hour' ? '9.5' : '8.5'} fontFamily="monospace"
                  fontWeight={isSel ? 'bold' : 'normal'}
                  fill={isSel ? '#3b82f6' : '#6b7280'}>
                  {label}
                </text>
              </g>
            );
          })}
          {clockMode === 'hour' ? (
            <line x1={CX} y1={CY} x2={hourHandPos.x} y2={hourHandPos.y}
              stroke="#3b82f6" strokeWidth="2" strokeLinecap="round"
              style={{ transition: 'all 0.25s ease' }} />
          ) : (
            <>
              <line x1={CX} y1={CY}
                x2={CX + (hourHandPos.x - CX) * 0.65}
                y2={CY + (hourHandPos.y - CY) * 0.65}
                stroke="#9ca3af" strokeWidth="1.5" strokeLinecap="round" />
              <line x1={CX} y1={CY} x2={minuteHandPos.x} y2={minuteHandPos.y}
                stroke="#3b82f6" strokeWidth="2" strokeLinecap="round"
                style={{ transition: 'all 0.25s ease' }} />
            </>
          )}
          <circle cx={CX} cy={CY} r="3.5" fill="#3b82f6" />
        </svg>

        <div className="flex flex-col gap-1.5 flex-1 pt-1" {...sp}>
          <div className="text-center">
            <span className="text-lg font-mono font-bold text-blue-500 dark:text-blue-400">
              {`${String(displayH12).padStart(2, '0')}:${String(atMin).padStart(2, '0')}`}
            </span>
          </div>
          <div className="flex gap-0.5 p-0.5 bg-gray-100 dark:bg-gray-800 rounded" {...sp}>
            {(['hour', 'minute'] as ClockMode[]).map((cm) => (
              <button key={cm} type="button"
                onClick={(e) => { stop(e); setClockMode(cm); }}
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
          <div className="flex gap-0.5 p-0.5 bg-gray-100 dark:bg-gray-800 rounded" {...sp}>
            {(['AM', 'PM'] as const).map((ampm) => (
              <button key={ampm} type="button"
                onClick={(e) => { stop(e); handleAmPmToggle(ampm === 'PM'); }}
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
          {rightExtra}
        </div>
      </div>

      <div className="space-y-1" {...sp}>
        <div className="flex gap-1">
          {[{ label: 'every day', val: 'day' }, { label: 'every week', val: 'week' }].map(({ label, val }) => (
            <button key={val} type="button"
              onClick={(e) => { stop(e); handleRecurrenceSelect(val); }}
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
        {atRecurrence === 'week' && onAtWeekdayChange && (
          <div className="flex gap-0.5">
            {WEEKDAYS.map((day, i) => (
              <button key={day} type="button"
                onClick={(e) => { stop(e); onAtWeekdayChange(atWeekday === WEEKDAY_VALS[i] ? '' : WEEKDAY_VALS[i]); }}
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
  );
};
