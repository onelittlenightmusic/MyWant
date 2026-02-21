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
export {};
//# sourceMappingURL=server.d.ts.map