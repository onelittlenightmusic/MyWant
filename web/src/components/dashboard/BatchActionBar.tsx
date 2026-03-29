import React, { useEffect } from 'react';
import { Play, Square, Trash2, X } from 'lucide-react';
import { classNames } from '@/utils/helpers';

interface ActionCellProps {
  icon: React.ReactNode;
  label: string;
  onClick: () => void;
  colorClass: string;
  disabled?: boolean;
  delay?: number;
}

const ActionCell: React.FC<ActionCellProps> = ({ icon, label, onClick, colorClass, disabled = false, delay = 0 }) => (
  <button
    onClick={(e) => { e.stopPropagation(); if (!disabled) onClick(); }}
    disabled={disabled}
    className={classNames(
      'flex flex-col items-center justify-center gap-1 h-full px-4 sm:px-6 transition-all duration-150',
      disabled
        ? 'bg-gray-400/30 cursor-not-allowed grayscale opacity-50'
        : `hover:brightness-110 active:opacity-80 ${colorClass}`
    )}
    style={{
      animation: 'quickActionBtnIn 150ms ease-out both',
      animationDelay: `${delay}ms`,
    }}
  >
    <div className="w-5 h-5 flex items-center justify-center text-white dark:text-black">{icon}</div>
    <span className="text-white dark:text-black text-[10px] font-bold leading-none uppercase tracking-tighter hidden sm:block">{label}</span>
  </button>
);

interface BatchActionBarProps {
  selectedCount: number;
  onBatchStart: () => void;
  onBatchStop: () => void;
  onBatchDelete: () => void;
  onExit: () => void;
  loading?: boolean;
}

export const BatchActionBar: React.FC<BatchActionBarProps> = ({
  selectedCount,
  onBatchStart,
  onBatchStop,
  onBatchDelete,
  onExit,
  loading = false,
}) => {
  const hasSelection = selectedCount > 0;

  // Keyboard shortcuts: s=start, x=stop, d=delete
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement;
      if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable) return;
      if (!hasSelection || loading) return;
      switch (e.key.toLowerCase()) {
        case 's': e.preventDefault(); e.stopImmediatePropagation(); onBatchStart(); break;
        case 'x': e.preventDefault(); e.stopImmediatePropagation(); onBatchStop(); break;
        case 'd': e.preventDefault(); e.stopImmediatePropagation(); onBatchDelete(); break;
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [hasSelection, loading, onBatchStart, onBatchStop, onBatchDelete]);

  return (
    <div className="h-full flex items-stretch">
      {/* Left: selection count */}
      <div className="flex items-center px-3 sm:px-6 min-w-[72px] sm:min-w-[96px]">
        <span className="text-white dark:text-black text-sm font-semibold tabular-nums">
          {selectedCount}
          <span className="text-white/60 dark:text-black/60 text-xs ml-1 hidden sm:inline">selected</span>
        </span>
      </div>

      {/* Center: action buttons */}
      <div className="flex flex-1 items-stretch justify-center">
        <div className="flex h-full">
          <ActionCell
            icon={<Play className="w-5 h-5" fill="currentColor" />}
            label="Start"
            onClick={onBatchStart}
            colorClass="bg-green-600/90"
            disabled={!hasSelection || loading}
            delay={0}
          />
          <div className="w-px bg-white/15 dark:bg-black/15 self-stretch" />
          <ActionCell
            icon={<Square className="w-5 h-5" fill="currentColor" />}
            label="Stop"
            onClick={onBatchStop}
            colorClass="bg-red-600/90"
            disabled={!hasSelection || loading}
            delay={30}
          />
          <div className="w-px bg-white/15 dark:bg-black/15 self-stretch" />
          <ActionCell
            icon={<Trash2 className="w-5 h-5" />}
            label="Delete"
            onClick={onBatchDelete}
            colorClass="bg-rose-700/90"
            disabled={!hasSelection || loading}
            delay={60}
          />
        </div>
      </div>

      {/* Right: exit button */}
      <div className="flex items-center px-3 sm:px-6 min-w-[72px] sm:min-w-[96px] justify-end">
        <button
          onClick={onExit}
          className="flex flex-col items-center justify-center gap-1 px-3 py-1.5 rounded text-white/70 hover:text-white hover:bg-white/10 dark:text-black/70 dark:hover:text-black dark:hover:bg-black/10 transition-colors"
        >
          <X className="w-5 h-5" />
          <span className="text-[10px] font-bold uppercase tracking-tighter hidden sm:block">Exit</span>
        </button>
      </div>
    </div>
  );
};
