"use strict";
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
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || (function () {
    var ownKeys = function(o) {
        ownKeys = Object.getOwnPropertyNames || function (o) {
            var ar = [];
            for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) ar[ar.length] = k;
            return ar;
        };
        return ownKeys(o);
    };
    return function (mod) {
        if (mod && mod.__esModule) return mod;
        var result = {};
        if (mod != null) for (var k = ownKeys(mod), i = 0; i < k.length; i++) if (k[i] !== "default") __createBinding(result, mod, k[i]);
        __setModuleDefault(result, mod);
        return result;
    };
})();
Object.defineProperty(exports, "__esModule", { value: true });
const index_js_1 = require("@modelcontextprotocol/sdk/server/index.js");
const stdio_js_1 = require("@modelcontextprotocol/sdk/server/stdio.js");
const types_js_1 = require("@modelcontextprotocol/sdk/types.js");
const playwright_1 = require("playwright");
const ws_1 = require("ws");
const http = __importStar(require("http"));
const crypto = __importStar(require("crypto"));
const sessions = new Map();
const debugSessions = new Map();
const replaySessions = new Map();
// ---------------------------------------------------------------------------
// HTTP + WebSocket server for the browser UI
// ---------------------------------------------------------------------------
const WS_PORT = parseInt(process.env.PLAYWRIGHT_WS_PORT ?? '9321', 10);
const HTTP_PORT = parseInt(process.env.PLAYWRIGHT_HTTP_PORT ?? '9322', 10);
const wss = new ws_1.WebSocketServer({ port: WS_PORT });
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
            }
            else if (msg.type === 'type' && typeof msg.text === 'string') {
                await session.page.keyboard.type(msg.text);
                session.actions.push(`  await page.keyboard.type(${JSON.stringify(msg.text)});`);
            }
            else if (msg.type === 'navigate' && typeof msg.url === 'string') {
                await session.page.goto(msg.url);
                session.actions.push(`  await page.goto(${JSON.stringify(msg.url)});`);
            }
        }
        catch {
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
function startScreenshotStream(session) {
    session.screenshotTimer = setInterval(async () => {
        if (session.clients.size === 0)
            return;
        try {
            const buf = await session.page.screenshot({ type: 'png' });
            const b64 = buf.toString('base64');
            const msg = JSON.stringify({ type: 'screenshot', data: b64 });
            for (const client of session.clients) {
                if (client.readyState === ws_1.WebSocket.OPEN) {
                    client.send(msg);
                }
            }
        }
        catch {
            // Page may be navigating
        }
    }, 200);
}
function startReplayScreenshotStream(session) {
    session.screenshotTimer = setInterval(async () => {
        if (session.clients.size === 0)
            return;
        try {
            const buf = await session.page.screenshot({ type: 'png' });
            const b64 = buf.toString('base64');
            const msg = JSON.stringify({ type: 'screenshot', data: b64 });
            for (const client of session.clients) {
                if (client.readyState === ws_1.WebSocket.OPEN)
                    client.send(msg);
            }
        }
        catch { }
    }, 200);
}
// ---------------------------------------------------------------------------
// Playwright codegen script builder
// ---------------------------------------------------------------------------
function buildPlaywrightScript(targetUrl, actions) {
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
function buildUIHtml(sessionId, wsPort) {
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
const server = new index_js_1.Server({ name: 'playwright-mcp-app', version: '1.0.0' }, { capabilities: { tools: {} } });
server.setRequestHandler(types_js_1.ListToolsRequestSchema, async () => ({
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
    ],
}));
server.setRequestHandler(types_js_1.CallToolRequestSchema, async (request) => {
    const { name, arguments: args } = request.params;
    if (name === 'start_recording') {
        const targetUrl = args?.target_url ?? 'https://example.com';
        const sessionId = crypto.randomUUID();
        const browser = await playwright_1.chromium.launch({ headless: true });
        const context = await browser.newContext({
            viewport: { width: 1280, height: 800 },
            recordVideo: undefined,
        });
        const page = await context.newPage();
        // Track navigation actions
        const actions = [];
        page.on('framenavigated', (frame) => {
            if (frame === page.mainFrame()) {
                actions.push(`  await page.goto(${JSON.stringify(frame.url())});`);
            }
        });
        await page.goto(targetUrl);
        const session = {
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
        const sessionId = args?.session_id ?? '';
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
        const cdpUrl = args?.cdp_url ?? 'http://localhost:9222';
        const targetUrl = args?.target_url ?? '';
        const sessionId = crypto.randomUUID();
        let browser;
        try {
            browser = await playwright_1.chromium.connectOverCDP(cdpUrl, { timeout: 10000 });
        }
        catch (err) {
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
        const session = {
            sessionId,
            browser,
            page,
            startUrl,
            allActions: [],
            pollTimer: null,
            lastActionIndex: 0,
        };
        const pollTimer = setInterval(async () => {
            try {
                // Harvest new actions since last poll
                const currentActions = await page.evaluate(() => window.__playwright_recorder__?.actions ?? []);
                if (currentActions.length > session.lastActionIndex) {
                    const newActions = currentActions.slice(session.lastActionIndex);
                    session.allActions.push(...newActions);
                    session.lastActionIndex = currentActions.length;
                }
                // Re-inject recorder (handles post-navigation page context reset)
                await injectDebugRecorder(page);
            }
            catch {
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
        const sessionId = args?.session_id ?? '';
        const session = debugSessions.get(sessionId);
        if (!session) {
            return {
                content: [{ type: 'text', text: JSON.stringify({ error: 'debug session not found', session_id: sessionId }) }],
                isError: true,
            };
        }
        clearInterval(session.pollTimer);
        // Harvest remaining actions from the current page and capture selection/focus state
        let targetObject = {};
        try {
            const harvested = await session.page.evaluate(() => {
                const actions = window.__playwright_recorder__?.actions ?? [];
                // Capture currently selected text
                const sel = window.getSelection();
                const selectedText = sel?.toString() ?? '';
                // Capture focused / active element
                const activeEl = document.activeElement;
                let activeElement = null;
                if (activeEl && activeEl !== document.body && activeEl !== document.documentElement) {
                    const el = activeEl;
                    activeElement = {
                        tag: el.tagName.toLowerCase(),
                        id: el.id || null,
                        name: el.name || null,
                        text: el.textContent?.trim().substring(0, 500) || null,
                        value: el.value ?? null,
                        href: el.href || null,
                        src: el.src || null,
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
        }
        catch {
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
        const startUrl = args?.start_url ?? 'https://example.com';
        const actions = args?.actions ?? [];
        const sessionId = crypto.randomUUID();
        const browser = await playwright_1.chromium.launch({ headless: false });
        const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
        const page = await context.newPage();
        const replaySession = {
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
                    if (!trimmed || trimmed.startsWith('//'))
                        continue;
                    // eslint-disable-next-line no-new-func
                    const fn = new Function('page', `return (async () => { ${trimmed} })()`);
                    await fn(page);
                }
                const result = await page.evaluate(() => {
                    const sel = window.getSelection();
                    const activeEl = document.activeElement;
                    let activeElement = null;
                    if (activeEl && activeEl !== document.body && activeEl !== document.documentElement) {
                        const el = activeEl;
                        activeElement = {
                            tag: el.tagName.toLowerCase(),
                            value: el.value ?? null,
                            text: el.textContent?.trim().substring(0, 500) ?? null,
                        };
                    }
                    return {
                        selected_text: sel?.toString() ?? '',
                        active_element: activeElement,
                        url: window.location.href,
                    };
                });
                replaySession.result = result;
            }
            catch (err) {
                replaySession.error = String(err);
            }
            finally {
                replaySession.done = true;
            }
        })();
        return {
            content: [{ type: 'text', text: JSON.stringify({ session_id: sessionId, ui_url: uiUrl }) }],
        };
    }
    if (name === 'check_replay') {
        const sessionId = args?.session_id ?? '';
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
        if (replaySession.screenshotTimer)
            clearInterval(replaySession.screenshotTimer);
        for (const client of replaySession.clients)
            client.close();
        await replaySession.context.close();
        await replaySession.browser.close();
        replaySessions.delete(sessionId);
        return {
            content: [{ type: 'text', text: JSON.stringify({ done: true, result: replaySession.result, error: replaySession.error }) }],
        };
    }
    return {
        content: [{ type: 'text', text: `Unknown tool: ${name}` }],
        isError: true,
    };
});
// ---------------------------------------------------------------------------
// Debug recorder injection
// ---------------------------------------------------------------------------
async function injectDebugRecorder(page) {
    await page.evaluate(() => {
        if (window.__playwright_recorder__)
            return;
        const actions = [];
        window.__playwright_recorder__ = { actions };
        function getSelector(el) {
            if (!el)
                return null;
            if (el.id)
                return `#${CSS.escape(el.id)}`;
            const tag = el.tagName.toLowerCase();
            // data-testid
            const testId = el.getAttribute('data-testid');
            if (testId)
                return `[data-testid="${testId}"]`;
            // name attribute for inputs
            const name = el.getAttribute('name');
            if (name && (tag === 'input' || tag === 'select' || tag === 'textarea')) {
                return `${tag}[name="${name}"]`;
            }
            return null;
        }
        // Track mouse drag for text selection recording
        let __mouseDownX = 0, __mouseDownY = 0, __isDragSelect = false;
        document.addEventListener('mousedown', (e) => {
            __mouseDownX = Math.round(e.clientX);
            __mouseDownY = Math.round(e.clientY);
            __isDragSelect = false;
        }, true);
        document.addEventListener('mouseup', (e) => {
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
        document.addEventListener('click', (e) => {
            if (__isDragSelect) {
                __isDragSelect = false;
                return;
            }
            const target = e.target;
            const sel = getSelector(target);
            if (sel) {
                actions.push(`  await page.click(${JSON.stringify(sel)});`);
            }
            else {
                actions.push(`  await page.mouse.click(${Math.round(e.clientX)}, ${Math.round(e.clientY)});`);
            }
        }, true);
        document.addEventListener('change', (e) => {
            const target = e.target;
            if (!target || !target.tagName)
                return;
            const tag = target.tagName.toLowerCase();
            if (tag !== 'input' && tag !== 'select' && tag !== 'textarea')
                return;
            const sel = getSelector(target);
            if (!sel)
                return;
            if (tag === 'select') {
                actions.push(`  await page.selectOption(${JSON.stringify(sel)}, ${JSON.stringify(target.value)});`);
            }
            else if (target.type === 'checkbox' || target.type === 'radio') {
                if (target.checked) {
                    actions.push(`  await page.check(${JSON.stringify(sel)});`);
                }
                else {
                    actions.push(`  await page.uncheck(${JSON.stringify(sel)});`);
                }
            }
            else {
                actions.push(`  await page.fill(${JSON.stringify(sel)}, ${JSON.stringify(target.value)});`);
            }
        }, true);
    });
}
// ---------------------------------------------------------------------------
// Start stdio transport
// ---------------------------------------------------------------------------
async function main() {
    const transport = new stdio_js_1.StdioServerTransport();
    await server.connect(transport);
}
main().catch((err) => {
    console.error('[playwright-mcp-app] Fatal error:', err);
    process.exit(1);
});
//# sourceMappingURL=server.js.map