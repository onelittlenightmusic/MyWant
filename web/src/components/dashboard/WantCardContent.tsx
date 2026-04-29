import React, { useState, useRef } from 'react';
import { AlertTriangle, Bot, Heart, Clock, ThumbsUp, ThumbsDown, Copy, Check, MessageSquare } from 'lucide-react';
import { Want } from '@/types/want';
import { StatusBadge } from '@/components/common/StatusBadge';
import { ArrayResultTable } from '@/components/common/ArrayResultTable';
import { formatDate, formatDuration, truncateText, classNames } from '@/utils/helpers';
import { useWantStore } from '@/stores/wantStore';
import styles from './WantCard.module.css';

// Register all plugins (side-effect imports)
import './WantCard/plugins';
import { getWantCardPlugin } from './WantCard/plugins/registry';

// ── FinalResultDisplay ────────────────────────────────────────────────────────
const FinalResultDisplay: React.FC<{
  value: unknown;
  isChild: boolean;
  copied: boolean;
  onCopy: (e: React.MouseEvent) => void;
  onView: () => void;
}> = ({ value, isChild, copied, onCopy, onView }) => {
  const isArrayOfObjects =
    Array.isArray(value) &&
    value.length > 0 &&
    typeof value[0] === 'object' &&
    value[0] !== null;

  const fullText = typeof value === 'string' ? value : JSON.stringify(value, null, 2);
  const truncateLimit = isChild ? 40 : 50;

  if (isArrayOfObjects) {
    const data = value as Record<string, unknown>[];
    return (
      <div>
        <div className="flex items-center justify-between mb-0.5">
          <button
            onClick={onView}
            className={`${isChild ? 'text-[0.5rem]' : 'text-[0.55rem] sm:text-[0.6rem]'} font-mono text-green-400/70 hover:text-green-400 cursor-pointer`}
          >
            [{data.length} items] — view all
          </button>
          <button onClick={onCopy} className="p-0.5 rounded text-green-400" title="Copy to clipboard">
            {copied
              ? <Check className={isChild ? 'w-3 h-3' : 'w-3.5 h-3.5'} />
              : <Copy className={isChild ? 'w-3 h-3' : 'w-3.5 h-3.5'} />
            }
          </button>
        </div>
        <ArrayResultTable data={data} maxRows={isChild ? 3 : 5} size="compact" />
      </div>
    );
  }

  return (
    <div className={`relative flex justify-start`}>
      <button
        onClick={onView}
        className={`inline-flex items-center gap-1.5 ${isChild ? 'text-[0.6rem] sm:text-[0.7rem]' : 'text-[0.7rem] sm:text-[0.8rem]'} font-mono font-bold text-green-400 bg-gray-900/80 border border-green-700/60 rounded-md px-2 py-0.5 w-full text-left cursor-pointer pr-7`}
      >
        <span className="truncate">{truncateText(fullText, truncateLimit)}</span>
      </button>
      <button onClick={onCopy}
        className="absolute right-1 top-1/2 -translate-y-1/2 p-0.5 rounded text-green-400"
        title="Copy to clipboard">
        {copied
          ? <Check className={isChild ? 'w-3 h-3' : 'w-3.5 h-3.5'} />
          : <Copy className={isChild ? 'w-3 h-3' : 'w-3.5 h-3.5'} />
        }
      </button>
    </div>
  );
};

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

// ── Props ─────────────────────────────────────────────────────────────────────
interface WantCardContentProps {
  want: Want;
  isChild?: boolean;
  hasChildren?: boolean;
  isFocused?: boolean;
  isSelectMode?: boolean;
  onView: (want: Want) => void;
  onViewAgents?: (want: Want) => void;
  onViewResults?: (want: Want) => void;
  onViewChat?: (want: Want) => void;
  onEdit?: (want: Want) => void;
  onDelete?: (want: Want) => void;
  onSuspend?: (want: Want) => void;
  onResume?: (want: Want) => void;
  onShowReactionConfirmation?: (want: Want, action: 'approve' | 'deny') => void;
  onSliderActiveChange?: (active: boolean) => void;
}

// ── Main component ────────────────────────────────────────────────────────────
export const WantCardContent: React.FC<WantCardContentProps> = ({
  want,
  isChild = false,
  hasChildren = false,
  isFocused = false,
  isSelectMode = false,
  onView,
  onViewAgents,
  onViewResults,
  onViewChat,
  onEdit,
  onDelete,
  onSuspend,
  onResume,
  onShowReactionConfirmation,
  onSliderActiveChange,
}) => {
  const wantName = want.metadata?.name || want.metadata?.id || 'Unnamed Want';
  const wantType = want.metadata?.type || 'unknown';
  const labels = want.metadata?.labels || {};

  const { updateWant } = useWantStore();

  // Reaction support
  const queueId = want.state?.current?.reaction_queue_id as string | undefined;
  const requireReaction = want.spec?.params?.require_reaction !== false;
  const isReminder = wantType === 'reminder';
  const isGoal = wantType === 'goal';
  const reminderPhase = want.state?.current?.reminder_phase as string | undefined;
  const goalPhase = want.state?.current?.phase as string | undefined;
  const isAwaitingApproval =
    (isReminder && reminderPhase === 'reaching') ||
    (isGoal && goalPhase === 'awaiting_approval');
  const shouldShowReactionButtons = queueId && requireReaction && isAwaitingApproval;
  const proposedBreakdown = want.state?.current?.proposed_breakdown as any[] | undefined;
  const proposedResponse = want.state?.current?.proposed_response as string | undefined;

  const [isSubmittingReaction, setIsSubmittingReaction] = useState(false);
  const submitReaction = async (approved: boolean) => {
    if (!queueId || isSubmittingReaction) return;
    setIsSubmittingReaction(true);
    try {
      await fetch(`/api/v1/reactions/${queueId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          approved,
          comment: `User ${approved ? 'approved' : 'denied'} ${isGoal ? 'decomposition proposal' : 'reminder'}`,
        }),
      });
    } catch (error) {
      console.error('Error submitting reaction:', error);
    } finally {
      setIsSubmittingReaction(false);
    }
  };

  // Copy for final_result
  const [finalResultCopied, setFinalResultCopied] = useState(false);
  const handleCopyFinalResult = (e: React.MouseEvent) => {
    e.stopPropagation();
    const value = want.state?.final_result;
    const text = typeof value === 'string' ? value : JSON.stringify(value);
    const onCopied = () => {
      setFinalResultCopied(true);
      setTimeout(() => setFinalResultCopied(false), 1500);
    };
    if (navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(text).then(onCopied).catch(() => fallbackCopy(text, onCopied));
    } else {
      fallbackCopy(text, onCopied);
    }
  };
  const fallbackCopy = (text: string, onCopied: () => void) => {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.cssText = 'position:fixed;opacity:0';
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    try { document.execCommand('copy'); onCopied(); } catch (_) {}
    document.body.removeChild(ta);
  };

  const isRunning = want.status === 'reaching' || want.status === 'reaching_with_warning' || want.status === 'waiting_user_action';
  const isFailed = want.status === 'failed';
  const hasError = Boolean(isFailed && want.state?.current?.error);
  const hasScheduling = want.spec?.when && want.spec.when.length > 0;
  const isInteractive = want.state?.current?.interactive === true;
  const isControl = labels['user-control'] === 'true';
  const isFullScreen = labels['full-screen-display'] === 'true';

  // Responsive sizes
  const sizes = isChild ? {
    titleClass: 'text-[11px] sm:text-sm font-semibold',
    iconSize: 'h-2.5 w-2.5 sm:h-3 w-3',
    statusSize: 'xs' as const,
    agentDotSize: 'w-1 h-1 sm:w-1.5 h-1.5',
    errorIconSize: 'h-2.5 w-2.5',
    errorTextSize: 'text-[10px]',
    textTruncate: 20,
  } : {
    titleClass: 'text-[9px] sm:text-[13px] font-semibold',
    iconSize: 'h-2 w-2 sm:h-3 w-3',
    statusSize: 'xs' as const,
    agentDotSize: 'w-1.5 h-1.5 sm:w-2 h-2',
    errorIconSize: 'h-3.5 w-3.5 sm:h-4 w-4',
    errorTextSize: 'text-[11px] sm:text-sm',
    textTruncate: 25,
  };

  // Plugin lookup
  const plugin = getWantCardPlugin(wantType);

  return (
    <>
      <div className="flex flex-col h-full relative">

        {/* Status badge */}
        {!isSelectMode && !isFullScreen && (
          <div className="absolute top-3 right-3 z-20 pointer-events-none">
            <StatusBadge status={want.status} size="sm" />
          </div>
        )}

        {/* Reaction overlay */}
        {shouldShowReactionButtons && (
          <div className="absolute inset-x-0 top-0 z-[30] border-b border-white/10 dark:border-gray-800 shadow-lg animate-in slide-in-from-top duration-300">
            <div className="grid grid-cols-2 h-12 divide-x divide-white/10 dark:divide-gray-800">
              <button
                onClick={(e) => { e.stopPropagation(); submitReaction(false); }}
                disabled={isSubmittingReaction}
                className={classNames(
                  'flex items-center justify-center gap-2 transition-all duration-150',
                  isSubmittingReaction ? 'bg-gray-400/40 cursor-not-allowed opacity-60' : 'bg-red-600 hover:bg-red-700 active:opacity-90',
                )}>
                <ThumbsDown className="h-4 w-4 text-white" />
                <span className="text-xs font-bold uppercase tracking-wider text-white">Deny</span>
              </button>
              <button
                onClick={(e) => { e.stopPropagation(); submitReaction(true); }}
                disabled={isSubmittingReaction}
                className={classNames(
                  'flex items-center justify-center gap-2 transition-all duration-150',
                  isSubmittingReaction ? 'bg-gray-400/40 cursor-not-allowed opacity-60' : 'bg-green-600 hover:bg-green-700 active:opacity-90',
                )}>
                {isSubmittingReaction
                  ? <div className="h-4 w-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                  : <ThumbsUp className="h-4 w-4 text-white" />
                }
                <span className="text-xs font-bold uppercase tracking-wider text-white">Approve</span>
              </button>
            </div>
          </div>
        )}

        {/* Header container (fixed at top if not full-screen) */}
        {(!isFullScreen || isFocused) && (
            <div className={classNames(
                'absolute top-0 left-0 right-0 z-20',
                styles.controlCardHeader,
                isControl && !isFocused ? styles.controlCardHeaderHidden : styles.controlCardHeaderVisible,
            )}>
                <div className={`backdrop-blur-[2px] transition-colors duration-200 ${isFocused ? 'bg-blue-200/90 dark:bg-blue-900/70' : 'bg-white/60 dark:bg-gray-900/70'} ${isChild ? 'px-2 sm:px-4 py-1' : 'px-3 sm:px-6 py-1'}`}>
                <div className="flex items-center justify-between">
                    <div className="flex-1 min-w-0">
                    <h3 className={`${sizes.titleClass} text-gray-900 dark:text-gray-100 truncate group-hover:text-primary-600 dark:group-hover:text-primary-400 transition-colors flex items-center gap-1.5`}>
                        {labels['recipe-based'] === 'true'
                        ? hasChildren
                            ? <HeartInBottle className={`${isChild ? 'h-3 w-3 sm:h-3.5 sm:w-3.5' : 'h-2.5 w-2.5 sm:h-3.5 sm:w-3.5'} flex-shrink-0 text-pink-500`} />
                            : <BottleOnly className={sizes.iconSize} />
                        : <Heart className={`${sizes.iconSize} flex-shrink-0 text-pink-500`} />
                        }
                        {wantType}
                    </h3>
                    </div>
                    <div className="flex items-center space-x-1 sm:space-x-2 ml-1 sm:ml-2">
                    {isInteractive && (
                        <button
                        onClick={(e) => { e.stopPropagation(); if (!isSelectMode && onViewChat) onViewChat(want); }}
                        className={classNames('flex items-center p-1 rounded-md transition-colors',
                            isSelectMode ? 'cursor-default' : 'hover:bg-blue-50 dark:hover:bg-blue-900/30 cursor-pointer')}
                        title="Click to chat with agent">
                        <MessageSquare className={`${sizes.iconSize} text-blue-600 dark:text-blue-400`} />
                        </button>
                    )}
                    {(want.current_agent || (want.running_agents && want.running_agents.length > 0) || (want.history?.agentHistory && want.history.agentHistory.length > 0)) && (
                        <button
                        onClick={(e) => { e.stopPropagation(); if (!isSelectMode && onViewAgents) onViewAgents(want); }}
                        className={classNames('flex items-center space-x-1 p-1 rounded-md transition-colors',
                            isSelectMode ? 'cursor-default' : 'hover:bg-blue-50 dark:hover:bg-blue-900/30 cursor-pointer')}
                        title="Click to view agent details">
                        <Bot className={`${sizes.iconSize} text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300`} />
                        {want.current_agent && (
                            <div className={`${sizes.agentDotSize} bg-green-500 rounded-full ${styles.pulseGlow}`} title="Agent running" />
                        )}
                        {want.history?.agentHistory && want.history.agentHistory.length > 0 && (
                            <span className={classNames(`${sizes.agentDotSize} rounded-full`,
                            want.history.agentHistory[want.history.agentHistory.length - 1]?.status === 'achieved' && 'bg-green-500',
                            want.history.agentHistory[want.history.agentHistory.length - 1]?.status === 'failed' && 'bg-red-500',
                            want.history.agentHistory[want.history.agentHistory.length - 1]?.status === 'running' && `bg-blue-500 ${styles.pulseGlow}`,
                            )} title={`Latest agent: ${want.history.agentHistory[want.history.agentHistory.length - 1]?.status || 'unknown'}`} />
                        )}
                        </button>
                    )}
                    {hasScheduling && (
                        <button
                        onClick={(e) => { e.stopPropagation(); if (!isSelectMode) onView(want); }}
                        className={classNames(
                            'inline-flex items-center gap-1 font-medium rounded-full border hover:opacity-80 transition-opacity px-1.5 py-0.5 text-xs',
                            'bg-amber-100 text-amber-800 border-amber-200 dark:bg-amber-900/30 dark:text-amber-200 dark:border-amber-700',
                        )}
                        title="Click to view scheduling settings">
                        <Clock className={sizes.iconSize} />
                        </button>
                    )}
                    </div>
                </div>
                </div>
            </div>
        )}

        {/* Content container (fills space, respects header) */}
        <div className={classNames('flex-1 relative flex flex-col', !isFullScreen && 'pt-10')}>
          
          {/* Error indicator */}
          {hasError && (!isControl || isFocused) && (
            <div className="mt-4 p-3 bg-red-100 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md">
                ...Error handling code...
            </div>
          )}

          {/* ── Plugin content section ── */}
          {plugin && (
            <plugin.ContentSection
              want={want}
              isChild={isChild}
              isControl={isControl}
              isFocused={isFocused}
              isSelectMode={isSelectMode}
              onView={onView}
              onViewResults={onViewResults}
              onSliderActiveChange={onSliderActiveChange}
            />
          )}

          {/* Final result display */}
          {want.state?.final_result != null && want.state?.final_result !== '' && !isFullScreen && (
            <FinalResultDisplay
              value={want.state!.final_result}
              isChild={isChild}
              copied={finalResultCopied}
              onCopy={handleCopyFinalResult}
              onView={() => onViewResults ? onViewResults(want) : onView(want)}
            />
          )}
        </div>
      </div>
    </>
  );
};
