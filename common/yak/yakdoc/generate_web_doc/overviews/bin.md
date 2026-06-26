`bin` 库是二进制结构解析工具，用声明式的"描述符（PartDescriptor）"定义一段二进制数据的字段布局，然后把原始字节按布局解析成结构化结果。它适合解析自定义协议报文、文件格式头、TLV 结构等场景。

典型使用场景：

- 定义字段：`bin.toInt8/16/32/64`、`bin.toUint8/16/32/64` 解析定长整数，`bin.toBytes` / `bin.toRaw` 解析定长字节块，`bin.toBool` 解析布尔位。
- 组合结构：`bin.toStruct` 把若干字段组成结构体，`bin.toList` 解析重复字段列表，支持嵌套。
- 执行解析与取值：`bin.Read(data, descriptors...)` 按描述符解析输入数据得到结果切片，`bin.Find(results, name)` 按字段名取出某个解析结果。

与相邻库的关系：`bin` 偏底层二进制解析，常与 `codec`（编解码）、`pcapx`（抓包/造包）、`fuzz`（协议变异）配合，用于自定义协议的解析与构造。
