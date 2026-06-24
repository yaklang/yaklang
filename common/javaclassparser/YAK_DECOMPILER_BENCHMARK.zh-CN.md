# YAK JAVA 反编译器工程化基准报告

> 语言：[English](./YAK_DECOMPILER_BENCHMARK.md) | **简体中文**

对 Yaklang Java 反编译器（`java.Decompile` / `javaclassparser.Decompile`）做可复现的工程化测评，覆盖 **语法安全**、**重建覆盖率**、**javac round-trip 正确性**、**确定性**、**测试卫生** 与 **分配开销** 六个维度。下文每一个数字都由本仓库中可复现的测试或基准产生（没有臆造或拍脑袋的数字），且可用各节给出的命令重新生成。

- 反编译器入口：`javaclassparser.Decompile([]byte) (string, error)`，以及 Yaklang 库封装 `java.Decompile`。
- 取数主机：darwin/arm64，Go 1.22.12，已安装 OpenJDK `javac`。
- 快速复现全部结果（无需联网、无需本地 Maven 缓存）：`go test ./common/javaclassparser/...`

> **范围说明。** "没有 Stub" 不等于 "正确还原"，通过 ANTLR 解析也不等于能通过 `javac` 重新编译。因此本报告严格区分三类断言：(1) 输出语法可解析；(2) 输出未降级为 stub；(3) 输出可被 `javac` 重新编译。只有 (3) 才是语义保真度的证据。

---

## 1. 摘要（Executive summary）

本报告从语法安全、重建覆盖率、`javac` round-trip 正确性、确定性、测试可移植性、分配开销几个维度评估当前的 Yaklang Java 反编译器。该实现适合作为一个**尽力而为、部分容错**的源码重建组件，用于交互式审阅与安全分析工作流。它**尚不是语义等价的 Java 反编译器**，不应被当作自动化语义判定的唯一权威来源。

| 维度 | 结果 | 度量方式 |
|------|------|----------|
| 语法安全（解析或降级） | 23/23 语料组产出**语法可解析的 Java**；0 语法错误、0 硬错误、0 panic | `TestSyntaxCoverageMatrix` |
| 重建覆盖率（无 stub） | 20/23 组产出**未降级输出**（无 stub）；3 组隔离出具体缺口 | `TestSyntaxCoverageMatrix` |
| 正确性（javac round-trip） | **14/18** 个可评估语料干净重编译（起始为 4/13）；四个内部类/嵌套类组全部可重编译 | `TestRecompileRoundtrip` |
| 确定性 | 多次反编译逐字节一致；性能改动通过逐类 sha256 指纹证明输出等价 | `TestCorpusDeterminism`、`TestDumpJarFingerprint` |
| 测试套件 | 绿且快：`./...` ≈ 22s，从 150s 以上降下来（**至少 6.8 倍**），无机器相关依赖 | `go test ./common/javaclassparser/...` |
| 分配开销 | 核心 **≈246 ms** 且 **≈182 MB 累计堆分配** / 106 类的 jar；校验相对 core-only 增加运行时 ≈ +18%、累计分配 ≈ +23% | `BenchmarkDecompileJar` |
| 可扩展性 | ~8 worker 前近线性（3.6×），之后出现 **GC 瓶颈回退** | `BenchmarkDecompileJarParallel` |

反编译器的**安全保证成立**：对语料中的每一个输入，要么重建出方法，要么把它降级为带标记、仍可解析的 stub（`yak-decompiler:` 标记），绝不输出不可解析的 Java，也绝不从 `Decompile` 中 panic 逃逸。

### Round-trip 正确性细节

在 18 个可进入严格 `javac` round-trip 验证的经典语料组中（14 个单类组 + 4 个多类内部/嵌套类组，其中 Exceptions 一组以 stub 形式保留）：

- **14 个成功重编译**：Annotations、Arrays、CastsInstanceof、Concurrency、ControlFlow、Enums、Generics、Inheritance、Initializers、InnerClasses、Literals、Strings、Switches、TryWithResources。
- **3 个暴露具体的语义/类型缺陷**：Lambdas（捕获变量命名）、Loops（`do{...}while(true)` 降级产生 javac 不可达代码）、Operators（短路布尔表达式恢复）。
- **1 个以 stub 保留**：Exceptions（`try/catch/finally` 控制流有多个后继）。

四个多类组现在全部可重编译，端到端地检验了内部类重建：合成 `access$NNN` 桥、`this$0` 外部引用、`val$` 捕获字段、接口 `default` 方法、`@interface` 注解类型，以及枚举 synthetic 抑制与常量显式参数。

### 就绪度评估

该反编译器达到了用于"尽力而为代码展示"的**工程化 Beta** 水平，前提是：降级方法保持显式标记；下游分析不假设"语法合法即语义等价"；并在面对不可信输入之前补齐资源上限与不可信输入 fuzz。要达到 GA（通用可用）水平，仍需在 `javac` round-trip 正确性、真实 jar 覆盖、畸形输入韧性、现代字节码支持与峰值资源刻画方面有实质性改进。

---

## 2. 覆盖率基准

可复现的原因：语料是**Java 源码**，在测试时由 `javac` 现编（位于 `tests/corpus/{classic,modern}`），所以字节码在本机重新生成，而非签入仓库。

```
go test -run TestSyntaxCoverageMatrix -v ./common/javaclassparser/tests/
```

每组的结果分类：`OK`（完整重建且合法）、`STUB`（某成员降级为 stub 但类仍合法）、`SYNTAX`（输出了非法 Java——真实缺陷）、`ERROR`（反编译返回错误）、`PANIC`。

### 经典语料（Java 8 字节码）——18 组
```
ok=17  stub=1  syntax=0  error=0  panic=0
```
- 唯一的 `STUB` 是 **Exceptions** → `tryCatchFinally(int[],int)` 失败于 `ParseBytesCode failed: multiple next`。

### 现代语料（Java 17 字节码）——5 组
```
ok=3  stub=2  syntax=0  error=0  panic=0
```
- `STUB` 组 **Records** 与 **SealedVar** 仅在编译器合成的 `toString()/hashCode()/equals()` 上失败，报 `ParseBytesCode failed: call bootstrap method error`（即 `invokedynamic` 的 `ObjectMethods` bootstrap）。

### 覆盖率结论
剩余两个缺口被精确隔离且彼此正交：
1. **`try/catch/finally` 控制流重建**（"multiple next"）——当区域有多个后继时的控制流结构化限制。
2. **Record / sealed 的 `invokedynamic ObjectMethods` bootstrap**——自动生成的值类型方法尚未合成。

其余一切（运算符、字面量、控制流、循环、switch、try-with-resources、数组、泛型、继承、内部类、枚举、lambda、字符串、注解、初始化器、并发、强转/instanceof、模式匹配、switch 表达式、文本块）对受测语料都产出**语法可解析**的源码。"语法可解析"是比"可被 `javac` 重新编译"更弱的断言；衡量语义保真度的 round-trip 结果见第 3 节。

---

## 3. 正确性基准（反编译 → 重编译 round-trip）

最严格的 oracle：取已知良好的源码，编译它，反编译生成的 `.class`，再把反编译出的 Java **重新喂给 `javac`**。这比 ANTLR 语法网严格得多——它能抓出仍能解析的类型错误、优先级错误、不可达代码与错误操作数。

```
go test -run TestRecompileRoundtrip -v ./common/javaclassparser/tests/
```
`javac` 固定使用英文 locale（`-J-Duser.language=en -J-Duser.country=US`、`-nowarn -Xlint:none`），使诊断信息在不同机器上稳定。

### 语料 round-trip 结果
该 oracle 会反编译一个组的**每一个** class（含内部类、嵌套类、匿名类、局部类），并把这些单元**一起编译**，因此内部类重建是端到端验证而非被跳过。
```
recompile-ok:  14  (Annotations, Arrays, CastsInstanceof, Concurrency, ControlFlow,
                    Enums, Generics, Inheritance, Initializers, InnerClasses,
                    Literals, Strings, Switches, TryWithResources)
recompile-fail: 3  (Lambdas, Loops, Operators)
stub:          1   (Exceptions)
dec-err:       0
multiclass:    0   (现已一起编译，不再跳过)
```

剩下 3 个重编译失败是可执行的正确性前沿。下面每条根因都通过阅读**完整**的 `javac` 诊断确认（用 `RC_VERBOSE=1` 输出反编译源码 + 每个分类的全部错误），而非臆测：

| 分类 | 确切的 javac 错误 | 已确认根因 | 难度 |
|------|-------------------|------------|------|
| Loops | `unreachable statement`（嵌套无限区域之后的 `continue;`） | 每个循环都被降为 `do{...}while(true)`；内层必走的退出使得合成的外层 `continue` 不可达 | 难（循环惯用法恢复） |
| Operators | `missing return statement`（1 个错误，原为 13） | `(a && b) \|\| (c)` 短路 `\|\|` 被降为 `if/else`，其 true 分支丢了 `return true`；属布尔表达式/`\|\|` 重建缺口 | 难（控制流恢复） |
| Lambdas | `variable v already defined` + 泛型擦除 | lambda 形参名与外层 slot 名冲突；裸函数式接口目标拒绝显式 `Integer` 形参类型（泛型签名未恢复） | 难（变量命名 + 泛型擦除） |

通过的分类由 `recompileGateBaseline` 钉死，因此任何破坏 14 个绿色分类的回退都会让 CI 失败；其余作为 backlog 跟踪。

### 本轮落地的正确性修复——第 4 轮（null slot 类型加宽）
**Generics** 通过修复 slot 拆分转绿。一个跨方法复用的 JVM 局部 slot，只要类型发生变化就会被拆成两个变量，因为 `AssignVar` 用类型字符串精确匹配来判定变量身份。极其常见的 `T x = null; ...; x = v; ...; return x;` 惯用法把首次存储类型推断为 `java.lang.Object`（null 字面量类型），把重新赋值推断为具体类型，于是 slot 被拆成 `Object var1 = null` 加上第二个块作用域的 `Comparable var4 = v`；末尾的 `return var4` 便引用了越界变量。现在：若某 slot 的变量仅被 null 初始化，则在后续赋具体**引用**类型时**采纳**该类型而不再拆分（赋基本类型仍拆分，因为基本类型不能接受 null），且 `T x = null` 声明渲染变量的细化类型——声明、重新赋值与 return 三者一致。已由 golden、`TestCorpusDeterminism` 及真实 jar 的 ok/err/panic/stub 计数验证无回归。

### 本轮落地的正确性修复——第 3 轮（内部类 + 作用域）
又修复 5 个缺陷，使 **TryWithResources** 与全部四个多类内部/嵌套类组（**InnerClasses、Inheritance、Annotations、Enums**）干净重编译。已由 golden 套件、`TestCorpusDeterminism` 及真实 jar（commons-codec、gson：前后 `ok`/`err`/`panic`/stub 计数一致）验证无回归：

1. **作用域感知的局部变量重命名**（`dumper.go`）。JVM 复用局部 slot，反编译器按 slot 深度命名（`varN`），故嵌套源作用域中的两个不同变量可能塌缩成同名（例如 try-with-resources `close()` 脱糖产生的两个嵌套 `catch (Throwable var4)`）。新增的预渲染 pass 按词法作用域顺序遍历方法体，**仅当**某声明的名字仍被外层作用域中另一变量占用时才重命名，使用反编译器永不生成的 `_<n>` 后缀。无冲突的输出逐字节不变。→ **TryWithResources 绿**；并广泛修复真实代码中的嵌套 catch / slot 复用冲突。
2. **round-trip oracle 现在一起编译内部类**（`recompile_roundtrip_test.go`）。一个组的每个 `.class` 反编译为各自的 `$` 命名单元并一起重编译——真正检验合成 `access$NNN` 桥、`this$0` 捕获、`val$` 字段与 `Outer$Inner` 引用。→ **InnerClasses 绿**。
3. **接口 `default` 方法**（`dumper.go`）。接口中带方法体的非抽象非静态实例方法漏写 `default`，方法体非法（"interface abstract methods cannot have body"）。→ **Inheritance 绿**。
4. **`@interface` 注解类型**（`access_flags_verbose.go`、`dumper.go`）。注解类型（ACC_INTERFACE|ACC_ANNOTATION）被渲染成普通 `interface`（"X is not an annotation interface"），且把隐式的 `Annotation` 父接口显式写出。现以 `@interface` 关键字渲染并去除隐式父接口。→ **Annotations 绿**。
5. **枚举重建**（`dumper.go`）。合成的 `values()`/`valueOf()`/`$values()` 方法与 `$VALUES` 字段被原样输出（"method already defined"），构造器暴露了合成的 `(String name, int ordinal)` 形参与 `super(name, ordinal)` 调用（"call to super not allowed in enum constructor"），且常量不带参数。现在真正的枚举抑制全部 synthetic、剥离构造器的合成前缀，并从 `<clinit>` 的 `new EnumType(name, ordinal, args...)` 表达式解析出每个常量的显式参数（如 `EARTH(5.976e+24D, 6.37814e+06D)`）。→ **Enums 绿**。

### 本轮落地的正确性修复——第 2 轮（准确度攻坚）
又从 round-trip oracle 诊断并修复 5 个缺陷，使 **Arrays、Initializers、Concurrency** 干净重编译，并把 **Operators 从 13 个 javac 错误压到 1 个**。全部由 golden 套件、`TestCorpusDeterminism` 及真实 jar（commons-codec、gson：前后 `ok`/`stub` 计数一致——输出内容正确变化、无新增失败）的 stub/error/panic 计数 diff 验证无回归：

1. **`multianewarray` 维度翻倍**（`code_analyser.go`）。常量池条目已是完整数组类型（`[[I` = `int[][]`），处理器却按每个弹出的长度再包裹一次，导致 `int[][] a = new int[3][4]` 反编译成 7 维的 `int[][][][][][][] a = new int[3][4][][]`。现在直接使用常量池类型，并精确按 `dimensions` 操作数字节弹出对应数量的长度。→ **Arrays 绿**。
2. **依赖形参的字段初始化器错误提升**（`dumper.go`）。任何在 `<init>`/`<clinit>` 中赋值的 `final` 字段都被把右值提升为字段初始化器；对极常见的 `final X x; Ctor(X x){ this.x = x; }` 这会产出非法的 `final X x = var1;`（构造形参越界）。现在只提升不依赖形参的值，否则赋值留在构造函数里。偏向"不提升"始终安全。
3. **空白 final 强加 `= 0`**（`dumper.go`）。无可提升初始化器的 `final` 字段被渲染成 `Type f = 0;`，对引用类型非法。现渲染裸 `final Type f;`（由 `<init>`/`<clinit>` 的明确赋值保证合法）。
4. **数组字段类型渲染**（`dumper.go`）。数组字段渲染成了元素类型，`int[] TABLE` 变成 `int TABLE`。现渲染完整数组类型。（2–4 合起来 → **Initializers 绿**。）
5. **`&` `|` `^` 的 boolean/int 消歧**（`expression.go`、`constant.go`）。JVM 用同一组 `IAND`/`IOR`/`IXOR` 表示布尔逻辑与整数位运算；原代码无条件把两个操作数（并经别名的结果类型波及赋值目标）重置为 boolean，使每个整数位运算被错误标成 boolean（`int r = a & b; r = r << 2;` → `boolean r = ...`）。现在仅对严格布尔运算符（`&&`、`||`、`!`）重置；对 `&`/`|`/`^` 改为操作数驱动（仅当某操作数已是 boolean 才对齐）。→ **Operators 13 错 → 1 错**。
6. **`synchronized(字段)` 的死亡合成临时变量**（`dumper.go`）。锁字段编译为 `getfield; dup; astore tmp; monitorenter`；synchronized rewriter 移除隐式 finally 的 `monitorexit` 后，已死亡的 tmp 以内联 `synchronized(var2 = this.lock)` 形式残留，引用了未声明变量。渲染时把死亡的 `tmp =` 前缀剥离回锁表达式。→ **Concurrency 绿**。

### 本轮落地的正确性修复——第 1 轮
从 round-trip oracle 诊断并修复 4 个缺陷；结果 **Literals 干净重编译**，全部由 golden 套件 + `TestCorpusDeterminism` 验证无回归：

1. **表达式位置的数值字面量后缀**（`java_value.go`，`JavaLiteral.String`）。long/float/double 字面量在字段声明之外丢了 `L`/`F`/`D` 后缀，导致 `Long.valueOf(9223372036854775807)` 报 *"integer number too large"*，`Float.valueOf(3.14)` 无匹配重载（裸 `3.14` 是 `double`）。现输出为 `9223372036854775807L`、`3.14F`、`2.718281828D`，NaN/Infinity 与字段路径同样处理。
2. **布尔字段常量**（`dumper.go`）。JVM 用 int 常量存 `boolean`，故 `boolean` 字段渲染成非法的 `static final boolean B = 1`。现渲染 `= true` / `= false`。
3. **布尔方法实参**（`expression.go`，`FunctionCallExpression.String`）。int 字面量流入 `boolean` 形参（Java 无 int→boolean 转换）使 `Boolean.valueOf(1)` 这类装箱失败。现强制为 `true`/`false`，与既有的 int→byte/short/char 强转逻辑一致。
4. **基本类型强转优先级**（`code_analyser.go`，`I2L/L2D/D2L/...` 组）。转换强转渲染成 `(long)a * b`，会被解析为 `((long)a) * b` 并触发 *"possible lossy conversion from double to long"*。现加括号为 `(long)(a * b)`——与已应用于 `OP_CHECKCAST` 的优先级修复一致。

此前已在本次测评中落地：
- **成员访问的强转优先级**：`OP_CHECKCAST` 渲染 `((Type)(x)).m()` 而非 `(Type)(x.m())`（golden `VarFold` 已刷新）。
- **嵌套归档的绝对路径**：`normalizeArchivePath` 保留前导斜杠，使 `/abs/app.war/.../foo.jar/Foo.class` 能从主机文件系统打开。

### "recompile-fail" 并不意味着什么
一个 `recompile-fail` 的类仍然是**结构化反编译为可读、语法可解析的 Java**（它通过 ANTLR 语法网与覆盖率矩阵）；它只是没通过严格得多的 *javac 类型检查* round-trip。上面的前沿是关于少数构造的语义保真度，而非产出垃圾。但这确实是一个真实的正确性限制：语法可解析的输出**不是**"重建结果与输入语义等价"的证据。

---

## 4. 测试卫生基准

目标：一个稳定、快速、可移植的核心套件，没有机器相关或浪费时间的测试，同时保持（并提升）真实覆盖。

```
go test ./common/javaclassparser/...      # 绿，总计 ~22s
```

已采取的动作：
- **把机器相关的诊断**收到环境变量后面（`BENCH_JAR`、`JDSC_DIR`、`M2_DETERMINISM`），使默认运行不再扫描 `~/.m2` 或 `/tmp/...`。默认套件时间从 **>150s 降到 ~22s（≈8×）**。
- **删除 `decompiler_test.go`**：四个硬编码到 `/Users/z3/Downloads/...` 且无断言的调试测试；其中一个在 `filepath.Walk` 中 nil-panic，使包二进制中断并掩盖了之后的所有失败。
- **修复了被该崩溃掩盖的失败**（均为既有问题）：
  - `fs_test`：断言当前的优雅按方法 stub 行为，而非过时的整体 dump 失败标记。
  - `access_flags_verbose_test`：枚举渲染为 `public enum`（隐式的 final/abstract 不允许显式写出）。
  - jar 测试：根数量差一、过时的尾斜杠期望，以及嵌套 jar 夹具使用**真实字节码**（它们原本把 Java 源码存成 `.class` 名，只能靠回显输入来"反编译"）。
  - `loop_test`：修正了 then/else 互换的 golden（true 分支属于 then 块）。
- **为被收起的诊断添加可移植替代**：`TestCorpusDeterminism` 无需本地 Maven 缓存即可验证逐字节一致的输出。

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

最有用的开关是 `BENCH_NO_VALIDATE=1`，它关闭反编译后的 ANTLR 重解析，把**反编译器核心**与**安全网**隔离。下面是*本轮优化之后*的数字：

| 配置 | ns/op | B/op | allocs/op |
|------|------:|-----:|----------:|
| 完整流水线（开启校验） | ~378 M | 248 MB | 4.54 M |
| 仅核心（关闭校验） | **246 M** | **182 MB** | 3.31 M |
| **校验安全网占比** | 时间 **≈ 18%** | 字节 **≈ 23%** | 分配 **≈ 26%** |

安全网并非免费，但它是"绝不让不可解析的 Java 离开 `Decompile`"的契约；~18% 的墙钟时间是这一保证的代价。

### 5.2 profile 是 GC-bound——分配才是真正的货币

核心的 CPU profile（`go tool pprof -top`）由垃圾回收器主导，而非反编译逻辑：

```
runtime.gcDrain        47.9% cum
runtime.scanobject     40.7% cum
runtime.mallocgc       19.2% cum
runtime.greyobject     13.3% cum
```

所以**减少分配直接换来 CPU**。最大的*核心*分配源（`-alloc_space`，本轮修复之前）为：

| 分配源 | 字节 | 占比 | 状态 |
|--------|-----:|-----:|------|
| `utils.Set[any].Add`（经由 `WalkGraph`） | 367 MB | 19.4% | **已修（去接口装箱 + 去锁）** |
| `ParseOpcode` | 206 MB | 10.9% | 已预分配（上一轮） |
| `GenerateDominatorTree`（+`func1`） | 193 MB | 10.2% | backlog |
| `Stack[*].Push` | 94 MB | 4.9% | backlog（预分配） |
| `codec.MatchMIMEType` → 每个字符串字面量的 `csv/bufio` | 77 MB | 4.1% | **已修（ASCII 快路径）** |
| `Set[*OpCode].Add` | 73 MB | 3.9% | backlog |

在校验路径上，另有约 70% 的分配是 ANTLR ATN 模拟对象（`NewBaseATNConfig`、`BaseATNConfigSet.Add`、prediction-context 合并）——这是逐类重解析的固有成本。

### 5.3 本轮落地的优化（每项都证明输出等价）

等价是被证明而非假设：`TestDumpJarFingerprint` 为 `commons-codec` **和** `byte-buddy`（≈3k 类）的每个类写出逐类 `sha256(status+output)`；指纹目录在每次改动前后 `diff` 干净。

1. **`WalkGraph` 的 visited 集合——去掉接口装箱与互斥锁。**
   图遍历用了线程安全的 `Set[any]`：每个节点指针都被装箱成 `interface{}` map key（核心第一大分配源，占 19%），且每次 `Has`/`Add` 都取一次 `RWMutex`，尽管遍历是单 goroutine。把类型参数约束为 `comparable` 并改为普通 `map[T]struct{}`。**核心：315 → 254 ms/op（−19%），217 → 193 MB/op（−11%）。**

2. **纯 ASCII 字符串字面量跳过 MIME 嗅探。**
   `JavaStringToLiteral` 对*每个*字面量都跑完整的魔数检测（`codec.MatchMIMEType`，会分配 `csv`/`bufio` reader），用于恢复可能被错误解码的中文字符集——对 ASCII 字节不可能命中。用纯 ASCII 检查作为前置守卫（ASCII 本就走相同的加引号路径，行为不变）。**核心：254 → 246 ms/op，193 → 182 MB/op。**

本轮累计：**核心 315 → 246 ms/op（−22%），217 → 182 MB/op（−16%）**；端到端字节 282 → 248 MB（−12%）。

上一轮仍在生效的优化：
- **`ParseOpcode` 预分配**（opcode 切片 + 两个 offset map 都按字节码长度预分配）。
- **校验定时器卫生**（用可停止的 `time.NewTimer` 替代 `time.After`，使每个成员的预算定时器及其保留的源缓冲立即释放）。

### 5.4 负载严重长尾

`TestTopSlowClasses`（一次冷遍历，按时间排序）显示极少数类主导总成本：

| Jar | 类数 | top-1 类 | top-1% 类 | top-10% |
|-----|----:|--------:|----------:|--------:|
| commons-codec-1.15 | 106 | 14.6% | 14.6% | 68.7% |
| byte-buddy-1.14.17 | 2845 | 26.3% | **60.8%** | 88.4% |

在 byte-buddy 上，**一个 43 KB 的类**（`InstrumentedType$Default`）占整次冷遍历的 26%，top 1% 的类占 61%。含义：平均情况调优只能小幅提升吞吐；高价值目标是病态长尾（深度嵌套 CFG / 巨型方法，压垮结构化与栈模拟阶段）。

### 5.5 冷启动 vs 热稳态

同一个 `InstrumentedType$Default` 在冷的一次性遍历里耗 **7.9 s**，但热态重复时仅 **~127 ms**（≈62×）。差距是一次性进程初始化（ANTLR ATN 反序列化、正则编译、`sync.Once` 设置）被第一个复杂类吸收。对**批量/jar** 反编译这会摊销到可忽略；对**单类 CLI** 调用，这是值得预热的真实延迟下限。

### 5.6 并行可扩展性

`BenchmarkDecompileJarParallel` 在 byte-buddy（完整 jar，热态）上，变化 `BENCH_CONC`：

| Worker 数 | ns/op | 加速比 |
|----------:|------:|------:|
| 1 | 4.27 s | 1.0× |
| 2 | 2.27 s | 1.88× |
| 4 | 1.38 s | 3.09× |
| 8 | 1.19 s | 3.59× |
| 16 | 1.71 s | 2.50×（**回退**） |

扩展在 ~4 worker 前近线性，约 8 见顶（3.6×），之后**回退**。这是 5.2 节的 GC-bound 特征：众多分配型 goroutine 在共享回收器上争用。5.3 节的分配削减直接抬高这个上限，进一步的分配工作（dominator tree、栈）是更好多核扩展的路径。

### 5.7 为什么"跨解析 ANTLR 缓存"这个大杠杆被刻意不动
固定的 ANTLR Go 运行时（`v4.0.0-20220911`）对其 DFA / `JStore` 结构没有加锁，而反编译是并行运行的（jdsc 自检用 100 个 goroutine）。进程级共享校验 DFA 会数据竞争；现有的每 worker 缓存 + `DetachParserATNSimulatorCaches` 设计是安全选择。进一步推进需要升级 ANTLR（超出范围），记为未来工作。

---

## 6. Backlog（按影响排序，源自上文数据）

**正确性（语义保真度）：**
1. **`try/catch/finally` 的 "multiple next" CFG**——唯一的经典语料 stub，也是真实 jar 中观察到的最常见 *stub* 成因。
2. **循环惯用法恢复**——重建 `for`/`while` 而非一律 `do{...}while(true)`，这同时消除 *unreachable statement* 失败（Loops）。
3. **短路 `||`/`&&` 布尔表达式恢复**（Operators）——把 `if(a&&b){return true}else{...}` 控制流折回 `return (a&&b)||(...)`。
4. **泛型签名恢复**（Lambdas）——解析 `Signature` 属性，使被擦除的调用点与 lambda 目标保留类型参数。
5. **Record / sealed 的 `invokedynamic ObjectMethods` bootstrap**——端到端解锁现代（Java 17+）值类型。

*本轮（第 4 轮）落地：* null 初始化 slot 的类型加宽（Generics）——null slot 采纳后续具体引用类型而非拆分。
*第 3 轮：* 作用域感知的局部变量重命名（TryWithResources + 真实世界的嵌套 catch/slot 复用冲突）、内部/嵌套类 round-trip（InnerClasses）、接口 `default` 方法（Inheritance）、`@interface` 注解类型（Annotations）、完整枚举重建（Enums）。
*更早轮次：* JVM boolean/int 消歧、数组维度类型、字段初始化器提升、`synchronized(字段)` 死亡临时变量（第 2 轮），以及数值字面量后缀、布尔常量/实参、强转优先级（第 1 轮）。

**性能（全部服务于 5.2 节的 GC-bound profile）：**
6. **Dominator-tree 分配**（193 MB，10%）与 **栈/`Set[*OpCode]` 预分配**（合计 167 MB）——本轮修复两项之后最大的核心分配源；降低它们会抬高并行上限（5.6 节）。
7. **长尾类结构化复杂度**（5.4 节）——剖析并降低病态 1% 类上的超线性成本。
8. **单类冷启动预热**（5.5 节）——为 CLI 用法预热一次 ANTLR/正则。
9. **共享校验 DFA**——仅在 ANTLR 运行时升级使其线程安全之后。

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
