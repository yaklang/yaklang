---
name: blue-team-task-design
description: >
  蓝队安全分析任务设计与执行总指导。作为蓝队的顶层编排技能，将防御分析需求映射到可直接调用的
  工具链，定义从输入接收到报告输出的完整决策流程。串联 web-log-analysis、incident-response、
  threat-hunting、detection-rule-engineering、network-config-audit 等全部蓝队子技能。
  当用户要求进行蓝队分析、防御分析、安全排查、日志研判、应急响应协调、威胁狩猎或网络加固时，
  应首先参考此技能进行任务设计。
---

# 蓝队任务设计与执行指导

本技能是蓝队的顶层编排指导。它将防御分析方法论与系统中可用的具体工具一一对应，
确保每一步任务设计都可直接执行。当收到蓝队分析任务时，按照本指导分阶段设计和执行。

与红队（`pentest-task-design`）的对照：红队是"目标驱动"（给目标去攻击）；
蓝队是"**输入驱动**"（用户给出日志/流量/配置/告警/样本，需要研判、排查、加固）。
因此蓝队的核心流程是**识别输入类型 → 路由到对应子技能 → 执行分析 → 关联扩展 → 输出报告**。

---

## 0. 核心原则

1. **输入先行**：先判断用户提供了什么（日志？流量？配置？告警？样本？），再决定走哪个子技能，不要盲目套流程。
2. **证据驱动**：结论必须有原始数据支撑。grep 真实命中、真实日志片段、真实配置行才算证据；"可能存在风险"只是假设，需标注为待验证。
3. **工具驱动**：每一步都映射到具体可调用工具（`grep`/`read_file`/`analyze_pcap`/`write_file` 等）。没有对应工具的转为手动检查项并标注。
4. **关联扩展**：单点发现要横向扩展（同 IOC、同主机、同时间窗、同 TTP），避免孤立结论。
5. **留档贯穿全程**：发现即写入文件或用 `cybersecurity-risk` 落库，不要攒到末尾。
6. **分级与可执行**：输出按严重程度分级，加固/处置建议必须可执行（具体命令/规则/步骤）。
7. **授权边界**：涉及主机实时操作（`system`/`bash`/`cmd`）、请求回放（`do_http_request`）须在已授权范围内执行，未授权时降级为"建议手动执行"并标注。

### 0.1 模式选择

收到蓝队请求时，先判断任务模式：

- **纯分析模式**：用户要求研判/排查/审计/核查 → 直接进入数据分析，产出发现与建议。
- **处置模式**：用户要求遏制/加固/生成规则 → 在分析基础上输出可执行动作（命令、规则、步骤）。
- **规划模式**：用户只要方案/检查清单/响应预案 → 输出结构化计划，不冒充已验证发现。

---

## 1. 工具能力矩阵（蓝队）

| 工具 | 功能 | 蓝队用途 |
|------|------|---------|
| `grep` | 文本搜索（子串/正则） | 在日志/配置/样本中匹配攻击特征、IOC、风险项 |
| `read_file` / `read_file_lines` | 读取文件 | 查看日志、配置、样本、证据上下文 |
| `tail_file` / `head_file` | 读尾/读头 | 实时事件排查、日志格式识别 |
| `write_file` | 写文件 | 保存证据、分析结果、规则、报告 |
| `find_file` / `tree` | 文件查找/目录树 | 定位日志、配置、Webshell、证据文件 |
| `analyze_pcap` | PCAP 流量分析 | 分析抓包、异常外联、C2、隧道 |
| `do_http_request` | 发送 HTTP 请求 | 回放验证可疑请求（需授权）、查询情报 API |
| `web_search` | 互联网搜索 | IOC 情报富化、漏洞/漏洞情报查询 |
| `system` (bash/cmd/powershell) | 执行系统命令 | 收集主机实时状态（需授权） |
| `decode` / `auto_decode` | 解码 | 解码日志/样本中的编码 payload |
| `cybersecurity-risk` | 风险落库 | 把确认的风险/失陷/漏洞以标准格式留档 |

---

## 2. 子技能矩阵

| 子技能 | 适用场景 | 核心产出 |
|--------|---------|---------|
| `web-log-analysis` | 分析访问日志 / HTTP 流量，找 Web 攻击 | 攻击时间线、分级研判报告 |
| `incident-response` | 排查失陷主机 / 处理入侵事件 | IR 流程、证据清单、根因、处置建议 |
| `threat-hunting` | 主动搜寻潜在威胁、IOC 关联、ATT&CK 分析 | 狩猎假设验证、覆盖矩阵、发现 |
| `detection-rule-engineering` | 生成 Sigma/Yara/Suricata/WAF 规则 | 可部署检测规则 + 测试调优 |
| `network-config-audit` | 核查防火墙/交换机配置、安全基线 | 风险项清单、加固命令 |
| `code-review` | 源码安全审计（已有，偏漏洞发现） | 代码漏洞 CWE 报告 |

---

## 3. 任务设计决策流程

```
收到输入
  │
  ├─ 输入是访问日志 / HTTP 流量? ──> web-log-analysis
  ├─ 输入是入侵告警 / 怀疑失陷?   ──> incident-response
  ├─ 输入是可疑样本 / 要主动搜威胁?──> threat-hunting
  ├─ 输入是设备配置?             ──> network-config-audit
  ├─ 输入是源代码?               ──> code-review
  ├─ 用户要求生成检测规则?        ──> detection-rule-engineering
  └─ 输入是抓包文件 (pcap)?      ──> threat-hunting (流量段) / web-log-analysis (流量段)
  │
  v
Phase 1: 执行对应子技能的核心分析
  │
  v
Phase 2: 关联扩展（同 IOC / 同主机 / 同时间窗 / 同 TTP）
  │
  v
Phase 3: 处置 / 加固 / 生成检测规则（按需）
  │
  v
Phase 4: 汇总报告
```

> **多技能协同**：真实场景常需多个子技能。例如"排查 Web 入侵"= `web-log-analysis`（找攻击请求）+ `incident-response`（查主机失陷）+ `detection-rule-engineering`（沉淀检测规则）。按输入先走主技能，再用其他技能补充。

---

## 4. Phase 1: 输入识别与执行

### 4.1 Web 攻击研判（输入：日志/流量）

```
主技能: web-log-analysis
步骤:
  1. read_file / head_file 识别日志格式
  2. grep 分阶段匹配攻击特征（扫描器/注入/XSS/穿越/Webshell/C2/暴破）
  3. grep 统计基线（高频 IP、4xx 聚集、异常时间）
  4. read_file_lines 还原攻击链上下文
  5. （授权）do_http_request 回放验证
  6. write_file 分级报告
  流量类输入: 建议切 http_flow_analyze 专注模式，本技能模式表同样适用 match_flows
```

### 4.2 失陷排查（输入：告警/怀疑失陷）

```
主技能: incident-response
步骤:
  1. 事件分类定级（Web 入侵 / 主机失陷 / 数据泄露 / 勒索 / 挖矿）
  2. 按类型执行检查点（Linux: crontab/systemd/.bashrc/authorized_keys；
     Windows: 注册表 Run/计划任务/服务/事件日志 4624/4688/7045）
  3. grep 持久化与痕迹（参考 incident-response 第 4-5 节模式）
  4. （授权）system 收集主机实时状态（ps/netstat/last）
  5. 证据保全优先 → 制定遏制策略（断网优先，避免重启）
  6. 根因分析（入口 → 横向 → 影响）
  7. write_file IR 报告；确认失陷用 cybersecurity-risk 留档
```

### 4.3 威胁狩猎（输入：样本/主动搜寻假设）

```
主技能: threat-hunting
步骤:
  1. 提出假设（基于情报/ATT&CK/异常）
  2. grep 提取 IOC（IP/域名/URL/hash）→ web_search 富化
  3. 按 ATT&CK 战术映射检测（持久化/凭据/横向/C2/防御规避）
  4. analyze_pcap 分析抓包（异常外联/beacon/隧道）
  5. 关联扩展（同 IOC/同 TTP/同时间窗）
  6. write_file 狩猎报告；有效假设 → 转 detection-rule-engineering 沉淀规则
```

### 4.4 网络设备核查（输入：设备配置）

```
主技能: network-config-audit
步骤:
  1. read_file 识别设备类型（Cisco IOS/ASA、H3C、Huawei、Juniper、iptables/firewalld）
  2. grep 按设备类型匹配风险项（明文口令/Telnet/SNMP v1/ACL 过宽/未关端口/未开日志）
  3. 风险分级 + 给出对应设备的加固命令
  4. write_file 核查报告
```

### 4.5 检测规则生成（输入：威胁描述/样本/要检测的技术）

```
主技能: detection-rule-engineering
步骤:
  1. 明确检测目标与规则类型（日志→Sigma；文件→Yara；网络→Suricata；Web→ModSecurity）
  2. grep / read_file 从样本提取稳定特征
  3. 编写规则（多特征组合降误报）
  4. 正/负样本验证命中与排除
  5. write_file 落地规则文件 + 说明
```

---

## 5. Phase 2: 关联扩展

任一子技能产出发现后，从三个维度横向扩展，避免孤立结论：

- **同 IOC 扩展**：命中的 IP/域名/hash/文件 → 在其他日志/主机/时间是否再现
- **同主机扩展**：涉事主机 → 全面排查（web-log-analysis + incident-response 联动）
- **同时间窗/TTP 扩展**：同时间段的相关事件、同 ATT&CK 技术的其他痕迹

典型联动：
- `web-log-analysis` 发现 Webshell 访问 → 立即 `incident-response` 排查该 Web 主机失陷
- `incident-response` 发现异常外联 → `threat-hunting` 用 `analyze_pcap` 深挖 C2
- `threat-hunting` 验证某 TTP → `detection-rule-engineering` 沉淀为 Sigma/Yara 规则
- `network-config-audit` 发现 ACL 过宽 → 评估是否为入侵路径，联动排查

---

## 6. Phase 3: 处置 / 加固 / 规则化（按需）

根据分析结论输出可执行动作：

| 场景 | 处置/加固动作 | 产出 |
|------|-------------|------|
| 确认失陷 | 遏制策略（断网/封禁/取证）、根除、恢复 | IR 处置步骤 |
| 配置缺陷 | 设备加固命令、最小化 ACL、关闭不安全服务 | network-config-audit 加固清单 |
| 重复威胁 | 生成检测规则（Sigma/Yara/Suricata/WAF） | detection-rule-engineering 规则文件 |
| Web 漏洞被利用 | 联动 `code-review` 定位代码 sink、修复建议 | 漏洞修复建议 |

---

## 7. Phase 4: 报告输出

```
Step 1: read_file 读取各阶段产出的证据/结果文件
Step 2: 按严重程度汇总发现
Step 3: write_file 生成报告

蓝队报告结构:
  1. 执行摘要 — 任务范围、输入、关键结论、整体风险评级
  2. 分析范围 — 涉及资产/日志源/时间跨度/数据量
  3. 发现详情（按严重程度排序）
     每条: 类型、严重度、证据（日志/配置原文）、影响、ATT&CK 映射（如适用）
  4. 攻击链 / 事件时间线（如涉及入侵）
  5. 处置与加固建议（可执行命令/规则/步骤）
  6. 检测规则建议（如适用）
  7. 附录 — 工具列表、原始证据引用
```

**严重程度判定参考**：
- **严重**：核心系统失陷、域控被控、敏感数据泄露、勒索扩散、C2 已建立
- **高危**：Webshell 落地、主机失陷、横向移动迹象、明文特权口令、公网 Telnet、可写 SNMP
- **中危**：未成功的攻击、已知漏洞未利用、SNMP v1/v2c、配置缺陷、未开日志
- **低危**：扫描探测、信息泄露、ICMP 全开、低危基线不符
- **信息**：建议性加固、版本信息

---

## 8. 常见场景快速参考

### 8.1 场景："分析这段 Nginx 日志有没有被攻击"

```
1. read_file 识别日志格式
2. grep 攻击特征（参考 web-log-analysis 模式表）
3. grep 统计基线（高频 IP / 4xx 聚集）
4. read_file_lines 还原可疑 IP 的请求序列
5. （授权）do_http_request 回放验证
6. write_file 分级研判报告
```

### 8.2 场景："这台服务器好像被黑了，帮我排查"

```
1. incident-response: 事件分类定级
2. grep 持久化项（crontab/systemd/.bashrc/authorized_keys/注册表）
3. （授权）system: ps/netstat/last 收集实时状态
4. analyze_pcap 看是否有异常外联（如有抓包）
5. 证据保全 → 遏制策略 → 根因分析
6. write_file IR 报告；失陷确认用 cybersecurity-risk 留档
```

### 8.3 场景："我们怀疑内网有 APT，做一次威胁狩猎"

```
1. threat-hunting: 提出假设（基于情报/ATT&CK）
2. grep 提取 IOC → web_search 富化
3. 按 ATT&CK 战术逐项检测（持久化/凭据/横向/C2/防御规避）
4. analyze_pcap 分析异常外联/beacon
5. 关联扩展 → 验证假设
6. write_file 狩猎报告；有效线索 → detection-rule-engineering 沉淀规则
```

### 8.4 场景："核查这台防火墙/交换机的配置安全性"

```
1. read_file 识别设备类型
2. grep 按设备类型匹配风险项（参考 network-config-audit 检查项）
3. 风险分级 + 给出加固命令
4. write_file 核查报告
```

### 8.5 场景："帮我写一条规则检测 mimikatz / 这个 webshell"

```
1. detection-rule-engineering: 确定规则类型
2. grep / read_file 提取稳定特征
3. 编写规则（Sigma/Yara/Suricata/WAF）
4. 正负样本验证
5. write_file 规则文件 + 说明
```

### 8.6 场景："把这个可疑样本分析一下"

```
1. read_file 读取样本（注意二进制用十六进制特征）
2. grep / decode 提取字符串、解码 payload
3. threat-hunting: 提取 IOC → web_search 情报富化
4. （可选）detection-rule-engineering 生成 Yara 规则
5. write_file 分析报告
```

---

## 9. 文件组织规范

```
<工作目录/ArtifactsDir>/
  evidence/        # 原始证据（日志片段、配置、样本提取）
  analysis/        # 各子技能的分析过程与发现
  rules/           # 生成的检测规则（sigma/yara/suricata/waf）
  report/          # 最终报告
    blue-team-report.md
```

---

## 10. 执行检查清单

- [ ] 输入类型已识别，主子技能已选定
- [ ] 核心分析已执行（grep/analyze_pcap 等）
- [ ] 发现均有原始证据支撑
- [ ] 已做关联扩展（同 IOC/同主机/同时间窗）
- [ ] 处置/加固/规则建议可执行
- [ ] 发现按严重程度分级
- [ ] 报告已生成并写入文件
- [ ] 确认的风险已留档（write_file / cybersecurity-risk）
