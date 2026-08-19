import type { PhraseCatalog } from '@/i18n/phrase'

export default [
  ['已有应用任务正在运行：{0}', 'An app task is already running: {0}'],
  ['应用状态缺少安全版本标识。', 'The app state is missing its safe version identifier.'],
  ['Agent 未返回可交互的脚本任务。', 'The Agent did not return an interactive script task.'],
  ['应用标识无效。', 'The app identifier is invalid.'],
  ['应用目录中没有找到对应应用。', 'The matching app was not found in the catalog.'],
  ['此应用没有可用的脚本管理入口。', 'Script management is not available for this app.'],
  ['无法打开应用脚本终端。', 'Unable to open the app script terminal.'],
  ['正在启动脚本终端…', 'Starting the script terminal…'],
  ['正在校验安装状态、管理能力和资源版本。', 'Checking installation state, management capability, and resource version.'],
  ['脚本终端无法启动', 'The script terminal could not start'],
  ['重新尝试', 'Try again'],
  ['终端已在后台保持', 'The terminal is being kept in the background'],
  ['重新聚焦此窗口后继续显示脚本交互。', 'Focus this window again to continue the script interaction.'],
] as const satisfies PhraseCatalog
