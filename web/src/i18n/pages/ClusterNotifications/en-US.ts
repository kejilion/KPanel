export default [
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
