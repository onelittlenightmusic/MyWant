import React, { useEffect, useState } from 'react';
import { Check, X } from 'lucide-react';
import { classNames } from '@/utils/helpers';
import { useInputActions } from '@/hooks/useInputActions';

interface DeleteConfirmOverlayProps {
  onConfirm: () => void;
  onCancel: () => void;
}

export const DeleteConfirmOverlay: React.FC<DeleteConfirmOverlayProps> = ({ onConfirm, onCancel }) => {
  // 'no' is the safe default focus so accidental Enter doesn't delete
  const [focused, setFocused] = useState<'no' | 'yes'>('no');

  useInputActions({
    captureInput: true,
    ignoreWhenInputFocused: false,
    onNavigate: (dir) => {
      if (dir === 'left')  setFocused('no');
      if (dir === 'right') setFocused('yes');
    },
    onConfirm: () => { focused === 'yes' ? onConfirm() : onCancel(); },
    onCancel,
  });

  // y/n shortcuts
  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (e.key.toLowerCase() === 'n') { e.preventDefault(); onCancel(); }
      else if (e.key.toLowerCase() === 'y') { e.preventDefault(); onConfirm(); }
    };
    document.addEventListener('keydown', handleKey);
    return () => document.removeEventListener('keydown', handleKey);
  }, [onConfirm, onCancel]);

  return (
    <div
      className="absolute inset-0 z-40 rounded-[inherit] overflow-hidden"
      style={{ animation: 'quickActionsIn 150ms ease-out forwards' }}
    >
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black/70 rounded-[inherit]"
        onClick={(e) => { e.stopPropagation(); onCancel(); }}
      />

      {/* Label */}
      <div className="absolute inset-x-0 top-0 flex items-center justify-center pt-3 pointer-events-none z-10">
        <span className="text-white text-xs font-bold uppercase tracking-widest opacity-80">Delete?</span>
      </div>

      {/* 2x1 grid */}
      <div
        className="absolute inset-0 grid pointer-events-none"
        style={{ gridTemplateColumns: 'repeat(2, 1fr)', gridTemplateRows: '1fr' }}
      >
        {/* No */}
        <div className="pointer-events-auto h-full w-full">
          <button
            onClick={(e) => { e.stopPropagation(); onCancel(); }}
            onMouseEnter={() => setFocused('no')}
            className={classNames(
              'flex flex-col items-center justify-center gap-1 w-full h-full',
              'bg-gray-600/90 hover:brightness-110 active:opacity-80 transition-all duration-150',
              focused === 'no' && 'ring-2 ring-inset ring-white/80'
            )}
            style={{ animation: 'quickActionBtnIn 150ms ease-out both', animationDelay: '0ms' }}
          >
            <X className="w-6 h-6 text-white" />
            <span className="text-white text-[10px] font-bold leading-none uppercase tracking-tighter">No</span>
          </button>
        </div>

        {/* Yes */}
        <div className="pointer-events-auto h-full w-full">
          <button
            onClick={(e) => { e.stopPropagation(); onConfirm(); }}
            onMouseEnter={() => setFocused('yes')}
            className={classNames(
              'flex flex-col items-center justify-center gap-1 w-full h-full',
              'bg-rose-700/90 hover:brightness-110 active:opacity-80 transition-all duration-150',
              focused === 'yes' && 'ring-2 ring-inset ring-white/80'
            )}
            style={{ animation: 'quickActionBtnIn 150ms ease-out both', animationDelay: '60ms' }}
          >
            <Check className="w-6 h-6 text-white" />
            <span className="text-white text-[10px] font-bold leading-none uppercase tracking-tighter">Yes</span>
          </button>
        </div>
      </div>
    </div>
  );
};
