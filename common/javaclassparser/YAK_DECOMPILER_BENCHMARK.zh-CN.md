# YAK Java 反编译器 Benchmark

> 语言：[English](./YAK_DECOMPILER_BENCHMARK.md) | **简体中文**
>
> 快照日期：2026-06-26。取数主机：darwin/arm64，Go 1.22.12。

本文记录 Yak Java 反编译器入口的发布基准：

- `javaclassparser.Decompile([]byte) (string, error)`
- Yaklang 封装 `java.Decompile`

当前分支已经从“已知结构性缺陷报告”推进为 GA 就绪报告：可移植回归套件通过，
安全契约通过，正在修复推进的 `.m2` 扫描窗口也已清零。

## 1. 当前状态

| 维度 | 结果 | 状态 |
|------|------|------|
| 合成语法语料 | 31/31 组，0 stub、0 语法错误、0 panic | GA |
| 合成 round trip（`反编译 -> javac`） | 26/26 个可评估组可重编译 | GA |
| 内嵌真实字节码回归 | 46 个聚焦 `.class` 回归全部可解析 | GA |
| 硬用例族 | switch、三元、try/catch、lambda、内部类、interface `<clinit>` 均覆盖 | GA |
| 确定性 | 多次反编译输出逐字节一致 | GA |
| 安全契约 | `Decompile` 不向外逃逸 panic；失败显式降级为 `yak-decompiler:` stub | GA |
| 当前 `.m2` 修复窗口 | 下方列出的已修窗口均为 `partial=0 err=0 stubs=0` | 已验证窗口 GA |

完整本地 Maven 缓存天然是动态目标：新依赖可能带来新的字节码形态。本分支规则很严格：
在主动扫描窗口内，任何字段/方法被 drop 的 `WARNING`，或任何生成 stub，均视为缺陷；
必须修复、补回归 class，然后重新扫描。

## 2. 最新验证

可移植测试：

```bash
go test -run TestDecompileSyntaxRegression -v ./common/javaclassparser/tests/
go test ./common/javaclassparser/...
```

这两个命令在本快照均已通过。

最近 `.m2` 分片结果：

| 范围 | 结果 |
|------|------|
| `943-1200` | `classes=31189 ok=31189 partial=0 err=0 stubs=0` |
| `1461-1600` | `classes=61150 ok=61150 partial=0 err=0 stubs=0` |
| `1756-1926` | Elasticsearch 系列修复后推进到 Liquibase，新缺陷出现前已有 `60103` 个 class ok |
| `1926-2000` | `classes=23262 ok=23262 partial=0 err=0 stubs=0` |

最后两行有意拆开记录：Liquibase slot 0 重用问题出现在 jar index `1926`；修复后，
已经重新跑完 `1926-2000` 后缀并清零。

## 3. 已修复的边界类型

以下真实世界字节码族已经由回归测试锁住：

- 局部槽在数组值、循环计数器、catch 变量之间复用。示例：XMLBeans
  `QNameHelper.hexsafe`。
- 泛型/原始类型参数槽冲突，以及空 `param_placeholder` 参数渲染。示例：
  Elasticsearch `CopyOnWriteHashMap$InnerNode.put`。
- interface 的 `static final` 字段 `<clinit>` initializer 无法源码 hoist 时仍需保留合法字段。
  示例：Elasticsearch `Client.CLIENT_TYPE_SETTING_S`。
- 实例方法在初始 receiver load 之后又写入 local slot 0。示例：Liquibase `co.at(ax)`。
- 多维 primitive 数组和 category-2 栈处理。示例：SparseBitSet `long[][][]`。
- try/finally 包裹循环容器时曾经产生 statement graph 自环。示例：Commons Collections
  `ExtendedProperties.load`。
- interface / annotation 的 `<clinit>` final static 字段 hoist。示例：ECJ `TypeConstants`
  与 `JavadocTagConstants`。
- 大型 boolean 三元树、boolean 构造参数、nil-safe return type reset。示例：JTidy、
  OpenRewrite、Saxon、ECJ。

这些不再作为已知残留缺陷跟踪；如果回归，应该先由回归套件失败暴露，而不必等到
`.m2` 长扫。

## 4. 安全契约

反编译器必须优先选择显式降级，而不是返回非法 Java：

- Go panic 不得逃逸 `Decompile`；
- 递归 statement walker 不得把进程打到 stack overflow；
- 非法源码不得作为成功反编译结果返回；
- 无法表示的方法体必须替换为带标记的 `yak-decompiler:` stub；
- 主动扫描工作中发现字段或方法 drop 不可接受，必须修。

当前实现会通过 Java 语法前端校验生成源码，并对畸形成员重新渲染或降级。
`.m2` harness 会记录 stub reason 与精确 jar/class 位置，便于把新失败沉淀成
regression fixture。

## 5. 扫描流程

本地 Maven 缓存建议用有边界的分片扫描：

```bash
GOMAXPROCS=2 \
STUB_REASONS=1 \
STOP_ON_FIRST=1 \
M2_INDUSTRY=1 \
M2_START_JAR_INDEX=1926 \
M2_START_JAR_END=2000 \
M2_CONCURRENT_JARS=1 \
M2_MAX_CLASSES=1000000 \
M2_MAX_PER_JAR=1000000 \
PROBLEM_DIR=/tmp/jdec-shard-1926-2000 \
PROGRESS_EVERY=100 \
M2_PROGRESS_FILE=/tmp/jdec-progress/1926-2000.env \
go test -timeout 30m -run TestM2StubReasons -v ./common/javaclassparser/tests/
```

推荐流程：

1. 用 `STOP_ON_FIRST=1` 跑分片。
2. 任何 warning、partial、panic、syntax failure、stub 都按缺陷处理。
3. 用 `TestDiagDecompileClass` 复现单个 class。
4. 用字节码（`javap -c -v`）和可用的外部反编译器交叉验证。
5. 修反编译器，补 regression `.class`，跑可移植测试，再从失败 jar 继续扫描。

## 6. 可移植复现命令

```bash
# 合成覆盖与 javac round trip
go test -run 'TestSyntaxCoverageMatrix|TestRecompileRoundtrip' -v ./common/javaclassparser/tests/

# 真实字节码语法回归
go test -run TestDecompileSyntaxRegression -v ./common/javaclassparser/tests/

# panic / 卡死 / 崩溃边界
go test -run 'TestGAPanicFreeBoundary|TestDecompileCyclicStatementTreeNoCrash' -count=1 ./common/javaclassparser/tests/

# 确定性
go test -run TestCorpusDeterminism -v ./common/javaclassparser/tests/

# javaclassparser 全包
go test ./common/javaclassparser/...
```
