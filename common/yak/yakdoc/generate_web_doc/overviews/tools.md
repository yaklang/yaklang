`tools` 库提供一组底层工具构造器，目前包含爆破工具与 PoC 调用器的创建入口，作为更高层安全功能的基础组件。

典型使用场景：

- 构造工具：`tools.NewBruteUtil(type)` 创建底层爆破工具实例，`tools.NewPocInvoker()` 创建 PoC 调用器用于驱动 PoC 执行。

与相邻库的关系：`tools` 偏底层构造，`brute`（爆破框架）、`nuclei`/`httptpl`（PoC 执行）等在更高层提供更易用的封装；一般优先用上层库。
