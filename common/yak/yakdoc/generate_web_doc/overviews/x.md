`x` 库是函数式与集合工具集（funk 风格），提供对切片/映射的 Map/Filter/Reduce、集合运算、聚合统计与常用辅助函数，让数据处理更简洁。

典型使用场景：

- 遍历变换：`x.Map` / `x.Filter` / `x.Reduce` / `x.Foreach` / `x.Find` 处理集合，`x.Chunk` / `x.Reverse` / `x.Shuffle` / `x.Zip` 重组数据。
- 集合运算：`x.Contains` / `x.IndexOf`、`x.Intersect` / `x.Subtract` / `x.Difference` / `x.IsSubset`、`x.RemoveRepeat` 去重。
- 聚合判定：`x.Sum` / `x.Max` / `x.Min`、`x.All` / `x.Any` / `x.Every` / `x.Some`、`x.Keys` / `x.Values` / `x.ToMap`。
- 辅助：`x.If`（三元）、`x.Range`、`x.Retry`（重试）、`x.Sort`、`x.WaitConnect`。

与相邻库的关系：`x` 是通用数据处理工具，无副作用，与 `str`（字符串）、`json`（结构化数据）配合，常用于把扫描/分析结果做整理与统计。
