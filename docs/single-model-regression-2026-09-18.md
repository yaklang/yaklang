# 辅助任务迁移回归记录（2026-09-18）

测试分支：`feat/single-ai-model-mode`，测试基线 `bcba359fc`（包含下述工作区修复）；对照 main：`ab520b4aa`。

## 2026-09-20 撤销短标识特殊处理（当前方案）

按用户确认，短标识与其他辅助任务使用同一条调度链路：多模型模式正常调用 Config 的 Speed 回调；单模型模式通过注册表中的 Skip 策略跳过，返回原有截断兜底。

- 删除此前新增的默认短标识响应包装器、启用/禁用开关及专门为其添加的测试入口与测试文件。
- `NewTestReAct` 和普通 `aid.NewCoordinator` 调用恢复原状；撤销整套测试构造器替换，不再自动截获 `task-short-id`。
- `generateSemanticIdentifier` 删除重复的单模型提前返回，是否 Skip 完全由 `ScheduleAuxiliaryTask` 判定，没有增加例外策略。
- 非命名流程测试使用短、明确的任务名称。恢复测试的唯一标记保留在 Goal 和 TaskID 中，且准备完整的持久化标识；原有恢复、取消、跳过/重做断言不变。这些用例不再额外模拟 AI 命名。
- 长名字的实际 Speed 调用、失败兜底和单模型零调用由 `TestSemanticIdentifierAuxiliaryProtocolAndFallback` 使用普通 Speed 回调验证；补充 Intelligence 零调用断言。
- Timeline 和记忆测试对 Config 调度入口的正常适配、插件数据库 fixture、记忆落库验证保持不变。

本轮定向回归通过：aireact（3.905s）、aid/test（11.308s）、aiforge（1.147s），覆盖上述模式边界及此前失败的恢复、取消、Skip/Redo 用例。日志：`/private/tmp/aid-normal-final.DNrWwK/result.log`。

核心整包复验通过：aicommon（117.509s）、aicommon/mock（10.654s）、aireact（160.245s）、aiforge（4.549s）。日志：`/private/tmp/aid-normal-packages.TtCklB/result.log`。短标识的多模型成功/失败、单模型 Skip、短名字无需 AI 四个分支以 `-count=3` 复验通过，均检查 Intelligence 零调用，日志：`/private/tmp/aid-normal-final.DNrWwK/identifier-repeat.log`。本次最终方案没有重跑全 AID 的所有行为测试，不能沿用已撤销包装器方案的全量结果作为本次全量证明。

此前包装器方案及其“默认响应”测试结果仅属已撤销方案的历史记录，不代表当前代码仍有这套机制。下文保留原回归过程。

最终代码已分组提交：

- `7f9e00c42`：短标识统一由辅助调度策略决定，补充模式边界断言。
- `424a736dd`：回归测试数据与调度 mock 适配，验证非法记忆不落库。
- `e043a3dd4`：HTTP 工具查询测试使用隔离数据库 fixture。

## 历史记录：2026-09-20 首轮修复与复验

此前报告的失败项均已修复并通过。修改限于测试及 mock 辅助代码，生产 Config 调度、输出校验、Speed/Intelligence 选择均未修改；没有新增跳过或放宽业务断言。

- Timeline 测试将原 mock provider 接到 `ScheduleAuxiliaryTaskFunc`，保留原压缩断言。
- 恢复、取消及 Skip/Redo 测试按 `liteforge[task-short-id]` 精确匹配并提供合法响应，不再把辅助请求当成主循环请求；取消后的调用检查仍在分流之前执行。新增单测确保其他 CallerLabel 不会被该 mock 吞掉。
- 记忆测试在 Config 层提供调度 mock，保留原结果和数据库断言；异步调用计数改为原子读取/更新。
- 两个 Yak 查询测试使用独立临时 profile 数据库，载入仓库内的真实 HTTP 工具脚本，测试结束恢复原数据库绑定。HTTP 执行错误也纳入断言。

验证范围为 `./common/ai/aid/... ./common/aiforge`，仍使用临时 `YAKIT_HOME`：

- 之前失败的四个包相关定向用例全部通过。
- 全量运行中 57 个有测试包直接通过，22 个包无测试，包括 aireact 整包（156.8s）、aicommon、aimem、aiforge、Loop 集成和 Yak 工具整包。
- mock 包在新增标签边界单测中出现一次 `[]byte`/`string` 编译错误；修正后整包复跑通过（12.058s）。
- AID 综合包的进度输出因日志缓冲滞后被误判为停顿，诊断时人工中断了测试进程。完整日志确认当时交互用例已通过，运行到计划审查；**这不是已证实的死锁或行为失败，也不能把该次全量命令写成一次性成功退出。** 随后按测试清单补跑所有未完成的用例（33.026s），与首轮结果合并核对，149 个顶层测试全部有结果：148 个通过，`TestCoordinator_Recovery_ToolUseReview` 保持代码原有的跳过。没有未覆盖顶层用例。
- 取消路径及两个插件查询测试再以 `-count=3` 连续复验通过。
- 前次新增的记忆真实链路三项测试在 aiforge 整包中继续通过。
- `git diff --check`、修改文件 gofmt 检查通过；没有提交 commit。

因此当前结论是：**全量范围分批复验完成，无遗留失败项；并非一条全量命令一次性全绿。** 以下章节保留 9 月 18 日的原始失败记录。

本次日志：

- 失败项定向复验：`/private/tmp/aid-fix-targeted.n6aSpG/results.jsonl`
- 全量原始日志：`/private/tmp/aid-fixed-full.qXp2A3/full.jsonl.gz`；事件摘要：同目录 `events.jsonl`
- mock 整包复跑：`/private/tmp/aid-fixed-mock.NpXWas/result.log`
- AID 综合剩余用例补跑：`/private/tmp/aid-fixed-remaining.gCKNxC/events.jsonl`
- 取消及插件三次复验：`/private/tmp/aid-fix-review.UvnCG7/result.log`

## 结论

**未全绿，不应将此前定向回归通过视为完整回归通过。**

首轮执行 `go test -p 4 ./common/ai/aid/... ./common/aiforge -json -count=1 -timeout=5m`，共 81 个包：55 个通过，4 个失败，22 个无测试。记录到 4237 个通过、13 个失败、13 个跳过的测试事件（包含子用例，不能等同于顶层测试数量）。aireact 和 aid/test 达到包级 5 分钟上限，未执行到的用例不能算通过。

使用独立临时 `YAKIT_HOME` 隔离数据库和工作目录。main 对照在独立 detached worktree 和全新测试目录下运行，没有切换或修改用户当前分支。

## 已通过的重点范围

- `aicommon` 整包：125.563s。
- `aireact/reactloops` 整包：30.796s；`reactloopstests` 集成包通过。
- `aimem`、`aiforge` 整包通过。
- Mini Task、Goal 验收、Speed Loop 的调度/协议/重试/取消相关测试包含在上述包中。
- 新增真实链路测试 `common/aiforge/liteforge_memory_validation_test.go`，单独运行 3 个子用例全部通过：
  - 持续非法记忆数组：两次调用（一次重试）后失败，数据库始终为空。
  - 首次非法、修复为空数组：成功返回，数据库为空。
  - 首次非法、修复为合法记忆：只保存修复后的单条记忆。
  - 每次模型调用都检查尚未写入非法响应中的有效前缀，且 Intelligence 调用计数始终为 0。

## 失败分类与证据

| 范围 | 当前分支现象 | main 对照 | 定位 |
|---|---|---|---|
| `aicommon/mock` 的 6 个 Timeline 压缩测试 | 没有生成压缩段 | 6 项均通过 | 测试只提供 `CallSpeedPriorityAI`；`NewMockedAIConfig` 的 `ScheduleAuxiliaryTaskFunc` 未接线，新入口直接返回 |
| `aireact` 的两个 RecoveryPlanAndExec 测试 | SkipCompletedTasks 失败；StartFromSpecifiedTask 超时，独立运行也超时 | 两项均通过 | mock 预期所有请求带 CURRENT_TASK，但新增 `task-short-id` 请求不包含该段，触发测试 Fatal；不宜直接认定真实恢复逻辑有错，须适配 mock 后复验 |
| `aid/test` 的 Skip/Redo Subtask 4 项 | 15s/10s/30s/10s 超时 | 四项均通过 | mock 对辅助请求返回 `finish`，与 `task-short-id` 输出协议不符，引起事务重试，预期交互未及时到达 |
| `yakscripttools/test` 的 GetYakScript、SearchYakScript | 找不到预置插件 | 两项同样失败 | 全新 profile 库缺少测试假定已存在的插件，属于环境/fixture 数据依赖 |

Timeline 六项：`TestMemoryTimelineWithBatchCompression`、`TestMemoryTimelineWithReachLimitBatchCompression`、`TestBinarySearchCompression`、`TestCompressionBoundary`、`TestCompressionRatio`、`TestCompressionWithDifferentSizes`。

Skip/Redo 四项：`TestCoordinator_SkipSubtaskInPlan_BySubtaskId`、`TestCoordinator_SkipSubtaskInPlan_NoIndexOrSubtaskId`、`TestCoordinator_RedoSubtaskInPlan_BySubtaskId`、`TestCoordinator_RedoSubtaskInPlan_SubtaskIdNotFound`。

## aireact 补跑

跳过上述两个已定位的 RecoveryPlanAndExec 测试，重新运行整包其余用例，161.349s 完成；另有 3 项失败，均已在 main 单独验证通过：

- `TestReAct_PlanAndExecute_SkipAfterCancel`：mock 遇到未处理的 Prompt，触发 `AI callback reached unreachable code`。
- `TestReAct_PlanAndExecute_TaskCancel`：同类 mock 分支触发 `unreachable code`。
- `TestReAct_ToolUse_Reject_WithAIMemory`：仍覆盖旧 runtime 的 `InvokeSpeedPriorityLiteForge`，新 Config 调度不经过该覆盖，预期 mock 调用计数未增加。

补跑记录有 456 个通过的测试事件（包含子用例且与首轮有重叠，不能累加为独立测试数）。

首轮 aid/test 在 `TestCoordinator_ReviewPlan_Incomplete` 执行期间达到包级超时；该用例在当前分支和 main 上单独运行均通过（约 30.22s），不能将它判定为迁移失败。该包仍有超时后未执行的用例。

## 后续建议

1. 将 Timeline、记忆相关测试桩接到 Config 调度入口。
2. 为恢复/规划测试显式提供 Speed 辅助响应，按 CallerLabel 返回正确 schema，避免将辅助请求误认成 Intelligence 主循环请求。保留当前生产调度边界和严格输出校验。
3. 为 Yak 插件查询测试准备独立 fixture，不依赖个人 profile 数据库。
4. 适配后复跑失败项，再重新跑全量；本次因包级超时未执行的 aid/test 后续用例仍待验证。

本轮只新增回归测试与本报告，没有修改生产代码、没有提交 commit。

## 日志

- 首轮全量：`/private/tmp/aid-regression.rsR8OG/results.jsonl`
- 记忆真实链路：`/private/tmp/aid-memory-validation.qVpBhB/result.log`
- aireact 补跑（压缩保留）：`/private/tmp/aid-aireact-remaining.ayvYDu/results.jsonl.gz`
- main Timeline/Recovery 对照：`/private/tmp/aid-main-testdata.WzDWCj/results.jsonl`
- main 指定恢复任务/Yak 查询对照：`/private/tmp/aid-main-more.5UGYv2/results.jsonl`
- main Skip 对照：`/private/tmp/aid-main-plan.VAUIlv/results.jsonl`
- main Redo 对照：`/private/tmp/aid-main-redo.Tz9cuy/results.jsonl`
- main aireact 追加三项对照：`/private/tmp/aid-main-extra.hj8g0e/results.jsonl`
- 计划审查独立复验：`/private/tmp/aid-review-repro.WmtzRY/results.jsonl`；main 对照：`/private/tmp/aid-review-main.rDnuXh/results.jsonl`
