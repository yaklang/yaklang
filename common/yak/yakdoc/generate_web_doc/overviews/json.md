`json` 库提供 JSON 的序列化/反序列化与路径查询能力，并能从混杂文本中智能提取 JSON 片段，是数据处理与接口测试的常用工具。

典型使用场景：

- 序列化：`json.dumps`（对象转字符串，可配 `json.withIndent` / `json.noEscapeHTML`）、`json.Marshal`，`json.loads` 解析字符串。
- 路径查询：`json.Find` / `json.FindPath`（JSONPath 取值）、`json.ReplaceAll`（按路径替换）。
- 提取：`json.ExtractJSON` / `json.ExtractJSONEx` 从任意文本里捞出合法 JSON 片段。

与相邻库的关系：`json` 是纯数据处理库，常与 `http`/`poc`（解析接口响应）、`jsonstream`（流式大 JSON）、`jsonschema`（结构定义）配合。
