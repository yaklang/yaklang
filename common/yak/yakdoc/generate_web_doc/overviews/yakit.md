`yakit` 库是 yak 脚本与 Yakit 客户端之间的"输出与交互"桥梁：脚本在 Yakit 中以插件形式运行时，通过该库把日志、状态、表格、图表、进度、文件等结构化信息实时回传到客户端界面进行可视化展示。脱离 Yakit 客户端（如命令行直接 `yak xxx.yak`）时，这些输出函数会安全降级为标准日志或空操作，因此可以放心在任意脚本里调用。

典型使用场景：

- 实时日志与状态：`yakit.Info` / `yakit.Warn` / `yakit.Error` / `yakit.Debug` / `yakit.Success` 输出带级别的日志；`yakit.StatusCard` 在界面顶部展示关键指标卡片（如已扫描数量、命中漏洞数）。
- 富文本与文件：`yakit.Text` / `yakit.Code` / `yakit.Markdown` 输出富文本块，`yakit.File` 配合 `yakit.FileReadAction` 等动作展示文件操作。
- 表格与图表：`yakit.EnableTable` + `yakit.TableData` 渲染固定列表格，`yakit.NewTable` / `yakit.NewLineGraph` / `yakit.NewBarGraph` / `yakit.NewPieGraph` 构建数据可视化。
- 进度反馈：`yakit.SetProgress` / `yakit.SetProgressEx` 驱动进度条，适合长耗时扫描任务。

与相邻库的关系：`yakit` 关注"把结果展示给人"，`db` 关注"把结果持久化到数据库"，`risk` / `report` 关注"漏洞与报告对象"。三者常组合使用：扫描发现结果后，用 `risk` 记录漏洞、用 `db` 入库、用 `yakit` 在界面上实时呈现。
