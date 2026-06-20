---
name: incident-response
description: >
  安全事件应急响应技能。依据 NIST SP 800-61 与 SANS IR 流程，指导事件的检测分析、遏制、根除、
  恢复与复盘，涵盖 Web 入侵、主机失陷、数据泄露、勒索软件等场景的证据收集、遏制策略、根因分析
  与证据保全。通过 grep 检索日志/配置/持久化项定位入侵痕迹，将事件时间线与处置过程写入报告。
  当用户报告安全事件、要求应急响应、排查失陷主机、处理入侵告警时，应使用此技能。
---

# 应急响应技能 (Incident Response)

基于 NIST SP 800-61 事件生命周期与 SANS IR 六步法，对安全事件进行系统化处置。
通过 `grep` 检索日志、配置文件、持久化项定位入侵痕迹，用 `read_file`/`read_file_lines`
查看证据上下文，必要时用 `system`(bash/cmd) 收集主机实时状态（**需用户授权**），
最终将事件时间线、影响范围与处置过程写入应急响应报告。

> **重要原则**：应急响应中证据保全优先。在确认取证完成前，避免对失陷主机进行重启、
> 杀进程、删除文件等可能破坏证据或触发攻击者"自毁"的操作。

---

## 1. 事件响应生命周期 (NIST SP 800-61)

```
准备 → 检测与分析 → 遏制(Containment) → 根除(Eradication) → 恢复(Recovery) → 复盘(Post-Incident)
```

### 1.1 检测与分析（当前重点）

本技能聚焦"检测与分析"阶段，提供在日志、文件、主机状态中发现入侵痕迹的系统化检查点。

### 1.2 流程

1. **事件分类与定级**（第 2 节）— 判断事件类型与严重度，决定处置优先级
2. **证据收集与入侵痕迹排查**（第 3-5 节）— 按 Web 入侵 / Linux 失陷 / Windows 失陷分别检查
3. **遏制**（第 6 节）— 制定遏制策略，止损
4. **根因分析**（第 7 节）— 还原入口点 → 横向移动 → 影响范围
5. **报告**（第 8 节）

---

## 2. 事件分类与定级

| 事件类型 | 特征 | 典型初始信号 |
|---------|------|-------------|
| Web 入侵 | Web 应用被攻陷 | 访问日志出现 Webshell 访问、异常大响应、500 报错 |
| 主机失陷 | 服务器/终端被控制 | 异常进程、异常外联、可疑计划任务、新增账户 |
| 数据泄露 | 敏感数据外传 | 异常大流量外发、数据库异常查询、新增导出文件 |
| 勒索软件 | 文件被加密 | 大量文件扩展名变更、勒索说明文件、加密进程 |
| 挖矿木马 | 资源被占用 | CPU 持续 100%、异常矿池连接、隐藏进程 |

**严重度定级**：
- **严重**：核心系统失陷、敏感数据泄露、域控/堡垒机被控、勒索扩散
- **高危**：单台重要主机失陷、Webshell 已落地、横向移动迹象
- **中危**：未成功的攻击尝试、已发现但未利用的漏洞
- **低危**：扫描探测、低危配置缺陷

---

## 3. Web 入侵排查

针对 Web 服务器，优先排查访问日志与 Web 根目录：

| 检查项 | 工具 | grep 模式 / 方法 |
|--------|------|-----------------|
| 攻击请求 | `grep` 访问日志 | 参考 `web-log-analysis` 技能的攻击特征表（SQLi/XSS/路径穿越/命令注入） |
| Webshell 落地 | `find_file` + `grep` Web 根 | 搜索未知 `.php/.jsp/.asp` 文件；`grep` 危险函数 `eval\|assert\|system\|exec\|passthru\|base64_decode\|create_function` |
| 上传目录异常文件 | `find_file` upload 目录 | 按时间排序找近期新增的可执行脚本 |
| 配置文件篡改 | `read_file` | 对比关键配置（数据库连接、中间件配置）是否被改动 |
| Web 进程异常子进程 | `system` | Web 进程（www-data/nginx/tomcat）派生 bash/python/curl = 命令执行成功 |

---

## 4. Linux 失陷排查（持久化与痕迹）

| 检查项 | 路径 / 方法 | grep 模式 |
|--------|------------|----------|
| 计划任务 | `/etc/crontab`、`/var/spool/cron/*`、`/etc/cron.*/*` | 异常下载/执行条目 `wget\|curl\|bash\|python` |
| 用户级定时 | `~/.config/cron*`、用户 crontab | 同上 |
| 自启动服务 | `/etc/systemd/system/*.service`、`/lib/systemd/system/` | `ExecStart=.*curl\|ExecStart=.*wget\|/tmp/` |
| 启动脚本 | `~/.bashrc`、`~/.bash_profile`、`~/.profile`、`/etc/profile`、`/etc/rc.local` | 异常 alias / 环境变量 / 反弹 shell |
| SSH 后门 | `~/.ssh/authorized_keys` | 未知公钥、重复条目 |
| 异常账户 | `/etc/passwd`、`/etc/shadow` | UID=0 的非 root 账户、新增账户 |
| SUID 提权 | `find / -perm -4000` | 异常 SUID 二进制 |
| 隐藏文件 | `/tmp`、`/dev/shm`、`/var/tmp` | 以 `.` 开头的可执行/脚本 |
| 历史命令 | `~/.bash_history`（可能被清空） | 清空本身即为可疑信号 |
| 日志篡改 | `/var/log/*` | 检查日志是否被截断/缺失时段 |

排查要点：
- 攻击者常把恶意文件放在 `/tmp`、`/dev/shm`、`/var/tmp`（可写、常被忽略）
- `grep` 全盘可疑关键词需限定目录避免超时：`grep -r "bash -i\|/dev/tcp\|nc -e\|mknod" /etc /tmp /var/spool`

---

## 5. Windows 失陷排查（持久化与痕迹）

| 检查项 | 方法 / 位置 | grep 模式 |
|--------|------------|----------|
| 注册表自启动 | `HKLM\...\Run`、`HKCU\...\Run`、`RunOnce` | 异常启动项指向 temp/下载目录 |
| 计划任务 | `schtasks /query`、XML 配置 | 异常任务调用 powershell/cmd/下载 |
| 服务 | `services.msc`、注册表 `Services` | 异常服务、路径含空格可利用项 |
| 启动文件夹 | `shell:startup` | 异常快捷方式/脚本 |
| 异常账户 | 本地用户与组、新增管理员 | 新建账户、隐藏账户（`$` 结尾） |
| WMI 订阅 | `__EventFilter`/`CommandLineEventConsumer` | 无文件持久化 |
| 日志 | 事件日志 `Security`、`System`、`Application`、`PowerShell`/`Sysmon` | 4624(登录)/4625(失败)/4688(进程)/7045(新服务) |
| 凭据窃取 | LSASS 访问痕迹、`mimikatz` | Sysmon 事件 10（LSASS 访问） |
| 恶意脚本 | `.ps1`/`.bat`/`.vbs`/`.js` | `powershell -enc\|Invoke-\|DownloadString\|IEX` |

PowerShell 关键排查模式（日志/脚本中）：
- `powershell\s+-enc|-e\s+[A-Za-z0-9+/=]{20,}`（Base64 编码命令，高度可疑）
- `DownloadString|DownloadFile|Invoke-WebRequest|iex|Invoke-Expression`
- `Add-Type|Reflection|Assembly::Load`（内存加载）

---

## 6. 遏制策略 (Containment)

根据事件类型与定级选择遏制手段，**优先保全证据**：

| 策略 | 适用场景 | 证据风险 |
|------|---------|---------|
| 网络隔离（断网/防火墙阻断外联） | 主机失陷、C2 通信、数据外传 | 低，内存证据保留 |
| 账号封禁/改密 | 凭据泄露、异常登录 | 中，需先记录会话 |
| 进程终止 | 活跃恶意进程 | 高，内存证据丢失；先 dump 内存 |
| 服务下线 | Web 入侵、被利用服务 | 中 |
| 系统重启 | **避免**（除非迫不得已） | 高，内存/易失性证据全部丢失 |

遏制决策原则：
- 能断网不重启，能隔离不删文件
- 关键易失证据（内存、网络连接、进程）优先采集
- 遏制动作需记录时间戳，写入处置时间线

---

## 7. 根因分析

还原攻击链，回答三个核心问题：

1. **入口点（Initial Access）**：攻击者如何进入？
   - Web 漏洞（核对访问日志的攻击请求是否成功）
   - 弱口令/凭据泄露（核对登录日志、异常登录源 IP）
   - 钓鱼/恶意附件（核对邮件、下载记录）
2. **横向移动（Lateral Movement）**：攻击者如何扩散？
   - 内网扫描、凭据复用、RDP/SMB/WinRM 跳转痕迹
3. **影响范围（Impact）**：造成了什么后果？
   - 数据是否泄露（外发流量、数据库访问记录）
   - 是否植入持久化（第 4-5 节检查结果）
   - 是否完成勒索/破坏

输出攻击链时间线：`入口 → 提权 → 持久化 → 横向移动 → 目标动作（窃取/破坏）`。

---

## 8. 工具使用指南

### 8.1 grep 检索证据
- 限定目录避免全盘扫描超时（如 `/etc /tmp /var/log`）
- `pattern-mode` 设为 `regexp`，`context-buffer` 留出上下文

### 8.2 read_file / read_file_lines 查看上下文
- 对 grep 命中的配置/脚本，读取前后行确认是否为恶意内容
- 对比疑似被篡改文件与已知正常版本（`diff_file`）

### 8.3 system 收集主机实时状态（需用户授权）
- Linux：`ps aux`、`netstat -antp`、`ss -antp`、`last`、`w`
- Windows：`tasklist`、`netstat -ano`、`wmic process`、`query user`

### 8.4 write_file 写报告
将事件时间线与处置过程落盘。

---

## 9. 应急响应报告格式

报告应包含：

- **事件概述**：事件类型、严重度、发现时间、受影响系统
- **事件时间线**：按时间排序的关键事件（攻击发生 → 检测 → 遏制 → 恢复）
- **入侵分析**：入口点、攻击链、使用的漏洞/技术（映射 ATT&CK 可参考 `threat-hunting` 技能）
- **影响评估**：受影响资产、数据泄露情况、业务影响
- **证据清单**：采集的证据文件、日志片段、内存镜像
- **处置措施**：已执行的遏制/根除/恢复动作及时间
- **根因与改进建议**：漏洞修复、配置加固、监控补充、流程改进
