// Default mywant server origin, used until the user sets a different one via
// the options page (options.html/options.js), which persists it to
// chrome.storage.local under "mywantApiOrigin". manifest.json's
// host_permissions already covers http(s)://*/* statically — required
// because the auto-launch/nav-launch polls below open/inject tabs with no
// user gesture, so activeTab and chrome.permissions.request() (which needs
// one) can't help here — so this works for any origin with no setup.
var DEFAULT_MYWANT_API_ORIGIN = 'http://localhost:8080';

chrome.action.onClicked.addListener(function (tab) {
  if (!tab.id) return;
  chrome.scripting.executeScript({ target: { tabId: tab.id }, files: ['content.js'] });
});

// Opens url in a new tab, then calls inject(tabId) once it finishes loading.
// Shared by all three pollForPendingAction branches (handleAutoLaunch,
// handleNavLaunch, handleBrowserRun below) — they all need the exact same
// "wait for the tab, then run some injection" shape, just with a different
// injection step. active defaults to true (foreground tab, matching prior
// behavior); handleBrowserRun passes false for claims marked `background`
// (e.g. claude_info's 60s gauge poll) so a run doesn't steal focus every tick.
function openTabAndRun(url, inject, active) {
  chrome.tabs.create({ url: url, active: active !== false }, function (tab) {
    if (!tab || !tab.id) return;
    var tabId = tab.id;
    var done = false;
    function run() {
      if (done) return;
      done = true;
      chrome.tabs.onUpdated.removeListener(onUpdated);
      inject(tabId);
    }
    function onUpdated(updatedTabId, info) {
      if (updatedTabId !== tabId || info.status !== 'complete') return;
      run();
    }
    chrome.tabs.onUpdated.addListener(onUpdated);
    // Race guard: fast-loading pages (e.g. example.com) can reach status
    // "complete" before the listener above is registered — the async hops
    // leading up to this call (storage.local.get, fetch, .then chains) all
    // eat into that window. Re-check the tab's current status immediately
    // so an already-missed "complete" transition still triggers inject().
    chrome.tabs.get(tabId, function (t) {
      if (t && t.status === 'complete') run();
    });
  });
}

// ---------------------------------------------------------------------------
// Pending-action polling: a single alarm-driven poll against GET
// /api/v1/web-wants/pending-action, dispatched by response.kind to one of
// the three handlers below. This used to be three separate polls (one fetch
// each to pending-auto-launch/pending-nav-launch/pending-browser-run, same
// alarm tick) — collapsed into one poll/one endpoint since the lifecycles
// they each drive only diverge *after* the claim is dequeued, not in how
// it's polled for. See handlers_web_wants.go's pendingBrowserAction for the
// server-side counterpart (still three internally distinct claim sources —
// want-flag for auto-launch, FIFO queues for the other two — just merged
// into one response envelope).
//
// chrome.alarms is the only MV3-safe way to run recurring background work —
// a plain setInterval would die whenever this service worker is suspended
// (usually within ~30s of inactivity) — but its minimum period is 1 minute,
// so there's up to ~1min latency between a claim being queued and this
// picking it up.
var PENDING_ACTION_ALARM = 'mywant-pending-action-poll';

chrome.runtime.onInstalled.addListener(function () {
  chrome.alarms.create(PENDING_ACTION_ALARM, { periodInMinutes: 1 });
});
chrome.runtime.onStartup.addListener(function () {
  chrome.alarms.create(PENDING_ACTION_ALARM, { periodInMinutes: 1 });
});

chrome.alarms.onAlarm.addListener(function (alarm) {
  if (alarm.name !== PENDING_ACTION_ALARM) return;
  pollForPendingAction();
});

function withMywantOrigin(fn) {
  chrome.storage.local.get('mywantApiOrigin', function (data) {
    fn(data.mywantApiOrigin || DEFAULT_MYWANT_API_ORIGIN);
  });
}

function pollForPendingAction() {
  withMywantOrigin(function (origin) {
    drainPendingActions(origin);
  });
}

// Dequeues and dispatches claims one at a time (each pending-action GET is a
// cheap FIFO pop — see pendingBrowserAction in handlers_web_wants.go), but
// does NOT await a claim's own handler before dequeuing the next one — each
// handler opens its own tab and runs independently, so multiple queued
// claims (e.g. smartgolf-list's 3-store parallel fetch, each its own
// browser_run call) execute concurrently instead of being throttled to one
// per 1-minute alarm tick. Recursion bottoms out as soon as the queue is
// empty ({kind: ""}).
function drainPendingActions(origin) {
  fetch(origin + '/api/v1/web-wants/pending-action')
    .then(function (res) { return res.ok ? res.json() : null; })
    .then(function (action) {
      if (!action || !action.kind) return;
      if (action.kind === 'auto_launch') handleAutoLaunch(action.auto_launch);
      else if (action.kind === 'nav_launch') handleNavLaunch(action.nav_launch);
      else if (action.kind === 'browser_run') handleBrowserRun(action.browser_run);
      drainPendingActions(origin);
    })
    .catch(function () { /* server unreachable — retry on the next alarm tick */ });
}

// Opens a tab for a web_inspector want created with launch_mode=manual,
// instead of requiring the user to navigate there and click the toolbar
// icon themselves. This is the CDP-free equivalent of
// engine/types/agent_web_inspector.go's openInspectorTab — the server never
// drives a browser directly here, it only tells us (via the claim) which
// want is waiting so exactly one tab gets opened for it.
function handleAutoLaunch(want) {
  if (!want || !want.target_url) return;
  openTabAndRun(want.target_url, function (tabId) {
    // content.js re-derives everything it needs (webhook URL, marks, nav
    // elements) from GET active-inspection?url=<this tab's URL> — it
    // doesn't need anything from the pending-action response.
    chrome.scripting.executeScript({ target: { tabId: tabId }, files: ['content.js'] })
      .catch(function (err) {
        console.error('[mywant] auto-launch inject failed for tab', tabId, err);
      });
  });
}

// The CDP-free equivalent of engine/types/web_navigator.go's
// OpenWebNavigator/FillAndSubmitForm — covers both the GUI's "Inspect"
// action (WebWantPage.tsx's handleInspect → POST /web-wants/{name}/launch
// with no navigate_only/field_values → read-only nav-highlight overlay) and
// a web want deployed as a want card having its plan fields filled in and
// approved (agent_web_form_monitor.go's webFormMonitorSubmit → same
// endpoint, with field_values → fills + submits instead). Neither ever
// creates a want, so neither can reuse the auto-launch claim; distinguished
// by whether field_values is present. Injected via executeScript's
// func+args (elements/values passed directly, no round-trip fetch needed)
// rather than a separate content.js file, since both overlay functions
// below are small and self-contained.
function handleNavLaunch(claim) {
  if (!claim || !claim.target_url) return;
  var elements = claim.nav_elements || [];
  var fieldValues = claim.field_values || null;
  openTabAndRun(claim.target_url, function (tabId) {
    var injection = fieldValues
      ? { target: { tabId: tabId }, func: mywantFillAndSubmit, args: [elements, fieldValues] }
      : { target: { tabId: tabId }, func: mywantNavOverlay, args: [elements] };
    chrome.scripting.executeScript(injection)
      .catch(function (err) {
        console.error('[mywant] nav-launch inject failed for tab', tabId, err);
      });
  });
}

// The CDP-free replacement for ~/.mywant/custom-types plugins (gmail,
// smartgolf, ...) that used to Playwright-connect_over_cdp to an existing
// --remote-debugging-port Chrome to scrape/interact with a site.
// handlers_web_wants.go's browserRun queues a claim here and blocks
// (synchronously, from the Python caller's point of view) until
// browser-run-result arrives or it times out — see webext-src/
// browser-run-interpreter.ts (bundled into browser-run-bundle.js by
// build-webext.js) for the actual @puppeteer/replay-based step runner this
// injects and calls.
function handleBrowserRun(claim) {
  if (!claim || !claim.request_id || !claim.url) return;
  var steps = claim.steps || [];
  openTabAndRun(claim.url, function (tabId) {
    // Two-step injection: browser-run-bundle.js (a files: injection, since
    // it's an esbuild-bundled npm dependency rather than one small
    // function) defines window.MywantBrowserRun as a side effect; the
    // second call's func+args then passes `steps` as real arguments to the
    // already-loaded runBrowserSteps — both injections share the same
    // page/tab, so state persists between them within the same page load.
    //
    // world: 'MAIN' (unlike every other injection in this file) — the
    // reactChange customStep reads a target element's own
    // __reactProps$xxx expando property to reach the page's React fiber
    // directly (see browser-run-interpreter.ts). Expando properties set by
    // the page's own scripts on a DOM node are invisible from the default
    // ISOLATED world (a separate JS realm that shares the DOM's built-in
    // structure but not custom properties other scripts attached to it) —
    // confirmed empirically (Object.keys() on the element came back empty
    // from ISOLATED, non-empty from MAIN). Both injections must agree on
    // the same world since window.MywantBrowserRun (set by the first) is
    // itself a MAIN/ISOLATED-scoped global, invisible across worlds.
    chrome.scripting.executeScript({ target: { tabId: tabId }, files: ['browser-run-bundle.js'], world: 'MAIN' })
      .then(function () {
        return chrome.scripting.executeScript({
          target: { tabId: tabId },
          world: 'MAIN',
          func: function (steps) { return window.MywantBrowserRun.runBrowserSteps(steps); },
          args: [steps],
        });
      })
      .then(function (results) {
        var result = results && results[0] ? results[0].result : {};
        postBrowserRunResult({ request_id: claim.request_id, result: result });
      })
      .catch(function (err) {
        console.error('[mywant] browser-run failed for tab', tabId, err);
        postBrowserRunResult({ request_id: claim.request_id, error: String(err) });
      })
      .finally(function () {
        if (!claim.keep_open) chrome.tabs.remove(tabId).catch(function () {});
      });
  }, !claim.background);
}

function postBrowserRunResult(payload) {
  withMywantOrigin(function (origin) {
    fetch(origin + '/api/v1/web-wants/browser-run-result', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    }).catch(function (err) {
      console.error('[mywant] failed to post browser-run result', err);
    });
  });
}

// Runs inside the target page (via chrome.scripting.executeScript's func).
// Read-only element-highlight overlay for navigating a want type's saved
// elements — arrow keys / gamepad D-pad step through `elements`, no capture
// UI. Deliberately kept functionally identical to engine/types/web_navigator.go's
// BuildNavJS (same CSS classes, same panel text, same keybindings) since that
// Go-templated string is what the CDP path already injects for this same
// feature — kept as a hand-written duplicate rather than shared source
// because one side is a Go string template and the other a literal JS
// function chrome.scripting.executeScript can pass args to; keep both in
// sync by hand if this overlay's behavior ever changes.
function mywantNavOverlay(elements) {
  if (window.__mywantNavLoaded) return;
  window.__mywantNavLoaded = true;
  var style = document.createElement('style');
  style.textContent = '.mwn-focused{outline:3px solid #f59e0b!important;box-shadow:0 0 0 4px rgba(245,158,11,.25)!important;}.mwn-panel{position:fixed;bottom:12px;right:12px;background:rgba(15,23,42,.95);color:#f1f5f9;font:12px/1.4 system-ui,sans-serif;padding:8px 12px;border-radius:8px;z-index:2147483647;border:1px solid #334155;}';
  document.head.appendChild(style);
  var idx = 0;
  var find = function (sel) { try { return document.querySelector(sel); } catch (e) { return null; } };
  var panel = document.createElement('div');
  panel.className = 'mwn-panel';
  document.body.appendChild(panel);
  var setFocus = function (i) {
    elements.forEach(function (e) {
      var el = find(e.selector || e.name);
      if (el) el.classList.remove('mwn-focused');
    });
    var cur = elements[i] ? find(elements[i].selector || elements[i].name) : null;
    if (cur) { cur.classList.add('mwn-focused'); cur.scrollIntoView({ block: 'nearest' }); }
    panel.textContent = '🤖 ' + (elements[i] ? elements[i].name : '?') + ' (' + (i + 1) + '/' + elements.length + ') ↑↓:移動';
  };
  document.addEventListener('keydown', function (e) {
    if (e.key === 'ArrowDown' || e.key === 'ArrowRight') { e.preventDefault(); idx = (idx + 1) % elements.length; setFocus(idx); }
    else if (e.key === 'ArrowUp' || e.key === 'ArrowLeft') { e.preventDefault(); idx = (idx - 1 + elements.length) % elements.length; setFocus(idx); }
  }, true);
  var prev = [];
  var gp = function () {
    var pads = navigator.getGamepads ? navigator.getGamepads() : [];
    for (var i = 0; i < pads.length; i++) {
      var g = pads[i];
      if (!g) continue;
      var cur = g.buttons.map(function (b) { return b.pressed; });
      if ((cur[13] && !prev[13]) || (cur[15] && !prev[15])) { idx = (idx + 1) % elements.length; setFocus(idx); }
      if ((cur[12] && !prev[12]) || (cur[14] && !prev[14])) { idx = (idx - 1 + elements.length) % elements.length; setFocus(idx); }
      prev = cur;
    }
    requestAnimationFrame(gp);
  };
  requestAnimationFrame(gp);
  setFocus(0);
}

// Runs inside the target page (via chrome.scripting.executeScript's func).
// Fills each non-button element from fieldValues (keyed by field_key), then
// clicks buttons or presses Enter to submit — a hand-written JS port of
// mcp/playwright-app/server.ts's fill_form Playwright tool (same role
// classification, same fill-then-click-then-Enter-fallback order), since
// that one drives the page from outside via CDP/Playwright's page.fill()/
// page.click(), which has no content-script equivalent to call into.
// Waits briefly for each selector to exist, matching Playwright's
// waitForSelector — elements loaded async by the page (SPA hydration etc.)
// may not be in the DOM yet the instant this runs.
function mywantFillAndSubmit(elements, fieldValues) {
  if (window.__mywantFillLoaded) return;
  window.__mywantFillLoaded = true;

  function waitFor(selector, timeoutMs) {
    return new Promise(function (resolve) {
      var existing = document.querySelector(selector);
      if (existing) { resolve(existing); return; }
      var elapsed = 0;
      var iv = setInterval(function () {
        var el = document.querySelector(selector);
        elapsed += 100;
        if (el || elapsed >= timeoutMs) {
          clearInterval(iv);
          resolve(el);
        }
      }, 100);
    });
  }

  function isButtonRole(role) {
    var r = (role || '').toLowerCase();
    return r === 'button' || r === 'submit' || r === 'link' || r === 'menuitem' || r === 'menuitemcheckbox';
  }

  // Set .value through the native setter so frameworks that wrap onChange
  // (React etc.) still see the update — a direct el.value= assignment is
  // invisible to their synthetic event system.
  function setNativeValue(el, value) {
    var proto = el.tagName === 'TEXTAREA' ? window.HTMLTextAreaElement.prototype : window.HTMLInputElement.prototype;
    var setter = Object.getOwnPropertyDescriptor(proto, 'value');
    if (setter && setter.set) setter.set.call(el, value); else el.value = value;
    el.dispatchEvent(new Event('input', { bubbles: true }));
    el.dispatchEvent(new Event('change', { bubbles: true }));
  }

  (async function () {
    var inputEls = elements.filter(function (e) { return !isButtonRole(e.role); });
    var buttonEls = elements.filter(function (e) { return isButtonRole(e.role); });
    var lastEl = null;

    for (var i = 0; i < inputEls.length; i++) {
      var el = inputEls[i];
      var fk = el.field_key || '';
      var value = fieldValues[fk];
      if (!fk || value === undefined || value === '') continue;
      try {
        var target = await waitFor(el.selector, 5000);
        if (!target) continue;
        target.focus();
        setNativeValue(target, String(value));
        lastEl = target;
      } catch (e) {
        console.error('[mywant] fill failed for', el.selector, e);
      }
    }

    if (buttonEls.length > 0) {
      for (var j = 0; j < buttonEls.length; j++) {
        var btn = buttonEls[j];
        var bfk = btn.field_key || '';
        var flagVal = bfk ? (fieldValues[bfk] !== undefined ? fieldValues[bfk] : 'true') : 'true';
        if (flagVal === 'false' || flagVal === 'False') continue;
        try {
          var btnTarget = await waitFor(btn.selector, 3000);
          if (btnTarget) btnTarget.click();
        } catch (e) {
          console.error('[mywant] click failed for', btn.selector, e);
        }
      }
    } else if (lastEl) {
      try {
        lastEl.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', code: 'Enter', bubbles: true }));
        var form = lastEl.closest('form');
        if (form) { form.requestSubmit ? form.requestSubmit() : form.submit(); }
      } catch (e) {
        console.error('[mywant] submit failed', e);
      }
    }
  })();
}

// Relay for content.js: content-script fetches inherit the host page's CSP
// connect-src (blocked on CSP-strict sites like x.com), but this background
// service worker runs in its own chrome-extension:// origin with its own
// CSP, so it can reach the configured mywant origin freely once
// host_permissions grants it. See build-chrome-extension.js for the
// content.js side of this relay.
chrome.runtime.onMessage.addListener(function (msg, _sender, sendResponse) {
  if (msg && msg.type === 'MYWANT_API_ORIGIN') {
    withMywantOrigin(function (origin) { sendResponse({ origin: origin }); });
    return true; // keep the message channel open for the async sendResponse above
  }
  if (msg && msg.type === 'MYWANT_FETCH') {
    fetch(msg.url, msg.init)
      .then(function (res) {
        return res.text().then(function (body) {
          sendResponse({ ok: res.ok, status: res.status, statusText: res.statusText, body: body });
        });
      })
      .catch(function (err) {
        sendResponse({ error: String(err) });
      });
    return true; // keep the message channel open for the async sendResponse above
  }
});
