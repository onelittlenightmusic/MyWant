var DEFAULT_ORIGIN = 'http://localhost:8080';
var input = document.getElementById('origin');
var status = document.getElementById('status');

chrome.storage.local.get('mywantApiOrigin', function (data) {
  input.value = data.mywantApiOrigin || DEFAULT_ORIGIN;
});

function setStatus(text, ok) {
  status.textContent = text;
  status.style.color = ok ? '#16a34a' : '#dc2626';
}

document.getElementById('save').addEventListener('click', function () {
  var raw = input.value.trim();
  var origin;
  try {
    var u = new URL(raw);
    if (u.protocol !== 'http:' && u.protocol !== 'https:') throw new Error('http/httpsのみ対応です');
    origin = u.origin;
  } catch (e) {
    setStatus('不正なURLです: ' + e.message, false);
    return;
  }

  // optional_host_permissions in manifest.json covers http(s)://*/*, so this
  // is always grantable — chrome.permissions.request must run inside this
  // click handler's own call stack (a user-gesture requirement).
  chrome.permissions.request({ origins: [origin + '/*'] }, function (granted) {
    if (!granted) {
      setStatus('権限が許可されませんでした。保存していません。', false);
      return;
    }
    chrome.storage.local.set({ mywantApiOrigin: origin }, function () {
      input.value = origin;
      setStatus('保存しました: ' + origin, true);
    });
  });
});
