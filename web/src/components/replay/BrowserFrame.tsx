import React, { useEffect, useRef } from 'react';

interface BrowserFrameProps {
  iframeUrl: string;
  wantId: string;
  stopWebhookId: string;
}

/**
 * BrowserFrame renders the Playwright MCP App Server UI inside an iframe.
 * It acts as the ext-apps host bridge, listening for PostMessage events
 * from the iframe and forwarding the "Finish" action to the stop webhook.
 */
export const BrowserFrame: React.FC<BrowserFrameProps> = ({
  iframeUrl,
  wantId,
  stopWebhookId,
}) => {
  const iframeRef = useRef<HTMLIFrameElement>(null);

  useEffect(() => {
    const handleMessage = (event: MessageEvent) => {
      if (iframeRef.current && event.source !== iframeRef.current.contentWindow) {
        return;
      }
      // ext-apps app-bridge: recording_finish is sent when the user clicks "Finish" inside the iframe
      if (event.data?.type === 'recording_finish') {
        fetch(`/api/v1/webhooks/${stopWebhookId}`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ action: 'stop_recording' }),
        }).catch((err) => console.error('[BrowserFrame] stop webhook failed:', err));
      }
    };

    window.addEventListener('message', handleMessage);
    return () => window.removeEventListener('message', handleMessage);
  }, [stopWebhookId]);

  if (!iframeUrl) {
    return null;
  }

  return (
    <div className="mt-2 rounded overflow-hidden border border-gray-300 dark:border-gray-600 bg-black" style={{ height: '24rem' }}>
      <div className="flex items-center justify-between px-2 py-1 bg-gray-100 dark:bg-gray-800 border-b border-gray-300 dark:border-gray-600">
        <span className="text-xs text-gray-500 dark:text-gray-400 truncate max-w-xs" title={iframeUrl}>
          {iframeUrl}
        </span>
        <span className="text-xs text-red-500 font-medium flex items-center gap-1">
          <span className="inline-block w-2 h-2 rounded-full bg-red-500 animate-pulse" />
          REC
        </span>
      </div>
      <iframe
        ref={iframeRef}
        src={iframeUrl}
        title={`Browser recording - want ${wantId}`}
        sandbox="allow-scripts allow-same-origin allow-forms allow-popups"
        className="w-full border-0"
        style={{ height: 'calc(100% - 2rem)' }}
      />
    </div>
  );
};
