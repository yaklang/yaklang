`xml` 库提供 XML 的序列化、反序列化、转义与美化能力，用于处理 XML 格式的配置、接口数据与协议报文。

典型使用场景：

- 解析与生成：`xml.loads(v)` 把 XML 解析为 map，`xml.dumps(v, opts...)` 把数据序列化为 XML（配 `xml.escape` 控制转义）。
- 处理：`xml.Escape` 转义特殊字符，`xml.Prettify` 美化排版 XML。

与相邻库的关系：`xml` 是数据处理库，与 `json` / `yaml`（其他结构化格式）并列，常用于 XML 接口测试（如 XXE 场景）与配置解析。
