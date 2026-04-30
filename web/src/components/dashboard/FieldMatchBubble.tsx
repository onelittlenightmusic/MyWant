import React, { useState } from 'react';
import { ProximityState, FieldMatchRec, ProximityDirection } from './hooks/useFieldMatchProximity';

interface FieldMatchBubbleProps {
  proximity: ProximityState;
  /** Canvas scale — used to position the bubble in viewport space */
  scale: number;
  offsetX: number;
  offsetY: number;
  scrollLeft: number;
  scrollTop: number;
  /** Called once per render to get the scroll container's bounding rect (avoids storing DOMRect in state) */
  getContainerRect: () => DOMRect | null;
  onApply: (rec: FieldMatchRec) => Promise<void>;
  onDismiss: () => void;
}

const DIRECTION_ARROW: Record<ProximityDirection, string> = {
  left:  '◀',
  right: '▶',
  above: '▲',
  below: '▼',
};

const DIRECTION_LABEL: Record<ProximityDirection, string> = {
  left:  '横方向 · current',
  right: '横方向 · current',
  above: '縦方向 · plan / goal',
  below: '縦方向 · plan / goal',
};

/** Returns one-letter badge + colour classes for a state label. */
function labelBadge(label: string): { letter: string; classes: string } | null {
  switch (label) {
    case 'goal':    return { letter: 'G', classes: 'bg-amber-500/20 text-amber-300 border-amber-500/40' };
    case 'plan':    return { letter: 'P', classes: 'bg-purple-500/20 text-purple-300 border-purple-500/40' };
    case 'current': return { letter: 'C', classes: 'bg-blue-500/20 text-blue-300 border-blue-500/40' };
    default: return null;
  }
}

export const FieldMatchBubble: React.FC<FieldMatchBubbleProps> = ({
  proximity,
  scale,
  offsetX,
  offsetY,
  scrollLeft,
  scrollTop,
  getContainerRect,
  onApply,
  onDismiss,
}) => {
  const [applying, setApplying] = useState<string | null>(null);
  const [applied, setApplied] = useState<Set<string>>(new Set());

  const containerRect = getContainerRect();
  if (!containerRect) return null;

  // Convert canvas-space midpoint → viewport pixel
  const vx = proximity.midX * scale + offsetX - scrollLeft + containerRect.left;
  const vy = proximity.midY * scale + offsetY - scrollTop + containerRect.top;

  const recKey = (rec: FieldMatchRec) => `${rec.source.field_name}→${rec.target.param_name}`;

  const handleApply = async (rec: FieldMatchRec) => {
    const key = recKey(rec);
    setApplying(key);
    try {
      await onApply(rec);
      setApplied(prev => new Set(prev).add(key));
    } finally {
      setApplying(null);
    }
  };

  const arrow = DIRECTION_ARROW[proximity.direction];
  const axisLabel = DIRECTION_LABEL[proximity.direction];
  const sourceName = proximity.recs[0]?.source.want_name ?? '';
  const targetName = proximity.recs[0]?.target.want_name ?? '';

  return (
    <div
      className="fixed z-[200] pointer-events-auto"
      style={{ left: vx, top: vy, transform: 'translate(-50%, -50%)' }}
      onClick={e => e.stopPropagation()}
    >
      {/* Connector line dots */}
      <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
        <div className="w-2 h-2 rounded-full bg-blue-400 animate-ping opacity-60" />
      </div>

      {/* Bubble panel */}
      <div
        className="relative ml-4 mt-4 bg-gray-900/95 border border-blue-500/40 rounded-xl shadow-2xl backdrop-blur-sm"
        style={{ minWidth: 240, maxWidth: 320 }}
      >
        {/* Header — direction arrow + axis label */}
        <div className="flex items-center justify-between px-3 pt-2 pb-1 border-b border-white/10">
          <div className="flex items-center gap-1.5">
            <span className="text-blue-400 text-xs font-mono">{arrow}</span>
            <span className="text-white text-xs font-semibold">接続候補</span>
            <span className="text-white/40 text-[10px]">{axisLabel}</span>
          </div>
          <button
            onClick={onDismiss}
            className="text-white/40 hover:text-white/80 text-xs transition-colors leading-none"
          >✕</button>
        </div>

        {/* Source → Target with provider/consumer hint */}
        <div className="px-3 pt-1.5 pb-1 flex items-center gap-1.5 text-[10px]">
          <span className="text-white/70 font-medium">{sourceName}</span>
          <span className="text-white/30">expose</span>
          <span className="text-blue-400">→</span>
          <span className="text-white/70 font-medium">{targetName}</span>
          <span className="text-white/30">receive</span>
        </div>

        {/* Recommendation list — max 5, one line per item */}
        <div className="px-2 pb-2 space-y-0.5">
          {proximity.recs.slice(0, 5).map(rec => {
            const key = recKey(rec);
            const isApplied = applied.has(key);
            const isApplying = applying === key;
            const score = Math.round(rec.score * 100);
            const badge = labelBadge(rec.source.label);

            return (
              <div
                key={key}
                className={`flex items-center gap-1.5 rounded px-1.5 py-1 transition-colors ${
                  isApplied
                    ? 'bg-green-900/40'
                    : 'hover:bg-white/5'
                }`}
              >
                {/* Label badge */}
                {badge && (
                  <span className={`shrink-0 text-[9px] px-1 py-0.5 rounded border font-bold leading-none ${badge.classes}`}>
                    {badge.letter}
                  </span>
                )}

                {/* Field name */}
                <span className="text-[11px] text-blue-300 font-mono truncate flex-1 min-w-0">
                  {rec.source.field_name}
                  {rec.source.is_final && <span className="text-yellow-400/70 ml-0.5">★</span>}
                </span>

                {/* Score */}
                <span className="shrink-0 text-[10px] text-white/25">{score}%</span>

                {/* Apply / Applied */}
                {isApplied ? (
                  <span className="shrink-0 text-[10px] text-green-400">✓</span>
                ) : (
                  <button
                    onClick={() => handleApply(rec)}
                    disabled={isApplying}
                    className="shrink-0 text-[10px] font-medium px-1.5 py-0.5 rounded bg-blue-600 hover:bg-blue-500 text-white transition-colors disabled:opacity-50"
                  >
                    {isApplying ? '…' : '適用'}
                  </button>
                )}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
};
