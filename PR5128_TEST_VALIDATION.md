# PR #5128 合并后回归验证

## 实际版本

- 查询时间：2026-09-23 03:54 UTC。
- PR #5128：已合并，原 base `28c60c153f94327e553050149f3d85c7d8062816`，原 head `c1f1c9f06c575667806ba1f9eb8e7e4bf5e81980`，合并提交 `5f541e6727a97dbaa27fed9d3a48793b174f7014`。
- 最初测试基线：`origin/main` 的 `5f541e6727a97dbaa27fed9d3a48793b174f7014`；工作副本最初无本地改动。
- 新 PR 的最终主分支基线：`56f708b340a51308ef657ea6f06cc4eac783a63d`（2026-09-23 03:59 UTC 再次查询）。期间 `main` 新增两笔与本轮文件不重叠的 SyntaxFlow 提交；本分支已变基到最新 `main`。
- 本轮分支：`codex/pr5128-main-regression`；变基后重新执行四个涉及包的全部新增定向测试，均通过。
- 环境：Go 1.22.12，darwin/arm64，CGO_ENABLED=1。
- 生产文件修改：`common/aiforge/liteforge.go`、`common/ai/aid/aicommon/config.go`、`common/ai/aispec/config.go`。修复前先运行了新增断言并记录红灯，修复后没有削弱断言。

## 测试结果

| 任务 | 新增测试 | 实际执行命令 | 结果 | 关键证据 |
| --- | --- | --- | --- | --- |
| T1 | `TestPR5128_AuxiliaryLifecycle_*`、`TestPR5128_SpeedLoop_LifecycleAcrossRounds` | 定向命令 A | 通过 | 真实 Config → 注册桥 → LiteForge → Coordinator；流尚未结束时，带标签的事件/热补丁循环均被观测到；12 次辅助调用逐次回到 0/0 基线，父上下文仍存活。覆盖成功、最终错误、单请求取消、分段字段流和两轮 Speed loop。 |
| T2 | `TestPR5128_ModeSnapshot_*`、`TestPR5128_TierRole_ModelConfigCanRefresh`、`TestPR5128_ChildConfig_InheritsParentModeAcrossGlobalChange` | 定向命令 B | 通过 | 同一旧 Config 在全局开关双向变化后，模式、Skip/Run、禁用项、消费 payload 和 Quality/Speed/Vision 的实际 provider/model 保持一致；模型配置能刷新；新独立 Config 使用新全局模式；经 `ConvertConfigToOptions` 派生的子 Config 继承父模式。 |
| T3 | `TestPR5128_ModelInfoCallback_*`、`TestPR5128_ModelInfoConfirmCallback_*` | 定向命令 C、D | 通过 | 两个公开入口均验证未命名/命名函数 typed-nil、旧命名二参数函数；本地 gateway 实际触发回调并检查 provider/model/thinking level；失败请求不触发 confirm，禁用 fallback。 |
| 原有回归 | 单模型选项、显式 Speed callback、子配置继承、LiteForge Speed loop、模型信息回调 | 对应三个包的 `go test -run` 命令，见下文 | 通过 | 五个指定旧测试均实际匹配并执行。 |

定向命令：

```bash
# A
go test ./common/aiforge -run '^TestPR5128_(AuxiliaryLifecycle|SpeedLoop)' -count=1 -p 1 -timeout=5m
# B
go test ./common/ai/aid/aicommon -run '^TestPR5128_(ModeSnapshot|TierRole|ChildConfig)' -count=1 -p 1 -timeout=5m
# C
go test ./common/ai/aispec -run '^TestPR5128_ModelInfo' -count=1 -p 1 -timeout=3m
# D
go test ./common/ai -run '^TestPR5128_ModelInfo' -count=1 -p 1 -timeout=5m
```

原有回归命令：

```bash
go test ./common/ai/aid/aicommon -run '^(TestWithTieredAICallbackSingleModelOption|TestCustomSpeedCallbackKeepsOptionSemantics|TestSingleModelSchedulingOptionInheritedByChild)$' -count=1 -p 1 -timeout=5m
go test ./common/aiforge -run '^TestSpeedLoopLiteForgeIntegration$' -count=1 -p 1 -timeout=5m
go test ./common/ai -run '^TestModelInfoCallbacksIncludeThinkingLevelAndKeepLegacyCompatibility$' -count=1 -p 1 -timeout=5m
```

## 修复前红灯与归因

- **T1：子协调器未及时回收。** 分段字段流接受完最终内容且 `ScheduleAuxiliaryTask` 返回后，父上下文仍为 `nil` 错误，但带同一调用标签的事件循环和热补丁循环分别保留 1 个，超出 0/0 基线。匹配栈为 `Config.StartEventLoopEx.func1.1`（`config_inputevent_loop.go:87`）和 `Config.StartHotPatchLoop.func1.1`（`config_hotpatch.go:29`）。`LiteForge.ExecuteEx` 原来直接把存活的父上下文传给子 Coordinator，结束时没有取消。PR #5128 的调度器和 Speed round 接入放大了这条已有 LiteForge 生命周期路径；不能据此断言基础问题首次由 #5128 引入。修复在 `liteforge.go` 中为单次调用建立派生上下文，完成流解析和回调后取消它。
- **T2：同一 Config 的有效模式被全局变化改写。** 修复前 off→on 后旧实例返回 `true`，on→off 后旧实例返回 `false`，均与创建时已经固定的 tier callback 角色相冲突；子系统禁用项及消费 payload 仍保持创建时值。根因是 `Config.IsSingleAIModelMode` 在每次调用时 OR 实时全局开关。现在 `NewConfig` 在选项应用完成后固定有效模式，`ConvertConfigToOptions` 显式传递父会话快照；角色下的模型配置仍按调用时最新值读取。
- **T3：回调动态类型处理不完整。** 修复前未命名二/三参数 typed-nil 被包成非 nil 回调，真正调用时会 panic；旧式命名二参数函数被记录为 unsupported 并静默丢弃。现在先识别 typed-nil，再仅对与原二参数签名完全可转换的命名函数做转换。两个公开入口共享同一 normalizer。

## 重复性与竞态

- `go test ./common/ai/aid/aicommon ./common/aiforge -run '^TestPR5128_' -count=10 -p 1 -timeout=15m`：通过。
- `go test -race ./common/ai/aispec ./common/ai ./common/ai/aid/aicommon ./common/aiforge -run '^TestPR5128_' -count=1 -p 1 -timeout=15m`：`aispec` 和 `ai` 两包的新增测试已通过 race；`aicommon`、`aiforge` **执行受阻**。后两包的 race 依赖编译阶段磁盘空间耗尽，Go 报 `no space left on device`，尚未运行；不能视为竞态测试通过或产品断言失败。失败时主机剩余空间约 1.9 GiB，Go 构建缓存约 160 GiB，本轮临时构建目录随后由 Go 清理。
- 未运行所有 `./...` 测试；该仓库包含依赖真实模型/外部环境的测试，PR 的 GitHub CI 将执行仓库配置的完整门禁。

## 边界

- 子 Config 通过实际 `ConvertConfigToOptions` 路径派生，因此按父会话延续处理；`coordinator_invoker.go` 与 P&E 派生路径都使用该转换。新建独立 Config 仍从最新全局开关解析模式。
- 生命周期观测仅统计同时带本测试唯一标签、且栈属于真实子事件循环/热补丁循环的 goroutine 数量；未使用总 goroutine 数差值，也没有先取消父任务。
