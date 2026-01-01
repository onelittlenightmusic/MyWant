import React from 'react';
import { Play, Pause, Square, Trash2 } from 'lucide-react';
import { classNames } from '@/utils/helpers';

export interface WantControlButtonsProps {
  onStart?: () => void;
  onStop?: () => void;
  onSuspend?: () => void;
  onDelete?: () => void;
  canStart?: boolean;
  canStop?: boolean;
  canSuspend?: boolean;
  canDelete?: boolean;
  isSuspended?: boolean;
  loading?: boolean;
  className?: string;
  labels?: {
    start?: string;
    stop?: string;
    suspend?: string;
    delete?: string;
  };
  showLabels?: boolean;
}

export const WantControlButtons: React.FC<WantControlButtonsProps> = ({
  onStart,
  onStop,
  onSuspend,
  onDelete,
  canStart = false,
  canStop = false,
  canSuspend = false,
  canDelete = false,
  isSuspended = false,
  loading = false,
  className = '',
  labels = {
    start: 'Start',
    stop: 'Stop',
    suspend: 'Suspend',
    delete: 'Delete'
  },
  showLabels = false
}) => {
  return (
    <div className={classNames("flex gap-1 justify-center", className)}>
      {/* Start / Resume */}
      <button
        onClick={onStart}
        disabled={!canStart || loading}
        title={canStart ? (isSuspended ? 'Resume execution' : 'Start execution') : 'Cannot start in current state'}
        className={classNames(
          'p-2 rounded-md transition-colors flex items-center gap-2',
          canStart && !loading
            ? 'bg-green-100 text-green-600 hover:bg-green-200'
            : 'bg-gray-100 text-gray-400 cursor-not-allowed'
        )}
      >
        <Play className="h-4 w-4" />
        {showLabels && <span className="text-sm font-medium">{labels.start}</span>}
      </button>

      {/* Suspend */}
      {(onSuspend || showLabels) && (
        <button
          onClick={onSuspend}
          disabled={!canSuspend || loading} // Hide if action not provided unless labels shown (layout)
          style={{ display: !onSuspend && !canSuspend ? 'none' : undefined }}
          title="Suspend execution"
          className={classNames(
            'p-2 rounded-md transition-colors flex items-center gap-2',
            canSuspend && !loading
              ? 'bg-orange-100 text-orange-600 hover:bg-orange-200'
              : 'bg-gray-100 text-gray-400 cursor-not-allowed'
          )}
        >
          <Pause className="h-4 w-4" />
          {showLabels && <span className="text-sm font-medium">{labels.suspend}</span>}
        </button>
      )}

      {/* Stop */}
      <button
        onClick={onStop}
        disabled={!canStop || loading}
        title={canStop ? 'Stop execution' : 'Cannot stop in current state'}
        className={classNames(
          'p-2 rounded-md transition-colors flex items-center gap-2',
          canStop && !loading
            ? 'bg-red-100 text-red-600 hover:bg-red-200'
            : 'bg-gray-100 text-gray-400 cursor-not-allowed'
        )}
      >
        <Square className="h-4 w-4" />
        {showLabels && <span className="text-sm font-medium">{labels.stop}</span>}
      </button>

      {/* Delete */}
      <button
        onClick={onDelete}
        disabled={!canDelete || loading}
        title={canDelete ? 'Delete want' : 'No want selected'}
        className={classNames(
          'p-2 rounded-md transition-colors flex items-center gap-2',
          canDelete && !loading
            ? 'bg-gray-100 text-gray-600 hover:bg-gray-200'
            : 'bg-gray-100 text-gray-400 cursor-not-allowed'
        )}
      >
        <Trash2 className="h-4 w-4" />
        {showLabels && <span className="text-sm font-medium">{labels.delete}</span>}
      </button>
    </div>
  );
};
