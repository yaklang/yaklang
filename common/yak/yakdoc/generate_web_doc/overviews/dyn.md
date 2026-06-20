`dyn` 库提供动态执行与模块导入能力，允许在运行时求值 yak 代码、从 `.yak` 文件导入变量/函数，实现脚本的动态加载与组合。

典型使用场景：

- 动态求值：`dyn.Eval` 在当前运行时执行一段 yak 代码。
- 模块导入：`dyn.Import` 从文件导入导出项，`dyn.LoadVarFromFile` 加载文件中的变量（可配 `dyn.params` 传参、`dyn.recursive` 递归）。
- 类型判断：`dyn.IsYakFunc` 判断某值是否为 yak 函数。

与相邻库的关系：`dyn` 提供脚本级的动态能力，常用于插件框架、把多个 `.yak` 文件组合调用的场景，与 `hook`（插件调用）思路互补。
