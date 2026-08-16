// Registered before the vendor bundles load: a top-level failure in
// scramjet.js/controller.api.js (or anywhere in app.js) would otherwise
// leave the page silently frozen on "正在加载可用节点…" with nothing but a
// browser-console error to explain it. This surfaces that failure directly
// in the page instead. Loaded via <script src>, not inline — the site-wide
// CSP is script-src 'self' with no 'unsafe-inline', so an inline block here
// would itself be silently blocked (exactly the bug this file exists to
// catch had it stayed inline).
function reportBrowserAppError(message) {
  var target = document.getElementById('host-list') || document.getElementById('status');
  if (target) {
    target.innerHTML = '<div id="host-list-empty" style="color:#c5221f">加载失败：' +
      String(message).replace(/[&<>]/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]; }) +
      '</div>';
  }
}
window.addEventListener('error', function (e) {
  reportBrowserAppError((e && e.message) || '未知脚本错误');
});
window.addEventListener('unhandledrejection', function (e) {
  var reason = e && e.reason;
  reportBrowserAppError((reason && (reason.message || String(reason))) || '未知 Promise 错误');
});
