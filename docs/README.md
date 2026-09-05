# KPanel 文档索引

本索引只提供导航，不定义规则。任何规则冲突以 `PROJECT_RULES.md` →
`docs/project-management.md` → `docs/multi-agent-collaboration.md` → `.codex-workflows/`
的权威顺序为准。

`docs/` 目录包含两类文件：本索引覆盖的实质性文档，以及 `release-v*-acceptance.md`
发布验收记录。验收记录是不可变证据，按版本号检索，不在本索引逐条列出。

## 治理与规范

| 文档 | 用途 |
| --- | --- |
| [项目管理规范](project-management.md) | 任务契约、Definition of Ready/Done、发布接管、本地磁盘回收与集成 |
| [多智能体协作](multi-agent-collaboration.md) | 角色、所有权、worktree/分支模型、冲突恢复 |
| [开发质量标准](development-quality-standard.md) | 编码、测试与评审的质量基线 |
| [会话协作](session-collaboration.md) | 跨会话与跨工具的协作约定 |
| [发布验收模板](release-acceptance-template.md) | 验收记录结构、滚动指标与流程异常块语义 |
| [质量改进提案模板](quality-improvement-proposal-template.md) | 把重复问题转化为可验证改进的模板 |
| [本地功能预览标准](local-feature-preview-standard.md) | 提交前的本地预览证据要求 |

## 产品与架构

| 文档 | 用途 |
| --- | --- |
| [架构与事实来源](architecture.md) | 组件边界、业务真源与双端互通 |
| [安全模型](security-model.md) | 认证、授权与权限边界 |
| [安全入口与根文件](security-entry-and-root-files.md) | 入口暴露面与根文件约束 |
| [存储策略](storage-strategy.md) | 数据落盘、备份与迁移 |
| [生态互通基线](ecosystem-parity.md) | 与 `kejilion.sh` 的能力对齐 |
| [兼容基线](compatibility.md) | 向后兼容契约 |
| [平台支持](platform-support.md) | 宿主机系统兼容矩阵 |
| [部署文档](deployment.md) | 构建产物、镜像 digest 与部署流程 |
| [操作边界审计](operational-boundary-audit.md) | 允许与禁止的运维动作 |
| [UI 视觉语言](ui-visual-language.md) | 布局、字体、主题、图标与动效的唯一入口 |
| [品牌图标](brand-icons.md) | 图标资产规范 |
| [国际化](internationalization.md) | 多语言架构与本地化契约 |

## 功能域

| 文档 | 用途 |
| --- | --- |
| [AI 工作区](ai-workspace.md) | AI 能力入口与边界 |
| [应用市场](application-market.md) | 应用分发与安装 |
| [Docker 管理](docker-management-v0.18.md) | 容器生命周期管理 |
| [Docker 命令部署](docker-command-deployment.md) | 命令式部署路径 |
| [LDNMP 环境管理](ldnmp-environment-management.md) | Web 运行环境编排 |
| [网站业务分析](legacy-site-contract.md) | 站点契约与迁移 |
| [站点图标缓存](site-icon-cache.md) | 图标抓取与缓存 |
| [文件管理器设计](file-manager-design.md) | 文件浏览与操作 |
| [文件共享](file-sharing.md) | 共享链接与权限 |
| [跨 KPanel 文件传输](cross-kpanel-file-transfer.md) | 实例间传输 |
| [Windows 文件下载兼容](windows-file-download-compatibility.md) | 下载路径兼容处理 |
| [磁盘分区管理](disk-partition-management.md) | 分区与挂载 |
| [进程管理器设计](process-manager-design.md) | 进程视图与操作 |
| [系统管理](system-management.md) | 宿主机系统操作 |
| [多主机终端](multi-host-terminal.md) | 终端安全契约 |
| [集群监控](cluster-monitoring.md) | 多节点指标采集 |
| [集群通知](cluster-notifications.md) | 告警与通知通道 |
| [集群公开分享](cluster-public-share.md) | 对外分享入口 |
| [历史监控设计](history-monitoring-design.md) | 指标留存与查询 |
| [体检与第三方测试](diagnostics.md) | 诊断协议 |
| [两步验证](two-factor-authentication.md) | 2FA 安全契约 |
| [管理员密码恢复](password-recovery.md) | 单管理员恢复路径 |
| [外部配置源](external-config-sources.md) | 外部配置接入 |
| [桌面图标工作区](desktop-icon-workspace.md) | 桌面布局与图标 |

## 质量评审与基线

| 文档 | 用途 |
| --- | --- |
| [当前产品质量评审](product-quality-review-current.md) | 定位业务域的起点，随业务上下文刷新 |
| [运行时性能基线](runtime-performance-baseline.md) | 性能对照基线 |
| [业务对齐 v0.17](business-alignment-v0.17.md) | 历史业务对齐快照 |
| [范围 v0.1](scope-v0.1.md) | 初始范围快照 |

历史评审快照按日期归档，仅作证据引用，不代表当前状态：
[2026-08-02 质量审计](quality-audit-2026-08-02.md)、
[2026-07-28 安全与性能加固](security-performance-hardening-2026-07-28.md)、
[2026-08-11 产品质量评审](product-quality-review-2026-08-11.md)、
[2026-08-13 产品质量评审](product-quality-review-2026-08-13.md)。

## 质量改进提案与历史依据

提案保留形成时的讨论与验证状态，不是当前任务队列。是否已采纳以批准基线中的规范、精确提交和
验收证据为准；不得仅凭旧提案的“待复核”文字恢复任务，也不为刷新索引改写历史原件。

| 提案 | 查阅用途 |
| --- | --- |
| [后台浏览器验收](quality-improvement-2026-08-13-background-browser-validation.md) | 后台作业方案及初始验证依据 |
| [治理反馈闭环](quality-improvement-2026-08-15-governance-feedback-loop.md) | 受控改进循环的设计依据 |
| [治理验收合同](quality-improvement-2026-08-17-governance-acceptance-contract.md) | 固定六维矩阵和停止条件的设计依据 |
| [本地功能预览标准](quality-improvement-2026-08-17-local-feature-preview-standard.md) | 当前入口为 [local-feature-preview-standard.md](local-feature-preview-standard.md) |
| [执行效率](quality-improvement-2026-08-20-execution-efficiency.md) | 分级检查与执行效率的历史分析 |
| [治理候选 CI](quality-improvement-2026-08-23-governance-candidate-ci.md) | 同 SHA 候选 CI 的设计依据 |
| [执行兼容性](quality-improvement-2026-08-24-execution-compatibility.md) | 管理工作树和跨 Shell 入口的设计依据 |
| [规范一致性与发布接管](quality-improvement-2026-09-05-standards-alignment.md) | 本次治理候选的范围、验证与待复核边界 |

## 发布验收记录

`docs/release-v<version>-acceptance.md` 按 `docs/release-acceptance-template.md` 结构化，
是不可变发布证据，禁止删除或改写。滚动指标汇总由唯一入口生成：

```bash
node scripts/report-release-metrics.mjs --days 14 --releases 20 --format markdown
```

该报告同时暴露滚动 5 个版本内重复的流程异常指纹，用于 `PROJECT_RULES.md` 5.1.7 的重复
流程缺陷判定；机器只校验计数与结构，根因与永久处置的真实性仍由发布复核者判断。
