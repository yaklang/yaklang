`netutils` 库提供系统路由表的增删操作，把指定 IP/网段的路由绑定到某个网络接口或删除，常用于流量牵引、分流与代理环境配置。这些操作通常需要管理员/root 权限。

典型使用场景：

- 添加路由：`netutils.AddIPRouteToNetInterface` / `netutils.AddSpecificIPRouteToNetInterface` 把 IP 路由到指定接口，`netutils.BatchAddSpecificIPRouteToNetInterface` 批量添加。
- 删除路由：`netutils.DeleteIPRoute` / `netutils.DeleteSpecificIPRoute` / `netutils.BatchDeleteSpecificIPRoute` 删除路由，`netutils.DeleteAllRoutesForInterface` 清空某接口的路由。

与相邻库的关系：`netutils` 与 `netstack`（用户态网络栈）配合，用于把目标流量牵引到代理/虚拟网络栈处理。
