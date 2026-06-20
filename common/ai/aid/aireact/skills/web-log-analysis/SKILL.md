---
name: web-log-analysis
description: >
  Web 访问日志与 HTTP 流量研判技能。基于 grep 文本搜索与请求回放，从 Nginx/Apache/IIS/Tomcat
  访问日志以及 Yakit 抓取的 HTTP 流量中识别漏洞扫描器、SQL 注入、XSS、路径穿越、命令注入、
  Webshell 上传与访问、C2 回连 beacon、暴力破解等攻击行为，结合统计基线发现异常，并输出按
  严重程度分级的时间线与证据报告。当用户提供访问日志、要求分析 Web 攻击、排查入侵、研判可疑
  HTTP 流量时，应使用此技能。
---

# Web 流量与日志研判技能 (Web Log Analysis)

基于 `grep` 文本搜索对 Web 访问日志和 HTTP 流量进行系统化安全研判。
通过精心构造的正则模式在日志中定位攻击特征，结合统计基线发现异常请求，
对可疑请求可使用 `do_http_request` 回放验证，最终将研判结果写入分级报告。

---

## 1. 研判流程

### 1.1 识别日志格式

先用 `read_file` 或 `head_file` 查看日志头部，判断格式与字段含义：

| 日志源 | 格式特征 | 字段顺序（典型） |
|--------|---------|----------------|
| Nginx combined | `$remote_addr - $remote_user [$time] "$request" $status $body_bytes "$referer" "$user_agent"` | IP 时间 方法 路径 状态 字芈 referer UA |
| Apache common | `host ident authuser [date] "request" status bytes` | 同上去掉 referer/UA |
| IIS W3C | 字段以空格分隔，首行 `#Fields:` 声明字段名 | `date time s-ip cs-method cs-uri-stem ...` |
| Tomcat | `%h %l %u %t "%r" %s %b` | 类似 Apache |

判断要点：
- 时间字段的位置和格式（用于后续时间线重建）
- 是否有独立 User-Agent、Referer、响应大小字段（决定可用哪些检测维度）
- 是否为 JSON 结构化日志（若是，grep 仍可工作，但模式需适配 JSON 转义）

### 1.2 分阶段 grep 扫描攻击特征

按攻击类型分阶段扫描，每次聚焦一类（避免模式合并导致结果过载）：

- **阶段一：自动化扫描器与爬虫**（UA 特征）
- **阶段二：注入类攻击**（SQLi / 命令注入 / SSTI / XXE）
- **阶段三：跨站与路径类**（XSS / 路径穿越 / LFI）
- **阶段四：Webshell 与后门**（敏感路径访问 / 特征参数）
- **阶段五：C2 回连 beacon**（固定周期高频请求）

### 1.3 统计基线检测

对单维度异常做聚合统计（见第 4 节），找出偏离基线的 IP / 路径 / 时间窗。

### 1.4 上下文验证与回放

对 grep 命中的可疑请求：
- 用 `read_file`/`read_file_lines` 查看该 IP 前后的请求序列，还原攻击链
- 必要时用 `do_http_request` 回放该请求（**仅在已授权前提下**），观察响应确认是否成功

### 1.5 时间线重建与报告

按时间排序所有确认的攻击行为，输出分级报告。

---

## 2. 攻击特征 grep 模式表

> 以下模式以正则形式给出，`grep` 调用时 `pattern-mode` 设为 `regexp`。
> 模式默认大小写不敏感（如需精确匹配请自行调整）。

### 2.1 自动化扫描器 (Scanner User-Agent)

| 特征 (grep regexp) | 说明 |
|--------------------|------|
| `sqlmap|nikto|nuclei|masscan|zgrab|wpscan|dirbuster|gobuster|hydra|metasploit|burpcollaborator|acunetix|nessus|awvs|nmap scripting` | 已知扫描器 UA |
| `python-requests|go-http-client|curl\/|wget\/|libwww-perl|scrapy|httpx|ja3f` | 编程式 HTTP 客户端（结合行为判断） |
| `(bot|crawler|spider)` | 需结合是否伪装正常 UA 进一步判断 |

研判要点：扫描器常伴随大量 4xx/404 + 少量 200，且请求路径呈字典枚举特征（`/admin`、`/.env`、`/phpmyadmin`、`/wp-admin`）。

### 2.2 SQL 注入 (SQLi)

| 特征 (grep regexp) | 说明 |
|--------------------|------|
| `union\s+select|union%20select` | 联合查询注入 |
| `'\s*or\s*'?\d|'\s*or\s*1=1|or%201=1` | 布尔注入 |
| `sleep\(|benchmark\(|pg_sleep|waitfor\s+delay` | 时间盲注 |
| `extractvalue|updatexml|xp_cmdshell` | 报错注入 / MSSQL 命令执行 |
| `information_schema|sysobjects|syscolumns` | 数据库结构探测 |
| `%27|%22|--|0x[0-9a-f]+` | URL 编码的单双引号 / 注释符 / 十六进制 |

### 2.3 XSS (跨站脚本)

| 特征 (grep regexp) | 说明 |
|--------------------|------|
| `<script|%3Cscript|onerror=|onload=|onclick=` | 脚本标签 / 事件处理器 |
| `javascript:|alert\(|prompt\(|confirm\(` | JS 协议 / 弹窗函数 |
| `<img|<svg|<iframe|%3Cimg|%3Csvg` | 常见注入载体标签 |
| `document\.cookie|window\.location` | DOM 操作 payload |

### 2.4 路径穿越与文件包含 (LFI / Path Traversal)

| 特征 (grep regexp) | 说明 |
|--------------------|------|
| `\.\./|\.\.\\|\.\.%2f|\.\.%5c` | 目录回溯 |
| `%2e%2e%2f|\.\.%252f` | 双重编码绕过 |
| `/etc/passwd|/etc/shadow|/proc/self/environ|c:\\windows\\win\.ini` | 敏感文件读取探测 |
| `php://|file://|data://|expect://|phar://` | PHP 伪协议（文件包含） |

### 2.5 命令注入 (Command Injection)

| 特征 (grep regexp) | 说明 |
|--------------------|------|
| `;\s*(ls|cat|id|whoami|uname|ping|wget|curl)|\|\|\s*\w|&&\s*\w` | 命令分隔符 + 命令 |
| `\$\(|\` \`|` | 命令替换语法 |
| `%0a|%0d` | 换行符注入（CRLF / 命令换行） |
| `nc\s+-e|bash\s+-i|/bin/sh|/bin/bash` | 反弹 shell 特征 |

### 2.6 SSTI / 模板注入

| 特征 (grep regexp) | 说明 |
|--------------------|------|
| `\{\{.*\}\}|\{%.*%\}` | Jinja2/Twig 模板语法 |
| `\$\{.*\}|#\{.*\}` | FreeMarker/Thymeleaf 表达式 |
| `7\*7|49` | 数学表达式探测（返回 49 即存在） |

### 2.7 Webshell 与后门访问

| 特征 (grep regexp) | 说明 |
|--------------------|------|
| `\.(php|jsp|asp|aspx)\?\w+=` | 脚本文件 + 查询参数（命令执行型 shell） |
| `/shell|/cmd|/webshell|/c99|/r57|/b374k|/eval` | 常见 shell 文件名 |
| `(cmd|command|exec|run|do|action|func)=` | 常见命令执行参数名 |
| `\.(php|jsp|asp)x?\s+HTTP/1\.[01]` 配合异常 UA 或固定 IP | 可疑脚本直接访问 |

研判要点：关注被频繁 POST 访问、参数值长度异常、响应 200 但来自非常规路径的脚本文件；结合文件系统 `find_file` 在 Web 根目录搜索未知 `.php/.jsp` 文件。

### 2.8 C2 回连 beacon 特征

| 特征 (grep regexp) | 说明 |
|--------------------|------|
| 固定 UA + 固定路径 + 固定间隔 | 通过 `grep` 该 IP/UA 后人工观察时间戳间隔 |
| `/favicon\.ico`、`/(index|portal)\.html` 等伪装路径 + HEAD 请求 + 周期性 | 伪装型 beacon |
| 请求体/响应体大小固定、Jitter 极小 | 流量层面特征（需 `analyze_pcap`） |

研判要点：单条请求无法判定，需聚合该源 IP 的所有请求观察周期性（见第 4 节基线检测），并配合 `analyze_pcap` 查看外联流量。

### 2.9 暴力破解与撞库

| 特征 (grep regexp) | 说明 |
|--------------------|------|
| `/login|/signin|/api/login|/oauth/token` | 登录端点 |
| 搭配大量不同用户名参数 + 高频 401/403 | 用户名枚举 |
| 搭配同一用户名高频请求 + 401/429 | 密码爆破 |

---

## 3. 可疑状态码与响应特征

| 特征 | 含义 |
|------|------|
| 同一 IP 大量 `404` 聚集 | 目录/文件枚举扫描 |
| `404` 后跟 `200`（同 IP） | 扫描命中可用路径 |
| `500` + 注入特征 payload | 注入触发服务端异常（可能成功） |
| `200` + 异常大响应体 + 探测路径 | 数据库结构/文件内容泄露 |
| `302` → 登录后页面 + 来自非常规 IP | 潜在越权或撞库成功 |

---

## 4. 统计基线检测

用 `grep` + 人工聚合发现偏离基线的异常（日志量大时建议先按 IP/时间切片）：

- **单 IP 高频请求**：`grep` 某 IP 后统计请求数，远超正常用户基线
- **高频 4xx/5xx 聚集**：某 IP 短时间大量错误响应 = 扫描/枚举
- **异常时间窗**：非业务时段（如凌晨 3-5 点）的集中请求
- **异常 User-Agent 多样化**：同一 IP 频繁更换 UA = 爬虫/扫描器
- **异常响应体大小**：对同一路径，某次响应体显著大于其他 = 数据泄露

---

## 5. 与 HTTP 流量研判协同

当研判对象是 **Yakit 抓取的 HTTP 流量**（而非文本日志）时：

- 切换到 `http_flow_analyze` 专注模式，可使用 `query_http_flows` / `match_flows` / `get_http_flow_detail` / `record_http_flow_evidence` 等专用动作
- 本技能的攻击特征模式表同样适用于流量匹配（`match_flows` 的匹配器规则）
- 对确认的恶意流量用 `record_http_flow_evidence` 留证，或 `dispatch_fuzz_test` 验证可利用性

---

## 6. 工具使用指南

### 6.1 grep 工具

- **pattern-mode 设为 regexp**：使用正则匹配
- **limit 合理设置**：建议 50-200，避免结果过载
- **context-buffer**：建议 50-200 字节，提供上下文
- **分类搜索**：每次只搜一类攻击特征
- **先按源 IP 过滤再搜特征**：缩小范围、还原攻击链

示例：
```
{
  "path": "/var/log/nginx/access.log",
  "pattern": "union\\s+select|'\\s*or\\s*'1|sleep\\(|<script",
  "pattern-mode": "regexp",
  "limit": 100,
  "context-buffer": 100
}
```

### 6.2 read_file_lines / tail_file

- `tail_file`：查看最新日志，适合实时事件排查
- `read_file_lines`：按行范围读取，定位 grep 命中点的前后上下文

### 6.3 do_http_request 回放验证

仅**在已授权范围**内回放可疑请求以确认可利用性：
- 复制命中请求的方法、路径、参数、请求头
- 观察响应状态码、响应体、响应头，判断攻击是否成功

### 6.4 结果保存

用 `write_file` 将研判结果写入报告。

---

## 7. 研判输出格式

按严重程度分级：**严重 > 高危 > 中危 > 低危 > 信息**

每条发现包含：
- 攻击类型
- 源 IP 与时间戳
- 命中的日志原文 / 证据片段
- 命中的特征模式
- 关联的攻击链（前后请求）
- 风险说明（是否成功 / 影响范围）
- 处置建议

最终报告应包含：
- 研判范围（日志源、时间跨度、总请求数）
- 攻击时间线（按时间排序的关键事件）
- 发现统计（各严重程度数量、攻击类型分布）
- 涉事的源 IP / 目标路径清单
- 总体安全结论与处置优先级
