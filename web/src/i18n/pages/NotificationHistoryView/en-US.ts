import type { PhraseCatalog } from '@/i18n/phrase'

export default [
  ['服务器到期', 'Server expiry'],
  ['面板登录', 'Panel login'],
  ['到期日期', 'Expiry date'],
  ['剩余天数', 'Days remaining'],
  ['通知记录', 'Notification history'],
  ['时间', 'Time'],
  ['事件信息', 'Event details'],
  ['查看原文', 'View original'],
  ['通知原文', 'Original notification'],
  ['阈值', 'Threshold'],
  ['到达值', 'Reached'],
  ['当前值', 'Current'],
  ['状态', 'State'],
  ['用户', 'User'],
  ['来源', 'Source'],
  ['方式', 'Method'],
  [
    "本机与集群事件默认保存在当前 KPanel，外部推送可选。",
    "Local and cluster events are saved on this KPanel by default. External delivery is optional."
  ],
  [
    "搜索记录",
    "Search history"
  ],
  [
    "主机名称或通知内容",
    "Host name or notification text"
  ],
  [
    "时间范围",
    "Time range"
  ],
  [
    "最近 24 小时",
    "Last 24 hours"
  ],
  [
    "最近 7 天",
    "Last 7 days"
  ],
  [
    "最近 30 天",
    "Last 30 days"
  ],
  [
    "主机",
    "Host"
  ],
  [
    "全部主机",
    "All hosts"
  ],
  [
    "仅本机",
    "This host only"
  ],
  [
    "事件类型",
    "Event type"
  ],
  [
    "全部类型",
    "All types"
  ],
  [
    "查询",
    "Search"
  ],
  [
    "外部投递",
    "External delivery"
  ],
  [
    "告警",
    "Alert"
  ],
  [
    "恢复",
    "Recovery"
  ],
  [
    "信息",
    "Information"
  ],
  [
    "仅本地",
    "Local only"
  ],
  [
    "待发送",
    "Pending"
  ],
  [
    "已发送",
    "Sent"
  ],
  [
    "发送失败",
    "Delivery failed"
  ],
  [
    "已停止发送",
    "Delivery stopped"
  ],
  [
    "CPU 使用率",
    "CPU usage"
  ],
  [
    "内存使用率",
    "Memory usage"
  ],
  [
    "磁盘使用率",
    "Disk usage"
  ],
  [
    "网络吞吐",
    "Network throughput"
  ],
  [
    "累计接收",
    "Total received"
  ],
  [
    "累计传送",
    "Total sent"
  ],
  [
    "主机连接",
    "Host connection"
  ],
  [
    "SSH 登录",
    "SSH login"
  ],
  [
    "保留最近",
    "Retain the last"
  ],
  [
    "天，最多",
    "days, up to"
  ],
  [
    "条；达到容量上限时清理最早记录。",
    "events; the oldest are removed when storage fills."
  ],
  [
    "暂无符合条件的通知",
    "No matching notifications"
  ],
  [
    "可以调整筛选条件；首次启用后只记录新发生的事件。",
    "Adjust the filters. Only new events are recorded after first enabling this feature."
  ],
  [
    "关联告警编号",
    "Related alert ID"
  ],
  [
    "渠道",
    "Channel"
  ],
  [
    "发送次数",
    "Attempts"
  ],
  [
    "最近尝试",
    "Last attempt"
  ],
  [
    "发送失败会自动重试，最长 24 小时；本地记录已保存。",
    "Failed delivery is retried for up to 24 hours. The local event is saved."
  ],
  [
    "外部推送已关闭、配置已变化或重试已到期，本地记录仍保留。",
    "Delivery is off, configuration changed, or retries expired. The local event remains."
  ],
  [
    "正在加载…",
    "Loading…"
  ],
  [
    "加载更多",
    "Load more"
  ],
  [
    "登录已过期，请重新登录。",
    "Your session expired. Please sign in again."
  ],
  [
    "筛选条件无效，请缩短搜索内容或调整筛选后重试。",
    "Invalid filters. Shorten the search or adjust the filters and retry."
  ],
  [
    "通知记录暂时不可用，请重试或检查 KPanel 数据目录。",
    "Notification history is unavailable. Retry or check the KPanel data directory."
  ]
] as const satisfies PhraseCatalog
