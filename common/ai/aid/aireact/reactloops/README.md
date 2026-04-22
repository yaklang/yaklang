# ReActLoop 模块说明

## 模块概述

`reactloops` 是 Yak AI 框架中的 ReAct（Reasoning and Acting）循环实现：在多次迭代中调用模型、解析结构化 **Action**、在本地执行确定性逻辑、再把结果以 **Feedback** 与 **响应式 Prompt** 喂回模型，直到 `finish` / `directly_answer` 或达到迭代上限。

---

## 第一部分：如何创建、注册与使用

本部分说明**从零加一个新 Loop** 时要在仓库里动哪些地方，以及**产品侧 / 运行时时如何选 Loop**。若你只关心单轮里发生了什么，请看 **「第二部分：运行原理与内部机制」**。

### 1.1 新 Loop 的落地清单（文件与命名）

按顺序做即可，漏一步通常表现为「`reactloop[xxx] not found`」或工厂从未执行。


| 步骤         | 位置                                                   | 说明                                                                                                             |
| ---------- | ---------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| 1. 固定字符串名  | 例如 `common/schema/ai_event.go`                       | 增加常量，值为 **唯一** 的 loop 名（如 `my_feature = "my_feature"`）。`RegisterLoopFactory` 与 `NewReActLoop` 的**第一参数**必须与此一致。 |
| 2. 新子包     | `common/ai/aid/aireact/reactloops/loop_<name>/`      | 一个 Loop 一个包，包内 `init()` 里注册。可参考 `loop_http_flow_analyze`、`loop_default`。                                       |
| 3. 空白导入    | `common/ai/aid/aireact/reactloops/reactinit/init.go` | `import _ ".../loop_<name>"`，否则 `init()` 不跑，注册表里没有该名。                                                          |
| 4. 元数据（推荐） | 与 `RegisterLoopFactory` 同级的选项                        | 供能力检索、Yakit 展示：中英文描述、用法提示、展示名、是否隐藏等。                                                                           |


### 1.2 注册工厂：`RegisterLoopFactory`

- **注册表**：`register.go` 中按 **字符串名** 保存 `LoopFactory`。
- **创建**：`CreateLoopByName(name, invoker, opts...)` 调对应工厂，内部再 `NewReActLoop(name, invoker, preset...)`。
- **元数据**（`WithLoopDescription`、`WithLoopDescriptionZh`、`WithLoopUsagePrompt`、`WithLoopOutputExample`、`WithVerboseName`、`WithLoopIsHidden` 等）挂在同一名字下，给意图识别、Schema 的 `x-@action-rules` 与前端用。

**注意**：同一 `name` 只能注册一次；测试里用随机名避免冲突。

### 1.3 工厂函数里应装配什么（重点）

`LoopFactory` 形如 `func(r aicommon.AIInvokeRuntime, opts ...ReActLoopOption) (*ReActLoop, error)`。在返回 `NewReActLoop` 前，用 `ReActLoopOption` 把行为钉死。常见项：


| 选项                                                                                 | 作用                                                                                               |
| ---------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| `WithMaxIterations`                                                                | 最大迭代轮数。                                                                                          |
| `WithPersistentInstruction` 或 `WithPersistentInstructionProvider`                  | **长期**角色与规则（建议 `embed` 的 `prompts/persistent_instruction.txt`）。每轮进 Prompt 的 *Persistent* 区。      |
| `WithReflectionOutputExample` 或对应 Provider                                         | **输出示例/长说明**（如 `prompts/reflection_output_example.txt`），进 *OutputExample* 区，补充 `Schema` 文字说明。    |
| `WithReactiveDataBuilder`                                                          | **每轮动态上下文**：读 `loop.Get`、反馈缓冲区等，渲染 `prompts/reactive_data.tpl` 或自建模板。用于 FINDINGS、上一步结果摘要、当前中间状态。 |
| `WithRegisterLoopAction` / `WithRegisterLoopActionWithStreamField`                 | 注册本 Loop 的 **业务 Action**；参数用 `aitool.*` 描述，会进入全局合并 Schema。                                       |
| `WithOverrideLoopAction`                                                           | 覆盖默认的 `directly_answer` 等行为（如某 Loop 要发 artifact 再 `Exit`）。                                       |
| `WithAllowRAG` / `WithAllowToolCall` / `WithAllowAIForge` / `WithAllowPlanAndExec` | 是否打开通用能力（RAG、工具、蓝图、计划执行等）。                                                                       |
| `WithOnPostIteraction`                                                             | 每轮结束后的钩子（收 findings、强总结、改错误策略等）。                                                                 |
| `WithInitTask`                                                                     | 首帧任务（同步跑子流程、建目录、预检等）。                                                                            |
| `WithVar` / 工厂闭包                                                                   | 子 Loop 可注入路径、调试开关。                                                                               |


`opts ...ReActLoopOption` 会拼在 `preset` 后传入，便于调用方在 `CreateLoopByName` 时**覆盖**部分行为（测试或上层注入）。

**最小心智模型**：*Persistent* = 不变的业务合同；*ReactiveData* = 每轮变的「世界状态」；*Schema* = 模型下一跳可发的 JSON 字段集（由 Action 的 `aitool` 自动合并，见 1.4）。

### 1.4 Action 与 JSON Schema：模型怎么知道能发哪些字段

不必手写整份 `jsonschema`：

- 每个 `LoopAction` 的 `Options []aitool.ToolOption` 描述**业务参数**（如 `keyword`、`limit`）。
- `action.go` 的 `buildSchema` 会合并：**公共字段**（`@action` 枚举当前 Loop 所有动作名、`identifier`、`human_readable_thought`）+ **各 Action 展开后的字段**。
- 主模板 `prompts/loop_template.tpl` 中会把合并结果放在 `Schema` 代码块里；**每轮**都会带这份 Schema。

因此，**要扩展查询/操作能力 = 新注册一个 `WithRegisterLoopAction` 并写好 `aitool` 参数即可**；不需要改 `buildSchema` 核心逻辑。

### 1.5 运行时时如何进入某个 Loop（产品 / 用户侧）

Loop **不会自动出现**，需把「当前主循环名」设成你注册时用的字符串（与 schema 常量一致）：

1. **运行时 `config.Focus`**：默认主循环（空则回退到 `default`）。
2. **任务级 `task.SetFocusMode(loopName)`**：优先级**高于**输入内嵌指令，适合 Yakit 里点选「某模式」。
3. **用户输入内嵌**：`@__FOCUS__ <loop名>` 可在解析后剥掉并指定 focus（见 `re-act_mainloop.go` 中 `parseLoopDirectives`）。

主路径：`ReAct.executeMainLoop` → `ExecuteLoopTask(loopName, task, opts...)` → `CreateLoopByName` → `ExecuteWithExistedTask`。

**在代码里起子 Loop**（不经过用户点选）：`reactloops.CreateLoopByName(子Loop名, 同一 invoker, WithVar...)` 再 `ExecuteWithExistedTask` 或子任务，例如代码审计里嵌套 `dir_explore`。

### 1.6 单测 / 工具里直接用 `NewReActLoop`

不必每测都 `RegisterLoopFactory`：可对 **mock `AIInvokeRuntime`** 调 `NewReActLoop("test_"+nonce, r, options...)`，与生产工厂同一套 `WithRegisterLoopAction` 即可。全链路集成测再 `RegisterLoopFactory` 或调用 `CreateLoopByName`。

### 1.7 新 Loop 自查表

- `schema` 常量与 `RegisterLoopFactory` / `NewReActLoop` 名一致  
- `reactinit` 已 `import _` 你的包  
- 持久指令 +（可选）reactive 模板 + 需要时的 `WithOnPostIteraction`  
- 每个 Action 有 `ActionVerifier`（参数）+ `ActionHandler`（`Continue`/`Exit`/`Fail`/`Feedback`）  
- 元数据是否足以被意图/能力系统描述清楚

---

## 第二部分：运行原理与内部机制

本部分说明**一轮迭代里**从 Prompt 到 Action 再回 Prompt 的链路，以及与 **aireact、状态存储** 的关系。实现细节以 `exec.go`、`prompt.go`、`action.go`、`register.go` 为准。

### 2.1 与 `aireact` 的衔接

- 任务入队后 `processReActTask` → `executeMainLoop`：根据 focus 选 **loop 名字符串** → `ExecuteLoopTask`。
- `ExecuteLoopTask` 合并一批全局 `ReActLoopOption`（记忆、自反应、计划任务回调等）后 `CreateLoopByName` → `mainloop.ExecuteWithExistedTask(task)`。

### 2.2 单次迭代的执行链（从 Prompt 到 Handler）

1. **生成 Prompt**：`generateLoopPrompt` → 组装 `Background`、`UserQuery`、*Persistent*、*Reflection（ReactiveData）*、可选 `InjectedMemory`、*Schema*、*OutputExample* 等，再套 `loop_template.tpl`。
2. **调用模型**：`CallAITransaction` 流式读模型输出。
3. **解析 Action**：`aicommon.ExtractActionFromStream` 从流中抽出 JSON/`next_action` 等，得到 `*aicommon.Action`（并记录如 `last_ai_decision_response` 供排错）。
4. **校验**：当前 Action 的 `ActionVerifier`。
5. **执行**：`ActionHandler`；通过 `LoopActionHandlerOperator` 的 `Continue` / `Exit` / `Fail` / `Feedback` 控制是否进入下一轮、是否结束、是否把文本反馈写入下一轮 **ReactiveData**。

### 2.3 Prompt 各块职责（与「创建时」的选项对应）


| 区块                | 来源（典型）                                 | 作用                                 |
| ----------------- | -------------------------------------- | ---------------------------------- |
| UserQuery         | 当轮任务                                   | 用户目标                               |
| PersistentContext | `WithPersistentInstruction`            | 稳定规则、工具说明                          |
| ReactiveData      | `WithReactiveDataBuilder` + 反馈缓冲       | **上轮执行结果、FINDINGS、中间状态**（每轮重算）     |
| Schema            | `buildSchema` + `generateSchemaString` | **机器可读**的下一跳 JSON 形状与 `@action` 枚举 |
| OutputExample     | `WithReflectionOutputExample`          | 人读补充、长参数说明、示例（不替代 Schema）          |
| InjectedMemory    | 运行时若启用                                 | RAG/记忆注入                           |


**要点**：*Schema* 和 *OutputExample* 在**每一轮**都会再次出现在 Prompt 中，不是「会话说一次就结束」。

### 2.4 状态、反馈与是否「都塞进 Prompt」

- **Loop 内存**：`loop.Set` / `loop.Get` 存跨轮键（如某业务 Loop 的 `last_query_summary`、findings 文档串）。`operator.Feedback` 进缓冲区，下一轮打进 ReactiveData。
- **超大中间结果**：可在 Handler 里写入工作目录，只在 Prompt 里给**短预览 + 路径**（并 pin 到前端），避免把整表流量贴进模型。
- **业务真数据**：仍在你查询的 **DB/服务**（如 HTTP 流量在项目库的 `http_flow`）；Loop 不替代持久化，只把**摘要/结论**在对话状态与 Prompt 间传递。

### 2.5 核心组件与文件索引

- `**ReActLoop`**：主循环、迭代与状态。
- `**LoopAction` + `buildSchema`**：声明动作与合并 Schema（`action.go`）。
- `**exec.go`**：`Execute` / `ExecuteWithExistedTask`、流处理、`ExtractActionFromStream`、与 Handler 衔接。
- `**prompt.go`**：`generateLoopPrompt` / `generateSchemaString`。
- `**register.go**`：`RegisterLoopFactory`、`CreateLoopByName`、元数据表。

#### 状态转换（任务）

`Created → Processing → Completed/Aborted`（与 `aicommon` 任务状态一致）。

#### 同步与异步动作

- **同步**（默认 `AsyncMode: false`）：Handler 跑完再进下一轮。  
- **异步**：Handler 早退，需通过 `WithOnAsyncTaskTrigger` 等于路径显式收束任务状态，否则主循环不会自动完成功能。

#### 反馈

`operator.Feedback` 的文本在下一轮经 ReactiveData 回到模型，减少「模型只记得自己上一段自由文本」的漂移。

#### Stream / AI Tag

`WithRegisterLoopActionWithStreamField` 及 `LoopAITagField` 可把流式字段或标签（如长 Markdown、代码块）解到 `loop` 变量并推到 Emitter；详见代码与同目录用例。

---

## 测试说明

本模块大量测试采用 **Mock Runtime + 受控流式 JSON** 驱动。要点：

- 为 `*aicommon.Action` 提供合法 `@action` 与参数。  
- 单测可优先 `NewReActLoop`；要测 `CreateLoopByName` 时先用唯一名 `RegisterLoopFactory`，或在 `reactloopstests` 用现成夹具。

**运行**：

```bash
go test ./common/ai/aid/aireact/reactloops/ -v
go test ./common/ai/aid/aireact/reactloops/ -coverprofile=coverage.out
go tool cover -html=coverage.out
```

---

## 注意事项

1. **迭代限制**：用 `WithMaxIterations` 控制；避免 Handler 里忘记 `Exit` 导致打满。
2. **Emitter**：生产路径需有效 Emitter 才能向 UI/时间线发事件。
3. **动作名全局**：`@action` 必须对应当前 Loop 已注册的 `ActionType`。
4. **Panic**：执行路径有恢复逻辑，但业务 Handler 仍应尽量不 panic。
5. **异步子任务**：与计划执行相关时注意 `GetCurrentPlanExecutionTask` 对主循环 Action 的裁剪（见 `ExecuteLoopTask`）。

---

## 最佳实践

1. **持久 vs 动态** 分文件维护（`embed`），避免一坨字符串难 diff。
2. **Verifier** 做所有参数与前置条件；**Handler** 专注副作用与给 AI 的反馈。
3. **反馈可检索**：`Feedback` 里写清「查了什么、条数、错误原因」，少写空话。
4. **大结果落盘** + Prompt 中引用路径，与现有 HTTP Flow 等 Loop 行为一致。
5. 为新 Loop 写至少一条 **从工厂到单轮 Action** 的集成测。

---

## 常见问题


| 问题                         | 说明                                                                                    |
| -------------------------- | ------------------------------------------------------------------------------------- |
| `reactloop[xxx] not found` | 名拼错、未 `RegisterLoopFactory` 或未在 `reactinit` 中空白导入。                                    |
| 模型总选错 Action               | 检查 `Description`/`UsagePrompt` 与 `OutputExample`；检查 Schema 里是否误禁用动作。                  |
| 一轮里字段解析失败                  | 看 `last_ai_decision_response` 与 `ExtractActionFromStream` 错误；核对你的 `aitool` 与模型输出是否一致。 |
| 异步 Loop 不结束                | 在回调里把任务设到 Completed，或别用异步除非确有需要。                                                      |
| 调试 Prompt                  | 在 `generateLoopPrompt` 或调用侧打日志；注意 nonce 分块。                                           |


---

## 相关代码

- 动作提取：`common/ai/aid/aicommon/`（如 `action_extractor`、流式解析）  
- 主入口衔接：`common/ai/aid/aireact/re-act_mainloop.go`（`ExecuteLoopTask`、`selectLoopForTask`）  
- 空白导入汇聚：`common/ai/aid/aireact/reactloops/reactinit/init.go`

---

**维护者**: Yaklang Team  