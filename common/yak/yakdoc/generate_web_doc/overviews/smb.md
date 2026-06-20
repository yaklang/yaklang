`smb` 库提供 SMB（Windows 文件共享）协议连接能力，用于会话建立、凭据验证与共享访问测试，常用于内网横向与认证安全评估。

典型使用场景：

- 建立连接：`smb.Connect(addr, opts...)` 连接 SMB 服务并建立会话，配 `smb.username` / `smb.password` / `smb.hash`（哈希传递）/ `smb.domain` / `smb.workstation` 提供认证信息。

与相邻库的关系：`smb` 属于协议交互工具，与 `brute`（口令/哈希爆破）、`ldap`、`servicescan`（服务识别）配合，常用于内网 SMB 的安全评估。
