`hook` 库是 Yakit 插件调用框架，把 yak 插件按其声明的 Hook 点（如 `mirrorNewWebsitePath`、`handle` 等）装载并驱动，常用于在脚本中复用现有插件、构建 MITM/扫描的插件调用链。

典型使用场景：

- 管理器：`hook.NewManager` 创建调用管理器，`hook.LoadYakitPlugin` / `hook.LoadYakitPluginByName` / `hook.LoadYakitPluginByID` 按类型/名称/ID 装载插件，`hook.RemoveYakitPluginByName` 卸载。
- 混合调用器：`hook.NewMixPluginCaller` / `hook.NewMixPluginCallerWithFilter` 创建可同时驱动多类插件（端口/MITM/Web）的调用器。
- 直接调用：`hook.CallYakitPluginFunc` 调用插件中导出的某个函数。

与相邻库的关系：`hook` 通常与 `db.CreateTemporaryYakScript`（创建临时插件）、`mitm`（流量劫持）、`fuzz`/`poc`（请求处理）协同，把"一段检测逻辑"以插件形式插入到流量/扫描链路中。
