# YAK JAVA 反编译器 - 当前状态与已知核心缺陷

> 语言：[English](./YAK_DECOMPILER_BENCHMARK.md) | **简体中文**
>
> 本文记录 Java 反编译器的**当前状态**及制约它的**核心缺陷**。这是一份状态记录，
> 不是变更流水账：核心缺陷的深度修复在别处单独跟踪与攻克。

入口：`javaclassparser.Decompile([]byte) (string, error)` 以及 Yaklang 封装
`java.Decompile`。取数主机：darwin/arm64，Go 1.22.12。

## 1. 当前状态

| 维度 | 结果 | 状态 |
|------|------|------|
| 合成语料（语法） | 31/31 组，0 stub / 0 语法错误 / 0 panic | GA |
| 合成 round-trip（反编译 -> javac） | 26/26 可评估组干净重编译，0 失败 | GA |
| 硬用例族（switch / 三元 / try-catch / 内部类） | 全部 PASS | GA |
| 确定性 | 多次反编译输出逐字节一致 | GA |
| 安全契约 | 绝不从 `Decompile` panic 逃逸、绝不卡死；降级为带标记 `yak-decompiler:` stub | GA |
| 真实 jar partial（`.m2`，最近快照） | 120 jar / 12000 类：ok=11965、partial=35、syntax=0、err=0、panic=0；全量 `~/.m2` 约 0.4% partial | 未达 GA |

合成语料已完备。剩余缺口完全在真实世界控制流上——少量类仍会降级为带标记、
仍可解析的 stub。降级始终显式（`yak-decompiler:` 标记），绝不产出"貌似正确但实际错误"的代码。

复现绿色子集（无需 `~/.m2`）：

```
go test -run 'TestSyntaxCoverageMatrix|TestRecompileRoundtrip|TestDecompileSyntaxRegression|TestSwitchHardCasesNoCorruption|TestTernaryHardCasesNoCorruption|TestTryCatchHardCasesNoCorruption|TestGAPanicFreeBoundary|TestDecompileCyclicStatementTreeNoCrash|TestCorpusDeterminism' \
  -count=1 ./common/javaclassparser/tests/
```

## 2. 核心缺陷 A：栈模拟没有 CFG 数据流合并

这是绝大多数残留真实 jar partial 背后的**唯一根因**。它**不是**逐类的偶发问题；
第 4 节列出的逐类症状全部是它的下游表现。

**位置。** `common/javaclassparser/decompiler/core/code_analyser.go` 中的
`(*Decompiler).CalcOpcodeStackInfo`。

**问题。** 操作数栈与局部变量模拟用单趟 DFS（`WalkGraph`，`core/utils.go`）遍历
opcode CFG。DFS 既不保证在到达汇合点前已访问其所有前驱，更**不会在控制流汇合点
合并各前驱的变量表 / 操作数栈**。于是在汇合点（或任何前驱尚未模拟就被访问的节点）
处，模拟状态是不完整的。

**具体失败链**（fastjson2 `TypeUtils.doubleValue`，`panic_nilref_typeutils.class`）：

1. 某个 `*load N` 读取局部槽 N，而该槽的定义只通过一条尚未模拟的前驱边到达此处，
   于是 `GetVar(N)` 返回 nil。
2. `loadVarBySlot` 把这个 nil `*JavaRef` 当作 `varUserMap` 的 key 注册进去。
3. 变量折叠阶段因 `nil ref key in varUserMap` 中止；`parser.go` 抑制该错误，
   用未折叠的（破损的）图继续。
4. 下游改写器（`ScanCoreInfo` 等）随后解引用 nil 分支节点，整方法降级为 stub。

**为什么现有代码只是掩盖。** 已落地的几处守卫（`CalcOpcodeStackInfo` 的空 if-merge
源容错、`parser.go` 的 nil-key 抑制、`statement_wrap.go` 各消费点的 nil 守卫）
全都是在给这同一个缺口的症状打补丁。直接在"未初始化槽的 load 处合成可验证 live 的
局部变量"做局部修复，确实能消除本类的 stub，但它会改动共享的模拟状态，从而**回归**
其它类（switch 操作数栈重建：`loopSwitchTail`、`doubleToBigInt`）——这反证了真正的
修复必须在数据流层面，而非任何单个消费点。

**真正的修复（在别处跟踪）。** 把 `CalcOpcodeStackInfo` 改造成正规数据流 pass：
以逆后序 + worklist 处理节点，并在每个汇合点对各前驱的变量表 / 操作数栈做**合并**
直到定点（回边显式处理）。这是反编译器核心级改动，会影响每个类的逐字节输出，
因此落地前必须用全量 `javaclassparser` 语料回归验证。

## 3. 核心缺陷 B：结构化会产出自环 / 共享容器

**位置。** 改写器结构化阶段（`decompiler/rewriter`，循环 / if 结构化），在
`AssertStatementsAcyclic`（`rewrite_var.go`）处暴露。

**问题。** 在某些真实类上，结构化阶段会产出自引用容器（一个 `IfStatement`，其自身
体内传递性地包含它自己），或把循环后的容器同时挂在循环尾部与嵌套 `do-while` 体内
（双重挂接）。自环会驱动所有递归树遍历器（`FoldAssertionGuards`、`rewriteVar`、
`Statement.String` 等）进入 Go **不可恢复**的 `fatal error: stack overflow`。

**当前的兜底。** `AssertStatementsAcyclic` 在任何递归 pass 之前以迭代方式
（自带显式栈）运行，并区分真实环（节点在当前 DFS 路径上 -> panic -> 干净 stub）
与有限共享 DAG（节点在路径外被访问过 -> 可安全跳过）。因此真实环会降级为带标记
stub，而非崩溃整个进程（zxing Aztec `Encoder.encode`，`cyclic_if_tree.class`）。

**真正的修复（在别处跟踪）。** 对循环 / if 结构化动刀，使其绝不双重挂接节点、
并把循环后代码挡在循环体之外，从源头消除自环 / 共享容器，而非降级为 stub。

## 4. 残留真实 jar partial 家族（全部是第 2/3 节的下游）

1. **variable-fold nil ref key** —— 第 2 节（fastjson2 `TypeUtils.doubleValue`）。
2. **自环 / 共享容器** —— 第 3 节（druid `TDDLHint`、jackson
   `UTF8DataInputJsonParser`、zxing `Encoder`）。
3. **multiple next** —— 结构化后节点仍保留两条 `Next` 边，与不同循环层级的
   `break`/`continue` 相关（fastjson2 `seekLine`）。
4. **post-decompile syntax** —— 已结构化 `IfStatement` 体内未被递归进入的残留
   `ConditionStatement`，以及循环出口归属错误。

四者最终都归结为不完整的 CFG 数据流（第 2 节）与循环 / 合流值结构化（第 3 节）
——这是反编译器最难的结构性问题。每一项当前都有安全网兜住，故安全契约成立。

## 5. 本分支已落地的安全修复

以下消除了两类硬故障（都严格比 stub 更糟），它们在真实类进入结构化阶段后才暴露。
这些修复本身并不清除 partial：

- **`mergeIf` 收敛**（`statement_wrap.go`）：nil 分支守卫在"未改动图就 return"
  之前就把定点循环的 `result=true` 置上，导致 `MergeIf()` 永远重新发现同一对
  不可合并节点（死循环 / 卡死）。现在只有发生真实合并后才置 `result`。修复
  `panic_nilref_typeutils` 的卡死。
- **`AssertStatementsAcyclic` 检查顺序**（`rewrite_var.go`）：原先先查 `visited`
  再查 `ancestors`，导致真实自环被误判为共享 DAG 而漏过，最终 fatal stack
  overflow。现在先查 `ancestors`。修复 `cyclic_if_tree` 的崩溃。

## 6. 性能画像（当前刻画）

- **GC 主导。** 核心反编译 ~215 ms / ~161 MB 累计堆 / 106 类的 jar；反编译后的
  ANTLR 重解析（语法安全网）额外增加运行时 ~+60%、字节 ~+42%。
- **长尾主导。** byte-buddy（2845 类）上单个 43 KB 类占一次冷遍历的 26%、top 1%
  类占 61%；高价值目标是病态长尾而非平均情况。
- **并行扩展。** ~4 worker 前近线性，约 8 见顶（3.6×），之后 GC 回退；抬高上限
  需要削减分配。
- **跨解析 ANTLR 缓存**刻意不共享：固定的 ANTLR Go 运行时对 DFA/`JStore` 无加锁，
  而反编译并行运行，共享校验 DFA 会数据竞争（需升级 ANTLR）。

## 7. 复现命令

```
# 合成覆盖 + round-trip（无需 ~/.m2）
go test -run 'TestSyntaxCoverageMatrix|TestRecompileRoundtrip' -v ./common/javaclassparser/tests/

# panic / 卡死 / 崩溃边界（第 2/3 节的类，必须干净降级）
go test -run 'TestGAPanicFreeBoundary|TestDecompileCyclicStatementTreeNoCrash' -count=1 ./common/javaclassparser/tests/

# 确定性（可移植，无需 Maven 缓存）
go test -run TestCorpusDeterminism -v ./common/javaclassparser/tests/

# 真实 jar stub 原因归因（需要 ~/.m2）
STUB_REASONS=1 M2_MAX_JARS=120 M2_MAX_CLASSES=12000 go test -run TestM2StubReasons -v ./common/javaclassparser/tests/
```
