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
└── legacydrivers/           # 旧驱动实现（仅供差分测试，主程序不引用）
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

### 依赖收益（相对 origin/main，yakgrpc 依赖闭包）

- 默认构建：**-61 包**（移除 go-mssqldb、mongo-driver、go-pg 闭包；
  外部模块含 golang-sql、vmihailenco/msgpack、montanaflynn/stats、tmthrgd、youmark/pkcs8 等）
- yakslim 构建：再 -13 包（go-ora 闭包）
- 二进制（含 yakgrpc 的空 main，darwin/arm64）：
  main 基线 290,461,010B → 默认 285,107,570B（**-5.1 MiB**）→ slim 278,332,290B（**-11.6 MiB**）
- `core` 包依赖图 0 驱动（`TestNoDriverImports` 强制）。

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

## 差分验证（删除旧驱动的前置条件）

`legacydrivers/differential_test.go`（`YAK_BRUTE_REAL=1` 启用）在真实
服务上对照旧驱动与新探针：

| 服务 | 环境 | 结果 |
|---|---|---|
| MySQL 8.0 / 5.7 / MariaDB 11 | caching_sha2 / native | 正确凭证、错误密码、未知用户、空密码全部一致 |
| PostgreSQL 16 / 12 | SCRAM-SHA-256 / MD5 | 全部一致 |
| MongoDB 7 / 4.4 | SCRAM-SHA-256 | 全部一致 |
| SQL Server 2022 | TDS 内嵌 TLS | 模拟器全通过；真实服务器验证依赖 amd64 容器（ARM 主机模拟受限） |

## 测试

```bash
# 单元 + 模拟器（全部协议的怪异场景：错误 banner、超时、断连、Unicode 凭证、ctx 取消、锁定、限流）
go test -race ./common/brute/... ./common/utils/bruteutils/

# 真实服务器（先启动上文差分表中的容器）
YAK_BRUTE_REAL=1 go test ./common/brute/probes/... ./common/utils/bruteutils/legacydrivers/

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
- 旧驱动实现保留在 `legacydrivers`（仅测试引用），差分验证通过一个
  发布周期后随 go.mod 依赖一并删除。
