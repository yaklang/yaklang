`regen` 库是"反向正则"——根据一个正则表达式生成能匹配它的字符串，常用于批量生成测试数据、字典与符合特定格式的 Payload。

典型使用场景：

- 全量生成：`regen.Generate(pattern)` / `regen.MustGenerate` 生成所有匹配字符串（注意模式不要过于宽泛导致组合爆炸）。
- 单条生成：`regen.GenerateOne` / `regen.MustGenerateOne` 生成一条，`regen.GenerateVisibleOne` 生成可见字符串。
- 流式生成：`regen.GenerateStream` / `regen.GenerateOneStream` 在上下文控制下流式产出，适合大规模生成。

与相邻库的关系：`regen` 与 `re`（正则匹配）互为逆操作，产出的数据常喂给 `fuzz`（变异测试）、`brute`/`dictutil`（字典）使用。提示：正则模式建议用反引号原始串书写。
