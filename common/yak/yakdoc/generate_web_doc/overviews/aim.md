`aim` 库是 yaklang 的 AI 引擎（AI Engine）封装，提供一个开箱即用的 ReAct 智能体运行时：给定一句自然语言输入，引擎自动规划、调用工具、与知识库/Forge 协作，并通过丰富的事件回调把过程实时吐出来。相比 `aiagent` 更偏"完整引擎 + 事件流"的一体化体验。

典型使用场景：

- 启动引擎：`aim.InvokeReAct` 同步执行一次 ReAct 任务，`aim.InvokeReActAsync` 异步返回 `*AIEngine` 句柄，`aim.NewAIEngine` 创建可复用引擎。
- 接入模型与能力：`aim.aiConfig` / `aim.aiCallback` 配置模型，`aim.attachedAITool` / `aim.attachedAIForge` / `aim.attachedKnowledgeBase` 挂载工具、Forge 与知识库，`aim.includeToolNames` / `aim.excludeToolNames` 精选工具集。
- 过程观测与交互：`aim.onStream` / `aim.onStreamContent` / `aim.onEvent` / `aim.onFinished` 订阅流式输出与事件，`aim.onInputRequired` 处理需要人工补充输入的场景，`aim.maxIteration` / `aim.timeout` 控制迭代与超时。

与相邻库的关系：`aim` 把 `ai`（模型对话）、`aiagent`（编排）、`rag`（知识检索）、`liteforge`/Forge（结构化任务）整合为统一引擎，适合需要"一行调用即可跑通一个带工具的智能体"的脚本。
