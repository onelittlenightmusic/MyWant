import React, { useState } from 'react';
import { X, ArrowRight, Zap, Check, Plus } from 'lucide-react';
import { ProximityState, FieldMatchRec } from './hooks/useFieldMatchProximity';
import { classNames } from '@/utils/helpers';

interface FieldMatchBubbleProps {
  proximity: ProximityState;
  scale: number;
  offsetX: number;
  offsetY: number;
  scrollLeft: number;
  scrollTop: number;
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

  const sourceName = proximity.recs[0]?.source.want_name ?? '';
  const targetName = proximity.recs[0]?.target.want_name ?? '';

  return (
    <div
      className="fixed z-[200] pointer-events-auto"
      style={{ left: vx, top: vy, transform: 'translate(-50%, -50%)' }}
      onClick={e => e.stopPropagation()}
    >
      {/* Connector dots */}
      <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
        <div className="w-1.5 h-1.5 rounded-full bg-blue-500 animate-ping opacity-40" />
      </div>

      {/* Bubble panel */}
      <div
        className="relative ml-3 mt-3 bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded-lg shadow-xl overflow-hidden flex flex-col"
        style={{ minWidth: 240, maxWidth: 300 }}
      >
        {/* Compact Header */}
        <div className="flex items-stretch justify-between border-b border-gray-100 dark:border-gray-800 h-9">
          <div className="flex items-center gap-2 px-3 flex-1 min-w-0">
            <Zap className="w-3 h-3 text-blue-500 dark:text-blue-400 shrink-0" />
            <div className="flex items-center gap-1.5 min-w-0 text-[10px] font-bold">
              <span className="text-gray-500 dark:text-gray-400 truncate max-w-[80px]">{sourceName}</span>
              <ArrowRight className="w-2.5 h-2.5 text-gray-300 dark:text-gray-600 shrink-0" />
              <span className="text-blue-600 dark:text-blue-400 truncate max-w-[80px]">{targetName}</span>
            </div>
          </div>
          <button
            onClick={onDismiss}
            className="px-2.5 h-full text-gray-400 hover:text-gray-600 dark:text-gray-500 dark:hover:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors border-l border-gray-100 dark:border-gray-800"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>

        {/* List */}
        <div className="p-1 space-y-0.5">
          {proximity.recs.slice(0, 5).map(rec => {
            const key = recKey(rec);
            const isApplied = applied.has(key);
            const isApplying = applying === key;

            return (
              <div
                key={key}
                className={classNames(
                  "flex items-center gap-2 rounded px-2 py-1 transition-colors",
                  isApplied ? 'bg-green-50 dark:bg-green-900/10' : 'hover:bg-gray-50 dark:hover:bg-white/5'
                )}
              >
                <div className="flex-1 min-w-0 flex items-center gap-1.5">
                  <span className="text-[10.5px] text-gray-600 dark:text-gray-400 font-mono truncate">
                    {rec.source.field_name}
                    {rec.source.is_final && <span className="text-amber-500 ml-0.5">★</span>}
                  </span>
                  <span className="text-[9px] text-gray-300 dark:text-gray-600">→</span>
                  <span className="text-[10.5px] text-blue-600 dark:text-blue-400 font-mono truncate">
                    {rec.target.param_name}
                  </span>
                </div>

                <div className="shrink-0">
                  {isApplied ? (
                    <div className="w-7 h-7 flex items-center justify-center text-green-600 dark:text-green-400">
                      <Check className="w-3.5 h-3.5" />
                    </div>
                  ) : (
                    <button
                      onClick={() => handleApply(rec)}
                      disabled={isApplying}
                      className={classNames(
                        "w-7 h-7 flex flex-col items-center justify-center gap-0.5 transition-all duration-150 focus:outline-none",
                        isApplying
                          ? "bg-gray-100 dark:bg-gray-800 text-gray-400 cursor-not-allowed"
                          : "bg-blue-600 text-white hover:brightness-110 active:opacity-80 shadow-sm"
                      )}
                    >
                      {isApplying ? (
                        <div className="w-2.5 h-2.5 border-2 border-gray-300 border-t-blue-400 rounded-full animate-spin" />
                      ) : (
                        <>
                          <Plus className="w-3 h-3" style={{ strokeWidth: 3 }} />
                          <span className="text-[6px] font-black uppercase tracking-tighter">Apply</span>
                        </>
                      )}
                    </button>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
};
