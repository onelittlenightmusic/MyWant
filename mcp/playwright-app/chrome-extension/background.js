// Default mywant server origin, used until the user sets a different one via
// the options page (options.html/options.js), which persists it to
// chrome.storage.local under "mywantApiOrigin". manifest.json's
// host_permissions already covers http(s)://*/* statically — required
// because the auto-launch poll below opens/injects tabs with no user
// gesture, so activeTab and chrome.permissions.request() (which needs one)
// can't help here — so this works for any origin with no setup.
var DEFAULT_MYWANT_API_ORIGIN = 'http://localhost:8080';

chrome.action.onClicked.addListener(function (tab) {
  if (!tab.id) return;
  chrome.scripting.executeScript({ target: { tabId: tab.id }, files: ['content.js'] });
});

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
  if (alarm.name === AUTO_LAUNCH_ALARM) pollForAutoLaunch();
});

function pollForAutoLaunch() {
  chrome.storage.local.get('mywantApiOrigin', function (data) {
    var origin = data.mywantApiOrigin || DEFAULT_MYWANT_API_ORIGIN;
    fetch(origin + '/api/v1/web-wants/pending-auto-launch')
      .then(function (res) { return res.ok ? res.json() : null; })
      .then(function (want) {
        // pending-auto-launch already claimed this want server-side (see
        // handlers_web_wants.go's pendingAutoLaunch) before returning it, so
        // a second poll tick will never see the same want again even if
        // opening/injecting this tab takes a while.
        if (!want || !want.target_url) return;
        chrome.tabs.create({ url: want.target_url, active: true }, function (tab) {
          if (!tab || !tab.id) return;
          var tabId = tab.id;
          var injected = false;
          function inject() {
            if (injected) return;
            injected = true;
            chrome.tabs.onUpdated.removeListener(onUpdated);
            // content.js re-derives everything it needs (webhook URL, marks,
            // nav elements) from GET active-inspection?url=<this tab's URL> —
            // it doesn't need anything from the pending-auto-launch response.
            chrome.scripting.executeScript({ target: { tabId: tabId }, files: ['content.js'] })
              .catch(function (err) {
                // Was silent before — a host-permission gap here (e.g. a
                // chrome:// target_url that can never be scripted) previously
                // meant the tab just opened with no visible sign the
                // auto-launch had failed. Logging at least makes it
                // debuggable from the service worker's console.
                console.error('[mywant] auto-launch inject failed for tab', tabId, err);
              });
          }
          function onUpdated(updatedTabId, info) {
            if (updatedTabId !== tabId || info.status !== 'complete') return;
            inject();
          }
          chrome.tabs.onUpdated.addListener(onUpdated);
          // Race guard: fast-loading pages (e.g. example.com) can reach
          // status "complete" before the listener above is registered — the
          // multiple async hops above (storage.local.get, fetch, two .then
          // chains, this callback) all eat into that window. Re-check the
          // tab's current status immediately so an already-missed
          // "complete" transition still triggers the injection.
          chrome.tabs.get(tabId, function (t) {
            if (t && t.status === 'complete') inject();
          });
        });
      })
      .catch(function () { /* server unreachable — retry on the next alarm tick */ });
  });
}

// Relay for content.js: content-script fetches inherit the host page's CSP
// connect-src (blocked on CSP-strict sites like x.com), but this background
// service worker runs in its own chrome-extension:// origin with its own
// CSP, so it can reach the configured mywant origin freely once
// host_permissions grants it. See build-chrome-extension.js for the
// content.js side of this relay.
chrome.runtime.onMessage.addListener(function (msg, _sender, sendResponse) {
  if (msg && msg.type === 'MYWANT_API_ORIGIN') {
    chrome.storage.local.get('mywantApiOrigin', function (data) {
      sendResponse({ origin: data.mywantApiOrigin || DEFAULT_MYWANT_API_ORIGIN });
    });
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
