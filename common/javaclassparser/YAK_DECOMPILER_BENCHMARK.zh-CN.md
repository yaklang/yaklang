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
| 语法安全（解析或降级） | 31/31 语料组产出**语法可解析的 Java**；0 语法错误、0 硬错误、0 panic | `TestSyntaxCoverageMatrix` |
| 重建覆盖率（无 stub） | 29/31 组产出**未降级输出**（无 stub）；2 个预览组（Records、SealedVar）隔离出具体缺口 | `TestSyntaxCoverageMatrix` |
| 正确性（javac round-trip） | **24/26** 个可评估语料干净重编译（起始为 4/13）；经典语料现已**零 stub**；四个内部类/嵌套类组全部可重编译；专用边界、数值边界、字段/数组与嵌套控制流语料均已纳入门禁 | `TestRecompileRoundtrip` |
| 确定性 | 多次反编译逐字节一致；性能改动通过逐类 sha256 指纹证明输出等价 | `TestCorpusDeterminism`、`TestDumpJarFingerprint` |
| 测试套件 | 绿且快：`./...` ≈ 22s，从 150s 以上降下来（**至少 6.8 倍**），无机器相关依赖 | `go test ./common/javaclassparser/...` |
| 分配开销 | 核心 **≈215 ms** 且 **≈161 MB 累计堆分配** / 106 类的 jar（自 ≈246 ms / ≈182 MB 降低）；反编译后的 ANTLR 重解析相对 core-only 增加运行时 ≈ +60%、字节 ≈ +42% | `BenchmarkDecompileJar` |
| 可扩展性 | ~8 worker 前近线性（3.6×），之后出现 **GC 瓶颈回退** | `BenchmarkDecompileJarParallel` |

反编译器的**安全保证成立**：对语料中的每一个输入，要么重建出方法，要么把它降级为带标记、仍可解析的 stub（`yak-decompiler:` 标记），绝不输出不可解析的 Java，也绝不从 `Decompile` 中 panic 逃逸。

### Round-trip 正确性细节

在 26 个可进入严格 `javac` round-trip 验证的经典语料组中（22 个单类组 + 4 个多类内部/嵌套类组）：

- **24 个成功重编译**：Annotations、Arrays、Boundary、CastsInstanceof、ComplexExpressions、ComplexMisc、Concurrency、ControlFlow、ControlFlowEdge、Enums、Exceptions、ExceptionsComplex、FieldsAndArrays、Generics、Inheritance、Initializers、InnerClasses、Literals、Loops、NestedControlFlow、NumericEdge、Strings、Switches、TryWithResources。
- **2 个暴露具体的语义/类型缺陷**：Lambdas（lambda 形参作用域冲突 + 泛型擦除）、Operators（短路布尔 `||` 返回值恢复）。
- **经典语料 0 stub**：每个方法都结构化为真实 Java。

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

### 经典语料（Java 8 字节码）——26 组
```
ok=26  stub=0  syntax=0  error=0  panic=0
```
- 原先的 `STUB`（**Exceptions** → `tryCatchFinally(int[],int)` 失败于 `ParseBytesCode failed: multiple next`）已修复；见第 3 节第 5 轮。
- 第 7 轮新增两个边界条件组（**Boundary**、**ControlFlowEdge**）以加固门禁；二者均完整重建（见第 3 节第 7 轮）。
- 第 8 轮新增三个复杂形态组（**ComplexExpressions**、**ComplexMisc**、**ExceptionsComplex**）；三者均完整重建，并为此修复了两个正确性缺陷（见第 3 节第 8 轮）。
- 本轮（第 9 轮）新增三个组（**NumericEdge**、**FieldsAndArrays**、**NestedControlFlow**）；三者均完整重建，并为此修复了一个正确性缺陷（见第 3 节第 9 轮）。

### 现代语料（Java 17 字节码）——5 组
```
ok=3  stub=2  syntax=0  error=0  panic=0
```
- `STUB` 组 **Records** 与 **SealedVar** 仅在编译器合成的 `toString()/hashCode()/equals()` 上失败，报 `ParseBytesCode failed: call bootstrap method error`（即 `invokedynamic` 的 `ObjectMethods` bootstrap）。

### 覆盖率结论
经典语料现已零 stub；剩余唯一覆盖缺口在现代语料，且被精确隔离：
1. **Record / sealed 的 `invokedynamic ObjectMethods` bootstrap**——自动生成的值类型方法尚未合成。

（原先的 `try/catch/finally` "multiple next" 缺口已闭合——见第 3 节第 5 轮。）

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
recompile-ok:  21  (Annotations, Arrays, Boundary, CastsInstanceof, ComplexExpressions,
                    ComplexMisc, Concurrency, ControlFlow, ControlFlowEdge, Enums,
                    Exceptions, ExceptionsComplex, Generics, Inheritance, Initializers,
                    InnerClasses, Literals, Loops, Strings, Switches, TryWithResources)
recompile-fail: 2  (Lambdas, Operators)
stub:          0
dec-err:       0
multiclass:    0   (现已一起编译，不再跳过)
```

剩下 2 个重编译失败是可执行的正确性前沿。下面每条根因都通过阅读**完整**的 `javac` 诊断确认（用 `RC_VERBOSE=1` 输出反编译源码 + 每个分类的全部错误），而非臆测：

| 分类 | 确切的 javac 错误 | 已确认根因 | 难度 |
|------|-------------------|------------|------|
| Operators | `missing return statement`（1 个错误，原为 13） | `(a && b) \|\| (c)` 作为布尔值返回时是一个 **DAG** 而非树：两个 true 臂汇聚到*同一个* `iconst_1` 叶子，于是 `CalcMergeOpcode` 把外层 `&&` 条件归属到这个常量叶子（而非 `ireturn` 值合并点）。外层条件因而被排除在值折叠之外，泄漏成一个独立的 `if (a&&b){}`——空 then 分支且无尾随 return。已通过对合并检测器插桩（`OPDBG`）确认 | 难（短路-DAG 值恢复，在 `CalcMergeOpcode`/合并器中） |
| Lambdas | `variable v already defined` + lambda 形参类型不兼容 + 非法方法引用（5 个错误） | 两个独立根因：**(A)** lambda 体用*外层*方法的 `VariableId` 转储，故其形参（`var2,var3`）与外层共享命名空间，并与 lambda 自身的赋值目标冲突（`BiFunction var2 = (Integer var2,…)`）；**(B)** 泛型被擦除——没有 `LocalVariableTypeTable`，目标渲染成裸 `BiFunction`/`List`/`Function`，显式 `Integer` 形参与 `Integer::intValue` 引用便无法对裸类型通过类型检查。类型实参只能从合成 `lambda$…` 方法自身的签名中恢复 | 难（独立 lambda 形参作用域 + 泛型 `Signature` 恢复） |

通过的分类由 `recompileGateBaseline` 钉死，因此任何破坏 18 个绿色分类的回退都会让 CI 失败；其余作为 backlog 跟踪。

> **已知语义限制（非重编译失败）。** `Loops.labeled` 能干净重编译，但当 `continue <label>`
> 的目标是外层 `for` 循环的*自增*、且该自增节点与循环的自然退出边共享时，该 `continue` 当前会被丢弃：
> do{...}while(true) 模型只能把共享的自增语句（`i++`）放在某一条后继路径上，另一条路径
> （`continue outer` 分支）便渲染成空的 `if` 体。这忠实到足以编译通过，但对这一特定的
> labeled-continue 惯用法可能在运行期产生分歧。已在 backlog 的"循环惯用法恢复"下跟踪；
> 循环语义 round-trip 电池（`TestLoopSemanticsRoundTrip`，执行并比对指纹）覆盖所有非 labeled
> 形态且全部通过。

### 本轮落地的正确性修复 + 语料扩充——第 9 轮（数值/字段/嵌套）
再新增三个语料并**纳入门禁**，使严格 round-trip 达到 **24/26**、经典覆盖率矩阵达到 **26/26（零 stub）**。新语料暴露的一个真实正确性缺陷被修复；另有两个更深层的结构化缺口被隔离并显式跟踪。

- **NumericEdge**——整型溢出环绕、达到与超过类型位宽的移位计数（`<<32`、`>>>33`）、`int/long/byte/short/char` 混合提升、带隐式窄化的复合赋值、十六/二/八进制与下划线字面量、`char` 算术，以及 `float`/`double` 特殊值（`NaN`、`+/-Infinity`）。一次性重编译通过。
- **FieldsAndArrays**——实例/静态字段、对**字段数组元素**的复合赋值与前后自增（`this.buf[i] *= 2`）、多维与交错数组、数组初始化器。暴露了下述修复 1。
- **NestedControlFlow**——三层循环嵌套、跨两层以上的带标签 `break`/`continue`、`while` 内嵌 `switch`（派发 + `break`/`return` 臂）、深层 `if/else-if` 链、`break`/`continue` 混合。

**修复 1——`dup2` 的 ref-fold 回调在两个被复制槽位间被共享（`core/code_analyser.go`）。** 对字段数组元素的复合赋值（`this.buf[i] *= 2`）编译为 `getfield;iload;dup2;iaload;…;iastore`：`dup2` 复制 `(arrayref, index)` 这一对，使同一数组槽位既读又写。反编译器会把非平凡的数组引用折叠进临时变量（`var t = this.buf; t[i] = t[i] * 2`），但 `dup2` 处理器为整对只保留了**一个** ref-fold 回调，且被覆盖为最后一次转换的值。于是较深值的折叠规则（把*数组引用*折进临时变量）也作用到了较浅的*索引*上，错误地产出 `int t = i; t[i] = t[i] * 2`（把 `int` 当数组下标——`javac` 拒绝）。修复：每个被复制的槽位现在各自携带**自己的**回调（`dup2Item{val, addUser}`），并把 `checkAndConvertRef` 实际转换出的值按 opcode 记录（`dupConvertedRefValue`），使临时变量赋值处理器把临时变量绑定到真实的数组引用，而非 `stackConsumed[i]`（对 `dup2` 而言因索引先于数组引用出栈而错位）。已由完整 `./common/javaclassparser/...` 套件加 `TestCorpusDeterminism`/`TestDecompileDeterminism` 验证。

**已跟踪（尚未纳入门禁）。** 构建本轮语料时隔离出两个更深层的结构化缺口，作为显式 backlog 项而非静默规避：（1）从 `switch` case 内部跳向**外层循环**的 `continue`/`break` 会产生第二个 switch 出口边，`SwitchRewriter1` 目前不建模（它断言只有单个 end 节点）；（2）**3 维及以上数组形参**的类型推断会给声明形参类型多加一维（`int[][][] cube` 渲染为 `int[][][][]`），导致其元素与 `int` 比较时类型不匹配。本轮 `NestedControlFlow` 改用 2 维数组与"循环内嵌、非 `continue`"的 switch，以保持在当前正确性边界内。

### 本轮落地的正确性修复 + 语料扩充——第 8 轮（复杂形态）
第 8 轮新增三个复杂形态语料并**纳入门禁**，使严格 round-trip 达到 **21/23**、经典覆盖率矩阵达到 **23/23（零 stub）**。新语料暴露的两个真实正确性缺陷被修复（二者在真实代码中都很常见，因此收益远超语料本身）：

- **ComplexExpressions**——1 维/2 维数组初始化、`int/long/float/double` 混合提升、`StringBuilder` 与 `+` 字符串拼接、递归（阶乘/斐波那契）、可变参数、增强 `for`，以及**深层右倾链式三元**（`a?:b?:c?:...`）。
- **ExceptionsComplex**——嵌套 `try/catch/finally`、单资源与多资源 try-with-resources、重抛、`return` 后的 `finally`、带 `finally` 的多 catch 链。一次性重编译通过。
- **ComplexMisc**——从嵌套循环跳出的带标签 `break`/`continue`、`StringBuilder` 流式链、**default 在中间的 switch**、`do/while`、作为方法实参的三元、`instanceof`+强转派发链。

**修复 1——链式三元 condition 被错误合并（`rewriter/statement_wrap.go`、`core/code_analyser.go`）。** 深层右倾三元（`x<0?-1:x==0?0:x<10?1:...`）会降级为 stub 并报 *"empty stack slot leaked into method body"*。结构化组合器其实已正确构建出值树（`-1,0,1,...` 右倾嵌套），但随后 `MergeIf` 把各臂的**条件**节点折叠成了一个短路 `||`（`(x<0)||(x==0)||(x<10)`），只触发了最外层条件回调，使内层三元的 `Condition` 槽位为空（渲染为空槽占位符，从而触发方法降级）。根因：当某个三元臂的叶子值被抽取后，各臂条件都汇聚到 merge 节点，*看起来*像短路链。修复：为供给**独立嵌套三元臂**的条件 opcode 打上 `TernaryChainArm` 标记（在组合器的嵌套三元分支与结构化探测提交处设置），并传播到其 `ConditionStatement`；`MergeIf` 拒绝把已标记的条件折叠进 `&&`/`||`。真正的短路条件（全部供给**同一个**三元条件）不打标记，合并行为与之前完全一致——已由 `TestDecompiler/LogicalOperation*` 与 `empty_slot_stub` 仍通过验证。

**修复 2——switch case 变量作用域提升（`rewriter/rewrite_var.go`）。** 极常见的写法 `int r; switch(x){ case 1: r=...; break; ... } return r;` 无法重编译，报 *"cannot find symbol: variable r"*：反编译器把 `int r = ...` 放进了第一个 case 体内，于是 switch 之后的读取越界（switch 体是单一块，但困在某个 case 里的声明在 switch 之后不可见）。修复：新增一个后处理 pass（`hoistSwitchDeclarations`，在声明放置**之后**运行，使其 `IsFirst` 决策已最终确定）检测"声明于 case 内**且**在 switch 之后被读取"的局部，把 case 内的 `T r = ...` 降级为 `r = ...`，并在 switch 之前插入单条 `T r;`。"switch 之后被读取"的触发判定是精确的（对 switch 之后语句做按变量名的引用扫描），因此仅在后续 case 中使用、本就合法的变量保持不变（`SwitchTest` golden 未变）。提升只会扩大作用域，绝不删除或破坏可达代码。

两处修复均为外科手术式改动，已由完整 `./common/javaclassparser/...` 套件、`TestCorpusDeterminism` 与 `TestDecompileDeterminism` 验证。

### 本轮落地的语料扩充——第 7 轮（边界条件语料）
新增两个专用边界语料并**纳入门禁**，使严格 round-trip 达到 **18/20**、经典覆盖率矩阵达到 **20/20（零 stub）**：

- **Boundary**——数值极值（`Integer.MIN/MAX_VALUE`、`Long.MIN/MAX_VALUE`）、有符号整数除法/取模、窄化强转链（`double→long→int→short→byte`）、嵌套三元、`long` 全宽位运算（`& | ^ << >> >>> ~`）、`char` 算术、多维数组遍历、对数组元素的复合赋值。
- **ControlFlowEdge**——switch 穿透、`String` switch、稀疏（lookup）vs 稠密（table）switch、嵌套循环的普通 `break`/`continue`、**作为条件**使用的短路布尔（这些能正确重建——Operators 的缺口仅限于*被返回*的 `(a&&b)||c`）、链式 `if/else-if` 派发、`while(true)`+break。

二者均一次性重编译通过，证明操作数定型、字面量渲染、优先级、switch-case 映射与 CFG 结构化在这些边界上是健壮的。它们现已成为硬回归门禁。已由完整包套件与 `TestCorpusDeterminism` 验证。

### 本轮落地的正确性修复——第 6 轮（不可达语句裁剪）
**Loops** 转为干净重编译，使 round-trip 达到 **16/18**。由于结构化阶段把每个循环都降为
`do{...}while(true)`，一条回边 `continue;` 可能被生成在一个永不顺序穿过的内层区域*之后*
（内层无限循环只能经 `return` 或对外层的带标签 `continue` 退出）。`javac` 会把这条尾随的
`continue;` 判为 *unreachable statement*。新增的结构化后处理 pass
（`rewriter/PruneUnreachableStatements`，在 `parser.go` 中 `RewriteVar` 之后接入）删除同一块内
跟在*终止*语句之后的语句。终止判定是 JLS "无法正常完成" 规则的一个刻意**严格子集**
（`return`/`throw`/`break`/`continue`、两臂都终止的 `if/else`，以及无逃逸 `break` 的无限
`while(true)`/`do{...}while(true)`）；因为是子集，它只会删除 `javac` 同样会拒绝的代码，故任何
已能重编译的类保持逐字节不变、不丢任何可达代码。`subtreeHasBreak` 辅助对"该循环可能顺序穿出"
做过近似（任何 break 类标记都会抑制裁剪），只会*少*删、绝不*多*删。已由 golden 套件、
`TestCorpusDeterminism`、`TestLoopSemanticsRoundTrip` 及完整包套件验证无回归。

### 本轮落地的正确性修复——第 5 轮（try/catch/finally 处理器分组）
**Exceptions** 从语料里最后一个 stub 转为干净重编译，经典语料现已**零 stub**。`javac` 把 `finally` 脱糖为一个合成的 catch-all（`any`，catch type 0）处理器——`astore t; <finally>; aload t; athrow`——它同时保护 try 区域与每个真实 catch，并把 finally 体额外内联到每条正常退出路径上。当一个真实 catch 与该 catch-all **共享同一 try 区域的 end index** 时，try 节点构造把"按 end index 分组的处理器"槽位**覆盖**而非追加，丢掉了真实 catch；被丢的处理器残留为 pre-try 语句节点的悬挂后继，使其有两个后继，被线性结构化以 `multiple next` 拒绝。现在构造器把共享同一 end index 的所有处理器**追加**进一个分组（保留原始边的重数：多重 catch `A | B` 共享一个 handler PC、因而在 node.Next 里是两条相同的边，必须保留重数才能把两条边都改接）。重建出的方法语义忠实——finally 体出现在正常路径、catch 路径与 catch-all（`catch (Throwable t) { <finally>; throw t; }`）上，与字节码执行完全一致——且可重编译。在真实 jar 上价值很高：gson 的 stub 标记从 38 降到 18，无新增 error 或 panic。已由 golden、`TestCorpusDeterminism` 及真实 jar 的 ok/err/panic/stub 计数验证无回归（多重 catch `Exceptions.multiCatch` 仍可重编译）。

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

所以**减少分配直接换来 CPU**。最大的*核心*分配源（`-alloc_space`，本轮修复之前）为：

| 分配源 | 字节 | 占比 | 状态 |
|--------|-----:|-----:|------|
| `utils.Set[any].Add`（经由 `WalkGraph`） | 367 MB | 19.4% | **已修（去接口装箱 + 去锁）** |
| `WalkGraph` 的栈/visited（每次遍历） | — | — | **已修（链表栈 → 切片栈，见下方轮次）** |
| `ParseOpcode` | 206 MB | 10.9% | 已预分配（上一轮） |
| `GenerateDominatorTree`（+`func1`） | 193 MB | 10.2% | **已修（跨 sweep 复用 scratch 位集）** |
| `Stack[*].Push` | 94 MB | 4.9% | **已修（`WalkGraph` 改用切片栈）** |
| `codec.MatchMIMEType` → 每个字符串字面量的 `csv/bufio` | 77 MB | 4.1% | **已修（ASCII 快路径）** |
| `Set[*OpCode].Add` | 73 MB | 3.9% | **已修（`CalcMergeOpcode` 改用普通 map）** |
| `fixJavaStringEscapes` 每个字符串字面量重编译 3 个正则 | ~270 MB 累计 | — | **已修（包级预编译正则）** |

在校验路径上，绝大部分分配是 ANTLR ATN 模拟对象（`NewBaseATNConfig`、`BaseATNConfigSet.Add`、prediction-context 合并）——这是逐类重解析的固有成本，不改 ANTLR 运行时无法消除。

### 5.3 本轮落地的优化（每项都证明输出等价）

等价是被证明而非假设：`TestDumpJarFingerprint` 为某个 jar 的每个类写出逐类 `sha256(status+output)`；指纹目录在每次改动前后 `diff` 干净。本轮在 `commons-codec`（106 类）**和** `hazelcast-5.1.7`（≈数千类）上重跑——两者 diff 均干净。

**最新一轮分配/CPU 优化（针对 §5.2 的 GC-bound profile）。** 五项输出等价的改动，在 `commons-codec` 上测量（核心 `-benchtime=30x` / 完整 `20x`）：

1. **`WalkGraph` 的 DFS 栈：链表 → 切片。** `utils.Stack[T]` 每次 `Push` 都堆分配一个 node 结构；由于几乎每次 CFG/opcode 遍历都会用到，它占核心字节约 6%。改用普通 `[]T`、保持相同的 LIFO 弹出顺序，摊销增长（遍历顺序一致 ⇒ 输出一致）。
2. **`GenerateDominatorTree`：复用单个 scratch 位集。** 不动点循环原本为每节点每 sweep 新分配一个 `netSet`；现改为复用单个 scratch，仅在发生变化时拷回 `dom[i]` 既有底层数组。语义不变（仍由 `TestGenerateDominatorTreeEquivalence` 的 4000 个随机 CFG 守护）。
3. **`CalcMergeOpcode`：去掉互斥 `Set`、复用 `next` 缓冲。** 用普通 map 替换 `utils.Set[*OpCode]`（带互斥锁，占核心字节约 4.6%），并跨访问复用单个 `next` 过滤切片（安全：`WalkGraph` 会把返回切片拷入自己的栈，从不持有它）。
4. **`fixJavaStringEscapes`：3 个正则只编译一次。** 它对*每个反编译出的字符串字面量*都新建三个 `RegexpWrapper`——每次都重编译模式（累计 ~270 MB）。提升为包级变量、只编译一次（`*regexp.Regexp` 并发安全，共享 wrapper 也服务并行反编译）。
5. **`DumpClass.assemble`：用 `strings.Builder` 替代 `attrs += …`。** 含大量方法的类原本触发 O(n²) 字符串拼接；builder 为 O(n) 且产出相同字节。
6. **`ScanJmp` / `DropUnreachableOpcode`：普通 map + 去掉一次逐节点拷贝。** 两者都在单 goroutine 遍历里用了互斥 `utils.Set[*OpCode]`（→ 普通 map），且 `DropUnreachableOpcode` 每访问一个节点都新分配一份 `code.Target` 的 `[]*OpCode` 拷贝——现在直接返回 `code.Target`（`WalkGraph` 会拷入自己的栈，从不修改/持有它）。

`commons-codec` 上的结果：**核心 246 → 222 ms/op（−10%），182 → 167 MB/op（−8%），3.31 → 2.34 M allocs/op（−31%）**；**完整流水线 378 → 351 ms/op（−7%），248 → 234 MB/op（−6%），4.54 → 3.59 M allocs/op（−21%）**。两种配置三项指标全部改善，且与优化前基线的指纹 diff 证明输出逐字节一致。（曾原型化一个 OpCode chunk 竞技场分配器并**否决**：它降低了 malloc 次数，但对许多小方法过度分配，使字节回退 +7%——为小幅 CPU 收益换取内存损失，项目不接受。）

**后续 map 预分配一轮（仍在生效）。** 对上一轮后的核心再做 profiling，发现 `CalcOpcodeStackInfo`
内两个最大的 per-opcode 分配点其实是普通 map 的*扩容*：`opcodeToSim`
（`map[*OpCode]*StackSimulationImpl`，约 104 MB）与 `nodeToVarScope`
（`map[*OpCode]*Scope`，约 102 MB）。两者每个 opcode 恰好写入一条，而此处 opcode 数量
（`len(d.opCodes)`）已知，因此都改为 `make(…, len(d.opCodes))` 预分配。预分配只改变容量
（Go map 迭代顺序本就随机），输出不变。结果：**核心 222 → 218 ms/op，167 → 163 MB/op**，
分配次数持平；**完整流水线 351 → 344 ms/op，234 → 231 MB/op**。与优化前基线的指纹 diff
仍逐字节一致。

**支配树结果构建一轮（仍在生效）。** 下一个最大的分配点是 `GenerateDominatorTree` 的收尾阶段：
`dominatorMap[idom] = append(...)` 对每个 idom 的子节点切片逐步扩容（约 122 MB），外加每个 idom
一次 `sort.Slice` 闭包（约 36 MB）。现在该循环拆成两遍——计数遍记录每个节点的直接支配者 id 以及每个
idom 收集多少子节点，使第二遍能按确切的最终容量分配每个子切片（结果 map 也按去重后的 idom 数预分配）。
显式排序被移除：子节点按 node-id 递增顺序追加，而 `nodeToId[nodes[i]] == i` 且 id 唯一，所以这种顺序
填充已得到与原排序完全一致的顺序（对顺序敏感的 `TestGenerateDominatorTreeEquivalence` 在 4000 个随机
CFG 上仍全部通过）。结果：**核心 218 → 215 ms/op，163 → 161 MB/op，分配 2.33 → 2.28 M**；
**完整流水线 344 → 343 ms/op，231 → 229 MB/op，3.59 → 3.54 M**。指纹仍逐字节一致。

**上一轮分配/CPU 优化（仍在生效）：**

1. **`WalkGraph` 的 visited 集合——去掉接口装箱与互斥锁。**
   图遍历用了线程安全的 `Set[any]`：每个节点指针都被装箱成 `interface{}` map key（核心第一大分配源，占 19%），且每次 `Has`/`Add` 都取一次 `RWMutex`，尽管遍历是单 goroutine。把类型参数约束为 `comparable` 并改为普通 `map[T]struct{}`。**核心：315 → 254 ms/op（−19%），217 → 193 MB/op（−11%）。**

2. **纯 ASCII 字符串字面量跳过 MIME 嗅探。**
   `JavaStringToLiteral` 对*每个*字面量都跑完整的魔数检测（`codec.MatchMIMEType`，会分配 `csv`/`bufio` reader），用于恢复可能被错误解码的中文字符集——对 ASCII 字节不可能命中。用纯 ASCII 检查作为前置守卫（ASCII 本就走相同的加引号路径，行为不变）。**核心：254 → 246 ms/op，193 → 182 MB/op。**

那一轮累计：**核心 315 → 246 ms/op（−22%），217 → 182 MB/op（−16%）**；端到端字节 282 → 248 MB（−12%）。最新几轮（上文）进一步推进到核心 215 ms / 161 MB。

更早仍在生效的优化：
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
1. **短路 `||`/`&&` 布尔表达式恢复**（Operators）——当布尔 `(a&&b)||(c)` 被*返回/存储*（而非用作 `if` 条件）时，两个 true 臂共享一个 `iconst_1` 叶子，于是 `CalcMergeOpcode` 把外层条件误归属到该常量叶子，使其泄漏成一个游离 `if`。需让合并检测器（或 `CalcMergeOpcode`）透过共享的布尔叶子看到下游值合并点，从而把整个表达式折叠为 `return (a&&b)||(c)`。
2. **泛型签名 + lambda 作用域恢复**（Lambdas）——(a) 在独立的 `VariableId` 命名空间中转储每个 lambda 体，使其形参不会与外层作用域或 lambda 自身赋值目标冲突；(b) 从合成 `lambda$…` 方法签名（以及类/字段/方法的 `Signature` 属性）恢复类型实参，使目标渲染为 `BiFunction<Integer,Integer,Integer>` 而非裸 `BiFunction`，从而保持显式 lambda 形参类型与 `Type::method` 引用的类型正确性。
3. **循环惯用法恢复**——重建 `for`/`while` 而非一律 `do{...}while(true)`。*unreachable statement* 失败已被第 6 轮裁剪消除；恢复真正的 `for` 循环还能额外修复 `labeled` 的 `continue <外层自增>` 语义限制（do-while 模型只能把共享自增节点放在一条后继上）。
4. **Record / sealed 的 `invokedynamic ObjectMethods` bootstrap**——端到端解锁现代（Java 17+）值类型。
5. **idiomatic `finally` 折叠**——`try/catch/finally` 的 round-trip 当前已正确（采用忠实的脱糖形式：finally 体重复 + `catch (Throwable)` 重抛，与字节码运行完全一致）。未来可加一个 pass 把它折叠为单个 idiomatic 的 `finally {}` 块以提升可读性。

*本轮（第 8 轮）落地：* 复杂形态语料（ComplexExpressions、ComplexMisc、ExceptionsComplex）新增并纳入门禁——严格 round-trip 现为 **21/23**，经典覆盖率 **23/23** 且零 stub。修复两个真实正确性缺陷：(1) 深层链式三元的各臂条件不再被错误折叠为短路 `||`（不再出现空槽 stub），通过 `MergeIf` 尊重的 `TernaryChainArm` 标记实现；(2) 声明于 switch case 内但在 switch 之后被读取的局部，会被提升到 switch 之前，修复了极常见的 `int r; switch{...} return r;` 写法。
*第 7 轮落地：* 边界条件语料（Boundary、ControlFlowEdge）新增并纳入门禁——严格 round-trip 18/20，经典覆盖率 20/20 且零 stub。
*第 6 轮：* 不可达语句裁剪（Loops）——跟在不顺序穿过的内层区域之后的回边 `continue;` 用 JLS 可达性规则的严格子集删除。
*第 5 轮：* try/catch/finally 处理器分组（Exceptions）——经典语料现已零 stub；真实 jar 的 stub 标记大幅下降（gson 38 → 18）。
*第 4 轮：* null 初始化 slot 的类型加宽（Generics）——null slot 采纳后续具体引用类型而非拆分。
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
