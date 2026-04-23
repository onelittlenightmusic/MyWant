import React, { useState, useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { AlertTriangle, Bot, Heart, Pause, Clock, ThumbsUp, ThumbsDown, Trash2, Circle, X, Camera, Copy, Check, MessageSquare, Settings } from 'lucide-react';
import { Want } from '@/types/want';
import { StatusBadge } from '@/components/common/StatusBadge';
import { BrowserFrame } from '@/components/replay/BrowserFrame';
import { formatDate, formatDuration, truncateText, classNames } from '@/utils/helpers';
import { useWantStore } from '@/stores/wantStore';
import styles from './WantCard.module.css';

// ── FinalResultDisplay ────────────────────────────────────────────────────────
// Shows the final_result value truncated. Clicking opens the detail view.
const FinalResultDisplay: React.FC<{
  value: unknown;
  isChild: boolean;
  copied: boolean;
  onCopy: (e: React.MouseEvent) => void;
  onView: () => void;
}> = ({ value, isChild, copied, onCopy, onView }) => {
  const fullText = typeof value === 'string' ? value : JSON.stringify(value, null, 2);
  const truncateLimit = isChild ? 40 : 50;

  return (
    <div className={`${isChild ? "mt-4" : "mt-8"} relative flex justify-start`}>
      <button
        onClick={onView}
        className={`inline-flex items-center gap-1.5 ${isChild ? 'text-[0.6rem] sm:text-[0.7rem]' : 'text-[0.7rem] sm:text-[0.8rem]'} font-mono font-bold text-green-400 bg-gray-900/80 border border-green-700/60 rounded-md px-2 py-0.5 w-full text-left cursor-pointer pr-7`}
      >
        <span className="truncate">{truncateText(fullText, truncateLimit)}</span>
      </button>
      <button
        onClick={onCopy}
        className="absolute right-1 top-1/2 -translate-y-1/2 p-0.5 rounded text-green-400"
        title="Copy to clipboard"
      >
        {copied
          ? <Check className={isChild ? "w-3 h-3" : "w-3.5 h-3.5"} />
          : <Copy className={isChild ? "w-3 h-3" : "w-3.5 h-3.5"} />
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
  onSliderActiveChange
}) => {
  const wantName = want.metadata?.name || want.metadata?.id || 'Unnamed Want';
  const wantType = want.metadata?.type || 'unknown';
  const labels = want.metadata?.labels || {};

  // Inline name editing (item 2)
  const [isEditingName, setIsEditingName] = useState(false);
  const [editedName, setEditedName] = useState(wantName);
  const nameInputRef = useRef<HTMLInputElement>(null);
  const { updateWant } = useWantStore();
  const createdAt = want.stats?.created_at;
  const startedAt = want.stats?.started_at;
  const completedAt = want.stats?.completed_at;

  // Interaction check
  const isInteractive = want.state?.current?.interactive === true;

  // Reaction support (Reminders, GoalThinker, etc.)
  const queueId = want.state?.current?.reaction_queue_id as string | undefined;
  const requireReaction = want.spec?.params?.require_reaction !== false; // Default to true
  const isReminder = want.metadata?.type === 'reminder';
  const isGoal = want.metadata?.type === 'goal';
  const reminderPhase = want.state?.current?.reminder_phase as string | undefined;
  const goalPhase = want.state?.current?.phase as string | undefined;

  const isAwaitingApproval =
    (isReminder && reminderPhase === 'reaching') ||
    (isGoal && goalPhase === 'awaiting_approval');

  const shouldShowReactionButtons = queueId && requireReaction && isAwaitingApproval;

  const proposedBreakdown = want.state?.current?.proposed_breakdown as any[] | undefined;
  const proposedResponse = want.state?.current?.proposed_response as string | undefined;

  // Slider-specific state
  const isSlider = wantType === 'slider';
  const sliderValue = typeof want.state?.current?.value === 'number' ? want.state.current.value : 0;
  const sliderMin = typeof want.state?.current?.min === 'number' ? want.state.current.min : 0;
  const sliderMax = typeof want.state?.current?.max === 'number' ? want.state.current.max : 100;
  const sliderStep = typeof want.state?.current?.step === 'number' ? want.state.current.step : 1;
  const sliderTargetParam = (want.state?.current?.target_param as string) || '';
  const [localSliderValue, setLocalSliderValue] = useState(sliderValue);
  const sliderDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Choice-specific state
  const isChoice = wantType === 'choice';
  const choiceSelected = want.state?.current?.selected;
  const choices = Array.isArray(want.state?.current?.choices) ? want.state.current.choices : [];
  const choiceTargetParam = (want.state?.current?.target_param as string) || '';
  const [localChoiceValue, setLocalChoiceValue] = useState(choiceSelected);

  useEffect(() => {
    setLocalChoiceValue(choiceSelected);
  }, [choiceSelected]);

  const handleChoiceChange = async (newValue: any) => {
    setLocalChoiceValue(newValue);
    const id = want.metadata?.id;
    if (!id) return;
    try {
      await fetch(`/api/v1/states/${id}/selected`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(newValue),
      });
    } catch (err) {
      console.error('[WantCard] choice state update failed:', err);
    }
  };

  // Timer-specific state (global WhenSpec parameter control)
  const isTimer = wantType === 'timer';
  const timerEvery = (want.state?.current?.every as string) || '';
  const timerAt = (want.state?.current?.at as string) || '';
  const timerTargetParam = (want.state?.current?.target_param as string) || '';
  const [localTimerEvery, setLocalTimerEvery] = useState(timerEvery);
  const [localTimerAt, setLocalTimerAt] = useState(timerAt);
  const timerEveryDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const timerAtDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => { setLocalTimerEvery(timerEvery); }, [timerEvery]);
  useEffect(() => { setLocalTimerAt(timerAt); }, [timerAt]);

  const handleTimerEveryChange = (value: string) => {
    setLocalTimerEvery(value);
    if (timerEveryDebounceRef.current) clearTimeout(timerEveryDebounceRef.current);
    timerEveryDebounceRef.current = setTimeout(async () => {
      const id = want.metadata?.id;
      if (!id) return;
      try {
        await fetch(`/api/v1/states/${id}/every`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(value),
        });
      } catch (err) {
        console.error('[WantCard] timer every update failed:', err);
      }
    }, 400);
  };

  const handleTimerAtChange = (value: string) => {
    setLocalTimerAt(value);
    if (timerAtDebounceRef.current) clearTimeout(timerAtDebounceRef.current);
    timerAtDebounceRef.current = setTimeout(async () => {
      const id = want.metadata?.id;
      if (!id) return;
      try {
        await fetch(`/api/v1/states/${id}/at`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(value),
        });
      } catch (err) {
        console.error('[WantCard] timer at update failed:', err);
      }
    }, 400);
  };

  const TIMER_EVERY_PRESETS = ['1m', '5m', '10m', '30m', '1h', '6h', '1d'];

  useEffect(() => {
    setLocalSliderValue(sliderValue);
  }, [sliderValue]);

  const handleSliderChange = (newValue: number) => {
    setLocalSliderValue(newValue);
    if (sliderDebounceRef.current) clearTimeout(sliderDebounceRef.current);
    sliderDebounceRef.current = setTimeout(async () => {
      const id = want.metadata?.id;
      if (!id) return;
      try {
        await fetch(`/api/v1/states/${id}/value`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(newValue),
        });
      } catch (err) {
        console.error('[WantCard] slider state update failed:', err);
      }
    }, 150);
  };

  // Replay-specific state
  const isReplay = wantType === 'replay';
  const recordingActive = want.state?.current?.recording_active === true;
  const debugRecordingActive = want.state?.current?.debug_recording_active === true;
  const replayActive = want.state?.current?.replay_active === true;
  const iframeUrl = want.state?.current?.recording_iframe_url as string | undefined;
  const replayIframeUrl = want.state?.current?.replay_iframe_url as string | undefined;
  const hasFinalResult = want.state?.final_result != null && want.state?.final_result !== '';
  const hasReplayActions = Boolean(want.state?.current?.replay_actions && (want.state?.current?.replay_actions as string) !== '[]');
  const debugRecordingError = want.state?.current?.debug_recording_error as string | undefined;
  const replayError = want.state?.current?.replay_error as string | undefined;
  const replayResultRaw = want.state?.current?.replay_result as string | undefined;
  const replayResult = (() => {
    if (!replayResultRaw) return null;
    try { return JSON.parse(replayResultRaw); } catch { return null; }
  })();
  const replayScreenshotUrl = want.state?.current?.replay_screenshot_url as string | undefined;


  // Webhook IDs: prefer state value (set by MonitorAgent), fall back to predictable pattern from want ID.
  // This ensures the Record button appears immediately after want creation, before the MonitorAgent runs.
  const wantId = want.metadata?.id ?? '';
  const startWebhookId = (want.state?.current?.startWebhookId as string | undefined) || (wantId ? `${wantId}-start` : undefined);
  const stopWebhookId = (want.state?.current?.stopWebhookId as string | undefined) || (wantId ? `${wantId}-stop` : undefined);
  const debugStartWebhookId = (want.state?.current?.debugStartWebhookId as string | undefined) || (wantId ? `${wantId}-debug-start` : undefined);
  const debugStopWebhookId = (want.state?.current?.debugStopWebhookId as string | undefined) || (wantId ? `${wantId}-debug-stop` : undefined);
  const replayWebhookId = (want.state?.current?.replayWebhookId as string | undefined) || (wantId ? `${wantId}-replay` : undefined);

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

  // Reaction submission state
  const [isSubmittingReaction, setIsSubmittingReaction] = useState(false);

  // Copy state for final_result
  const [finalResultCopied, setFinalResultCopied] = useState(false);
  const handleCopyFinalResult = (e: React.MouseEvent) => {
    e.stopPropagation();
    const value = want.state?.final_result;
    const text = typeof value === 'string' ? value : JSON.stringify(value);
    const onCopied = () => {
      setFinalResultCopied(true);
      setTimeout(() => setFinalResultCopied(false), 1500);
    };
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(onCopied).catch(() => {
        // fallback for iOS/older browsers
        const ta = document.createElement('textarea');
        ta.value = text;
        ta.style.position = 'fixed';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.focus();
        ta.select();
        try { document.execCommand('copy'); onCopied(); } catch (_) {}
        document.body.removeChild(ta);
      });
    } else {
      // fallback for iOS/older browsers
      const ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.focus();
      ta.select();
      try { document.execCommand('copy'); onCopied(); } catch (_) {}
      document.body.removeChild(ta);
    }
  };

  const isRunning = want.status === 'reaching' || want.status === 'reaching_with_warning' || want.status === 'waiting_user_action';
  const isFailed = want.status === 'failed';
  const hasError = Boolean(isFailed && want.state?.current?.error);
  const isSuspended = want.status === 'suspended';
  const canControl = want.status === 'reaching' || want.status === 'reaching_with_warning' || want.status === 'waiting_user_action' || want.status === 'stopped';
  const canSuspendResume = isRunning && (onSuspend || onResume);
  const hasScheduling = (want.spec?.when && want.spec.when.length > 0);

  // Submit reaction directly without confirmation
  const submitReaction = async (approved: boolean) => {
    if (!queueId || isSubmittingReaction) return;
    setIsSubmittingReaction(true);
    try {
      const typeLabel = isGoal ? 'decomposition proposal' : 'reminder';
      await fetch(`/api/v1/reactions/${queueId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          approved,
          comment: `User ${approved ? 'approved' : 'denied'} ${typeLabel}`,
        }),
      });
    } catch (error) {
      console.error('Error submitting reaction:', error);
    } finally {
      setIsSubmittingReaction(false);
    }
  };

  const handleApproveClick = () => submitReaction(true);
  const handleDenyClick = () => submitReaction(false);

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
    titleClass: 'text-[9px] sm:text-[13px] font-semibold',
    typeClass: 'text-[10px] sm:text-sm',
    idClass: 'text-[10px] sm:text-xs',
    iconSize: 'h-2 w-2 sm:h-3 w-3',
    statusSize: 'xs' as const,
    agentDotSize: 'w-1.5 h-1.5 sm:w-2 h-2',
    errorIconSize: 'h-3.5 w-3.5 sm:h-4 w-4',
    errorTextSize: 'text-[11px] sm:text-sm',
    textTruncate: 25
  };

  const isControl = labels['user-control'] === 'true';

  return (
    <>
      <div className="flex flex-col h-full relative">
        {/* Status indicator - absolute top right */}
        {!isSelectMode && (
          <div className="absolute top-3 right-3 z-20 pointer-events-none">
            <StatusBadge status={want.status} size="sm" />
          </div>
        )}

        {/* Top-level Reaction Overlay - Full width flat grid style */}
        {shouldShowReactionButtons && (
          <div className="absolute inset-x-0 top-0 z-[30] border-b border-white/10 dark:border-gray-800 shadow-lg animate-in slide-in-from-top duration-300">
             <div className="grid grid-cols-2 h-12 divide-x divide-white/10 dark:divide-gray-800">
              <button
                onClick={(e) => { e.stopPropagation(); handleDenyClick(); }}
                disabled={isSubmittingReaction}
                className={classNames(
                  'flex items-center justify-center gap-2 transition-all duration-150',
                  isSubmittingReaction
                    ? 'bg-gray-400/40 cursor-not-allowed opacity-60'
                    : 'bg-red-600 hover:bg-red-700 active:opacity-90'
                )}
              >
                <ThumbsDown className="h-4 w-4 text-white" />
                <span className="text-xs font-bold uppercase tracking-wider text-white">Deny</span>
              </button>
              <button
                onClick={(e) => { e.stopPropagation(); handleApproveClick(); }}
                disabled={isSubmittingReaction}
                className={classNames(
                  'flex items-center justify-center gap-2 transition-all duration-150',
                  isSubmittingReaction
                    ? 'bg-gray-400/40 cursor-not-allowed opacity-60'
                    : 'bg-green-600 hover:bg-green-700 active:opacity-90'
                )}
              >
                {isSubmittingReaction ? (
                   <div className="h-4 w-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                ) : (
                  <ThumbsUp className="h-4 w-4 text-white" />
                )}
                <span className="text-xs font-bold uppercase tracking-wider text-white">Approve</span>
              </button>
            </div>
          </div>
        )}

      <div className={classNames(
        "order-2 mt-auto",
        styles.controlCardHeader,
        isControl && !isFocused ? styles.controlCardHeaderHidden : styles.controlCardHeaderVisible
      )}>
        <div className={`backdrop-blur-[2px] transition-colors duration-200 ${isFocused ? 'bg-blue-200/90 dark:bg-blue-900/70' : 'bg-white/60 dark:bg-gray-900/70'} ${isChild ? 'px-2 sm:px-4 py-1' : 'px-3 sm:px-6 py-1'}`}>
          <div className="flex items-center justify-between">
          <div className="flex-1 min-w-0">
            <h3
              className={`${sizes.titleClass} text-gray-900 dark:text-gray-100 truncate group-hover:text-primary-600 dark:group-hover:text-primary-400 transition-colors flex items-center gap-1.5`}
            >
              {labels['recipe-based'] === 'true' ? (
                hasChildren ? (
                  <HeartInBottle className={`${isChild ? 'h-3 w-3 sm:h-3.5 sm:w-3.5' : 'h-2.5 w-2.5 sm:h-3.5 sm:w-3.5'} flex-shrink-0 text-pink-500`} />
                ) : (
                  <BottleOnly className={sizes.iconSize} />
                )
              ) : (
                <Heart className={`${sizes.iconSize} flex-shrink-0 text-pink-500`} />
              )}
              {wantType}
            </h3>
          </div>

          <div className="flex items-center space-x-1 sm:space-x-2 ml-1 sm:ml-2">
            {/* Chat indicator - clickable */}
            {isInteractive && (
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  if (isSelectMode) return;
                  if (onViewChat) {
                    onViewChat(want);
                  }
                }}
                className={classNames(
                  "flex items-center p-1 rounded-md transition-colors",
                  isSelectMode ? "cursor-default" : "hover:bg-blue-50 dark:hover:bg-blue-900/30 cursor-pointer"
                )}
                title="Click to chat with agent"
              >
                <MessageSquare className={`${sizes.iconSize} text-blue-600 dark:text-blue-400`} />
              </button>
            )}

            {/* Agent indicator - clickable */}
            {(want.current_agent || (want.running_agents && want.running_agents.length > 0) || (want.history?.agentHistory && want.history.agentHistory.length > 0)) && (
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  if (isSelectMode) return;
                  if (onViewAgents) {
                    onViewAgents(want);
                  }
                }}
                className={classNames(
                  "flex items-center space-x-1 p-1 rounded-md transition-colors",
                  isSelectMode ? "cursor-default" : "hover:bg-blue-50 dark:hover:bg-blue-900/30 cursor-pointer"
                )}
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
                    want.history.agentHistory[want.history.agentHistory.length - 1]?.status === 'running' && `bg-blue-500 ${styles.pulseGlow}`
                  )} title={`Latest agent: ${want.history.agentHistory[want.history.agentHistory.length - 1]?.status || 'unknown'}`} />
                )}
              </button>
            )}

            {/* Scheduling indicator */}
            {hasScheduling && (
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  if (isSelectMode) return;
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
          </div>
        </div>
      </div>
    </div>

      <div className={isChild ? "px-2 sm:px-4 pb-2 pt-2 order-1" : "px-3 sm:px-6 pb-3 pt-3 order-1"}>


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
      {hasError && (!isControl || isFocused) && (
        <div className="mt-4 p-3 bg-red-100 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md">
          <div className="flex items-start">
            <AlertTriangle className={`${sizes.errorIconSize} text-red-600 dark:text-red-400 mt-0.5 mr-2 flex-shrink-0`} />
            <div className="flex-1 min-w-0">
              <p className={`${sizes.errorTextSize} font-medium text-red-800 dark:text-red-200`}>Execution Failed</p>
              <p className={`${sizes.errorTextSize} text-red-600 dark:text-red-400 mt-1 truncate`}>
                {truncateText(typeof want.state?.current?.error === 'string' ? want.state.current.error : 'Unknown error', isChild ? 60 : 100)}
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
      {want.results && Object.keys(want.results).length > 0 && (!isControl || isFocused) && (
        <div className="mt-4 pt-4 border-t border-gray-200 dark:border-gray-700">
          <p className={`${sizes.errorTextSize} text-gray-600 dark:text-gray-400`}>
            Results: {Object.keys(want.results).length} item{Object.keys(want.results).length !== 1 ? 's' : ''}
          </p>
        </div>
      )}

      {/* Slider type: range slider to control parent parameter */}
      {isSlider && (
       <div
         className={`${(isChild || (isControl && !isFocused)) ? "mt-2" : "mt-4"} space-y-1`}
         onPointerEnter={() => onSliderActiveChange?.(true)}
         onPointerLeave={() => onSliderActiveChange?.(false)}
         onMouseDown={(e) => e.stopPropagation()}
         onTouchStart={(e) => e.stopPropagation()}
         onTouchMove={(e) => e.stopPropagation()}
       >
         <div className="flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
           <span className="font-medium truncate mr-2" title={sliderTargetParam}>
             {sliderTargetParam || 'value'}
           </span>
           <span className="font-mono tabular-nums text-sm font-semibold text-gray-800 dark:text-gray-200">
             {localSliderValue}
           </span>
         </div>
         <input
           type="range"
           min={sliderMin}
           max={sliderMax}
           step={sliderStep}
           value={localSliderValue}
           onChange={(e) => handleSliderChange(Number(e.target.value))}
           onClick={(e) => e.stopPropagation()}
           className="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded-lg appearance-none cursor-pointer accent-blue-500"
         />
         <div className="flex justify-between text-[10px] text-gray-400 dark:text-gray-500">
           <span>{sliderMin}</span>
           <span>{sliderMax}</span>
         </div>
       </div>
      )}

      {isChoice && (
       <div className={`${(isChild || (isControl && !isFocused)) ? "mt-2" : "mt-4"} space-y-1`}>
         <div className="flex items-center justify-between text-[9px] text-gray-500 dark:text-gray-400 mb-1">
           <span className="font-medium truncate mr-2" title={choiceTargetParam}>
             {choiceTargetParam || 'Selection'}
           </span>
         </div>
         <select
           value={localChoiceValue === undefined || localChoiceValue === null ? "" : (typeof localChoiceValue === 'object' ? JSON.stringify(localChoiceValue) : String(localChoiceValue))}
           onChange={(e) => {
             const val = e.target.value;
             try {
               handleChoiceChange(JSON.parse(val));
             } catch {
               handleChoiceChange(val);
             }
           }}
           onClick={(e) => e.stopPropagation()}
           onMouseDown={(e) => e.stopPropagation()}
           className={classNames("w-full appearance-none border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-1 focus:ring-blue-500", styles.compactSelect)}
         >
           <option value="" disabled>Select an option...</option>
           {choices.map((choice, idx) => {
             const label = typeof choice === 'object' ? 
               (choice.room && choice.date && choice.time) ? 
                 `${choice.room} (${choice.date} ${choice.time})` : 
                 (choice.label || choice.name || choice.room || JSON.stringify(choice)) : 
               String(choice);
             const value = typeof choice === 'object' ? JSON.stringify(choice) : choice;
             return (
               <option key={idx} value={value}>
                 {label}
               </option>
             );
           })}

         </select>
       </div>
      )}

      {isTimer && (() => {
        const cx = 70, cy = 70, rFace = 56, rTick = 42, rLabel = 57;
        const selectedIdx = TIMER_EVERY_PRESETS.indexOf(localTimerEvery);

        // Convert preset string to seconds for log-scale positioning
        const toSeconds = (s: string) => {
          if (s.endsWith('d')) return parseInt(s) * 86400;
          if (s.endsWith('h')) return parseInt(s) * 3600;
          if (s.endsWith('m')) return parseInt(s) * 60;
          return parseInt(s);
        };
        const startDeg = -70, arcDeg = 270; // slightly right of top → lower-left, clockwise (gap on upper-left)
        const logVals = TIMER_EVERY_PRESETS.map(p => Math.log(toSeconds(p)));
        const logMin = logVals[0], logRange = logVals[logVals.length - 1] - logMin;
        const angleFor = (i: number) => {
          const norm = (logVals[i] - logMin) / logRange;
          return (startDeg + norm * arcDeg) * (Math.PI / 180);
        };
        const handAngle = selectedIdx >= 0 ? angleFor(selectedIdx) : -Math.PI / 2;
        const handX = cx + rTick * Math.cos(handAngle);
        const handY = cy + rTick * Math.sin(handAngle);
        return (
          <div className={`${(isChild || (isControl && !isFocused)) ? "mt-1" : "mt-2"} space-y-1`}>
            {/* Clock dial + value side by side */}
            <div className="flex items-center gap-2">
              <svg width="140" height="140" viewBox="0 0 140 140" className="flex-shrink-0">
                {/* Face: active arc (log-scale range) */}
                {(() => {
                  const a0 = angleFor(0);
                  const a1 = angleFor(TIMER_EVERY_PRESETS.length - 1);
                  const x0 = cx + rFace * Math.cos(a0), y0 = cy + rFace * Math.sin(a0);
                  const x1 = cx + rFace * Math.cos(a1), y1 = cy + rFace * Math.sin(a1);
                  return (
                    <>
                      <circle cx={cx} cy={cy} r={rFace} fill="none" stroke="#e5e7eb" strokeWidth="1.5" />
                      <path d={`M ${x0} ${y0} A ${rFace} ${rFace} 0 ${arcDeg > 180 ? 1 : 0} 1 ${x1} ${y1}`}
                        fill="none" stroke="#d1d5db" strokeWidth="2" strokeLinecap="round" />
                    </>
                  );
                })()}
                {/* 0m marker at top (12 o'clock) */}
                <text x={cx} y={cy - rLabel - 1} textAnchor="middle" dominantBaseline="auto"
                  fontSize="7" fontFamily="monospace" fill="#9ca3af">0</text>
                <line x1={cx} y1={cy - rFace + 3} x2={cx} y2={cy - rFace + 8}
                  stroke="#d1d5db" strokeWidth="1.5" strokeLinecap="round" />
                {/* Preset ticks + labels */}
                {TIMER_EVERY_PRESETS.map((preset, idx) => {
                  const angle = angleFor(idx);
                  const tx = cx + rTick * Math.cos(angle);
                  const ty = cy + rTick * Math.sin(angle);
                  const lx = cx + rLabel * Math.cos(angle);
                  const ly = cy + rLabel * Math.sin(angle);
                  const isSel = preset === localTimerEvery;
                  return (
                    <g key={preset} style={{ cursor: 'pointer' }}
                      onClick={(e) => { e.stopPropagation(); handleTimerEveryChange(preset); }}>
                      {/* Larger hit area */}
                      <circle cx={tx} cy={ty} r={10} fill="transparent" />
                      <circle cx={tx} cy={ty} r={isSel ? 5.5 : 3.5}
                        fill={isSel ? '#3b82f6' : '#9ca3af'}
                        style={{ transition: 'all 0.2s' }} />
                      <text x={lx} y={ly} textAnchor="middle" dominantBaseline="central"
                        fontSize="9.5" fontFamily="monospace"
                        fontWeight={isSel ? 'bold' : 'normal'}
                        fill={isSel ? '#3b82f6' : '#6b7280'}>
                        {preset}
                      </text>
                    </g>
                  );
                })}
                {/* Hand */}
                {selectedIdx >= 0 && (
                  <line x1={cx} y1={cy} x2={handX} y2={handY}
                    stroke="#3b82f6" strokeWidth="2" strokeLinecap="round"
                    style={{ transition: 'all 0.25s ease' }} />
                )}
                {/* Center pivot */}
                <circle cx={cx} cy={cy} r="3.5" fill="#3b82f6" />
              </svg>
              {/* Value display to the right */}
              <div className="flex flex-col items-start gap-0.5 flex-1">
                <span className="text-[10px] text-gray-400 dark:text-gray-500 font-mono truncate leading-none" title={timerTargetParam}>
                  {timerTargetParam || 'timer'}
                </span>
                <span className="text-xl font-mono font-bold text-blue-500 dark:text-blue-400 leading-tight">
                  {localTimerEvery || '--'}
                </span>
                {localTimerAt && (
                  <span className="text-[10px] font-mono text-gray-500 dark:text-gray-400">
                    @ {localTimerAt}
                  </span>
                )}
              </div>
            </div>
            {/* at input */}
            <input
              type="text"
              value={localTimerAt}
              onChange={(e) => handleTimerAtChange(e.target.value)}
              onClick={(e) => e.stopPropagation()}
              onMouseDown={(e) => e.stopPropagation()}
              placeholder="at (optional, e.g. 09:00)"
              className="w-full px-2 py-0.5 text-[10px] font-mono border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-1 focus:ring-blue-500"
            />
          </div>
        );
      })()}
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
      {want.state?.final_result != null && want.state?.final_result !== '' && (
        <FinalResultDisplay
          value={want.state!.final_result}
          isChild={isChild}
          copied={finalResultCopied}
          onCopy={handleCopyFinalResult}
          onView={() => onViewResults ? onViewResults(want) : onView(want)}
        />
      )}

      </div>
      
      </div>{/* end flex container */}

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