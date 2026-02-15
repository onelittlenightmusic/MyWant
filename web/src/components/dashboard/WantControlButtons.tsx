import React from 'react';
import { Play, Pause, Square, Trash2, BookOpen } from 'lucide-react';
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
  labels = {
    start: 'Start',
    stop: 'Stop',
    suspend: 'Suspend',
    delete: 'Delete',
    saveRecipe: 'Save Recipe'
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
          'p-1.5 sm:p-2 rounded-md transition-colors flex items-center gap-2',
          canStart && !loading
            ? 'bg-green-100 dark:bg-green-900/30 text-green-600 dark:text-green-400 hover:bg-green-200 dark:hover:bg-green-900/40'
            : 'bg-gray-100 dark:bg-gray-800 text-gray-400 dark:text-gray-600 cursor-not-allowed'
        )}
      >
        <Play className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
        {showLabels && <span className="text-xs sm:text-sm font-medium">{labels.start}</span>}
      </button>

      {/* Suspend */}
      {(onSuspend || showLabels) && (
        <button
          onClick={onSuspend}
          disabled={!canSuspend || loading} // Hide if action not provided unless labels shown (layout)
          style={{ display: !onSuspend && !canSuspend ? 'none' : undefined }}
          title="Suspend execution"
          className={classNames(
            'p-1.5 sm:p-2 rounded-md transition-colors flex items-center gap-2',
            canSuspend && !loading
              ? 'bg-orange-100 dark:bg-orange-900/30 text-orange-600 dark:text-orange-400 hover:bg-orange-200 dark:hover:bg-orange-900/40'
              : 'bg-gray-100 dark:bg-gray-800 text-gray-400 dark:text-gray-600 cursor-not-allowed'
          )}
        >
          <Pause className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
          {showLabels && <span className="text-xs sm:text-sm font-medium">{labels.suspend}</span>}
        </button>
      )}

      {/* Stop */}
      <button
        onClick={onStop}
        disabled={!canStop || loading}
        title={canStop ? 'Stop execution' : 'Cannot stop in current state'}
        className={classNames(
          'p-1.5 sm:p-2 rounded-md transition-colors flex items-center gap-2',
          canStop && !loading
            ? 'bg-red-100 dark:bg-red-900/30 text-red-600 dark:text-red-400 hover:bg-red-200 dark:hover:bg-red-900/40'
            : 'bg-gray-100 dark:bg-gray-800 text-gray-400 dark:text-gray-600 cursor-not-allowed'
        )}
      >
        <Square className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
        {showLabels && <span className="text-xs sm:text-sm font-medium">{labels.stop}</span>}
      </button>

      {/* Save Recipe */}
      {onSaveRecipe && (
        <button
          onClick={onSaveRecipe}
          disabled={!canSaveRecipe || loading}
          title={canSaveRecipe ? 'Save as recipe' : 'Only target wants can be saved as recipes'}
          className={classNames(
            'p-1.5 sm:p-2 rounded-md transition-colors flex items-center gap-2',
            canSaveRecipe && !loading
              ? 'bg-blue-100 dark:bg-blue-900/30 text-blue-600 dark:text-blue-400 hover:bg-blue-200 dark:hover:bg-blue-900/40'
              : 'bg-gray-100 dark:bg-gray-800 text-gray-400 dark:text-gray-600 cursor-not-allowed'
          )}
        >
          <BookOpen className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
          {showLabels && <span className="text-xs sm:text-sm font-medium">{labels.saveRecipe}</span>}
        </button>
      )}

      {/* Delete */}
      <button
        onClick={onDelete}
        disabled={!canDelete || loading}
        title={canDelete ? 'Delete want' : 'No want selected'}
        className={classNames(
          'p-1.5 sm:p-2 rounded-md transition-colors flex items-center gap-2',
          canDelete && !loading
            ? 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-600'
            : 'bg-gray-100 dark:bg-gray-800 text-gray-400 dark:text-gray-600 cursor-not-allowed'
        )}
      >
        <Trash2 className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
        {showLabels && <span className="text-xs sm:text-sm font-medium">{labels.delete}</span>}
      </button>
    </div>
  );
};
