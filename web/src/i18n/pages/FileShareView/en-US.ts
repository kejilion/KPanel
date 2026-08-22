import type { PhraseCatalog } from '@/i18n/phrase'

export default [
  ['有效期至 {0}', 'Valid until {0}'],
  ['永久有效', 'Never expires'],
  ['当前下载较多，请稍后重试。', 'Downloads are busy right now. Try again shortly.'],
  ['链接无效、已过期，或文件已发生变化。', 'The link is invalid or expired, or the file has changed.'],
  ['暂时无法读取分享文件，请检查网络后重试。', 'Unable to load the shared file. Check your connection and try again.'],
  ['分享链接格式无效。', 'The share link format is invalid.'],
  ['正在读取分享文件…', 'Loading the shared file…'],
  ['请稍候，这通常只需要几秒。', 'Please wait. This usually takes only a few seconds.'],
  ['无法打开分享文件', 'Unable to open the shared file'],
  ['KPanel 文件分享', 'KPanel file sharing'],
  ['文件发生修改、移动或删除后，此链接将无法访问。', 'If the file is modified, moved, or deleted, this link becomes inaccessible.'],
  ['公开页不会显示服务器路径或管理信息', 'This public page never shows server paths or management details'],
  ['切换浅色模式', 'Switch to light mode'],
  ['切换深色模式', 'Switch to dark mode'],
  ['重试', 'Retry'],
  ['未知文件类型', 'Unknown file type'],
  ['下载文件', 'Download file'],
  ['在浏览器中打开', 'Open in browser'],
] as const satisfies PhraseCatalog
