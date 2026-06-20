`brute` 库是弱口令爆破框架，内置对多种协议/服务（如 SSH、FTP、数据库、Web 等）的爆破插件，支持自定义用户名/密码字典、并发控制与自定义校验回调，是认证安全评估的核心工具。

典型使用场景：

- 选择爆破类型：`brute.GetAvailableBruteTypes` 列出支持的服务类型，`brute.New(type, opts...)` 创建爆破器并对目标执行。
- 字典管理：`brute.userList` / `brute.passList` 指定字典，`brute.autoDict` 启用内置字典，`brute.GetUsernameListFromBruteType` / `brute.GetPasswordListFromBruteType` 取出某类型的内置字典。
- 速率与策略：`brute.concurrent` / `brute.concurrentTarget` 控制并发，`brute.minDelay` / `brute.maxDelay` 控制节流，`brute.okToStop` / `brute.finishingThreshold` 控制命中后停止，`brute.bruteHandler` 自定义校验逻辑。

与相邻库的关系：`brute` 常接在资产发现之后——`synscan`/`servicescan` 找到开放服务，`brute` 对其做口令爆破，命中结果可经 `risk` 记录、`report` 汇总。
