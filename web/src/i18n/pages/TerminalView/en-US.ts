import type { PhraseCatalog } from '@/i18n/phrase'

export default [
  ['收起的主机列表', 'Collapsed host list'],
  ['多主机终端', 'Multi-host terminal'],
  ['通过集群加密通道连接本机与已授权 KPanel 节点，无需开放额外 SSH 或公网端口。', 'Connect to this server and authorized KPanel nodes through the encrypted cluster channel without opening additional SSH or public ports.'],
  ['连接列表加载失败，请检查 Agent 与集群状态。', 'Failed to load connections. Check the Agent and cluster status.'],
  ['已达到终端会话上限，请先关闭不用的连接。', 'The terminal session limit has been reached. Close an unused connection first.'],
  ['终端连接失败，请确认目标 KPanel 在线且双方均已更新。', 'Terminal connection failed. Confirm the target KPanel is online and both panels are up to date.'],
  ['连接列表', 'Connections'],
  ['台主机', 'hosts'],
  ['搜索主机', 'Search hosts'],
  ['打开主机选择', 'Open host selector'],
  ['关闭主机选择', 'Close host selector'],
  ['暂无可显示主机', 'No hosts to display'],
  ['本机终端', 'Local terminal'],
  ['加密直连', 'Encrypted direct connection'],
  ['轻量监控节点', 'Monitoring-only node'],
  ['需要重新配对', 'Pair again to enable'],
  ['已打开终端', 'Open terminals'],
  ['选择一台主机开始', 'Select a host to begin'],
  ['左侧会明确标记本机、可加密直连的 KPanel，以及仅提供监控的轻量节点。', 'The list distinguishes the local host, encrypted KPanel connections, and monitoring-only lightweight nodes.'],
  ['关闭窗口将断开 {0} 个终端会话，是否继续？', 'Closing this window will disconnect {0} terminal session(s). Continue?'],
] satisfies PhraseCatalog
