`re2` 库是基于 regexp2 引擎的正则工具，支持比标准 `re` 更丰富的语法（如反向引用、零宽断言等 .NET 风格特性），适合需要高级正则能力的复杂文本匹配场景。

典型使用场景：

- 匹配查找：`re2.Find` / `re2.FindAll`、`re2.FindSubmatch` / `re2.FindSubmatchAll`、`re2.FindGroup` / `re2.FindGroupAll` 提取分组。
- 替换：`re2.ReplaceAll` / `re2.ReplaceAllWithFunc`。
- 编译：`re2.Compile` / `re2.CompileWithOption`（指定选项位），`re2.QuoteMeta` 转义。

与相邻库的关系：`re2` 与 `re` 互补——`re` 走标准 RE2 引擎、性能稳定且无灾难性回溯；`re2` 提供高级语法但需注意回溯开销。优先用 `re`，确需高级特性时用 `re2`。
