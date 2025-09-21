import React from 'react';
import { Eye, Edit, Trash2, Play, Square, Pause, MoreHorizontal } from 'lucide-react';
import { Want } from '@/types/want';
import { StatusBadge } from '@/components/common/StatusBadge';
import { formatDate, formatDuration, truncateText, classNames } from '@/utils/helpers';

interface WantCardProps {
  want: Want;
  onView: (want: Want) => void;
  onEdit: (want: Want) => void;
  onDelete: (want: Want) => void;
  onSuspend?: (want: Want) => void;
  onResume?: (want: Want) => void;
  className?: string;
}

export const WantCard: React.FC<WantCardProps> = ({
  want,
  onView,
  onEdit,
  onDelete,
  onSuspend,
  onResume,
  className
}) => {
  const wantName = want.metadata?.name || want.metadata?.id || 'Unnamed Want';
  const wantType = want.metadata?.type || 'unknown';
  const labels = want.metadata?.labels || {};
  const createdAt = want.stats?.created_at;
  const startedAt = want.stats?.started_at;
  const completedAt = want.stats?.completed_at;

  const isRunning = want.status === 'running';
  const isSuspended = want.suspended === true;
  const canControl = want.status === 'running' || want.status === 'stopped';
  const canSuspendResume = isRunning && (onSuspend || onResume);

  return (
    <div className={classNames(
      'card hover:shadow-md transition-shadow duration-200 cursor-pointer group',
      className
    )}>
      {/* Header */}
      <div className="flex items-start justify-between mb-4">
        <div className="flex-1 min-w-0">
          <h3
            className="text-lg font-semibold text-gray-900 truncate group-hover:text-primary-600 transition-colors"
            onClick={() => onView(want)}
          >
            {truncateText(wantName, 30)}
          </h3>
          <p className="text-sm text-gray-500 mt-1">
            Type: <span className="font-medium">{wantType}</span>
          </p>
          <p className="text-xs text-gray-400 mt-1">
            ID: {want.id}
          </p>
        </div>

        <div className="flex items-center space-x-2">
          <StatusBadge status={want.status} size="sm" />

          {/* Actions dropdown */}
          <div className="relative group/menu">
            <button className="p-1 rounded-md text-gray-400 hover:text-gray-600 hover:bg-gray-100">
              <MoreHorizontal className="h-4 w-4" />
            </button>

            <div className="absolute right-0 top-8 w-48 bg-white rounded-md shadow-lg border border-gray-200 z-10 opacity-0 invisible group-hover/menu:opacity-100 group-hover/menu:visible transition-all duration-200">
              <div className="py-1">
                <button
                  onClick={() => onView(want)}
                  className="flex items-center w-full px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
                >
                  <Eye className="h-4 w-4 mr-2" />
                  View Details
                </button>

                {!isRunning && (
                  <button
                    onClick={() => onEdit(want)}
                    className="flex items-center w-full px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
                  >
                    <Edit className="h-4 w-4 mr-2" />
                    Edit
                  </button>
                )}

                {canSuspendResume && !isSuspended && onSuspend && (
                  <button
                    onClick={() => onSuspend(want)}
                    className="flex items-center w-full px-4 py-2 text-sm text-orange-600 hover:bg-orange-50"
                    title="Suspend execution"
                  >
                    <Pause className="h-4 w-4 mr-2" />
                    Suspend
                  </button>
                )}

                {canSuspendResume && isSuspended && onResume && (
                  <button
                    onClick={() => onResume(want)}
                    className="flex items-center w-full px-4 py-2 text-sm text-green-600 hover:bg-green-50"
                    title="Resume execution"
                  >
                    <Play className="h-4 w-4 mr-2" />
                    Resume
                  </button>
                )}

                {canControl && (
                  <button
                    onClick={() => {/* TODO: Implement stop/start */}}
                    className="flex items-center w-full px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
                  >
                    {isRunning ? (
                      <>
                        <Square className="h-4 w-4 mr-2" />
                        Stop
                      </>
                    ) : (
                      <>
                        <Play className="h-4 w-4 mr-2" />
                        Start
                      </>
                    )}
                  </button>
                )}

                <hr className="my-1" />

                <button
                  onClick={() => onDelete(want)}
                  className="flex items-center w-full px-4 py-2 text-sm text-red-600 hover:bg-red-50"
                >
                  <Trash2 className="h-4 w-4 mr-2" />
                  Delete
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Labels */}
      {Object.keys(labels).length > 0 && (
        <div className="mb-4">
          <div className="flex flex-wrap gap-1">
            {Object.entries(labels).slice(0, 3).map(([key, value]) => (
              <span
                key={key}
                className="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium bg-gray-100 text-gray-800"
              >
                {key}: {value}
              </span>
            ))}
            {Object.keys(labels).length > 3 && (
              <span className="text-xs text-gray-500">
                +{Object.keys(labels).length - 3} more
              </span>
            )}
          </div>
        </div>
      )}

      {/* Timeline */}
      <div className="space-y-2 text-sm text-gray-600">
        {createdAt && (
          <div className="flex justify-between">
            <span>Created:</span>
            <span>{formatDate(createdAt)}</span>
          </div>
        )}

        {startedAt && (
          <div className="flex justify-between">
            <span>Started:</span>
            <span>{formatDate(startedAt)}</span>
          </div>
        )}

        {completedAt && (
          <div className="flex justify-between">
            <span>Completed:</span>
            <span>{formatDate(completedAt)}</span>
          </div>
        )}

        {startedAt && (
          <div className="flex justify-between">
            <span>Duration:</span>
            <span>{formatDuration(startedAt, completedAt)}</span>
          </div>
        )}
      </div>

      {/* Progress indicator for running wants */}
      {isRunning && (
        <div className="mt-4">
          <div className="flex items-center justify-between text-sm text-gray-600 mb-1">
            <span>{isSuspended ? 'Suspended' : 'Running...'}</span>
            <div className={classNames(
              'w-2 h-2 rounded-full',
              isSuspended ? 'bg-orange-500' : 'bg-blue-500 animate-pulse'
            )} />
          </div>
          <div className="w-full bg-gray-200 rounded-full h-1">
            <div
              className={classNames(
                'h-1 rounded-full',
                isSuspended ? 'bg-orange-500' : 'bg-blue-500 animate-pulse-slow'
              )}
              style={{ width: isSuspended ? '50%' : '70%' }}
            />
          </div>
          {isSuspended && (
            <div className="flex items-center mt-2 text-xs text-orange-600">
              <Pause className="h-3 w-3 mr-1" />
              Execution paused
            </div>
          )}
        </div>
      )}

      {/* Results summary */}
      {want.results && Object.keys(want.results).length > 0 && (
        <div className="mt-4 pt-4 border-t border-gray-200">
          <p className="text-sm text-gray-600">
            Results: {Object.keys(want.results).length} item{Object.keys(want.results).length !== 1 ? 's' : ''}
          </p>
        </div>
      )}
    </div>
  );
};