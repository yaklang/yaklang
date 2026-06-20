`ssa` 库是 yaklang 的静态分析引擎入口，把多语言源码编译为统一的 SSA（静态单赋值）中间表示并构建程序数据库，供 SyntaxFlow 规则做污点/数据流查询，是代码审计能力的底座。

典型使用场景：

- 编译程序：`ssa.Parse` 编译一段代码，`ssa.ParseLocalProject` / `ssa.ParseProject` 编译整个项目（配 `ssa.withLanguage` / `ssa.withProgramName` / `ssa.withExcludeFile` / `ssa.withConcurrency` 等），`ssa.NewFromProgramName` / `ssa.NewProgramFromDB` 从数据库加载已编译程序。
- 项目与结果：`ssa.NewSSAProject` / `ssa.GetSSAProjectByID` 管理审计项目，`ssa.NewResultFromDB` 读取分析结果。
- 静态检查：`ssa.SyntaxFlowRuleChecking` 校验 SyntaxFlow 规则，`ssa.YaklangScriptChecking` 检查 yak 插件代码。

与相邻库的关系：`ssa` 负责"把代码编译成可查询的程序"，`syntaxflow` 在其之上写规则做查询，`sfreport` 输出审计报告，`risk` 记录代码风险；常配合 `git`/`filesys` 提供源码。
