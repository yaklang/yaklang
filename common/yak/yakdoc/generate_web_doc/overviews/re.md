`re` 库是基于 Go RE2 引擎的正则表达式工具，提供匹配、查找、分组提取、替换，以及一批开箱即用的常见信息抽取器（IP、邮箱、URL、MAC 等）和 Grok 解析，是文本处理与数据提取的主力。

典型使用场景：

- 匹配与查找：`re.Match` 判断是否匹配，`re.Find` / `re.FindAll` 查找，`re.FindSubmatch` / `re.FindGroup` / `re.FindGroupAll` 提取分组（含命名分组）。
- 替换：`re.ReplaceAll` 文本替换，`re.ReplaceAllWithFunc` 用回调动态替换。
- 内置抽取器：`re.ExtractIP` / `re.ExtractIPv4` / `re.ExtractEmail` / `re.ExtractURL` / `re.ExtractMac` / `re.ExtractHostPort` 等直接抽取常见实体，`re.Grok` 用 Grok 规则结构化解析日志。
- 编译：`re.Compile` / `re.MustCompile` 预编译复用，`re.QuoteMeta` 转义。

与相邻库的关系：`re` 基于标准 RE2（不支持反向引用等高级特性），需要时用 `re2`（支持更丰富语法）；常与 `str`（字符串处理）、`json`（数据抽取）配合。提示：含 `\d` 等的模式建议用反引号原始串书写。
