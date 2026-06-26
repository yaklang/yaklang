`netstack` 库提供用户态网络栈与系统路由管理能力，可创建特权网络设备（TUN）、构建用户态网络虚拟机、管理系统路由，常用于流量重定向、代理底座与高级网络操作。这些操作通常需要管理员/root 权限。

典型使用场景：

- 网络设备与虚拟机：`netstack.CreatePrivilegedDevice` / `netstack.CreatePrivilegedDeviceWithMTU` 创建 TUN 设备，`netstack.NewVMFromDevice` 在其上构建用户态网络栈虚拟机。
- 路由管理：`netstack.GetSystemRouteManager` / `netstack.GetPrivilegedSystemRouteManager` 管理系统路由，`netstack.FastKillTCP` 快速断开 TCP 连接。

与相邻库的关系：`netstack` 偏底层网络栈，与 `netutils`（路由表增删）、`pcapx`（抓包/造包）配合，用于构建代理、流量牵引等高级网络场景。
