import type { PhraseCatalog } from '@/i18n/phrase'

export default [
  ['浏览器', 'Browser'],
  ['正在建立浏览会话…', 'Starting browsing session...'],
  ['需要先设置浏览器专用域名', 'A browser-only hostname is required'],
  [
    '浏览器功能必须运行在和面板不同的域名上。被浏览的网页会在它所在的域名下执行，和面板同域时它就能读到面板的登录状态并调用面板接口，因此这一项没填就不放行。',
    'The browser must run on a hostname other than the panel’s. A browsed page executes under whatever hostname serves it, so sharing one with the panel would let that page read your session and call the panel API — which is why this stays blocked until the hostname is set.',
  ],
  [
    '在“设置 → 面板访问域名”右侧填入一个专用域名（例如 browse.example.com）， 并把它解析到这台服务器。填好保存后本窗口会自动进入，不用关掉重开。',
    'Set a dedicated hostname (for example browse.example.com) on the right-hand side of Settings → Panel hostnames and point it at this server. This window picks it up on its own once you save — no need to close and reopen it.',
  ],
  ['立即重试', 'Retry now'],
  ['前往设置', 'Open settings'],
  ['无法建立浏览会话', 'Could not start a browsing session'],
  ['重试', 'Retry'],
] as const satisfies PhraseCatalog
