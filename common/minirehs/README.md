# minirehs

`minirehs` 是一个**零外部依赖、可移植**的多正则批量匹配引擎, 借鉴 Intel Hyperscan 的
"统一编译, 一次扫描" (compile then scan) 模型: 把成百上千条正则统一编译成一个不可变
`Database`, 对输入只做一次字面量预过滤即可定位候选, 再只对候选做正则验证, 从而避免
"几百条正则逐条全量匹配" 的 `O(N_patterns x N_bytes)` 开销。

名字取 "mini Regex Hyperscan"。它**不是** Hyperscan 的移植, 而是用纯 Go (+ 可选自带
SIMD) 实现的轻量近似。

## 设计原则

1. **可移植优先**: 默认构建 (`CGO_ENABLED=0`, 不加任何 build tag) 在所有平台/架构上
   编译运行且功能完整, 走纯 Go 自研引擎。
2. **零外部依赖**: 不引入新的第三方库。CGO SIMD 是自带的 C 内核 (`native/teddy.c`),
   不链接 libhs 等外部库。Vectorscan 仅在对照基准里出现, 由独立 build tag 隔离。
3. **优雅退化**: CGO/SIMD 不可用时自动退化为纯 Go, 结果完全一致。
4. **结果一致**: 任何后端 (纯 Go / CGO-SIMD / stdlib) 对同一条 RE2 正则的命中集合
   **逐字节相同** (差分测试保证), 因为预过滤只决定"验证哪些位置", 验证逻辑共享。
5. **复用 yaklang 生态**: 特性 gate 与 regexp2 兜底直接复用 `common/utils/regexp-utils`。

## 后端分档 (Tier)

| Tier | 后端 | 技术 | 语义 | 触发条件 |
|------|------|------|------|----------|
| 1 | vectorscan | Vectorscan/Hyperscan 单一 SIMD 自动机 (运行时 dlopen) | **存在性** (From/To=-1) | `-tags minirehs_vectorscan` + 运行时可加载 libhs; 经 `WithBackend(BackendVectorscan)` 选用 |
| 2 | engine | 自带 SIMD (NEON / SSSE3) Aho-Corasick 跳过 | RE2 精确偏移 | `CGO_ENABLED=1` 且 `-tags minirehs_cgo` |
| 3 | engine | 纯 Go 标量 Aho-Corasick | RE2 精确偏移 | 默认 (任意平台) |
| 4 | stdlib | 无 (逐条全量匹配) | RE2 精确偏移 | 显式 `WithBackend(BackendStdlib)`, 作 oracle / 基线 |

引擎 (Tier 2/3) 核心流程: **字面量提取 -> Aho-Corasick 预过滤 (一次扫描, 含位置) -> 邻域窗口验证**。
对"有界宽度且无位置锚点"的正则, 只在字面量命中点附近的小窗口内跑 RE2 验证 (邻域锚定),
显著降低验证成本; 对无界/含锚点的正则退化为整段验证; 无必需字面量的正则归入 always-on。

**`Auto` 默认选用引擎 (Tier 2/3)**: 保证全平台可移植、RE2 精确偏移语义。Vectorscan 后端是
**可选的高性能存在性加速** (见下文), 需显式 `WithBackend(BackendVectorscan)` 启用。

### Vectorscan 加速后端 (Tier 1, 可选)

把成百上千条正则**一次性**编译进 Vectorscan/Hyperscan 的单一 SIMD 自动机, 对每条数据只扫描
一次即得"哪些规则命中"。这是成熟系统 (Suricata 等) 处理海量规则的标准做法, 对 MITM 打标这类
**以命中存在性为准**的场景 (yaklang MITM replacer 第一步即逐规则 `MatchString`) 是最优解。

- **运行时按需加载 (dlopen)**: 二进制**不在链接期依赖 libhs**, 启动时尝试加载
  `libhs.so/.dylib/.dll` (可用 `MINIREHS_HS_LIB` 指定路径)。**加载不到就退化为引擎**, 程序绝不
  因缺库而崩溃 —— 这是"易分发 + 不崩溃 + 及时退化"的核心保证。
- **不依赖 hs.h**: 所需函数签名/常量/结构体全部自声明 (已对照官方头文件), **构建机也无需安装
  Vectorscan**。
- **CPU 不支持自动退化**: 通过 `hs_valid_platform()` 校验 (x86 需 SSSE3), 不满足即退化为引擎。
- **语义为存在性**: 用 `HS_FLAG_SINGLEMATCH` 让每条正则至多上报一次, 命中以 `From/To=-1` 表示
  (与 regexp2-only 一致)。**不提供精确偏移** —— 需要精确 `[From,To)` 的场景请用引擎后端。
- **regexp2-only 规则 (backref/lookaround) 由 fallback 承载**: 这类 Vectorscan 无法编译的正则
  自动转交其原有 verifier 逐条判定存在性, 不丢规则。
- **一致性已验证**: Vectorscan 后端逐记录命中的**规则 ID 集合**与 stdlib RE2 oracle **完全相同**
  (合成随机 + 真实 MITM 全量差分测试)。

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

启用 Vectorscan 加速 (MITM 打标等存在性场景, 需 `-tags minirehs_vectorscan` 构建):

```go
// 不可用时 (未带 tag / 缺 libhs / CPU 不支持) 自动退化为引擎, Compile 绝不因此报错。
db, _ := minirehs.Compile(patterns, minirehs.WithBackend(minirehs.BackendVectorscan))
// db.Info().Backend == BackendVectorscan 表示加速生效; == BackendEngine 表示已退化。
// 命中以存在性上报 (m.From == m.To == -1), 表示"该规则在本数据中命中"。
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

Vectorscan 后端的差分一致性 (逐记录命中**规则 ID 集合**与 stdlib oracle 完全相同) + 退化/并发/早停:

```bash
CGO_ENABLED=1 go test -tags minirehs_vectorscan ./common/minirehs/ -run TestVectorscan -v
```

`vectorscan_test.go` 覆盖: 合成随机正则差分、真实 MITM 全量差分、regexp2 fallback、并发竞争
(`-race`)、提前终止、以及用 `MINIREHS_HS_DISABLE=1` 强制模拟"libhs 不可用"时的优雅退化。

### 测试覆盖率

```bash
go test ./common/minirehs/ -coverprofile=cov.out                              # 默认构建
CGO_ENABLED=1 go test -tags minirehs_cgo ./common/minirehs/ -coverprofile=cov_cgo.out
CGO_ENABLED=1 go test -tags minirehs_vectorscan ./common/minirehs/ -coverprofile=cov_vs.out -timeout 600s
go tool cover -func=cov.out
```

- 默认 (NoCGO) 构建: **99.1%** 语句覆盖。
- CGO SIMD 构建: **98.6%** 语句覆盖。
- CGO Vectorscan 构建: **98.2%** 语句覆盖 (含 dlopen 桥、存在性后端、退化/并发/早停路径)。
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

# Vectorscan 加速后端 (经同一 Scan API 端到端对照; 需运行时可加载 libhs)
CGO_ENABLED=1 go test -tags minirehs_vectorscan ./common/minirehs/ \
    -run '^$' -bench BenchmarkEngineVsVectorscan -benchtime 3x -timeout 600s
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
| **minirehs Vectorscan 后端** | **14.8 MB/s** | **87x** | 存在性 |

结论: 对这个**对抗性规则集**, 纯 Go 引擎 (精确偏移, 全平台可移植) 是 **2.4x 保底**; 而启用
**Vectorscan 加速后端** (存在性语义, 契合 MITM 打标) 直接达到 **87x (相对现状) / 36x (相对纯 Go
引擎)**, 且这是经过完整 Scan API、回调与 scratch 管理后的端到端数字 (Vectorscan 裸 C 循环计数
可达 ~80 MB/s)。环境不支持时自动退化为 2.4x 的引擎, 绝不崩溃。

下面是对纯 Go 引擎在该对抗集上"为何只有 2.4x"的诚实分析 (仍是 RE2 逐条之上做预过滤的能力边界):
这是一个**对抗性规则集** —— 大量形如 `(...key|auth|user|pass...).*?`
的宽泛正则, 字面量短而常见 (`key`/`auth`/`user` 在网页/JS 中遍地都是), 且含无界
`.*?`/`[^'"]+?` 构造。这类正则一旦字面量命中就需要近乎整段验证, 邻域窗口无法安全收窄。
本引擎仍比现状 (StdlibLoop) 快约 2.4 倍 (靠跳过其余约 70 条未触发的正则), 但远不及
Vectorscan —— 后者把所有正则编译进**单一 SIMD NFA**, 验证成本极低, 这正是成熟系统的
价值所在, 也是本轻量实现当前的能力边界。

## 分发模型 (易分发 + 不崩溃 + 及时退化)

- **默认产物 (不带任何 tag)**: 纯 Go, 零原生依赖, 全平台可移植, RE2 精确偏移。这是永远可分发、
  永不崩溃的基线 (对抗集 2.4x, 字面量友好场景几十~上百倍)。
- **带 `minirehs_vectorscan` 的产物**: 通过 dlopen 在运行时**可选**加载 libhs ——
  - 二进制**不在链接期依赖 libhs**, 即使目标机器没有 libhs 也能正常启动 (退化为引擎);
  - 构建机也**无需安装 Vectorscan** (不依赖 hs.h, 函数全自声明);
  - CPU 不满足 (x86 缺 SSSE3) 时 `hs_valid_platform()` 校验失败 -> 退化为引擎;
  - 可用 `MINIREHS_HS_LIB` 指定库路径; `MINIREHS_HS_DISABLE=1` 可强制禁用 (排障/测试)。
- 一句话: **无论哪种环境, `Compile(..., WithBackend(BackendVectorscan))` 都不会失败/崩溃**,
  能加速就加速 (Tier 1), 不能就退化为引擎 (Tier 2/3)。用 `db.Info().Backend` 可观测实际生效后端。

## 取舍与边界

- **强项**: 字面量丰富、命中稀疏、正则数 (N) 大的场景 (指纹识别、扫描规则、IOC 匹配),
  线性劣化被消除, 加速比随 N 增长。海量规则的**存在性打标** (MITM) 可启用 Vectorscan 后端获得
  数十倍端到端加速。
- **弱项 (纯 Go 引擎)**: 充斥宽泛正则 (`.*` + 常见短字面量) 的对抗性规则集; 此时验证成本主导,
  纯 Go 仅 ~2.4x。需要更高吞吐时启用 Vectorscan 后端 (代价: 存在性语义、需 libhs 运行时)。
- **Vectorscan 后端不提供精确偏移** (存在性 `From/To=-1`): 需要 `[From,To)` 做替换/抽取的场景请用
  引擎后端 (精确), 或仅用 Vectorscan 做"是否命中"的快速门, 命中后再用引擎/regexp2 取精确位置。
- 与 stdlib `regexp` 一样, 引擎是 RE2 自动机, 不支持 backreference 与任意 lookaround
  (数学本质); 这类正则经 regexp2 兜底为 always-on (两后端均如此)。

### 已验证为负向的优化 (避免后人重复踩坑)

针对对抗性规则集的整段验证瓶颈, 曾尝试以下两种"看似显然"的优化, 经实测均**变慢**, 已回退:

- **组合 gate (RE2::Set 式并集预筛)**: 把多条非窗口正则用 alternation 合并为一个自动机,
  整组只跑一次 `Match`, 不命中则跳过逐条验证。对**字面量友好**的正则有效, 但对抗集里这些
  正则各自昂贵, 合并后的自动机成本**叠加**, 且其短而常见的字面量几乎每条报文都触发 gate;
  实测 12.9s -> 17.5s。根因: 组合自动机的单次代价 ≈ 各成员之和, 而非各成员之最廉者。
- **无界正则的方向性 (单侧有界) 邻域窗口**: 对"字面量左侧有界、右侧无界"的正则按命中点开窗。
  问题在于这类正则的字面量通常常见 (多次命中), 每次命中都开一个延伸到缓冲末尾的窗口, 多次
  near-full 扫描反而劣于"整段只扫一次" (`fullDone` 去重) 的现状。

要在对抗集上真正逼近 Vectorscan, 需要的是**更快的统一验证引擎** (单一多模式 NFA), 而非在
RE2 逐条验证之上加预筛 —— 这是本轻量实现的能力边界, 也是下一步方向。

## 当前未实现 (后续可扩展)

- 流式 (streaming) 与向量化 (vectored) 扫描; 当前仅 block 模式。
- 数据库序列化 / 反序列化。
- 更强的验证引擎 (统一 NFA / Thompson 多模式), 以缩小与 Vectorscan 在宽泛规则集上的差距
  (见上文"已验证为负向的优化": 在 RE2 逐条之上加预筛已被证明无效, 须换验证引擎本身)。
- x86 AVX2 预过滤 (当前 x86 提供 SSSE3, arm64 提供 NEON)。

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
backend.go           后端选择 (含 Vectorscan 优雅退化)
composite.go         主后端 + 可选兜底子集 组合容器
vectorscan_bridge.go [tag] Vectorscan 加速后端 (dlopen 运行时加载, 自声明 hs API, 存在性匹配)
vectorscan_stub.go   非 tag 构建占位 (newVectorscanBackend 返回 nil -> 退化为引擎)
fuzz_test.go         随机 RE2 生成器 + 差分模糊安全网 (引擎 vs oracle 逐字节一致)
unit_test.go         各组件单元/边界/错误路径测试 (覆盖率 >=99%)
testdata/            rules.json / traffic_corpus.bin / gen_corpus.go
```
