`rdp` 库提供 RDP（远程桌面协议）相关能力，用于检测 RDP 服务版本与凭据验证，常用于资产识别与认证安全评估。

典型使用场景：

- 版本探测：`rdp.Version(addr, timeout)` 探测目标 RDP 服务的版本信息。
- 登录验证：`rdp.Login(ip, domain, user, password, port)` 验证一组凭据是否能登录 RDP。

与相邻库的关系：`rdp` 属于协议交互工具，常与 `brute`（凭据爆破）、`servicescan`（服务识别）配合用于远程桌面的安全评估。
