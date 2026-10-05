#!/usr/bin/env bash

set -euo pipefail

if (( $# != 4 )); then
  echo "usage: $0 <version> <image> <digest> <output>" >&2
  exit 2
fi

version="$1"
image="$2"
digest="$3"
output="$4"
changelog="${CHANGELOG_FILE:-CHANGELOG.md}"

stable_pattern='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'
preview_pattern='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-rc\.([1-9][0-9]*)$'
channel=""
image_label=""
if [[ "$version" =~ $stable_pattern ]]; then
  channel=stable
  image_label=生产
elif [[ "$version" =~ $preview_pattern ]]; then
  channel=preview
  image_label=预览
else
  echo "release notes version must be X.Y.Z or X.Y.Z-rc.N" >&2
  exit 1
fi
release_train="${version%%-rc.*}"
IFS=. read -r major minor patch <<< "$release_train"
for number in "$major" "$minor" "$patch"; do
  (( 10#$number <= 999999 )) || {
    echo "release notes version components must not exceed 999999" >&2
    exit 1
  }
done
if [ "$channel" = preview ]; then
  rc_number="${version##*-rc.}"
  (( 10#$rc_number <= 999999 )) || {
    echo "release notes candidate number must not exceed 999999" >&2
    exit 1
  }
fi
test -f "$changelog" || {
  echo "release notes changelog not found: $changelog" >&2
  exit 1
}

section="$({
  awk -v version="$version" '
    $0 == "## [" version "]" || index($0, "## [" version "] - ") == 1 {
      found = 1
      next
    }
    found && /^## \[/ { exit }
    found && !started && /^[[:space:]]*$/ { next }
    found {
      started = 1
      if ($0 == "### Added") print "#### 新增"
      else if ($0 == "### Changed") print "#### 变更"
      else if ($0 == "### Fixed") print "#### 修复"
      else if ($0 == "### Security") print "#### 安全"
      else if ($0 == "### Performance") print "#### 性能"
      else if ($0 == "### Documentation") print "#### 文档"
      else if ($0 == "### Upgrade Notes") print "#### 升级注意事项"
      else if ($0 == "### Deprecated") print "#### 弃用"
      else if ($0 == "### Removed") print "#### 移除"
      else if ($0 ~ /^### /) {
        sub(/^### /, "#### ")
        print
      } else print
    }
    END { if (!found) exit 3 }
  ' "$changelog"
} || true)"

test -n "${section//[[:space:]]/}" || {
  echo "CHANGELOG.md is missing the [$version] release section" >&2
  exit 1
}
grep -Eq '^- ' <<<"$section" || {
  echo "CHANGELOG.md [$version] must contain at least one explicit update item" >&2
  exit 1
}

mkdir -p "$(dirname "$output")"
{
  echo "## KPanel $version"
  echo
  if [ "$channel" = preview ]; then
    echo "> [!WARNING]"
    echo "> 这是 RC 预览版，可能包含尚未充分验证的功能。生产环境默认应继续使用稳定版。"
    echo
  fi
  echo "### 版本更新内容"
  echo
  printf '%s\n' "$section"
  echo
  echo "### 升级说明"
  echo
  if [ "$channel" = stable ]; then
    echo "- 已安装用户可继续通过 kejilion.sh 应用市场或 KPanel 稳定版通道原位升级。"
  else
    echo "- 仅建议已主动加入预览版计划的用户，通过 KPanel 设置页检查并安装此版本。"
    echo "- 退出预览版计划只切回稳定版来源，不会自动降级已安装的预览版。"
  fi
  echo "- 默认兼容升级并保留现有数据目录、端口与反向代理配置；如上方列有升级注意事项，以该版本的明确说明为准。"
  echo "- 更新前建议记录当前版本与镜像摘要；失败时按原应用市场回滚流程恢复上一镜像。"
  echo
  echo "### 发布产物与完整性"
  echo
  echo "- ${image_label}镜像：\`$image@$digest\`"
  echo "- 面板本体仅支持 Docker 部署；Release 不提供 paneld 原生二进制或 web dist。"
  echo "- \`kejilion-panel-meta-$version.tar.gz\` 仅含部署脚本、文档和许可等元数据，不是可构建源码包。"
  echo "- Agent、轻量节点和元数据归档须使用附件 \`SHA256SUMS\` 校验后再使用。"
  if [ "${KPANEL_WINDOWS_NODE_RELEASE:-false}" = true ]; then
    echo "- Windows 轻量节点提供 amd64/arm64 EXE、\`bootstrap-windows.ps1\` 和 \`install-windows.ps1\`；未签名，使用前必须通过固定官方 HTTPS 版本来源及附件 SHA256SUMS 校验脚本和 EXE；此校验不提供证书发布者身份保证。"
  fi
  echo "- Release 流水线已经执行测试、漏洞扫描、双架构构建、镜像运行契约及摘要一致性检查。"
  echo
  echo "完整版本记录：[CHANGELOG.md](https://github.com/kejilion/KPanel/blob/v$version/CHANGELOG.md)"
} > "$output"
