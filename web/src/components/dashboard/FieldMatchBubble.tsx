import React, { useState } from 'react';
import { ProximityState, FieldMatchRec } from './hooks/useFieldMatchProximity';

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
        style={{ minWidth: 220, maxWidth: 300 }}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-3 pt-2 pb-1 border-b border-white/10">
          <div className="flex items-center gap-1.5">
            <span className="text-blue-400 text-xs">⟷</span>
            <span className="text-white text-xs font-semibold">接続候補</span>
          </div>
          <button
            onClick={onDismiss}
            className="text-white/40 hover:text-white/80 text-xs transition-colors leading-none"
          >✕</button>
        </div>

        {/* Recommendation list */}
        <div className="p-2 space-y-1.5">
          {proximity.recs.map(rec => {
            const key = recKey(rec);
            const isApplied = applied.has(key);
            const isApplying = applying === key;
            const score = Math.round(rec.score * 100);

            return (
              <div
                key={key}
                className={`rounded-lg px-2.5 py-2 border transition-colors ${
                  isApplied
                    ? 'bg-green-900/40 border-green-500/40'
                    : 'bg-white/5 border-white/10 hover:bg-white/10'
                }`}
              >
                {/* Field path */}
                <p className="text-[11px] text-white/80 font-mono leading-tight mb-1.5">
                  <span className="text-blue-300">{rec.source.field_name}</span>
                  <span className="text-white/40 mx-1">→</span>
                  <span className="text-purple-300">{rec.target.param_name}</span>
                </p>

                {/* Score + type + button */}
                <div className="flex items-center justify-between gap-2">
                  <div className="flex items-center gap-1.5">
                    <span className={`text-[10px] px-1.5 py-0.5 rounded font-mono ${
                      rec.source.field_type === 'array'
                        ? 'bg-blue-500/20 text-blue-300'
                        : 'bg-gray-500/20 text-gray-300'
                    }`}>
                      {rec.source.field_type}
                    </span>
                    <span className="text-[10px] text-white/30">{score}%</span>
                    {rec.source.is_final && (
                      <span className="text-[10px] text-yellow-400/70">★</span>
                    )}
                  </div>
                  {isApplied ? (
                    <span className="text-[10px] text-green-400">✓ 適用済み</span>
                  ) : (
                    <button
                      onClick={() => handleApply(rec)}
                      disabled={isApplying}
                      className="text-[11px] font-medium px-2 py-0.5 rounded bg-blue-600 hover:bg-blue-500 text-white transition-colors disabled:opacity-50"
                    >
                      {isApplying ? '...' : '適用'}
                    </button>
                  )}
                </div>
              </div>
            );
          })}
        </div>

        {/* Source → Target names */}
        <div className="px-3 pb-2 flex items-center gap-1 text-[10px] text-white/30">
          <span>{proximity.recs[0]?.source.want_name}</span>
          <span>→</span>
          <span>{proximity.recs[0]?.target.want_name}</span>
        </div>
      </div>
    </div>
  );
};
