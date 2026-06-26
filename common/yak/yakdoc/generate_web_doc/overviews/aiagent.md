`aiagent` 库是 yaklang 的 AI 智能体（Agent）编排入口，把"大模型 + 工具调用 + 计划执行"封装成可在脚本里驱动的自动化流程。它围绕 ReAct 式的"思考-行动-观察"循环，让 AI 自主拆解任务、调用 yak 工具/插件、并在需要时请求人工确认。

典型使用场景：

- 构建执行器：`aiagent.NewExecutor` / `aiagent.NewExecutorFromJson` 创建协调器（Coordinator），驱动一次完整的多步任务；`aiagent.ExecuteForge` / `aiagent.CreateForge` / `aiagent.CreateLiteForge` 运行预制的 Forge 蓝图。
- 工具与计划：`aiagent.AllYakScriptAiTools` 列出可用 AI 工具，`aiagent.ParseYakScriptToAiTools` 把 yak 插件转成工具；`aiagent.ExtractPlan` / `aiagent.ExtractAction` 从模型输出里解析计划与动作。
- 行为控制：通过 `aiagent.aiCallback` 接入模型、`aiagent.tool(s)` 注入工具、`aiagent.agreeAuto` / `aiagent.agreeManual` / `aiagent.agreeYOLO` 控制审批策略、`aiagent.debug` 打开调试。

与相邻库的关系：`aiagent` 是高层编排，底层模型对话由 `ai` 库提供，结构化输出与 schema 由 `liteforge` / `jsonschema` 支撑，知识检索可结合 `rag`，被编排的能力多来自 `hook`/yak 插件。它面向"让 AI 自己完成一串安全任务"的场景。
