import React from 'react';
import { AlertTriangle, Bot, Pause } from 'lucide-react';
import { Want } from '@/types/want';
import { StatusBadge } from '@/components/common/StatusBadge';
import { formatDate, formatDuration, truncateText, classNames } from '@/utils/helpers';

interface WantCardContentProps {
  want: Want;
  isChild?: boolean;
  onView: (want: Want) => void;
  onViewAgents?: (want: Want) => void;
  onEdit?: (want: Want) => void;
  onDelete?: (want: Want) => void;
  onSuspend?: (want: Want) => void;
  onResume?: (want: Want) => void;
}

export const WantCardContent: React.FC<WantCardContentProps> = ({
  want,
  isChild = false,
  onView,
  onViewAgents,
  onEdit,
  onDelete,
  onSuspend,
  onResume
}) => {
  const wantName = want.metadata?.name || want.metadata?.id || 'Unnamed Want';
  const wantType = want.metadata?.type || 'unknown';
  const labels = want.metadata?.labels || {};
  const createdAt = want.stats?.created_at;
  const startedAt = want.stats?.started_at;
  const completedAt = want.stats?.completed_at;

  const isRunning = want.status === 'reaching';
  const isFailed = want.status === 'failed';
  const hasError = Boolean(isFailed && want.state?.error);
  const isSuspended = want.status === 'suspended';
  const canControl = want.status === 'reaching' || want.status === 'stopped';
  const canSuspendResume = isRunning && (onSuspend || onResume);

  // Responsive sizing based on whether it's a child card
  const sizes = isChild ? {
    titleClass: 'text-sm font-semibold',
    typeClass: 'text-xs',
    idClass: 'text-xs',
    iconSize: 'h-3 w-3',
    statusSize: 'xs' as const,
    agentDotSize: 'w-1.5 h-1.5',
    progressHeight: 'h-1',
    errorIconSize: 'h-3 w-3',
    errorTextSize: 'text-xs',
    textTruncate: 25
  } : {
    titleClass: 'text-lg font-semibold',
    typeClass: 'text-sm',
    idClass: 'text-xs',
    iconSize: 'h-4 w-4',
    statusSize: 'sm' as const,
    agentDotSize: 'w-2 h-2',
    progressHeight: 'h-1',
    errorIconSize: 'h-4 w-4',
    errorTextSize: 'text-sm',
    textTruncate: 30
  };

  return (
    <>
      {/* Header */}
      <div className="mb-4">
        <div className="flex items-start justify-between">
          <div className="flex-1 min-w-0">
            <h3
              className={`${sizes.titleClass} text-gray-900 truncate group-hover:text-primary-600 transition-colors`}
            >
              {wantType}
            </h3>
            <p className={`${sizes.typeClass} text-gray-500 mt-1 truncate`}>
              {truncateText(wantName, sizes.textTruncate)}
            </p>
          </div>

          <div className="flex items-center space-x-2 ml-2">
            {/* Agent indicator - clickable */}
            {(want.current_agent || (want.running_agents && want.running_agents.length > 0) || (want.history?.agentHistory && want.history.agentHistory.length > 0)) && (
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  if (onViewAgents) {
                    onViewAgents(want);
                  }
                }}
                className="flex items-center space-x-1 p-1 rounded-md hover:bg-blue-50 transition-colors cursor-pointer"
                title="Click to view agent details"
              >
                <Bot className={`${sizes.iconSize} text-blue-600 hover:text-blue-800`} />
                {want.current_agent && (
                  <div className={`${sizes.agentDotSize} bg-green-500 rounded-full animate-pulse`} title="Agent running" />
                )}
                {want.history?.agentHistory && want.history.agentHistory.length > 0 && (
                  <span className={classNames(
                    `${sizes.agentDotSize} rounded-full`,
                    want.history.agentHistory[want.history.agentHistory.length - 1]?.status === 'completed' && 'bg-green-500',
                    want.history.agentHistory[want.history.agentHistory.length - 1]?.status === 'failed' && 'bg-red-500',
                    want.history.agentHistory[want.history.agentHistory.length - 1]?.status === 'reaching' && 'bg-blue-500 animate-pulse'
                  )} title={`Latest agent: ${want.history.agentHistory[want.history.agentHistory.length - 1]?.status || 'unknown'}`} />
                )}
              </button>
            )}

            <StatusBadge status={want.status} size={sizes.statusSize} />

          </div>
        </div>
      </div>


      {/* Timeline - only for parent cards */}
      {!isChild && (
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
      )}

      {/* Progress indicator for running wants */}
      {isRunning && (
        <div className="mt-4">
          <div className={`flex items-center justify-between ${sizes.errorTextSize} text-gray-600 mb-1`}>
            <span>{isSuspended ? 'Suspended' : 'Running...'}</span>
            <div className={classNames(
              `${sizes.agentDotSize} rounded-full`,
              isSuspended ? 'bg-orange-500' : 'bg-blue-500 animate-pulse'
            )} />
          </div>
          <div className={`w-full bg-gray-200 rounded-full ${sizes.progressHeight}`}>
            <div
              className={classNames(
                `${sizes.progressHeight} rounded-full`,
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

      {/* Error indicator */}
      {hasError && (
        <div className="mt-4 p-3 bg-red-100 border border-red-200 rounded-md">
          <div className="flex items-start">
            <AlertTriangle className={`${sizes.errorIconSize} text-red-600 mt-0.5 mr-2 flex-shrink-0`} />
            <div className="flex-1 min-w-0">
              <p className={`${sizes.errorTextSize} font-medium text-red-800`}>Execution Failed</p>
              <p className={`${sizes.errorTextSize} text-red-600 mt-1 truncate`}>
                {truncateText(typeof want.state?.error === 'string' ? want.state.error : 'Unknown error', isChild ? 60 : 100)}
              </p>
              <button
                onClick={() => onView(want)}
                className="text-xs text-red-700 hover:text-red-800 underline mt-1"
              >
                View details →
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Results summary */}
      {want.results && Object.keys(want.results).length > 0 && (
        <div className="mt-4 pt-4 border-t border-gray-200">
          <p className={`${sizes.errorTextSize} text-gray-600`}>
            Results: {Object.keys(want.results).length} item{Object.keys(want.results).length !== 1 ? 's' : ''}
          </p>
        </div>
      )}
    </>
  );
};