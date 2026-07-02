# TrafficGuard: 内置敏感信息实时检测引擎

MITM 实时流量中敏感凭证/密钥泄漏的"超级正则组"检测引擎。默认随 MITM 开启、不可关闭,
命中即把流量标红、按既有 MITM 规则机制标注(extracted_data 高亮 + flow.Payload)并生成一条
Risk(每流量合并为一条, 严重度上限中危, 描述用 markdown 给出命中值与前后上下文以便人工判真假)。

## 架构: 三阶段扫描(低开销实时热路径)

```
请求/响应报文
   │
   ▼  阶段一(热路径): minirehs MVS existence-only
   │   内置高危正则统一编译为一个位并行 NFA 数据库
   │   纯位运算 + Aho-Corasick 字面量预过滤, 一次扫描判定"是否可能命中"
   │   ── 纯净流量(绝大多数)在此快速排除, 代价极低
   ▼  (仅命中时)阶段二(冷路径): go-pcre2-lite 底层接口
   │   对命中的具体规则用 PCRE2(cgo, 线性时间, 字节级偏移)精确定位提取
   ▼  (仅命中时)阶段三: validateFinding 上下文/值形态校验(见 validators.go)
   │   厂商自有域抑制(Google)、JWT 方向+alg 校验、口令字段值收紧, 剔除明显误报
   ▼
Finding → 合并(每流量一条) → 标红 + extracted_data 标注 + Payload + Risk
```

## 覆盖的敏感特征

私钥(PEM)、云厂商凭证(AWS AKIA/SK、Google API Key/OAuth、Azure AccountKey)、
SaaS Token(GitHub/GitLab/Slack/Stripe/OpenAI/SendGrid/Twilio/Mailgun/Square/AWS MWS/Discord)、
通用认证凭证(JWT)、数据库/中间件连接串、敏感字段(password/secret/api_key/token)、
URL 凭证参数、自定义鉴权头(X-API-Key/X-Auth-Token)。规则集以 `rules.go` 的 `builtinRules` 为准。

> 注: `Authorization: Bearer/Basic` 头(原规则 20/21)已移除——正常带鉴权浏览里几乎每个请求都携带,
> 全是用户对目标站点的第一方会话凭证, 噪声大且非泄漏。X-API-Key 等自定义头(规则 25)保留。

## 误报治理(validators.go 第三阶段校验)

基于真实历史流量取证, 在 PCRE2 精确提取后按 host/方向/值形态做上下文校验, 剔除典型误报:

- **厂商自有域(第一方噪声)抑制**: Google/Chrome 自有域上的第一方自用流量一律丢弃 ——
  不仅是 Google API Key/OAuth(`AIza`/`ya29.`, 规则4/5), 还包括 JWT(19)、通用 api-key/凭证字段(23)、
  URL 凭证参数(24)、自定义鉴权头(25)。典型: `content-autofill.googleapis.com` 的 Chrome 自动填充
  请求携带 `x-goog-api-key: AIza...`(同时命中 4/23/25)。自有域后缀见 `validators.go` 的 `googleOwnedSuffixes`
  (google.com/googleapis.com/gstatic.com/gvt1.com/app-measurement.com/doubleclick.net ...),
  规则集见 `vendorFirstPartyNoiseRules`。强特征第三方凭证(AKIA/ghp_/sk_live_/PEM 等)即便在 Google 域也保留。
- **JWT 校验**: 请求方向的 JWT 视为第一方会话凭证(等同 Authorization 头)丢弃;
  响应/脚本方向要求首段是含 `alg` 的真实 JWT header, 否则视为普通 base64 `eyJ` 块丢弃。
  ── 精准消除 data.bilibili.com 等埋点请求里的 JWT/ticket 噪声。
- **口令字段值收紧(规则23, 分层 + 注释感知)**: 规则23 提取键值对的"值", 按三层判定, 在
  "登录框不能误报"与"注释里的默认口令必须报出"之间取得平衡(见 `validators.go` 的 `validateSecretField`):
  - **A 层(永远抑制)**: 一眼是代码的源码型噪声 —— 保留字(`function`/`true`...)、运算符/路径开头
    (`/passApi`、`+encodeURIComponent`)、成员表达式 `a.b.c`、掩码 `****`。被注释掉的代码片段同样不报。
  - **B 层(常规抑制 / 注释中放行)**: 登录框/页面的**自然语言文案**, 即用户反馈最强烈的
    "访问任意登录页就报 password"误报源头 —— 含 **CJK** 本地化文案(`password:"设置密码"` /
    `pwd:"忘记密码"` / `"请输入密码"`)、通用 **UI 词**(`Password`/`Login`/`Submit`/`Username` 等,
    见 `loginFormLabelWords`)、纯小写 **slug**(`routes-api-failed`)。常规上下文一律丢弃;
    但若命中落在**注释**里(`<!-- -->` / `/* */` / 行首 `//` 或 `#`, 见 `isInCommentContext`),
    则视为开发者写进注释的**默认/初始口令**(如 `<!-- 默认密码 password: admin -->`)而**必须报出**
    (注释里口令往往很短, 此时最小长度放宽到正则下限 4)。
  - **C 层(保留)**: 看起来像真实凭证的值(大小写+数字混合、含特殊字符、足够长度的随机串)。
  - **不跳过 JS**(硬编码/注释里可能藏真凭证), 只收紧值形态; 注释判定刻意只认"行首"行注释标记
    (不扫行内 `//`), 以免把压缩 JS 里的 `http://` / 协议相对地址 `//cdn` 误判成注释而把本该抑制的文案放行。

## 关键设计

- **默认开启、不可关闭**: 保证用户始终感知敏感信息泄漏线索。
- **每流量一条 Risk**: 一个请求+响应的全部命中合并为单条 Risk; 严重度统一受上限约束(最高中危,
  见 `SeverityCeiling`); 合并 Hash 基于 `host/path + 命中规则ID集合`(去掉 query), 同一接口的
  重复命中走 `CreateOrUpdateRisk` 去重更新而非反复新建, 显著降频。
- **markdown 描述判真假**: Description 给出每条命中的命中值 + 命中处前后上下文(以 「」 标注),
  让人一眼判断真凭证还是源码/埋点误报; Details(机器可读)仍只含脱敏值与指纹。
- **既有标注机制**: 命中即 `flow.Red()` + `trafficguard-secret` 总括 TAG + 每条命中规则名 TAG
  (`TrafficGuard: <规则名>`, 去重, 便于在 History 按具体命中规则筛选); 每条命中写一条
  `extracted_data`(SourceType=httpflow, TraceId=HiddenIndex, DataIndex/Length 高亮, 进"提取数据"专表);
  命中内容写入 `flow.Payload`, 流量列表一眼可见。
- **高亮按 rune(字符)下标对齐**: `extracted_data` 的 DataIndex/Length 必须是 rune 下标(与 yaklang HookColor
  及前端约定一致, 见 `go-pcre2-lite/regexp2`: Capture.Index/Length 为 rune 下标), 而阶段二 PCRE2 给出的是
  byte 偏移。报文(尤其含中文注释/字符串的 JS/JSON)命中点之前若有多字节字符, 直接用 byte 偏移会让高亮整体右移;
  落库前统一用 `runeSpan` 换算为 rune 下标, 修复"偏移/颜色错位"。
- **被过滤命中流量改插件类型保存**: 本应被 MITM 过滤(静态资源/大 JS 等)但命中敏感信息的流量,
  不再强制取消过滤进入 MITM History(避免污染 MITM TAB), 而是置 `SourceType=scan` +
  `FromPlugin=内置敏感信息检测` 以"插件流量"形式保存到插件输出; 也不再因命中而强制劫持。
- **fail-open**: 任何异常仅记日志、绝不阻断代理流量。

## 调优: minLiteralLen

实测在真实流量上, 阶段一字面量预过滤的最小字面量阈值是吞吐关键:

| minLiteralLen | always-on | 合成纯净流量 | 真实流量 | 命中率 |
|:---:|:---:|:---:|:---:|:---:|
| 2 | 1 | 62 MB/s | 4.6 MB/s | 6/80 |
| 3 | 4 | 35 MB/s | 5.9 MB/s | 6/80 |
| **4** | **11** | **15 MB/s** | **15.7 MB/s** | **6/80** |
| 5 | 15 | 9 MB/s | 9.2 MB/s | 6/80 |

选 minLiteralLen=4: 在真实流量上吞吐最优(15.7MB/s, 较 2/3 提升 ~3x),
且检测率不变; 各类流量吞吐一致稳定。always_on=11 受控(<= 硬上限 16), 无 regexp2 兜底。

## 集成

MITM 保存 flow 路径上(v1 `grpc_mitm.go` / v2 `grpc_mitm_v2.go`):

1. 过滤判定前调用 `trafficguard.ScanFindings(reqUrl, plainRequest, plainResponse)` 拿到命中
   (`reqUrl` 提供 host 上下文给第三阶段校验)。
2. 本应被过滤但命中敏感信息时(`tgSaveAsPlugin`), 在 `CalcHash` 前置 `flow.SourceType=scan` +
   `flow.FromPlugin=内置敏感信息检测`, 以插件流量形式保存(不进 MITM TAB)。
3. `InsertHTTPFlowEx` 之前调用 `trafficguard.ApplyToFlow(db, flow, findings, req, rsp)`:
   标红 + TAG + 写 extracted_data + 写 flow.Payload + 合并保存一条 Risk。

便捷入口: `trafficguard.MarkAndSaveRisksForFlow(db, flow, req, rsp)`(扫描+应用一次完成)。

## 真实流量仿真(本机 ~/yakit-projects 历史)

从本地历史 2095 条真实流量抽样 80 条:
- 纯净流量扫描 ~15MB/s(平均 93KB 流量 ~6ms, 在独立 goroutine 不阻塞代理);
- 命中 6/80(均为真实凭证: JWT、bilibili GAIA 口令/token 字段), 零误报高危。

## Benchmark

```
BenchmarkScanClean32K        ~15 MB/s   (合成纯净)
BenchmarkScanClean256K       ~15 MB/s
BenchmarkScanHit32K          ~15 MB/s   (含命中)
BenchmarkSimulationRealCorpus ~15 MB/s  (真实历史流量)
```

并发: `DefaultScanner` 单例 + minirehs Group 并发安全只读, Scratch sync.Pool 复用;
`-race` 16 goroutine × 200 次扫描无数据竞争。
