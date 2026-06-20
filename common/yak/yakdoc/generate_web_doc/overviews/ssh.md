`ssh` 库提供 SSH 客户端能力，支持口令与密钥认证，连接后可执行命令、做运维与安全测试，常用于远程主机交互与认证评估。

典型使用场景：

- 建立连接：`ssh.Connect(host, opts...)` 通用连接（配 `ssh.username` / `ssh.password` / `ssh.privateKey` / `ssh.keyPassphrase` / `ssh.port` / `ssh.timeout`），`ssh.ConnectWithPasswd` / `ssh.ConnectWithKey` 为口令/密钥认证的便捷入口，返回客户端后执行命令。

与相邻库的关系：`ssh` 属于协议交互工具，与 `brute`（SSH 口令爆破）、`servicescan`（服务识别）配合用于远程主机的安全评估与自动化运维。
