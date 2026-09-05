#!/usr/bin/env node

import { existsSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const repoRoot = resolve(fileURLToPath(new URL('..', import.meta.url)));
const failures = [];

const requiredFiles = [
  'PROJECT_RULES.md',
  'AGENTS.md',
  'CLAUDE.md',
  'docs/project-management.md',
  'docs/multi-agent-collaboration.md',
  'docs/development-quality-standard.md',
  'docs/ui-visual-language.md',
  'docs/local-feature-preview-standard.md',
  'docs/product-quality-review-current.md',
  'docs/release-acceptance-template.md',
  'docs/quality-improvement-proposal-template.md',
  'dependency-policy.json',
  'environment-policy.json',
  'scripts/check-environment-policy.mjs',
  'scripts/check-governance-candidate-ci.mjs',
  'scripts/verify-governance.sh',
  'scripts/tests/check-environment-policy.test.mjs',
  'scripts/tests/governance-candidate-ci.test.mjs',
  'scripts/check-collaboration-state.mjs',
  'scripts/tests/collaboration-state.test.mjs',
  'scripts/run-repo-bash.mjs',
  'scripts/tests/run-repo-bash.test.mjs',
  'scripts/background-browser-test.mjs',
  'scripts/tests/background-browser-test.test.mjs',
  'scripts/local-feature-preview.mjs',
  'scripts/mock-app-market-api.mjs',
  'scripts/tests/local-feature-preview.test.mjs',
  'scripts/run-release-gate.sh',
  'scripts/tests/release-gate-runner.test.mjs',
  'scripts/run-release-l3.mjs',
  'scripts/run-release-l3-remote.sh',
  'scripts/tests/release-l3-orchestrator.test.mjs',
  'scripts/run-production-evidence.mjs',
  'scripts/run-production-evidence-remote.sh',
  'scripts/tests/production-evidence-orchestrator.test.mjs',
  'scripts/report-release-metrics.mjs',
  'scripts/tests/report-release-metrics.test.mjs',
  'scripts/check-business-context-freshness.mjs',
  'scripts/tests/business-context-freshness.test.mjs',
  'scripts/report-dependency-freshness.mjs',
  'scripts/tests/report-dependency-freshness.test.mjs',
  'scripts/check-release-acceptance-coverage.mjs',
  'scripts/tests/release-acceptance-coverage.test.mjs',
  '.codex-workflows/README.md',
  '.codex-workflows/session-collaboration.workflow.yaml',
  '.codex-workflows/background-browser-validation.workflow.yaml',
  '.codex-workflows/local-feature-preview.workflow.yaml',
  '.codex-workflows/release-kpanel.workflow.yaml',
  '.codex-workflows/quality-audit-kpanel.workflow.yaml',
  '.codex-workflows/evolve-kpanel.workflow.yaml',
  '.codex-workflows/maintain-kpanel-dependencies.workflow.yaml',
  '.codex-workflows/kpanel-real-machine-app-lifecycle.workflow.yaml',
  '.codex-workflows/kpanel-site-icon-cache-validation.workflow.yaml',
  '.codex-workflows/normalize-kpanel-app-icons.workflow.yaml',
];

function read(relativePath) {
  const absolutePath = resolve(repoRoot, relativePath);
  if (!existsSync(absolutePath)) {
    failures.push(relativePath + ': required file is missing');
    return '';
  }
  return readFileSync(absolutePath, 'utf8');
}

function requireText(relativePath, tokens) {
  const content = read(relativePath);
  for (const token of tokens) {
    if (!content.includes(token)) failures.push(relativePath + ': missing required reference "' + token + '"');
  }
}

function checkRepositoryHygiene() {
  const trackedPaths = execFileSync(
    'git',
    ['ls-files', '--cached', '--others', '--exclude-standard', '-z'],
    {
      cwd: repoRoot,
      encoding: 'utf8',
    },
  ).split('\0').filter(Boolean);
  const forbiddenTrackedPaths = [
    /(^|\/)\.codex-tmp\//i,
    /(^|\/)node_modules\//i,
    /(^|\/)web\/dist\//i,
    /(^|\/)coverage\//i,
  ];
  const forbiddenContent = [
    {
      pattern: /[A-Za-z]:[\\/]+Users[\\/]+[^\\/\s"'`<>]+[\\/]+/i,
      label: 'machine-specific Windows user path',
    },
    {
      pattern: /AppData[\\/]+Local[\\/]+Temp/i,
      label: 'local temporary attachment path',
    },
    {
      pattern: new RegExp('codex-' + 'clipboard', 'i'),
      label: 'temporary clipboard attachment marker',
    },
    {
      pattern: new RegExp('<codex_' + 'delegation|<source_' + 'thread_id', 'i'),
      label: 'Codex session delegation envelope',
    },
  ];

  for (const relativePath of trackedPaths) {
    const normalizedPath = relativePath.replaceAll('\\', '/');
    if (forbiddenTrackedPaths.some((pattern) => pattern.test(normalizedPath))) {
      failures.push(relativePath + ': generated or temporary path must not be tracked');
      continue;
    }

    const absolutePath = resolve(repoRoot, relativePath);
    if (!existsSync(absolutePath)) continue;
    const raw = readFileSync(absolutePath);
    if (raw.includes(0)) continue;
    const content = raw.toString('utf8');
    for (const rule of forbiddenContent) {
      const match = rule.pattern.exec(content);
      if (!match) continue;
      const line = content.slice(0, match.index).split('\n').length;
      failures.push(relativePath + ':' + line + ': contains ' + rule.label);
    }
  }
}

for (const relativePath of requiredFiles) read(relativePath);
checkRepositoryHygiene();

const adapterTokens = [
  'PROJECT_RULES.md',
  'docs/project-management.md',
  'docs/multi-agent-collaboration.md',
  'Definition of Ready',
  'Definition of Done',
  'make verify-change',
  'make verify-release',
  'docs/ui-visual-language.md',
  'docs/product-quality-review-current.md',
  'scripts/check-collaboration-state.mjs',
  'scripts/run-repo-bash.mjs',
];
requireText('AGENTS.md', adapterTokens);
requireText('CLAUDE.md', adapterTokens);

requireText('PROJECT_RULES.md', [
  'docs/development-quality-standard.md',
  'docs/ui-visual-language.md',
  'docs/release-acceptance-template.md',
  '受控自我改进',
  '不得自动放宽',
  '公共默认',
  'dependency-policy.json',
  '最新稳定版',
  'dependency-policy.json#adoptionLifecycle',
  '传递依赖的跨范围 `latest` 是归属信号',
  '基座 Patch/Minor',
  'environment-policy.json',
  '后台浏览器',
  '本地功能预览',
  'prod-108',
  '有界容器内控制台',
  '规范验收契约 v1.0',
  '正确性',
  '一致性',
  '完整性',
  '可执行性',
  '效率与比例性',
  '可演进性',
  'PASS WITH FOLLOW-UP',
  '停止条件',
  '首次生产写操作前被拦截的流程异常不计变更失败',
  '流程异常指纹',
  'kpanel-release-process-incidents:start/end',
  'scripts/check-collaboration-state.mjs',
  'scripts/run-release-l3.mjs',
  '候选分支保留到同一 SHA 的主线 CI 成功',
  '1.5 KPanel 与 `kejilion.sh` 跨仓库发布联动',
  'crossRepositoryReleaseLinkage',
  'not-required',
  '无需发布脚本（不适用）',
  'coupled',
  'script-only',
]);
requireText('docs/development-quality-standard.md', [
  'ui-visual-language.md',
  '宿主机系统动作',
  '`kejilion.sh` 原生交互',
  '目标容器内控制台',
  '有生产完成证据的部署频率',
  '两类同时计入',
  '7/14/30 天',
  '14/30/60 天',
  '30/90/90 天',
  '传递依赖归属信号',
  '版本变化幅度决定处理期限',
]);
requireText('docs/product-quality-review-current.md', [
  'KPanel 当前业务事实与规范适配基线',
  '基线提交',
  'scripts/check-business-context-freshness.mjs',
  '目标容器内有界控制台',
]);
requireText('docs/project-management.md', [
  'docs/ui-visual-language.md',
  'Definition of Ready',
  'Definition of Done',
  '标准交付包',
  '受控自我改进循环',
  'docs/quality-improvement-proposal-template.md',
  '.codex-workflows/evolve-kpanel.workflow.yaml',
  '.codex-workflows/maintain-kpanel-dependencies.workflow.yaml',
  'make dependency-report',
  '每日安全审计',
  'background-browser-validation.workflow.yaml',
  'local-feature-preview-standard.md',
  '本地功能预览',
  '生产已部署',
  '仅唯一集成/发布任务且需要明确主线集成授权',
  'kpanel-release-process-metrics:start/end',
  'scripts/check-collaboration-state.mjs',
  '流程异常指纹',
  'scripts/check-governance-candidate-ci.mjs',
  'Patch 最晚 7 天启动/14 天决策/30 天完成处置',
  '重复报告不能重置期限',
  'crossRepositoryReleaseLinkage',
  'scriptLinkageState',
  '无需发布脚本（不适用）',
  'coupled',
  'script-only',
]);
requireText('docs/multi-agent-collaboration.md', [
  'scripts/check-collaboration-state.mjs',
  '只隔离',
]);
// Keep lifecycle ownership in one canonical section; these checks validate document wiring,
// not task messages, directory ownership, or permission to delete local files.
requireText('docs/project-management.md', [
  '### 10.1 发布接管后的责任',
  '不得重新唤醒已交付的开发或验证任务',
  '协调中心不定时催问发布任务',
  '### 13.1 本地磁盘空间回收',
  '实际净释放字节',
  'Remove-Item -LiteralPath',
  '禁止 `--force`',
  '唯一原件',
]);
for (const path of ['AGENTS.md', 'CLAUDE.md', '.codex-workflows/README.md',
  '.codex-workflows/session-collaboration.workflow.yaml', '.codex-workflows/release-kpanel.workflow.yaml']) {
  requireText(path, ['docs/project-management.md', '10.1', '13.1']);
}
for (const path of ['docs/session-collaboration.md', 'docs/multi-agent-collaboration.md',
  '.codex-workflows/background-browser-validation.workflow.yaml', '.codex-workflows/evolve-kpanel.workflow.yaml']) {
  requireText(path, ['10.1']);
}
for (const path of ['docs/product-quality-review-current.md', 'docs/development-quality-standard.md']) {
  if (/0\.x\s*(阶段|产品)/.test(read(path))) failures.push(path + ': current product context must not retain the obsolete 0.x stage');
}
for (const [path, obsolete] of [
  ['docs/project-management.md', '回到原开发任务修复'],
  ['.codex-workflows/release-kpanel.workflow.yaml', '由监控任务读取终态'],
]) {
  if (read(path).includes(obsolete)) failures.push(path + ': obsolete release ownership instruction: ' + obsolete);
}
requireText('.codex-workflows/README.md', [
  'ui-visual-language.md',
  'evolve-kpanel.workflow.yaml',
  'maintain-kpanel-dependencies.workflow.yaml',
  'make dependency-policy-check',
  'make dependency-report',
  'background-browser-validation.workflow.yaml',
  'local-feature-preview.workflow.yaml',
  'environment-policy.json',
  'docs/product-quality-review-current.md',
  '规范复核执行 `PROJECT_RULES.md` 5.3',
  'kpanel-site-icon-cache-validation.workflow.yaml',
  'crossRepositoryReleaseLinkage',
  '无需发布脚本（不适用）',
  'script-only',
]);
requireText('docs/local-feature-preview-standard.md', [
  'ui-visual-language.md',
  'UI Mock',
  'Local Integration',
  'Isolated Real Host',
  '`draft`',
  '`acceptance`',
  'scripts/local-feature-preview.mjs',
  '`visual-composition`',
  '全量笛卡尔积',
  '停止方式',
]);
requireText('scripts/local-feature-preview.mjs', [
  'local preview only accepts loopback API targets',
  'acceptance preview requires a clean checkpoint',
  'ownership marker is missing',
  'workingTreeFingerprint',
  'acceptanceProfile',
  'affectedJourneys',
]);
requireText('Makefile', [
  'governance-check:',
  'node scripts/run-repo-bash.mjs scripts/verify-governance.sh',
  'node scripts/run-repo-bash.mjs scripts/verify-change.sh',
]);
requireText('scripts/run-repo-bash.mjs', [
  'Git for Windows Bash was not found',
  "startsWith('GIT_')",
  'spawnSync',
]);
requireText('scripts/verify-governance.sh', [
  'scripts/tests/governance-candidate-ci.test.mjs',
  'scripts/tests/run-repo-bash.test.mjs',
  'scripts/tests/release-l3-orchestrator.test.mjs',
  'scripts/tests/production-evidence-orchestrator.test.mjs',
  'scripts/tests/release-acceptance-coverage.test.mjs',
  'node scripts/check-release-acceptance-coverage.mjs',
]);
requireText('scripts/verify-change.sh', [
  'needs_governance=false',
  'GITHUB_TOKEN="$governance_ci_token" node scripts/check-governance-candidate-ci.mjs',
  'unset GOVERNANCE_CI_TOKEN GITHUB_TOKEN',
  'bash scripts/verify-governance.sh',
  'node scripts/check-governance-consistency.mjs',
  'node scripts/check-business-context-freshness.mjs',
  'scripts/local-feature-preview.mjs',
  'scripts/tests/local-feature-preview.test.mjs',
  'scripts/check-collaboration-state.mjs',
  'scripts/tests/collaboration-state.test.mjs',
  'scripts/run-repo-bash.mjs',
  'scripts/tests/run-repo-bash.test.mjs',
  'scripts/run-release-l3.mjs',
  'scripts/run-release-l3-remote.sh',
  'scripts/tests/release-l3-orchestrator.test.mjs',
  'scripts/run-production-evidence.mjs',
  'scripts/run-production-evidence-remote.sh',
  'scripts/tests/production-evidence-orchestrator.test.mjs',
  'scripts/tests/governance-candidate-ci.test.mjs',
  '--validate-acceptance',
  'node scripts/check-release-acceptance-coverage.mjs',
  'scripts/tests/release-acceptance-coverage.test.mjs',
  '--diff-filter=ACMRTD',
  '.github/workflows/*.yml|.github/workflows/*.yaml',
  'verification_preflight=pass',
  'gofmt -l',
]);
requireText('.codex-workflows/release-kpanel.workflow.yaml', [
  'scripts/check-collaboration-state.mjs',
  'scripts/run-release-gate.sh',
  'scripts/run-release-l3.mjs',
  'scripts/run-production-evidence.mjs',
  '--artifact-dir',
  '--target',
  'Docker 自动分配',
  '持久业务结果',
  'evidence_dir=<本次唯一持久化证据目录>',
  'timeout_seconds=<风险窗口加清理余量>',
  'command_spec=<无秘密、已哈希的仓库测试命令规格>',
  '--validate-acceptance',
  '首次生产写操作前被门禁拦截的流程异常只计流程指标',
  '有生产完成证据的部署频率',
  '流程异常指纹',
  '冻结执行方案',
  '一次列全必需能力',
  '`visual-composition`',
  'crossRepositoryReleaseLinkage',
  'scriptLinkageState',
  '无需发布脚本（不适用）',
  'block-kpanel-candidate-or-remove-dependent-scope',
  'script-only',
]);
if (read('.codex-workflows/release-kpanel.workflow.yaml').includes('git fetch origin --tags')) {
  failures.push('.codex-workflows/release-kpanel.workflow.yaml: unbounded tag fetch is forbidden');
}
requireText('scripts/run-release-l3.mjs', [
  'shell: false',
  'bundle',
  'check-business-context-freshness.mjs',
  'check-environment-policy.mjs',
  'candidate worktree must be clean',
  'retries require a new run ID',
]);
requireText('scripts/run-release-l3-remote.sh', [
  'git init --bare',
  'bundle verify',
  'run-release-gate.sh',
  'PIPESTATUS[0]',
  'use a new run ID for every attempt',
]);
requireText('.codex-workflows/quality-audit-kpanel.workflow.yaml', [
  'docs/product-quality-review-current.md',
  'docs/ui-visual-language.md',
  '100%/125%/200%',
  'node scripts/check-business-context-freshness.mjs',
  '流程异常指纹',
]);
requireText('.codex-workflows/session-collaboration.workflow.yaml', [
  'scripts/check-collaboration-state.mjs',
  'quarantine only that worktree',
]);
requireText('.codex-workflows/local-feature-preview.workflow.yaml', [
  'docs/local-feature-preview-standard.md',
  'docs/ui-visual-language.md',
  '--profile "${{acceptance_profile}}"',
  '--journeys "${{affected_journeys}}"',
  '全量笛卡尔积',
]);
requireText('.codex-workflows/evolve-kpanel.workflow.yaml', [
  'PROJECT_RULES.md` 5.3',
  '六项固定矩阵',
  'PASS WITH FOLLOW-UP',
  '停止条件',
  '禁止重新开启无界全量探索',
  'scripts/check-governance-candidate-ci.mjs',
  '候选分支保留到主线 CI 成功',
]);
requireText('.codex-workflows/maintain-kpanel-dependencies.workflow.yaml', [
  '直接依赖',
  '传递依赖',
  '7/14/30 天',
  '30/90/90 天',
  '完成处置是采用、以证据拒绝，或建立有期限例外',
]);
requireText('.github/workflows/ci.yml', [
  'actions: read',
  'GOVERNANCE_CI_TOKEN: ${{ github.token }}',
]);

requireText('docs/release-acceptance-template.md', [
  '## 发布画像',
  '## 多维质量结论',
  '## 自动门禁',
  '## 依赖与技术栈变化',
  '最近每日安全通告审计、EOL 复核状态及证据',
  '直接/基座行动项、传递依赖归属信号',
  '## 隔离真机与浏览器验收',
  '后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256',
  '## 生产部署安全核对',
  '禁用全部 KPanel 操作',
  '## 回滚',
  '## 交付节奏数据',
  '<!-- kpanel-release-metrics:start -->',
  '<!-- kpanel-release-metrics:end -->',
  '两者之间只能按模板顺序保留六行固定纯文本字段',
  '首个纳入提交时间',
  '公共默认更新通道决策',
  '<!-- kpanel-release-process-metrics:start -->',
  '<!-- kpanel-release-process-metrics:end -->',
  '已记录发布流程异常或无效证据拦截次数',
  '两类同时计入',
  '### 流程异常明细',
  '<!-- kpanel-release-process-incidents:start -->',
  '<!-- kpanel-release-process-incidents:end -->',
  'before-production-write',
  'L3 外层入口 run ID',
  '## 遗留风险与后续准入',
  '## 跨仓库联动判定',
  'scriptLinkageState',
  '无需发布脚本（不适用）',
  'coupled',
  'script-only',
]);
requireText('docs/quality-improvement-proposal-template.md', [
  '## 观察证据',
  '## 原因假设',
  '## 基线、目标与观察窗口',
  '## 最小改动方案',
  '## 独立复核',
  '## 回滚',
  '## 采纳决策与结果',
  '### 规范验收合同',
  '规范验收结论',
  '规范验收停止依据',
]);
requireText('docs/ui-visual-language.md', [
  '小字最小 12px',
  '正文和操作控件最小 14px',
  '100%、125% 和 200%',
  '历史低于该基线的样式按受影响功能迁移',
  'WCAG 2.2',
]);

const workflows = [
  '.codex-workflows/session-collaboration.workflow.yaml',
  '.codex-workflows/background-browser-validation.workflow.yaml',
  '.codex-workflows/local-feature-preview.workflow.yaml',
  '.codex-workflows/release-kpanel.workflow.yaml',
  '.codex-workflows/quality-audit-kpanel.workflow.yaml',
  '.codex-workflows/evolve-kpanel.workflow.yaml',
  '.codex-workflows/maintain-kpanel-dependencies.workflow.yaml',
  '.codex-workflows/kpanel-real-machine-app-lifecycle.workflow.yaml',
  '.codex-workflows/kpanel-site-icon-cache-validation.workflow.yaml',
  '.codex-workflows/normalize-kpanel-app-icons.workflow.yaml',
];
for (const workflow of workflows) {
  const content = read(workflow);
  for (const key of ['name:', 'description:', 'version:', 'params:', 'updated:']) {
    if (!content.includes('\n' + key)) failures.push(workflow + ': missing frontmatter key "' + key.slice(0, -1) + '"');
  }
  for (const heading of ['## Purpose', '## Prerequisites', '## Steps', '## Verification', '## Notes']) {
    if (!content.includes(heading)) failures.push(workflow + ': missing section "' + heading + '"');
  }
}

requireText('.codex-workflows/kpanel-real-machine-app-lifecycle.workflow.yaml', [
  '--purpose candidate-validation',
  '--purpose failure-injection',
  'golang:1.26.7-alpine@sha256:',
]);
requireText('.codex-workflows/local-feature-preview.workflow.yaml', [
  'docs/local-feature-preview-standard.md',
  'docs/ui-visual-language.md',
  'scripts/local-feature-preview.mjs start',
  '模拟数据预览',
  '草稿预览，变更尚未冻结',
  'background-browser-validation',
]);
requireText('.codex-workflows/kpanel-site-icon-cache-validation.workflow.yaml', [
  'golang:1.26.7-bookworm@sha256:',
]);

for (const adapter of ['AGENTS.md', 'CLAUDE.md']) {
  const content = read(adapter);
  if (content.includes('# KPanel 永久工程规范')) {
    failures.push(adapter + ': tool adapter must not duplicate PROJECT_RULES.md');
  }
}

if (failures.length > 0) {
  process.stderr.write('Governance consistency check failed:\n');
  for (const failure of [...new Set(failures)]) process.stderr.write('- ' + failure + '\n');
  process.exit(1);
}

process.stdout.write('Governance consistency check passed.\n');
