`sfreport` 库用于把 SyntaxFlow 代码审计结果转换为报告（JSON/Markdown），并支持数据流路径、源码片段的导出与 SSA 风险的导入导出，是代码审计结果交付的专用工具。

典型使用场景：

- 结果转报告：`sfreport.ConvertSingleResultToJSON` / `sfreport.ConvertSingleResultToJSONWithOptions` 把单个 SyntaxFlow 结果转 JSON，`sfreport.GenerateSSAReportMarkdownForTask` 为审计任务生成 Markdown 报告。
- 导入与定制：`sfreport.NewReport` 创建报告对象，`sfreport.ImportSSARiskFromJSON` 导入 SSA 风险，`sfreport.withDataflowPath` / `sfreport.withFileContent` 控制是否包含数据流路径与源码内容。

与相邻库的关系：`sfreport` 是 `syntaxflow`/`ssa`（代码审计引擎）的报告输出层，与 `risk`（SSA 风险对象）、`report`（通用报告）配合，把审计发现交付为可读报告。
