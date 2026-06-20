---
name: email-phishing-analysis
description: >
  钓鱼邮件研判技能。解析邮件（.eml / MIME）并系统化分析其是否为钓鱼/社工邮件，覆盖发件人真实性
  （SPF/DKIM/DMARC、显示名欺骗、Reply-To 不一致、源 IP 溯源）、诱饵话术、钓鱼链接（仿冒域名、
  IDN 同形异义、缩短链接、裸 IP、HTML 伪装）、恶意附件（危险类型、宏、sha256 情报）、正文编码隐藏
  与 IOC 提取富化。通过 parse_email 工具（底层 mail 库）解析邮件、grep/auto_decode 处理原始内容、
  web_search 富化情报，输出分级研判报告。当用户提供邮件（.eml）、要求分析钓鱼邮件、研判可疑邮件、
  邮件取证、识别社工攻击时，应使用此技能。
---

# 钓鱼邮件研判技能 (Email Phishing Analysis)

对邮件（.eml / RFC 5322 + MIME）进行系统化研判，判定是否为钓鱼 / 社工邮件。
核心工具是 `parse_email`（底层 `mail` 库，已实现 MIME 解析、RFC 2047 / quoted-printable / base64
解码、charset 转换、SPF/DKIM/DMARC 提取、源 IP 溯源、URL/附件提取、可疑指标自动识别），
配合 `grep` / `auto_decode` 处理原始内容、`web_search` 富化情报，输出分级研判报告。

> 与 `incident-response` 的关系：若研判确认邮件已被点击/附件已落地，转入 `incident-response`
> 排查主机失陷；本技能聚焦**邮件本身的研判**。

---

## 1. 研判流程

### 1.1 解析邮件

```
工具: parse_email（参数 file=<邮件路径>）
产出: 结构化结果 —— 发件人 / 收件人 / 主题 / 认证结果 / 正文 / 附件 / URL / 可疑指标
```

- 若只有邮件原文（粘贴文本），用 `write_file` 落盘为 `.eml` 后再解析，或直接用 `mail.Parse(原文)`
- 若需从邮箱直接取证（已知 POP3 凭据），用 `mail.Fetch` / `mail.FetchList` 拉取（见第 8 节）

### 1.2 分维度研判

1. **发件人真实性**（第 2 节）—— 最关键，先判这个
2. **诱饵话术**（第 3 节）
3. **钓鱼链接**（第 4 节）
4. **恶意附件**（第 5 节）
5. **正文编码与隐藏**（第 6 节）
6. **IOC 提取与情报富化**（第 7 节）

### 1.3 分级与报告

综合各维度证据，按严重程度给出判定与处置建议（第 9 节）。

---

## 2. 发件人真实性分析（最重要维度）

### 2.1 SPF / DKIM / DMARC 判读

`parse_email` 已从 `Authentication-Results` 头提取 `spf` / `dkim` / `dmarc` 结果：

| 结果 | 含义 | 风险 |
|------|------|------|
| `pass` | 通过认证 | 低（仍可能是被攻陷的合法账号发来的） |
| `fail` / `softfail` | 域名不匹配 | **高**，强烈提示伪造 |
| `neutral` / `none` | 未配置 / 无记录 | 中，无法证明真伪 |
| `temperror` / `permerror` | 检查出错 | 中，需人工核查 DNS |

判读要点：
- **SPF fail + DMARC fail** ≈ 几乎确定伪造发件人
- DMARC 的 `policy`（none/quarantine/reject）决定接收方处置，`none` 说明域方未强制防护
- DKIM 签名域（`d=`）应与 From 头可见域一致，不一致是伪造信号
- 若完全没有 `Authentication-Results` 头，说明收件方未做认证或被剥离，需查 `Received-SPF` / `DKIM-Signature` 原始头

### 2.2 显示名与地址欺骗

- **显示名欺骗**：显示名是 `CEO 张三 <attacker@evil.com>`，收件人列表只看到"CEO 张三"
- `parse_email` 的 `suspicious` 已自动检测"显示名含其它邮箱"
- 检查 `from.display` 是否包含品牌名/人名但 `from.address` 是陌生域

### 2.3 Reply-To / Return-Path / Sender 不一致

- `Reply-To` ≠ `From`：回信会发到攻击者控制的地址（钓鱼常用）
- `Return-Path`（退信地址）与 `From` 域不一致
- `Sender` 头存在且与 `From` 不同（代表"代发"）

`parse_email` 的 `suspicious` 已自动检测 Reply-To 不一致。

### 2.4 源 IP 溯源（Received 链）

- `parse_email` 提取了 `received_ips`（Received 头链中的所有 IP）
- **最早加入的 Received（链底部）**通常揭示真实发件源
- 对源 IP 用 `web_search` 查：是否为已知恶意 IP / Tor 出口 / 与发件域不符的地理位置
- 若源 IP 属于住宅宽带 / VPN / 与企业邮件架构无关，高度可疑

### 2.5 域名相似度核查

- 发件域是否为目标品牌的**仿冒**：`micros0ft.com` / `paypa1.com` / `arnazon.com`
- 用 `grep` / 人工比对易混淆字符

---

## 3. 诱饵话术识别

从 `body_text` / `body_html` 提取正文，识别社工套路：

| 话术类型 | 特征关键词（grep） |
|---------|------------------|
| 紧迫感 | `urgent\|immediately\|立即\|马上\|24小时内\|限时` |
| 恐吓 | `suspend\|disable\|locked\|account closed\|冻结\|封号\|异常\|涉嫌` |
| 利益诱惑 | `winner\|prize\|lottery\|中奖\|退款\|补贴\|免费` |
| 权威冒充 | `IT department\|HR\|CEO\|support\|客服\|管理员\|财务` |
| 凭据索取 | `verify password\|confirm account\|login to\|验证密码\|确认账号` |

`parse_email` 的 `suspicious.urgent_subject` 已对主题做初步检测；正文需用 `grep` 补充。

研判要点：
- 异常称谓（"Dear Customer" 而非具体姓名）
- 语法/拼写错误（机器翻译 / 非母语攻击者）
- 与发件方身份不符的语气

---

## 4. 钓鱼链接分析

`parse_email` 已提取 `urls`（正文 + HTML href/src 去重）。逐条分析：

### 4.1 仿冒域名
- 域名与合法品牌相似但有细微差异（`secure-paypal.com.example.ru`）
- 子域伪装：`paypal.com.attacker.com`（真实域是 `attacker.com`）

### 4.2 IDN 同形异义（Punycode）
- 用非 ASCII 字符替换：`аррӏе.com`（西里尔字符）伪装 `apple.com`
- 用 `auto_decode` / 在线工具解码 Punycode（`xn--...`）

### 4.3 缩短 / 跳转链接
- `bit.ly` / `t.cn` / `tinyurl` 等缩短链接 → 用 `do_http_request`（HEAD/跟随重定向）还原真实目标
- 开放重定向滥用：`target.com/redir?url=http://evil.com`

### 4.4 裸 IP / 可疑主机
- URL 主机是裸 IP（`http://1.2.3.4/login`）—— `parse_email.suspicious.url_uses_ip` 已检测
- URL 含凭据（`http://user:pass@host`）—— `suspicious.url_with_credentials` 已检测
- 新注册域名 / 免费子域服务

### 4.5 HTML 伪装（显示文本 ≠ 真实 href）
- `<a href="http://evil.com">http://paypal.com</a>`：用户看到 paypal.com，点击跳 evil.com
- 用 `read_file` / `grep` 看原始 HTML 核对 href 与显示文本

### 4.6 链接情报富化
- 对每个可疑 URL 用 `web_search` 查是否为已知钓鱼/恶意 URL

---

## 5. 恶意附件识别

`parse_email` 的 `attachments` 已给出 `filename` / `content_type` / `size` / `sha256`。

### 5.1 高风险类型
| 扩展名 | 风险 |
|--------|------|
| `.docm` `.xlsm` `.pptm` | Office 宏（VBA） |
| `.lnk` | 快捷方式执行 |
| `.exe` `.scr` `.bat` `.cmd` `.com` `.msi` | 直接可执行 |
| `.js` `.vbs` `.jse` `.vba` `.hta` | 脚本执行 |
| `.iso` `.img` | 镜像绕过 MOTW |
| `.zip` `.rar` `.7z` | 压缩包藏可执行/宏文档 |

`parse_email.suspicious.dangerous_attachment` 已自动标注。

### 5.2 宏与恶意内容检测
- 对 Office 文档：`grep` 查 `AutoOpen\|Document_Open\|Shell\|WScript\|PowerShell\|CreateObject`（VBA 宏特征）
- 用 `zip_viewer` 工具解压 Office 文档（本质是 zip），`grep` `vbaProject.bin` / 内嵌宏
- 对脚本附件：`read_file` + `auto_decode` 还原内容，`grep` 恶意行为

### 5.3 附件哈希情报
- 用附件 `sha256` 做 `web_search`：是否为已知恶意样本（VirusTotal / 威胁情报报告）

---

## 6. 正文编码与隐藏

### 6.1 异常编码
- 整封正文 base64 / quoted-printable 编码但内容无害——可能为绕过邮件网关检测
- `parse_email` 已自动解码，用解码后内容研判

### 6.2 HTML 隐藏手法
- `grep` HTML 查：
  - `display:\s*none\|visibility:\s*hidden`（隐藏文字混淆检测）
  - `<!-- ... -->`（HTML 注释藏指令）
  - `font-size:\s*0\|color:\s*white`（白底白字）
  - `<iframe>` / `<form>` 跨域提交（凭据窃取）
  - 外部资源加载（`<img src=http://...>` 信标追踪）

### 6.3 多部分混淆
- `text/plain` 与 `text/html` 内容不一致（HTML 版含钓鱼，纯文本版伪装无害，绕过扫描）

---

## 7. IOC 提取与情报富化

从邮件提取全部 IOC，用 `web_search` 富化：
- **URL / 域名**：是否已知钓鱼基础设施
- **源 IP**：是否已知恶意 / 地理位置
- **附件 sha256**：是否已知恶意样本
- **发件邮箱**：是否在钓鱼团伙 IOC 库中

关联扩展（参考 `threat-hunting`）：
- 同发件人是否在企业历史邮件中出现过
- 同 URL/附件是否在其他邮件中传播（钓鱼活动范围）

---

## 8. 从邮箱直接取证（mail 库收件能力）

若研判需要从邮箱拉取可疑邮件（已知 POP3 凭据），使用 `mail` 库：

```
# 列出收件箱所有邮件摘要
list = mail.FetchList(
    mail.pop3Server("pop.example.com", 995), mail.pop3SSL(),
    mail.pop3Username("user"), mail.pop3Password("authcode"),
)

# 取回第 N 封并自动解析（结果与 parse_email 一致）
result = mail.Fetch(
    mail.pop3Server("pop.example.com", 995), mail.pop3SSL(),
    mail.pop3Username("user"), mail.pop3Password("authcode"),
    mail.messageID(3),
)
```

> 注意：收件操作需授权凭据，仅在合规取证前提下使用。

---

## 9. 工具使用指南

### 9.1 parse_email（首选）
- `file` 参数指向 `.eml`，一次解析输出全部研判所需字段
- 自动检测的可疑指标（`suspicious`）优先排查

### 9.2 grep / auto_decode（深挖原始内容）
- 对 `body_html` 用 `grep` 查隐藏手法、伪装链接
- 对编码内容用 `auto_decode` 还原

### 9.3 web_search（情报富化）
- 查 URL/IP/hash/domain 的威胁情报背景

### 9.4 do_http_request（链接还原，需授权）
- HEAD 请求缩短链接，跟随重定向看真实目标
- **仅用于研判、不得造成实际交互**

### 9.5 write_file（落报告）

---

## 10. 判定与报告格式

### 10.1 判定结论
- **确认钓鱼**：多个高危维度命中（认证失败 + 仿冒链接 + 恶意附件 / 话术）
- **疑似钓鱼**：1-2 个高危信号，需进一步情报佐证
- **垃圾/营销**：无恶意意图但未授权
- **正常邮件**：认证通过且无异常

### 10.2 严重程度参考
- **严重**：含可执行恶意附件 + 仿冒登录页 + 大范围投递
- **高危**：SPF/DKIM/DMARC 全失败 + 仿冒品牌 + 凭据钓鱼链接
- **中危**：可疑链接但无附件、或附件需用户交互且危害有限
- **低危**：营销/社工但无直接恶意载荷
- **信息**：认证正常，仅记录

### 10.3 报告结构
- **邮件标识**：主题 / Message-ID / 发件人（显示名+地址）/ 日期
- **判定结论**：钓鱼/疑似/正常 + 严重程度
- **证据明细**（按维度）：
  - 认证结果（SPF/DKIM/DMARC + 判读）
  - 发件人欺骗（显示名/Reply-To/源 IP）
  - 诱饵话术（命中的类型 + 片段）
  - 钓鱼链接（URL + 仿冒/伪装分析 + 情报）
  - 恶意附件（文件名/类型/sha256 + 情报）
  - 隐藏手法（编码/HTML 隐藏）
- **IOC 清单**：URL / IP / hash / 邮箱 / 域名
- **处置建议**：阻断链接/域名、隔离附件、全网邮件清理、用户告警、是否启动 `incident-response`
