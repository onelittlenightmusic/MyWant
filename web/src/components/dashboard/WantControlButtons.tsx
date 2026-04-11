import React from 'react';
import { Play, PlayCircle, Pause, Square, Trash2, BookOpen } from 'lucide-react';
import { classNames } from '@/utils/helpers';

export interface WantControlButtonsProps {
  onStart?: () => void;
  onStop?: () => void;
  onSuspend?: () => void;
  onDelete?: () => void;
  onSaveRecipe?: () => void;
  canStart?: boolean;
  canStop?: boolean;
  canSuspend?: boolean;
  canDelete?: boolean;
  canSaveRecipe?: boolean;
  isSuspended?: boolean;
  loading?: boolean;
  className?: string;
  labels?: {
    start?: string;
    stop?: string;
    suspend?: string;
    delete?: string;
    saveRecipe?: string;
  };
  showLabels?: boolean;
}

interface GridButtonProps {
  icon: React.ReactNode;
  label: string;
  onClick?: () => void;
  colorClass: string;
  disabled?: boolean;
}

const GridButton: React.FC<GridButtonProps> = ({ icon, label, onClick, colorClass, disabled = false }) => (
  <button
    onClick={onClick}
    disabled={disabled}
    title={label}
    className={classNames(
      'flex flex-col items-center justify-center gap-0.5 sm:gap-1 w-full h-full transition-all duration-150',
      disabled
        ? 'bg-gray-400/20 dark:bg-gray-700/30 cursor-not-allowed grayscale opacity-40'
        : `${colorClass} hover:brightness-110 active:opacity-80`
    )}
  >
    <div className="w-3.5 h-3.5 sm:w-4 sm:h-4 flex items-center justify-center">
      {icon}
    </div>
    <span className="text-[9px] sm:text-[10px] font-bold leading-none uppercase tracking-tighter text-white">{label}</span>
  </button>
);

export const WantControlButtons: React.FC<WantControlButtonsProps> = ({
  onStart,
  onStop,
  onSuspend,
  onDelete,
  onSaveRecipe,
  canStart = false,
  canStop = false,
  canSuspend = false,
  canDelete = false,
  canSaveRecipe = false,
  isSuspended = false,
  loading = false,
  className = '',
}) => {
  const cols = onSaveRecipe ? 5 : 4;

  return (
    <div
      className={classNames('grid h-full', className)}
      style={{ gridTemplateColumns: `repeat(${cols}, 1fr)` }}
    >
      {/* Start / Resume */}
      <GridButton
        icon={isSuspended
          ? <PlayCircle className="w-4 h-4 text-white" />
          : <Play className="w-4 h-4 text-white" fill="currentColor" />}
        label={isSuspended ? 'Resume' : 'Start'}
        onClick={onStart}
        colorClass="bg-green-600/90"
        disabled={!canStart || loading}
      />

      {/* Suspend */}
      <GridButton
        icon={<Pause className="w-4 h-4 text-white" fill="currentColor" />}
        label="Suspend"
        onClick={onSuspend}
        colorClass="bg-amber-500/90"
        disabled={!canSuspend || loading}
      />

      {/* Stop */}
      <GridButton
        icon={<Square className="w-4 h-4 text-white" fill="currentColor" />}
        label="Stop"
        onClick={onStop}
        colorClass="bg-red-600/90"
        disabled={!canStop || loading}
      />

      {/* Save Recipe */}
      {onSaveRecipe && (
        <GridButton
          icon={<BookOpen className="w-4 h-4 text-white" />}
          label="Recipe"
          onClick={onSaveRecipe}
          colorClass="bg-blue-600/90"
          disabled={!canSaveRecipe || loading}
        />
      )}

      {/* Delete */}
      <GridButton
        icon={<Trash2 className="w-4 h-4 text-white" />}
        label="Delete"
        onClick={onDelete}
        colorClass="bg-rose-700/90"
        disabled={!canDelete || loading}
      />
    </div>
  );
};
