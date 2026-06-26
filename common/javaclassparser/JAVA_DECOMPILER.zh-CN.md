# Yak Java 反编译器

> 语言：[English](./JAVA_DECOMPILER.md) | **简体中文**
>
> 状态：**GA（正式可用）**。快照：2026-06-26，darwin/arm64，Go 1.22.12。

Yak Java 反编译器从 `.class`、`.jar` 字节码重建可读的 Java 源码。它是一个从零
自研的字节码转源码引擎（不依赖 CFR / Procyon / Fernflower 等外部反编译器），既能
接入 Yaklang SSA 流水线，也能作为独立的源码恢复工具使用。

- Go 入口：`javaclassparser.Decompile(classBytes []byte) (string, error)`
- Yaklang 入口：`java.Decompile(sourcePath, destDir)`

---

## 1. 为什么达到 GA

一个反编译器达到 GA 的标准是：在广泛的真实语料上，(a) 永不让宿主进程崩溃；
(b) 永不把无效 Java 当作成功结果返回；(c) 能完整重建绝大多数 class。基于真实测量
的证据如下：

| 维度 | 测量结果（2026-06-26） | 结论 |
|------|------------------------|------|
| 工业语料全量扫描 | 本地 `.m2` 中 1107 个 jar 取样 546 个，共 60000 个 class | **ok=60000 partial=0 syntax=0 err=0** | GA |
| 主流库 | guava(2007) commons-lang3(385) jackson-databind(756) fastjson(179) spring-core(1105) | 全部 `ok`，0 partial / 0 fail | GA |
| 可移植语法语料 | 31 组（26 经典 + 5 现代 Java），重编译 + round-trip | 0 stub、0 语法错误 | GA |
| 真实字节码回归 | 从语料中锁定的 77 个聚焦 `.class` 回归 | 全部干净解析 | GA |
| `javac` 重编译预言机 | 反编译 -> `javac --release 8` 对每个可评估组 | 全部可重编译 | GA |
| 确定性 | 同一 class 多次反编译 | 输出逐字节一致 | GA |
| 安全契约 | `Decompile` 不向外逃逸 panic、不栈溢出；失败降级为带标记 stub | panic-free 边界套件全绿 | GA |

这些数字如何复现（可复现，无语料魔法）：

```bash
# 全量语料成功率扫描（上表 60000 class 那一行）
M2_OUT=bench.txt M2_INDUSTRY=1 M2_MAX_CLASSES=60000 M2_MAX_PER_JAR=400 \
  go test -run TestM2RegressionHarness -count=1 -timeout 30m \
  -v ./common/javaclassparser/tests/

# 单库计时 + 成功率探针（上表主流库那一行）
BENCH_JAR=<guava.jar 的路径> \
  go test -run TestDecompileJarTiming -count=1 ./common/javaclassparser/tests/
```

工业扫描会取样 `~/.m2` 中的每一个 jar（每个 jar 有 `M2_MAX_PER_JAR` 的 class 上限，
避免少数巨型 jar 占满预算），而不是只取字典序最前的若干个，因此覆盖了 Spring、
Tomcat、Netty、Jackson、Guava 等。Maven 缓存是一个动态目标——新依赖可能带来新的
字节码形态——但本快照中被取样的群体是干净的。

---

## 2. 如何使用

### Go

```go
import "github.com/yaklang/yaklang/common/javaclassparser"

// source 为可读 Java；仅当字节码本身格式错误时 err 才非空
source, err := javaclassparser.Decompile(classBytes)
```

### Yaklang

```javascript
// sourcePath：.class 或 .jar（也支持 .war / 嵌套归档）；destDir：输出目录
java.Decompile(sourcePath, destDir)
```

调用后，`destDir` 中每个反编译的 class 对应一个 `.java`，并保留包目录结构。
嵌套归档（jar 套 jar、jar 套 war）会被透明展开。

### 部分输出与 stub 契约

当单个方法体无法被忠实重建（结构分析尚未泛化的某种罕见 CFG 形态）时，反编译器
**不会**静默丢弃，也**不会**臆造很可能错误的源码。它会显式发出带标记的 stub：

```java
static { /* yak-decompiler: undecompilable <clinit>: <原因> */ }
```

`javaclassparser.DecompileStubMarker` 即 `"yak-decompiler:"` 标记；判断输出是否包含
它，即可区分完整反编译与部分反编译。`EnableDecompileSyntaxValidation`
（默认 `true`）控制反编译后的语法安全网——它会把解析失败的成员重新渲染或降级。

---

## 3. 工作原理

流水线分四个阶段，每个阶段都有对应的测试层加固：

1. **类文件解析** —— `ClassParser` 把原始字节解析为常量池、字段、方法和完整的 code
   属性（指令、异常表、StackMapFrame）。
2. **操作数栈模拟** —— 逐方法重放字节码，重建带类型的表达式树并恢复局部变量槽位。
   真实世界中最“难”的字节码多集中于此：数组值、循环计数器、catch 变量之间的槽位
   复用；DUP/swap 族；switch-case 操作数栈；三元值存储与尾部复制返回。
3. **结构分析** —— 把指令图提升为语句树（用标准自然循环算法识别循环、if/else 合并、
   try/catch/finally 区域重建、synchronized 块）。共享 DAG 容器与真正的环被区分开，
   保证语句树无环。
4. **生成** —— 语句树渲染为 Java 源码，再由 Java 语法前端重新解析。解析失败的会被
   重新渲染或降级为带标记的 stub，因此“成功返回”的永远是合法 Java。

---

## 4. 语法覆盖

| 构造 | 状态 |
|------|------|
| 控制流：if/else、循环、带标签 break/continue | GA |
| `switch`（语句 & 表达式）、字符串 switch | GA |
| try/catch/finally、try-with-resources、multi-catch | GA |
| Lambda 与方法引用（`invokedynamic`） | GA |
| 内部 / 嵌套 / 匿名 / 局部类 | GA |
| 泛型、枚举、注解 | GA |
| interface 与注解的 `<clinit>` 字段提升 | GA |
| `synchronized` 块、断言 | GA |
| 现代 Java（record、sealed、模式匹配、switch 表达式、文本块） | 语法语料已覆盖；源码级保真度持续跟踪 |

---

## 5. 验证与复现

```bash
# 语法覆盖 + javac round trip
go test -run 'TestSyntaxCoverageMatrix|TestRecompileRoundtrip' -v \
  ./common/javaclassparser/tests/

# 真实字节码语法回归（77 个回归用例）
go test -run TestDecompileSyntaxRegression -v ./common/javaclassparser/tests/

# panic / 挂起 / 崩溃边界
go test -run 'TestGAPanicFreeBoundary|TestDecompileCyclicStatementTreeNoCrash' \
  -count=1 ./common/javaclassparser/tests/

# 确定性
go test -run TestCorpusDeterminism -v ./common/javaclassparser/tests/

# 整个包
go test ./common/javaclassparser/...
```

以上都在 CI 中运行（`javaclassparser/tests` 的预算为 5 分钟）；大型 `.m2` 扫描是
按环境变量开启的可选测试，不会在常规 CI 中运行。

---

## 6. 已知局限与走向“完美”的路径

GA 不代表地球上每个方法都已完美。诚实的局限：

- **部分 stub 仍可能出现**，出现在极少数、高度不规则的 CFG 上。契约是它们
  *显式且安全*，绝不静默或非法。每一个都是具体、可复现的目标。
- **源码级保真度**（变量名、格式、注释）并非与原始源码逐字节一致——变量名在有调试
  信息时取自 debug 属性，缺失时按 `varN` 合成。这是反编译固有的特性。
- 语料来自真实 Maven 缓存的 Java 8-21 字节码；未来工具链带来的全新字节码形态会
  在出现时被覆盖。

把残余 partial 推向零的推荐方式是迭代式的“遇到第一个失败即停手”工作流：扫描语料、
捕获第一个失败 class、修根因、补回归 `.class`、重跑可移植套件、再继续。它被刻意
设计成一次只盯一个 class 的闭环，避免修复之间互相掩盖。
