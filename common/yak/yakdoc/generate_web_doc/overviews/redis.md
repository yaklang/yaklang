`redis` 库提供 Redis 客户端能力，用于连接 Redis 服务并执行命令，常用于未授权访问检测、数据读取与服务交互测试。

典型使用场景：

- 创建客户端：`redis.New(opts...)` 创建客户端，配 `redis.host` / `redis.port` / `redis.addr` 指定目标，`redis.username` / `redis.password` 提供凭据，`redis.timeoutSeconds` / `redis.retry` 控制连接行为，之后执行 Redis 命令。

与相邻库的关系：`redis` 属于协议交互工具，与 `brute`（口令爆破）、`servicescan`（服务识别）配合，常用于 Redis 未授权/弱口令的检测。
