import React from 'react';
import {
  EVERY_PRESETS, CX, CY, R_FACE, R_TICK_E, R_LABEL_E,
  ARC_DEG, START_DEG, angleForEvery,
} from './timerUtils';

export interface TimerEveryDialProps {
  every: string;
  /** Amber accent when true (e.g. uncommitted dirty state in want card) */
  isDirty?: boolean;
  onSelect: (preset: string) => void;
  /** Stop click/mousedown event propagation (needed inside want cards) */
  stopPropagation?: boolean;
  /** Content rendered to the right of the SVG dial */
  rightSlot?: React.ReactNode;
}

export const TimerEveryDial: React.FC<TimerEveryDialProps> = ({
  every, isDirty = false, onSelect, stopPropagation = false, rightSlot,
}) => {
  const selectedIdx = EVERY_PRESETS.indexOf(every);
  const handAngle = selectedIdx >= 0 ? angleForEvery(selectedIdx) : START_DEG * (Math.PI / 180);
  const handX = CX + R_TICK_E * Math.cos(handAngle);
  const handY = CY + R_TICK_E * Math.sin(handAngle);
  const a0 = angleForEvery(0);
  const a1 = angleForEvery(EVERY_PRESETS.length - 1);
  const arcX0 = CX + R_FACE * Math.cos(a0), arcY0 = CY + R_FACE * Math.sin(a0);
  const arcX1 = CX + R_FACE * Math.cos(a1), arcY1 = CY + R_FACE * Math.sin(a1);

  const accent   = isDirty ? '#f59e0b' : '#3b82f6';
  const arcStroke = isDirty ? '#fde68a' : '#d1d5db';
  const ringStroke = isDirty ? '#fbbf24' : '#e5e7eb';

  const stop = (e: React.MouseEvent) => { if (stopPropagation) e.stopPropagation(); };

  return (
    <div className="flex items-center gap-2">
      <svg width="140" height="140" viewBox="0 0 140 140" className="flex-shrink-0">
        <circle cx={CX} cy={CY} r={R_FACE} fill="none" stroke={ringStroke} strokeWidth="1.5"
          style={{ transition: 'stroke 0.2s' }} />
        <path
          d={`M ${arcX0} ${arcY0} A ${R_FACE} ${R_FACE} 0 ${ARC_DEG > 180 ? 1 : 0} 1 ${arcX1} ${arcY1}`}
          fill="none" stroke={arcStroke} strokeWidth="2" strokeLinecap="round"
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
          const isSel = preset === every;
          return (
            <g key={preset} style={{ cursor: 'pointer' }}
              onClick={(e) => { stop(e); onSelect(preset); }}>
              <circle cx={tx} cy={ty} r={10} fill="transparent" />
              <circle cx={tx} cy={ty} r={isSel ? 5.5 : 3.5}
                fill={isSel ? accent : '#9ca3af'} style={{ transition: 'all 0.2s' }} />
              <text x={lx} y={ly} textAnchor="middle" dominantBaseline="central"
                fontSize="9.5" fontFamily="monospace"
                fontWeight={isSel ? 'bold' : 'normal'}
                fill={isSel ? accent : '#6b7280'}>
                {preset}
              </text>
            </g>
          );
        })}
        {selectedIdx >= 0 && (
          <line x1={CX} y1={CY} x2={handX} y2={handY}
            stroke={accent} strokeWidth="2" strokeLinecap="round"
            style={{ transition: 'all 0.25s ease' }} />
        )}
        <circle cx={CX} cy={CY} r="3.5" fill={accent} style={{ transition: 'fill 0.2s' }} />
      </svg>

      {rightSlot && (
        <div className="relative flex flex-col items-start gap-0.5 flex-1">
          {rightSlot}
        </div>
      )}
    </div>
  );
};
