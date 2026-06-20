`excel` 库基于 excelize 提供 Excel 文件的读写能力，用于把扫描/分析结果导出为表格，或解析已有表格作为输入数据源。

典型使用场景：

- 读取解析：`excel.Parse` / `excel.ParseTableOnly` / `excel.ParseTableFast` 解析表格内容，`excel.ClassifyNodes` 对节点分类。
- 创建写入：`excel.NewFile` 新建工作簿，`excel.NewSheet` / `excel.DeleteSheet` 管理工作表，`excel.WriteCell` / `excel.SetFormula` / `excel.InsertImage` 写入内容，`excel.Save` 保存。
- 样式：`excel.CreateStyle` / `excel.SetCellStyle` / `excel.SetSheetVisible` 控制样式与可见性。

与相邻库的关系：`excel` 是数据导入导出工具，常作为报告/结果落地的一种格式，与 `report`（HTML 报告）、`file`（文件读写）互补。
