export default [
  ['自动备份通知', 'Automatic backup notifications'],
  ['启用自动备份通知', 'Enable automatic backup notifications'],
  ['当前面板的自动备份失败、漏跑或旧副本清理失败时提醒，成功恢复后通知。', 'Notify when this panel’s automatic backup fails, misses its window, or cannot prune old copies; notify again after recovery.'],
  ["服务器到期提醒","Server expiry reminders"],
  ["启用服务器到期提醒","Enable server expiry reminders"],
  ["统一提醒所有已填写到期日期的主机，按当前 KPanel 时区，在提前 7、3、1 天及到期当天各提醒一次。","Notify for all hosts with an expiry date, once 7, 3 and 1 days before expiry and on the expiry date, using this KPanel's timezone."],
  ["通知记录", "Notification history"],
  ["事件默认保存在本机，可选开启外部渠道推送。", "Events are saved locally by default. External delivery is optional."],
  ["本地记录暂时不可用", "Local recording unavailable"],
  ["本地记录已启用", "Local recording enabled"],
  ["请检查 KPanel 数据目录，恢复前无法保存新的通知。", "Check the KPanel data directory. New events cannot be saved until storage recovers."],
  ["查看通知记录", "View notification history"],
  ["外部推送", "External delivery"],
  ["关闭后继续保存本地记录，只暂停外部发送。", "When off, local recording continues and external delivery pauses."],

  ['启用服务异常通知', 'Enable service alerts'],
  ['监控所有主机的 Ping、TCP、HTTP 检测项，连续 3 次失败时告警，恢复后通知。', 'Monitor Ping, TCP and HTTP checks on all hosts. Alert after 3 consecutive failures and notify on recovery.'],
  ["服务异常通知", "Service alerts"],
  [
    "本机资源提醒",
    "Local resource alerts"
  ],
  [
    "默认关闭。仅提醒本机已发现的证书和所选容器；不检测网站公网可用性。",
    "Off by default. Alerts cover discovered local certificates and selected containers; public website availability is not checked."
  ],
  [
    "当前服务尚不支持本机资源提醒。",
    "This server does not support local resource alerts yet."
  ],
  [
    "提醒状态已达上限，新资源暂不发送；已有提醒继续处理。",
    "Alert state capacity reached. New resources wait; existing alerts continue."
  ],
  [
    "全部暂停 1 小时",
    "Pause all for 1 hour"
  ],
  [
    "解除暂停",
    "Resume alerts"
  ],
  [
    "暂停至",
    "Paused until"
  ],
  [
    "选择或暂停后，点击保存设置生效；暂停到期自动恢复评估。",
    "Save settings to apply selections or pauses. Evaluation resumes when the pause expires."
  ],
  [
    "证书到期",
    "Certificate expiry"
  ],
  [
    "启用证书到期提醒",
    "Enable certificate expiry alerts"
  ],
  [
    "按 30 / 7 / 1 天和已过期分级提醒，每阶段一次；换证后重新判断。",
    "Alerts at 30 / 7 / 1 days and expiration, once per stage; replacement certificates are evaluated again."
  ],
  [
    "证书读取数量达到上限，当前状态未知。",
    "Certificate discovery limit reached; current state is unknown."
  ],
  [
    "证书读取不可用，当前状态未知。",
    "Certificate discovery unavailable; current state is unknown."
  ],
  [
    "查看本机证书",
    "View local certificates"
  ],
  [
    "尚未发现启用 TLS 的网站证书。",
    "No certificates for TLS websites discovered."
  ],
  [
    "状态未知",
    "State unknown"
  ],
  [
    "维护方式未知",
    "Maintenance mode unknown"
  ],
  [
    "自有证书，请自行换证",
    "Custom certificate; replace it manually"
  ],
  [
    "具备脚本自动续期资格；不代表续期成功",
    "Eligible for script renewal; renewal success is unconfirmed"
  ],
  [
    "关键容器",
    "Important containers"
  ],
  [
    "仅对勾选容器提醒：持续不健康、未运行或重启中，以及 5 分钟内观测到至少 3 次重启。连续 3 次采样后发送，不会自动启动容器。",
    "Only selected containers: unhealthy, not running, restarting, or at least 3 observed restarts in 5 minutes. Alerts require 3 consecutive samples and never start containers."
  ],
  [
    "容器读取数量达到上限，当前状态未知。",
    "Container discovery limit reached; current state is unknown."
  ],
  [
    "容器读取不可用，当前状态未知。",
    "Container discovery unavailable; current state is unknown."
  ],
  [
    "尚未发现本机容器。",
    "No local containers discovered."
  ],
  [
    "提醒容器",
    "Alert for container"
  ],
  [
    "暂停 1 小时",
    "Pause for 1 hour"
  ],
  [
    "最多选择 64 个容器。未知、读取失败和已移除不会被当成故障或恢复。",
    "Select up to 64 containers. Unknown, failed reads and removal do not count as failure or recovery."
  ],
  [
    "状态未知或资源已移除",
    "Unknown state or resource removed"
  ],
  [
    "重启中",
    "Restarting"
  ],
  [
    "未运行",
    "Not running"
  ],
  [
    "不健康",
    "Unhealthy"
  ],
  [
    "健康检查启动中",
    "Health check starting"
  ],
  [
    "运行中",
    "Running"
  ],
  [
    "已过期",
    "Expired"
  ],
  [
    "30 天内到期",
    "Expires within 30 days"
  ],
  [
    "有效",
    "Valid"
  ],
  [
    "资源状态已过期，请重新读取。",
    "Resource observations expired; read them again."
  ],
  [
    "重新读取",
    "Read again"
  ],
  [
    "读取时间",
    "Observed at"
  ]
] as const
