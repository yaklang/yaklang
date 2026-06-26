`yaml` 库提供 YAML 的序列化与反序列化能力，用于读写 YAML 格式的配置、PoC 模板与数据。

典型使用场景：

- 解析：`yaml.Unmarshal(b)` 把 YAML 解析为对象，`yaml.UnmarshalStrict` 严格模式解析（未知字段报错）。
- 生成：`yaml.Marshal(in)` 把对象序列化为 YAML。

与相邻库的关系：`yaml` 是数据处理库，与 `json` / `xml` 并列，常用于解析 Nuclei/httptpl 的 YAML PoC 模板与各类配置文件。
