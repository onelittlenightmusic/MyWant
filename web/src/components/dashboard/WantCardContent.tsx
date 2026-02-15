import React, { useState } from 'react';
import { AlertTriangle, Bot, Heart, Pause, Clock, ThumbsUp, ThumbsDown, Trash2 } from 'lucide-react';
import { Want } from '@/types/want';
import { StatusBadge } from '@/components/common/StatusBadge';
import { ConfirmationBubble } from '@/components/notifications';
import { formatDate, formatDuration, truncateText, classNames } from '@/utils/helpers';
import styles from './WantCard.module.css';

interface WantCardContentProps {
  want: Want;
  isChild?: boolean;
  onView: (want: Want) => void;
  onViewAgents?: (want: Want) => void;
  onViewResults?: (want: Want) => void;
  onEdit?: (want: Want) => void;
  onDelete?: (want: Want) => void;
  onSuspend?: (want: Want) => void;
  onResume?: (want: Want) => void;
  onShowReactionConfirmation?: (want: Want, action: 'approve' | 'deny') => void;
}

export const WantCardContent: React.FC<WantCardContentProps> = ({
  want,
  isChild = false,
  onView,
  onViewAgents,
  onViewResults,
  onEdit,
  onDelete,
  onSuspend,
  onResume,
  onShowReactionConfirmation
}) => {
  const wantName = want.metadata?.name || want.metadata?.id || 'Unnamed Want';
  const wantType = want.metadata?.type || 'unknown';
  const labels = want.metadata?.labels || {};
  const createdAt = want.stats?.created_at;
  const startedAt = want.stats?.started_at;
  const completedAt = want.stats?.completed_at;

  // Reminder-specific state
  const isReminder = want.metadata?.type === 'reminder';
  const reminderPhase = want.state?.reminder_phase as string | undefined;
  const queueId = want.state?.reaction_queue_id as string | undefined;
  const requireReaction = want.spec?.params?.require_reaction !== false; // Default to true
  const shouldShowReactionButtons = isReminder && reminderPhase === 'reaching' && queueId && requireReaction;

  // Confirmation dialog state
  const [showConfirmation, setShowConfirmation] = useState(false);
  const [confirmationAction, setConfirmationAction] = useState<'approve' | 'deny' | null>(null);
  const [isSubmittingReaction, setIsSubmittingReaction] = useState(false);
  const [confirmationMessage, setConfirmationMessage] = useState<string | null>(null);

  const isRunning = want.status === 'reaching' || want.status === 'waiting_user_action';
  const isFailed = want.status === 'failed';
  const hasError = Boolean(isFailed && want.state?.error);
  const isSuspended = want.status === 'suspended';
  const canControl = want.status === 'reaching' || want.status === 'waiting_user_action' || want.status === 'stopped';
  const canSuspendResume = isRunning && (onSuspend || onResume);
  const hasScheduling = (want.spec?.when && want.spec.when.length > 0);

  // Handler for approval button
  const handleApproveClick = () => {
    if (onShowReactionConfirmation) {
      onShowReactionConfirmation(want, 'approve');
    } else {
      // Fallback to local confirmation if handler not provided
      setConfirmationAction('approve');
      setConfirmationMessage('reminder');
      setShowConfirmation(true);
    }
  };

  // Handler for denial button
  const handleDenyClick = () => {
    if (onShowReactionConfirmation) {
      onShowReactionConfirmation(want, 'deny');
    } else {
      // Fallback to local confirmation if handler not provided
      setConfirmationAction('deny');
      setConfirmationMessage('reminder');
      setShowConfirmation(true);
    }
  };

  // Handler for confirmation dialog confirmation
  const handleReactionConfirm = async () => {
    if (!queueId || !confirmationAction) return;

    console.log('[REACTION] Starting reaction submission...');
    console.log('[REACTION] Queue ID:', queueId);
    console.log('[REACTION] Action:', confirmationAction);

    setIsSubmittingReaction(true);
    try {
      const requestBody = {
        approved: confirmationAction === 'approve',
        comment: `User ${confirmationAction === 'approve' ? 'approved' : 'denied'} reminder`
      };
      console.log('[REACTION] Request body:', requestBody);

      const url = `/api/v1/reactions/${queueId}`;
      console.log('[REACTION] Sending PUT request to:', url);

      const response = await fetch(url, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify(requestBody)
      });

      console.log('[REACTION] Response status:', response.status);
      console.log('[REACTION] Response ok:', response.ok);

      if (!response.ok) {
        const errorText = await response.text();
        console.error('[REACTION] Error response:', errorText);
        throw new Error(`Failed to submit reaction: ${response.statusText}`);
      }

      const responseData = await response.json();
      console.log('[REACTION] Response data:', responseData);

      // Success - close confirmation dialog
      setShowConfirmation(false);
      setConfirmationAction(null);
      setConfirmationMessage(null);

      // Optionally refresh the want data or show success message
      console.log(`Reminder ${confirmationAction === 'approve' ? 'approved' : 'denied'} successfully`);
    } catch (error) {
      console.error('Error submitting reaction:', error);
      // Could show error notification here
    } finally {
      setIsSubmittingReaction(false);
    }
  };

  // Handler for confirmation dialog cancellation
  const handleReactionCancel = () => {
    setShowConfirmation(false);
    setConfirmationAction(null);
    setConfirmationMessage(null);
  };

  // Responsive sizing based on whether it's a child card
  type SizeConfig = {
    titleClass: string;
    typeClass: string;
    idClass: string;
    iconSize: string;
    statusSize: 'xs' | 'sm' | 'md' | 'lg';
    agentDotSize: string;
    errorIconSize: string;
    errorTextSize: string;
    textTruncate: number;
  };

  const sizes: SizeConfig = isChild ? {
    titleClass: 'text-[11px] sm:text-sm font-semibold',
    typeClass: 'text-[9px] sm:text-xs',
    idClass: 'text-[9px] sm:text-xs',
    iconSize: 'h-2.5 w-2.5 sm:h-3 w-3',
    statusSize: 'xs' as const,
    agentDotSize: 'w-1 h-1 sm:w-1.5 h-1.5',
    errorIconSize: 'h-2.5 w-2.5',
    errorTextSize: 'text-[10px]',
    textTruncate: 20
  } : {
    titleClass: 'text-xs sm:text-lg font-semibold',
    typeClass: 'text-[10px] sm:text-sm',
    idClass: 'text-[10px] sm:text-xs',
    iconSize: 'h-3 w-3 sm:h-4 w-4',
    statusSize: 'xs' as const,
    agentDotSize: 'w-1.5 h-1.5 sm:w-2 h-2',
    errorIconSize: 'h-3.5 w-3.5 sm:h-4 w-4',
    errorTextSize: 'text-[11px] sm:text-sm',
    textTruncate: 25
  };

  return (
    <>
      {/* Header */}
      <div className="mb-2 sm:mb-4">
        <div className="flex items-start justify-between">
          <div className="flex-1 min-w-0">
            <h3
              className={`${sizes.titleClass} text-gray-900 dark:text-gray-100 truncate group-hover:text-primary-600 dark:group-hover:text-primary-400 transition-colors flex items-center gap-1.5`}
            >
              <Heart className={`${sizes.iconSize} flex-shrink-0 text-pink-500`} />
              {wantType}
            </h3>
            <p className={`${sizes.typeClass} text-gray-500 dark:text-gray-400 mt-1 truncate`}>
              {truncateText(wantName, sizes.textTruncate)}
            </p>
          </div>

          <div className="flex items-center space-x-1 sm:space-x-2 ml-1 sm:ml-2">
            {/* Agent indicator - clickable */}
            {(want.current_agent || (want.running_agents && want.running_agents.length > 0) || (want.history?.agentHistory && want.history.agentHistory.length > 0)) && (
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  if (onViewAgents) {
                    onViewAgents(want);
                  }
                }}
                className="flex items-center space-x-1 p-1 rounded-md hover:bg-blue-50 dark:hover:bg-blue-900/30 transition-colors cursor-pointer"
                title="Click to view agent details"
              >
                <Bot className={`${sizes.iconSize} text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300`} />
                {want.current_agent && (
                  <div className={`${sizes.agentDotSize} bg-green-500 rounded-full ${styles.pulseGlow}`} title="Agent running" />
                )}
                {want.history?.agentHistory && want.history.agentHistory.length > 0 && (
                  <span className={classNames(
                    `${sizes.agentDotSize} rounded-full`,
                    want.history.agentHistory[want.history.agentHistory.length - 1]?.status === 'achieved' && 'bg-green-500',
                    want.history.agentHistory[want.history.agentHistory.length - 1]?.status === 'failed' && 'bg-red-500',
                    want.history.agentHistory[want.history.agentHistory.length - 1]?.status === 'reaching' && `bg-blue-500 ${styles.pulseGlow}`
                  )} title={`Latest agent: ${want.history.agentHistory[want.history.agentHistory.length - 1]?.status || 'unknown'}`} />
                )}
              </button>
            )}

            {/* Scheduling indicator */}
            {hasScheduling && (
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  onView(want);
                }}
                className={classNames(
                  'inline-flex items-center gap-1 font-medium rounded-full border hover:opacity-80 transition-opacity',
                  sizes.statusSize === 'xs' ? 'px-1.5 py-0.5 text-xs' :
                  sizes.statusSize === 'sm' ? 'px-2 py-1 text-xs' :
                  sizes.statusSize === 'md' ? 'px-2.5 py-1.5 text-sm' :
                  'px-3 py-2 text-base',
                  'bg-amber-100 text-amber-800 border-amber-200 dark:bg-amber-900/30 dark:text-amber-200 dark:border-amber-700'
                )}
                title="Click to view scheduling settings"
              >
                <Clock className={sizes.iconSize} />
              </button>
            )}

            <button
              onClick={(e) => {
                e.stopPropagation();
                onView(want);
              }}
              className="hover:opacity-80 transition-opacity"
              title="Click to view details"
            >
              <StatusBadge status={want.status} size={sizes.statusSize} />
            </button>

            {/* Delete Button - Common mechanism for all cards */}
            {onDelete && (
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  onDelete(want);
                }}
                className="p-1.5 text-gray-400 dark:text-gray-500 hover:text-red-600 dark:hover:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30 rounded-md transition-all ml-1"
                title="Delete"
              >
                <Trash2 className={sizes.iconSize} />
              </button>
            )}

          </div>
        </div>
      </div>


      {/* Timeline - only for parent cards - DISABLED to keep consistent height */}
      {false && !isChild && (
        <div className="space-y-2 text-sm text-gray-600 dark:text-gray-400">
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
              <span>Achieved:</span>
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
      {/* Error indicator */}
      {hasError && (
        <div className="mt-4 p-3 bg-red-100 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md">
          <div className="flex items-start">
            <AlertTriangle className={`${sizes.errorIconSize} text-red-600 dark:text-red-400 mt-0.5 mr-2 flex-shrink-0`} />
            <div className="flex-1 min-w-0">
              <p className={`${sizes.errorTextSize} font-medium text-red-800 dark:text-red-200`}>Execution Failed</p>
              <p className={`${sizes.errorTextSize} text-red-600 dark:text-red-400 mt-1 truncate`}>
                {truncateText(typeof want.state?.error === 'string' ? want.state.error : 'Unknown error', isChild ? 60 : 100)}
              </p>
              <button
                onClick={() => onView(want)}
                className="text-xs text-red-700 dark:text-red-300 hover:text-red-800 dark:hover:text-red-200 underline mt-1"
              >
                View details →
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Results summary */}
      {want.results && Object.keys(want.results).length > 0 && (
        <div className="mt-4 pt-4 border-t border-gray-200 dark:border-gray-700">
          <p className={`${sizes.errorTextSize} text-gray-600 dark:text-gray-400`}>
            Results: {Object.keys(want.results).length} item{Object.keys(want.results).length !== 1 ? 's' : ''}
          </p>
        </div>
      )}

      {/* Final result display */}
      {want.state?.final_result && (
        <div className={isChild ? "mt-2" : "mt-4 pt-4 border-t border-gray-200 dark:border-gray-700"}>
          <button
            onClick={() => onViewResults ? onViewResults(want) : onView(want)}
            className={`${isChild ? 'text-xs sm:text-base' : 'text-sm sm:text-lg'} font-bold text-gray-900 dark:text-white truncate w-full text-left transition-colors cursor-pointer hover:text-primary-600 dark:hover:text-primary-400`}
            title="Click to view results"
          >
            {truncateText(
              typeof want.state.final_result === 'string'
                ? want.state.final_result
                : JSON.stringify(want.state.final_result),
              isChild ? 40 : 50
            )}
          </button>
        </div>
      )}

      {/* Reminder Reaction Buttons */}
      {shouldShowReactionButtons && (
        <div className={isChild ? "mt-2" : "mt-4 pt-4 border-t border-gray-200 dark:border-gray-700"}>
          <div className="flex gap-2">
            <button
              onClick={handleDenyClick}
              disabled={isSubmittingReaction}
              className={classNames(
                'flex-1 flex items-center justify-center gap-2 px-3 py-2 rounded-md text-sm font-medium',
                'bg-red-100 text-red-700 hover:bg-red-200 dark:bg-red-900/30 dark:text-red-300 dark:hover:bg-red-900/50',
                'disabled:opacity-50 disabled:cursor-not-allowed',
                'transition-colors'
              )}
              title="Reject this reminder"
            >
              <ThumbsDown className="h-4 w-4" />
              <span className={isChild ? 'hidden' : ''}>Deny</span>
            </button>
            <button
              onClick={handleApproveClick}
              disabled={isSubmittingReaction}
              className={classNames(
                'flex-1 flex items-center justify-center gap-2 px-3 py-2 rounded-md text-sm font-medium',
                'bg-green-100 text-green-700 hover:bg-green-200 dark:bg-green-900/30 dark:text-green-300 dark:hover:bg-green-900/50',
                'disabled:opacity-50 disabled:cursor-not-allowed',
                'transition-colors'
              )}
              title="Approve this reminder"
            >
              <ThumbsUp className="h-4 w-4" />
              <span className={isChild ? 'hidden' : ''}>Approve</span>
            </button>
          </div>
          <p className="text-xs text-gray-500 dark:text-gray-400 mt-2 text-center">
            Waiting for your decision...
          </p>
        </div>
      )}

      {/* Confirmation Message Notification */}
      <ConfirmationBubble
        message={confirmationMessage}
        isVisible={showConfirmation}
        onDismiss={() => setShowConfirmation(false)}
        onConfirm={handleReactionConfirm}
        onCancel={handleReactionCancel}
        loading={isSubmittingReaction}
        title="Confirm"
      />
    </>
  );
};