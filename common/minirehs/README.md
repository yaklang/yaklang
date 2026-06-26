# minirehs

`minirehs` 是一个**零外部依赖、可移植**的多正则批量匹配引擎, 借鉴 Intel Hyperscan 的
"统一编译, 一次扫描" (compile then scan) 模型: 把成百上千条正则统一编译成一个不可变
`Database`, 对输入只做一次字面量预过滤即可定位候选, 再只对候选做正则验证, 从而避免
"几百条正则逐条全量匹配" 的 `O(N_patterns x N_bytes)` 开销。

名字取 "mini Regex Hyperscan"。它**不是** Hyperscan 的移植, 而是用纯 Go (+ 可选自带
SIMD) 实现的轻量近似。

## 在 yak 语言里使用 (rehs 库)

本模块对外以 `rehs` 库注册进 yaklang, 一行编译、一次扫描即可批量判定哪些正则命中:

```yak
// 统一编译一组正则 (失败用 ~ 抛错)
group = rehs.BuildGroup(["admin", "(?i)password", "token=\\w+"])~

// 存在性: 是否有任意一条命中 (命中即停, 最快)
if group.Match(data) { ... }

// 取全部命中 (含正则下标/表达式/字节偏移/命中内容)
for m in group.Find(data) {
    println(m.Index, m.Pattern, m.From, m.To, m.Value)
}

// 只要"哪些规则命中"
pats = group.MatchedPatterns(data)   // []string, 去重
idxs = group.MatchedIndexes(data)    // []int,   去重

// 流式回调 (返回 false 提前终止)
group.Scan(data, func(m) { println(m.Pattern); return true })

group.Close()
```

接口一览:

- `rehs.BuildGroup(patterns, opts...) -> (group, err)` — 编译一组正则; `patterns` 接受字符串列表。
- `rehs.MatchAny(patterns, data) -> (bool, err)` — 一次性判定是否命中任意一条。
- 选项 (yak 风格): `rehs.caseInsensitive()`、`rehs.dotAll()`、`rehs.multiline()`、
  `rehs.existenceOnly()` (只判存在性、不取偏移, 最快)、`rehs.minLiteralLen(n)`、`rehs.backend("mvs"|"engine"|"stdlib")`。
- `group` 方法: `Match(data)`、`MatchString(s)`、`MatchBytes(b)`、`Find(data)`、
  `MatchedIndexes(data)`、`MatchedPatterns(data)`、`Count(data)`、`Scan(data, cb)`、
  `Patterns()`、`Len()`、`Info()`、`Close()`。

**默认 CGO 最强, 按系统逐步退化**: `BuildGroup` 默认走自托管 `mvscan` 后端 ——
启用 CGO (`CGO_ENABLED=1`) 时自动编入纯 C99 位并行内核 (最强档, 编译日志 `c_kernel=true`);
无 CGO 时优雅退化为纯 Go 参考执行器, 全平台可移植、结果完全一致。无需任何额外 build tag。

## 设计原则

1. **可移植优先 + CGO 默认最强**: 不加任何 build tag 即可 —— 启用 CGO 时默认编入
   `mvscan` 的纯 C99 位并行内核 (最强档); `CGO_ENABLED=0` 时在所有平台/架构上编译运行且
   功能完整, 退化为纯 Go 自研引擎。两档结果逐字节一致。
2. **零外部依赖 / 不加载任何动态库**: 不引入新的第三方库。CGO SIMD 是自带的纯 C99 内核
   (`native/teddy.c` / `native/mvscan/`), **不链接、不 dlopen libhs 等任何外部库**。
   (历史上的 vectorscan/libhs 对照桥已于 2026-06 整体移除, 见 `MINI_VECTOR_SCAN_IMPL.md` 第 0' 节。)
3. **优雅退化**: CGO/SIMD 不可用时自动退化为纯 Go, 结果完全一致。
4. **结果一致**: 任何后端 (纯 Go / CGO-SIMD / stdlib) 对同一条 RE2 正则的命中集合
   **逐字节相同** (差分测试保证), 因为预过滤只决定"验证哪些位置", 验证逻辑共享。
5. **复用 yaklang 生态**: 特性 gate 与 regexp2 兜底直接复用 `common/utils/regexp-utils`。

## 后端分档 (Tier)

| Tier | 后端 | 技术 | 语义 | 触发条件 |
|------|------|------|------|----------|
| 1 | mvs | 自托管 mvscan: rune/字节级 Glushkov 位并行 NFA + 字面量预过滤 (CGO 时纯 C99 内核) | **存在性** (From/To=-1) 或精确偏移 | `WithBackend(BackendMVS)`; `BuildGroup` 默认选用 |
| 2 | engine | 自带 Teddy SIMD (NEON / SSSE3) 多字面量预过滤 | RE2 精确偏移 | `CGO_ENABLED=1` (默认, 无需额外 tag) |
| 3 | engine | 纯 Go 标量 Aho-Corasick | RE2 精确偏移 | 默认 (任意平台) |
| 4 | stdlib | 无 (逐条全量匹配) | RE2 精确偏移 | 显式 `WithBackend(BackendStdlib)`, 作 oracle / 基线 |

引擎 (Tier 2/3) 核心流程: **字面量提取 -> Aho-Corasick 预过滤 (一次扫描, 含位置) -> 邻域窗口验证**。
对"有界宽度且无位置锚点"的正则, 只在字面量命中点附近的小窗口内跑 RE2 验证 (邻域锚定),
显著降低验证成本; 对无界/含锚点的正则退化为整段验证; 无必需字面量的正则归入 always-on。

**`rehs.BuildGroup` 默认选用 mvs (Tier 1)**: 启用 CGO 时下沉纯 C99 位并行内核 (最强档),
无 CGO 时优雅退化为纯 Go 参考执行器, 全平台可移植、结果逐字节一致, **全程零外部依赖、不加载任何动态库**。

### mvs 自托管后端 (Tier 1, 默认)

把成百上千条正则**一次性**编译为各自的 rune/字节级 Glushkov 位并行 bit-NFA, 对每条数据做一次
字面量预过滤定位候选, 再只对候选 pattern 跑 bit-NFA 验证, 一趟即得"哪些规则命中"。这是
成熟系统 (Suricata/Hyperscan 等) 处理海量规则的标准思路, 对 MITM 打标这类**以命中存在性为准**
的场景 (yaklang MITM replacer 第一步即逐规则 `MatchString`) 尤其契合。

- **自托管、不加载任何动态库**: 运行期内核为自带的纯 C99 (`native/mvscan/`), **既不链接也不
  dlopen** libhs/vectorscan 或任何系统库; 这是本引擎"可移植、易分发、分发不崩溃"的核心定位。
- **CGO 默认最强, 按系统退化**: `CGO_ENABLED=1` 时自动编入纯 C99 位并行内核 (最强档);
  无 CGO / 未带 tag 时退化为纯 Go 参考执行器, 结果逐字节一致。
- **双语义**: 默认可由 NFA 自身给出精确字节偏移 (leftmost-longest); 只需"是否命中"时显式
  `WithReportLocation(false)` 走纯位运算存在性快路径 (命中以 `From/To=-1` 上报)。
- **regexp2-only 规则 (backref/lookaround) 由 fallback 承载**: 这类非正则构造 (数学边界, 任何
  NFA 都无法表达) 自动转交 regexp2 verifier 判定存在性, 不丢规则; 并尽量用超集 NFA 作字面量门
  减少其触发频率 (见下文 Phase 1)。
- **一致性已验证**: mvs 逐记录命中的**规则 ID 集合 / 精确偏移**与 stdlib RE2 oracle **完全相同**
  (合成随机 + 真实 MITM 全量差分测试 + 对抗性窗口健全性测试)。

## 与 yaklang regexp-utils 的结合

特性 gate 使用 `regexp_utils.NewYakRegexpUtils(expr).CanUse()`: 优先标准库 RE2,
失败时回退 `regexp2` (支持 lookahead/lookbehind/backref)。因此:

- RE2 可表达的正则: 提取字面量, 走预过滤 + 邻域验证 (快路径)。
- RE2 不可表达但 regexp2 可编译 (如负向先行 `(?!...)`、possessive): 归入 always-on,
  用 regexp2 验证, 命中以"存在性"上报 (偏移 -1), 不会被丢弃。
- 两者都无法编译 (例如括号不匹配的非法正则): 按 `UnsupportedPolicy` 处理 (默认 Reject)。

## 用法

```go
import "github.com/yaklang/yaklang/common/minirehs"

db, err := minirehs.Compile([]minirehs.Pattern{
    {ID: 1, Expr: `password\s*=\s*\S+`},
    {ID: 2, Expr: `AKIA[0-9A-Z]{16}`},
    {ID: 3, Expr: `Druid`, Flags: minirehs.FlagCaseless},
})
if err != nil {
    // handle
}
defer db.Close()

sc, _ := db.NewScratch() // 每个 goroutine 独占一份 scratch
defer sc.Close()

db.Scan(data, sc, func(m minirehs.Match) bool {
    // m.ID / m.From / m.To
    return true // 返回 false 提前终止
})
```

`Database` 编译后不可变、并发安全 (只读); `Scratch` 非并发安全, 每 goroutine 一份。

启用 mvs 存在性快路径 (MITM 打标等只需"是否命中"的场景):

```go
// CGO 时自动下沉纯 C99 内核, 无 CGO 退化纯 Go; 全程不加载任何动态库, Compile 绝不因环境缺失报错。
db, _ := minirehs.Compile(patterns,
    minirehs.WithBackend(minirehs.BackendMVS),
    minirehs.WithReportLocation(false)) // 只判存在性, 最快
// 命中以存在性上报 (m.From == m.To == -1), 表示"该规则在本数据中命中"。
// 需要精确 [From,To) 时去掉 WithReportLocation(false), 由 NFA 给出 leftmost-longest 偏移。
```

## 测试与基准复现

测试数据在 `testdata/`:

- `rules.json`: rule4yak 的 89 条 MITM replacer 规则 (来源 SexyBeast233/rule4yak)。
- `traffic_corpus.bin`: 从本地 yaklang 项目库 (`default-yakit.db`) 抽取的约 5MB 真实
  HTTP 流量 (1332 条报文), 用 `testdata/gen_corpus.go` 生成 (`//go:build ignore`)。

重新生成语料 (需 CGO, go-sqlite3 是 cgo 驱动):

```bash
CGO_ENABLED=1 go run testdata/gen_corpus.go -db ~/yakit-projects/default-yakit.db -max 5242880
```

差分一致性测试 (引擎 vs stdlib oracle, 逐字节一致):

```bash
go test ./common/minirehs/ -run TestConsistency            # 完整 (慢, oracle 逐条匹配)
go test ./common/minirehs/ -run TestConsistency -short     # 快速 (前 200 条)
CGO_ENABLED=1 go test -tags minirehs_cgo ./common/minirehs/ -run TestConsistency -short
```

模糊差分安全网 (随机生成大量 RE2 正则 + 随机语料, 引擎与 oracle 必须逐字节一致):

```bash
go test ./common/minirehs/ -run TestFuzz                   # 60 轮 x 20 语料 x ~20 正则
```

`fuzz_test.go` 内置一个 RE2 正则生成器 (字面量/字符类/交替/分组/量词/锚点/各种 flag 全覆盖),
以固定 seed 复现; 任何预过滤/窗口/去重引入的偏差都会在此暴露。它是所有性能优化的正确性兜底。

mvs 后端的差分一致性 (逐记录命中**规则 ID 集合 / 偏移**与 stdlib oracle 完全相同) + C==Go 逐位 + 窗口健全性:

```bash
go test ./common/minirehs/ -run 'TestMVS' -v                                  # 纯 Go MVS 全套
CGO_ENABLED=1 go test -tags minirehs_mvs ./common/minirehs/ -run 'TestMVS' -v  # C 内核 (amalgamation)
```

`mvs_*_test.go` 覆盖: 合成随机正则差分、真实 MITM 全量差分、regexp2 fallback (超集门)、
C 内核与 Go 参考执行器逐位一致、对抗性字面量窗口健全性 (`mvs_window_test.go`, 防本地化假阴)、
并发竞争 (`-race`)、提前终止。

### 测试覆盖率

```bash
go test ./common/minirehs/ -coverprofile=cov.out                              # 默认构建
CGO_ENABLED=1 go test -tags minirehs_cgo ./common/minirehs/ -coverprofile=cov_cgo.out
CGO_ENABLED=1 go test -tags minirehs_mvs ./common/minirehs/ -coverprofile=cov_mvs.out -timeout 600s
go tool cover -func=cov.out
```

- 默认 (NoCGO) 构建: **99.1%** 语句覆盖。
- CGO SIMD 构建: **98.6%** 语句覆盖。
- CGO mvs (amalgamation) 构建: 覆盖纯 C99 内核存在性/定位、批量、窗口化与退化路径。
- 余下未覆盖的极少数语句均为**不可达防御分支**: 对永不出错的内部调用 (`backend.compile`、
  `NewScratch`、已被 `regexp.Compile` 验证过的 `syntax.Parse`) 的惯用错误处理, 以及
  原生内核的 OOM / 越界 / `SINGLEMATCH` 不可能溢出 等防御。保留它们是为稳健性, 不为凑覆盖率而删改。

基准:

```bash
# 合成规模 (N 条字面量正则, 展示 "扫一遍 vs 扫 N 遍")
go test ./common/minirehs/ -run '^$' -bench BenchmarkSyntheticScale -benchtime 5x
CGO_ENABLED=1 go test -tags minirehs_cgo ./common/minirehs/ -run '^$' -bench BenchmarkSyntheticScale -benchtime 5x

# 真实 MITM 规则 + 真实流量
go test ./common/minirehs/ -run '^$' -bench BenchmarkMITMRealTraffic -benchtime 2x -timeout 600s

# mvs 存在性 / 定位 / 纯 RE2 子集 (默认纯 Go; 加 -tags minirehs_mvs 走 C 内核)
go test ./common/minirehs/ -run '^$' -bench BenchmarkMVSExistence -benchtime 3x -timeout 600s
CGO_ENABLED=1 go test -tags minirehs_mvs ./common/minirehs/ \
    -run '^$' -bench BenchmarkMVSExistence -benchtime 3x -timeout 600s
```

## 基准结果 (darwin/arm64, Apple Silicon)

### 合成规模: 固定约 1MB 语料 (256 x 4KB 报文), N 条字面量正则

字面量丰富、命中稀疏 (典型扫描/指纹场景)。引擎吞吐**与 N 基本无关** (一次扫描),
而逐条匹配随 N 线性劣化:

| N | Engine Tier3 (纯Go) | Engine Tier2 (SIMD) | StdlibLoop | 加速比 (Tier3 / SIMD vs Stdlib) |
|------|--------|--------|--------|--------|
| 50 | 215 MB/s | 391 MB/s | 49.9 MB/s | 4.3x / 7.8x |
| 100 | 229 MB/s | 510 MB/s | 25.6 MB/s | 9.0x / 20x |
| 300 | 230 MB/s | 485 MB/s | 8.9 MB/s | **26x / 55x** |
| 500 | 222 MB/s | 509 MB/s | 5.1 MB/s | 44x / 97x |
| 1000 | 209 MB/s | 452 MB/s | 2.6 MB/s | 79x / 166x |

结论: 这正是 "比现有 300 正则一次匹配性能强得多" 的目标场景 —— N=300 时纯 Go 已快约
26 倍, 开 SIMD 约 55 倍; 自带 SIMD 预过滤相对纯 Go 再提速约 2 倍。

### 真实 MITM 规则集 + 真实流量 (rule4yak 84~87 条规则, ~5MB / 1332 条报文)

经**同一个 `Scan` API** 端到端对照 (含回调派发、scratch 管理), 逐条 block 扫描:

| 方案 | 吞吐 | 相对 StdlibLoop | 语义 |
|------|------|--------|------|
| StdlibLoop (现状: 逐条全量匹配) | 0.17 MB/s | 1.0x | 精确偏移 |
| minirehs Engine (纯 Go) | 0.41 MB/s | **2.4x** | 精确偏移 |
| minirehs **mvs · 定位档** (C 内核) | 1.75 MB/s | **~10x** | 精确偏移 |
| minirehs **mvs · 存在性档** (C 内核) | 2.44 MB/s | **~14x** | 存在性 |
| minirehs **mvs · 存在性 (纯 RE2 子集)** | 3.83 MB/s | **~22x** | 存在性 |

结论: 对这个**对抗性规则集**, 自托管 mvscan (零外部依赖、不加载任何动态库) 在存在性档达
**~14x (全集) / ~22x (纯 RE2 子集)**, 定位档 **~10x**, 均为经完整 `Scan` API、回调与 scratch
管理的端到端数字。环境无 CGO 时自动退化为纯 Go (结果逐字节一致), 绝不崩溃。

**为什么还没到 Hyperscan/vectorscan 那种量级 (诚实分析, 数据见 `MINI_VECTOR_SCAN_IMPL.md` 第 0' 节)**:
这是一个**对抗性规则集** —— 大量形如 `(...key|auth|user|pass...).*?` 的宽泛正则, 字面量短而常见,
且含无界 `.*?`/`[^'"]+?`。差距的真实来源有二: (1) 默认预过滤是标量 Aho-Corasick 而非 Teddy SIMD
(设计稿把 100x 主要押在 Teddy, 但它尚未接入默认热路径); (2) 命中字面量后做的是 per-pattern **整段**
NFA 验证, 而非 Hyperscan 的 Rose 式"仅在命中点小邻域验证"。这两点都是设计稿主动登记的"暂缓/丢弃"项,
其回收 (Teddy 默认化 + Rose-lite 链分解) 是继续逼近 vectorscan 吞吐的主路径。

## 性能剖析 (PROFILE) —— MVS 后端 (rune 级 Glushkov 位并行 NFA)

> `BackendMVS` 是在 Engine 之后落地的**自研 bit-NFA 后端**: 把每条 RE2 正则编译为 rune 级
> Glushkov 位并行 NFA (而非"逐条 RE2 验证"), 可由 NFA 自身给出精确字节偏移 (leftmost-longest),
> 也可只判存在性走纯位运算快路径; 存在性热路径还可经 `-tags minirehs_mvs` 下沉到纯 C99 内核。
> 本节用**真实规则集 + 真实流量**的实测数据回答: 效果如何、CPU/内存花在哪、能否更好。

### 复现命令

```bash
# 三方吞吐 (MVS / Engine / StdlibLoop), 默认构建 = 纯 Go MVS
go test ./common/minirehs/ -run='^$' -bench BenchmarkMVSFullRuleset -benchtime=3x -timeout 900s

# 存在性档 vs 定位档 vs StdlibLoop
go test ./common/minirehs/ -run='^$' -bench BenchmarkMVSExistence  -benchtime=3x -timeout 900s

# 同上, 但 MVS 存在性热路径走纯 C99 内核 (cgo)
CGO_ENABLED=1 go test -tags minirehs_mvs ./common/minirehs/ -run='^$' -bench BenchmarkMVSExistence -benchtime=4x

# CPU + 内存 profile (单独打 MVS 子基准, 再用 pprof 看热点)
go test ./common/minirehs/ -run='^$' -bench 'BenchmarkMVSFullRuleset/MVS$' -benchtime=6x \
    -cpuprofile=/tmp/mvs.cpu -memprofile=/tmp/mvs.mem
go tool pprof -top -nodecount=18 /tmp/mvs.cpu
go tool pprof -top -sample_index=alloc_space -nodecount=14 /tmp/mvs.mem
```

### 实测吞吐 (darwin/arm64, Apple Silicon; rule4yak 真实 MITM 规则 87 条 / 真实流量 ~5MB·1332 报文; 同一 `Scan` API)

| 后端 / 档位 | 吞吐 | vs StdlibLoop | vs Engine | allocs/op | 语义 |
|---|---|---|---|---|---|
| StdlibLoop (现状: 逐条全量) | 0.17 MB/s | 1.0x | — | ~30k | 精确偏移 |
| Engine (纯 Go, 字面量预过滤) | 0.36 MB/s | 2.1x | 1.0x | ~32k | 精确偏移 |
| **MVS 纯 Go · 定位档** | **1.13 MB/s** | **6.6x** | **3.1x** | ~96k | 精确偏移 |
| **MVS 纯 Go · 存在性档** | **1.38 MB/s** | **8.1x** | 3.8x | ~46k | 存在性 |
| MVS C 内核 · 定位档 | 1.17 MB/s | 6.9x | 3.3x | ~73k | 精确偏移 |
| **MVS C 内核 · 存在性档** | **1.41 MB/s** | **8.3x** | 3.9x | **~22k** | 存在性 |

要点 (用数据说话):

1. **MVS 把这个对抗集的纯 Go 吞吐从 Engine 的 2.1x 提到 6.6x (定位) / 8.1x (存在性)** —— 因为它用
   bit-NFA 一次位并行递推替代了"逐条 RE2 验证整段"。这是**全平台可移植、零原生依赖**下的数字。
2. **存在性档比定位档快约 +22% (1.13→1.38) 且 allocs 减半 (96k→46k)** —— 只需"哪些规则命中"的
   MITM 打标场景应显式 `WithReportLocation(false)`。
3. **C 内核的明确收益是分配, 不是吞吐**: 存在性档 allocs 46k→**22k (-51%)**、定位档 96k→73k (-24%);
   但吞吐几乎不变 (1.38→1.41 / 1.13→1.17)。原因见下方热点分析。

### CPU 花在哪 (pprof, 纯 Go 定位档, 采样总 ~30.4s)

> **注**: 下列 profile 为 regexp2 后端切换为 `go-pcre2-lite`（PCRE2）**之前**的基线快照，保留作历史参考。
> 切换后 `dlclark/regexp2.(*runner).execute` 与 `getRunes` 两项已被 PCRE2 线性引擎替代，瓶颈分布已改变；
> 新基线需重测（regexp2 兜底占比预计大幅下降，Rose-lite 双向锚定省下的扫描占比随之放大）。

```text
cum%   函数
35.8%  github.com/dlclark/regexp2.(*runner).execute      <- regexp2 兜底 (2~3 条 always-on URL 规则, 切换前)
22.2%  minirehs.(*mvsNFA).existsIn                        <- bit-NFA 存在性
13.7%  minirehs.(*mvsNFA).findLocFrom                     <- 定位 (leftmost-longest)
12.9%  minirehs.(*mvsNFA).existsInAssert                  <- 零宽断言 NFA
 8.9%  regexp2/syntax.CharSet.CharIn                      <- regexp2 兜底 (切换前)
 5.5%  unicode/utf8.DecodeRune
```

切到 C 内核后, `existsIn` (22%) 基本被 `runtime.cgocall` (~22%) 取代, **但 regexp2 (35%)、
findLocFrom (14%)、existsInAssert (11%) 原样不变**, 故总时间持平 —— C 只加速了存在性 NFA 这一段,
而它本就不是瓶颈; 且当前每 pattern 一次 cgo 调用, cgocall 跨界开销吃掉了省下的存在性 CPU。

### 内存花在哪 (pprof, alloc_space)

> **注**: 同为切换前基线；切换 PCRE2 后 `getRunes`（整段输入转 `[]rune`）这一分配大头预计消失。

```text
flat%  函数
57.4%  github.com/dlclark/regexp2.getRunes               <- 每次把整段输入转 []rune (每 pattern·每报文一次, 切换前)
15.2%  minirehs.(*mvsNFA).findLocFrom                     <- 定位时的起点追踪/结果切片
 ----  (Regexp2Wrapper.Match 累计 ~74%)
```

**~74% 的分配来自 regexp2 兜底** (`getRunes` 把输入复制为 rune 切片), **~15% 来自定位**。C 内核把
Go 侧 `existsIn` 的分配清零 (这就是 allocs 下降的来源), 但动不了 regexp2 与定位这两块。

### 能否更好? (数据驱动的下一步, 按收益排序)

1. **干掉 regexp2 always-on 税 (最大头: CPU ~35% / 内存 ~74%)** —— 这 2~3 条 regexp2-only URL 规则
   (含 lookahead/unicode 区间) 每条报文都全量跑。两条路:
   - **共享 rune 转换**: 一份 `[]rune(input)` 给所有 regexp2-only pattern 复用 (现在每 pattern 各转一次),
     可砍掉绝大部分 `getRunes` 分配;
   - **更强字面量门控**: 让它们落到 `http`/`://` 命中点后才验证 (route-B 已部分覆盖, 但仍 always-on,
     需排查为何未门控)。
2. **批量化 cgo 调用 (省下被 cgocall 吃掉的存在性 CPU)** —— 把"每 pattern 一次 `nfaExists`"改为
   "每报文一次 C 入口遍历全部 NFA + 合并自动机", cgocall 次数从 O(pattern·报文) 降到 O(报文)。
3. **定位的零分配化 (内存 ~15%) [已落地]** —— `findLocFrom` 的 4 个位并行状态切片
   (prevActive/cand/candStart/prevStart) 改用 scratch 复用缓冲 (写后读语义, 无需逐次清零);
   `sc` 经 `finalizeHit`/`findAllLoc` 透传. A/B (BenchmarkMVSFindAllLocScratch, 77 lean NFA·
   同语料): allocs/op **41242→189 (-99.5%)**、B/op **11.0MB→58KB (-99.5%)**, 吞吐持平 (定位耗时
   本就在递推而非分配). 真实流量定位差分 (MITM 77 规则) + 随机差分 (10241 例) 全绿, 无假阴假阳.
4. 对抗集要真正逼近 Vectorscan (单一 SIMD NFA), 仍需统一多模式验证引擎; 在 RE2/regexp2 逐条之上
   加预筛已被证明无效 (见"已验证为负向的优化")。

### Phase 1 已落地 (regexp2 税消除 + 窗口化存在性) —— 实测更新

> 上面"能否更好"第 1 条 (regexp2 税) 已实现并验证。做法是**用 route-B 的 `re2Superset` 把
> regexp2-only 规则改写为语言只增不减的 RE2 骨架, 编成 bit-NFA 作存在性门**:
> - 语言**等价**改写 (仅 `\uXXXX`→`\x{}` / 原子组归一): NFA 即权威, **彻底不跑 regexp2** (如 `Url信息`);
> - 语言**严格超集**改写 (移除 lookahead/backref): NFA 作"存在性门", **仅门命中时才 regexp2 复核**滤假阳
>   (如 `Email`/`参数-URL设计`), 把 regexp2 从"每条报文"降到"门命中的少数报文"。绝不漏报 (R_super ⊇ R_orig)。
>
> 结果: MITM 87 条里 **regexp2-only 兜底 3→0**, 仅余 2 条 gate 在命中时复核。另对 25 条有界宽 lean NFA
> 启用**邻域窗口存在性** (命中点 ±2·winW 内 `existsIn`, 把 per-trigger 从 O(record) 降到 O(winW))。

| 档位 (darwin/arm64, 同一对抗集) | Phase 0 | Phase 1 + 窗口化 | 变化 |
|---|---|---|---|
| MVS 纯 Go · 存在性 (全 87 条) | 1.41 MB/s · 75 MB/op | **1.95 MB/s · 21.8 MB/op** | **+38% 吞吐, -71% 内存** |
| MVS 纯 Go · 存在性 (纯 RE2 子集, NFA 天花板) | 2.81 MB/s | **3.04 MB/s** | +8% (窗口化) |
| MVS 纯 Go · 定位 (全 87 条) | 1.13 MB/s | 1.50 MB/s | +33% (regexp2 复核减少) |

正确性: 新增 `TestMVSExistenceVsOracleMITM_NoLoc` (存在性快门档 hit 集 == stdlib oracle, 覆盖窗口化/gate
复核分支), 连同既有 13 项差分/oracle 测试全过。

**仍未到 80x 的根因 (已用数据定位, 决定下一步)**:
1. **per-pattern cgo 开销**: C 内核存在性 (2.03/2.51 MB/s) ≈ 甚至略低于纯 Go (1.95/2.81) —— "每 pattern
   一次 `nfaExists`"的 cgocall 跨界成本吃光了 SIMD 收益。→ Phase 2: **按报文一次 cgo 入口**遍历全部 NFA。
2. **53 条无界 (`.*`) 模式仍整段 `existsIn`**: 窗口化只覆盖 25 条有界宽。`existsIn` 仍占纯 NFA 路径 ~58% CPU。
3. **naive 全并已实测否决**: 把 77 条 lean NFA 并成单一自动机 → npos=3766/nword=59, 每字节代价 =
   活跃位置×59 字, 吞吐反降到 **1.26 MB/s**。→ 故 80x 只能走 **Phase 3 LimEx**: 状态重排使后继多为
   i→i+1 (一次 SIMD shift 推进整条状态向量)、少量"异常边"特判, 把每字节代价降到 O(nword) 与活跃数无关。

### Phase 2 (批处理 cgo) + Phase 3 (LimEx) 的实测结论 —— 为什么这个对抗集到不了 80x

> 沿 "能否更好" 路线继续推进, 把第 2 条 (批量 cgo) 与"统一自动机/LimEx"都**实现并实测**了。结论用
> 数据说话: 对**这一对抗性规则集**(87 条, 含大 alternation / unicode 类 / lookahead), 存在性路径的
> 实际天花板约 **3.2 MB/s (~19x)**, 80x (13.6 MB/s) 在自研 MVS 下不可达。

1. **批处理 cgo 无效 → cgo 从来不是瓶颈**。把"每 pattern 一次 `nfaExists`"改为"每报文一次
   `nfaExistsMany`(C 内循环)"后:`MVS_Exist_RE2only` **3.20→3.20 MB/s (零变化)**, 全集 2.03→2.15。
   profile 里那 57% `runtime.cgocall` **是 C 里真实的整段扫描工作, 不是跨界开销** —— 批处理省不掉扫描本身。
   (差分护栏 `TestMVSKernelExistsManyMITM`: many==single==go。)

2. **全并 + LimEx 仍慢于"逐模式 + 门控"**。把全部 lean NFA 并成单一自动机, 用 LimEx 递推 (链边
   `prev<<1` 一次推进 + 稀疏异常边):
   - npos=3789, nword=60, **异常位置 756 (20%)**;
   - 吞吐: 朴素全并 1.19 → **LimEx 1.36 MB/s (+14%)** —— 但**仍远低于逐模式+门控的 3.20**。
   - 根因: 这 20% 异常是**自动机分支结构的固有属性** (大 alternation / 字符类 → 高出度位置),
     不是编号没排好; 即便最优重排 + SIMD (~2x) 也只到 ~2.7, 够不到 per-pattern 路径。
   - 更本质的原因: 全并**每字节扫全部 87 条** (nword=60 宽状态); 而真实流量里每条报文只触发**少数**
     规则, 故"字面量门控 + 逐模式稀疏验证"反而更优 —— 这与 Hyperscan 用 SIMD 字面量匹配 + 仅在命中点
     激活微型 NFA 的思路一致, 我们已是该结构 (AC 门控 + per-pattern NFA)。
   - (差分护栏 `TestMVSLimExVsMerged`: LimEx 命中集 == 朴素全并 == existsIn/oracle。)

**因此当前最优架构 = 字面量门控 + 逐模式 NFA + 有界宽窗口化** (本轮已落地), 对抗集存在性 ~3.2 MB/s。
要再上一个台阶只剩两条路, 均需人类决策: (a) 投入 Hyperscan 级 SIMD 工程 (FDR/Teddy 全量字面量 + 分组
微型 NFA, 数周~数月, 且原型数据显示对抗集仍可能够不到 80x); (b) 锁定 ~19x 这一**全平台可移植、零原生
依赖、不崩溃**的成果, 转而打磨确定收益项 (断言路径共享 boundary、gate regexp2 复核 rune 复用)。

### Phase 4 (2026-06): bridge 移除 + 确定收益项落地 —— 实测更新

> 按"只允许自研、不加载任何动态库"的硬要求, 先**整体移除 vectorscan/libhs 对照桥** (4 文件删除 +
> 枚举/选择/API/scratch 字段清理), mvscan 自此是唯一高性能后端。随后落地上文 (b) 的两个确定收益项,
> 全程不破坏存在性语义 (差分 oracle + 新增对抗性窗口测试全过):
>
> 1. **断言路径边界共享** (`mvs_assert.go` + `scratch.assertBound`): 一份报文内多条零宽断言 NFA
>    (`\b \B`/行锚) 改为**每报文惰性算一次 boundary、全员复用** (原本各自重算 `computeBoundaries`/rune 解码)。
> 2. **存在性本地化 / Rose-lite 左右截窗** (`mvs_window.go`): 对 RE2-exact、非锚定的字面量门控 pattern,
>    命中后按 AST 上下文宽度算 `[head,tail]` 界, 把整段 `nfaExists` 收窄到命中点邻域的 union 窗口
>    (锚定 / 尾部无界者安全退回整段, 零假阴; 对抗性测试 `mvs_window_test.go` 护栏)。

| 档位 (darwin/arm64, C 内核, 同一对抗集) | Phase 1~3 | Phase 4 | 变化 |
|---|---|---|---|
| MVS · 存在性 (全 87 条) | 2.13 MB/s | **2.44 MB/s** | **+14.5%** |
| MVS · 定位 (全 87 条) | 1.52 MB/s | **1.75 MB/s** | **+15%** |
| MVS · 存在性 (纯 RE2 子集) | 3.21 MB/s | **3.83 MB/s** | **+19.3%** |
| MVS · 存在性 (纯 RE2 子集) 内存/op | 29.5 MB | **0.45 MB** | **降 65x** |

仍受限于"尾部无界"模式 (`token=\w+`/`.*foo`) 必须整段扫描 —— 这正是 Rose-lite 链分解要解决的;
连同 Teddy 默认化, 是继续逼近 vectorscan 的两条主路 (见 `MINI_VECTOR_SCAN_IMPL.md` 第 0'.4 节)。

### 这些测试正常吗? (有效性自检)

- **测的是真吞吐**: `b.SetBytes(语料总字节)` → MB/s 为聚合真实吞吐; 回调里累加 `hits` 并 `_ = hits`
  防止被编译器优化掉; 三个后端共用**同一份语料、同一套规则、同一个 `Scan` API** (含回调派发/scratch)。
- **可复现且稳定**: 多次重复 MVS 定位档 1.12~1.15 MB/s、存在性档 1.38~1.41 MB/s、StdlibLoop 稳定
  0.17 MB/s (与上文 Engine 章节基线一致), 抖动 <3%。
- **正确性有独立护栏**: 吞吐数字不掺正确性判断; 命中集合/偏移由差分测试 (`mvs_*_test.go`, MVS==stdlib
  逐字节 + C==Go 逐位) 单独保证。
- **一个口径提示**: pprof 的 alloc_space / allocs-per-op 含一次性建库与 `loadCorpus` (~5%) 折算入采样;
  稳态每 op 分配以 `ReportAllocs` 数为准, 其大头确为 regexp2。基准为该**对抗性**真实集; 字面量友好的
  合成集 (见上文"合成规模") 仍有数十~上百倍加速, 两者不矛盾。

## 分发模型 (易分发 + 不崩溃 + 及时退化)

- **默认产物 (不带任何 tag, `CGO_ENABLED=0`)**: 纯 Go, 零原生依赖, 全平台可移植, RE2 精确偏移。
  这是永远可分发、永不崩溃的基线 (对抗集 2.4x, 字面量友好场景几十~上百倍)。
- **CGO 产物 (`CGO_ENABLED=1`, mvs 默认)**: 自动编入自带的纯 C99 位并行内核 (最强档) ——
  - **既不链接、也不 dlopen 任何外部库** (无 libhs/vectorscan/系统正则), 内核源码随仓库分发;
  - **任意架构都能编译** (未知架构只编标量孪生), x86 上仍可 SSSE3/AVX2、arm64 NEON 加速;
  - SIMD 探测不到时一律落标量, 结果与纯 Go 逐字节一致。
- 一句话: **无论哪种环境, `Compile(..., WithBackend(BackendMVS))` 都不会失败/崩溃**,
  能下沉 C 内核就下沉 (Tier 1), 不能就退化为纯 Go。用 `db.Info().Backend` 可观测实际生效后端。

## 取舍与边界

- **强项**: 字面量丰富、命中稀疏、正则数 (N) 大的场景 (指纹识别、扫描规则、IOC 匹配),
  线性劣化被消除, 加速比随 N 增长。海量规则的**存在性打标** (MITM) 用 mvs 存在性档获得
  端到端加速 (对抗集 ~14x, 字面量友好场景更高)。
- **弱项**: 充斥宽泛正则 (`.*` + 常见短字面量) 的对抗性规则集; 此时整段验证成本主导, mvs 存在性
  约 14x (纯 Go 引擎约 2.4x)。再上台阶需 Teddy 默认化 + Rose-lite 链分解 (见 PROFILE 第 0'.4 节)。
- **存在性档不提供精确偏移** (`From/To=-1`): 需要 `[From,To)` 做替换/抽取时去掉 `WithReportLocation(false)`,
  由 NFA 给出精确偏移; 或先用存在性档做"是否命中"的快速门, 命中后再取精确位置。
- 与 stdlib `regexp` 一样, NFA 是 RE2 自动机, 不支持 backreference 与任意 lookaround
  (数学本质); 这类正则经 regexp2 兜底为 always-on (尽量带超集字面量门减少触发)。

### 已验证为负向的优化 (避免后人重复踩坑)

针对对抗性规则集的整段验证瓶颈, 曾尝试以下两种"看似显然"的优化, 经实测均**变慢**, 已回退:

- **组合 gate (RE2::Set 式并集预筛)**: 把多条非窗口正则用 alternation 合并为一个自动机,
  整组只跑一次 `Match`, 不命中则跳过逐条验证。对**字面量友好**的正则有效, 但对抗集里这些
  正则各自昂贵, 合并后的自动机成本**叠加**, 且其短而常见的字面量几乎每条报文都触发 gate;
  实测 12.9s -> 17.5s。根因: 组合自动机的单次代价 ≈ 各成员之和, 而非各成员之最廉者。
- **无界正则的方向性 (单侧有界) 邻域窗口**: 对"字面量左侧有界、右侧无界"的正则按命中点开窗。
  问题在于这类正则的字面量通常常见 (多次命中), 每次命中都开一个延伸到缓冲末尾的窗口, 多次
  near-full 扫描反而劣于"整段只扫一次" (`fullDone` 去重) 的现状。

要在对抗集上真正逼近 Hyperscan/vectorscan 的吞吐, 需要的是 **Teddy SIMD 默认化 + Rose-lite
链分解** (命中点小邻域验证), 而非在 RE2 逐条之上加预筛 —— 后者已被实测证否, 详见 PROFILE 第 0' 节。

## 当前未实现 (后续可扩展, 按收益排序)

- **Teddy SIMD 预过滤默认化** (最大头): 把 `native/teddy.c` 接为默认 CGO 预过滤替掉标量 AC,
  标量孪生兜底 `CGO_ENABLED=0`。这是逼近 Hyperscan 吞吐的主路径。
- **Rose-lite 字面量链分解**: 对"尾部无界"模式 (`token=\w+`/`.*foo`) 做命中点小邻域验证,
  消除目前仍存在的整段 forward 扫描。
- **断言 NFA 合并**: 仿 `mvs_merged.go` 把带 guard 的断言 NFA 并为单趟 (profile ~18% CPU)。
- 流式 (streaming) 与向量化 (vectored) 扫描; 当前仅 block 模式。
- 数据库序列化 / 反序列化 (平台无关 blob 落盘缓存)。

## 文件结构

```
minirehs.go          公共类型 (Pattern/Match/Flag/...)
options.go           functional options + Logger (转发 common/log)
database.go          Compile 编排 + Database/Scratch + 邻域窗口参数
verifier.go          re2Verifier (精确偏移) / regexp2Verifier (yaklang regexp-utils 兜底)
feature_gate.go      RE2 可表达性判定
literal.go           必需字面量提取 (字面量因式分解)
width.go             最大宽度/锚点分析 (决定能否邻域验证)
ahocorasick.go       自研 Aho-Corasick 自动机 (含命中位置)
prefilter.go         预过滤契约 + 纯 Go 标量实现 + ASCII 小写
prefilter_nocgo.go   默认构建: 标量预过滤
prefilter_cgo.go     CGO 构建: SIMD 预过滤 (含失败退化)
native/teddy.c       自带 SIMD 内核 (NEON / SSSE3 / 标量), 零外部依赖
engine_purego.go     自研引擎后端 (Tier 2/3)
backend_stdlib.go    stdlib 逐条后端 (Tier 4, oracle/基线)
backend.go           后端选择 (mvs/engine/stdlib)
composite.go         主后端 + 可选兜底子集 组合容器
mvs_backend.go       mvs 后端编排 (Glushkov bit-NFA + 字面量门控 + 窗口化存在性)
mvs_assert.go        零宽断言 NFA (\b \B / 行锚; 每报文共享 boundary)
mvs_window.go        Rose-lite 字面量上下文界 (本地化左右截窗, 零假阴)
mvs_merged.go        always-on lean NFA 合并单趟扫描
mvs_cgo.go / native/mvscan/  纯 C99 位并行内核 (amalgamation, 不加载任何动态库)
fuzz_test.go         随机 RE2 生成器 + 差分模糊安全网 (引擎 vs oracle 逐字节一致)
unit_test.go         各组件单元/边界/错误路径测试 (覆盖率 >=99%)
testdata/            rules.json / traffic_corpus.bin / gen_corpus.go
```
