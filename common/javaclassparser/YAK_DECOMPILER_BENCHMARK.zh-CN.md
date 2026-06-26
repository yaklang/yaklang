# YAK JAVA 反编译器工程化基准报告

> 语言：[English](./YAK_DECOMPILER_BENCHMARK.md) | **简体中文**

对 Yaklang Java 反编译器（`java.Decompile` / `javaclassparser.Decompile`）做可复现的工程化测评，覆盖 **语法安全**、**重建覆盖率**、**javac round-trip 正确性**、**确定性**、**测试卫生** 与 **分配开销** 六个维度。下文每一个数字都由本仓库中可复现的测试或基准产生（没有臆造或拍脑袋的数字），且可用各节给出的命令重新生成。

- 反编译器入口：`javaclassparser.Decompile([]byte) (string, error)`，以及 Yaklang 库封装 `java.Decompile`。
- 取数主机：darwin/arm64，Go 1.22.12，已安装 OpenJDK `javac`。
- 快速复现全部结果（无需联网、无需本地 Maven 缓存）：`go test ./common/javaclassparser/...`

> **范围说明。** "没有 Stub" 不等于 "正确还原"，通过 ANTLR 解析也不等于能通过 `javac` 重新编译。因此本报告严格区分三类断言：(1) 输出语法可解析；(2) 输出未降级为 stub；(3) 输出可被 `javac` 重新编译。只有 (3) 才是语义保真度的证据。

---

## 1. 摘要（Executive summary）

本报告从语法安全、重建覆盖率、`javac` round-trip 正确性、确定性、测试可移植性、分配开销几个维度评估 Yaklang Java 反编译器。该实现是一个**尽力而为、部分容错**的源码重建组件，适用于交互式审阅与安全分析工作流。它**不是语义等价的 Java 反编译器**，不应被当作自动化语义判定的唯一权威来源。

| 维度 | 结果 | 度量方式 |
|------|------|----------|
| 语法安全（解析或降级） | 31/31 语料组产出**语法可解析的 Java**；0 语法错误、0 硬错误、0 panic | `TestSyntaxCoverageMatrix` |
| 重建覆盖率（无 stub） | 31/31 组产出**未降级输出**（经典与现代语料均零 stub） | `TestSyntaxCoverageMatrix` |
| 正确性（javac round-trip） | **26/26** 个可评估语料干净重编译；0 失败、0 stub、0 反编译错误 | `TestRecompileRoundtrip` |
| 真实 jar 正确性（.m2 语料） | 80+ jar / ~12000 类：**ok=11830、partial=170、syntax=0、err=0**；逐类 sha256 指纹 diff 证明多次运行输出逐字节一致 | `TestM2RegressionHarness` |
| 确定性 | 多次反编译逐字节一致；性能改动由逐类 sha256 指纹守护 | `TestCorpusDeterminism`、`TestDumpJarFingerprint` |
| 测试套件 | 绿且快：`./...` ≈ 22s，无机器相关依赖 | `go test ./common/javaclassparser/...` |
| 分配开销 | 核心 **≈215 ms** 且 **≈161 MB 累计堆分配** / 106 类的 jar；反编译后的 ANTLR 重解析相对 core-only 增加运行时 ≈ +60%、字节 ≈ +42% | `BenchmarkDecompileJar` |
| 可扩展性 | ~8 worker 前近线性（3.6×），之后出现 **GC 瓶颈回退** | `BenchmarkDecompileJarParallel` |

反编译器的**安全保证成立**：对语料中的每一个输入，要么重建出方法，要么把它降级为带标记、仍可解析的 stub（`yak-decompiler:` 标记），绝不输出不可解析的 Java，也绝不从 `Decompile` 中 panic 逃逸。

### Round-trip 正确性细节

所有 26 个可进入严格 `javac` round-trip 验证的语料组（22 个单类组 + 4 个多类内部/嵌套类组）全部成功重编译：Annotations、Arrays、Boundary、CastsInstanceof、ComplexExpressions、ComplexMisc、Concurrency、ControlFlow、ControlFlowEdge、Enums、Exceptions、ExceptionsComplex、FieldsAndArrays、Generics、Inheritance、Initializers、InnerClasses、Lambdas、Literals、Loops、NestedControlFlow、NumericEdge、Operators、Strings、Switches、TryWithResources。该集合中 **0 重编译失败、0 stub、0 反编译错误**。

四个多类组端到端可重编译，检验了内部类重建：合成 `access$NNN` 桥、`this$0` 外部引用、`val$` 捕获字段、接口 `default` 方法、`@interface` 注解类型，以及枚举 synthetic 抑制与常量显式参数。

### 就绪度评估

该反编译器达到了用于"尽力而为代码展示"的**工程化 Beta** 水平，前提是：降级方法保持显式标记；下游分析不假设"语法合法即语义等价"；并在面对不可信输入之前补齐资源上限与不可信输入 fuzz。要达到 GA（通用可用）水平，仍需在真实 jar 覆盖（剩余的真实 jar partial）、畸形输入韧性与峰值资源刻画方面进一步改进。

---

## 2. 覆盖率基准

可复现的原因：语料是**Java 源码**，在测试时由 `javac` 现编（位于 `tests/corpus/{classic,modern}`），所以字节码在本机重新生成，而非签入仓库。

```
go test -run TestSyntaxCoverageMatrix -v ./common/javaclassparser/tests/
```

每组的结果分类：`OK`（完整重建且合法）、`STUB`（某成员降级为 stub 但类仍合法）、`SYNTAX`（输出了非法 Java——真实缺陷）、`ERROR`（反编译返回错误）、`PANIC`。

### 经典语料（Java 8 字节码）——26 组
```
ok=26  stub=0  syntax=0  error=0  panic=0
```

### 现代语料（Java 17 字节码）——5 组
```
ok=5  stub=0  syntax=0  error=0  panic=0
```

### 覆盖率结论
两类语料均产出**零 stub**——每组的每个成员都重建为真实 Java 而非降级。运算符、字面量、控制流、循环、switch、try-with-resources、数组、泛型、继承、内部类、枚举、lambda、字符串、注解、初始化器、并发、强转/instanceof、模式匹配、switch 表达式、文本块、record 与 sealed 类型，对受测语料都产出**语法可解析**的源码。"语法可解析"是比"可被 `javac` 重新编译"更弱的断言；衡量语义保真度的 round-trip 结果见第 3 节。

---

## 3. 正确性基准（反编译 → 重编译 round-trip）

最严格的 oracle：取已知良好的源码，编译它，反编译生成的 `.class`，再把反编译出的 Java **重新喂给 `javac`**。这比 ANTLR 语法网严格得多——它能抓出仍能解析的类型错误、优先级错误、不可达代码与错误操作数。

```
go test -run TestRecompileRoundtrip -v ./common/javaclassparser/tests/
```
`javac` 固定使用英文 locale（`-J-Duser.language=en -J-Duser.country=US`、`-nowarn -Xlint:none`），使诊断信息在不同机器上稳定。用 `RC_VERBOSE=1` 可输出反编译源码及每个分类的全部 `javac` 错误。

### 语料 round-trip 结果
该 oracle 会反编译一个组的**每一个** class（含内部类、嵌套类、匿名类、局部类），并把这些单元**一起编译**，因此内部类重建是端到端验证而非被跳过。
```
recompile-ok:  26  (Annotations, Arrays, Boundary, CastsInstanceof, ComplexExpressions,
                    ComplexMisc, Concurrency, ControlFlow, ControlFlowEdge, Enums,
                    Exceptions, ExceptionsComplex, FieldsAndArrays, Generics, Inheritance,
                    Initializers, InnerClasses, Lambdas, Literals, Loops, NestedControlFlow,
                    NumericEdge, Operators, Strings, Switches, TryWithResources)
recompile-fail: 0
stub:           0
dec-err:        0
multiclass:     0   (一起编译，不再跳过)
```

每个通过的分类都由 `recompileGateBaseline` 钉死，因此任何破坏绿色分类的回退都会让 CI 失败。

### round-trip 覆盖了什么
每个语料组检验一个独立的构造族，并端到端验证：

- **控制流**：`if/else` 链、`switch`（穿透、`String` switch、稀疏 lookup vs 稠密 table、default 在中间）、嵌套循环的 `break`/`continue`、跨多层的带标签 `break`/`continue`、`while(true)`+break、do/while、三层循环嵌套。
- **表达式与运算符**：`int/long/float/double` 混合提升、`long` 全宽位运算（`& | ^ << >> >>> ~`）、`&`/`|`/`^` 的 boolean/int 消歧、短路 `&&`/`||`（既作条件又作被返回/存储的布尔值）、深层右倾链式三元、`instanceof`+强转派发链、成员访问的强转优先级。
- **数值边界**：整型溢出环绕、达到/超过类型位宽的移位计数（`<<32`、`>>>33`）、带隐式窄化的复合赋值、十六/二/八进制与下划线字面量、`char` 算术、`float`/`double` 特殊值（`NaN`、`±Infinity`），以及数值字面量后缀（`9223372036854775807L`、`3.14F`、`2.718281828D`）。
- **字段与数组**：实例/静态字段、对字段数组元素的复合赋值与前后自增（`this.buf[i] *= 2`）、多维与交错数组、数组初始化器、正确的 `multianewarray` 维度、数组字段类型渲染、空白 `final` 字段。
- **异常**：`try/catch/finally`、嵌套 try/catch/finally、单资源与多资源 try-with-resources、多重 catch（`A | B`）、重抛、`return` 后的 `finally`。`finally` 体以忠实的脱糖形式重建（在每条退出路径上复制 + 一个 `catch (Throwable)` 重抛），与字节码执行完全一致。
- **类型与成员**：带 null 初始化 slot 类型加宽的泛型、继承、接口 `default` 方法、`@interface` 注解类型、完整枚举重建（抑制合成的 `values()/valueOf()/$VALUES`、剥离构造器合成前缀、显式常量参数）、内部/嵌套/匿名/局部类、形参作用域隔离且能恢复泛型签名的 lambda、并发（对 `this`/字段的 `synchronized`）。
- **pre-Java-6 字节码**：用 `jsr`/`ret` 子程序编译的 `try/finally` 在结构化前被内联为现代的 finally 复制形态，使老 jar 能反编译而非降级（见 §3.1）。

> **已知语义限制（非重编译失败）。** `Loops.labeled` 能干净重编译，但当 `continue <label>`
> 的目标是外层 `for` 循环的*自增*、且该自增节点与循环的自然退出边共享时，该 `continue` 可能被丢弃：
> `do{...}while(true)` 降级只能把共享的自增语句（`i++`）放在某一条后继路径上，另一条路径
> （`continue outer` 分支）便渲染成空的 `if` 体。这能编译通过，但对这一特定的 labeled-continue
> 惯用法可能在运行期产生分歧。已在 backlog 的"循环惯用法恢复"下跟踪；循环语义 round-trip 电池
> （`TestLoopSemanticsRoundTrip`，执行并比对指纹）覆盖所有非 labeled 形态且全部通过。

### 3.1 真实 jar 验证（.m2 语料）
除合成语料外，反编译器还针对真实 Maven 缓存做验证。`TestM2RegressionHarness` 在 80+ jar / ~12000 类上运行，并写出逐类 sha256 指纹：

```
ok=11830  partial=170  syntax=0  err=0
```

`syntax=0` 与 `err=0` 表示没有任何类产出不可解析的 Java，也没有任何反编译返回错误或 panic；`partial` 统计"至少一个成员降级为带标记 stub"的类。pre-Java-6 的 `try/finally` 子程序（`jsr`/`ret`）由 `core/jsr_inline.go` 内联：finally 体在每个 `jsr` 调用点复制，`ret` 改写为 `goto`，jsr 回边被重定向，finally 内嵌套的 try/catch 异常表项按调用点克隆。该 pass 在任何改写**之前**先校验整体形态，并保守保留一切非规范形态（`jsr_w`/`goto_w`/`switch` 宽目标、跨子程序边界的异常表项、16-bit 偏移溢出等）——退化为 stub 而非产出错误代码——且对不含 `jsr`/`ret` 的方法是 no-op。提供 `JSR_INLINE_OFF` 紧急开关回退旧行为。剩余的 170 个 partial 是 backlog 中跟踪的真实 jar 收敛前沿。

### "partial" / "stub" 并不意味着什么
被 stub 的成员周围仍是**结构化反编译出的、可读、语法可解析的 Java**（类的其余部分），且 stub 本身被显式打标（`yak-decompiler:` 标记），下游工具可检测。降级的成员绝不会被静默替换为"貌似正确但实际错误"的代码：对一个安全工具而言，明确标记的 stub 严格优于可编译但错误的重建。

---

## 4. 测试卫生基准

目标：一个稳定、快速、可移植的核心套件，没有机器相关或浪费时间的测试，同时保持真实覆盖。

```
go test ./common/javaclassparser/...      # 绿，总计 ~22s
```

套件特性：
- **机器相关诊断被收到环境变量后面**（`BENCH_JAR`、`JDSC_DIR`、`M2_DETERMINISM`），故默认运行从不扫描 `~/.m2` 或 `/tmp/...`，在 ~22s 内无外部依赖地完成。
- **可移植确定性检查**：`TestCorpusDeterminism` 无需本地 Maven 缓存即可验证逐字节一致的输出。
- **语料是源码而非字节码**：`tests/corpus/{classic,modern}` 是 `.java` 文件，在测试时由 `javac` 现编，因此夹具在本机重新生成并与运行中的 JDK 保持同步。

---

## 5. 性能基准

```
# 仅核心反编译器（关闭校验安全网）
BENCH_NO_VALIDATE=1 BENCH_JAR=<jar> go test -run xxx -bench 'BenchmarkDecompileJar$' -benchmem ./common/javaclassparser/tests/
# 完整流水线（默认开启校验）
BENCH_JAR=<jar> go test -run xxx -bench 'BenchmarkDecompileJar$' -benchmem ./common/javaclassparser/tests/
```

目标：`commons-codec-1.15.jar`（106 类），`-benchtime=5x -count=2`。

### 5.1 吞吐与校验安全网的开销

`BENCH_NO_VALIDATE=1` 关闭反编译后的 ANTLR 重解析，把**反编译器核心**与**安全网**隔离：

| 配置 | ns/op | B/op | allocs/op |
|------|------:|-----:|----------:|
| 完整流水线（开启校验） | ~343 M | 229 MB | 3.54 M |
| 仅核心（关闭校验） | **215 M** | **161 MB** | 2.28 M |
| **校验安全网占比** | 时间 **≈ 37%** | 字节 **≈ 30%** | 分配 **≈ 36%** |

安全网并非免费，但它是"绝不让不可解析的 Java 离开 `Decompile`"的契约；~36% 的墙钟时间是这一保证的代价（它是对整个类的一次 ANTLR 重解析，其 ATN 模拟分配主导了这部分占比，且为第三方运行时的固有成本）。

### 5.2 profile 是 GC-bound——分配才是真正的货币

核心的 CPU profile（`go tool pprof -top`）由垃圾回收器主导，而非反编译逻辑：

```
runtime.gcDrain        47.9% cum
runtime.scanobject     40.7% cum
runtime.mallocgc       19.2% cum
runtime.greyobject     13.3% cum
```

所以**减少分配直接换来 CPU**。核心被刻意构建为低分配，并由 `TestDumpJarFingerprint`（逐类 `sha256(status+output)`，改动前后 diff 干净）证明输出等价。当前核心中的低分配设计选择：

- `WalkGraph` 使用普通 `map[T]struct{}` 的 visited 集合（无接口装箱、无互斥锁——遍历是单 goroutine）与切片支撑的 DFS 栈。
- `GenerateDominatorTree` 跨不动点 sweep 复用单个 scratch 位集，并以"先计数后填充"的两遍法按确切最终容量构建每个 idom 的子切片（无逐步 `append` 扩容，无 per-idom 排序闭包）。
- `CalcMergeOpcode`、`ScanJmp` 与 `DropUnreachableOpcode` 使用普通 map 并复用缓冲，替代带互斥锁的 `Set[*OpCode]`。
- `CalcOpcodeStackInfo` 把 `opcodeToSim` 与 `nodeToVarScope` 预分配为 `len(d.opCodes)`（每个 opcode 恰好一条）。
- `fixJavaStringEscapes` 使用包级预编译正则；纯 ASCII 字符串字面量完全跳过 MIME 嗅探。
- `ParseOpcode` 按字节码长度预分配 opcode 切片与两个 offset map；`DumpClass.assemble` 使用 `strings.Builder`（O(n)，而非 O(n²)）。

在校验路径上，绝大部分分配是 ANTLR ATN 模拟对象（`NewBaseATNConfig`、`BaseATNConfigSet.Add`、prediction-context 合并）——这是逐类重解析的固有成本，不改 ANTLR 运行时无法消除。

### 5.3 负载严重长尾

`TestTopSlowClasses`（一次冷遍历，按时间排序）显示极少数类主导总成本：

| Jar | 类数 | top-1 类 | top-1% 类 | top-10% |
|-----|----:|--------:|----------:|--------:|
| commons-codec-1.15 | 106 | 14.6% | 14.6% | 68.7% |
| byte-buddy-1.14.17 | 2845 | 26.3% | **60.8%** | 88.4% |

在 byte-buddy 上，**一个 43 KB 的类**（`InstrumentedType$Default`）占整次冷遍历的 26%，top 1% 的类占 61%。含义：平均情况调优只能小幅提升吞吐；高价值目标是病态长尾（深度嵌套 CFG / 巨型方法，压垮结构化与栈模拟阶段）。

### 5.4 冷启动 vs 热稳态

同一个 `InstrumentedType$Default` 在冷的一次性遍历里耗 **7.9 s**，但热态重复时仅 **~127 ms**（≈62×）。差距是一次性进程初始化（ANTLR ATN 反序列化、正则编译、`sync.Once` 设置）被第一个复杂类吸收。对**批量/jar** 反编译这会摊销到可忽略；对**单类 CLI** 调用，这是值得预热的真实延迟下限。

### 5.5 并行可扩展性

`BenchmarkDecompileJarParallel` 在 byte-buddy（完整 jar，热态）上，变化 `BENCH_CONC`：

| Worker 数 | ns/op | 加速比 |
|----------:|------:|------:|
| 1 | 4.27 s | 1.0× |
| 2 | 2.27 s | 1.88× |
| 4 | 1.38 s | 3.09× |
| 8 | 1.19 s | 3.59× |
| 16 | 1.71 s | 2.50×（**回退**） |

扩展在 ~4 worker 前近线性，约 8 见顶（3.6×），之后**回退**。这是 5.2 节的 GC-bound 特征：众多分配型 goroutine 在共享回收器上争用。进一步的分配削减是抬高多核上限的路径。

### 5.6 为什么"跨解析 ANTLR 缓存"这个大杠杆被刻意不动
固定的 ANTLR Go 运行时（`v4.0.0-20220911`）对其 DFA / `JStore` 结构没有加锁，而反编译是并行运行的（jdsc 自检用 100 个 goroutine）。进程级共享校验 DFA 会数据竞争；现有的每 worker 缓存 + `DetachParserATNSimulatorCaches` 设计是安全选择。进一步推进需要升级 ANTLR（超出范围），记为未来工作。

---

## 6. Backlog（按影响排序，源自上文数据）

**正确性（语义保真度）：**
1. **真实 jar partial 收敛**——通过诊断真实字节码上残留的逐类 stub 原因，把 `.m2` 剩余的 170 个 partial 推向零（合成语料已是 0 stub / 0 round-trip 失败）。
2. **循环惯用法恢复**——重建 `for`/`while` 而非一律 `do{...}while(true)` 降级。这能修复 `labeled` 的 `continue <外层自增>` 语义限制（do-while 模型只能把共享自增节点放在一条后继上），并提升可读性。
3. **idiomatic `finally` 折叠**——`try/catch/finally` 的 round-trip 当前已正确（采用忠实的脱糖形式：finally 体重复 + `catch (Throwable)` 重抛，与字节码运行完全一致）。未来可加一个 pass 把它折叠为单个 idiomatic 的 `finally {}` 块以提升可读性。
4. **不可信输入加固**——在面对敌意输入之前补齐资源上限与畸形输入 fuzz。

**性能（全部服务于 5.2 节的 GC-bound profile）：**
5. **进一步削减分配**——在结构化与栈模拟阶段继续降低分配，以抬高并行上限（5.5 节）。
6. **长尾类结构化复杂度**（5.3 节）——剖析并降低病态 1% 类上的超线性成本。
7. **单类冷启动预热**（5.4 节）——为 CLI 用法预热一次 ANTLR/正则。
8. **共享校验 DFA**——仅在 ANTLR 运行时升级使其线程安全之后。

---

## 7. 复现速查

```
# 覆盖率矩阵（javac 现编语料）
go test -run TestSyntaxCoverageMatrix -v ./common/javaclassparser/tests/

# 正确性 round-trip（反编译 -> javac）；RC_VERBOSE 输出完整诊断
go test -run TestRecompileRoundtrip -v ./common/javaclassparser/tests/
RC_VERBOSE=1 go test -run TestRecompileRoundtrip -v ./common/javaclassparser/tests/

# 确定性（可移植，无需 Maven 缓存）
go test -run TestCorpusDeterminism -v ./common/javaclassparser/tests/

# 完整快速套件
go test ./common/javaclassparser/...

# 性能：核心 vs 完整流水线、扩展性、长尾分布与等价性
BENCH_JAR=<jar> go test -run xxx -bench 'BenchmarkDecompileJar$' -benchmem ./common/javaclassparser/tests/
BENCH_NO_VALIDATE=1 BENCH_JAR=<jar> go test -run xxx -bench 'BenchmarkDecompileJar$' -benchmem ./common/javaclassparser/tests/
BENCH_JAR=<jar> BENCH_CONC=8 go test -run xxx -bench 'BenchmarkDecompileJarParallel$' ./common/javaclassparser/tests/
BENCH_JAR=<jar> go test -run TestTopSlowClasses -v ./common/javaclassparser/tests/   # 长尾分布
OUT_DIR=/tmp/fp DIFF_JARS=<jarA:jarB> go test -run TestDumpJarFingerprint ./common/javaclassparser/tests/   # 输出等价性证明
```
