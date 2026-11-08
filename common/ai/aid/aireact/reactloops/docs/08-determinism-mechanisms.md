# 08. 长链路稳定机制：感知 / 卡顿监测 / TODO 软检查点

ReAct 主循环依靠确定性的运行时信号维持长链路稳定，不再为每轮行动额外调用 AI 做自我评价，也不再使用通用重复行动检测来强制退出。

| 机制 | 解决的问题 | 是否新增 AI 调用 |
|------|------------|------------------|
| Perception | 按需刷新环境和任务态势 | 仅在既有感知流程需要时 |
| Stall heartbeat | 识别无推进并在硬阈值取消任务，排除活跃工具与近期子代理进展 | 否 |
| Finished TODO Checkpoint | 拦截仍有开放 TODO 的结束请求 | 否，复用下一轮主循环 |
| CURRENT TODO Checkpoint | 长时间聚焦同一事项时克制地校正路径 | 否，复用下一轮主循环 |

## 8.1 Perception

Perception 负责形成当前任务的结构化态势。普通反馈仍通过 timeline、ReactiveData、operator feedback 和下一轮主循环交给模型处理。工具失败不会启动额外的评价调用。

即时刷新只保留明确的运行时触发，例如强制刷新和 loop 切换；普通轮次继续受原有频率控制。

## 8.2 自动满意度核验已移除

普通 ReAct 工具完成后不再触发满意度核验，也不创建对应 watchdog、token/轮数门或核验快照。旧周期配置仅兼容读取，不再参与运行时。任务退出仍由 finish 的 TODO、Goal 与子代理检查控制；独立 stall heartbeat 继续监测卡顿。HTTP fuzz 的业务核验仍显式调用 VerifyUserSatisfaction。

## 8.3 Finished TODO 软检查点

只有模型选择 `finish` 且当前 task scope 仍有开放 TODO 时，主循环继续，并为下一轮排队简短提示：

```text
[FINISH BLOCKED BY TODO]
```

提示要求推进具体未完成项，不能为结束而清空 TODO。重复 `finish` 不会绕过开放 TODO；当前任务无开放 TODO 时，首次请求即可结束，不再要求固定的确认次数。阶段性 `directly_answer` 不触发这个检查点。提示注入前会重新核对任务作用域和开放 TODO，丢弃已失效的检查点。

默认关闭耗时同步初始化时仍执行本地问候识别。简单问候交付答复后，若本轮无有效 `todo_delta` 且当前任务没有 TODO 历史，宿主直接结束；同任务已有开放或关闭历史时仍走普通 finish 流程。

Goal mode 的最小迭代门禁优先于 Finished checkpoint。

## 8.4 CURRENT TODO 25 轮软检查点

每个 ReAct task scope 独立记录当前 TODO 和连续有效迭代数。有效迭代必须满足：

1. action 已成功解析和校验；
2. `todo_delta` 已应用；
3. ActionHandler 已实际返回；
4. action 不是 `finish`。

handler 返回业务失败仍表示进行了一次真实尝试，因此计数。AI transaction 重试、流式字段、解析或校验失败、以及没有进入 handler 的失败不计数。

同一 CURRENT TODO 达到 25 次后，只为下一次正常主循环请求排队：

```text
[CURRENT TODO CHECKPOINT]
```

注入后立即从零重新计数；同一 TODO 再执行 25 次可再次触发。current 切换、关闭或清除会重置；同轮切换 current 时，该 action 计为新 current 的第 1 轮。checkpoint 注入前 current 已变化时，过期 checkpoint 被丢弃。

Finished 和 CURRENT checkpoint 同时存在时，只注入 Finished checkpoint，并从零开启后续 CURRENT 计数窗口。

## 8.5 Prompt 边界

两种 checkpoint 共用 `LoopPromptAssemblyInput.TodoCheckpoint`。动态区固定顺序为：

```text
UserQuery
AutoContext
ExtraCapabilities
ReactiveData
InjectedMemory
TodoCheckpoint
PROMPT_SECTION_dynamic_END
```

`TodoCheckpoint` 是纯动态区域最后一个内容块，不进入 high-static、frozen、semi-dynamic 或 timeline-open，也不会被 lightweight prompt 裁剪。Prompt observation 将它记录为独立的 `section.dynamic.todo_checkpoint` 子项。

## 8.6 错误处理原则

- 工具或能力失败：写入普通 feedback 和 timeline，由下一轮主决策选择替代路径。
- 可确定识别的重复读取、重复查询或路径失败：保留局部、无 AI 调用的保护。
- 不因 action 类型相同就判定任务无进展；长链路校正由 CURRENT TODO 的 25 轮软检查点承担。
- checkpoint 只提示模型检查，不制造 TODO 修改，也不伪造完成。

## 8.7 移除自动核验后的 token 测量

2026-11-08 使用本地已配置的 `deepseek-v4.1-flash`（经 aibalance）做同任务回放。基线为 `9d21a5b1834d36cf440e8e68883cb8b91aa88749`，两侧均使用真实 default loop、工具、prompt 构建和 finish 路径：24 批合成来源数据，每批 230 行，25 次主决策。主模型动作固定；仅旧版实际触发的满意度请求发送给 DeepSeek，使用接口返回的 usage 计数。

| 情形 | 修改前核验次数 | 修改后核验次数 | 修改前核验输入 token | 修改前核验输出 token | 直接节省 token |
|------|---------------:|---------------:|--------------------:|--------------------:|---------------:|
| cold：未建立核验基线，回放无两分钟空闲窗口 | 0 | 0 | 0 | 0 | 0 |
| warm：模拟已有一次 watchdog 核验建立的基线 | 10 | 0 | 1,345,842 | 839 | **1,346,681** |

warm 只在第一轮初始化旧版核验快照，之后保持生产 token/轮数门不变，不使用 interval=0 强制每轮核验。初始化快照本身没有发送 AI 请求，其代表的前序 watchdog 请求不计入测量。因此 warm 结果描述的是**进入周期检查后**的一段长上下文任务，不能解释为所有默认任务都会多调用十次。两侧、两种情形都实际完成了 24 批工具操作。

该 warm 样本每次核验平均约 13.47 万 token。其他任务可按 `实际核验次数 × 每次核验平均（输入 + 输出）token` 估算直接节省；短上下文、稀疏触发时会小得多。这也取消了核验 API 错误或阻塞影响普通工具后续执行的路径。

主决策 prompt 的本地累计估算为：warm 修改前 3,643,560、修改后 3,624,215；cold 修改前 3,636,465、修改后 3,626,015。这些是本地估算，不与 provider usage 混合计算“整任务实际计费下降百分比”。本实验没有自主主模型 A/B，不证明回答语义、任务成功率或提前 finish 概率相同；缓存命中及实际费用也未折算。

原始无密钥结果保存在 [verification_token_replay](../../testdata/verification_token_replay/)，回放代码为 [verification_token_replay_test.go](../../verification_token_replay_test.go)。默认测试会跳过联网实验。复现时在基线和修改后分别编译同一回放文件，并设置：

```powershell
$env:YAK_VERIFICATION_TOKEN_LIVE = '1'
$env:YAK_VERIFICATION_TOKEN_CONFIG_DB = '<本地 profile 数据库路径>' # 可选，只读读取配置
$env:YAK_VERIFICATION_TOKEN_PROFILE = 'warm' # 或 cold
$env:YAK_VERIFICATION_TOKEN_VARIANT = 'baseline' # 修改后用 removed
$env:YAK_VERIFICATION_TOKEN_OUTPUT = '<结果 JSON 的绝对路径>'
go test ./common/ai/aid/aireact -run '^TestVerificationTokenReplay$' -count=1 -v
```

`TestDefaultLoopDoesNotAutomaticallyVerifySatisfaction` 另以禁止核验的 callback 覆盖 native/文本协议、scalar/batch/工具派生 action、开放 TODO 后请求 finish，以及旧周期配置 0/20，共 16 组。它检查六个真实本地文件操作、超过旧 token 门的上下文增长、零核验调用及工具活动钩子；finish 的已有 TODO、Goal、子代理保护测试继续保留。

## 相关源码

- [soft_todo_checkpoint.go](../soft_todo_checkpoint.go)
- [prompt.go](../prompt.go)
- [perception.go](../perception.go)
- [loop_stall_heartbeat.go](../loop_stall_heartbeat.go)
- [verification.go](../../verification.go)
