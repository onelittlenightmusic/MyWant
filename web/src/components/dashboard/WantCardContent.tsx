import React, { useState, useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { AlertTriangle, Bot, Heart, Pause, Clock, ThumbsUp, ThumbsDown, Trash2, Circle, X, Camera, Copy, Check } from 'lucide-react';
import { Want } from '@/types/want';
import { StatusBadge } from '@/components/common/StatusBadge';
import { ConfirmationBubble } from '@/components/notifications';
import { BrowserFrame } from '@/components/replay/BrowserFrame';
import { formatDate, formatDuration, truncateText, classNames } from '@/utils/helpers';
import styles from './WantCard.module.css';

const HeartInBottle: React.FC<{ className?: string }> = ({ className }) => (
  <span className={`relative inline-flex items-center justify-center flex-shrink-0 ${className ?? ''}`}>
    <span className="leading-none">🫙</span>
    <Heart className="absolute w-[45%] h-[45%] bottom-[10%] text-pink-500 drop-shadow-sm" fill="currentColor" strokeWidth={0} />
  </span>
);

const BottleOnly: React.FC<{ className?: string }> = ({ className }) => (
  <span className={`inline-flex items-center justify-center flex-shrink-0 leading-none ${className ?? ''}`}>
    🫙
  </span>
);

interface WantCardContentProps {
  want: Want;
  isChild?: boolean;
  hasChildren?: boolean;
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
  hasChildren = false,
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

  // Replay-specific state
  const isReplay = wantType === 'replay';
  const recordingActive = want.state?.recording_active === true;
  const debugRecordingActive = want.state?.debug_recording_active === true;
  const replayActive = want.state?.replay_active === true;
  const iframeUrl = want.state?.recording_iframe_url as string | undefined;
  const replayIframeUrl = want.state?.replay_iframe_url as string | undefined;
  const hasFinalResult = Boolean(want.state?.final_result);
  const hasReplayActions = Boolean(want.state?.replay_actions && (want.state?.replay_actions as string) !== '[]');
  const debugRecordingError = want.state?.debug_recording_error as string | undefined;
  const replayError = want.state?.replay_error as string | undefined;
  const replayResultRaw = want.state?.replay_result as string | undefined;
  const replayResult = (() => {
    if (!replayResultRaw) return null;
    try { return JSON.parse(replayResultRaw); } catch { return null; }
  })();
  const replayScreenshotUrl = want.state?.replay_screenshot_url as string | undefined;


  // Webhook IDs: prefer state value (set by MonitorAgent), fall back to predictable pattern from want ID.
  // This ensures the Record button appears immediately after want creation, before the MonitorAgent runs.
  const wantId = want.metadata?.id ?? '';
  const startWebhookId = (want.state?.startWebhookId as string | undefined) || (wantId ? `${wantId}-start` : undefined);
  const stopWebhookId = (want.state?.stopWebhookId as string | undefined) || (wantId ? `${wantId}-stop` : undefined);
  const debugStartWebhookId = (want.state?.debugStartWebhookId as string | undefined) || (wantId ? `${wantId}-debug-start` : undefined);
  const debugStopWebhookId = (want.state?.debugStopWebhookId as string | undefined) || (wantId ? `${wantId}-debug-stop` : undefined);
  const replayWebhookId = (want.state?.replayWebhookId as string | undefined) || (wantId ? `${wantId}-replay` : undefined);

  const handleStartRecording = async () => {
    if (!startWebhookId) return;
    try {
      await fetch(`/api/v1/webhooks/${startWebhookId}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action: 'start_recording' }),
      });
    } catch (err) {
      console.error('[WantCard] start recording webhook failed:', err);
    }
  };

  const handleStartDebugRecording = async () => {
    if (!debugStartWebhookId) return;
    try {
      await fetch(`/api/v1/webhooks/${debugStartWebhookId}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action: 'start_debug_recording' }),
      });
    } catch (err) {
      console.error('[WantCard] start debug recording webhook failed:', err);
    }
  };

  const handleFinishDebugRecording = async () => {
    if (!debugStopWebhookId) return;
    try {
      await fetch(`/api/v1/webhooks/${debugStopWebhookId}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action: 'stop_debug_recording' }),
      });
    } catch (err) {
      console.error('[WantCard] stop debug recording webhook failed:', err);
    }
  };

  const handleStartReplay = async () => {
    if (!replayWebhookId) return;
    try {
      await fetch(`/api/v1/webhooks/${replayWebhookId}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action: 'start_replay' }),
      });
    } catch (err) {
      console.error('[WantCard] start replay webhook failed:', err);
    }
  };

  // Replay / screenshot bubble state
  const [showReplayBubble, setShowReplayBubble] = useState(false);
  const [showScreenshotBubble, setShowScreenshotBubble] = useState(false);
  const [replayBubbleStyle, setReplayBubbleStyle] = useState<React.CSSProperties>({});
  const [screenshotBubbleStyle, setScreenshotBubbleStyle] = useState<React.CSSProperties>({});
  const replayBubbleRef = useRef<HTMLDivElement>(null);
  const screenshotBubbleRef = useRef<HTMLDivElement>(null);

  // Close bubbles on outside click
  useEffect(() => {
    if (!showReplayBubble && !showScreenshotBubble) return;
    const handleMouseDown = (e: MouseEvent) => {
      if (showReplayBubble && replayBubbleRef.current && !replayBubbleRef.current.contains(e.target as Node)) {
        setShowReplayBubble(false);
      }
      if (showScreenshotBubble && screenshotBubbleRef.current && !screenshotBubbleRef.current.contains(e.target as Node)) {
        setShowScreenshotBubble(false);
      }
    };
    document.addEventListener('mousedown', handleMouseDown);
    return () => document.removeEventListener('mousedown', handleMouseDown);
  }, [showReplayBubble, showScreenshotBubble]);

  // Auto-close replay bubble when replay finishes
  useEffect(() => {
    if (!replayActive) {
      const timer = setTimeout(() => setShowReplayBubble(false), 800);
      return () => clearTimeout(timer);
    }
  }, [replayActive]); // eslint-disable-line react-hooks/exhaustive-deps

  // Calculate bubble position anchored near the card
  const calcBubbleStyle = (e: React.MouseEvent, widthMultiplier = 1.3): React.CSSProperties => {
    const btn = e.currentTarget as HTMLElement;
    const card = btn.closest('[data-keyboard-nav-id]');
    const cardRect = card?.getBoundingClientRect() ?? btn.getBoundingClientRect();
    const btnRect = btn.getBoundingClientRect();

    const bubbleWidth = cardRect.width * widthMultiplier;
    const bubbleMaxHeight = Math.min(window.innerHeight * 0.75, 560);

    // Align left edge with card, adjust if off-screen
    let left = cardRect.left;
    if (left + bubbleWidth > window.innerWidth - 8) {
      left = window.innerWidth - 8 - bubbleWidth;
    }
    left = Math.max(8, left);

    // Position below the button, adjust if off bottom
    let top = btnRect.bottom + 8;
    if (top + bubbleMaxHeight > window.innerHeight - 8) {
      top = Math.max(8, btnRect.top - bubbleMaxHeight - 8);
    }

    return { position: 'fixed', left, top, width: bubbleWidth, maxHeight: bubbleMaxHeight };
  };

  // Confirmation dialog state
  const [showConfirmation, setShowConfirmation] = useState(false);
  const [confirmationAction, setConfirmationAction] = useState<'approve' | 'deny' | null>(null);
  const [isSubmittingReaction, setIsSubmittingReaction] = useState(false);
  const [confirmationMessage, setConfirmationMessage] = useState<string | null>(null);

  // Copy state for final_result
  const [finalResultCopied, setFinalResultCopied] = useState(false);
  const handleCopyFinalResult = (e: React.MouseEvent) => {
    e.stopPropagation();
    const value = want.state?.final_result;
    const text = typeof value === 'string' ? value : JSON.stringify(value);
    navigator.clipboard.writeText(text).then(() => {
      setFinalResultCopied(true);
      setTimeout(() => setFinalResultCopied(false), 1500);
    });
  };

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
              {labels['recipe-based'] === 'true' ? (
                hasChildren ? (
                  <HeartInBottle className={`${sizes.iconSize} flex-shrink-0 text-pink-500`} />
                ) : (
                  <BottleOnly className={sizes.iconSize} />
                )
              ) : (
                <Heart className={`${sizes.iconSize} flex-shrink-0 text-pink-500`} />
              )}
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

      {/* Replay type: Record / Record in debug buttons (shown when idle, no final result yet) */}
      {isReplay && !recordingActive && !debugRecordingActive && !hasFinalResult && (
        <div className={`flex items-center gap-2 ${isChild ? "mt-2" : "mt-4"}`}>
          {startWebhookId && (
            <button
              onClick={(e) => { e.stopPropagation(); handleStartRecording(); }}
              className="flex items-center gap-1.5 px-2 py-1 rounded text-xs bg-red-600 text-white hover:bg-red-700 transition-colors"
              title="Start browser recording (new browser window)"
            >
              <Circle className="w-3 h-3 fill-current" />
              Record
            </button>
          )}
          {debugStartWebhookId && (
            <button
              onClick={(e) => { e.stopPropagation(); handleStartDebugRecording(); }}
              className="flex items-center gap-1.5 px-2 py-1 rounded text-xs bg-orange-600 text-white hover:bg-orange-700 transition-colors"
              title="Record in debug Chrome (port 9222)"
            >
              <Circle className="w-3 h-3 fill-current" />
              Record in debug
            </button>
          )}
        </div>
      )}

      {/* Replay type: iframe shown while normal recording is active */}
      {isReplay && recordingActive && iframeUrl && (
        <BrowserFrame
          iframeUrl={iframeUrl}
          wantId={want.metadata?.id ?? ''}
          stopWebhookId={stopWebhookId ?? ''}
        />
      )}

      {/* Replay type: Finish button shown while debug recording is active (no iframe) */}
      {isReplay && debugRecordingActive && (
        <div className={isChild ? "mt-2" : "mt-4"}>
          <div className="flex items-center gap-2 p-2 rounded bg-orange-50 dark:bg-orange-900/20 border border-orange-200 dark:border-orange-800">
            <span className="inline-block w-2 h-2 rounded-full bg-orange-500 animate-pulse flex-shrink-0" />
            <span className="text-xs text-orange-700 dark:text-orange-300 flex-1">Recording debug Chrome…</span>
            <button
              onClick={(e) => { e.stopPropagation(); handleFinishDebugRecording(); }}
              className="flex items-center gap-1 px-2 py-1 rounded text-xs bg-orange-600 text-white hover:bg-orange-700 transition-colors flex-shrink-0"
              title="Finish debug recording"
            >
              Finish
            </button>
          </div>
        </div>
      )}

      {/* Replay type: debug recording error */}
      {isReplay && debugRecordingError && !debugRecordingActive && (
        <div className={isChild ? "mt-2" : "mt-4"}>
          <div className="flex items-start gap-2 p-2 rounded bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800">
            <span className="text-xs text-red-700 dark:text-red-300 flex-1 break-all">
              Debug recording failed: {debugRecordingError}
            </span>
          </div>
        </div>
      )}

      {/* Replay type: Replay button / replaying indicator */}
      {isReplay && hasReplayActions && replayWebhookId && (
        <div className={isChild ? "mt-2" : "mt-4"}>
          {!replayActive ? (
            /* Idle: Replay button - click triggers replay, does NOT open bubble */
            <button
              onClick={(e) => { e.stopPropagation(); handleStartReplay(); }}
              className="flex items-center gap-1.5 px-2 py-1 rounded text-xs bg-green-600 text-white hover:bg-green-700 transition-colors"
              title="Replay the recorded script in a new browser"
            >
              ▶ Replay
            </button>
          ) : (
            /* Replaying: pulsing indicator - click opens floating bubble */
            <button
              onClick={(e) => {
                e.stopPropagation();
                setReplayBubbleStyle(calcBubbleStyle(e));
                setShowReplayBubble(true);
              }}
              className="flex items-center gap-1.5 px-2 py-1 rounded text-xs bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300 border border-green-300 dark:border-green-700 hover:bg-green-200 dark:hover:bg-green-900/50 transition-colors"
              title="Click to view replay"
            >
              <span className="w-1.5 h-1.5 rounded-full bg-green-500 animate-pulse flex-shrink-0" />
              Replaying…
            </button>
          )}
        </div>
      )}

      {/* Replay type: replay result */}
      {isReplay && replayResult && !replayActive && (
        <div className={isChild ? "mt-2" : "mt-4"}>
          <div className="p-2 rounded bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800">
            <div className="flex items-start justify-between gap-2">
              <div className="flex-1 min-w-0">
                <p className="text-xs font-semibold text-green-700 dark:text-green-300 mb-1">Replay result</p>
                {replayResult.selected_text && (
                  <p className="text-xs text-green-800 dark:text-green-200 break-all">
                    Selected: <span className="font-mono bg-green-100 dark:bg-green-900 px-1 rounded">{replayResult.selected_text}</span>
                  </p>
                )}
                {replayResult.url && (
                  <p className="text-xs text-green-700 dark:text-green-400 mt-0.5 truncate">URL: {replayResult.url}</p>
                )}
              </div>
              {replayScreenshotUrl && (
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    setScreenshotBubbleStyle(calcBubbleStyle(e, 1.3));
                    setShowScreenshotBubble(true);
                  }}
                  className="flex-shrink-0 p-1 rounded bg-green-100 dark:bg-green-900/40 text-green-600 dark:text-green-400 hover:bg-green-200 dark:hover:bg-green-800 transition-colors"
                  title="View replay screenshot"
                >
                  <Camera className="w-3.5 h-3.5" />
                </button>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Replay type: replay error */}
      {isReplay && replayError && !replayActive && (
        <div className={isChild ? "mt-2" : "mt-4"}>
          <div className="flex items-start gap-2 p-2 rounded bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800">
            <span className="text-xs text-red-700 dark:text-red-300 flex-1 break-all">Replay failed: {replayError}</span>
          </div>
        </div>
      )}

      {/* Final result display */}
      {want.state?.final_result && (
        <div className={`${isChild ? "mt-2" : "mt-4"} group/finalresult relative`}>
          <button
            onClick={() => onViewResults ? onViewResults(want) : onView(want)}
            className={`inline-flex items-center gap-1.5 ${isChild ? 'text-xs sm:text-sm' : 'text-sm sm:text-base'} font-bold text-purple-700 dark:text-purple-300 bg-purple-500/15 dark:bg-purple-900/30 border border-purple-400/40 dark:border-purple-800 rounded-full px-2 py-0.5 w-full text-left transition-colors cursor-pointer hover:text-primary-600 dark:hover:text-primary-400 pr-7`}
            title="Click to view results"
          >
            <svg xmlns="http://www.w3.org/2000/svg" className={isChild ? "h-3 w-3 shrink-0" : "h-4 w-4 shrink-0"} viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M4 4a2 2 0 012-2h4.586A2 2 0 0112 2.586L15.414 6A2 2 0 0116 7.414V16a2 2 0 01-2 2H6a2 2 0 01-2-2V4zm2 6a1 1 0 011-1h6a1 1 0 110 2H7a1 1 0 01-1-1zm1 3a1 1 0 100 2h6a1 1 0 100-2H7z" clipRule="evenodd" />
            </svg>
            <span className="truncate">
              {truncateText(
                typeof want.state.final_result === 'string'
                  ? want.state.final_result
                  : JSON.stringify(want.state.final_result),
                isChild ? 40 : 50
              )}
            </span>
          </button>
          <button
            onClick={handleCopyFinalResult}
            className="absolute right-1 top-1/2 -translate-y-1/2 opacity-0 group-hover/finalresult:opacity-100 transition-opacity p-0.5 rounded text-purple-500 hover:text-purple-700 dark:text-purple-400 dark:hover:text-purple-200 hover:bg-purple-100 dark:hover:bg-purple-900/40"
            title="Copy to clipboard"
          >
            {finalResultCopied
              ? <Check className={isChild ? "w-3 h-3" : "w-3.5 h-3.5"} />
              : <Copy className={isChild ? "w-3 h-3" : "w-3.5 h-3.5"} />
            }
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

      {/* Floating Replay Bubble - portal, no backdrop, anchored near card */}
      {showReplayBubble && createPortal(
        <div
          ref={replayBubbleRef}
          className="z-[9999] bg-white dark:bg-gray-900 rounded-2xl shadow-2xl border border-gray-200 dark:border-gray-700 flex flex-col overflow-hidden"
          style={replayBubbleStyle}
        >
          {/* Header */}
          <div className="flex items-center justify-between px-3 py-2 border-b border-gray-200 dark:border-gray-700 flex-shrink-0">
            <div className="flex items-center gap-2">
              {replayActive && <span className="w-2 h-2 rounded-full bg-green-500 animate-pulse flex-shrink-0" />}
              <span className="text-xs font-semibold text-gray-800 dark:text-gray-100">
                {replayActive ? 'Replaying…' : 'Replay'}
              </span>
              <span className="text-xs text-gray-400 dark:text-gray-500 truncate max-w-[120px]">{wantName}</span>
            </div>
            <button
              onClick={() => setShowReplayBubble(false)}
              className="p-1 rounded text-gray-400 hover:text-gray-600 hover:bg-gray-100 dark:hover:text-gray-200 dark:hover:bg-gray-800 transition-colors"
            >
              <X className="w-3.5 h-3.5" />
            </button>
          </div>
          {/* Iframe */}
          <div className="flex-1 min-h-0">
            {replayIframeUrl ? (
              <iframe
                src={replayIframeUrl}
                className="w-full h-full border-0"
                title="Replay viewer"
              />
            ) : (
              <div className="flex items-center justify-center h-32 gap-2 text-gray-400 dark:text-gray-500">
                <span className="w-2 h-2 rounded-full bg-gray-300 dark:bg-gray-600 animate-pulse" />
                <span className="text-xs">Starting replay…</span>
              </div>
            )}
          </div>
        </div>,
        document.body
      )}

      {/* Floating Screenshot Bubble - portal, no backdrop, anchored near card */}
      {showScreenshotBubble && replayScreenshotUrl && createPortal(
        <div
          ref={screenshotBubbleRef}
          className="z-[9999] bg-white dark:bg-gray-900 rounded-2xl shadow-2xl border border-gray-200 dark:border-gray-700 flex flex-col overflow-hidden"
          style={screenshotBubbleStyle}
        >
          {/* Header */}
          <div className="flex items-center justify-between px-3 py-2 border-b border-gray-200 dark:border-gray-700 flex-shrink-0">
            <div className="flex items-center gap-2">
              <Camera className="w-3.5 h-3.5 text-gray-500 dark:text-gray-400" />
              <span className="text-xs font-semibold text-gray-800 dark:text-gray-100">Screenshot</span>
              <span className="text-xs text-gray-400 dark:text-gray-500 truncate max-w-[120px]">{wantName}</span>
            </div>
            <button
              onClick={() => setShowScreenshotBubble(false)}
              className="p-1 rounded text-gray-400 hover:text-gray-600 hover:bg-gray-100 dark:hover:text-gray-200 dark:hover:bg-gray-800 transition-colors"
            >
              <X className="w-3.5 h-3.5" />
            </button>
          </div>
          {/* Image */}
          <div className="overflow-auto">
            <img src={replayScreenshotUrl} alt="Replay screenshot" className="block w-full" />
          </div>
        </div>,
        document.body
      )}
    </>
  );
};