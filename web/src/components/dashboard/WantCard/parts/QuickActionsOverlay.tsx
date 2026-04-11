import React, { useEffect } from 'react';
import { Play, PlayCircle, Square, Trash2, Pause, RotateCcw, Settings, X } from 'lucide-react';
import { Want } from '@/types/want';
import { classNames } from '@/utils/helpers';

interface ActionButtonProps {
  icon: React.ReactNode;
  label: string;
  onClick: () => void;
  colorClass: string;
  delay?: number;
  disabled?: boolean;
}

const ActionButton: React.FC<ActionButtonProps> = ({ icon, label, onClick, colorClass, delay = 0, disabled = false }) => (
  <button
    onClick={(e) => { e.stopPropagation(); if (!disabled) onClick(); }}
    disabled={disabled}
    className={classNames(
      "flex flex-col items-center justify-center gap-0.5 w-full h-full transition-all duration-150",
      disabled ? "bg-gray-400/30 cursor-not-allowed grayscale opacity-50" : `hover:brightness-110 active:opacity-80 ${colorClass}`
    )}
    style={{ 
      animation: 'quickActionBtnIn 150ms ease-out both',
      animationDelay: `${delay}ms` 
    }}
  >
    <div className="w-4 h-4 sm:w-5 sm:h-5 flex items-center justify-center">
      {icon}
    </div>
    <span className="text-white text-[9px] font-bold leading-none uppercase tracking-tighter">{label}</span>
  </button>
);

interface QuickActionsOverlayProps {
  want: Want;
  onClose: () => void;
  onView: () => void;
  onStart: () => void;
  onStop: () => void;
  onSuspend: () => void;
  onResume: () => void;
  onRestart: () => void;
  onEdit: () => void;
  onDelete: () => void;
}

export const QuickActionsOverlay: React.FC<QuickActionsOverlayProps> = ({
  want,
  onClose,
  onView,
  onStart,
  onStop,
  onSuspend,
  onResume,
  onRestart,
  onEdit,
  onDelete,
}) => {
  const status = want.status;

  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', handleKey);
    return () => document.removeEventListener('keydown', handleKey);
  }, [onClose]);

  // Status checks
  const isRunning = status === 'reaching' || status === 'waiting_user_action';
  const isStopped = status === 'stopped' || status === 'created' || status === 'failed' || status === 'achieved' || status === 'terminated';
  const isSuspended = status === 'suspended';

  // Toggle buttons
  const tlButton = isStopped ? (
    <ActionButton
      icon={<Play className="w-5 h-5 text-white" fill="currentColor" />}
      label="Start"
      onClick={() => { onStart(); onClose(); }}
      colorClass="bg-green-600/90"
      delay={0}
    />
  ) : (
    <ActionButton
      icon={<Square className="w-5 h-5 text-white" fill="currentColor" />}
      label="Stop"
      onClick={() => { onStop(); onClose(); }}
      colorClass="bg-red-600/90"
      delay={0}
    />
  );

  const blButton = (isRunning) ? (
    <ActionButton
      icon={<Pause className="w-5 h-5 text-white" fill="currentColor" />}
      label="Suspend"
      onClick={() => { onSuspend(); onClose(); }}
      colorClass="bg-amber-500/90"
      delay={60}
    />
  ) : isSuspended ? (
    <ActionButton
      icon={<PlayCircle className="w-5 h-5 text-white" />}
      label="Resume"
      onClick={() => { onResume(); onClose(); }}
      colorClass="bg-green-600/90"
      delay={60}
    />
  ) : (
    <ActionButton
      icon={<Pause className="w-5 h-5 text-white" />}
      label="Suspend"
      onClick={() => {}}
      colorClass="bg-gray-400/30"
      delay={60}
      disabled={true}
    />
  );

  return (
    <div
      className="absolute inset-0 z-40 rounded-[inherit] overflow-hidden"
      style={{
        animation: 'quickActionsIn 150ms ease-out forwards',
      }}
    >
      {/* Backdrop - click to dismiss */}
      <div
        className="absolute inset-0 bg-black/60 rounded-[inherit]"
        onClick={(e) => { e.stopPropagation(); onClose(); }}
      />

      {/* 3x2 grid - NO PADDING, NO GAP */}
      <div
        className="absolute inset-0 grid pointer-events-none"
        style={{ 
          gridTemplateColumns: 'repeat(3, 1fr)', 
          gridTemplateRows: 'repeat(2, 1fr)',
        }}
      >
        {/* Row 1 */}
        <div className="pointer-events-auto h-full w-full">
          {tlButton}
        </div>
        <div className="pointer-events-auto h-full w-full">
          <ActionButton
            icon={<RotateCcw className="w-5 h-5 text-white" />}
            label="Restart"
            onClick={() => { onRestart(); onClose(); }}
            colorClass="bg-blue-500/90"
            delay={30}
          />
        </div>
        <div className="pointer-events-auto h-full w-full">
          <ActionButton
            icon={<Settings className="w-5 h-5 text-white" />}
            label="Edit"
            onClick={() => { onEdit(); onClose(); }}
            colorClass="bg-indigo-600/90"
            delay={60}
          />
        </div>

        {/* Row 2 */}
        <div className="pointer-events-auto h-full w-full">
          {blButton}
        </div>
        <div className="pointer-events-auto h-full w-full">
          <ActionButton
            icon={<X className="w-4 h-4 sm:w-5 sm:h-5 text-white" />}
            label="Close"
            onClick={() => onClose()}
            colorClass="bg-gray-600/90"
            delay={90}
          />
        </div>
        <div className="pointer-events-auto h-full w-full">
          <ActionButton
            icon={<Trash2 className="w-5 h-5 text-white" />}
            label="Delete"
            onClick={() => { onDelete(); onClose(); }}
            colorClass="bg-rose-700/90"
            delay={120}
          />
        </div>
      </div>
    </div>
  );
};
