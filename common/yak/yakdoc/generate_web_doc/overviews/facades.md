`facades` 库提供一个多协议"恶意服务端"（Facade Server），同时监听 HTTP/RMI/LDAP 等协议，用于 Java 反序列化、JNDI 注入等漏洞的利用与带外验证：把恶意类/资源托管在该服务上，诱导目标回连加载。

典型使用场景：

- 启动服务：`facades.Serve(host, port, configs...)` 直接启动，或 `facades.NewFacadeServer` 创建实例后控制。
- 托管资源：`facades.httpResource` / `facades.evilClassResource` 托管 HTTP/恶意类资源，`facades.rmiResourceAddr` / `facades.ldapResourceAddr` 配置 RMI/LDAP 引用，`facades.javaClassName` / `facades.javaCodeBase` / `facades.javaFactory` / `facades.objectClass` 配置 JNDI 利用参数。

与相邻库的关系：`facades` 是 JNDI/反序列化利用的服务端，常与 `yso`（生成 gadget/恶意类）、`dnslog`（带外检测）、`poc`（发起触发请求）协同完成完整利用链。
