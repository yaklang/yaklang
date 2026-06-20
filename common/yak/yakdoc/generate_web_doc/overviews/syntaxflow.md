`syntaxflow` 库是 yaklang 的代码审计查询引擎，用 SyntaxFlow DSL 在 SSA 程序上做污点追踪与数据流查询，配套规则管理与扫描任务调度，是自动化代码审计的核心。

典型使用场景：

- 执行规则：`syntaxflow.ExecRule(rule, prog, opts...)` 在已编译程序上执行单条规则，`syntaxflow.QuerySyntaxFlowRules` 检索规则库。
- 扫描任务：`syntaxflow.StartScan` / `syntaxflow.ResumeScan` 启动/恢复扫描，`syntaxflow.GetScanStatus` 查询进度，`syntaxflow.RunSyntaxFlowProjectScanCheck` 做项目级扫描检查；配 `syntaxflow.withScanConcurrency` / `syntaxflow.withScanResultCallback` 等控制。

与相邻库的关系：`syntaxflow` 依赖 `ssa`（提供编译后的程序），查询结果经 `sfreport` 出报告、`risk` 记录代码风险，构成完整的代码审计流水线。
