/**
 * Playwright MCP App Server
 *
 * Exposes two MCP tools over stdio transport:
 *   - start_recording({ target_url })  → { session_id, ui_url }
 *   - stop_recording({ session_id })   → { script }
 *
 * Each session launches a Chromium browser via Playwright, streams screenshots
 * over WebSocket, and codegen-captures user interactions.
 * The WebSocket server URL is embedded in the ui_url so the React UI (served on
 * the same HTTP port) can connect automatically.
 */

import { Server } from '@modelcontextprotocol/sdk/server/index.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
} from '@modelcontextprotocol/sdk/types.js';
import { chromium, Browser, Page, BrowserContext } from 'playwright';
import { WebSocketServer, WebSocket } from 'ws';
import * as http from 'http';
import * as fs from 'fs';
import * as path from 'path';
import * as crypto from 'crypto';
import * as os from 'os';
import { webInspectorOverlayCore } from './webInspectorOverlayCore';

// ---------------------------------------------------------------------------
// Session registry
// ---------------------------------------------------------------------------

interface RecordingSession {
  sessionId: string;
  browser: Browser;
  context: BrowserContext;
  page: Page;
  actions: string[];   // Captured codegen-style action strings
  screenshotTimer: ReturnType<typeof setInterval> | null;
  clients: Set<WebSocket>;
}

interface DebugRecordingSession {
  sessionId: string;
  browser: Browser;  // connected via CDP (do NOT close)
  page: Page;
  startUrl: string;
  allActions: string[];    // accumulated actions across all page navigations
  pollTimer: ReturnType<typeof setInterval>;
  lastActionIndex: number; // index into current page's actions array
  lastKnownUrl: string;   // tracks URL to detect page navigations
}

interface ReplayResult {
  selected_text: string;
  active_element: Record<string, any> | null;
  url: string;
  screenshot_path?: string;
}

interface ReplaySession {
  sessionId: string;
  browser: Browser;
  context: BrowserContext;
  page: Page;
  screenshotTimer: ReturnType<typeof setInterval> | null;
  clients: Set<WebSocket>;
  done: boolean;
  result?: ReplayResult;
  error?: string;
}

const sessions = new Map<string, RecordingSession>();
const debugSessions = new Map<string, DebugRecordingSession>();
const replaySessions = new Map<string, ReplaySession>();

// ---------------------------------------------------------------------------
// HTTP + WebSocket server for the browser UI
// ---------------------------------------------------------------------------

const WS_PORT = parseInt(process.env.PLAYWRIGHT_WS_PORT ?? '9321', 10);
const HTTP_PORT = parseInt(process.env.PLAYWRIGHT_HTTP_PORT ?? '9322', 10);

const wss = new WebSocketServer({ port: WS_PORT });

wss.on('connection', (ws, req) => {
  const url = new URL(req.url ?? '/', `http://localhost:${WS_PORT}`);
  const sessionId = url.searchParams.get('session') ?? '';

  // Replay sessions: screenshot-only, no user input accepted
  const replaySession = replaySessions.get(sessionId);
  if (replaySession) {
    replaySession.clients.add(ws);
    ws.on('close', () => { replaySession.clients.delete(ws); });
    return;
  }

  const session = sessions.get(sessionId);
  if (!session) {
    ws.close(1008, 'session not found');
    return;
  }
  session.clients.add(ws);

  ws.on('message', async (data) => {
    try {
      const msg = JSON.parse(data.toString());
      if (msg.type === 'click' && typeof msg.x === 'number' && typeof msg.y === 'number') {
        await session.page.mouse.click(msg.x, msg.y);
        session.actions.push(`  await page.mouse.click(${msg.x}, ${msg.y});`);
      } else if (msg.type === 'type' && typeof msg.text === 'string') {
        await session.page.keyboard.type(msg.text);
        session.actions.push(`  await page.keyboard.type(${JSON.stringify(msg.text)});`);
      } else if (msg.type === 'navigate' && typeof msg.url === 'string') {
        await session.page.goto(msg.url);
        session.actions.push(`  await page.goto(${JSON.stringify(msg.url)});`);
      }
    } catch {
      // Ignore individual input errors
    }
  });

  ws.on('close', () => {
    session.clients.delete(ws);
  });
});

// Simple HTTP server that serves the browser-view React UI HTML
const httpServer = http.createServer((req, res) => {
  const url = new URL(req.url ?? '/', `http://localhost:${HTTP_PORT}`);

  if (url.pathname === '/health') {
    res.writeHead(200);
    res.end('ok');
    return;
  }

  // Serve the inline browser-view UI
  const sessionId = url.searchParams.get('session') ?? '';
  res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
  res.end(buildUIHtml(sessionId, WS_PORT));
});

httpServer.listen(HTTP_PORT, () => {
  // Server ready
});

// ---------------------------------------------------------------------------
// Screenshot streaming helper
// ---------------------------------------------------------------------------

function startScreenshotStream(session: RecordingSession) {
  session.screenshotTimer = setInterval(async () => {
    if (session.clients.size === 0) return;
    try {
      const buf = await session.page.screenshot({ type: 'png' });
      const b64 = buf.toString('base64');
      const msg = JSON.stringify({ type: 'screenshot', data: b64 });
      for (const client of session.clients) {
        if (client.readyState === WebSocket.OPEN) {
          client.send(msg);
        }
      }
    } catch {
      // Page may be navigating
    }
  }, 200);
}

function startReplayScreenshotStream(session: ReplaySession) {
  session.screenshotTimer = setInterval(async () => {
    if (session.clients.size === 0) return;
    try {
      const buf = await session.page.screenshot({ type: 'png' });
      const b64 = buf.toString('base64');
      const msg = JSON.stringify({ type: 'screenshot', data: b64 });
      for (const client of session.clients) {
        if (client.readyState === WebSocket.OPEN) client.send(msg);
      }
    } catch { }
  }, 200);
}

// ---------------------------------------------------------------------------
// Playwright codegen script builder
// ---------------------------------------------------------------------------

function buildPlaywrightScript(targetUrl: string, actions: string[]): string {
  return [
    `import { test, expect } from '@playwright/test';`,
    ``,
    `test('recorded session', async ({ page }) => {`,
    `  await page.goto(${JSON.stringify(targetUrl)});`,
    ...actions,
    `});`,
  ].join('\n');
}

// ---------------------------------------------------------------------------
// Inline HTML for the browser-view UI
// ---------------------------------------------------------------------------

function buildUIHtml(sessionId: string, wsPort: number): string {
  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>Browser Recording</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body { background: #000; display: flex; flex-direction: column; height: 100vh; font-family: sans-serif; }
    #toolbar {
      background: #1a1a1a; padding: 6px 10px; display: flex;
      align-items: center; gap: 8px; border-bottom: 1px solid #333;
    }
    #toolbar .rec-dot { width: 10px; height: 10px; border-radius: 50%; background: #ef4444; animation: pulse 1.4s infinite; }
    @keyframes pulse { 0%,100%{opacity:1} 50%{opacity:.4} }
    #toolbar span { color: #ccc; font-size: 12px; }
    #finish-btn {
      margin-left: auto; background: #7c3aed; color: #fff; border: none;
      padding: 4px 12px; border-radius: 4px; cursor: pointer; font-size: 12px;
    }
    #finish-btn:hover { background: #6d28d9; }
    #canvas-container { flex: 1; overflow: hidden; position: relative; }
    #screen { display: block; width: 100%; height: 100%; object-fit: contain; cursor: crosshair; }
  </style>
</head>
<body>
  <div id="toolbar">
    <div class="rec-dot"></div>
    <span>Recording…</span>
    <button id="finish-btn" onclick="finish()">Finish</button>
  </div>
  <div id="canvas-container">
    <img id="screen" alt="browser view" />
  </div>
  <script>
    const sessionId = ${JSON.stringify(sessionId)};
    const wsPort = ${wsPort};
    const screen = document.getElementById('screen');
    let ws;

    function connect() {
      ws = new WebSocket('ws://localhost:' + wsPort + '/?session=' + encodeURIComponent(sessionId));
      ws.binaryType = 'blob';
      ws.onmessage = (e) => {
        try {
          const msg = JSON.parse(e.data);
          if (msg.type === 'screenshot') {
            screen.src = 'data:image/png;base64,' + msg.data;
          }
        } catch {}
      };
      ws.onclose = () => setTimeout(connect, 1000);
    }
    connect();

    screen.addEventListener('click', (e) => {
      const rect = screen.getBoundingClientRect();
      const naturalW = screen.naturalWidth || rect.width;
      const naturalH = screen.naturalHeight || rect.height;
      const x = Math.round(((e.clientX - rect.left) / rect.width) * naturalW);
      const y = Math.round(((e.clientY - rect.top) / rect.height) * naturalH);
      if (ws && ws.readyState === 1) ws.send(JSON.stringify({ type: 'click', x, y }));
    });

    function finish() {
      // Notify the host frame (MyWant WantCardContent) via PostMessage
      window.parent.postMessage({ type: 'recording_finish', sessionId }, '*');
    }
  </script>
</body>
</html>`;
}

// ---------------------------------------------------------------------------
// MCP Server
// ---------------------------------------------------------------------------

const server = new Server(
  { name: 'playwright-mcp-app', version: '1.0.0' },
  { capabilities: { tools: {} } },
);

server.setRequestHandler(ListToolsRequestSchema, async () => ({
  tools: [
    {
      name: 'start_recording',
      description: 'Start a Playwright browser recording session. Returns session_id and ui_url for iframe embedding.',
      inputSchema: {
        type: 'object',
        properties: {
          target_url: {
            type: 'string',
            description: 'URL to open when recording starts',
            default: 'https://example.com',
          },
        },
      },
    },
    {
      name: 'stop_recording',
      description: 'Stop a recording session and return the generated Playwright .spec.ts script.',
      inputSchema: {
        type: 'object',
        properties: {
          session_id: {
            type: 'string',
            description: 'Session ID returned by start_recording',
          },
        },
        required: ['session_id'],
      },
    },
    {
      name: 'start_recording_debug',
      description: 'Attach to an existing Chrome running with --remote-debugging-port=9222 and start recording user interactions. Returns session_id only (no iframe).',
      inputSchema: {
        type: 'object',
        properties: {
          cdp_url: {
            type: 'string',
            description: 'Chrome DevTools Protocol endpoint URL',
            default: 'http://localhost:9222',
          },
          target_url: {
            type: 'string',
            description: 'Optional URL to navigate to before recording starts. If omitted, the current active page is used as-is.',
          },
        },
      },
    },
    {
      name: 'stop_recording_debug',
      description: 'Stop a debug recording session and return the generated Playwright .spec.ts script.',
      inputSchema: {
        type: 'object',
        properties: {
          session_id: {
            type: 'string',
            description: 'Session ID returned by start_recording_debug',
          },
        },
        required: ['session_id'],
      },
    },
    {
      name: 'run_replay',
      description: 'Start replaying a recorded script in a new browser. Returns session_id and ui_url immediately; execution runs in background.',
      inputSchema: {
        type: 'object',
        properties: {
          start_url: { type: 'string', description: 'URL to navigate to before executing actions' },
          actions: { type: 'array', items: { type: 'string' }, description: 'Recorded Playwright action strings' },
        },
        required: ['start_url', 'actions'],
      },
    },
    {
      name: 'check_replay',
      description: 'Poll for replay completion. Returns { done, result?, error? }. Cleans up the session when done=true.',
      inputSchema: {
        type: 'object',
        properties: {
          session_id: { type: 'string', description: 'Session ID returned by run_replay' },
        },
        required: ['session_id'],
      },
    },
    {
      name: 'open_inspector_tab',
      description: 'Open a target URL in an existing Chrome (via CDP) and inject the Web Want Inspector overlay. The overlay lets users navigate interactive elements with arrow keys/gamepad and select them. When done, it POSTs selected elements to the mywant webhook. Returns immediately; user interaction is async.',
      inputSchema: {
        type: 'object',
        properties: {
          cdp_url: {
            type: 'string',
            description: 'Chrome DevTools Protocol endpoint URL',
            default: 'http://localhost:9222',
          },
          target_url: {
            type: 'string',
            description: 'URL to open in the new inspector tab',
          },
          done_webhook_url: {
            type: 'string',
            description: 'Full URL to POST selected elements to when the user clicks Done (e.g. http://localhost:8080/api/v1/webhooks/{id}-done)',
          },
          suggest_name_url: {
            type: 'string',
            description: 'Optional full URL to POST {html} to for a speculative AI-suggested element name (e.g. http://localhost:8080/api/v1/web-wants/suggest-name). Omit to disable speculative naming.',
          },
          character_id: {
            type: 'string',
            description: 'ID of the character marking elements this session — echoed back in the Done payload for attribution.',
          },
          color: {
            type: 'string',
            description: 'Hex color for character_id, resolved server-side — new selections are outlined in this color.',
          },
          avatar: {
            type: 'string',
            description: 'Emoji avatar for character_id, resolved server-side — the shared CursorMan shows this emoji in a colored circle instead of the default stick-figure SVG, matching mywant-gui\'s CursorManIcon.',
          },
          existing_marks: {
            type: 'string',
            description: 'JSON array of {role,name,selector,fieldKey,htmlName,characterId,color} previously recorded for this hostname (any character), rendered as read-only indicators.',
          },
          nav_elements: {
            type: 'string',
            description: 'JSON array of {role,name,selector} — the same saved elements a web want type\'s Launch action navigates, shown here via an amber CursorMan cycled with Cmd+Arrow keys / gamepad L1+D-pad so it does not conflict with the inspector\'s own plain-arrow element navigation.',
          },
        },
        required: ['target_url', 'done_webhook_url'],
      },
    },
    {
      name: 'open_nav_tab',
      description: 'Open a target URL in an existing Chrome (via CDP) and inject a navigation-only overlay for pre-saved elements. Arrow keys / gamepad cycle only through those elements — no full-page scan. Used after web_inspector has already identified the elements.',
      inputSchema: {
        type: 'object',
        properties: {
          cdp_url: {
            type: 'string',
            description: 'Chrome DevTools Protocol endpoint URL',
            default: 'http://localhost:9222',
          },
          target_url: {
            type: 'string',
            description: 'URL to open',
          },
          nav_elements: {
            type: 'string',
            description: 'JSON array of {role, name, selector} objects to navigate between',
          },
        },
        required: ['target_url', 'nav_elements'],
      },
    },
    {
      name: 'fill_form',
      description: 'Open a URL in an existing Chrome (via CDP), fill text inputs from field_values, then click buttons or press Enter to submit. Input elements are processed before buttons.',
      inputSchema: {
        type: 'object',
        properties: {
          cdp_url: {
            type: 'string',
            description: 'Chrome DevTools Protocol endpoint URL',
            default: 'http://localhost:9222',
          },
          target_url: {
            type: 'string',
            description: 'URL to open',
          },
          elements: {
            type: 'string',
            description: 'JSON array of {role, name, selector, field_key} objects',
          },
          field_values: {
            type: 'string',
            description: 'JSON object mapping field_key to value string',
          },
        },
        required: ['target_url', 'elements', 'field_values'],
      },
    },
  ],
}));

server.setRequestHandler(CallToolRequestSchema, async (request) => {
  const { name, arguments: args } = request.params;

  if (name === 'start_recording') {
    const targetUrl = (args?.target_url as string | undefined) ?? 'https://example.com';
    const sessionId = crypto.randomUUID();

    const browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({
      viewport: { width: 1280, height: 800 },
      recordVideo: undefined,
    });
    const page = await context.newPage();

    // Track navigation actions
    const actions: string[] = [];
    page.on('framenavigated', (frame) => {
      if (frame === page.mainFrame()) {
        actions.push(`  await page.goto(${JSON.stringify(frame.url())});`);
      }
    });

    await page.goto(targetUrl);

    const session: RecordingSession = {
      sessionId,
      browser,
      context,
      page,
      actions,
      screenshotTimer: null,
      clients: new Set(),
    };
    sessions.set(sessionId, session);
    startScreenshotStream(session);

    const uiUrl = `http://localhost:${HTTP_PORT}/?session=${encodeURIComponent(sessionId)}`;
    const result = { session_id: sessionId, ui_url: uiUrl };

    return {
      content: [{ type: 'text', text: JSON.stringify(result) }],
    };
  }

  if (name === 'stop_recording') {
    const sessionId = (args?.session_id as string | undefined) ?? '';
    const session = sessions.get(sessionId);
    if (!session) {
      return {
        content: [{ type: 'text', text: JSON.stringify({ error: 'session not found', session_id: sessionId }) }],
        isError: true,
      };
    }

    // Stop screenshot streaming
    if (session.screenshotTimer) {
      clearInterval(session.screenshotTimer);
    }

    // Close all WebSocket clients
    for (const client of session.clients) {
      client.close();
    }

    const startUrl = (await session.page.url()) || 'about:blank';
    const script = buildPlaywrightScript(startUrl, session.actions);
    const actions = session.actions;

    // Cleanup browser resources
    await session.context.close();
    await session.browser.close();
    sessions.delete(sessionId);

    return {
      content: [{ type: 'text', text: JSON.stringify({ script, actions, start_url: startUrl }) }],
    };
  }

  if (name === 'start_recording_debug') {
    const cdpUrl = (args?.cdp_url as string | undefined) ?? 'http://localhost:9222';
    const targetUrl = (args?.target_url as string | undefined) ?? '';
    const sessionId = crypto.randomUUID();

    let browser: Browser;
    try {
      browser = await chromium.connectOverCDP(cdpUrl, { timeout: 10000 });
    } catch (err) {
      return {
        content: [{ type: 'text', text: JSON.stringify({ error: `Failed to connect to Chrome at ${cdpUrl}: ${err}` }) }],
        isError: true,
      };
    }

    // Use the first available page, or open a new one
    const contexts = browser.contexts();
    const pages = contexts.flatMap((c) => c.pages());
    const page = pages.length > 0 ? pages[0] : await browser.contexts()[0].newPage();

    // Navigate to target_url if provided
    if (targetUrl) {
      await page.goto(targetUrl);
    }
    const startUrl = page.url();

    // Inject recorder script into the page
    await injectDebugRecorder(page);

    // Accumulate actions from current page before it navigates away, then re-inject
    const session: DebugRecordingSession = {
      sessionId,
      browser,
      page,
      startUrl,
      allActions: [],
      pollTimer: null as any,
      lastActionIndex: 0,
      lastKnownUrl: startUrl,
    };

    const pollTimer = setInterval(async () => {
      try {
        // Harvest new actions since last poll
        const currentActions: string[] = await page.evaluate(
          () => (window as any).__playwright_recorder__?.actions ?? []
        );
        if (currentActions.length > session.lastActionIndex) {
          const newActions = currentActions.slice(session.lastActionIndex);
          session.allActions.push(...newActions);
          session.lastActionIndex = currentActions.length;
        }
        // Detect page navigation by URL change and record page.goto()
        const currentUrl = page.url();
        if (currentUrl && currentUrl !== 'about:blank' && currentUrl !== session.lastKnownUrl) {
          session.allActions.push(`  await page.goto(${JSON.stringify(currentUrl)});`);
          session.lastKnownUrl = currentUrl;
          session.lastActionIndex = 0; // reset for new page's recorder
        }
        // Re-inject recorder (handles post-navigation page context reset)
        await injectDebugRecorder(page);
      } catch {
        // Page may be navigating — reset index so next poll starts fresh
        session.lastActionIndex = 0;
      }
    }, 500);

    session.pollTimer = pollTimer;
    debugSessions.set(sessionId, session);

    return {
      content: [{ type: 'text', text: JSON.stringify({ session_id: sessionId }) }],
    };
  }

  if (name === 'stop_recording_debug') {
    const sessionId = (args?.session_id as string | undefined) ?? '';
    const session = debugSessions.get(sessionId);
    if (!session) {
      return {
        content: [{ type: 'text', text: JSON.stringify({ error: 'debug session not found', session_id: sessionId }) }],
        isError: true,
      };
    }

    clearInterval(session.pollTimer);

    // Harvest remaining actions from the current page and capture selection/focus state
    let targetObject: Record<string, any> = {};
    try {
      const harvested = await session.page.evaluate(() => {
        const actions = (window as any).__playwright_recorder__?.actions ?? [];

        // Capture currently selected text
        const sel = window.getSelection();
        const selectedText = sel?.toString() ?? '';

        // Capture focused / active element
        const activeEl = document.activeElement;
        let activeElement: Record<string, any> | null = null;
        if (activeEl && activeEl !== document.body && activeEl !== document.documentElement) {
          const el = activeEl as HTMLElement;
          activeElement = {
            tag: el.tagName.toLowerCase(),
            id: el.id || null,
            name: (el as HTMLInputElement).name || null,
            text: el.textContent?.trim().substring(0, 500) || null,
            value: (el as HTMLInputElement).value ?? null,
            href: (el as HTMLAnchorElement).href || null,
            src: (el as HTMLImageElement).src || null,
            role: el.getAttribute('role') || null,
            ariaLabel: el.getAttribute('aria-label') || null,
          };
        }

        return { actions, selectedText, activeElement, url: window.location.href };
      });

      // Append any actions not yet harvested by pollTimer
      if (harvested.actions.length > session.lastActionIndex) {
        session.allActions.push(...harvested.actions.slice(session.lastActionIndex));
      }
      targetObject = {
        selected_text: harvested.selectedText,
        active_element: harvested.activeElement,
        url: harvested.url,
      };
    } catch {
      // Page may be closed
    }

    // Use accumulated actions across all navigations
    const actions = session.allActions;
    const script = buildPlaywrightScript(session.startUrl, actions);
    const startUrl = session.startUrl;
    debugSessions.delete(sessionId);
    // Note: do NOT close the browser since we connected to an existing Chrome

    return {
      content: [{ type: 'text', text: JSON.stringify({ script, actions, start_url: startUrl, target_object: targetObject }) }],
    };
  }

  if (name === 'run_replay') {
    const startUrl = (args?.start_url as string | undefined) ?? 'https://example.com';
    const actions = (args?.actions as string[] | undefined) ?? [];
    const sessionId = crypto.randomUUID();

    const browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
    const page = await context.newPage();

    const replaySession: ReplaySession = {
      sessionId, browser, context, page,
      screenshotTimer: null, clients: new Set(),
      done: false,
    };
    replaySessions.set(sessionId, replaySession);
    startReplayScreenshotStream(replaySession);

    const uiUrl = `http://localhost:${HTTP_PORT}/?session=${encodeURIComponent(sessionId)}`;

    // Execute actions in background
    (async () => {
      try {
        await page.goto(startUrl);
        for (const action of actions) {
          const trimmed = action.trim();
          if (!trimmed || trimmed.startsWith('//')) continue;
          // eslint-disable-next-line no-new-func
          const fn = new Function('page', `return (async () => { ${trimmed} })()`);
          await (fn as (p: typeof page) => Promise<void>)(page);
        }
        const evalResult = await page.evaluate(() => {
          const sel = window.getSelection();
          const activeEl = document.activeElement;
          let activeElement: Record<string, any> | null = null;
          if (activeEl && activeEl !== document.body && activeEl !== document.documentElement) {
            const el = activeEl as HTMLElement;
            activeElement = {
              tag: el.tagName.toLowerCase(),
              value: (el as HTMLInputElement).value ?? null,
              text: el.textContent?.trim().substring(0, 500) ?? null,
            };
          }
          return {
            selected_text: sel?.toString() ?? '',
            active_element: activeElement,
            url: window.location.href,
          };
        });
        const screenshotBuf = await page.screenshot({ type: 'png' });
        let screenshotPath: string | undefined;
        try {
          const homeDir = process.env.HOME ?? process.env.USERPROFILE ?? '/tmp';
          const screenshotDir = `${homeDir}/.mywant/screenshots`;
          fs.mkdirSync(screenshotDir, { recursive: true });
          screenshotPath = `${screenshotDir}/${sessionId}.png`;
          fs.writeFileSync(screenshotPath, screenshotBuf);
        } catch {
          screenshotPath = undefined;
        }
        const result: ReplayResult = {
          ...evalResult,
          ...(screenshotPath ? { screenshot_path: screenshotPath } : {}),
        };
        replaySession.result = result;
      } catch (err) {
        replaySession.error = String(err);
      } finally {
        replaySession.done = true;
      }
    })();

    return {
      content: [{ type: 'text', text: JSON.stringify({ session_id: sessionId, ui_url: uiUrl }) }],
    };
  }

  if (name === 'check_replay') {
    const sessionId = (args?.session_id as string | undefined) ?? '';
    const replaySession = replaySessions.get(sessionId);
    if (!replaySession) {
      return {
        content: [{ type: 'text', text: JSON.stringify({ error: 'replay session not found', done: true }) }],
        isError: true,
      };
    }
    if (!replaySession.done) {
      return {
        content: [{ type: 'text', text: JSON.stringify({ done: false }) }],
      };
    }
    // Done: clean up
    if (replaySession.screenshotTimer) clearInterval(replaySession.screenshotTimer);
    for (const client of replaySession.clients) client.close();
    await replaySession.context.close();
    await replaySession.browser.close();
    replaySessions.delete(sessionId);
    return {
      content: [{ type: 'text', text: JSON.stringify({ done: true, result: replaySession.result, error: replaySession.error }) }],
    };
  }

  if (name === 'open_inspector_tab') {
    const cdpUrl = (args?.cdp_url as string | undefined) ?? 'http://localhost:9222';
    const targetUrl = (args?.target_url as string | undefined) ?? '';
    const doneWebhookUrl = (args?.done_webhook_url as string | undefined) ?? '';
    const suggestNameUrl = (args?.suggest_name_url as string | undefined) ?? '';
    const characterId = (args?.character_id as string | undefined) ?? '';
    const color = (args?.color as string | undefined) ?? '';
    const avatar = (args?.avatar as string | undefined) ?? '';
    const existingMarksRaw = (args?.existing_marks as string | undefined) ?? '';
    const navElementsRaw = (args?.nav_elements as string | undefined) ?? '';

    if (!targetUrl || !doneWebhookUrl) {
      return {
        content: [{ type: 'text', text: JSON.stringify({ error: 'target_url and done_webhook_url are required' }) }],
        isError: true,
      };
    }

    let existingMarks: Array<{role:string;name:string;selector:string;characterId:string;color:string}> = [];
    if (existingMarksRaw) {
      try { existingMarks = JSON.parse(existingMarksRaw); } catch { existingMarks = []; }
    }

    let navElements: Array<{role:string;name:string;selector:string}> = [];
    if (navElementsRaw) {
      try { navElements = JSON.parse(navElementsRaw); } catch { navElements = []; }
    }

    let browser: Browser;
    try {
      browser = await chromium.connectOverCDP(cdpUrl, { timeout: 10000 });
    } catch (err) {
      return {
        content: [{ type: 'text', text: JSON.stringify({ error: `Failed to connect to Chrome at ${cdpUrl}: ${err}` }) }],
        isError: true,
      };
    }

    // Open a new page in the first available context
    const ctx = browser.contexts()[0] ?? await browser.newContext();
    const page = await ctx.newPage();
    try {
      await page.goto(targetUrl, { waitUntil: 'domcontentloaded', timeout: 30000 });
    } catch (err) {
      return {
        content: [{ type: 'text', text: JSON.stringify({ error: `Failed to navigate to ${targetUrl}: ${err}` }) }],
        isError: true,
      };
    }

    await injectWebInspector(page, doneWebhookUrl, suggestNameUrl, characterId, color, avatar, existingMarks, navElements);

    // Close the tab automatically when the user clicks Done (__mwiDoneSignal is set in the overlay).
    // No timeout here — inspection sessions can legitimately run long (reviewing
    // many elements, waiting on AI name suggestions), so we wait indefinitely
    // for the explicit Done signal rather than force-closing the tab out from
    // under an in-progress session. If the tab/page is closed some other way
    // (e.g. the user closes it manually), waitForFunction rejects and the
    // catch below just no-ops the redundant page.close().
    (async () => {
      let done = false;
      try {
        await page.waitForFunction(() => !!(window as any).__mwiDoneSignal, { timeout: 0 });
        done = true;
      } catch (_) { /* page/context already closed — nothing to do */ }

      // Best-effort completion snapshot — captured here (Node/Playwright side)
      // because page.screenshot() isn't available from the in-page overlay JS.
      // This happens *after* the overlay's own elements webhook POST already
      // fired (see __mwiDone in injectWebInspector), so it's a separate
      // follow-up POST to the same done-webhook rather than part of that
      // payload — handleWebInspectorDone (Go) merges it into want state
      // without touching selected_elements.
      if (done && doneWebhookUrl) {
        try {
          const filename = await captureAndSaveScreenshot(page);
          if (filename) {
            const screenshotUrl = new URL('/api/v1/screenshots/' + filename, doneWebhookUrl).toString();
            await fetch(doneWebhookUrl, {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ __screenshot_url: screenshotUrl }),
            });
          }
        } catch (e) {
          console.error('[WEB-INSPECTOR] screenshot capture/post failed:', e);
        }
      }

      try { await page.close(); } catch (_) {}
    })();

    return {
      content: [{ type: 'text', text: JSON.stringify({ ok: true, url: page.url() }) }],
    };
  }

  if (name === 'open_nav_tab') {
    const cdpUrl     = (args?.cdp_url      as string | undefined) ?? 'http://localhost:9222';
    const targetUrl  = (args?.target_url   as string | undefined) ?? '';
    const navElems   = (args?.nav_elements as string | undefined) ?? '[]';

    if (!targetUrl) {
      return { content: [{ type: 'text', text: JSON.stringify({ error: 'target_url is required' }) }], isError: true };
    }

    let parsedElems: Array<{role: string; name: string; selector: string}>;
    try {
      parsedElems = JSON.parse(navElems);
    } catch {
      return { content: [{ type: 'text', text: JSON.stringify({ error: 'nav_elements must be valid JSON' }) }], isError: true };
    }

    let browser: Browser;
    try {
      browser = await chromium.connectOverCDP(cdpUrl, { timeout: 10000 });
    } catch (err) {
      return { content: [{ type: 'text', text: JSON.stringify({ error: `CDP connect failed: ${err}` }) }], isError: true };
    }

    const ctx = browser.contexts()[0] ?? await browser.newContext();
    const page = await ctx.newPage();
    try {
      await page.goto(targetUrl, { waitUntil: 'domcontentloaded', timeout: 30000 });
    } catch (err) {
      return { content: [{ type: 'text', text: JSON.stringify({ error: `Navigation failed: ${err}` }) }], isError: true };
    }

    await injectNavOverlay(page, parsedElems);

    return {
      content: [{ type: 'text', text: JSON.stringify({ ok: true, url: page.url(), elements: parsedElems.length }) }],
    };
  }

  if (name === 'fill_form') {
    interface FillElement {
      role: string;
      name: string;
      selector: string;
      field_key?: string;
    }

    const cdpUrl    = (args?.cdp_url      as string | undefined) ?? 'http://localhost:9222';
    const targetUrl = (args?.target_url   as string | undefined) ?? '';
    const elemsRaw  = (args?.elements     as string | undefined) ?? '[]';
    const fvRaw     = (args?.field_values as string | undefined) ?? '{}';

    if (!targetUrl) {
      return { content: [{ type: 'text', text: JSON.stringify({ error: 'target_url is required' }) }], isError: true };
    }

    let elements: FillElement[];
    let fieldValues: Record<string, string>;
    try {
      elements    = JSON.parse(elemsRaw);
      fieldValues = JSON.parse(fvRaw);
    } catch {
      return { content: [{ type: 'text', text: JSON.stringify({ error: 'elements and field_values must be valid JSON' }) }], isError: true };
    }

    const isButtonRole = (role: string) => {
      const r = role.toLowerCase();
      return r === 'button' || r === 'submit' || r === 'link' || r === 'menuitem' || r === 'menuitemcheckbox';
    };

    let browser: Browser;
    try {
      browser = await chromium.connectOverCDP(cdpUrl, { timeout: 10000 });
    } catch (err) {
      return { content: [{ type: 'text', text: JSON.stringify({ error: `CDP connect failed: ${err}` }) }], isError: true };
    }

    const ctx = browser.contexts()[0] ?? await browser.newContext();
    const page = await ctx.newPage();
    try {
      await page.goto(targetUrl, { waitUntil: 'domcontentloaded', timeout: 30000 });
    } catch (err) {
      return { content: [{ type: 'text', text: JSON.stringify({ error: `Navigation failed: ${err}` }) }], isError: true };
    }

    const inputEls  = elements.filter(e => !isButtonRole(e.role));
    const buttonEls = elements.filter(e => isButtonRole(e.role));

    let lastSelector: string | null = null;
    let filledCount = 0;

    // Fill non-button elements first
    for (const el of inputEls) {
      const fk = el.field_key ?? '';
      const value = fieldValues[fk];
      if (!fk || value === undefined || value === '') continue;
      try {
        await page.waitForSelector(el.selector, { state: 'visible', timeout: 5000 });
        await page.fill(el.selector, String(value));
        lastSelector = el.selector;
        filledCount++;
      } catch (e) {
        console.error(`[fill_form] fill "${el.selector}" failed: ${e}`);
      }
    }

    // Buttons last: click each unless field_values explicitly sets it to "false".
    // Backward compat: buttons with no field_key are always clicked.
    if (buttonEls.length > 0) {
      for (const btn of buttonEls) {
        const fk = btn.field_key ?? '';
        const flagVal = fk ? (fieldValues[fk] ?? 'true') : 'true';
        if (flagVal === 'false' || flagVal === 'False') continue;
        try {
          await page.waitForSelector(btn.selector, { state: 'visible', timeout: 3000 });
          await page.click(btn.selector);
        } catch (e) {
          console.error(`[fill_form] click "${btn.selector}" failed: ${e}`);
        }
      }
    } else if (lastSelector) {
      try {
        await page.press(lastSelector, 'Enter');
      } catch (e) {
        console.error(`[fill_form] Enter on "${lastSelector}" failed: ${e}`);
      }
    }

    return {
      content: [{ type: 'text', text: JSON.stringify({ ok: true, filled: filledCount, url: page.url() }) }],
    };
  }

  return {
    content: [{ type: 'text', text: `Unknown tool: ${name}` }],
    isError: true,
  };
});

// ---------------------------------------------------------------------------
// Web Want Inspector completion snapshot
// ---------------------------------------------------------------------------

/**
 * Saves a PNG screenshot of the page to ~/.mywant/screenshots/ — the same
 * directory the replay feature's moveReplayScreenshot (agent_playwright_record.go)
 * uses, served by the Go backend at /api/v1/screenshots/<filename>.
 * Returns the saved filename, or null on failure.
 */
async function captureAndSaveScreenshot(page: Page): Promise<string | null> {
  try {
    const buffer = await page.screenshot({ type: 'png' });
    const dir = path.join(os.homedir(), '.mywant', 'screenshots');
    fs.mkdirSync(dir, { recursive: true });
    const filename = `web-inspector-${crypto.randomUUID()}.png`;
    fs.writeFileSync(path.join(dir, filename), buffer);
    return filename;
  } catch (e) {
    console.error('[WEB-INSPECTOR] screenshot capture failed:', e);
    return null;
  }
}

// ---------------------------------------------------------------------------
// Web Want Inspector overlay injection
// ---------------------------------------------------------------------------

async function injectWebInspector(
  page: Page,
  doneWebhookUrl: string,
  suggestNameUrl: string,
  myCharacterId: string,
  myColor: string,
  myAvatar: string,
  existingMarks: Array<{role:string;name:string;selector:string;characterId:string;color:string}>,
  navElements: Array<{role:string;name:string;selector:string}>,
): Promise<void> {
  // The in-page logic itself lives in webInspectorOverlayCore.ts — shared
  // verbatim with the standalone (non-CDP) bootstrap served to Safari/iPhone
  // Chrome — see build-standalone-overlay.mjs. Passed by reference (not
  // inlined) so Playwright serializes and runs the exact same function here.
  await page.evaluate(webInspectorOverlayCore, { webhookUrl: doneWebhookUrl, suggestNameUrl, myCharacterId, myColor, myAvatar, existingMarks, navElements });
}

// ---------------------------------------------------------------------------
// Debug recorder injection
// ---------------------------------------------------------------------------

async function injectDebugRecorder(page: Page): Promise<void> {
  await page.evaluate(() => {
    if ((window as any).__playwright_recorder__) return;

    const actions: string[] = [];
    (window as any).__playwright_recorder__ = { actions };

    function getSelector(el: Element): string | null {
      if (!el) return null;
      if (el.id) return `#${CSS.escape(el.id)}`;
      const tag = el.tagName.toLowerCase();
      // data-testid
      const testId = el.getAttribute('data-testid');
      if (testId) return `[data-testid="${testId}"]`;
      // name attribute for inputs
      const name = el.getAttribute('name');
      if (name && (tag === 'input' || tag === 'select' || tag === 'textarea')) {
        return `${tag}[name="${name}"]`;
      }
      return null;
    }

    // Track mouse drag for text selection recording
    let __mouseDownX = 0, __mouseDownY = 0, __isDragSelect = false;

    document.addEventListener('mousedown', (e: MouseEvent) => {
      __mouseDownX = Math.round(e.clientX);
      __mouseDownY = Math.round(e.clientY);
      __isDragSelect = false;
    }, true);

    document.addEventListener('mouseup', (e: MouseEvent) => {
      const dx = Math.abs(Math.round(e.clientX) - __mouseDownX);
      const dy = Math.abs(Math.round(e.clientY) - __mouseDownY);
      if (dx > 5 || dy > 5) {
        const sel = window.getSelection();
        if (sel && sel.toString().trim()) {
          __isDragSelect = true;
          const ex = Math.round(e.clientX), ey = Math.round(e.clientY);
          actions.push(`  await page.mouse.move(${__mouseDownX}, ${__mouseDownY});`);
          actions.push(`  await page.mouse.down();`);
          actions.push(`  await page.mouse.move(${ex}, ${ey});`);
          actions.push(`  await page.mouse.up();`);
        }
      }
    }, true);

    document.addEventListener('click', (e: MouseEvent) => {
      if (__isDragSelect) { __isDragSelect = false; return; }
      const target = e.target as Element;
      const sel = getSelector(target);
      if (sel) {
        actions.push(`  await page.click(${JSON.stringify(sel)});`);
      } else {
        actions.push(`  await page.mouse.click(${Math.round(e.clientX)}, ${Math.round(e.clientY)});`);
      }
    }, true);

    document.addEventListener('change', (e: Event) => {
      const target = e.target as HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement;
      if (!target || !target.tagName) return;
      const tag = target.tagName.toLowerCase();
      if (tag !== 'input' && tag !== 'select' && tag !== 'textarea') return;
      const sel = getSelector(target);
      if (!sel) return;
      if (tag === 'select') {
        actions.push(`  await page.selectOption(${JSON.stringify(sel)}, ${JSON.stringify(target.value)});`);
      } else if ((target as HTMLInputElement).type === 'checkbox' || (target as HTMLInputElement).type === 'radio') {
        if ((target as HTMLInputElement).checked) {
          actions.push(`  await page.check(${JSON.stringify(sel)});`);
        } else {
          actions.push(`  await page.uncheck(${JSON.stringify(sel)});`);
        }
      } else {
        actions.push(`  await page.fill(${JSON.stringify(sel)}, ${JSON.stringify(target.value)});`);
      }
    }, true);
  });
}

// ---------------------------------------------------------------------------
// Nav overlay: navigate only pre-saved elements (no full-page scan)
// ---------------------------------------------------------------------------

async function injectNavOverlay(
  page: Page,
  elements: Array<{ role: string; name: string; selector: string }>,
): Promise<void> {
  await page.evaluate((navElements: Array<{ role: string; name: string; selector: string }>) => {
    if ((window as any).__mywantNavLoaded) return;
    (window as any).__mywantNavLoaded = true;

    // ── Styles ────────────────────────────────────────────────────────────
    const style = document.createElement('style');
    style.textContent = `
      .mwn-cursor { position:fixed; pointer-events:none; z-index:2147483647;
        transition:left .15s cubic-bezier(0.34,1.56,0.64,1),top .15s cubic-bezier(0.34,1.56,0.64,1); }
      .mwn-focused { outline:3px solid #f59e0b !important; box-shadow:0 0 0 4px rgba(245,158,11,.25) !important; }
      .mwn-panel {
        position:fixed; bottom:12px; right:12px; background:rgba(15,23,42,.97);
        border:1px solid #334155; border-radius:10px; padding:10px 14px;
        z-index:2147483646; color:#f1f5f9; font:13px/1.4 system-ui,sans-serif;
        min-width:180px; box-shadow:0 8px 32px rgba(0,0,0,.5);
      }
      .mwn-name { font-weight:700; color:#f59e0b; font-size:14px; }
      .mwn-role { font-size:10px; font-weight:700; padding:1px 6px; border-radius:4px; margin-left:4px; }
      .mwn-role-textbox  { background:#3b82f6; color:#fff; }
      .mwn-role-button   { background:#8b5cf6; color:#fff; }
      .mwn-role-link     { background:#10b981; color:#fff; }
      .mwn-role-other    { background:#475569; color:#fff; }
      .mwn-hint { font-size:10px; color:#64748b; margin-top:6px; }
      .mwn-pos  { font-size:11px; color:#94a3b8; margin-top:4px; }
    `;
    document.head.appendChild(style);

    // ── CursorMan SVG ─────────────────────────────────────────────────────
    const cursor = document.createElement('div');
    cursor.className = 'mwn-cursor';
    cursor.innerHTML = '<svg width="28" height="34" viewBox="0 0 24 29" fill="none" xmlns="http://www.w3.org/2000/svg" style="filter:drop-shadow(0 0 6px #f59e0b) drop-shadow(0 0 12px #d97706)"><circle cx="12" cy="5" r="3.5" fill="#f59e0b"/><rect x="8.5" y="9.5" width="7" height="7" rx="1" fill="#f59e0b"/><line x1="8.5" y1="11" x2="5" y2="15" stroke="#f59e0b" stroke-width="2.5" stroke-linecap="round"/><line x1="15.5" y1="11" x2="19" y2="15" stroke="#f59e0b" stroke-width="2.5" stroke-linecap="round"/><line x1="10" y1="16.5" x2="8" y2="22" stroke="#f59e0b" stroke-width="2.5" stroke-linecap="round"/><line x1="14" y1="16.5" x2="16" y2="22" stroke="#f59e0b" stroke-width="2.5" stroke-linecap="round"/><ellipse cx="12" cy="23.5" rx="4" ry="1.5" fill="rgba(0,0,0,0.25)"/></svg>';
    document.body.appendChild(cursor);

    // ── Panel ─────────────────────────────────────────────────────────────
    const panel = document.createElement('div');
    panel.className = 'mwn-panel';
    document.body.appendChild(panel);

    // ── State ─────────────────────────────────────────────────────────────
    let idx = 0;
    let cursorX = window.innerWidth / 2;
    let cursorY = window.innerHeight / 2;

    const roleClass = (role: string) => {
      const r = role.toLowerCase();
      if (r.includes('text') || r.includes('input') || r.includes('search')) return 'mwn-role-textbox';
      if (r.includes('button') || r.includes('submit')) return 'mwn-role-button';
      if (r.includes('link')) return 'mwn-role-link';
      return 'mwn-role-other';
    };

    const renderPanel = (el: { role: string; name: string } | null) => {
      if (!el) { panel.innerHTML = '<div style="color:#ef4444">要素が見つかりません</div>'; return; }
      const rClass = roleClass(el.role);
      panel.innerHTML = `
        <div style="display:flex;align-items:center;gap:4px">
          <span class="mwn-name">${el.name}</span>
          <span class="mwn-role ${rClass}">${el.role}</span>
        </div>
        <div class="mwn-pos">${idx + 1} / ${navElements.length}</div>
        <div class="mwn-hint">↑↓:移動 &nbsp;🎮:D-pad移動</div>
      `;
    };

    const setFocus = (i: number) => {
      // Remove previous focus
      navElements.forEach(e => {
        const el = document.querySelector(e.selector);
        el?.classList.remove('mwn-focused');
      });

      const def = navElements[i];
      if (!def) return;
      const el = document.querySelector(def.selector) as HTMLElement | null;
      if (!el) { renderPanel(def); return; }

      el.classList.add('mwn-focused');
      el.scrollIntoView({ block: 'nearest', behavior: 'instant' });
      requestAnimationFrame(() => {
        const r = el.getBoundingClientRect();
        cursorX = Math.max(4, Math.min(r.left + r.width / 2 - 14, window.innerWidth - 36));
        cursorY = Math.max(4, Math.min(r.top - 38, window.innerHeight - 40));
        cursor.style.left = cursorX + 'px';
        cursor.style.top  = cursorY + 'px';
      });
      renderPanel(def);
    };

    // ── Keyboard (capture phase) ──────────────────────────────────────────
    document.addEventListener('keydown', (e: KeyboardEvent) => {
      if (!['ArrowDown','ArrowRight','ArrowUp','ArrowLeft'].includes(e.key)) return;
      e.preventDefault(); e.stopPropagation();
      if (e.key === 'ArrowDown' || e.key === 'ArrowRight') {
        idx = (idx + 1) % navElements.length; setFocus(idx);
      } else {
        idx = (idx - 1 + navElements.length) % navElements.length; setFocus(idx);
      }
    }, true);

    // ── Gamepad ───────────────────────────────────────────────────────────
    const DEADZONE = 0.15;
    const CURSOR_SPD = 10;
    let prevBtns: boolean[] = [];

    const gpoll = () => {
      for (const gp of (navigator as any).getGamepads?.() ?? []) {
        if (!gp) continue;
        const cur = gp.buttons.map((b: GamepadButton) => b.pressed);
        // Left stick: free cursor move
        const lx = Math.abs(gp.axes[0] ?? 0) > DEADZONE ? (gp.axes[0] ?? 0) : 0;
        const ly = Math.abs(gp.axes[1] ?? 0) > DEADZONE ? (gp.axes[1] ?? 0) : 0;
        if (lx !== 0 || ly !== 0) {
          cursorX = Math.max(0, Math.min(window.innerWidth - 28, cursorX + lx * CURSOR_SPD));
          cursorY = Math.max(0, Math.min(window.innerHeight - 34, cursorY + ly * CURSOR_SPD));
          cursor.style.transition = 'none';
          cursor.style.left = cursorX + 'px';
          cursor.style.top  = cursorY + 'px';
        } else {
          cursor.style.transition = 'left .15s cubic-bezier(0.34,1.56,0.64,1),top .15s cubic-bezier(0.34,1.56,0.64,1)';
        }
        // D-pad: cycle elements
        if ((cur[13] && !prevBtns[13]) || (cur[15] && !prevBtns[15])) {
          idx = (idx + 1) % navElements.length; setFocus(idx);
        }
        if ((cur[12] && !prevBtns[12]) || (cur[14] && !prevBtns[14])) {
          idx = (idx - 1 + navElements.length) % navElements.length; setFocus(idx);
        }
        prevBtns = cur;
      }
      requestAnimationFrame(gpoll);
    };
    requestAnimationFrame(gpoll);

    // ── Init ──────────────────────────────────────────────────────────────
    setFocus(0);
  }, elements);
}

// ---------------------------------------------------------------------------
// Start stdio transport
// ---------------------------------------------------------------------------

async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
}

main().catch((err) => {
  console.error('[playwright-mcp-app] Fatal error:', err);
  process.exit(1);
});
