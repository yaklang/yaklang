# TrafficGuard: 内置高危凭证实时检测引擎

MITM 实时流量中最高危凭证泄漏的"超级正则组"检测引擎。默认随 MITM 开启、不可关闭,
命中即把流量标红并生成"高危/中危" Risk(每流量合并为一条), 让用户立刻感知发生了什么。

## 架构: 两阶段扫描(低开销实时热路径)

```
请求/响应报文
   │
   ▼  阶段一(热路径): minirehs MVS existence-only
   │   25 条高危正则统一编译为一个位并行 NFA 数据库
   │   纯位运算 + Aho-Corasick 字面量预过滤, 一次扫描判定"是否可能命中"
   │   ── 纯净流量(绝大多数)在此快速排除, 代价极低
   ▼  (仅命中时)阶段二(冷路径): go-pcre2-lite 底层接口
   │   对命中的具体规则用 PCRE2(cgo, 线性时间, 字节级偏移)精确定位提取
   │   ── 命中极少, 开销可忽略
   ▼
Finding → 合并(每流量一条) → 标红 + Risk
```

这是任务书要求的"existence-only 快速候选 → 命中后再精确定位"模型:
纯净流量一次扫描快速排除, 命中流量只额外付出极少定位开销。

## 覆盖的高危特征(25 条)

私钥(PEM)、云厂商凭证(AWS AKIA/SK、Google API Key、Azure AccountKey)、
SaaS Token(GitHub/GitLab/Slack/Stripe/OpenAI/SendGrid/Twilio/Mailgun/Square/AWS MWS/Discord)、
通用认证凭证(JWT、Bearer、Basic Auth)、数据库/中间件连接串、
敏感字段(password/secret/api_key/token)、URL/Header 中的凭证参数。

每条规则: RE2 兼容 + 稳定字面量前缀 + 字符集/定长约束, 把误报复压到极低。
危险度显式标注(critical/high/warning), 命中即生成对应级别 Risk。

## 关键设计

- **默认开启、不可关闭**: 保证用户始终感知最高危凭证泄漏(任务要求)。
- **每流量一条 Risk**: 一个请求+响应的全部命中合并为单条 Risk, 标题含 Host/Path
  与命中规则名, 整体取最高等级, details 聚合所有脱敏值与指纹(绝不含明文)。
- **红色标注**: 命中即 `flow.Red()` + 内置 `trafficguard-secret` TAG,
  HTTP History 一眼可见并可用 TAG 筛选。
- **不受过滤影响**: 在 flow 保存路径上无条件执行, 在 `MirrorHTTPFlow` 之前。
- **fail-open**: 任何异常仅记日志、绝不阻断代理流量。
- **零明文落库**: 普通 Risk 字段不含完整 Secret, 仅 SHA-256 指纹 + 脱敏值。

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

`trafficguard.MarkAndSaveRisksForFlow(db, flow, req, rsp)` 在 MITM 镜像保存 flow 时调用
(v1 `grpc_mitm.go` / v2 `grpc_mitm_v2.go` 的 `InsertHTTPFlowEx` 之前)。

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
