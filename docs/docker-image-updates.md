# Docker 镜像更新检查

普通容器通过 `POST /api/v1/docker/containers/{id}/check_update` 按需检查，携带当前
`resourceVersion`；复用 Panel 登录、Origin、CSRF、审计与 Agent 固定路由。
此接口只读取 Docker Engine，不 pull、不重建，不创建后台任务。

- 本地身份始终使用容器 inspect 的 `Image`，而不是可能已被重新拉取的 `Config.Image` 标签。
- 远端保留用户的 repository/tag；受管容器使用镜像 ID 创建时，复用已有镜像标签元数据。
- 本地 `RepoDigests` 必须属于规范化后的同一仓库；相同摘要为 `current`，表示该标签未发现更新。
- containerd 存储在标签移动后可能清空旧镜像的 `RepoDigests`；仅在该列表为空、inspect 返回的 ID
  与运行镜像一致且 `Descriptor` 为有效 manifest/index 时，使用该内容摘要，并继续核对仓库中的摘要层级与平台。
- 不同摘要先通过 Engine distribution 元数据确认层级可比；单平台 manifest 还需平台一致。
  多平台 index 与平台 manifest 不能直接比较。缺失类型、摘要或平台、仓库拒绝、超时、限流均无法确认。
- `available` 表示该标签的可比较镜像摘要改变，不表示上游最高版本，也不跨标签寻找版本。
- digest 或镜像 ID 固定引用为 `fixed`。共享应用检查仍保留错误语义，避免旧消费者报告错误成功。
- 检查完成前再次核对容器身份及资源版本；发生变化返回冲突，旧结果不可作为当前状态。

所有共享消费者合计最多两个并行检查，无排队，单次总时限 20 秒；继续复用既有有界 Docker
响应及 Docker Hub 加速回退。普通 Docker 页面只保留最多 200 个内存结果，5 分钟过期，刷新、资源变化、
切换页面或桌面窗口失活会失效并取消读取；无全量扫描、自动升级或新增私库凭据存储。

更新操作继续通过原应用市场或 Compose 的确认、资源版本与后台任务执行；镜像页 pull 只更新本地标签，
不会自动重建使用该标签的现有容器。没有新增通用容器重建协议，也不要求新的 `kejilion.sh` 契约。
