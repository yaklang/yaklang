`yso`（Yak Serialization Objects）库是 Java 反序列化利用 Payload 的生成中枢，对标 ysoserial：内置大量 gadget 链与恶意类模板，可生成命令执行、回显、反连、DNSLog 等各类 Java 反序列化 Payload 与恶意 class。

典型使用场景：

- gadget 链：`yso.GetCommonsCollections5JavaObject(cmd)` 等 `Get*JavaObject` 家族生成具体 gadget 链对象，`yso.GetGadget(name, opts...)` 按名取链，`yso.GetAllGadget` / `yso.GetAllRuntimeExecGadget` 列出可用链。
- 恶意类：`yso.GenerateRuntimeExecEvilClassObject(cmd)` / `yso.GenerateProcessBuilderExecEvilClassObject` 生成命令执行类，`yso.GenerateTomcatEchoClassObject` / `yso.GenerateSpringEchoEvilClassObject` 生成回显类，`yso.GenerateDNSlogEvilClassObject` 生成 DNSLog 类，`yso.GenerateTcpReverseShellEvilClassObject` 生成反连类。
- 选项与序列化：`yso.command` / `yso.dnslogDomain` / `yso.majorVersion` / `yso.useTemplate` / `yso.useRuntimeExecTemplate` 等选项定制，`yso.ToBytes` 序列化为字节、`yso.ToBcel` / `yso.ToJson` 转换其他形态，`yso.LoadClassFromBytes` 加载自定义类。

与相邻库的关系：`yso` 生成的 Payload 经 `poc`/`fuzz`（HTTP 投递）、`t3`/`iiop`（协议通道）、`facades`（JNDI 服务端）发往目标，配合 `dnslog`/`risk` 做无回显验证；底层对象构造能力来自 `java`。
