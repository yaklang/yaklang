---
name: detection-rule-engineering
description: >
  检测规则编写与转换技能。根据威胁描述、日志样本、PCAP 或 ATT&CK 技术，生成 Sigma、Yara、
  Suricata、Nginx WAF/ModSecurity 检测规则，支持规则间的语义转换（如 Sigma 转换为 SPL/KQL/
  Elastic 查询），并提供规则测试与误报漏报调优指导。通过 grep 从样本提取特征，用 write_file
  落地规则文件。当用户要求编写/生成检测规则、Sigma/Yara/Suricata/WAF 规则、规则转换或调优时，
  应使用此技能。
---

# 检测规则工程技能 (Detection Rule Engineering)

将威胁知识转化为可部署的检测规则。覆盖 Sigma（日志/SIEM）、Yara（文件/样本）、
Suricata（网络）、ModSecurity/Nginx WAF（Web）四类规则，以及 Sigma 向各 SIEM 查询语言的转换。
通过 `grep` 从日志/样本中提取特征，用 `write_file` 生成规则文件，并提供测试调优方法。

---

## 1. Sigma 规则（日志 / SIEM 检测）

Sigma 是日志事件的通用签名格式，一份规则可转换为各 SIEM 的查询语言。

### 1.1 完整结构

```yaml
title: 检测 Mimikatz 凭据转储            # 必填，简明描述
id: 5t2a9b3e-xxxx-xxxx-xxxx-xxxxxxxxxxxx   # 建议唯一 UUID
status: experimental                       # experimental|test|stable
description: 检测 mimikatz 转储 LSASS 凭据
references:
    - https://attack.mitre.org/techniques/T1003/001/
author: blue-team
date: 2026/06/20
logsource:                                  # 数据源
    product: windows
    service: security
detection:                                  # 核心：选择器 + 条件
    selection:
        EventID: 10                         # ProcessAccess
        TargetImage|endswith: lsass.exe
        GrantedAccess: '0x1410'             # 常见 LSASS 访问掩码
    filter legitimate_edr:
        SourceImage|startswith: 'C:\Program Files\EDR\'
    condition: selection and not filter legitimate_edr
fields:                                     # 输出字段
    - ComputerName
    - SourceImage
    - TargetImage
falsepositives:                             # 已知误报
    - 合法 EDR / 调试器访问 LSASS
level: high                                 # informational|low|medium|high|critical
tags:                                       # ATT&CK 标签
    - attack.credential_access
    - attack.t1003.001
```

### 1.2 选择器（selection）修饰符

Sigma 字段支持修饰符（用 `|` 连接），常用：

| 修饰符 | 含义 | 示例 |
|--------|------|------|
| `contains` | 包含 | `CommandLine\|contains: mimikatz` |
| `contains|all` | 同时包含全部 | `CommandLine\|contains\|all: [a, b]` |
| `startswith` | 前缀 | `Image\|startswith: 'C:\Temp\'` |
| `endswith` | 后缀 | `TargetImage\|endswith: lsass.exe` |
| `re` | 正则 | `CommandLine\|re: '.*-enc\s+[A-Za-z0-9+/=]{20,}'` |
| `base64` | base64 编码匹配 | 自动编码待匹配值 |

### 1.3 condition 表达式

- `selection` — 命中即告警
- `selection and not filter` — 排除误报
- `selection1 or selection2` — 多场景
- `selection | count() by field > 10` — 聚合（如同一 IP 登录失败 > 10 次）

---

## 2. Yara 规则（文件 / 样本检测）

Yara 用于基于文本/十六进制/正则模式识别恶意文件与内存样本。

### 2.1 完整结构

```yara
rule Webshell_PHP_Eval_Download {
    meta:
        description = "PHP webshell 使用 eval + 下载执行"
        author = "blue-team"
        date = "2026-06-20"
        reference = "ATT&CK T1505.003"
        severity = "high"
    strings:
        $eval     = "eval("                  // 文本字符串
        $assert   = "assert(" nocase         // 大小写不敏感
        $b64      = "base64_decode(" nocase
        $hex1     = { 65 76 61 6C 28 }       // 十六进制（eval(）
        $re_cmd   = /system\s*\(|exec\s*\(|passthru\s*\(/   // 正则
        $network  = "file_get_contents" nocase
    condition:
        // 文件大小限制 + 多字符串组合降低误报
        filesize < 100KB and
        ($eval or $assert or $b64) and
        ($re_cmd or $network) and
        uint16(0) == 0x3C3F                  // 文件头 <?（PHP 文件）
}
```

### 2.2 字符串类型

| 类型 | 语法 | 适用 |
|------|------|------|
| 文本 | `$a = "mimikatz"` | 明文字符串 |
| 文本 nocase | `$a = "Mimikatz" nocase` | 大小写不敏感 |
| 十六进制 | `$a = { 6D 69 6D 69 }` | 二进制、绕过简单混淆 |
| 正则 | `$a = /payload_[a-z]{8}/` | 变形模式 |
| 多行宽字节 | `$a = "xxx" wide ascii` | 匹配 ASCII 与 UTF-16 |

### 2.3 condition 要点
- 用 `filesize < N` 限定，避免大文件全扫
- 用多字符串组合（`and`）降低误报，避免单字符串宽匹配
- 用 `uint16(0)` / `uint32(0)` 匹配文件魔数做类型预筛

---

## 3. Suricata 规则（网络检测 / IDS）

```suricata
alert http $HOME_NET any -> $EXTERNAL_NET any (
    msg:"[BLUE-TEAM] 可疑 SQL 注入特征 in URI";
    flow:established,to_server;
    http.uri; pcre:"/union[\s%20]*select|'\s*or\s*'?1=1|sleep\s*\(/Ui";
    classtype:web-application-attack;
    sid:9900001; rev:1;
    reference:url,attack.mitre.org/techniques/T1190/;
    metadata:attack_target Server, created_at 2026_06_20;
)
```

关键字段：
- `alert <proto> <src> <sport> -> <dst> <dport>` — 流向
- `msg` — 告警标题（建议统一 `[BLUE-TEAM]` 前缀便于检索）
- `flow:established,to_server` — 只看已建立连接的请求方向
- `http.uri`/`http.request_body`/`http.user_agent` — 协议字段锚定
- `content:"...";` — 精确内容；`pcre:"/.../i";` — 正则（注意性能）
- `sid` 唯一规则 ID，`rev` 版本号
- `classtype`、`reference`、`metadata` — 分类与 ATT&CK 引用

要点：
- 能用 `content` + `nocase` 就别用 `pcre`（性能）
- 用 `http.uri`、`http.user_agent` 等协议字段锚定，避免全包扫描

---

## 4. Nginx WAF / ModSecurity 规则（Web 防护）

### 4.1 ModSecurity SecRule

```
SecRule REQUEST_URI "@rx (?i)(union\s+select|'\s*or\s*'?1=1|sleep\s*\()"
    "id:100001,phase:2,deny,status:403,log,msg:'[BLUE-TEAM] SQL 注入拦截',tag:'attack-sqli',severity:CRITICAL"
```

- `phase`：1=请求头、2=请求体、3=响应头、4=响应体、5=日志
- 操作符：`@rx`(正则)、`@contains`、`@pm`(多模式)、`@validateByteRange`
- 变量：`REQUEST_URI`、`ARGS`、`REQUEST_HEADERS:User-Agent`、`REQUEST_BODY`
- 动作：`deny`/`log`/`pass`/`redirect`

### 4.2 Nginx 原生拦截（location + if，简易 WAF）

```nginx
# 拦截常见 SQL 注入特征
location / {
    if ($query_string ~* "(union\s+select|'\s*or\s*'?1=1|sleep\(|<script)") {
        return 403;
    }
    # 拦截已知扫描器 UA
    if ($http_user_agent ~* "(sqlmap|nikto|nuclei|masscan)") {
        return 403;
    }
    proxy_pass http://backend;
}
```

> 生产级 WAF 建议用 ModSecurity + OWASP CRS，Nginx 原生 `if` 仅做轻量拦截。

---

## 5. Sigma 规则转换（Sigma → SIEM 查询）

Sigma 规则通过转换器（sigmac / pySigma）可转为各平台查询语言：

| 目标平台 | 转换后形态 |
|---------|-----------|
| Splunk SPL | `index=win EventID=10 TargetImage="*lsass.exe" GrantedAccess="0x1410"` |
| Elastic KQL/LSL | `event.code:10 and process.target.name:*lsass.exe` |
| Microsoft Sentinel KQL | `SecurityEvent \| where EventID == 10 ...` |
| Elastic Lucene | `EventID:10 AND TargetImage:*lsass.exe` |

转换要点：
- 本技能负责**编写语义正确的 Sigma 规则**，转换由目标平台工具执行
- 转换后应在目标平台验证语法与命中，必要时为平台特性微调

---

## 6. 从样本生成规则的工作流

1. **理解威胁**：明确要检测的攻击行为 / 样本 / ATT&CK 技术
2. **提取特征**：
   - 日志样本：用 `grep` 提取稳定、独特的字段值或模式
   - 文件样本：用 `read_file` 读取，提取独特字符串/字节序列
   - PCAP：用 `analyze_pcap` 提取独特请求特征
3. **选择规则类型**：日志→Sigma；文件→Yara；网络→Suricata；Web→ModSecurity
4. **编写规则**：遵循对应结构，用多特征组合降低误报
5. **测试**：用正/负样本验证命中与排除
6. **落地**：`write_file` 保存规则文件

---

## 7. 规则测试与调优

### 7.1 验证命中（查全）
- 用已知正样本（攻击日志/恶意样本）测试，确认规则能命中
- 漏报 → 放宽条件或补充特征变体

### 7.2 控制误报（查准）
- 用正常流量/样本测试，确认不误报
- 误报 → 增加 `filter`/`not` 排除条件，或提高组合要求（多特征 `and`）

### 7.3 调优原则
- **稳定性优先**：选攻击者难以避免的特征（如 LSASS 访问掩码、固定 beacon 路径），而非易变值
- **组合降误报**：单特征易误报，用"特征 A 且 特征 B"组合
- **分阶段写**：先写高置信度核心特征，再迭代补充覆盖
- **标注误报**：在 `falsepositives` 字段记录已知合法场景

---

## 8. 工具使用指南

### 8.1 grep 提取特征
- 从日志/样本中提取候选字符串与模式
- 统计特征出现频率，选高频且独特的作为规则锚点

### 8.2 read_file 读取样本
- 读取文件样本内容，提取 Yara 字符串
- 注意二进制文件需用十六进制字符串匹配

### 8.3 write_file 落地规则
- 按规范命名（如 `sigma_t1003_001_lsass_dump.yml`、`yara_webshell_php.yar`）
- 规则文件路径写入报告，便于部署

---

## 9. 规则输出格式

每个生成的规则文件应附带说明：

- **规则类型**：Sigma / Yara / Suricata / ModSecurity
- **检测目标**：攻击行为 / 样本 / ATT&CK 技术 ID
- **规则内容**：完整规则（可直接保存部署）
- **正样本验证**：用什么样本验证过命中
- **已知误报**：在 `falsepositives` 中记录的场景
- **调优建议**：部署后若误报/漏报的调整方向
- **部署位置**：该规则应部署到哪个系统（SIEM / IDS / WAF / 终端）
