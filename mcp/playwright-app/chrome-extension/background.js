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
// Shared by pollForAutoLaunch (web_inspector wants, launch_mode=manual) and
// pollForNavLaunch (POST /web-wants/{name}/launch's default "Inspect" mode)
// below — both need the exact same "wait for the tab, then run some
// injection" shape, just with a different injection step.
function openTabAndRun(url, inject) {
  chrome.tabs.create({ url: url, active: true }, function (tab) {
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
// Auto-launch polling: opens a new tab for a web_inspector want created with
// launch_mode=manual, instead of requiring the user to navigate there and
// click the toolbar icon themselves. This is the CDP-free equivalent of
// engine/types/agent_web_inspector.go's openInspectorTab — the server never
// drives a browser directly here, it only tells us (via the claim below)
// which want is waiting so exactly one tab gets opened for it.
//
// chrome.alarms is the only MV3-safe way to run recurring background work —
// a plain setInterval would die whenever this service worker is suspended
// (usually within ~30s of inactivity) — but its minimum period is 1 minute,
// so there's up to ~1min latency between want creation and the tab opening.
var AUTO_LAUNCH_ALARM = 'mywant-auto-launch-poll';

chrome.runtime.onInstalled.addListener(function () {
  chrome.alarms.create(AUTO_LAUNCH_ALARM, { periodInMinutes: 1 });
});
chrome.runtime.onStartup.addListener(function () {
  chrome.alarms.create(AUTO_LAUNCH_ALARM, { periodInMinutes: 1 });
});

chrome.alarms.onAlarm.addListener(function (alarm) {
  if (alarm.name !== AUTO_LAUNCH_ALARM) return;
  pollForAutoLaunch();
  pollForNavLaunch();
});

function withMywantOrigin(fn) {
  chrome.storage.local.get('mywantApiOrigin', function (data) {
    fn(data.mywantApiOrigin || DEFAULT_MYWANT_API_ORIGIN);
  });
}

function pollForAutoLaunch() {
  withMywantOrigin(function (origin) {
    fetch(origin + '/api/v1/web-wants/pending-auto-launch')
      .then(function (res) { return res.ok ? res.json() : null; })
      .then(function (want) {
        // pending-auto-launch already claimed this want server-side (see
        // handlers_web_wants.go's pendingAutoLaunch) before returning it, so
        // a second poll tick will never see the same want again even if
        // opening/injecting this tab takes a while.
        if (!want || !want.target_url) return;
        openTabAndRun(want.target_url, function (tabId) {
          // content.js re-derives everything it needs (webhook URL, marks,
          // nav elements) from GET active-inspection?url=<this tab's URL> —
          // it doesn't need anything from the pending-auto-launch response.
          chrome.scripting.executeScript({ target: { tabId: tabId }, files: ['content.js'] })
            .catch(function (err) {
              console.error('[mywant] auto-launch inject failed for tab', tabId, err);
            });
        });
      })
      .catch(function () { /* server unreachable — retry on the next alarm tick */ });
  });
}

// ---------------------------------------------------------------------------
// Nav-launch polling: the CDP-free equivalent of engine/types/web_navigator.go's
// OpenWebNavigator, for the GUI's "Inspect" action (WebWantPage.tsx's
// handleInspect → POST /web-wants/{name}/launch with no navigate_only/
// field_values) — unlike auto-launch above, this never creates a want, so it
// can't reuse pending-auto-launch; handlers_web_wants.go's launchWebWant
// queues onto the parallel pending-nav-launch endpoint instead. Injected via
// executeScript's func+args (elements passed directly, no round-trip fetch
// needed) rather than a separate content.js file, since mywantNavOverlay is
// small and self-contained.
function pollForNavLaunch() {
  withMywantOrigin(function (origin) {
    fetch(origin + '/api/v1/web-wants/pending-nav-launch')
      .then(function (res) { return res.ok ? res.json() : null; })
      .then(function (claim) {
        if (!claim || !claim.target_url) return;
        var elements = claim.nav_elements || [];
        openTabAndRun(claim.target_url, function (tabId) {
          chrome.scripting.executeScript({ target: { tabId: tabId }, func: mywantNavOverlay, args: [elements] })
            .catch(function (err) {
              console.error('[mywant] nav-launch inject failed for tab', tabId, err);
            });
        });
      })
      .catch(function () { /* server unreachable — retry on the next alarm tick */ });
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
