`ldap` 库提供 LDAP 连接与登录能力，用于目录服务的连通性测试、认证验证与凭据爆破等场景。

典型使用场景：

- 连接登录：`ldap.Login(addr, opts...)` 连接并登录 LDAP 服务，返回连接对象做后续查询。
- 选项：`ldap.username` / `ldap.password` 提供凭据，`ldap.port` 指定端口。

与相邻库的关系：`ldap` 与 `brute`（凭据爆破）、`smb`/`redis` 等协议库同属服务交互工具；在 Java 利用场景中，LDAP 引用相关能力则由 `facades` 提供。
