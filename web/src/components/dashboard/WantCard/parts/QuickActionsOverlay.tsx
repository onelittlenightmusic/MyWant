import React, { useState } from 'react';
import { Play, PlayCircle, Square, Trash2, Pause, RotateCcw, Settings, X } from 'lucide-react';
import { Want } from '@/types/want';
import { classNames } from '@/utils/helpers';
import { useInputActions } from '@/hooks/useInputActions';

const COLS = 3;
const ROWS = 2;

interface ActionDef {
  icon: React.ReactNode;
  label: string;
  onClick: () => void;
  colorClass: string;
  delay: number;
  disabled?: boolean;
}

interface ActionButtonProps extends ActionDef {
  focused: boolean;
}

const ActionButton: React.FC<ActionButtonProps> = ({ icon, label, onClick, colorClass, delay, disabled, focused }) => (
  <button
    onClick={(e) => { e.stopPropagation(); if (!disabled) onClick(); }}
    disabled={disabled}
    className={classNames(
      "flex flex-col items-center justify-center gap-0.5 w-full h-full transition-all duration-150",
      disabled
        ? "bg-gray-400/30 cursor-not-allowed grayscale opacity-50"
        : `hover:brightness-110 active:opacity-80 ${colorClass}`,
      focused && !disabled && 'ring-2 ring-inset ring-sky-400',
    )}
    style={{
      animation: 'quickActionBtnIn 150ms ease-out both',
      animationDelay: `${delay}ms`,
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
  onStart,
  onStop,
  onSuspend,
  onResume,
  onRestart,
  onEdit,
  onDelete,
}) => {
  const [focusedIndex, setFocusedIndex] = useState(0);
  const status = want.status;

  const isRunning  = status === 'reaching' || status === 'reaching_with_warning' || status === 'waiting_user_action';
  const isStopped  = status === 'stopped' || status === 'created' || status === 'failed' || status === 'achieved' || status === 'achieved_with_warning' || status === 'terminated';
  const isSuspended = status === 'suspended';

  // Row-major layout: [Start/Stop, Restart, Edit, Suspend/Resume, Close, Delete]
  const actions: ActionDef[] = [
    // Row 0
    isStopped
      ? { icon: <Play  className="w-5 h-5 text-white" fill="currentColor" />, label: 'Start',   onClick: () => { onStart();   onClose(); }, colorClass: 'bg-green-600/90', delay: 0 }
      : { icon: <Square className="w-5 h-5 text-white" fill="currentColor" />, label: 'Stop',   onClick: () => { onStop();    onClose(); }, colorClass: 'bg-red-600/90',   delay: 0 },
    {   icon: <RotateCcw className="w-5 h-5 text-white" />,                    label: 'Restart', onClick: () => { onRestart(); onClose(); }, colorClass: 'bg-blue-500/90',  delay: 30 },
    {   icon: <Settings  className="w-5 h-5 text-white" />,                    label: 'Edit',    onClick: () => { onEdit();    onClose(); }, colorClass: 'bg-indigo-600/90', delay: 60 },
    // Row 1
    isRunning
      ? { icon: <Pause     className="w-5 h-5 text-white" fill="currentColor" />, label: 'Suspend', onClick: () => { onSuspend(); onClose(); }, colorClass: 'bg-amber-500/90',  delay: 60 }
      : isSuspended
      ? { icon: <PlayCircle className="w-5 h-5 text-white" />,                    label: 'Resume',  onClick: () => { onResume();  onClose(); }, colorClass: 'bg-green-600/90',  delay: 60 }
      : { icon: <Pause     className="w-5 h-5 text-white" />,                    label: 'Suspend', onClick: () => {},                         colorClass: 'bg-gray-400/30',   delay: 60, disabled: true },
    {   icon: <X    className="w-4 h-4 sm:w-5 sm:h-5 text-white" />,            label: 'Close',   onClick: () => onClose(),                  colorClass: 'bg-gray-600/90',   delay: 90 },
    {   icon: <Trash2 className="w-5 h-5 text-white" />,                         label: 'Delete',  onClick: () => { onDelete();  onClose(); }, colorClass: 'bg-rose-700/90',   delay: 120 },
  ];

  useInputActions({
    captureInput: true,
    ignoreWhenInputFocused: false,
    onNavigate: (dir) => {
      setFocusedIndex(prev => {
        const row = Math.floor(prev / COLS);
        const col = prev % COLS;
        if (dir === 'right') return row * COLS + Math.min(COLS - 1, col + 1);
        if (dir === 'left')  return row * COLS + Math.max(0, col - 1);
        if (dir === 'down')  return Math.min(COLS * ROWS - 1, (row + 1) * COLS + col);
        if (dir === 'up')    return Math.max(0, (row - 1) * COLS + col);
        return prev;
      });
    },
    onConfirm: () => {
      const action = actions[focusedIndex];
      if (action && !action.disabled) action.onClick();
    },
    onCancel: onClose,
  });

  return (
    <div
      className="absolute inset-0 z-40 rounded-[inherit] overflow-hidden"
      style={{ animation: 'quickActionsIn 150ms ease-out forwards' }}
    >
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black/60 rounded-[inherit]"
        onClick={(e) => { e.stopPropagation(); onClose(); }}
      />

      {/* 3×2 grid */}
      <div
        className="absolute inset-0 grid pointer-events-none"
        style={{
          gridTemplateColumns: 'repeat(3, 1fr)',
          gridTemplateRows: 'repeat(2, 1fr)',
        }}
      >
        {actions.map((action, idx) => (
          <div key={idx} className="pointer-events-auto h-full w-full">
            <ActionButton {...action} focused={idx === focusedIndex} />
          </div>
        ))}
      </div>
    </div>
  );
};
