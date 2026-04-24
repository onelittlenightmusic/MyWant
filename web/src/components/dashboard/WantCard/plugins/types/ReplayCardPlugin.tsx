import React, { useState, useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { Circle, Camera, X } from 'lucide-react';
import { BrowserFrame } from '@/components/replay/BrowserFrame';
import { WantCardPluginProps, registerWantCardPlugin } from '../registry';

const calcBubbleStyle = (e: React.MouseEvent, widthMultiplier = 1.3): React.CSSProperties => {
  const btn = e.currentTarget as HTMLElement;
  const card = btn.closest('[data-keyboard-nav-id]');
  const cardRect = card?.getBoundingClientRect() ?? btn.getBoundingClientRect();
  const btnRect = btn.getBoundingClientRect();
  const bubbleWidth = cardRect.width * widthMultiplier;
  const bubbleMaxHeight = Math.min(window.innerHeight * 0.75, 560);
  let left = cardRect.left;
  if (left + bubbleWidth > window.innerWidth - 8) left = window.innerWidth - 8 - bubbleWidth;
  left = Math.max(8, left);
  let top = btnRect.bottom + 8;
  if (top + bubbleMaxHeight > window.innerHeight - 8) top = Math.max(8, btnRect.top - bubbleMaxHeight - 8);
  return { position: 'fixed', left, top, width: bubbleWidth, maxHeight: bubbleMaxHeight };
};

const ReplayContentSection: React.FC<WantCardPluginProps> = ({ want, isChild }) => {
  const wantName = want.metadata?.name || want.metadata?.id || 'Unnamed Want';
  const wantId = want.metadata?.id ?? '';

  const recordingActive = want.state?.current?.recording_active === true;
  const debugRecordingActive = want.state?.current?.debug_recording_active === true;
  const replayActive = want.state?.current?.replay_active === true;
  const iframeUrl = want.state?.current?.recording_iframe_url as string | undefined;
  const replayIframeUrl = want.state?.current?.replay_iframe_url as string | undefined;
  const hasFinalResult = want.state?.final_result != null && want.state?.final_result !== '';
  const hasReplayActions = Boolean(
    want.state?.current?.replay_actions &&
    (want.state.current.replay_actions as string) !== '[]',
  );
  const debugRecordingError = want.state?.current?.debug_recording_error as string | undefined;
  const replayError = want.state?.current?.replay_error as string | undefined;
  const replayResultRaw = want.state?.current?.replay_result as string | undefined;
  const replayResult = (() => {
    if (!replayResultRaw) return null;
    try { return JSON.parse(replayResultRaw); } catch { return null; }
  })();
  const replayScreenshotUrl = want.state?.current?.replay_screenshot_url as string | undefined;

  const startWebhookId =
    (want.state?.current?.startWebhookId as string | undefined) || (wantId ? `${wantId}-start` : undefined);
  const stopWebhookId =
    (want.state?.current?.stopWebhookId as string | undefined) || (wantId ? `${wantId}-stop` : undefined);
  const debugStartWebhookId =
    (want.state?.current?.debugStartWebhookId as string | undefined) || (wantId ? `${wantId}-debug-start` : undefined);
  const debugStopWebhookId =
    (want.state?.current?.debugStopWebhookId as string | undefined) || (wantId ? `${wantId}-debug-stop` : undefined);
  const replayWebhookId =
    (want.state?.current?.replayWebhookId as string | undefined) || (wantId ? `${wantId}-replay` : undefined);

  const [showReplayBubble, setShowReplayBubble] = useState(false);
  const [showScreenshotBubble, setShowScreenshotBubble] = useState(false);
  const [replayBubbleStyle, setReplayBubbleStyle] = useState<React.CSSProperties>({});
  const [screenshotBubbleStyle, setScreenshotBubbleStyle] = useState<React.CSSProperties>({});
  const replayBubbleRef = useRef<HTMLDivElement>(null);
  const screenshotBubbleRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!showReplayBubble && !showScreenshotBubble) return;
    const handleMouseDown = (e: MouseEvent) => {
      if (showReplayBubble && replayBubbleRef.current && !replayBubbleRef.current.contains(e.target as Node))
        setShowReplayBubble(false);
      if (showScreenshotBubble && screenshotBubbleRef.current && !screenshotBubbleRef.current.contains(e.target as Node))
        setShowScreenshotBubble(false);
    };
    document.addEventListener('mousedown', handleMouseDown);
    return () => document.removeEventListener('mousedown', handleMouseDown);
  }, [showReplayBubble, showScreenshotBubble]);

  useEffect(() => {
    if (!replayActive) {
      const t = setTimeout(() => setShowReplayBubble(false), 800);
      return () => clearTimeout(t);
    }
  }, [replayActive]); // eslint-disable-line react-hooks/exhaustive-deps

  const postWebhook = async (id: string, action: string) => {
    try {
      await fetch(`/api/v1/webhooks/${id}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action }),
      });
    } catch (err) {
      console.error(`[ReplayCard] webhook ${action} failed:`, err);
    }
  };

  const mt = isChild ? 'mt-2' : 'mt-4';

  return (
    <>
      {/* Idle: Record buttons */}
      {!recordingActive && !debugRecordingActive && !hasFinalResult && (
        <div className={`flex items-center gap-2 ${mt}`}>
          {startWebhookId && (
            <button onClick={(e) => { e.stopPropagation(); postWebhook(startWebhookId, 'start_recording'); }}
              className="flex items-center gap-1.5 px-2 py-1 rounded text-xs bg-red-600 text-white hover:bg-red-700 transition-colors">
              <Circle className="w-3 h-3 fill-current" /> Record
            </button>
          )}
          {debugStartWebhookId && (
            <button onClick={(e) => { e.stopPropagation(); postWebhook(debugStartWebhookId, 'start_debug_recording'); }}
              className="flex items-center gap-1.5 px-2 py-1 rounded text-xs bg-orange-600 text-white hover:bg-orange-700 transition-colors">
              <Circle className="w-3 h-3 fill-current" /> Record in debug
            </button>
          )}
        </div>
      )}

      {/* Recording active: iframe */}
      {recordingActive && iframeUrl && (
        <BrowserFrame iframeUrl={iframeUrl} wantId={wantId} stopWebhookId={stopWebhookId ?? ''} />
      )}

      {/* Debug recording active: Finish button */}
      {debugRecordingActive && (
        <div className={mt}>
          <div className="flex items-center gap-2 p-2 rounded bg-orange-50 dark:bg-orange-900/20 border border-orange-200 dark:border-orange-800">
            <span className="inline-block w-2 h-2 rounded-full bg-orange-500 animate-pulse flex-shrink-0" />
            <span className="text-xs text-orange-700 dark:text-orange-300 flex-1">Recording debug Chrome…</span>
            {debugStopWebhookId && (
              <button onClick={(e) => { e.stopPropagation(); postWebhook(debugStopWebhookId, 'stop_debug_recording'); }}
                className="flex items-center gap-1 px-2 py-1 rounded text-xs bg-orange-600 text-white hover:bg-orange-700 transition-colors flex-shrink-0">
                Finish
              </button>
            )}
          </div>
        </div>
      )}

      {/* Debug recording error */}
      {debugRecordingError && !debugRecordingActive && (
        <div className={mt}>
          <div className="flex items-start gap-2 p-2 rounded bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800">
            <span className="text-xs text-red-700 dark:text-red-300 flex-1 break-all">
              Debug recording failed: {debugRecordingError}
            </span>
          </div>
        </div>
      )}

      {/* Replay button / replaying indicator */}
      {hasReplayActions && replayWebhookId && (
        <div className={mt}>
          {!replayActive ? (
            <button onClick={(e) => { e.stopPropagation(); postWebhook(replayWebhookId, 'start_replay'); }}
              className="flex items-center gap-1.5 px-2 py-1 rounded text-xs bg-green-600 text-white hover:bg-green-700 transition-colors">
              ▶ Replay
            </button>
          ) : (
            <button
              onClick={(e) => { e.stopPropagation(); setReplayBubbleStyle(calcBubbleStyle(e)); setShowReplayBubble(true); }}
              className="flex items-center gap-1.5 px-2 py-1 rounded text-xs bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300 border border-green-300 dark:border-green-700 hover:bg-green-200 dark:hover:bg-green-900/50 transition-colors">
              <span className="w-1.5 h-1.5 rounded-full bg-green-500 animate-pulse flex-shrink-0" />
              Replaying…
            </button>
          )}
        </div>
      )}

      {/* Replay result */}
      {replayResult && !replayActive && (
        <div className={mt}>
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
                  onClick={(e) => { e.stopPropagation(); setScreenshotBubbleStyle(calcBubbleStyle(e, 1.3)); setShowScreenshotBubble(true); }}
                  className="flex-shrink-0 p-1 rounded bg-green-100 dark:bg-green-900/40 text-green-600 dark:text-green-400 hover:bg-green-200 dark:hover:bg-green-800 transition-colors">
                  <Camera className="w-3.5 h-3.5" />
                </button>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Replay error */}
      {replayError && !replayActive && (
        <div className={mt}>
          <div className="flex items-start gap-2 p-2 rounded bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800">
            <span className="text-xs text-red-700 dark:text-red-300 flex-1 break-all">Replay failed: {replayError}</span>
          </div>
        </div>
      )}

      {/* Floating Replay Bubble */}
      {showReplayBubble && createPortal(
        <div ref={replayBubbleRef}
          className="z-[9999] bg-white dark:bg-gray-900 rounded-2xl shadow-2xl border border-gray-200 dark:border-gray-700 flex flex-col overflow-hidden"
          style={replayBubbleStyle}>
          <div className="flex items-center justify-between px-3 py-2 border-b border-gray-200 dark:border-gray-700 flex-shrink-0">
            <div className="flex items-center gap-2">
              {replayActive && <span className="w-2 h-2 rounded-full bg-green-500 animate-pulse flex-shrink-0" />}
              <span className="text-xs font-semibold text-gray-800 dark:text-gray-100">
                {replayActive ? 'Replaying…' : 'Replay'}
              </span>
              <span className="text-xs text-gray-400 dark:text-gray-500 truncate max-w-[120px]">{wantName}</span>
            </div>
            <button onClick={() => setShowReplayBubble(false)}
              className="p-1 rounded text-gray-400 hover:text-gray-600 hover:bg-gray-100 dark:hover:text-gray-200 dark:hover:bg-gray-800 transition-colors">
              <X className="w-3.5 h-3.5" />
            </button>
          </div>
          <div className="flex-1 min-h-0">
            {replayIframeUrl
              ? <iframe src={replayIframeUrl} className="w-full h-full border-0" title="Replay viewer" />
              : <div className="flex items-center justify-center h-32 gap-2 text-gray-400 dark:text-gray-500">
                  <span className="w-2 h-2 rounded-full bg-gray-300 dark:bg-gray-600 animate-pulse" />
                  <span className="text-xs">Starting replay…</span>
                </div>
            }
          </div>
        </div>,
        document.body,
      )}

      {/* Floating Screenshot Bubble */}
      {showScreenshotBubble && replayScreenshotUrl && createPortal(
        <div ref={screenshotBubbleRef}
          className="z-[9999] bg-white dark:bg-gray-900 rounded-2xl shadow-2xl border border-gray-200 dark:border-gray-700 flex flex-col overflow-hidden"
          style={screenshotBubbleStyle}>
          <div className="flex items-center justify-between px-3 py-2 border-b border-gray-200 dark:border-gray-700 flex-shrink-0">
            <div className="flex items-center gap-2">
              <Camera className="w-3.5 h-3.5 text-gray-500 dark:text-gray-400" />
              <span className="text-xs font-semibold text-gray-800 dark:text-gray-100">Screenshot</span>
              <span className="text-xs text-gray-400 dark:text-gray-500 truncate max-w-[120px]">{wantName}</span>
            </div>
            <button onClick={() => setShowScreenshotBubble(false)}
              className="p-1 rounded text-gray-400 hover:text-gray-600 hover:bg-gray-100 dark:hover:text-gray-200 dark:hover:bg-gray-800 transition-colors">
              <X className="w-3.5 h-3.5" />
            </button>
          </div>
          <div className="overflow-auto">
            <img src={replayScreenshotUrl} alt="Replay screenshot" className="block w-full" />
          </div>
        </div>,
        document.body,
      )}
    </>
  );
};

registerWantCardPlugin({
  types: ['replay'],
  ContentSection: ReplayContentSection,
});
