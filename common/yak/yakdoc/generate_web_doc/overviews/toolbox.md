`toolbox` 库用于管理外部第三方安全工具/二进制的安装、卸载与查询，方便在脚本运行环境中按需准备所依赖的外部工具。

典型使用场景：

- 工具管理：`toolbox.Install(name, opts...)` 安装工具（配 `toolbox.proxy` / `toolbox.force` / `toolbox.progress` 控制下载），`toolbox.Uninstall` 卸载，`toolbox.List` 列出已安装工具及状态。

与相邻库的关系：`toolbox` 是环境准备工具，常配合 `exec`（调用安装好的外部工具）使用。
