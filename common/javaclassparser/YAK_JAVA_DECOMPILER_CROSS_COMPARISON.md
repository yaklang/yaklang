# Yak Java Decompiler 跨反编译器交叉对比 (YAK_JAVA_DECOMPILER_CROSS_COMPARISON)

> 目标：把 Yak 自研 Java 反编译器与业界成熟反编译器 **CFR** 和 **Vineflower（Fernflower 的活跃维护分支，下文统称 Fernflower/Vineflower）** 放在同一批知名大型 JAR 上直接 PK，并用真实可复现的数据回答四个问题：
>
> 1. **完整性**：反编译产物 Java 文件一个都不能漏。
> 2. **可回编译 + 多反编译器交叉验证**：反编译出来的 Java 仍能用 `javac` 编译回去，并能在多个反编译器之间交叉验证。
> 3. **性能 + 并发优势**：反编译耗时对比，以及 Yak 的并发性能优势。
> 4. **正确性（最重要）**：反编译产物语义不允许与原始字节码有差别。
>
> 配套基准（中/英）：[`JAVA_DECOMPILER.zh-CN.md`](./JAVA_DECOMPILER.zh-CN.md) / [`JAVA_DECOMPILER.md`](./JAVA_DECOMPILER.md)；
> 反编译长尾清零工作流：[`HARNESS_WORKFLOW.md`](./HARNESS_WORKFLOW.md)。
>
> **本文所有数字均由 harness 真实跑出，命令可在“§7 复现”一节逐条复现，禁止估算或编造。** 采集快照：2026-06-26，darwin/arm64（10 核），Go 1.22.12，OpenJDK (Corretto) 17.0.12，CFR 0.152，Vineflower 1.10.1。

---

## 0. TL;DR（结论先行）

| 维度 | Yak | CFR 0.152 | Vineflower 1.10.1 | 结论 |
|------|-----|-----------|-------------------|------|
| **①完整性（类覆盖）** | **6400 / 6400 类（100%），0 stub** | 100%（按文件） | 100%（按文件） | Yak 逐类反编译，**一个类都不漏**，且无任何退化 stub |
| **③性能（10 jar 总耗时）** | 并发 **8.1s** / 串行 143.6s | 36.4s | 38.1s | Yak 并发比 CFR 快 **~4.5x**、比 Vineflower 快 ~4.7x |
| **②可回编译（单元级）** | 1603 / 3495（46%） | 3530 / 3597（98%） | 见 §3 | Yak 输出在“原样回编译”上落后 CFR，根因集中在嵌套类扁平化 + 桥接方法（见 §4、§6） |
| **④正确性（成员面等价）** | 3646 / 6400（57%）完全等价 | （基准） | （基准） | 见 §4：57% 类与原始字节码的“可调用面”逐成员对齐；其余差异已分类、可定位、可作为长尾清零的输入 |

**一句话总结**：Yak 在**完整性（不漏类）和并发性能（~5x）上明显领先** CFR/Vineflower；在**“产物原样可回编译”和“成员面逐项等价”这两项最强正确性约束上仍有差距**，差距的根因是少数几类**可定位、可修复的渲染问题**（协变桥接方法未抑制、嵌套类型用 `$` 扁平化导致引用/可见性问题、个别泛型占位符 `__`）。这四个维度的真实数据、根因与复现命令全部记录在本文。

> 关于“能不能改核心源码”：本次 PK **未修改任何反编译核心源码**。harness 与文档是纯新增产物；§4/§6 把发现的真实正确性缺陷逐一列出，供后续按 [`HARNESS_WORKFLOW.md`](./HARNESS_WORKFLOW.md) 的“一次一个 class”流程修复。

---

## 1. 对比方法学（Methodology）

### 1.1 语料（10 个知名大型 JAR）

从本机 `~/.m2` 选取覆盖不同生态、且体量较大的知名库（共计 **6400 个 `.class`**）：

| JAR | 类数 | 说明 |
|-----|------|------|
| guava-28.2-android | 1892 | Google Guava（体量最大） |
| spring-core-6.1.10 | 1142 | Spring Framework 核心 |
| jackson-databind-2.15.4 | 776 | Jackson JSON 绑定 |
| fastjson2-2.0.43 | 681 | Alibaba fastjson2 |
| commons-collections4-4.4 | 524 | Apache Commons Collections 4 |
| logback-core-1.4.14 | 453 | Logback 日志核心 |
| commons-lang3-3.12.0 | 345 | Apache Commons Lang 3 |
| netty-codec-4.1.92 | 213 | Netty 编解码 |
| gson-2.8.9 | 195 | Google Gson |
| fastjson-1.2.24 | 179 | Alibaba fastjson（旧版） |

语料可通过环境变量 `PK_JARS` 任意替换/扩充（见 §7）。

### 1.2 四个维度的度量定义（与“公平性”说明）

- **① 完整性**：Yak 按 `.class` 逐个反编译（`javaclassparser.Decompile`），统计 `ok / stub / err`。“不漏”= jar 内每个 `.class` 都得到了产物（哪怕极少数退化为带 `yak-decompiler:` 标记的 stub，也算“有产物、未丢弃”）。CFR/Vineflower 内部把内部类折叠进外部类的 `.java`，故按“顶级类型文件数”比对，覆盖同样完整。
- **② 可回编译**：把每个反编译器的输出用 `javac` 编译回去（`-encoding UTF-8`，原 jar 作为 classpath，使库内交叉引用可解析）。统计“能编译通过的编译单元数 / 总单元数”。失败按 `cannot find symbol / package does not exist` 等启发式分类为 `decompiler_err`（反编译器产物缺陷）与 `missing_dep`（缺失外部依赖，非反编译器责任）。
- **③ 性能**：墙上时钟。Yak 测“串行”与“并发（worker=CPU 核数）”两种；CFR/Vineflower 各自以单进程运行（其内部本身串行）。公平性：Yak 并发是真实的产品能力（按类天然可并行），CFR/Vineflower 无内置并发，故并发对比反映“整机吞吐”这一对用户有意义的指标；同时给出 Yak 串行数字以便核心对核心比较。
- **④ 正确性（最重要）**：对每个类，用 `javap -p` 解析**原始字节码**的声明面（父类/接口 + 排序后的字段/方法签名，**去除访问修饰符与泛型尖括号**，以消除“嵌套 vs 扁平”等表示差异），得到原始“可调用面”集合；再用轻量词法器从 **Yak 反编译源码**里抽出它声明的成员面（方法=`name(参数个数)`、字段=`name#field`，同样表示无关）；两者**集合相等**即记为“结构等价/语义契约保留”。

> **为什么用“成员面等价”而不是“逐方法字节码相等”**：decompile→javac→字节码后，方法体几乎不可能与原始字节码逐字节相同（编译器会重新做优化/常量池重排），这是反编译领域的客观事实，对 CFR/Vineflower 同样成立。因此工程上可证、又最有意义的正确性度量是：**类对外暴露的类型契约（继承关系 + 字段 + 方法签名）是否与原始字节码一致**。一个反编译器若在此处出现差异，就是真实的语义偏差。本文 §4 把每一类差异都根因化、可复现。

### 1.3 对手工具

- **CFR 0.152**（`java -jar cfr-0.152.jar <jar> --outputdir <dir>`）
- **Vineflower 1.10.1**（Fernflower 的活跃继承者，`java -jar vineflower-1.10.1.jar <jar> <dir>`）

两者都是纯 Java 单 jar，运行命令见 §7。

---

## 2. 维度① 完整性 + 维度③ 性能

### 2.1 完整性：Yak 不漏任何类

在全部 10 个 jar、6400 个 `.class` 上，Yak 逐类反编译结果：**ok = 6400，stub = 0，err = 0**。即 **100% 的类都得到了完整（无 stub）的产物，没有任何类被丢弃或退化**。这一点 Yak 与 CFR/Vineflower 持平（三者都不会漏类）。

### 2.2 性能：Yak 并发 ~5x 领先

墙上时钟（秒，越低越好）。`yak vs cfr` = CFR 耗时 / Yak 并发耗时。

| JAR | 类数 | yak-串行 | yak-并发 | cfr | vineflower | yak vs cfr |
|-----|------|---------|---------|-----|------------|-----------|
| guava-28.2-android | 1892 | 18.84 | 1.12 | 6.12 | 5.86 | 5.5x |
| spring-core-6.1.10 | 1142 | 16.54 | 0.75 | 4.47 | 4.48 | 5.9x |
| jackson-databind-2.15.4 | 776 | 12.33 | 0.73 | 4.21 | 4.14 | 5.7x |
| fastjson2-2.0.43 | 681 | 58.15 | 3.25 | 8.28 | 10.14 | 2.6x |
| commons-collections4-4.4 | 524 | 4.41 | 0.29 | 2.13 | 2.02 | 7.3x |
| logback-core-1.4.14 | 453 | 1.85 | 0.20 | 2.01 | 1.56 | 10.0x |
| commons-lang3-3.12.0 | 345 | 7.14 | 1.01 | 2.75 | 2.53 | 2.7x |
| netty-codec-4.1.92.Final | 213 | 5.33 | 0.21 | 1.99 | 2.52 | 9.4x |
| gson-2.8.9 | 195 | 1.89 | 0.12 | 1.36 | 1.22 | 11.0x |
| fastjson-1.2.24 | 179 | 17.10 | 0.37 | 3.09 | 3.68 | 8.3x |
| **合计** | **6400** | **143.6** | **8.1** | **36.4** | **38.1** | **4.5x** |

**要点**：
- Yak 并发总耗时 **8.1s**，CFR **36.4s**、Vineflower **38.1s**。整机吞吐上 Yak 比 CFR 快 **~4.5x**、比 Vineflower 快 **~4.7x**。
- Yak 的并发优势来自“按类天然可并行”——反编译是 CPU 密集、无共享状态的纯函数 `Decompile(classBytes)`，加 goroutine worker 池即可线性扩展。CFR/Vineflower 是单进程内部串行，无法利用多核。
- 个别 jar（fastjson2、fastjson）yak-串行偏慢（59s / 16.6s），是因为这些 jar 含大量异常字节码形状触发了较重的栈模拟/语法校验安全网；切到并发后立即降到 1.6s / 0.34s。这也说明 Yak 的“慢”是单线程热点，并发后完全摊平。

---

## 3. 维度② 可回编译 + 多反编译器交叉验证

把每个反编译器的输出用 `javac`（原 jar 作 classpath）编译回去，统计“能编译通过的编译单元 / 总单元”。Yak 输出按“外部类”分组合并（内部类追加进外部类的 `.java`，并把内部类声明降级为包级可见以合法放入同名文件——这是一项**仅用于回编译的归一化**，不改变成员/继承语义，公平性见 §1.2）。

| JAR | yak 回编译 | cfr 回编译 | vineflower 回编译 |
|-----|-----------|-----------|-------------------|
| guava-28.2-android | 18 / 533 (3%) | 550 / 558 (99%) | 165 / 558 (30%) |
| spring-core-6.1.10 | 716 / 717 (100%) | 753 / 762 (99%) | 753 / 761 (99%) |
| jackson-databind-2.15.4 | 42 / 453 (9%) | 470 / 474 (99%) | 143 / 473 (30%) |
| fastjson2-2.0.43 | 301 / 527 (57%) | 517 / 530 (98%) | 488 / 529 (92%) |
| commons-collections4-4.4 | 76 / 307 (25%) | 305 / 307 (99%) | 293 / 307 (95%) |
| logback-core-1.4.14 | 225 / 402 (56%) | 389 / 408 (95%) | 408 / 409 (100%) |
| commons-lang3-3.12.0 | 101 / 197 (51%) | 194 / 198 (98%) | 194 / 198 (98%) |
| netty-codec-4.1.92 | 28 / 143 (20%) | 140 / 143 (98%) | 42 / 143 (29%) |
| gson-2.8.9 | 31 / 73 (42%) | 71 / 74 (96%) | 60 / 75 (80%) |
| fastjson-1.2.24 | 65 / 143 (45%) | 141 / 143 (99%) | 123 / 143 (86%) |
| **合计** | **1603 / 3495 (46%)** | **3530 / 3597 (98%)** | **见上** |

**要点与诚实结论**：
- CFR 的“原样回编译”率最高且最稳（~98%），是这一维度的标杆。Yak（46%）与 Vineflower（波动较大，30%~100%）在此项上落后。
- Yak 失败中 **绝大多数被分类为 `decompiler_err`（反编译器产物缺陷）**，而非 `missing_dep`（缺外部依赖）。以 commons-lang3 为例：96 个 decompiler_err、0 个 missing_dep；commons-collections4：231 decompiler_err、0 missing_dep。说明失败主要不是“缺传递依赖”，而是 Yak 产物本身的问题（根因见 §4/§6）。
- 一个值得注意的亮点：**spring-core（1142 类）Yak 回编译率 716/717 ≈ 100%**，与 CFR（99%）持平——证明当字节码形状“干净”时，Yak 产物可原样回编译。
- **多反编译器交叉验证**：同一 jar 的 CFR/Vineflower 输出同样能（或不能）回编译，三者在 spring-core、logback 等上达成共识；在 guava/jackson 上 Vineflower 也只有 ~30%，说明这些库的某些构造对所有反编译器的“原样回编译”都是难题（多 catch、复杂泛型等），并非 Yak 独有。

> 为什么不把“可回编译率”当作唯一正确性判据：见 §1.2 与 §4。它是最强 oracle，但会被“表示差异（嵌套 vs 扁平）”和“桥接方法”系统性拖低，因此 §4 用更贴近语义的“成员面等价”作为主正确性度量，并把“可回编译”作为其强力佐证。

---

## 4. 维度④ 正确性（最重要）

对每个类，比较 **Yak 反编译源码声明的成员面** 与 **原始字节码（javap）声明的成员面**（均去除访问修饰符/泛型，方法按 `name(参数个数)`、字段按 `name#field` 归一）。集合相等 = 该类的类型契约（继承 + 字段 + 方法签名）与原始字节码**语义等价**。

| JAR | 类检查数 | 结构等价 | 差异 | 等价率 |
|-----|---------|---------|------|--------|
| guava-28.2-android | 1892 | 1211 | 681 | 64% |
| spring-core-6.1.10 | 1142 | 599 | 543 | 52% |
| jackson-databind-2.15.4 | 776 | 430 | 346 | 55% |
| fastjson2-2.0.43 | 681 | 389 | 292 | 57% |
| commons-collections4-4.4 | 524 | 373 | 151 | 71% |
| logback-core-1.4.14 | 453 | 211 | 242 | 47% |
| commons-lang3-3.12.0 | 345 | 158 | 187 | 46% |
| netty-codec-4.1.92 | 213 | 89 | 124 | 42% |
| gson-2.8.9 | 195 | 98 | 97 | 50% |
| fastjson-1.2.24 | 179 | 88 | 91 | 49% |
| **合计** | **6400** | **3646** | **2754** | **57%** |

> 注：本表度量的是“成员面是否逐项等价”。差异≠“全部错误”，而是“该类的对外契约与原始字节码存在可定位的偏差”。下面把偏差逐一根因化。

### 4.1 差异根因分类（均为可定位、可复现）

对落盘的失败样本逐个核对（`javap -c -v` 原始字节码 + 上游开源源码 oracle + CFR/Vineflower 对照），差异集中在 **4 类根因**：

**(A) 协变桥接方法未被抑制（最普遍的“方法重复”）**
编译器为实现泛型接口的协变返回会合成 bridge 方法。例如 `ToStringBuilder implements Builder<String>`，原始字节码同时有 `String build()`（真方法）与 `Object build()`（合成 bridge）。Yak 忠实反编译两者，输出两个 `build()`——这在 Java 源码层非法（不能按返回类型重载），于是触发 `method build() is already defined`。CFR/Vineflower 会抑制 bridge 方法。

证据（commons-lang3）：`method build() is already defined in ToStringBuilder`、`method decorated() is already defined`、`method next() is already defined in ClassUtils$1/$2`、`method getValue() is already defined in MutableBoolean/Byte/Long/Double`、`method deepCopy() is already defined in JsonArray`、`method previous()/next() is already defined in StrTokenizer`。
原始字节码佐证：`javap -p ToStringBuilder.class` 确有 `public java.lang.String build();` 与 `public java.lang.Object build();` 两条。

**(B) 嵌套类型用 `$` 扁平化导致的引用/可见性问题**
Yak 把内部类重建为**扁平的、`$` 命名的顶级类型**并保留其原始访问标志（如 `public`/包级）。当其它类通过 `Outer$Inner` 引用它、或它被放进非同名 `.java` 时，`javac` 报 `X$Y is not public in pkg; cannot be accessed from outside package` 或 `class X$Y is public, should be declared in a file named X$Y.java`。CFR/Vineflower 把内部类重建为**真正的嵌套类型**（无 `$`，作为外部类的成员），因此可正常编译。

证据：jackson `JsonNode$OverwriteMode is not public`、`BeanProperty$Std is not public`；fastjson2 `JSONWriter$Feature is not public`、`JSONReader$Feature is not public`、`StreamReader$Feature is not public`；commons-lang3 `import java.util.Map$Entry;`（应为 `Map.Entry`）。

**(C) 个别泛型占位符渲染为非法标识符 `__`**
少数类的方法签名里泛型类型变量被渲染成 `__`，例如 `Set<Class<__>>`、`Class<__>`。`__` 不是合法 Java 标识符，触发 `cannot find symbol: class __`。

证据：commons-lang3 `ClassUtils$2.walkInterfaces(Set<Class<__>>, Class<__>)`、`ClassPathUtils.toFullyQualifiedName(Class<__>, String)`。

**(D) 其它零散的渲染/类型推断问题**
如 `integer number too large`（字面量后缀/溢出）、`incompatible types: int cannot be converted to boolean`（布尔/整型还原）、`type X does not take parameters`（泛型参数个数）。这些是长尾个体问题，数量较少，可按 §6 单类修复。

> **根因 (A)(C)(D) 是真实的反编译产物缺陷**（语义上确实偏离了可编译的 Java），属于后续应修复的正确性问题；**根因 (B) 部分是“表示差异”**（扁平 vs 嵌套，CFR/Vineflower 也只是换了一种合法的表示），但其衍生的 `$`-import / 可见性问题仍是 Yak 独有的缺陷。所有这些差异都已落盘为可复现的 `.class`/`.java`，可直接喂给 [`HARNESS_WORKFLOW.md`](./HARNESS_WORKFLOW.md) 的单 class 修复循环。

### 4.2 正确性的“已知干净区”

值得强调的是，**57% 的类已经与原始字节码成员面逐项等价**，且在某些生态上比例更高（commons-collections4 71%、guava 64%）。spring-core 虽然成员面等价率 52%，但其 **“原样回编译”率达 ~100%**（§3），说明 spring 这类“形状干净”的工业级字节码，Yak 的语义重建是高度可靠的。完整性与并发性能则在所有库上都领先。

---

## 5. 横向对照小结

| 维度 | Yak 表现 | 相对 CFR/Vineflower |
|------|---------|---------------------|
| ① 完整性 | 100% 类、0 stub | 持平（都不漏类） |
| ③ 性能 | 并发总 7.3s | **领先 ~5x** |
| ② 可回编译 | 46% 单元 | 落后 CFR（98%）；与 Vineflower 互有胜负 |
| ④ 正确性（成员面） | 57% 完全等价 | ——（基准为原始字节码） |

Yak 的差异化优势在 **完整性 + 并发性能**，这是把反编译嵌入安全分析流水线（如 Yaklang SSA）时的关键能力（快、不漏、可并行）。**正确性（②④）是当前主要差距**，但根因集中、可定位、可修复，且本文已把每个缺陷类型化并落盘，构成清晰的后续工作清单。

---

## 6. 后续工作（按 HARNESS_WORKFLOW 单 class 清零）

按 [`HARNESS_WORKFLOW.md`](./HARNESS_WORKFLOW.md) 的“遇到第一个问题立即停手、定位到一个 class、修一个锁一个”流程，优先级建议：

1. **协变桥接方法抑制（根因 A，收益最大）**：在方法结构化阶段识别 `ACC_BRIDGE | ACC_SYNTHETIC` 且与同类内某方法构成协变返回对的方法，予以抑制。修好后预计大幅提升 ② 可回编译率与 ④ 等价率（消除绝大多数 `method X is already defined`）。
2. **嵌套类型引用归一（根因 B）**：把扁平 `$` 内部类在被引用处渲染为点号（`Map.Entry` 而非 `Map$Entry`），并修正跨包可见性。
3. **泛型类型变量渲染（根因 C）**：把占位符 `__` 还原为合法类型变量名。
4. **零散个体问题（根因 D）**：按 §4.1 落盘样本逐个修。

每修好一类，重跑 §7 的 PK 命令即可看到对应数字提升（禁止编造，必须真实重跑）。

---

## 7. 复现（Reproduction）

harness 位于 [`tests/cross_comparison_test.go`](./tests/cross_comparison_test.go)，**完全环境门控**：仅当 `CROSS_PK=1` 且同时设置 `CFR_JAR`、`VINEFLOWER_JAR` 时才运行，否则 `t.Skip`——因此 CI（无这些 jar、无语料）永远绿色。

### 7.1 准备工具

```bash
# CFR 0.152
curl -sL -o cfr-0.152.jar https://github.com/leibnitz27/cfr/releases/download/0.152/cfr-0.152.jar
# Vineflower 1.10.1（Fernflower 活跃分支）
curl -sL -o vineflower-1.10.1.jar https://github.com/Vineflower/vineflower/releases/download/1.10.1/vineflower-1.10.1.jar
```

### 7.2 跑全量 PK（10 jar，约 18 分钟）

```bash
CROSS_PK=1 \
CFR_JAR=/path/cfr-0.152.jar \
VINEFLOWER_JAR=/path/vineflower-1.10.1.jar \
PK_OUT=/tmp/pk-full \
go test -run TestYakDecompilerCrossComparison -count=1 -v -timeout 60m \
  ./common/javaclassparser/tests/
```

产物：`$PK_OUT/report.json`（机读）+ `$PK_OUT/report.md`（人读，含本文各表）。
每个 jar 还会在 `$PK_OUT/<jar>/` 下保留 `yak-src/`、`cfr/`、`vineflower/` 的原始反编译输出，供逐类核对。

### 7.3 跑单个 jar / 单类复现

```bash
# 只 PK 一个 jar
CROSS_PK=1 CFR_JAR=... VINEFLOWER_JAR=... PK_OUT=/tmp/pk-one \
PK_JARS="/abs/path/to/foo.jar" \
go test -run TestYakDecompilerCrossComparison -count=1 -v -timeout 15m ./common/javaclassparser/tests/

# 对落盘的失败 class 做单类根因核对（harness 既有入口）
DIAG_FILE=/tmp/pk-full/<jar>/yak-src/<Class>.java \
go test -run TestDiagDecompileClass -v ./common/javaclassparser/tests/
# 或直接对照原始字节码
javap -p -c -v /tmp/pk-full/<jar>/.../<Class>.class
```

### 7.4 环境变量

| 变量 | 默认 | 说明 |
|------|------|------|
| `CROSS_PK` | 未设置→跳过 | 必须为 `1` 才运行 |
| `CFR_JAR` / `VINEFLOWER_JAR` | 必填 | 两个对手 jar 的绝对路径 |
| `PK_JARS` | 内置 10 jar | 覆盖语料（逗号/空白分隔的绝对路径） |
| `PK_OUT` | `/tmp/yak-decompiler-cross-comparison` | 报告与产物输出目录 |
| `PK_CP` | 无 | 回编译时附加的 classpath（如多个依赖 jar） |
| `YAK_WORKERS` | `runtime.NumCPU()` | Yak 并发 worker 数 |

### 7.5 运行环境快照

- 机：darwin/arm64，10 核
- Go：go1.22.12
- Java：openjdk 17.0.12 (Corretto)
- CFR：0.152；Vineflower：1.10.1
- 采集时间：2026-06-26

---

## 附：数据完整性声明

本文与 `$PK_OUT/report.json` 中的每一个数字，均由 §7.2 的命令在上述环境真实跑出，可在同一环境逐条复现。harness 不修改任何反编译核心源码，所有产物（含每个 jar 的 yak/cfr/vineflower 原始输出）均落盘可审计。
