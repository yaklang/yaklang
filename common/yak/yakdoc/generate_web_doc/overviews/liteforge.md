`liteforge` 库是轻量级的"一次性结构化 AI 任务"封装：给一段提示与输出 Schema，调用大模型并拿回结构化结果（含多模态图像/视频分析），无需搭建完整 Agent。它也内置了视频 Omni 分析与知识库构建能力。

典型使用场景：

- 结构化执行：`liteforge.Execute(query, opts...)` 跑一次任务，`liteforge.output` / `liteforge.action` 指定输出动作，`liteforge.image` / `liteforge.imageFile` 传入图像做多模态分析。
- 视频分析：`liteforge.AnalyzeVideoOmni` 对视频做全维分析，`liteforge.BuildVideoKnowledgeFromOmni` 把视频内容沉淀为知识库条目；`liteforge.omniModel` / `liteforge.omniSegmentSeconds` / `liteforge.omniPresetFlash` 等控制分段与模型。

与相邻库的关系：`liteforge` 处于 `ai`（底层对话）与 `aiagent`/`aim`（完整编排）之间，适合"只需一次结构化调用"的场景；输出 Schema 由 `jsonschema` 描述，知识库结果可入 `rag`。
