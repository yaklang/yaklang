---
name: network-config-audit
description: >
  防火墙与交换机配置安全核查与优化技能。依据安全基线（CIS / 等保）核查 iptables/firewalld、
  Cisco ASA、Cisco IOS、H3C、Huawei 等设备的运行配置，检查 ACL 过宽、弱口令、明文协议、SNMP
  弱配置、未关闭端口、日志审计缺失等风险项，并给出加固命令与优化建议。通过 read_file 读取
  配置、grep 匹配风险项，将核查结果写入报告。当用户提供防火墙/交换机配置、要求配置核查、
  安全基线检查、网络加固优化时，应使用此技能。
---

# 网络设备配置核查与优化技能 (Network Config Audit)

对防火墙与交换机的运行配置进行安全基线核查，依据 CIS Benchmark 与等保要求识别风险项，
并给出可执行的加固命令。通过 `read_file` 读取配置文件，用 `grep` 匹配各类风险模式，
将核查结果与加固建议写入报告。

---

## 1. 核查流程

### 1.1 识别设备与配置类型

先用 `read_file`/`head_file` 查看配置头部，判断设备类型：

| 设备/类型 | 配置特征 | 典型头部 |
|-----------|---------|---------|
| Cisco IOS（路由器/交换机） | `!` 注释、`version`、`hostname` | `version 15.x`、`service timestamps` |
| Cisco ASA（防火墙） | `ASA Version`、`enable password`、`nat` | `ASA Version 9.x` |
| H3C / Comware | `sysname`、`interface`、`#` 分段 | `sysname XXX` |
| Huawei VRP | `sysname`、`undo`、`#` 分段 | `!Software Version Vxxx` |
| Juniper Junos | `set system`、层次化 set 语句 | `## last committed` |
| iptables（Linux） | `-A INPUT`、`-p tcp`、规则链 | `*filter`、`-A` |
| firewalld（Linux） | zone/service/rich rule | `zone`、`service name=` |

判断要点：不同设备语法差异大，先正确识别再套用对应检查项，避免误报。

### 1.2 分项核查

按风险类别分阶段 grep（第 3 节），每类聚焦一组检查项。

### 1.3 风险分级与加固建议

对每个命中的风险项，给出风险等级与对应设备的加固命令。

### 1.4 输出核查报告

---

## 2. 核查维度总览

| 维度 | 检查要点 |
|------|---------|
| 认证与口令 | 明文 enable 密码、弱口令、本地账户口令强度、登录认证方式 |
| 管理协议 | Telnet 明文、SSH v1、HTTP 明文管理、未限制管理源 |
| SNMP | v1/v2c 弱 community、可写 community、默认 public/private |
| ACL 与访问控制 | 过宽规则（any any）、缺省拒绝缺失、未使用规则 |
| 端口与服务 | 未关闭的空闲端口、不必要服务（finger/chargen/CDP 等） |
| 日志与审计 | 未开启 logging、未配 NTP、日志级别不当 |
| 冗余与高可用 | 配置保存（startup vs running）、未加密配置文件 |
| 路由协议 | 明文认证的路由协议、未授权路由更新 |

---

## 3. 风险检查项与 grep 模式

### 3.1 Cisco IOS / ASA

| 检查项 | grep 模式 | 风险 | 加固建议 |
|--------|----------|------|---------|
| 明文 enable 密码 | `enable password` （非 `enable secret`） | 高危：明文存储特权密码 | 改用 `enable secret <level> <password>`，删 `no enable password` |
| 弱 enable secret | `enable secret 5 \$1\$` | 中危：MD5 弱哈希 | 升级到 type 8/9（scrypt） |
| 本地账户弱口令 | `username\s+\S+\s+(password|password 0)` | 高危：明文/弱哈希 | `username X privilege Y secret <pwd>` |
| Telnet 管理 | `transport input telnet`、`line vty 0 4` 配 telnet | 高危：明文传输 | 改 `transport input ssh`，`no transport input telnet` |
| SSH v1 | `ip ssh version 1` 或未指定版本 | 中危：协议不安全 | `ip ssh version 2` |
| HTTP 明文管理 | `ip http server` | 中危：明文 | `no ip http server`，用 `ip http secure-server` |
| 管理源未限制 | `access-class` 缺失于 vty | 中危：任意源可登录 | vty 下 `access-class <acl> in` |
| SNMP v1/v2c | `snmp-server community` 无 `v3` | 高危：明文 community | 改 `snmp-server group/user ... v3` |
| SNMP 默认 community | `community (public\|private)` | 严重 | 更换复杂 community 或升级 v3 |
| ACL 过宽 | `permit (ip\|tcp\|udp) any any` | 高危：放行所有 | 收紧到最小必要，缺省 `deny ip any any` |
| 未用端口未关 | 接口缺 `shutdown` | 中危 | 空闲接口 `shutdown` |
| CDP 泄露 | `cdp run`（面向不可信网络） | 低危：信息泄露 | 边界接口 `no cdp enable` |
| 未开启日志 | 无 `logging` | 中危 | `logging host <ip>`、`logging trap informational` |
| 未配 NTP | 无 `ntp` | 低危：日志时间不准 | `ntp server <ip>` + `ntp authenticate` |
| 不必要服务 | `service finger\|service tcp-small-servers\|ip finger\|no service tcp-keepalives-in` | 低-中危 | 关闭 `no service finger` 等 |
| 配置明文未加密 | 未 `service password-encryption` | 中危 | `service password-encryption` |

### 3.2 H3C / Huawei VRP

| 检查项 | grep 模式 | 风险 | 加固建议 |
|--------|----------|------|---------|
| 弱口令/明文 | `password simple` | 高危：明文 | 改 `password cipher <pwd>` |
| Telnet 管理 | `protocol inbound telnet`、`user-interface vty` 配 telnet | 高危 | `protocol inbound ssh` |
| SSH v1 | `ssh server compatible-ssh1x enable` | 中危 | `undo ssh server compatible-ssh1x enable` |
| HTTP 明文管理 | `undo ip http enable` 缺失（即开了 http） | 中危 | `undo ip http enable`，用 `ip https enable` |
| SNMP v1/v2c | `snmp-agent community (read\|write)` | 高危 | 改 `snmp-agent group/user v3` |
| 默认 community | `community (public\|private)` | 严重 | 更换或升级 v3 |
| ACL 过宽 | `rule permit (ip\|tcp) any` 末尾非 deny | 高危 | 末尾 `rule deny` |
| 未关闭端口 | 接口缺 `shutdown`/`undo shutdown` 异常 | 中危 | 空闲接口 `shutdown` |
| 未开日志 | 无 `info-center loghost` | 中危 | `info-center loghost <ip>` |
| 未配 NTP | 无 `ntp-service` | 低危 | `ntp-service unicast-server <ip>` |
| 明文密码加密 | H3C `password-control` 未启用 | 中危 | 启用密码复杂度策略 |

### 3.3 Juniper Junos

| 检查项 | grep 模式 | 风险 | 加固建议 |
|--------|----------|------|---------|
| 明文/弱口令 | `set system root-authentication` 含 plain-text-password | 高危 | 用 encrypted-password / sha1 |
| Telnet/FTP 服务 | `set system services telnet`、`ftp` | 高危 | `delete system services telnet`，用 ssh |
| HTTP 明文管理 | `set system services web-management http` | 中危 | 仅 `https` |
| SNMP v1/v2c | `set snmp community` | 高危 | 改 `set snmp v3 ...` |
| 管理源未限 | `set system services ssh` 无 `rate-limit`/防火墙过滤 | 中危 | 加 firewall filter 限制源 |

### 3.4 Linux iptables / firewalld

| 检查项 | grep 模式 | 风险 | 加固建议 |
|--------|----------|------|---------|
| 缺省策略 ACCEPT | `-P INPUT ACCEPT`（无 `-j DROP` 兜底） | 高危 | `-P INPUT DROP`，显式放行必要端口 |
| 放行所有 | `-A INPUT -j ACCEPT`（过宽） | 严重 | 收紧规则 |
| 放行任意源到管理端口 | `-A INPUT -p tcp --dport 22 -j ACCEPT`（无源限制） | 高危 | 加 `-s <管理网段>` |
| ICMP 全开 | `-A INPUT -p icmp -j ACCEPT` | 低危 | 限制类型/速率 |
| firewalld zone 过宽 | `default-zone` 为 public 且接口在 public | 中危 | 管理接口放 trusted/internal |
| firewalld 全开服务 | `service name=".*"` 过多 | 中危 | 仅开必要服务 |

---

## 4. 风险分级标准

- **严重**：默认/弱 SNMP community 可写、ACL 全放行 `any any`、特权明文口令、公网开放 Telnet
- **高危**：明文管理协议（Telnet/HTTP）、SSH v1、弱口令哈希、管理源未限制
- **中危**：SNMP v1/v2c、未开启日志、未配 NTP、明文配置未加密、CDP 泄露
- **低危**：未关闭空闲端口、ICMP 全开、不必要服务（finger/chargen 等）

---

## 5. 加固建议输出原则

每条加固建议应包含：
- **风险描述**：为何该配置有风险
- **当前配置**：命中的配置行（grep 结果）
- **加固命令**：该设备的**具体修正命令**（不同设备语法不同，必须对应）
- **预期效果**：加固后的安全状态

加固命令示例（务必匹配设备类型）：
- Cisco IOS 关 Telnet：`line vty 0 4` → `transport input ssh` → `no transport input telnet`
- H3C 改密文：`password cipher <pwd>`
- iptables 设缺省拒绝：`iptables -P INPUT DROP`

---

## 6. 工具使用指南

### 6.1 read_file 读取配置
- 整份读取设备 running-config / startup-config
- 支持多种来源：直接文件、`show running-config` 输出保存的文本

### 6.2 grep 匹配风险项
- 按设备类型分阶段 grep（先识别设备，再套第 3 节对应模式）
- `pattern-mode` 设为 `regexp`
- 注意区分"配置存在"与"配置缺失"：某些风险是"缺少某项"（如缺 `logging`），需结合上下文判断

### 6.3 find_file / tree 定位配置
- 在主机上寻找配置文件（`/etc/sysconfig/iptables`、`/etc/firewalld/`、备份配置目录）

### 6.4 write_file 写核查报告

---

## 7. 核查报告格式

报告应包含：

- **核查范围**：设备清单、设备类型、配置来源、核查时间
- **基线依据**：引用的 CIS / 等保版本
- **核查结果汇总**：各风险等级的项数统计、合规率
- **逐项发现**：每项含
  - 检查项名称与维度
  - 风险等级
  - 命中的配置行（证据）
  - 风险说明
  - 加固命令（对应设备）
  - 预期效果
- **合规性结论**：是否符合基线、未达标项清单
- **加固优先级建议**：按风险等级排序的处置顺序
