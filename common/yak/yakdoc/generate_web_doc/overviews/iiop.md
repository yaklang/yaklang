`iiop` 库用于构造与发送 IIOP/CORBA 协议的利用 Payload，常用于 Java 反序列化、JNDI 相关漏洞经 IIOP 通道的利用。

典型使用场景：

- 构造 Payload：`iiop.BindPayload` / `iiop.RebindPayload` 构造绑定/重绑定引用，`iiop.InvokePayload` 构造命令调用 Payload。
- 发送：`iiop.SendPayload(addr, payload)` 把生成的 Payload 发往目标。

与相邻库的关系：`iiop` 与 `facades`（恶意服务端）、`yso`（gadget 生成）、`java`（Java 对象构造）配合，构成 Java JNDI/反序列化利用链中的 IIOP 通道。
