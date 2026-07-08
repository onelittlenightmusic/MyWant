// Default mywant server origin, used until the user sets a different one via
// the options page (options.html/options.js), which persists it to
// chrome.storage.local under "mywantApiOrigin" and requests the matching
// optional_host_permissions entry so this background worker can actually
// reach it. Also covered by manifest.json's static host_permissions, so the
// extension works out of the box with no setup for the common desktop case.
var DEFAULT_MYWANT_API_ORIGIN = 'http://localhost:8080';

chrome.action.onClicked.addListener(function (tab) {
  if (!tab.id) return;
  chrome.scripting.executeScript({ target: { tabId: tab.id }, files: ['content.js'] });
});

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
