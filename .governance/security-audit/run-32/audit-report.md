# Smart-download scoped audit — run-32

## Status: incomplete

本轮冻结在 `0b808e7bb00ee5d058bf0b2857009c9307099965`，tree 为
`d189d37451d982bd42c4802617dbdfe92df4fca2`，开始和结束的源码工作树均 clean。
只选择该功能提交的 50 个变更路径及直接调用链；comparison base 为
`4c0694aa8e02e46145a775707b8d5a0355f7ce10`，其他待审提交不计覆盖。
后续功能修复、依赖升级和性能测试不在本轮 CF 审查范围。

20 个单元中，14 个记录为 `covered`、5 个 `out_of_scope`、1 个 `candidate`。
这些是已完成的单元检查记录，不代表整轮完成；本轮不计入已完成审计覆盖。
没有 confirmed finding，亦没有生成 `findings.json`。不能据此宣称没有漏洞。

## 未决项

`Bittorrent.PrivateMagnetDHTBeforeMetadata` 保持 `needs_validation`：磁力的 private
标志需要取得元数据后才能知道，在此之前是否发生携带该哈希的公网 DHT 查询尚未完成独立动态验证。
两个 Phase 3 源码复核提供了线索，但未观察到数据包写出；DHT 依赖归档身份也只得到部分核验。
待审摘要见 `pending-candidates.json`。未复核的完整 trace 和验证操作步骤保留在外部证据中。

另两个候选经独立 Phase 3 复核移除：当前固定 torrent 库在应用回调前约束 peer frame 大小；
人工 benchmark 使用由冻结仓库构建并记录摘要的可信二进制，未建立低信任主体跨界控制的证据。
后者的实际使用说明来自开发负责人，属于外部上下文，不是仓库证据。
宿主 benchmark 暂存和输出日志缺少磁盘配额仍作为操作侧加固建议保留。

## 过程限制

执行使用固定 Cloudflare skill commit `c1c8a8c1471069fb0e188eeaff69b8e8db6564a8`，
审计及其子代理均为 `gpt-6-luna` / `max`。CF 审计只读源码，未执行目标代码、测试、构建或网络探测。
Linux 结构验证属于父任务的独立行政检查，不是目标程序动态验证。

原始预算为 2400 秒、20 次子代理调用；因新增覆盖单元和独立验证调整为 3600 秒、24 次。
记录了 15 次子代理调用，包括一次在范围分配前被中止且不计覆盖的 hunter。
首个可核时钟样本至终态为 3631 秒，超出上限 31 秒；这是可观察区间，不是完整审计精确耗时。
tokens 与费用不可得，保留 null。详细预算调整、调用记录和时间不确定性见元数据。

scout 先于元数据初始化、第一次 hunter 先于账本分配，均作为偏差保留。
wave 1 前没有 pinned Linux validator 通过记录；首次后置检查失败 22 项。
后续两个阶段快照通过不能追认早期流程。预算停止时仍缺少 post-wave critic、final-clean critic、
DHT 的 fresh Phase 5 验证和独立最终记录核验，不继续启动审计子代理或补写通过状态。

## 终态后的结构整理与披露

原始终态账本 SHA-256 为 `f8ee6f37953051cb81f0f42b556f455f173781b65d5945ae9af60fe12b3b71d9`。
父任务在终态后检查该原件，返回 19 个结构错误：四个 covered 单元把空数组序列化成 `[null]`，
另一个单元的汇总路径遗漏了 local checks 已记录的三条路径。原件及失败日志保持不变。

入库版本仅将这些占位数组规范为 `[]`、补齐已有记录的汇总路径，并移除重复的未复核详细候选记录。
没有新增源码审查、变更单元状态、移除待审 fingerprint 或增加覆盖声明。
整理后的账本在 pinned、低权限、network-none Linux 环境通过 20 单元结构验证；
确切输入/输出摘要和命令见 `run-metadata.json`，原始 validator 输出随本目录保留。
这项 schema 通过不补齐缺失的 critic、Phase 5 或独立记录核验，也不改变 `incomplete`。

治理入库仅保留账本、状态元数据、本报告、待审摘要和结构验证日志；未包含秘密、原始 scratch、
本地机器配置或未复核的攻击/验证操作细节。外部原件摘要保存在元数据中。
最终功能候选采用 `Security-Audit: deferred`；稳定发布前需另批完成待审项及后续修复的边界覆盖。
