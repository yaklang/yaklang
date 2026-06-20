`cli` 库用于把 yak 脚本变成带参数的命令行工具/插件：声明参数类型、默认值、校验与帮助信息，运行时从命令行或 Yakit 插件界面读取输入。它同时驱动 Yakit 插件的参数表单（含 UI Schema 布局）。

典型使用场景：

- 声明参数：`cli.String` / `cli.Int` / `cli.Bool` / `cli.Float` / `cli.Text` 取基础类型，`cli.Url(s)` / `cli.Host(s)` / `cli.Port(s)` / `cli.Net` 取网络相关参数，`cli.File` / `cli.FileNames` / `cli.YakitPlugin` 取文件与插件，`cli.Json` 取结构化对象。
- 参数选项：`cli.setRequired` / `cli.setDefault` / `cli.setHelp` / `cli.setVerboseName` / `cli.setCliGroup` / `cli.setShortName` 等修饰每个参数；`cli.check()` 在声明完成后做必填校验。
- 界面与表单：`cli.setJsonSchema` + `cli.setUISchema` / `cli.uiGroups` / `cli.uiField` 定义 Yakit 插件参数表单的分组与布局，`cli.UI` / `cli.when*` 控制联动显隐。

与相邻库的关系：`cli` 是脚本的"输入层"，与 `yakit`（输出/界面）一上一下，二者共同把一个 yak 脚本封装成完整的 Yakit 插件。
