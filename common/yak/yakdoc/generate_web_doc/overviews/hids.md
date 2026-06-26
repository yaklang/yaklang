`hids` 库是主机入侵检测（Host-based IDS）工具集，提供进程、网络连接、资源、审计日志与 SSH 登录的采集与实时监控能力，用于主机安全监控、应急响应与威胁狩猎。

典型使用场景：

- 资源采集：`hids.CPUPercent` / `hids.MemoryPercent` 取资源占用，`hids.PS` 列进程，`hids.Netstat` / `hids.GetEstablishedConnections` / `hids.GetListeningPorts` 看连接。
- 进程关系：`hids.GetProcessTree` / `hids.GetProcessAncestors` / `hids.GetProcessChildren` 分析进程树，`hids.KillProcess` 终止进程。
- 实时监控：`hids.NewProcessMonitor` / `hids.NewConnectionMonitor`（配 `hids.WithOnProcessCreate` / `hids.WithOnNewConnection` 等回调）监控进程与连接事件；`hids.NewAuditMonitor` / `hids.WatchAuditEvents` 监控登录与命令；`hids.NewJournalSSHMonitor` 监控 SSH 登录。

与相邻库的关系：`hids` 偏主机侧防御与监控，与 `filemonitor`（文件监控）、`exec`/`os`（系统交互）配合，构成主机侧的"看得见"能力。
