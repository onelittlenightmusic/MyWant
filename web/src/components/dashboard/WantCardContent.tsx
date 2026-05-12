import React, { useState, useEffect, useReducer, useCallback } from 'react';
import { AlertTriangle, Bot, Heart, Clock, ThumbsUp, ThumbsDown, Copy, Check, MessageSquare } from 'lucide-react';
import { Want } from '@/types/want';
import { StatusBadge } from '@/components/common/StatusBadge';
import { ArrayResultTable } from '@/components/common/ArrayResultTable';
import { ObjectResultDisplay, isPlainObject } from '@/components/common/ObjectResultDisplay';
import { truncateText, classNames } from '@/utils/helpers';
import { useWantStore } from '@/stores/wantStore';
import { useConfigStore } from '@/stores/configStore';
import styles from './WantCard.module.css';

// Register all plugins (side-effect imports)
import './WantCard/plugins';
import { getWantCardPlugin, onPluginRegistered } from './WantCard/plugins/registry';

// ── FinalResultDisplay ────────────────────────────────────────────────────────
const FinalResultDisplay: React.FC<{
  value: unknown;
  isChild: boolean;
  copied: boolean;
  onCopy: (e: React.MouseEvent) => void;
  onView: () => void;
}> = ({ value, isChild, copied, onCopy, onView }) => {
  let parsedValue = value;
  if (typeof value === 'string') {
    try {
      const parsed = JSON.parse(value);
      if (typeof parsed === 'object' && parsed !== null) {
        parsedValue = parsed;
      }
    } catch (e) {
      // Not a JSON string, keep as is
    }
  }

  const isArrayOfObjects =
    Array.isArray(parsedValue) &&
    parsedValue.length > 0 &&
    typeof parsedValue[0] === 'object' &&
    parsedValue[0] !== null;

  const fullText = typeof parsedValue === 'string' ? parsedValue : JSON.stringify(parsedValue, null, 2);
  const truncateLimit = isChild ? 40 : 50;
  const iconSize = isChild ? 'w-3 h-3' : 'w-3.5 h-3.5';
  const labelClass = `${isChild ? 'text-[0.5rem]' : 'text-[0.55rem] sm:text-[0.6rem]'} font-mono text-green-400/70 hover:text-green-400 cursor-pointer`;

  if (isArrayOfObjects) {
    const data = parsedValue as Record<string, unknown>[];
    return (
      <div className="flex flex-col min-h-0 overflow-hidden">
        <div className="flex items-center justify-between mb-0.5">
          <button onClick={onView} className={labelClass}>
            [{data.length} items] — view all
          </button>
          <button onClick={onCopy} className="p-0.5 rounded text-green-400" title="Copy to clipboard">
            {copied ? <Check className={iconSize} /> : <Copy className={iconSize} />}
          </button>
        </div>
        <ArrayResultTable data={data} maxRows={isChild ? 3 : 5} size="compact" />
      </div>
    );
  }

  if (isPlainObject(parsedValue)) {
    const data = parsedValue as Record<string, unknown>;
    const fieldCount = Object.keys(data).length;
    return (
      <div className="flex flex-col min-h-0 overflow-hidden">
        <div className="flex items-center justify-between mb-0.5">
          <button onClick={onView} className={labelClass}>
            {'{'}&#8203;{fieldCount} fields{'}'} — view all
          </button>
          <button onClick={onCopy} className="p-0.5 rounded text-green-400" title="Copy to clipboard">
            {copied ? <Check className={iconSize} /> : <Copy className={iconSize} />}
          </button>
        </div>
        <ObjectResultDisplay data={data} maxRows={isChild ? 3 : 5} size="compact" />
      </div>
    );
  }

  return (
    <div className="relative flex justify-start">
      <button
        onClick={onView}
        className={`inline-flex items-center gap-1.5 ${isChild ? 'text-[0.6rem] sm:text-[0.7rem]' : 'text-[0.7rem] sm:text-[0.8rem]'} font-mono font-bold text-green-400 bg-gray-900/80 border border-green-700/60 rounded-md px-2 py-0.5 w-full text-left cursor-pointer pr-7`}
      >
        <span className="truncate">{truncateText(fullText, truncateLimit)}</span>
      </button>
      <button onClick={onCopy}
        className="absolute right-1 top-1/2 -translate-y-1/2 p-0.5 rounded text-green-400"
        title="Copy to clipboard">
        {copied ? <Check className={iconSize} /> : <Copy className={iconSize} />}
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
  isInnerFocused?: boolean;
  onExitInnerFocus?: () => void;
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
  isInnerFocused = false,
  onExitInnerFocus,
}) => {
  const wantType = want.metadata?.type || 'unknown';
  const labels = want.metadata?.labels || {};
  const config = useConfigStore(state => state.config);
  const [, forceUpdate] = useReducer(x => x + 1, 0);
  useEffect(() => onPluginRegistered(forceUpdate), []);

  const queueId = want.state?.current?.reaction_queue_id as string | undefined;
  const requireReaction = want.spec?.params?.require_reaction !== false;
  const isReminder = wantType === 'reminder';
  const isGoal = wantType === 'goal';
  const status = want.status;
  const isAwaitingApproval = (isReminder && (status as any) === 'waiting_user_action') || (isGoal && (status as any) === 'awaiting_approval');
  const shouldShowReactionButtons = queueId && requireReaction && isAwaitingApproval;
  console.log('DEBUG [WantCardContent]:', { queueId, requireReaction, isReminder, status, isAwaitingApproval, shouldShowReactionButtons, want });

  const [isSubmittingReaction, setIsSubmittingReaction] = useState(false);
  const submitReaction = useCallback(async (approved: boolean) => {
    const currentQueueId = want.state?.current?.reaction_queue_id as string | undefined;
    if (!currentQueueId || isSubmittingReaction) return;
    
    setIsSubmittingReaction(true);
    try {
      await fetch(`/api/v1/reactions/${currentQueueId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ approved, comment: 'Reaction submitted' }),
      });
    } catch (error) { console.error('Error submitting reaction:', error); } finally { setIsSubmittingReaction(false); }
  }, [want.state?.current?.reaction_queue_id, isSubmittingReaction]);

  const [finalResultCopied, setFinalResultCopied] = useState(false);
  const handleCopyFinalResult = (e: React.MouseEvent) => {
    e.stopPropagation();
    const value = want.state?.final_result;
    const text = typeof value === 'string' ? value : JSON.stringify(value);
    navigator.clipboard?.writeText(text).then(() => {
        setFinalResultCopied(true);
        setTimeout(() => setFinalResultCopied(false), 1500);
    });
  };

  const isFailed = want.status === 'failed';
  const hasError = Boolean(isFailed && want.state?.current?.error);
  const hasScheduling = want.spec?.when && want.spec.when.length > 0;
  const isInteractive = want.state?.current?.interactive === true;
  const isControl = labels['user-control'] === 'true';
  const isFullScreen = labels['full-screen-display'] === 'true';
  const isHeaderBottom = config?.header_position === 'bottom';

  const sizes = isChild ? {
    titleClass: 'text-[11px] sm:text-sm font-semibold',
    iconSize: 'h-2.5 w-2.5 sm:h-3 w-3',
    agentDotSize: 'w-1 h-1 sm:w-1.5 h-1.5',
  } : {
    titleClass: 'text-[9px] sm:text-[13px] font-semibold',
    iconSize: 'h-2 w-2 sm:h-3 w-3',
    agentDotSize: 'w-1.5 h-1.5 sm:w-2 h-2',
  };

  const plugin = getWantCardPlugin(wantType);

  return (
    <div className={classNames(
      "flex h-full w-full relative overflow-hidden",
      isHeaderBottom ? "flex-col-reverse" : "flex-col"
    )}>
      {/* ── Header ── */}
      {(!isFullScreen || isFocused) && (
        <div className={classNames(
          'flex-shrink-0 z-20 w-full',
          styles.controlCardHeader,
          isControl && !isFocused ? styles.controlCardHeaderHidden : styles.controlCardHeaderVisible,
        )}>
          <div className={`backdrop-blur-[2px] transition-colors duration-200 ${isFocused ? 'bg-blue-200/90 dark:bg-blue-900/70' : 'bg-white/60 dark:bg-gray-900/70'} ${isChild ? 'px-2 sm:px-4 py-1' : 'px-3 sm:px-6 py-1'}`}>
            <div className="flex items-center justify-between">
              <div className="flex-1 min-w-0">
                <h3 className={`${sizes.titleClass} text-gray-900 dark:text-gray-100 truncate group-hover:text-primary-600 dark:group-hover:text-primary-400 transition-colors flex items-center gap-1.5`}>
                  {labels['recipe-based'] === 'true'
                    ? hasChildren ? <HeartInBottle className={`${isChild ? 'h-3 w-3 sm:h-3.5 sm:w-3.5' : 'h-2.5 w-2.5 sm:h-3.5 sm:w-3.5'} flex-shrink-0 text-pink-500`} /> : <BottleOnly className={sizes.iconSize} />
                    : <Heart className={`${sizes.iconSize} flex-shrink-0 text-pink-500`} />
                  }
                  {wantType}
                </h3>
              </div>
              <div className="flex items-center space-x-1 sm:space-x-2 ml-1 sm:ml-2">
                {isInteractive && <MessageSquare className={`${sizes.iconSize} text-blue-600 dark:text-blue-400`} />}
                {(want.current_agent || (want.running_agents && want.running_agents.length > 0)) && (
                   <div className="flex items-center">
                     <Bot className={`${sizes.iconSize} text-blue-600 dark:text-blue-400`} />
                     <div className={`${sizes.agentDotSize} bg-green-500 rounded-full animate-pulse ml-0.5`} />
                   </div>
                )}
                {hasScheduling && <Clock className={`${sizes.iconSize} text-amber-600 dark:text-amber-400`} />}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ── Main Content Container (fills all space, centers content) ── */}
      <div className="flex-1 relative flex flex-col justify-center min-h-0">
        
        {/* Status badge: absolutely positioned over the content area */}
        {!isSelectMode && !isFullScreen && (
          <div className="absolute top-3 right-3 z-10 pointer-events-none">
            <StatusBadge status={want.status} size="sm" />
          </div>
        )}

        {/* Reaction overlay */}
        {shouldShowReactionButtons && (
          <div className="absolute inset-x-0 top-0 z-[30] border-b border-white/10 dark:border-gray-800 shadow-lg animate-in slide-in-from-top duration-300">
            <div className="grid grid-cols-2 h-12 divide-x divide-white/10 dark:divide-gray-800">
              <button onClick={(e) => { e.stopPropagation(); submitReaction(false); }} disabled={isSubmittingReaction} className="flex items-center justify-center gap-2 bg-red-600 hover:bg-red-700 text-white transition-colors h-12">
                <ThumbsDown className="h-4 w-4" />
                <span className="text-xs font-bold uppercase">Deny</span>
              </button>
              <button onClick={(e) => { e.stopPropagation(); submitReaction(true); }} disabled={isSubmittingReaction} className="flex items-center justify-center gap-2 bg-green-600 hover:bg-green-700 text-white transition-colors h-12">
                {isSubmittingReaction ? <div className="h-4 w-4 border-2 border-white/30 border-t-white rounded-full animate-spin" /> : <ThumbsUp className="h-4 w-4" />}
                <span className="text-xs font-bold uppercase">Approve</span>
              </button>
            </div>
          </div>
        )}

        <div className="flex flex-col flex-1 overflow-hidden min-h-0">
          {/* Error display */}
          {hasError && (!isControl || isFocused) && (
            <div className="p-3 bg-red-50 dark:bg-red-900/20 border-b border-red-100 dark:border-red-800 flex items-start gap-2">
              <AlertTriangle className="h-4 w-4 text-red-600 flex-shrink-0 mt-0.5" />
              <p className="text-[10px] sm:text-xs text-red-700 dark:text-red-300 leading-tight">
                {truncateText(String(want.state?.current?.error), 100)}
              </p>
            </div>
          )}

          {/* Plugin content section */}
          {plugin && (
            <div className="flex-1 overflow-hidden min-h-0 flex flex-col justify-center">
              <plugin.ContentSection
                want={want}
                isChild={isChild}
                isControl={isControl}
                isFocused={isFocused}
                isSelectMode={isSelectMode}
                onView={onView}
                onViewResults={onViewResults}
                onSliderActiveChange={onSliderActiveChange}
                isInnerFocused={isInnerFocused}
                onExitInnerFocus={onExitInnerFocus}
              />
            </div>
          )}

          {/* Final result text area */}
          {want.state?.final_result != null && want.state?.final_result !== '' && !isFullScreen && !plugin?.hideFinalResult && (
            <div className="flex-shrink-0 p-2 pt-0">
              <FinalResultDisplay
                value={want.state!.final_result}
                isChild={isChild}
                copied={finalResultCopied}
                onCopy={handleCopyFinalResult}
                onView={() => onViewResults ? onViewResults(want) : onView(want)}
              />
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
