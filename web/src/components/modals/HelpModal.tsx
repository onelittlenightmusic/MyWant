import React from 'react';
import { BaseModal } from './BaseModal';
import { Keyboard, Gamepad, MousePointer2, Move, ArrowUp, ArrowDown, ArrowLeft, ArrowRight, CornerDownLeft, X, Circle, Square, Triangle } from 'lucide-react';
import { useInputActions } from '@/hooks/useInputActions';

interface HelpModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export const HelpModal: React.FC<HelpModalProps> = ({ isOpen, onClose }) => {
  useInputActions({
    enabled: isOpen,
    captureInput: true,
    onCancel: onClose,
    onConfirm: onClose,
  });

  return (
    <BaseModal isOpen={isOpen} onClose={onClose} title="Shortcuts & Controls" size="lg">
      <div className="space-y-10">
        {/* Keyboard Section */}
        <section>
          <div className="flex items-center gap-2 mb-6 text-gray-900 dark:text-white border-b border-gray-100 dark:border-gray-800 pb-2">
            <Keyboard className="w-5 h-5 text-blue-500" />
            <h4 className="text-base font-bold tracking-tight">Keyboard Layout</h4>
          </div>
          
          <div className="grid grid-cols-1 md:grid-cols-2 gap-x-12 gap-y-6">
            <div className="space-y-4">
              <h5 className="text-[10px] font-bold uppercase tracking-wider text-gray-400 dark:text-gray-500">Navigation</h5>
              <div className="space-y-3">
                <ShortcutRow keys={['↑', '↓', '←', '→']} label="Navigate items / move canvas" />
                <ShortcutRow keys={['Enter']} label="Confirm / Enter inner focus" />
                <ShortcutRow keys={['Esc']} label="Cancel / Exit inner focus" />
                <ShortcutRow keys={['Tab']} label="Next element" />
                <ShortcutRow keys={['Shift', 'Tab']} label="Previous element" />
              </div>
            </div>

            <div className="space-y-4">
              <h5 className="text-[10px] font-bold uppercase tracking-wider text-gray-400 dark:text-gray-500">Actions</h5>
              <div className="space-y-3">
                <ShortcutRow keys={['Space']} label="Toggle (Switch / Checkbox)" />
                <ShortcutRow keys={['Shift', '↑↓←→']} label="Reorder / Move focused item" />
                <ShortcutRow keys={['Alt', 'Space']} label="Toggle Main Menu" />
                <ShortcutRow keys={['Shift', 'Space']} label="Quick Actions / Context Menu" />
                <ShortcutRow keys={['/']} label="Focus Search" />
              </div>
            </div>

            <div className="space-y-4">
              <h5 className="text-[10px] font-bold uppercase tracking-wider text-gray-400 dark:text-gray-500">Dashboard Shortcuts</h5>
              <div className="space-y-3">
                <ShortcutRow keys={['a']} label="Create new Want" />
                <ShortcutRow keys={['s']} label="Toggle Global Memo" />
                <ShortcutRow keys={['Shift', 'S']} label="Toggle Selection Mode" />
                <ShortcutRow keys={['q']} label="Focus AI Prompt" />
                <ShortcutRow keys={['y']} label="Focus Header Buttons" />
              </div>
            </div>
          </div>
        </section>

        {/* Gamepad Section */}
        <section>
          <div className="flex items-center gap-2 mb-6 text-gray-900 dark:text-white border-b border-gray-100 dark:border-gray-800 pb-2">
            <Gamepad className="w-5 h-5 text-purple-500" />
            <h4 className="text-base font-bold tracking-tight">Gamepad Layout</h4>
          </div>

          <div className="flex flex-col lg:flex-row gap-8 items-center lg:items-start">
            {/* Gamepad Diagram Simulation */}
            <div className="relative w-64 h-40 bg-gray-100 dark:bg-gray-800 rounded-[40px] border-4 border-gray-200 dark:border-gray-700 flex-shrink-0">
              {/* Left Side: D-Pad */}
              <div className="absolute left-6 top-1/2 -translate-y-1/2 w-12 h-12">
                <div className="absolute inset-0 flex flex-col items-center justify-between py-1">
                   <div className="w-3 h-3 bg-gray-300 dark:bg-gray-600 rounded-sm" />
                   <div className="w-3 h-3 bg-gray-300 dark:bg-gray-600 rounded-sm" />
                </div>
                <div className="absolute inset-0 flex items-center justify-between px-1">
                   <div className="w-3 h-3 bg-gray-300 dark:bg-gray-600 rounded-sm" />
                   <div className="w-3 h-3 bg-gray-300 dark:bg-gray-600 rounded-sm" />
                </div>
                <div className="absolute top-full mt-2 left-1/2 -translate-x-1/2 text-[9px] font-bold text-gray-500">D-PAD: NAV</div>
              </div>

              {/* Right Side: Face Buttons */}
              <div className="absolute right-6 top-1/2 -translate-y-1/2 w-12 h-12">
                <div className="absolute top-0 left-1/2 -translate-x-1/2 w-4 h-4 rounded-full border border-gray-300 dark:border-gray-600 flex items-center justify-center text-[8px] font-bold text-blue-400">Y</div>
                <div className="absolute bottom-0 left-1/2 -translate-x-1/2 w-4 h-4 rounded-full border border-gray-300 dark:border-gray-600 flex items-center justify-center text-[8px] font-bold text-green-400">A</div>
                <div className="absolute left-0 top-1/2 -translate-y-1/2 w-4 h-4 rounded-full border border-gray-300 dark:border-gray-600 flex items-center justify-center text-[8px] font-bold text-pink-400">X</div>
                <div className="absolute right-0 top-1/2 -translate-y-1/2 w-4 h-4 rounded-full border border-gray-300 dark:border-gray-600 flex items-center justify-center text-[8px] font-bold text-red-400">B</div>
              </div>

              {/* Bumpers */}
              <div className="absolute -top-1 left-8 w-10 h-3 bg-gray-300 dark:bg-gray-600 rounded-t-lg flex items-center justify-center text-[7px] font-bold text-white">L1</div>
              <div className="absolute -top-1 right-8 w-10 h-3 bg-gray-300 dark:bg-gray-600 rounded-t-lg flex items-center justify-center text-[7px] font-bold text-white">R1</div>

              {/* Middle Buttons */}
              <div className="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 flex gap-2">
                <div className="w-3 h-1.5 bg-gray-300 dark:bg-gray-600 rounded-full" />
                <div className="w-3 h-1.5 bg-gray-300 dark:bg-gray-600 rounded-full" />
              </div>
            </div>

            {/* Gamepad Labels */}
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-8 gap-y-3 flex-1">
              <GamepadAction btn="A (Cross)" color="text-green-500" action="Confirm / Reorder (Hold)" />
              <GamepadAction btn="B (Circle)" color="text-red-500" action="Cancel / Back" />
              <GamepadAction btn="X (Square)" color="text-pink-500" action="Toggle / Select" />
              <GamepadAction btn="Y (Triangle)" color="text-blue-500" action="Header Focus" />
              <GamepadAction btn="L1 / R1" color="text-gray-500" action="Tab Prev / Next" />
              <GamepadAction btn="D-Pad" color="text-gray-500" action="Navigate / Move" />
              <GamepadAction btn="Select" color="text-gray-500" action="Main Menu" />
              <GamepadAction btn="Start" color="text-gray-500" action="Quick Actions" />
              <GamepadAction btn="L-Stick" color="text-gray-500" action="Canvas Scroll" />
              <GamepadAction btn="R-Stick" color="text-gray-500" action="Canvas Zoom" />
              <GamepadAction btn="A (Long)" color="text-green-600" action="Canvas Drag / Recommend" />
            </div>
          </div>
        </section>
      </div>
      
      <div className="mt-8 pt-6 border-t border-gray-100 dark:border-gray-800 flex justify-center">
        <button
          onClick={onClose}
          className="px-6 py-2 rounded-xl bg-gray-100 hover:bg-gray-200 dark:bg-gray-800 dark:hover:bg-gray-700 text-gray-700 dark:text-gray-300 text-sm font-bold transition-colors"
        >
          Got it
        </button>
      </div>
    </BaseModal>
  );
};

const ShortcutRow: React.FC<{ keys: string[]; label: string }> = ({ keys, label }) => (
  <div className="flex items-center justify-between text-xs group">
    <div className="flex gap-1">
      {keys.map((k) => (
        <kbd 
          key={k} 
          className="min-w-[20px] h-5 px-1.5 flex items-center justify-center rounded bg-gray-100 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-[10px] font-bold text-gray-600 dark:text-gray-400 shadow-[0_1px_0_rgba(0,0,0,0.1)] dark:shadow-none"
        >
          {k}
        </kbd>
      ))}
    </div>
    <span className="text-gray-500 dark:text-gray-400 ml-3 group-hover:text-gray-700 dark:group-hover:text-gray-200 transition-colors">
      {label}
    </span>
  </div>
);

const GamepadAction: React.FC<{ btn: string; action: string; color: string }> = ({ btn, action, color }) => (
  <div className="flex items-center gap-3 text-xs">
    <span className={classNames("font-bold min-w-[60px] text-right", color)}>{btn}</span>
    <span className="text-gray-400 dark:text-gray-600">→</span>
    <span className="text-gray-600 dark:text-gray-400">{action}</span>
  </div>
);
