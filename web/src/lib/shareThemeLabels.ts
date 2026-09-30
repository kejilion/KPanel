// Vocabulary the host hands to protocol-2 themes, so a theme author never has
// to ship or maintain translations for the facts KPanel already knows how to say.
export type ShareThemeLocale = 'zh-CN' | 'zh-TW' | 'en-US'

const zhCN = {
  fleet: '公开集群', total: '全部机器', online: '在线', attention: '需关注', offline: '离线', pending: '等待数据',
  degraded: '需关注', cpu: 'CPU', memory: '内存', disk: '磁盘', load: '负载', uptime: '运行时间', cores: '核心',
  download: '下行', upload: '上行', received: '已接收', sent: '已发送', monthly: '月流量', cumulative: '累计流量',
  expiry: '到期', price: '价格', remaining: '剩余价值（估算）', valueHint: '按已公开价格及到期日估算，分币种展示。',
  covered: '台已计入', excluded: '台资料不足', noValue: '资料不足，暂无法估算', search: '搜索名称、地区或系统',
  empty: '没有匹配的机器', updated: '数据生成于', all: '全部', server: '机器', location: '地区', system: '系统',
  card: '卡片', list: '列表', map: '地图', view: '视图', details: '详情', network: '网络', traffic: '流量', unknownPlace: '位置未知', health: '在线率', regions: '个地区', clear: '清除筛选',
} as const
const zhTW: Record<keyof typeof zhCN, string> = {
  ...zhCN, fleet: '公開集群', total: '全部機器', online: '在線', attention: '需關注', offline: '離線', pending: '等待資料',
  degraded: '需關注', memory: '記憶體', disk: '磁碟', cores: '核心', download: '下行', upload: '上行', received: '已接收',
  sent: '已傳送', monthly: '月流量', cumulative: '累計流量', expiry: '到期', price: '價格', remaining: '剩餘價值（估算）',
  valueHint: '按已公開價格及到期日估算，分幣種展示。', covered: '台已計入', excluded: '台資料不足', noValue: '資料不足，暫時無法估算',
  search: '搜尋名稱、地區或系統', empty: '沒有符合的機器', updated: '資料產生於', all: '全部', server: '機器', location: '地區', system: '系統',
  card: '卡片', list: '列表', map: '地圖', view: '檢視', details: '詳情', network: '網路', traffic: '流量', unknownPlace: '位置未知', health: '在線率', regions: '個地區', clear: '清除篩選',
}
const enUS: Record<keyof typeof zhCN, string> = {
  fleet: 'Public fleet', total: 'Servers', online: 'Online', attention: 'Attention', offline: 'Offline', pending: 'Awaiting data',
  degraded: 'Attention', cpu: 'CPU', memory: 'Memory', disk: 'Disk', load: 'Load', uptime: 'Uptime', cores: 'cores',
  download: 'Down', upload: 'Up', received: 'Received', sent: 'Sent', monthly: 'Monthly traffic', cumulative: 'Total traffic',
  expiry: 'Expires', price: 'Price', remaining: 'Remaining value (estimate)', valueHint: 'Estimated from public price and expiry, grouped by currency.',
  covered: 'included', excluded: 'incomplete', noValue: 'Not enough details to estimate', search: 'Search name, location or OS',
  empty: 'No matching servers', updated: 'Updated', all: 'All', server: 'Server', location: 'Location', system: 'System',
  card: 'Cards', list: 'List', map: 'Map', view: 'View', details: 'Details', network: 'Network', traffic: 'Traffic', unknownPlace: 'Unknown location', health: 'Online rate', regions: 'regions', clear: 'Clear filter',
}
const tables: Record<ShareThemeLocale, Record<string, string>> = { 'zh-CN': zhCN, 'zh-TW': zhTW, 'en-US': enUS }

export function shareThemeLabels(locale: string): Record<string, string> {
  return { ...(tables[locale as ShareThemeLocale] || tables['en-US']) }
}
