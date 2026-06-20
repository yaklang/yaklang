`cwe` 库提供 CWE（通用弱点枚举）数据的查询与维护能力，用于把漏洞归类到标准弱点类型，提升报告的规范性与可读性。

典型使用场景：

- 查询：`cwe.Get` 按编号取单条，`cwe.ListAll` 流式遍历全部弱点。
- 维护：`cwe.Update` 更新本地数据（可配 `cwe.url` / `cwe.proxy`），`cwe.Import` / `cwe.Export` 导入导出，`cwe.AICompleteFields` 用 AI 补全描述。

与相邻库的关系：`cwe` 与 `cve`（具体漏洞）配套，前者是"弱点类型分类"，后者是"具体漏洞实例"，常一起用于漏洞报告（`report`/`risk`）中的标准化标注。
