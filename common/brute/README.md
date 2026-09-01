# brute：爆破调度核心与最小认证探针

本目录实现了爆破体系的重构：调度核心与协议实现彻底解耦，
认证探测使用最小协议实现替代通用数据库驱动。

## 目录结构

```
common/brute/
├── core/                    # 调度核心（禁止导入任何驱动/协议包）
│   ├── types.go             # Target/Credential/Result/Outcome 枚举/TLSPolicy/Prober
│   ├── registry.go          # 分协议注册表（替代全局静态 authFunc 表）
│   ├── scheduler.go         # 流式有界调度器
│   ├── iterate.go           # 惰性笛卡尔积组合源（O(1) 内存）
│   ├── throttle.go          # 令牌桶限速 + 抖动 + chanGate 并发闸
│   └── dial.go              # 策略化拨号 + WatchConn（取消即中断 I/O）
├── probes/
│   ├── mysql/               # 最小 MySQL 探针（native + caching_sha2 快/全量 + RSA + TLS + AuthSwitch）
│   ├── postgres/            # 最小 PG 探针（cleartext + MD5 + SCRAM-SHA-256 + SSLRequest TLS）
│   ├── mongodb/             # 最小 Mongo 探针（OP_MSG + 最小 BSON + SCRAM-SHA-1/256）
│   ├── mssql/               # 最小 TDS 探针（PRELOGIN + LOGIN7 + TDS 内嵌 TLS）
│   └── internal/scram/      # 共享 SCRAM(RFC5802) 客户端（stdlib PBKDF2 + x/text SASLprep）
└── dicts/                   # 共享默认字典

common/utils/bruteutils/     # 兼容层：旧 Yak API 不变，内部切换到新核心
                             # （旧数据库驱动已删除，见下文"依赖收缩"）
```

## 调度器（core.Scheduler）

- **流式有界**：组合经惰性迭代器产出，进入容量受限的有界队列；
  内存复杂度 `O(队列容量 + Worker 数 + 活跃目标状态)`，
  不再随 T×U×P 增长（10 万组合实测堆增长 2.6KB）。
- **并发控制**：全局并发、单目标（服务）并发两级。
- **限速**：全局限速（每秒尝试数）+ 随机抖动；兼容旧 delayer 语义。
- **账户锁定预算**：单目标 AccountLocked 达到阈值即短路该目标。
- **Retry-After 退避**：RateLimited 结果携带的退避在目标状态机内生效。
- **取消**：ctx 取消立即中断阻塞的 I/O（WatchConn），Worker 与连接
  在网络关闭期限内退出（实测亚毫秒级）。
- **无目标级 Timer Goroutine**：退避等待内联在 Worker 内。

## 结构化结果

`core.Result` 用枚举替代布尔组合与错误字符串判断：

`AuthSuccess / AuthFailed / TargetUnavailable / ProtocolMismatch /
AccountLocked / RateLimited / MFARequired / TLSRequired /
UnsafeDowngradeBlocked / Cancelled / Unknown`

结果记录协议、目标、凭证摘要（不含密码原文）、实际传输方式、
错误类别、Retry-After 与耗时。

## TLS 策略

按模块约定默认跳过证书校验（弱口令探测场景），但传输策略显式化：

| 策略 | 行为 |
|---|---|
| `TLSOpportunistic`（默认） | 尝试 TLS，失败回退明文并**如实记录传输方式**（不再静默降级） |
| `TLSStrict` | TLS 失败即终止，绝不发送明文凭证（结果 TLSRequired） |
| `PlaintextAllowed` | 直接明文 |

各协议使用自己的协议级 TLS 协商（MySQL SSLRequest、PG SSLRequest、
TDS PRELOGIN ENCRYPTION、Mongo TCP 层），而非旧的"TCP 层盲试 TLS"。

## 安全治理

- `BruteItem.String()` / `BruteItemResult.String()` 默认脱敏（密码只留 SHA256 摘要）。
- SSH/Telnet/MySQL/SASL/SNMP/fuzztag 等路径的明文密码日志已清除或脱敏。
- Panic 栈只进日志，绝不进入结果对象与远程事件。
- 哨兵密码扫描测试（全部内置协议 × 结果/错误/日志）零泄漏。
- 调度器对目标数、组合产出有界（有界队列 + 惰性源）。

## 构建标签

| 构建 | 说明 |
|---|---|
| 默认 | 全部协议；MySQL/MSSQL/MongoDB/PostgreSQL 使用最小探针；Oracle 暂保留 go-ora |
| `yakslim` | 额外排除 go-ora（Oracle 不可用），验证爆破闭包无数据库驱动 |

### 依赖收缩（最终状态，legacydrivers 已删除）

旧数据库驱动（go-mssqldb / go-pg / mongo-driver）及其闭包已从
go.mod 移除（相对 origin/main：**-3 直接依赖模块 + 13 indirect 模块**，
含 golang-sql、vmihailenco/msgpack、montanaflynn/stats、tmthrgd、
youmark/pkcs8、mellium.im/sasl 等）。go-sql-driver/mysql 仍被
common/consts 的 gorm 存储层引用，与爆破无关。

包级依赖闭环（`go list -deps` 实测，数据库驱动 grep 为 0）：

| 包 | 外部直接依赖 |
|---|---|
| `brute/core` | **无任何外部依赖**（纯标准库） |
| `brute/probes/mysql` | 无（原生 handshake + SHA1/SHA256/RSA stdlib） |
| `brute/probes/postgres` | 无（MD5/SCRAM-SHA-256 stdlib） |
| `brute/probes/mongodb` | 无（最小 BSON + SCRAM 自实现） |
| `brute/probes/mssql` | 无（TDS + 内嵌 TLS stdlib） |
| `brute/probes/...` 全部 | 仅 `golang.org/x/text/secure/precis`（SCRAM SASLprep） |
| `bruteutils` 兼容层 | 每库单一协议归属：`x/crypto/ssh`(SSH)、`jlaffaye/ftp`(FTP)、`go-ldap/ldap`(LDAP)、`gosnmp`(SNMP)、`mitchellh/go-vnc`(VNC)、`xdg-go/scram+stringprep`(SASL 邮件认证)、`sijms/go-ora`(Oracle，见下) |

RDP（grdp，仓库自带实现）、Redis、Telnet、RTSP、Memcached、
HTTP/SOCKS 代理、PPTP、Tomcat 均为仓库内最小自研实现，零外部依赖。
工具性依赖（go-spew、pkg/errors）已全部替换为标准库。

### Oracle 符号归因

任务验收要求默认实现消除 ≥6.6 MiB 数据库爆破可归因代码。当前默认构建
消除 5.1 MiB，其余为 go-ora 闭包（slim 差值 ≈6.5 MiB）。保留原因：
O5LOGON/PBKDF2 verifier 涉及多版本密码学交互，任务明确要求
"对无法安全实现的复杂认证机制，宁可保留窄范围、经过审计的依赖"，
在多版本 Oracle 真实环境完成最小探针兼容验证前不替换。

## 旧实现的缺陷（差分测试发现，均已在最小探针修复）

1. **MySQL PathEscape 密码破坏**：旧 `MYSQLAuth` 用 `url.PathEscape`
   编码密码拼 DSN，`!`→`%21` 等转义使含特殊字符的正确密码全部被误判
   为 Access denied（弱口令漏报）。实测 mysql8/5.7/mariadb 三环境复现。
2. **TLS-first dialer 污染连接**：旧全局 dialer 先向 MySQL 端口发 TLS
   ClientHello 再回退，驱动经此路径即使密码正确也认证失败。
3. **MongoDB 未授权误报**：旧 UnAuthVerify 用 Ping 判定，而 ping 命令
   在开启认证的 MongoDB 上也允许匿名执行——所有可达实例都被报为未授权。
   新实现改用需要权限的 listDatabases。
4. **既有数据竞态**：`DelayWaiter.nextDelay` 并发读写、lowhttp `dnsEnd`
   异步回调竞态（均已修复，`go test -race` 全绿）。

## 版本兼容矩阵（真实服务器，YAK_BRUTE_REAL=1 + 可达性自动跳过）

| 协议 | 已验证版本 |
|---|---|
| MySQL | 8.0 / 8.4 / 5.7 / MariaDB 11 / MariaDB 10.11 |
| PostgreSQL | 12 / 13 / 14 / 15 / 16(TLS+SCRAM) / 17 |
| MongoDB | 4.4 / 5.0 / 6.0 / 7.0 |
| SQL Server | 2019 / 2022（Rosetta amd64 模拟） |

每版本 7 个正反案例（正确/错误密码、未知用户、空密码、Unicode、
256 字符长密码、传输方式记录）+ MySQL 边界 8 例
（过期密码→AuthSuccess、账户锁定 3118→AuthFailed、Unicode 凭证正反、
200 字符密码正反）。

## 探针运行状况总表

四个验证维度：**Mock**（本地模拟器正反向测试，`go test` 默认执行；
数据库探针见 `common/brute/probes/*`，其余见 `bruteutils/mock_servers_test.go`）、
**真实容器**（`YAK_BRUTE_REAL=1`，版本见兼容矩阵）、**互联网实测**
（Shodan 采样，见下文直方图）、**依赖**（外部库，全部单一协议归属）。

| 协议 | 实现/依赖 | Mock 正反向 | 真实容器 | 互联网实测 |
|---|---|---|---|---|
| MySQL | 最小探针（零外部依赖） | ✅ 怪异 banner/超时/断连/Unicode/ctx 取消/锁定/限流 | ✅ 5 版本+8 边界 | ✅ 3+2 |
| PostgreSQL | 最小探针（零外部依赖） | ✅ | ✅ 6 版本（含 TLS） | ✅ 3+2 |
| MongoDB | 最小探针（零外部依赖） | ✅ | ✅ 4 版本 | ✅ 2+3 |
| SQL Server | 最小探针（零外部依赖） | ✅ | ✅ 2019/2022 | ✅ 2+3 |
| Oracle | go-ora（窄用法） | — | 差分通过（删除前） | 未采样 |
| RDP | 自研 grdp（零外部依赖） | ✅ CredSSP 模拟器 6 用例含 Unicode 正反 | ✅ xrdp×2 | ✅ 1 eof+4 tls |
| FTP | jlaffaye/ftp | ✅ 230/530 正反+未知用户 | — | ✅ 5/5 |
| SMTP | 自研（SASL: PLAIN/LOGIN/CRAM/SCRAM） | ✅ AUTH LOGIN 全流程正反 | — | — |
| Telnet | 自研（流式提示符匹配） | ✅ 登录流正反+未知用户 | — | ✅ 5/5 |
| Redis | 自研（RESP） | ✅ AUTH/SET-GET 回环+无密码模式 | — | ✅ 5/5 |
| LDAP | go-ldap | ✅ BER bindResponse 0/49 正反 | — | — |
| VNC | mitchellh/go-vnc | ✅ RFB3.8 VNC-Auth DES 挑战正反 | — | ✅ 5/5 |
| SNMPv2 | gosnmp | ✅ UDP community 正反（错误静默丢弃） | — | — |
| SNMPv3 | gosnmp | ✅（sasl 单测 + 差分） | — | — |
| Memcached | 自研 | ✅ stats 未授权路径 | — | ✅ 5/5 |
| SSH | x/crypto/ssh | ✅（ssh_test） | — | ✅ 5/5 |
| SMB/POP3/IMAP/RTSP/PPTP/Tomcat/HTTP-SOCKS 代理 | 自研 | ✅（哨兵全协议扫描 + 不可达分类） | — | ✅（SMB 5/5） |

不可达目标（127.0.0.1:1）全协议无 panic 分类测试：TestMockTargetUnavailable。
判定语义：RDP-NLA/全部数据库协议有协议级成败信号；xrdp SSL 模式
（无 NLA）为已知误报限制（无失败信号，见 RDP 章节）。

## RDP（grdp）修复与真实容器验证

xrdp 真实容器（0.9.17 / 0.9.24，`testdata/rdp/Dockerfile.nla`）测试
发现并修复五处致瘫缺陷——修复前 RDP 探测对 xrdp 部署 100% 失败：

1. Info PDU 缺 `INFO_MAXIMIZESHELL(0x20)`：xrdp 按 `RDP_LOGON_NORMAL(0x33)`
   校验即断连（"wrong flags"）。
2. `sendClientNewLicenseRequest` 解包目标误写 `&req`（应为 ServerCertificate），
   且误发 RSA 模数而非序列化后的 license request → 客户端必 15s 超时。
3. GCC `ServerCoreData` 用 struc 固定解 3 字段，xrdp 只发 8 字节（可选
   EarlyCapabilityFlags）→ EOF 中断整个响应解析，SC_SECURITY 丢失。
4. `readCapability` 对空 capability（len=4）返回 `(nil,nil)`，nil 接口
   方法调用 panic。
5. TPKT/`StartReadBytes` 不校验对端可控长度：RDP 探测打到非 RDP 端口
   （如 MySQL banner）`makeslice` panic。

其他：Context 贯通（取消亚毫秒级中断连接）、netx 重试错误保留底层
cause（恢复 connection refused 分类）、NTLM 明文密码日志清除
（Shodan 真实目标测试捕获）、Login 事件竞态治理（doneOnce）。

**NLA/CredSSP 判定**：xrdp 主线不支持服务端 NLA（源码注明 "We don't
yet support CredSSP"），SSL 模式下认证失败只显示图形登录框（无协议级
信号、不断连）。因此 `rdp_nla_test.go` 实现最小 CredSSP 模拟服务器，
服务端按 [MS-NLMP] 验证 NTProofStr：6 用例（正确/错误密码、未知用户、
Unicode 正反、空密码）全通过——NTLMv2 客户端实现首次获得确定性验证。
xrdp SSL 模式的误报限制已在 `rdp_real_test.go` 注明；互联网主流的
Windows NLA 目标由 CredSSP 阶段给出确定成败。

## Telnet 真实设备语料与判定增强（Shodan 100 样本）

互联网 telnet 协议形态高度不标准。Shodan 采样 100 例 banner 分析：
49% 标准 login 提示、13% 仅 user、1% 仅 password、**37% 无标准提示符**——
其中 ~14% 为爆破锁定提示（华为系 `Protection of brute force attack!!
Lockout remaining: TELNET[ppp0] N seconds`，多带 IAC 协商字节前缀）、
~4% 连接资源受限（`connections exceed 5` / `maximum number of telnet
sessions` / `no more connections`）、其余为 banner 后等待按键才出提示
（`Telnet Server 2.00` 类）。

判定增强（`telnet_corpus_test.go` 用真实 banner 回放回归）：
- **锁定识别**：`AccountLocked` 信号贯通 `coreResultFromLegacy` →
  `OutcomeAccountLocked`，调度器按 LockoutBudget 短路目标，不再撞击
  延长锁定期（30 目标实测 4 例正确识别，含 IAC 前缀原始流）；
- **资源受限/禁用**：标记 Finished（TargetUnavailable），~1s 内退出；
- **无提示符重触发**：banner 无提示词时先敲一次回车再读（覆盖
  "等待按键"类设备）；
- **读取双出口**：匹配词即返回；数据读完 600ms 静默或 1.2s 无任何
  字节（哑连接）即返回——单凭证实测最慢从 30.4s 降至 6.1s。

实测（30 个互联网目标）：4 account-locked / 4 unavailable / 22
auth-rejected，0 意外成功、0 panic。已知限制：IAC 协商字节未应答
（正则判定容忍二进制前缀，不影响分类）。

## 互联网真实目标分类实测（Shodan）

`shodan_real_test.go`（`YAK_BRUTE_SHODAN=1 + SHODAN_API_KEY`）：每协议
≤6 目标、每目标 1 次无效凭证尝试、尝试间隔 1s，输出错误分类直方图。

2026-09 实测（12 协议 × 5 目标 = 60 次）：**0 次意外成功、0 次 panic**。

| 协议 | 分类分布 |
|---|---|
| ftp / ssh / redis / telnet / memcached / smb / vnc | 5/5 auth-rejected |
| mysql / postgres | 3 auth-rejected + 2 target-unreachable |
| mongodb / mssql | 2 auth-rejected + 3 target-unreachable |
| rdp | 1 eof + 4 tls（真实 NLA 目标 TLS 拒绝正确分类，3s 内退出） |

## 稳定性

- 探针模拟器套件 `-count=5`（5 轮重复）全绿；`-race -count=3` 全绿。
- 真实 mysql8 大字典（5000 密码）中途取消：1.5s 完整退出（= 取消信号延迟），
  goroutine 净增 0。
- `TestGRPCMUSTPASS_Brute`（grpc→yak 脚本→调度器全链路）连过 5+ 次。
- 已知无关项：yakvm 日志引擎存在既有数据竞态（main 分支同样复现 6 处），
  非本 PR 引入；CI 对该包不启用 -race。

## 兼容层修复的调度缺陷（本地全链路测试发现）

1. delayer 限速换算把 time.Duration（纳秒）当秒 → 令牌间隔 158 年，
   第二个目标等到 ctx 超时（多目标场景全部饿死）。
2. 调度器 allStopped 在"后续目标状态机未创建"时误判全部完成 →
   OkToStop 命中即取消整个运行。
3. 旧 defaultDialer TLS-first 污染明文服务（见上文"旧实现的缺陷"）。

## 差分验证结论（旧驱动删除依据，实现已随依赖一并移除）

删除前在真实服务器上完成新旧实现全量对照（每服务：正确凭证、错误
密码、未知用户、空密码全部一致）：

| 服务 | 环境 | 结果 |
|---|---|---|
| MySQL 8.0 / 5.7 / MariaDB 11 | caching_sha2 / native | 全部一致 |
| PostgreSQL 16 / 12 | SCRAM-SHA-256 / MD5 | 全部一致 |
| MongoDB 7 / 4.4 | SCRAM-SHA-256 / SHA-1 | 全部一致 |
| SQL Server 2022 / 2019 | TDS 内嵌 TLS | 全部一致 |

差分测试代码（legacydrivers）已随旧驱动删除；结论由上表与版本矩阵
（YAK_BRUTE_REAL=1 可重跑）持续背书。

## 测试

```bash
# 单元 + 模拟器（全部协议的怪异场景：错误 banner、超时、断连、Unicode 凭证、ctx 取消、锁定、限流）
go test -race ./common/brute/... ./common/utils/bruteutils/

# 真实服务器（先启动上文版本矩阵中的容器）
YAK_BRUTE_REAL=1 go test ./common/brute/probes/... ./common/utils/bruteutils/

# Fuzz（解析器：MySQL greeting / BSON / PG ErrorResponse / TDS PRELOGIN+token）
go test -fuzz=FuzzMySQLGreeting -fuzztime=60s ./common/brute/probes/
go test -fuzz=FuzzBSONDecode   -fuzztime=60s ./common/brute/probes/
go test -fuzz=FuzzPGErrorResponse -fuzztime=60s ./common/brute/probes/
go test -fuzz=FuzzMSSQLPrelogin   -fuzztime=60s ./common/brute/probes/
go test -fuzz=FuzzMSSQLTokens     -fuzztime=60s ./common/brute/probes/

# 基准（10 次采样 + benchstat）
LOG_LEVEL=error go test -bench=Scheduler -benchmem -count=10 -run=NONE \
  ./common/utils/bruteutils/ | benchstat -

# 依赖边界
go list -deps ./common/brute/core          # 无任何驱动
go list -tags yakslim -deps ./common/yakgrpc
```

基准说明（10 样本中位数，1 万组合/次）：新调度器 32.1ms/万组合
（3.2µs/组合，约 31 万组合/秒调度能力）、7.4MiB/op 有界队列开销；
旧 Feed 路径 8.6ms/op、3.3MiB/op——但旧路径内存随组合数线性增长
（10 万组合需物化全部任务），新调度器恒定（实测堆增长 2.6KB）。
真实爆破受网络与限速约束（典型 ≤1000 次/秒），调度开销不构成瓶颈；
单组合开销换取有界内存、取消传播、限速与锁定预算。

## 兼容性

- `bruteutils` 对外 API（`GetBruteFuncByType` / `StreamBruteContext` /
  `NewMultiTargetBruteUtilEx` / `MYSQLAuth` / `MSSQLAuth` / `MongoDBAuth`
  / `AuthFunctionMap` 等）签名不变。
- `StreamBruteContext` 内部已切换到流式调度器，语义兼容
  （OkToStop / FinishingThreshold / UserEliminated / OnlyNeedPassword /
  delayer / beforeBruteCallback）。
- 旧数据库驱动与差分测试代码已删除（差分结论见上节）；go.mod 同步
  移除 go-mssqldb / go-pg / mongo-driver 及其 indirect 闭包。
