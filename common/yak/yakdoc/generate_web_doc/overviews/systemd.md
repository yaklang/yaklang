`systemd` 库用于生成 systemd 的 unit/service/timer 配置文件，便于把脚本/程序注册为系统服务或定时任务，常用于持久化、运维部署与权限维持研究。

典型使用场景：

- 生成配置：`systemd.Create(name, opts...)` 生成 service/timer 配置。Service 段用 `systemd.service_exec_start` / `systemd.service_user` / `systemd.service_restart` 等；Unit 段用 `systemd.unit_description` / `systemd.unit_after` / `systemd.unit_requires` 等；Timer 段用 `systemd.timer_calendar` / `systemd.timer_boot_sec` / `systemd.timer_unit` 等定义触发。

与相邻库的关系：`systemd` 是配置生成工具，生成的 unit 常配合 `ssh`/`exec`（部署到目标）、`file`（落盘）使用。
