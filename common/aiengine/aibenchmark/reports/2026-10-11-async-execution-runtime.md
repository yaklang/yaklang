# aim 异步执行真实模型验收与 defer 修复复测

日期：2026-10-11。Windows / Go 1.22.12。基线为 `eec514fcc`；复测从该提交加本次 VM、提示词改动编译，未使用旧版二进制。

## 结论

**本次案例验收通过：控制链路与脚本清理均通过，缓存未出现明显下降。** 原生 function call、文本 action 各复测四个场景，8/8 场景通过控制与清理判定，合计 134 项独立检查。两个验收器均退出 0。

基线中 14 个已进入 Yak 回调后被取消的任务全部跳过 defer；修复后对应 14 个任务均写出 `.cleaned` 并输出 `MONITOR_CLEANED`。另有两个已登记、尚未进入回调的任务被取消，没有脚本资源需要清理。正常完成与预期 panic 的清理也通过。

文本路径仍有 4 次缺失 action 的格式重试，最终均恢复。逐条核对响应后，其中 3 次为 DSML、1 次为提示词片段回显；不能把缺失 action 的次数全部算成 DSML。提示词没有彻底解决输出格式问题，验收通过不代表每个请求一次成功。

## 本次改动

- VM 执行已登记 defer 前切换到保留 context values 的清理 context，退出后恢复原 context，避免 owner 取消直接跳过清理指令。
- 正常清理不新增时间限制；owner 取消后提供 5 秒清理窗口。嵌套函数与 defer 共用窗口，不能逐层重新计时。取消状态与错误仍保留。
- 窗口约束 VM 指令及响应 context 的阻塞操作；忽略 context 的原生 Go 调用仍可能阻塞，不能承诺强制终止任意外部代码。
- 管理 action 的中文说明、默认循环静态指令与动态 Dump 明确：`manage_async_execution_runtime` 是始终可用的顶层 action，无需 require_tool，不能作为 directly_call_tool 的目标。回执不是成功结果；wait 超时不停止任务；stop 后等待终态；主任务结束取消未完成执行；减少全量输出轮询。
- 文本模式补充 JSON action 与 `@action` 约束。实际请求已包含这些指令；案例任务移除了额外的“它是顶层 action”说明，复测依赖框架提示词。

实现位置：`common/yak/antlr4yak/yakvm/vm_defer.go` 与 `vm_exec.go`。一个综合测试 `common/yak/aiagent_defer_cancel_test.go` 覆盖正常退出、panic、主动取消、deadline、清理期间取消和嵌套阻塞清理共享预算。嵌套阻塞约 5.04 秒返回，未延长为两份窗口。

## 配置与案例

- 主循环使用用户提供的 aibalance Key、`deepseek-v4.1-flash`，没有模拟 AI 回答。复测采集的 126 个 HTTP 请求正文中的 model 均为该模型。Key 仅在进程环境中注入，不写入脚本、报告或采样文件。
- 使用 `aim.InvokeReAct`、真实 `ai.Chat` 流式回调、正常 action 解析及真实 ToolCaller。显式 callback 采集 prompt、响应和使用量，保留框架 tool call / finish reason callbacks，不预设模型动作。
- 工具经 `aiagent.ParseYakScriptToAiTools`、`db.SaveAIYakScript` 注册到隔离 `YAKIT_HOME`，持续请求本机 HTTP 服务，每秒输出采样；支持一次探测、正常完成、600 秒长任务和预期 panic。
- HTTP oracle 固定返回 `bytes_per_second=128 / connections=3`。这验证真实模型与 Yak 工具调用链，没有验证操作系统抓包、网卡权限或真实流量统计准确性。
- 状态与输出来自 loop manager 和原始 ToolCaller Inspector。采样文件只作证据；主任务结束后验收代码仅 Wait / Inspect，不额外 StopAll / Shutdown 掩盖生命周期问题。
- 内置辅助调用仍有 `memfit-light-free` 等感知活动，绕过这里显式 callback 的采集。请求数与缓存统计不代表整个引擎的全部调用或总费用。

## 复测结果

请求数按实际 HTTP 请求计数；重试时 finish callback 可重复触发，不能用来统计请求数。

| 场景 | 原生：耗时 / 请求 | 文本：耗时 / 请求 | 后台执行数（每轮） | 控制 / 清理 |
|---|---|---|---:|---|
| 单调用完整生命周期 | 47.71 秒 / 21 | 58.10 秒 / 20 | 2 | 通过 / 通过 |
| 混合同步与异步批量 | 49.57 秒 / 17 | 105.02 秒 / 20 | 3 | 通过 / 通过 |
| 容量拒绝与预期故障 | 44.04 秒 / 13 | 53.86 秒 / 22 | 5 | 通过 / 通过 |
| 主任务结束自动取消 | 11.83 秒 / 5 | 26.22 秒 / 8 | 1 | 通过 / 通过 |

八个场景正常返回，无主任务 abort。原生合计 153.15 秒 / 56 请求，文本 243.19 秒 / 70 请求；基线分别为 162.42 秒 / 54 请求、240.97 秒 / 61 请求。单轮耗时与请求次数仍有波动，不能据此宣称提示词降低延迟或消除了重试。

| 检查 | 真实证据 |
|---|---|
| 默认同步 | instant 完成并清理后返回，没有异步登记 |
| 单调用异步 | 独立 execute_id、增长的 Inspector stdout，后续 prompt 有 YAML 状态 Dump |
| 等待超时 | wait(0) 反馈 wait_timed_out=true、state=running，没有取消任务 |
| 停止与清理 | stop 后 cancelled，Wait 返回，Inspector 含清理输出且 `.cleaned` 存在 |
| 正常结果 | 两秒采样完成，ToolResult 成功、label 正确、samples=2、清理标记存在 |
| 混合批量 | 同组三项 async=true / 默认 false / true；同步完成，异步回执独立 |
| 停止隔离 | 快照出现 mixed_a=cancelled、mixed_b=running，各 label stdout 不串流 |
| stop_all 可复用 | 停止已有执行后，after_stop_all 新执行完成并清理 |
| 容量拒绝 | 五项仅四项接收、一项拒绝；原有 invoke 并发为 3，第四个已接收项进入 Yak 前取消 |
| 失败观察 | 预期 panic 为 failed，stderr 保留故障标记，defer 运行，未报告业务成功 |
| 无效 ID | Inspect 不存在 ID 返回可恢复错误，主循环继续完成 |
| 所有者结束 | 未调用 stop / stop_all，finish 后 owner_hold cancelled、Wait 返回、defer 运行 |

复测原生 owner_hold 的实际观察摘录：

```yaml
execute_id: exec-ff4ff2abc1140a2da6e8aa1dda69717f
state: cancelled
cancel_reason: context canceled
wait_error: null
inspect_error: null
callback_started: true
script_defer_ran: true
stdout_contains: MONITOR_CLEANED label=owner_hold
```

两轮共登记 22 个后台执行：4 completed、2 failed、16 cancelled。取消原因可来自上游 context 或 manager 关闭；验收依据终态、原始观察与清理标记，不依赖原因固定为某一句文本。

## 缓存表现

统计只取 `ai_call_summary` 中 `caller_label=react-loop:default`、`token_source=usage` 的服务端 usage。命中率为 **Σ cache_hit_token / Σ input_tokens**，不是请求百分比的平均值；未返回有效 usage 的请求不纳入分母。

| 调用方式 | 基线命中率 | 复测命中率 | 变化 | 复测输入 / 缓存 token | 有效 usage 调用 |
|---|---:|---:|---:|---:|---:|
| 原生 | 87.22% | 86.91% | -0.31 个百分点 | 1,490,004 / 1,294,976 | 43 |
| 文本 | 90.53% | 89.49% | -1.04 个百分点 | 1,851,518 / 1,656,960 | 55 |

扣除每个场景首个有效 usage 调用后，基线 / 复测分别为：原生 88.94% / 89.16%，文本 93.86% / 93.00%。这是后续请求统计，不能保证已经完全预热。静态指令、场景路径与输出长度变化也会影响前缀。

本轮没有看到缓存明显失效。不过比较的是带异步功能的修复前后，没有“同一输入、无异步展示”的独立对照，不能定量归因异步系统本身的缓存成本。

复测主循环 prompt 估算 token 范围：原生 27,810–50,669，文本 31,047–45,373，包含系统提示、schema、Timeline 和 Inspector 输出，不全是异步 Dump。缓存命中不等于上下文免费；反复 Inspect 全量 stdout 仍增加上下文与处理成本，提示词不能替代输出预算。

## 仍存在的限制

- 文本基线有 3 次、复测有 4 次缺失 action 的解析重试；复测为 3 次 DSML 加 1 次提示词片段回显。新 JSON 指令确实进入请求，但不足以消除格式偏移；原生复测没有此类错误。
- 复测两条路径没有管理 action 被当成业务工具的验证错误，说明这组案例可用，不能保证其他模型和任务永不混淆。
- 感知 schema 校验重试：基线原生 1 次、文本 2 次；复测均 2 次，没有中断场景。禁用配置未消除全部辅助活动，配置传播问题尚未处理。
- Inspector result 为原始 ToolResult，其 data 是格式化文本，不直接等于 Yak RESULT。本次核验了其中业务结果，未验证通用 PartialResult 生产者。
- 未覆盖真实 pcap / 浏览器资源、跨子 agent 共享、重启恢复、大规模压力、128 条保留记录上限、大输出截断。跨子 agent 协作按约定保留为待办。

## 复现与证据

案例：`common/ai/aismoking/live/async_execution_runtime.yak`。独立验收器：`common/ai/aismoking/live/async_execution_runtime_verify.py`，仅 Python 标准库。在仓库根运行；先在进程环境注入 YAK_ASYNC_API_KEY，不写入脚本或报告。Python 需使用实际安装的解释器，Windows 应用执行别名不一定可用。

```powershell
$benchRoot = Join-Path $env:TEMP 'yak-async-live-repro'
New-Item -ItemType Directory -Force -Path $benchRoot | Out-Null
$env:YAKIT_HOME = Join-Path $benchRoot 'isolated-home'
$env:YAK_ASYNC_OUTPUT = Join-Path $benchRoot 'native'
$env:YAK_ASYNC_MODEL = 'deepseek-v4.1-flash'
go build -o "$benchRoot\yak.exe" ./common/yak/cmd/yak.go
& "$benchRoot\yak.exe" common/ai/aismoking/live/async_execution_runtime.yak
python common/ai/aismoking/live/async_execution_runtime_verify.py $env:YAK_ASYNC_OUTPUT

$env:YAK_ASYNC_TEXT = '1'
$env:YAK_ASYNC_OUTPUT = Join-Path $benchRoot 'text'
& "$benchRoot\yak.exe" common/ai/aismoking/live/async_execution_runtime.yak
python common/ai/aismoking/live/async_execution_runtime_verify.py $env:YAK_ASYNC_OUTPUT
Remove-Item Env:\YAK_ASYNC_API_KEY
```

YAK_ASYNC_CASES 可指定 single,mixed,capacity_failure,owner_release 子集。采集完成不等于通过，以验收器 control / cleanup 判定为准。

证据目录：`C:\Users\V\AppData\Local\Temp\yak-async-live-20261011`。基线为 native-4 / text-1；复测为 native-fixed / text-fixed。各场景包含请求正文、prompt、解码响应、事件、状态快照、最终元信息、Inspector 输出和复制的 action 记录；verification.json 保存独立断言。未保存认证头或用户日常数据库。

验证通过：当前源码构建；综合测试 `go test -race ./common/yak -run '^TestYakDeferredCleanupAfterCancellation$' -count=1 -v -timeout 30s`；yakvm 中 Defer / CurrentFrame / Synchronous / Sandbox / Debugger / Cancel 相关回归，以及 async action、混合批量、AutoContext、管理 action、loop manager 接入相关测试以 race 模式通过。基线独立 asyncexecmanager 包的 race 检查此前已通过。

## 文本 toolcall 提示词强化复测

本轮只调整 instruction.txt 与 output_example.txt 的文本协议说明，不改解析器、调用方式或异步实现。合并重复规则，明确工具调用也输出带 @action 的 JSON，动作名与业务工具名分开填写，保留显式 AITAG 承载方式。最终版给出可执行示例 `{"@action":"manage_async_execution_runtime","op":"list"}`，避免只写抽象格式要求。

相对上一轮修改前，两份模板源码合计减少 45 个字符；扣除删除的条件模板语法后，文本模式实际渲染内容合计减少 6 个字符。没有新增固定提示段。最终例子已确认进入所有主循环实际请求，原生 FunctionCall 分支的说明保持原样。

| 版本 / 场景 | 耗时 / 实际请求 | DSML 响应 | 缺失 action 错误 | 控制 / 清理 |
|---|---|---:|---:|---|
| 第一版 / single | 55.60 秒 / 20 | 0 | 1（普通意图文本） | 通过 / 通过 |
| 第一版 / mixed | 71.46 秒 / 20 | 0 | 0 | 通过 / 通过 |
| 第一版 / capacity_failure | 57.09 秒 / 21 | 2 | 2 | 通过 / 通过 |
| 最终版 / capacity_failure | 55.16 秒 / 19 | 2 | 2 | 通过 / 通过 |

第一版三个场景对应 53 项独立检查，最终版容量场景对应 18 项，两个验收器退出 0。最终版两次 DSML 分别发生于管理 list、wait；第一版发生于管理 inspect。错误都恢复，没有误执行或清理回归。辅助感知 schema 重试仍存在：第一版 3 次、最终版 1 次。

**没有足够证据证明提示词强化明显降低 DSML。** 第一版 single / mixed 没再出现，但容量场景及补充示例后的复测仍各出现两次，不能把局部减少当作稳定改善，也不能声称问题已修复。最终版没有出现提示词片段回显，但样本同样有限。

缓存按前文相同 usage 口径：第一版三场景 90.75%（对应旧三场景 90.01%）；最终版容量场景 89.78%（旧容量场景 89.77%）。这组数据未显示缓存明显受损。

证据在同一临时根目录的 `text-toolcall-prompt-1`（三个场景）与 `text-toolcall-prompt-2`（最终版容量场景）。第一版属于中间实验，不将它的全部场景冒充最终版复测。新增共 80 个实际请求，model 均为 deepseek-v4.1-flash，未启用原生 tools 或 response_format。

最终源码构建与现有四项提示词测试通过：输出模式渲染、答案承载互斥、标量与批量选择、批次失败恢复。新采样及日志共 224 个文件的 Key 格式检查无命中。

上述真实模型采样在创建 PR 前完成。创建 PR 时已合入 main 的最新提示词简化，保留本文的文本工具调用格式说明；合并后的本地回归及 CI 单独验证，不将此前的生产采样当作合并后提示词的再次实测。
