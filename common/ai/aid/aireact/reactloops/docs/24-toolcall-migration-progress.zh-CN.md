# 24. 工具调用流程迁移：进度与待办

## 进度快照

- 核对日期：2026-09-30。
- 当前分支：`feat/require-tool-schema-only`。
- 非批量代码最近提交：`0b44159ec8`；本轮代码提交见第 5 节。
- 用户要求按改动范围分离，不要求各提交可独立编译；非批量改动已分 commit 提交，批量实现和相关测试此前留在工作区，现已获用户批准随本次提交保存。
- 已提交旧工具参数生成模块清理、review 和单工具请求层改造；本轮清理 batch require 的参数生成/执行路径；按用户最终要求，保留同一个 require action 的 Schema-only 批量选项，真实并发执行只走直接调用批次。本轮改动随本次提交保存。
- 包含批量草稿的完整工作树此前通过五包回归；这不表示排除批量改动后的提交快照可独立编译。

## 1. 最终目标与约束

1. action 与原生 functioncall 的 `require_tool` 都只加载工具 schema，不生成参数、不执行工具；支持单个工具和同一 action 的批量 Schema 选项，多次 require 仍复用普通 `execCalls`。
2. AI 观察 `CACHE_TOOL_CALL` 后，通过 `directly_call_tool` 提交带明确参数的调用。
3. 指定工具的新提案复用 `generateLoopPrompt` 和 `callAILoopTransaction`，最多三次，校验目标且禁止额外调用。
4. `ToolCaller` 只执行明确参数；需要 AI 修正时由 review 或 ReAct 请求层发起。
5. 临时提示词只追加在本次 prompt 尾部，不增加 loop 字段、不替换 task/emitter、不新增 forced-call 锁。
6. 保留审批、guard、实际工具的 mutator、批量限流、checkpoint、结果顺序和单次收尾。
7. 本轮不清理 `lastNativeTools`、其他 `last*` 字段或响应缓存，也不改变原生多 action 的调度语义。

## 2. 已完成

### 已提交的前置工作

- [x] action/native `require_tool` 统一为 schema-only；保留同一个 require action 的批量 Schema 加载选项，但不会进入参数生成或工具执行器。
- [x] 迁移 `loopinfra`、`aireact`、`aid/test` 相关主流程 mock。
- [x] 指定工具调用复用主循环 prompt/transaction，支持 action JSON、原生 functioncall、原生分片流和 AITAG。
- [x] 删除强制调用的 task/emitter 替换、包装 task、专用 mutex 与 `TemporaryInstruction`。
- [x] 保留显式 context、caller options、单次结果收尾；`buildSchema` 恢复为包内函数。

### Review 与请求层：非批量已提交，直接批量改动随本次提交保存

- [x] 抽出 `requestToolCallParamsForTask`，只加载 schema、请求并解析提案，不执行工具、不写工具结果。
- [x] `wrong_params` 不再单独拼装旧 prompt/解析 `call-tool`，改为在共享请求尾部追加 review 反馈。
- [x] `wrong_tool` 保留选工具/澄清/放弃职责，选择新工具后在 review 内请求新参数，不再调用 `t.CallTool(newTool)`。
- [x] 人工已提供参数时直接使用；换工具即使参数相同也不继承旧审批，仍审核新提案。
- [x] 缺失回调、空的新工具、参数请求失败均 fail-closed，禁止执行已被拒绝的提案。
- [x] 单工具 guard/mutator 使用实际工具名，不闭包绑定原工具；每个提案只应用一次 mutator。
- [x] 删除批量 require 的初始参数请求、参数 semaphore/sequence 和配置；batch request 不再区分 require/direct mode。
- [x] 直接批量只调用 `CallToolWithExistedParams`；review 内仍能请求修正提案，受已有有序审批边界保护。
- [x] 保留批量审批顺序、invoke barrier、调用 ID、结果顺序、失败后兄弟项继续、直接回答全批取消与单次结果收尾。
- [x] review 参数请求取消映射到 cancelled，不重新进入 AI retry；真实工具执行 semaphore 保留。
- [x] 保留直接调用 reason 优先、human_readable_thought 兜底；review 校验只检查业务参数，不将保留的调用元数据当成业务字段。

### 旧模块清理：已提交

- [x] 删除 `generateParams`、`GenerateParamsResult`、旧 text/native 参数生成器、固定参数提交协议和专用测试。
- [x] 删除普通工具的参数 prompt builder、固定 functioncall 参数 schema 及其专用测试。
- [x] 删除参数生成 builder/options、caller 内参数 semaphore/sequence、`CallTool` 隐式入口与 direct-call 生成 fallback。
- [x] `CallToolWithExistedParams(tool, params)` 只接收明确参数，零参数工具使用空对象，不触发参数生成。
- [x] `DirectlyCallPrepareFunc` 移除生成 fallback 返回值，只返回参数、工具或错误。
- [x] 清理仅供旧参数生成流使用的 reason 标志及 artifact 参数生成耗时/原始 AI 响应字段。
- [x] 保留通用 AITAG、review、执行、reason 和 artifact 机制。
- [x] 保留 blueprint 参数生成共用的 instruction/output-example/dynamic 模板：blueprint 并非本轮移除的工具参数生成路径。
- [x] 保留既有参数引用/工具回调所有权，以及校验失败返回可观察 ToolResult、事件和 timeline 的契约。

## 3. 回归覆盖

- [x] action/native 下 wrong_tool 与 wrong_params：请求阶段不执行、第二次审批前不执行、最终仅执行一次且结果仅收尾一次。
- [x] 换工具但参数相同仍重新审批；显式人工编辑无需请求 AI；实际新工具的 mutator 与 guard 生效。
- [x] 缺失处理器、nil replacement、参数生成失败、取消、零参数工具、三次目标错误与额外调用拒绝。
- [x] 参数请求保留 owning task/context，主循环工具缓存包含冻结/未冻结 timeline 中的 schema。
- [x] 删除仅覆盖旧 batch require 参数生成/AITAG 的测试，保留直接批量的参数隔离、实际并发和 checkpoint 回放。
- [x] 原生同轮多次单工具 require：加载全部 Schema，零工具执行、零额外参数生成请求。
- [x] action 多轮单工具 require 后直接 batch：缓存可见、参数归属正确、工具回调实际并发，结果按声明顺序提交。
- [x] 两种 require schema 和提示词都发布 Schema-only 批量选项；批量不生成参数、不执行业务工具、不调用 batch executor；执行参数及 scalar/batch 混用在缓存前拒绝。
- [x] 审批 checkpoint 身份、批量 mutator 顺序、失败 all-settled、取消后的 semaphore 释放与单次结果写入。
- [x] 业务参数 identifier/call_expectations 不被误删；保留键元数据仍在执行前被取出。
- [x] 非法明确参数不重新请求 AI、不执行工具回调，保留原有可观察失败结果。

## 4. 剩余待办

- [x] 最后两个边界补充完成专项及完整复验，记录最终结果。
- [x] 最终 gofmt、`git diff --check` 与旧生产符号引用检查。
- [x] 按用户要求，将非批量的 review/请求层迁移、旧模块清理、测试迁移分 commit 提交。
- [x] 根据用户最终要求，恢复同一个 require action 的批量 Schema 选项；真实批量执行只保留 directly_call。
- [x] 完成本轮受影响包完整回归、最终 gofmt 和引用审计并记录结果。
- [x] 用户已确认批量调整并要求提交，随本次提交保存。

此前的分离提交仅用于隔离已认可代码与待评审批量改动，不补入批量适配来保证提交快照独立编译，也不运行独立提交回归；此前五包测试结果属于包含批量草稿的工作树。本次批量调整的七包回归结果见第 7 节。

分离提交时保留在工作区的六个文件（历史快照；本轮同步改动已扩展到 schema、prompt、配置和相关测试，旧 require 专用测试文件已删除）：

- `common/ai/aid/aireact/invoke_toolcall_batch.go`
- `common/ai/aid/aireact/invoke_toolcall_batch_hardening_test.go`
- `common/ai/aid/aireact/invoke_toolcall_batch_params_regression_test.go`
- `common/ai/aid/aireact/invoke_toolcall_batch_require_compat_test.go`
- `common/ai/aid/aireact/invoke_toolcall_batch_test.go`
- `common/ai/aid/aireact/reactloops/loopinfra/tool_batch_action_test.go`

## 5. 提交记录

| 日期 | Commit | 内容 |
| --- | --- | --- |
| 2026-09-29 | `b67f62433` | require_tool 只加载 schema，统一 action 与 functioncall |
| 2026-09-29 | `c269fd912` | 迁移 loopinfra 测试 |
| 2026-09-29 | `ff55c4971` | 迁移 aireact 集成 mock |
| 2026-09-29 | `2e21e5982` | 迁移 aid/test 协调器测试 |
| 2026-09-29 | `ab02f6683` | 引入 forced directly-call；手工 prompt/解析随后被替换 |
| 2026-09-30 | `dabf8ed4c` | forced 调用复用主循环 transaction，清理不必要状态并补回归 |
| 2026-09-30 | `d8f4921cce` | review 与指定工具参数请求复用主循环 transaction |
| 2026-09-30 | `07634f4eb1` | 删除旧工具参数生成模块、协议和专用测试 |
| 2026-09-30 | `0b44159ec8` | 非批量回归测试与 mock 迁移 |

## 6. 验证记录

以下为分离提交前完整工作树的测试记录，包含当时留在工作区的批量草稿，不能作为此前独立提交快照编译或回归通过的证明。

2026-09-30，本轮首次完整回归发现参数引用语义改变和提前校验破坏失败 ToolResult 契约，均已恢复既有行为；未修改测试来绕过契约。

恢复后五包完整回归通过（日志 `/tmp/toolcall-final.log`）。额外补充 reason/严格 schema 元数据边界后，专项测试通过（`aicommon` 1.143s、`aireact` 2.814s，日志 `/tmp/toolcall-audit-focused.log`）。

最终五包完整复验（日志 `/tmp/toolcall-final-verification.log`）：

| 包 | 结果 | 耗时 |
| --- | --- | --- |
| `common/ai/aid/aicommon` | 通过 | 74.115s |
| `common/ai/aid/aireact/reactloops` | 通过 | 36.064s |
| `common/ai/aid/aireact/reactloops/loopinfra` | 通过 | 31.174s |
| `common/ai/aid/aireact` | 通过 | 151.356s |
| `common/ai/aid/test` | 通过 | 183.056s |

所有修改的 Go 文件及新增测试均通过 gofmt 检查，`git diff --check` 通过；旧生成器、参数 prompt builder、固定提交协议、caller 参数生成 gate/sequence 的生产引用已清零。

测试使用临时 YAKIT_HOME 和 /tmp/go-cache，避免默认用户数据目录只读：

```sh
test_home=$(mktemp -d /tmp/toolcall-migration-test.XXXXXX)
YAKIT_HOME="$test_home" GOCACHE=/tmp/go-cache go test \
  ./common/ai/aid/aicommon \
  ./common/ai/aid/aireact/reactloops \
  ./common/ai/aid/aireact/reactloops/loopinfra \
  ./common/ai/aid/aireact \
  ./common/ai/aid/test \
  -count=1 -timeout 6m
```

临时日志可能随目录清理而消失，本文保留验证结果快照。

## 7. 本轮 Schema-only require / 直接批量调整（随本次提交保存）

1. 协议和 prompt：同一个 require action 支持单工具 `tool_require_payload` 和批量 `tool_require_calls` 选项；AI 决定申请哪些 Schema 和调用次数。两种选项都只向 CACHE_TOOL_CALL 添加 Schema，不生成参数、不执行工具。多次原生 require 走现有 `execCalls`，不把 Schema 加载当作执行批次。
2. 执行器：batch 只接受明确参数的直接调用，保留插件并发与有序 join；初始阶段不请求 AI。review 的新提案仍由 review 请求并再次审批。
3. 清理：删除 require 批量执行 parser、Mode 字段、参数生成限流配置、预留 parameter sequence、ordered mutator 专用分支与旧生成测试；保留 require 批量 Schema 选项和解析/示例。
4. 安全与兼容：保留标量 verifier 的流式优先级；在加载 handler 缓存前校验完整 action；第三方串行 fallback 仅执行明确参数，支持无参数工具。
5. 本轮七个受影响包完整回归均通过；第 6 节通过记录仅属于此前工作树。

### 本轮验证结果

首次七包完整回归中，六包通过；`reactloopstests` 的两个迭代上限测试仍通过旧参数生成请求统计调用次数。已迁移为直接调用提案，并在真实工具回调中统计执行次数；`load_capability` 的指定工具请求 mock 优先处理主循环的 forced directly-call 提案。相关专项通过（1.802s），随后该包完整复验通过。

| 包 | 结果 | 耗时 |
| --- | --- | --- |
| `common/ai/aid/aicommon` | 通过 | 79.363s |
| `common/ai/aid/aireact/reactloops` | 通过 | 35.466s |
| `common/ai/aid/aireact/reactloops/loopinfra` | 通过 | 31.776s |
| `common/ai/aid/aireact` | 通过 | 148.125s |
| `common/ai/aid/aireact/reactloops/loop_default` | 通过 | 3.468s |
| `common/ai/aid/aireact/reactloops/reactloopstests` | 完整复验通过 | 71.188s |
| `common/ai/aid/test` | 通过 | 183.032s |

日志：`/tmp/toolcall-schema-batch-verification.log`、`/tmp/toolcall-schema-batch-loop-migration.log`、`/tmp/toolcall-schema-batch-loops-verification.log`。两次完整测试均使用临时 YAKIT_HOME 和 `/tmp/go-cache`。

所有修改的 Go 文件通过 gofmt，`git diff --check` 通过。生产代码不再引用 require 批量执行 parser、require/direct Mode 或参数生成并发配置；批量 require 选项仅进入 Schema 加载路径。所有本轮改动已获用户批准，随本次提交保存。
