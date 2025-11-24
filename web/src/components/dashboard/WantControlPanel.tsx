import React from 'react';
import { Play, Pause, Square, Trash2 } from 'lucide-react';
import { Want } from '@/types/want';
import { classNames } from '@/utils/helpers';

interface WantControlPanelProps {
  selectedWant: Want | null;
  onStart: (want: Want) => void;
  onStop: (want: Want) => void;
  onSuspend: (want: Want) => void;
  onResume: (want: Want) => void;
  onDelete: (want: Want) => void;
  loading?: boolean;
  sidebarMinimized?: boolean;
}

export const WantControlPanel: React.FC<WantControlPanelProps> = ({
  selectedWant,
  onStart,
  onStop,
  onSuspend,
  onResume,
  onDelete,
  loading = false,
  sidebarMinimized = false
}) => {
  const isRunning = selectedWant?.status === 'reaching';
  const isSuspended = selectedWant?.status === 'suspended';
  const isCompleted = selectedWant?.status === 'achieved';
  const isStopped = selectedWant?.status === 'stopped' || selectedWant?.status === 'created';
  const isFailed = selectedWant?.status === 'failed';

  // Button enable/disable logic
  const canStart = selectedWant && (isStopped || isCompleted || isFailed || isSuspended);
  const canStop = selectedWant && isRunning && !isSuspended;
  const canSuspend = selectedWant && isRunning && !isSuspended;
  const canDelete = selectedWant !== null;

  const handleStart = () => {
    if (selectedWant) {
      // If suspended, resume; otherwise start
      if (isSuspended) {
        onResume(selectedWant);
      } else {
        onStart(selectedWant);
      }
    }
  };

  const handleStop = () => {
    if (selectedWant && canStop) onStop(selectedWant);
  };

  const handleSuspend = () => {
    if (selectedWant && canSuspend) onSuspend(selectedWant);
  };

  const handleDelete = () => {
    if (selectedWant && canDelete) onDelete(selectedWant);
  };

  return (
    <div className={classNames(
      "fixed bottom-0 right-0 bg-blue-50 border-t border-blue-200 shadow-lg z-30 transition-all duration-300 ease-in-out",
      sidebarMinimized ? "lg:left-20" : "lg:left-64",
      "left-0"
    )}>
      <div className="px-6 py-3">
        <div className="flex items-center">
          {/* Control Buttons */}
          <div className="flex items-center space-x-2 flex-wrap gap-y-2">
            {/* Start / Resume */}
            <button
              onClick={handleStart}
              disabled={!canStart || loading}
              className={classNames(
                'flex items-center space-x-2 px-4 py-2 rounded-md text-sm font-medium transition-colors',
                canStart && !loading
                  ? 'bg-green-600 text-white hover:bg-green-700'
                  : 'bg-gray-100 text-gray-400 cursor-not-allowed'
              )}
              title={canStart ? (isSuspended ? 'Resume execution' : 'Start execution') : 'Cannot start in current state'}
            >
              <Play className="h-4 w-4" />
              <span>{isSuspended ? 'Resume' : 'Start'}</span>
            </button>

            {/* Suspend */}
            <button
              onClick={handleSuspend}
              disabled={!canSuspend || loading}
              className={classNames(
                'flex items-center space-x-2 px-4 py-2 rounded-md text-sm font-medium transition-colors',
                canSuspend && !loading
                  ? 'bg-orange-600 text-white hover:bg-orange-700'
                  : 'bg-gray-100 text-gray-400 cursor-not-allowed'
              )}
              title={canSuspend ? 'Suspend execution' : 'Cannot suspend in current state'}
            >
              <Pause className="h-4 w-4" />
              <span>Suspend</span>
            </button>

            {/* Stop */}
            <button
              onClick={handleStop}
              disabled={!canStop || loading}
              className={classNames(
                'flex items-center space-x-2 px-4 py-2 rounded-md text-sm font-medium transition-colors',
                canStop && !loading
                  ? 'bg-red-600 text-white hover:bg-red-700'
                  : 'bg-gray-100 text-gray-400 cursor-not-allowed'
              )}
              title={canStop ? 'Stop execution' : 'Cannot stop in current state'}
            >
              <Square className="h-4 w-4" />
              <span>Stop</span>
            </button>

            {/* Delete */}
            <button
              onClick={handleDelete}
              disabled={!canDelete || loading}
              className={classNames(
                'flex items-center space-x-2 px-4 py-2 rounded-md text-sm font-medium transition-colors border',
                canDelete && !loading
                  ? 'border-gray-300 text-gray-700 hover:bg-gray-50'
                  : 'border-gray-200 text-gray-400 cursor-not-allowed'
              )}
              title={canDelete ? 'Delete want' : 'No want selected'}
            >
              <Trash2 className="h-4 w-4" />
              <span>Delete</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};
