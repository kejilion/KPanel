import type { PhraseCatalog } from '@/i18n/phrase'

export default [
  ['收起的主机列表', 'Collapsed host list'],
  ['多主机终端', 'Multi-host terminal'],
  ['通过集群加密通道连接本机、已授权 KPanel 节点和轻量节点，无需开放额外 SSH 或公网端口。', 'Connect to this server, authorized KPanel nodes, and lightweight nodes through the encrypted cluster channel without opening additional SSH or public ports.'],
  ['连接列表加载失败，请检查 Agent 与集群状态。', 'Failed to load connections. Check the Agent and cluster status.'],
  ['已达到终端会话上限，请先关闭不用的连接。', 'The terminal session limit has been reached. Close an unused connection first.'],
  ['终端连接失败，请确认目标节点在线且中心与节点均已更新。', 'Terminal connection failed. Confirm the target node is online and both the center and node are up to date.'],
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
  ['选择主机，打开终端', 'Select a host and open a terminal'],
  ['从连接列表选择一台可用主机，开始加密终端会话。', 'Choose an available host from the connection list to start an encrypted terminal session.'],
  ['关闭窗口将断开 {0} 个终端会话，是否继续？', 'Closing this window will disconnect {0} terminal session(s). Continue?'],
] satisfies PhraseCatalog
