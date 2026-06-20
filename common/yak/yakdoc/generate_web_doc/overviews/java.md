`java` 库提供 Java 序列化对象的解析、构造与序列化能力，以及字节码反编译，用于深度分析与手工构造 Java 反序列化数据流，是反序列化漏洞研究的底层工具。

典型使用场景：

- 解析序列化流：`java.ParseJavaObjectStream` / `java.ParseHexJavaObjectStream` 解析序列化字节，`java.ToJson` / `java.FromJson` 在对象与 JSON 间转换。
- 手工构造对象：`java.NewJavaObject` / `java.NewJavaClass` / `java.NewJavaClassDesc`、`java.NewJavaString`、各类 `java.NewJavaFieldXxxValue` 逐字段构造 Java 对象，`java.MarshalJavaObjects` 序列化输出。
- 反编译：`java.Decompile` 把 class/jar 反编译为源码。

与相邻库的关系：`java` 是底层手工构造层，`yso` 在其之上提供现成的 gadget 生成；二者配合 `facades`/`iiop` 完成完整反序列化利用链。
