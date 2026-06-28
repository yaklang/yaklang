# Codec 算法交叉验证 — 交接 TODO (CODEC_TODO.md)

> 分支: `codex/yak-java-decompiler-cross-comparison`
> 核心目标: 用「反编译 → 重编译回 class → 直接运行算法对比」验证反编译器的**语义正确性**, 达到 GA 水准, 而非仅"能反编译"。
> 配套文档: [YAK_JAVA_DECOMPILER_CROSS_COMPARISON.md](./YAK_JAVA_DECOMPILER_CROSS_COMPARISON.md), [HARNESS_WORKFLOW.md](./HARNESS_WORKFLOW.md)
>
> 本文件只记录**尚未修复**的缺陷与待扩展项; 已治本的 bug 不再保留文字记录 —— 其回归测试 (testdata
> 种子 + `Test*` 守卫) 即永久档案。新发现的 bug 必须在此登记 (症状 / 根因 / 复现 / 规避现状)。

---

## 当前状态 (只保留里程碑 / 指标 / 杠杆, 不留已治本叙述)

- **GA 里程碑 — guava 真实差分调用 = IDENTICAL**: guava 28.2-android 反编译 → 重编译成 overlay →
  classpath 覆盖原 jar → `GuavaProbe` 反射跑算法对比指纹。18 个类 (IntMath / Ints / Longs / UnsignedInts /
  UnsignedLongs / Ascii / Strings + 各自内部类) overlay 重编译 exit 0, 输出与原 jar byte-identical (同 MD5)。
  即直接回答「能否反编译 guava 并被外部调用」: **能**。
- **里程碑 — enum 常量体跨类折叠已接入生产 jar 路径**: 多类折叠 (合成 `Outer$N` 常量体内联回 enum, 并抑制
  `$N` 独立产出) 不再只是 `DecompileWithResolver` API, 已在 `JarFS.decompileClassBytes` 落地, 整 jar 反编译
  即享受。承重双测 `TestEnumConstantBodyFoldIsLoadBearing` (resolver API) + `TestEnumConstantBodyFoldJarPathIsLoadBearing`
  (jar 路径), 均带 `JDEC_NO_ENUM_FOLD` kill-switch。另抑制了 enum 合成 marker 构造器
  `<Enum>(String,int,<Enum>$N)` (承重测试 `TestEnumMarkerCtorSuppressionIsLoadBearing`, kill-switch
  `JDEC_NO_ENUM_MARKER_CTOR`)。实测 guava `CaseFormat` (release-8, 含 access$N + marker ctor) 折叠成单一合法
  enum 且 round-trip 通过。详见 Bug V。
- **差分门禁 `TestCodecSemanticsRoundTrip`: 57 个自托管 battery 全绿** (byte-for-byte round-trip), 覆盖
  控制流 / 排序 / 链式赋值 / SHA-256 / SHA-512 / TEA-XTEA-RC4 / String-switch / try-with-resources /
  try-finally-loop / 数值转换重命名 / iinc 宽化 / 循环计数器 slot 复用 / 嵌入式赋值 / Class 字面量 /
  int 类别收窄 / guava IntMath(gcd·log2·floor-sqrt·pow·isPrime·checkedAdd) / 严格 Hex 编解码(收窄+校验异常) /
  boolean-int 槽复用形态 / **空前导 case 的 tableswitch (Base32/Base64 EOF 形态)** /
  **对象-null 守卫后接底部测试循环 (Md5Crypt salt 形态)** /
  **短路 `(A&&B)||C` 中 C 为内联数组可变参调用 (DoubleMetaphone.conditionC0 形态)** 等。已治本 bug 的文字记录
  按约定删除, 其承重 `Test*` + testdata 种子即永久档案。
- **OPCODE 解析覆盖门禁 `TestOpcodeParseCoverage`: 195/195 (100.0%)** (语料 126 class + 31 battery,
  命中 198 distinct opcode), 7 个文档化排除
  (jsr / jsr_w / ret / goto_w / wide / ldc_w / nop —— 均为 javac 不由源码产生或前缀修饰)。
- **GA 里程碑 — guava `base` 整包「反编译→重编译→打回 jar→外部反射调用」= IDENTICAL (语义, 非仅编译)**:
  经生产 JarFS 路径重生 160 单元后整目录 `javac` (guava + 5 传递依赖上 classpath): **158/160 单元可重编译**
  (残留 2 单元 = Bug AH Group A, 临时手补 cast 后)。重编译产物覆盖回 overlay jar, 外部 `BaseProbe` 反射跑
  VerifyException / Predicates / Suppliers / **CaseFormat** / Ascii / Strings / Equivalence, 输出与原 jar
  **逐行 IDENTICAL**。其中 `CaseFormat` 此前 `StackOverflowError` (常量体 `super.convert` 被误渲染成
  `this.convert` 致无限递归) —— 这是**编译期测不出、必须运行**才暴露的语义 bug, 本轮已治本 (见下「本轮新增治本」
  第 4 项)。残留 2 条全部是 Group A「类型变量擦除成 Object 出现在非返回位置」(`PairwiseEquivalence` 的
  `Iterator var=it()` 应为 `Iterator<T>`、`Equivalence$Wrapper` 的 `Wrapper var=(Wrapper)o` 应为 `Wrapper<?>`);
  需把泛型类型沿数据流传播 (方法返回按接收者类型实参实例化), 属大特性, 见 Bug AH。
- **本轮新增治本 (codec 真实差分新发现, 4 项, 均已锁回归种子, 种子即档案)**:
  - **布尔合并 (mergeCondition) 重排 `Next` 后未刷新 true/false 闭包, 致条件取反 + then/else 对调 (Nysiis 编码截断)**
    (语义正确性, 编译期测不出): if 分支体 `return new char[]{...}` 这类**多 opcode 数组构建叶**被上游折叠把
    if 节点的 `Next` 重排成 `[true,false]`, 故 `JmpNode` 钉支捕获到 trueIndex=0; 随后短路布尔合并
    (`mergeCondition`) 把 `Next` 重建成 `[false,true]` (4 个分支恒按此序写) 却**没更新** `RemoveGotoStatement`
    早先按旧序捕获的 `TrueNode`/`FalseNode` 闭包 → 合并条件极性被悄悄取反 (丢 `!`)、then/else 两叶对调。源码
    仍可编译, 故只有「反编译→重编译→运行对比」能暴露: commons-codec `Nysiis.encode("Thompson")` 得 `TAN` 而非
    `TANPSA` (每个 encode 都被截断)。同形态但叶为 `int` 不触发重排、保持正确, 是它逃过纯语法/编译检查的原因。现于
    `mergeCondition` 每次重建 `parentNode.Next` 后, 按刚建好的 `[false,true]` 序重钉 `JmpNode` 并刷新
    `TrueNode`/`FalseNode` 闭包 (对常见 trueIndex=1 节点为 no-op)。实测修复 Nysiis (overlay 差分 byte-identical)。
    种子 `ShortCircuitArrayLeaf` (承重测试 `TestShortCircuitArrayLeafIsLoadBearing`, kill-switch `JDEC_MERGEIF_PIN_OFF`
    跑负向测试: 关闭即复现指纹分叉)。
  - **短路 `(A&&B)||C` 中 C 为「内联数组可变参调用」时布尔物化退化 (DoubleMetaphone.conditionC0)** (语义正确性 +
    编译错误, 编译期会报「缺少返回语句」): `return (c!='I' && c!='E') || contains(value, i, n, "BACHER");` —— 右
    操作数 C 是可变参调用, javac 用 `anewarray; dup; idx; ldc; aastore` 在栈上构建数组实参。原理化三目重建
    (`buildSharedLeafTernary` 的 `arm()` 走查) 把数组元素写 `aastore`(经 `isTernaryArmStore`) 误判成「语句级
    赋值分派」→ 整体 decline、退回旧合并器, 后者错连前导条件并丢 else, 产出缺返回的 `if (A'&&B') return C;` 且把
    第二个比较取反弄反。现新增 `isInlineArrayInitStore` (识别 `Xastore` 且数组引用是新建 `NewExpression` 的内联
    数组字面量构建), 在 `arm()` 两处把 `isTernaryArmStore(op)` 收紧为 `isTernaryArmStore(op) && !isInlineArrayInitStore`,
    使原理化构建器能跨过这些「取值型」数组写、正确物化短路 `||`。实测修复 commons-codec `DoubleMetaphone`。种子
    `ShortCircuitArrayArg` (承重测试 `TestShortCircuitArrayArgIsLoadBearing`, kill-switch `JDEC_ARRAYINIT_TERNARY_OFF`
    跑负向测试: 关闭即复现「缺少返回语句」)。
  - **空前导 case 的 tableswitch 结构化错位 (Base32/Base64 EOF 形态)** (语义正确性, 编译期测不出):
    `BaseNCodec.encode` 的 EOF 收尾 `switch(modulus)` —— 稠密 tableswitch、**首个 case (case 0) 为空**(仅
    break 到 switch 后)、真实工作在 case 1/2 各带内层 if、`default: throw`、switch 后还有所有 case 都要到达的
    尾码。goto 折叠后空前导 case 的起点节点恰是 switch 后的合并点。旧 `SwitchRewriter1` 用
    `mergeNode.RemoveAllSource()` + `node.AddNext()` 插 break, 把 switch→merge 边从原位移到 `node.Next` 末尾,
    而 `caseToIndexMap` 在解析期已按固定下标捕获 → **整体错位一格** (throw 落到真实 case, 尾码进 default), 且把
    某些 case 守卫的 then/else 也悄悄对调 → 运行时 `IllegalStateException: Impossible modulus N`。现改为按原位
    `source.ReplaceNextSliceKeepOrder(mergeNode, breakNode)` 原地替换, 并跨两次 `SwitchRewriter1` 用
    `Node.SwitchEmptyCaseMergeNode` 持久化已解出的合并点 (避免第二遍把 default/throw 误判成 merge)。实测修复
    Base32 / Base64 / BCodec。种子 `SwitchEmptyLeadingCaseAlgorithms`。
  - **对象-null 守卫的 then/else 在「后接底部测试循环」时被对调 (Md5Crypt salt 被忽略)** (语义正确性, 编译期测不出):
    `if (salt == null) { 随机 } else { 解析 }` (字节码 `ifnonnull`) 在 else 带内层 if+throw、且 switch 后跟
    test-at-bottom 循环 (`while`/`do-while` javac 编成 `goto test; body; test: if(cond) goto body`) 时, 节点图
    重建会在「条件 sense 已定」之后**重排守卫的两个后继 `node.Next`**, 而 `RemoveGotoStatement` 旧逻辑按
    `node.Next` 下标 (trueIndex 固定为 1) 选 true/false 分支 → 选错支 → salt 走了随机分支被忽略。现于建图时
    (`opcode.Target` 顺序仍可靠的 `[falseBranch, trueBranch]` 处) 把 true 分支按**节点身份**钉到 `Node.JmpNode`,
    `RemoveGotoStatement` 改用身份匹配 (`Next[0]==JmpNode` 选 0 否则 1) 选支, 顺序保持时为严格 no-op、节点被替换
    (非仅重排) 时优雅退回旧 fallback, 故不会回退既有正确用例。实测修复 Md5Crypt salt 解析。种子
    `NullCheckBranchThenLoop` (kill-switch `JDEC_IFBRANCH_PIN_OFF` 跑负向测试)。
- **此前本轮新增治本 (4 项, 均已锁回归种子, 种子即档案)**:
  - **invokespecial 超类调用渲染成 `super.m()` 而非 `this.m()`** (语义正确性, 编译期测不出): 非构造器
    invokespecial、接收者为 `this`、目标类 ≠ 当前类时, 是 `super.m(...)` 调用; 旧版一律渲染成 `this.m(...)`,
    在被子类覆盖的方法 (如 enum 常量体覆盖父 enum 的 `convert`) 上**虚分派回覆盖体→无限递归→StackOverflow**
    (guava `CaseFormat.LOWER_UNDERSCORE.to(...)` 实测崩)。现 `FunctionCallExpression.IsSpecialInvoke` 标记
    invokespecial (两处解码点均置位), `String()` 渲染时按「非 `<init>` + 接收者 `this` + 目标类≠当前类」判定
    super 调用; 私有同类 invokespecial (目标类 == 当前类) 仍 `this.m()`。种子 `invokespecial_super_call`
    (含 super 调用 + 私有同类调用双断言)。
  - **被误删的「已声明」构造器恢复** (语义正确性, 非编译): 空体构造器 (体仅隐式 `super()`) 以前因「返回类型
    void 的空体方法」一刀切被 `continue` 丢弃, 导致**真实声明的**无参构造器在类还有其它构造器时凭空消失
    (guava `VerifyException()` → 调用点 `new VerifyException()` 找不到构造器)。现 `isOmittableDefaultCtor`
    仅在「唯一 + 无参 + 可见性等同 javac 隐式默认」三条全满足时才省略 (此时省略无损, javac 自再生同款); 其余
    一律保留 —— 含**多构造器下的无参体**、**私有唯一构造器** (单例, 否则可见性被悄悄放宽)、**带参空体构造器**
    (真实重载)。种子 `keep_declared_noarg_ctor` / `keep_private_singleton_ctor` / `drop_implicit_default_ctor`。
    kill-switch `JDEC_NO_KEEP_DECLARED_CTOR`。
  - **合成访问桥构造器形参按目标泛型定型**: 桥构造器 (末参为合成匿名 marker 类) 无 Signature, 形参本会以擦除
    `Object`/raw 渲染, 其唯一体 `this(args...)` 转发到声明了类型变量的私有目标构造器时报「Object 无法转换为 T」
    (guava `Equivalence$Wrapper(Equivalence,Object,Equivalence$1)` / `Predicates$IsEqualToPredicate(Object,
    Predicates$1)`)。现 `reTypeSyntheticBridgeCtorParams` 按「擦除形参表等于桥去掉 marker 后的前缀」定位唯一非
    合成目标构造器, 取其 Signature 的泛型形参类型就地覆盖桥的非 marker 形参 (桥擦除回同 descriptor, 字节忠实;
    raw 调用点照样通过)。此形态在每个「私有泛型构造器被外层访问」的嵌套类反复出现, 故收益面广。种子
    `synthetic_bridge_ctor_generic_param`。kill-switch `JDEC_NO_SYN_BRIDGE_CTOR_RETYPE`。
  - **嵌套泛型返回强转走 raw 桥**: 类型变量返回强转的目标若带**嵌套**泛型实参 (`Function<Supplier<T>,T>`)、且
    返回值是被转成该接口的**另一类** (枚举单例 `SupplierFunctionImpl.INSTANCE` 具体实现 `Function<Supplier<
    Object>,Object>`), 直接强转被 javac 判为不可转 (`Supplier<Object>` ≠ `Supplier<T>`)。现 `nestedGenericRawBridge`
    在「目标含嵌套泛型 (多于一个 `<`) 且值擦除≠目标擦除」时插入 raw 桥 `(目标) (raw目标) (值)` (合法: 先降到 raw
    超类再非受检放宽); 裸类型实参 (`Predicate<T>` / `Converter<T,T>`) 仍用单层直转。种子
    `nested_generic_return_raw_bridge_cast` (真实 guava `Suppliers.class`)。
- **早前本轮治本 (4 项, 均已锁回归种子, 文字记录按约定删除, 种子即档案)**:
  - **方法级形参类型变量 `<...>` 还原** (原 Bug AG): `ParseMethodSignatureFull` + `parseFormalTypeParams`
    解析签名首部 `<T:...>` 段, dumper `writeMethodTypeParams` 渲染方法级 `<T>` 声明并把名字临时注入
    `FuncCtx.TypeParams`; 形参/返回保留类型变量 (含 `T[]` 数组形态), 冗余 `extends Object` 界已剥离。
    种子 `method_type_params.class` + `method_type_params_array.class`。清掉 guava base 大量
    `Object cannot be converted to CAP#N` (泛型推断调用点)。
  - **synchronized 块声明提升** (Bug Y synchronized 变体): 见下文 Bug Y。种子 `synchronized_return_scope.class`。
    清掉 guava base 整个 cannot-find-symbol 级联 (`Enums.getEnumConstants`)。
  - **合成访问桥构造器空体不再丢弃**: dumper `isSyntheticAccessBridgeCtor` (名为 `<init>` + ACC_SYNTHETIC +
    末参为合成匿名 marker 类型) 的空体 `this(...)` 委托构造器以前被「丢空体方法」逻辑删掉, 导致
    `new Platform$JdkPatternCompiler((Platform$1)null)` 等调用点找不到构造器。现保留渲染。
  - **字段单例返回的非受检强转**: 返回类型提及在作用域类型变量 (`Box<T>`)、而 return 的值是字段访问单例
    (`Box<?> INSTANCE`, 捕获为 `Box<CAP>`) 时补 `(Box<T>)` 非受检强转 (与 guava `Converter.identity` 返回
    `IdentityConverter.INSTANCE` 完全同构)。强转仅对字段访问值生效, 不波及 `return this`/`new`/方法调用。
    种子 `generic_field_return_cast.class`。清掉 guava base `IdentityConverter<CAP> cannot be converted to
    Converter<T>` 簇。
- 全量 `go test ./common/javaclassparser/...` 全绿。
- **权威多维对比报告 v2** (`report-v2.{md,json}`; 5 真实 jar × 4 工具 yak-syntax/yak-raw/
  cfr-0.152/vineflower-1.10.1, jar+deps 上 classpath, 逐单元 `javac`, 89 分钟):
  - **速度**: yak-syntax 吞吐 **9.3-15.1×** 快于 CFR (guava 13.0×, codec 15.1×); yak-raw 更快 (guava 反编译
    并发 0.52s vs CFR 5.01s / VF 4.45s)。
  - **健壮性**: 5 jar 全部 **100% 产出, 0 stub / 0 err** (从不崩/不退化)。
  - **重编译率 (单元口径, Yak 每个扁平内部类算 1 单元, CFR/VF 每外层文件 1 单元, 分母不同)**:
    guava yak **59%(1107/1892)** > cfr 44% < vf 70%; codec yak **92%** > cfr 82% ≈ vf 97%;
    jackson yak **18%** > cfr 16% ≈ vf 16%; spring yak 32% ≈ cfr 36% / vf 39%; fastjson2 yak 15% ~ cfr 12% ≪ **vf 79%**。
  - **绝对覆盖 (回填进可用 jar 的 class 数, §5 overlay)**: guava Yak **1092** ≫ cfr 430 / vf 953; codec Yak 98 > cfr 70 / vf 85。
    即 **Yak 实际成功回填的 class 比两个对手都多** (单元率被高分母拉低, 但绝对回填最多)。
  - **正确性**: 所有 jar/工具 **verify_fail=0** (spring 仅 1, 三家共有); guava **语义差分调用 = IDENTICAL**。
  - **missing_dep=0** (deps 已上 classpath), 故失败基本都是 decompiler_err; 但 spring 的
    `package javax.annotation does not exist` (jsr305 传递依赖缺失) 三家共有, 拉低 spring 全员、非 Yak 缺陷。
  - **明显偏低指标 (待探因)**: ① fastjson2 yak 15% ≪ vf 79% (当前最大单 jar 差距); ② jackson yak 18% (绝对值低,
    虽略胜 cfr/vf); ③ spring 32% (但含 jsr305 依赖缺失的全员共有噪声)。
- (历史 guava yak-syntax 重编译率: 7% → 标准库点号前 30% → 零参泛型返回+内部类自由类型变量+泛型(T)强转后 57.6%
  → 叠加「窄整型形参 descriptor 定型」**59%**)。
- **精确错误直方图** (整目录 `javac` 一次性编译 1892 个 guava yak-syntax 单元, `-Xmaxerrs` 不截断, locale=en,
  共 1212 条 error; 比逐单元抽样更准, 去掉了跨单元级联噪声) —— 下一步杠杆按此排序:
  - **enum 高级 idiom 重建 — 常量体 + marker 构造器均已治本 (核心 + jar 路径, 见 Bug V)**: 原 ≈200 条
    (`enum types are not extensible` 67 + `constructor <Enum> cannot be applied` ≈91 +
    `cannot inherit from final <Enum>` 8 + `call to this must be first statement in constructor` 21)
    主因是常量体合成子类 `<Enum>$N` 与合成 marker 构造器 `<Enum>(String,int,<Enum>$N)`; 本轮跨类折叠**已接入
    生产 jar 路径** (JarFS.ReadFile 内对 enum 用 resolver 折叠、对合成 `$N` 抑制独立产出), **marker 构造器亦已
    抑制** (清掉 `call to this must be first statement` 21 + 部分 `constructor cannot be applied`)。真实 guava
    `CaseFormat` (5 常量体, release-8 含 access$N + marker ctor) 实测折叠成单一合法 enum 且 round-trip 通过。
    待下次跑全量 1892-unit 直方图量化清零量 (本表数字是接入前旧值)。残留: enum-switch 脱糖复原 (低优, 非阻断,
    见 Bug V-2)。`access$N` 经查非缺陷 (保留声明即自洽可编译)。
  - `cannot find symbol` 214 (多为类型变量/同 jar 跨包嵌套残留, 多数需跨类整体重建, 见 Bug AD/V)。
  - 类型变量擦除成 `Object`/bound 出现在**非返回位置** (字段读/构造器形参赋值/方法实参):
    `Object→K/E/N/V/T/C/CAP` ≈ 196 + `Comparable→C/Cut<C>` ≈ 33 + `type argument not within bounds` ≈ 80。
    与「泛型返回 (T) 强转」同源, 但在赋值/实参位, 部分需 this$0 参数化即跨类重建。
  - `return outside method` 7 (见 Bug AC)。
  - boolean↔int 局部误定型 6 (见 Bug AI, 实际远小于此前抽样的 81)。

---

## 未修复缺陷 (下一轮治本目标)

### Bug V — enum 高级 idiom 跨类重建 (常量体已治本; 残留仅 enum-switch idiom 复原)

「把合成内部类折叠进外层类」的多类折叠能力**已落地并接入生产 jar 路径**, 常量体 idiom 已治本
(其承重测试 `TestEnumConstantBodyFoldIsLoadBearing` 单类 resolver 路径 +
`TestEnumConstantBodyFoldJarPathIsLoadBearing` jar 路径, 均带 `JDEC_NO_ENUM_FOLD` kill-switch, 即永久档案)。
实现散落: 附加式入口 `DecompileWithResolver` / `ClassObject.DumpWithResolver` (`parse_class_object.go`)、
折叠逻辑 `dumper_enum_fold.go` (`foldEnumConstantBodies` 从 `<clinit>` 的 `new Op$N(...)` 映射常量→子类,
内联方法体、剥离合成构造器、合并 import)、jar 接线 `fs.go` (`JarFS.decompileClassBytes`: 对真 enum 用
`enumSiblingResolver` 折叠, 对合成 `$N`(ACC_ENUM 且 super≠java.lang.Enum) 输出抑制标记, javac 重编译时自再生)。
另: **合成 marker 构造器抑制已治本** —— 带常量体的 enum (尤其 `--release 8` 等前 nestmates 字节码) 会有一个
javac 合成的 `<Enum>(String,int,<Enum>$N)` marker 构造器, 旧版渲染成 `this(var1,var2)` 垃圾 ctor (引用已折叠的
`$N`、且 `this(...)` 不在首句)。现于 `isSyntheticEnumMethod` 按「末参为本 enum 自身匿名子类 `L<self>$<digits>;`」
精确识别并抑制 (kill-switch `JDEC_NO_ENUM_MARKER_CTOR`), 承重测试 `TestEnumMarkerCtorSuppressionIsLoadBearing`。
注: 合成访问器 `access$N` 经查**不是缺陷** —— 反编译保留其方法声明且调用点一致引用, 自洽可编译 (实测 guava
`CaseFormat.access$100` round-trip 通过), 故无需内联还原。
实测: guava `CaseFormat` 折叠成单一合法 enum (5 常量体内联, `$StringConverter` 真嵌套类保留不抑制)。

**残留 (仅一项, 低优, 非阻断正确性)**: **enum-switch 脱糖复原** —— `switch(enumVar)` 被 javac 降级成合成类
`Outer$1` 内的 `$SwitchMap$<Enum>[]` 序数映射数组 + `enumVar.ordinal()` 查表。
- 症状: 反编译保留低层 `Outer$1.$SwitchMap$...[v.ordinal()]` 形态而非还原 `switch(enum)`。
- 现状: 外层与合成 `Outer$1` 两 class **同时**反编译并一起编译时可正常编译运行 (实测), 真实整 jar 会同时产出
  两文件, 故仅「美观/idiom 复原」, 非阻断。

### Bug Y (残留) — 纯分支内赋值的目标局部声明作用域过窄, 被读出作用域

- 嵌入式赋值 (int + 引用) 两形态均已治本, 其承重测试即档案 (battery `EmbeddedAssignDecl` /
  `EmbeddedAssignRef` + `TestEmbeddedAssignDeclIntIsLoadBearing` / `TestEmbeddedAssignRefIsLoadBearing`,
  kill-switch `JDEC_NO_EMBED_ASSIGN_INT` / `JDEC_NO_EMBED_ASSIGN_REF`)。
- **synchronized 块变体已治本**: `synchronized(lock){ ...; v=...; } return v;` —— javac 把 `return v` 编成
  load-v; monitorexit; areturn, 结构化器据此把 `return v` 作为 synchronized 语句的**兄弟节点**放到块外, 而 v 的
  声明仍在块内 → 读出作用域。现于 `rewrite_var.go` 的 `syncHoistDeclarations` (经 `hoistSwitchDeclarations` 接线)
  把「块内声明、块外被读」的局部声明提升到 synchronized 语句之前 (块内降级为纯赋值)。这是 guava base 整个
  cannot-find-symbol 重编译级联的单一根因 (`Enums.getEnumConstants`)。承重种子 `synchronized_return_scope.class`
  (`TestDecompileSyntaxRegression`)。
- 残留 (纯分支声明): `if(...) v=x; ... use(v);` 中 v 声明被限制在 if 块内 →
  `cannot find symbol`。这不是嵌入式赋值 (没有 `(v = ...)` 的条件内赋值形态), 而是普通分支内首次赋值后
  在分支外被读, 声明未提升到支配作用域。需把声明提升到支配作用域 / 跨分支合并同槽局部 (与 Bug W 同源)。
  (synchronized 变体已如上治本; 此残留专指 if/else 分支形态。)
- 复现: commons-codec `Metaphone` / `MatchRatingApproachEncoder` / `DaitchMokotoffSoundex` /
  `bm.Rule`。

### Bug AC — 控制流结构化产出 'return outside method'

- 症状: 某些控制流形态下出现位于方法体外的 `return`, javac 报 "return outside method"。
- 根因: 控制流结构化在特定 (含 switch/嵌套 try) 形态下越过方法边界生成语句。
- 复现: commons-codec `DaitchMokotoffSoundex` / `cli.Digest`。

### Bug AD (残留) — 非标准库外部依赖 / 同 jar 跨包嵌套类型按 `$` 引用

- JDK/标准库子集已治本 (`Map.Entry` 等); 同 jar 跨包扁平 `$` import 也已治本 (见承重测试)。以下残留仍未治:
  - **非标准库外部依赖 jar** 的嵌套类型 (如 `com.google.errorprone.*$*`, `org.checkerframework.*$*`):
    这些在 classpath 上是真正嵌套的, 应渲染 `.` + 外层 import; 但单类入口无法区分「同 jar 自有扁平单元」与
    「依赖 jar 外部类型」(两者都可能是 `com.google.*` 前缀), 故未扩到非标准库包。注: 该残留**不会**让原本可编译
    的单元退化 (扁平 `$` 引用在旧外层 import 下本就不可解析), 仅是这类单元仍编不过。
- 复现: guava 引用 `com.google.errorprone.annotations.*` 嵌套类型的单元。

### Bug AF — RuntimeInvisibleAnnotations 重新 marshal 时被写成 RuntimeVisibleAnnotations (pre-existing, 非反编译路径)

- 症状: 把含 `RuntimeInvisibleAnnotations` 的 class 解析后再 `obj.Bytes()` 序列化, 属性名索引被写成
  `RuntimeVisibleAnnotations` (#可见) 而非 `RuntimeInvisibleAnnotations`, 注解保留期从 invisible 翻成 visible,
  字节不一致 (GwtCompatible.class 偏移 0x275 即此)。
- 根因: `RuntimeInvisibleAnnotations` 复用 `RuntimeVisibleAnnotationsAttribute` 结构体解析 (见 `newAttributeInfo`
  注释), 但 `marshal.go` 的 `case *RuntimeVisibleAnnotationsAttribute` 硬编码 `findUtf8IndexFromPool(
  "RuntimeVisibleAnnotations")`, 无法区分可见/不可见。
- 影响范围: 仅 **重新 marshal Yak 解析后的 class** 路径; 反编译 (生成源码) 与 jar 重打包 (overlay javac 产物)
  两条用户路径均不经此, 故不影响当前重编译率/差分指标。治本: 解析时把真实属性名记入 struct 的 `Type` 字段,
  marshal 时据此选 `RuntimeVisibleAnnotations`/`RuntimeInvisibleAnnotations`。
- 复现: 任意带 `@Retention(CLASS)`/编译期注解的 class (如 guava `GwtCompatible.class`)。

### Bug AI (已定位 + 已取最小复现, 体量小) — 复用槽内局部变量被误定型成 boolean/int

- 症状 (整目录精确计数, 仅 ≈10 条): `int cannot be converted to boolean` 4 + `boolean cannot be converted
  to int` 6, 集中在 `MapMakerInternalMap$Segment` / `LocalCache$Segment` / `ImmutableSortedMap` /
  `LongMath` 等少数类。**自托管常见 boolean 形态 (位运算 & | ^、复合赋值 &=、三目、布尔数组、参数/返回) 全部
  干净 round-trip** (见 battery BooleanEdge / BooleanBitwise / BoolIntSlotReuse), 故本 bug 只在特定数据流出现。
- **最小复现 (本轮取得, 工作流第一相)**: 方法返回 boolean, 在 `return` 前把布尔结果**存入局部并跨一次方法调用**
  再读回 (`boolean r = false; this.touch(); return r;`); javac 把这个 boolean 局部复用到前面一个 int 临时量的
  同一槽 (slot 2)。Yak 输出 `boolean var2;` 后既有 `var2 = this.count - 1;`(int)/`this.count = var2;` 又有
  `var2 = false; return var2;`(boolean) → 两处类型冲突, 不可重编译。注意同方法的 else 分支 (`boolean var2_1 = true`)
  已被**名字冲突拆分**正确处理, 仅 if 分支的合并槽未拆。battery `BoolIntSlotReuse` 收录 Yak **能**正确处理的相邻
  变体 (布尔以字面量直接 return, 不经槽), 失败变体见此处描述。
- 根因 (已取实证): 同一 JVM 槽在不重叠活跃区间先后存放 int 与 boolean, 槽复用合并把它们并成**同一个**
  `VariableId` 并按其中一种 (boolean, 多由 ireturn / 返回类型主导) 定型。`dumper.go` 的名字冲突拆分
  (`declareLocalInScope`/`resolveLocalNameCollisions`) 只在「两个**不同** id 渲染成同名 varN」时触发, 此处是**单个**
  合并 id 跨两种类型, 故不触发。
- 治本难点 (为何本轮不强改): 真正治本是「复用槽按活跃区间拆分」, 必须在槽合并层 (`rewriter/rewrite_var.go`) 或
  dumper 做**基于支配/活跃性**的拆分; 槽合并层有专门的「禁止过度拆分」回归 (`regression_test.go` 的
  `var3_1 leaked` 等) 把握平衡, 贸然「遇冲突即不合并」会触发它们; 而在 dumper 做线性扫描拆分则在跨分支/循环
  时可能**静默错拆 (语义错误, 比编译报错更危险)**。故需先建活跃性分析再拆, 属与 Bug W/Y 同族的核心特性。
  体量小 (≈6-10 条), 优先级低于 enum 簇, 但已具备最小复现, 可作为活跃性拆分特性的首个承重用例。

### Bug AJ (新登记, 已取最小复现) — 两个不同槽的同型 int 局部被并成同一 varN, 致循环/返回引用错变量

- 症状 (语义错误, 编译期测不出 —— 类型一致仍能编过): `int kind` (跨分支两段活跃区, 循环内 + 返回处被读)
  与紧随其后的循环计数器 `int i` 是**两个不同 JVM 槽** (javac: kind=slot3, i=slot4), 反编译却把二者并成同一个
  `var3`: 循环体 `acc = acc*31 + kind` 与 `return (acc<<3) + kind` 里的 `kind` 全被渲染成计数器 `var3`(=i),
  数值算错。最小复现 (已实测): 见下源 (kind 在 if/else 各赋值一次, 循环内既用 kind 又自增 i, 返回再用 kind):
  ```java
  static int classify(String ref, int seed) {
      int acc = seed;
      int kind;
      if (ref == null) { kind = 1; acc += 7; }
      else { kind = 2 + (ref.charAt(0) & 7); acc += ref.length(); }
      int i = 0;
      while (i < 4) { acc = (acc * 31) + kind; i++; }   // 这里的 kind 被错成 i
      return (acc << 3) + kind;                          // 这里的 kind 被错成 i
  }
  ```
  javap 实证: LocalVariableTable 中 kind=slot3 (两段: Start8/Len6 + Start25/Len35), i=slot4, 且 kind[25,60)
  与 i[35,60) **活跃区重叠** → 必为不同槽, 不该合并。
- 根因 (待最终定位): 槽合并 / 变量命名层把 slot3(kind, 双活跃段) 与 slot4(i) 误并成单一 `var3`。疑似「kind 的
  第二活跃段」未被识别为 kind 而与 i 槽混淆, 或命名按声明序而非槽号致冲突未触发拆分。与 Bug AI/W 同属「槽×活跃区
  ×变量身份」族, 但方向相反 (AI 是单槽双型需拆; 此处是双槽被误并)。
- 现状: 未治本。该形态因「分支内首次赋值 + 后接循环」在真实算法 (Md5Crypt 已修的是其**分支顺序**, 变量身份仍可能
  踩此) 中可复发, 故先登记最小复现, 留作活跃性拆分特性的承重用例之一。注: NullCheckBranchThenLoop 种子已绕开此
  形态 (各局部独占槽), 以隔离已治本的分支顺序回归。

### Bug AH (新登记, guava base 唯一残留) — 类型变量擦除成 Object 出现在非返回位置 (需泛型沿数据流传播)

- 症状 (guava base 整目录精确计数: 3 条, 2 单元): 局部变量按字节码 descriptor 定型为 **raw**, 致其成员/返回值
  退化成 Object, 流入需要类型变量的形参时报「Object 无法转换为 CAP#N / T」:
  - `PairwiseEquivalence.doEquivalent/doHash`: `Iterator var3 = var1.iterator();` 中 `var1` 是 `Iterable<T>`,
    应推出 `Iterator<T> var3`, 则 `var3.next()` 为 `T`; 现 raw `Iterator` → `next()` 为 Object →
    `elementEquivalence.equivalent(var3.next(), ...)` (形参 `? super T`) 报错。
  - `Equivalence$Wrapper.equals`: `Equivalence$Wrapper var2 = (Equivalence$Wrapper)var1;` 应为 `Wrapper<?>`,
    则 `var2.reference` 为捕获类型; 现 raw → `var2.reference` 为 Object → `equivalent(this.reference, var2.reference)` 报错。
- 根因: 局部变量类型来自 descriptor (擦除态, 无泛型), 缺「按接收者类型实参实例化方法返回签名」与「checkcast 目标按
  上下文补泛型实参」的泛型传播。这是与「泛型返回 (T) 强转」同源、但发生在**赋值/局部声明**位的镜像问题, 全 guava
  直方图里属最大类 (≈196 `Object→K/E/V/T` + 33 `Comparable→C` + 80 `type argument not within bounds`)。
- 治本方向 (大特性, 须谨慎, 高回归风险): 在 dumper/类型恢复层做有限泛型实例化 —— 当局部 = `recv.m(...)` 且 recv
  有已知泛型类型、m 的 Signature 返回引用了 recv 的类型参数时, 用实例化后的泛型类型给局部定型 (而非 raw); checkcast
  目标同理按目标上下文补实参。须先有承重用例与 kill-switch, 避免误扩到无法静态判定的场景 (那比编译报错更危险)。
- 复现: guava `PairwiseEquivalence` / `Equivalence$Wrapper` (base 包仅此 2 单元残留)。

### Bug AE — 嵌套 enum 引用 / try-with-resources 抑制变量类型

- 症状: bm 包 `NameType`/`RuleType` 枚举引用 → `cannot find symbol: variable NameType`;
  try-with-resources 的抑制变量 `var10` 被误判为 String 致 `addSuppressed(Throwable)` 不可见。
- 根因: (前者) 同包顶层 enum 的类型引用被当作变量解析 (缺类型可见性/导入); (后者) twr 脱糖里
  `Throwable primaryExc` 槽类型推断错误。
- 复现: commons-codec `bm.{Lang,Languages,PhoneticEngine,Rule}` / `bm.Rule` (twr 抑制)。

---

## 待扩展覆盖 (向 GA 推进)

### 算法 battery 扩展
- [ ] UnixCrypt (DES crypt) / Sha2Crypt (SHA-512 crypt)
- [ ] 更多 spring / guava 形态 (随已修控制流能力逐步恢复自然写法, 持续探测新 bug)

### 真实库整库 round-trip (反编译 → 重编译成 jar → 反射差分调用)
- [ ] commons-codec 1.15: Base64/Base32/BaseNCodec (内部类 `$Context` + final 字段 + byte 收窄)、
  DigestUtils (final 字段赋值)、HmacUtils (try-catch 重建 + checked exception 声明)、
  UnixCrypt (变量重复定义)
- [ ] spring-core / guava 整库
- 依赖根因 (详见 YAK_JAVA_DECOMPILER_CROSS_COMPARISON.md §4.1): 协变桥接方法抑制 (根因 A)、
  嵌套类 `$` 引用归一 (根因 B)、泛型占位符 `__` (根因 C)

---

## 如何运行

```bash
# 本地快回归 (主闸门, CI 也跑这个)
go test ./common/javaclassparser/...

# 跑 codec 算法差分 (只要有 javac/java 就硬断言)
go test -run TestCodecSemanticsRoundTrip -count=1 -v ./common/javaclassparser/tests/

# OPCODE 解析 100% 覆盖门禁
go test -run TestOpcodeParseCoverage -count=1 -v ./common/javaclassparser/tests/

# 大型交叉对比 PK (需要 CFR/Vineflower jar + 语料)
CROSS_PK=1 CFR_JAR=... VINEFLOWER_JAR=... go test -run TestYakDecompilerCrossComparison ./common/javaclassparser/tests/
```
