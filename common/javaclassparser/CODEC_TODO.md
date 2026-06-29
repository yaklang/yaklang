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
- **差分门禁 `TestCodecSemanticsRoundTrip`: 65 个自托管 battery 全绿** (byte-for-byte round-trip), 覆盖
  控制流 / 排序 / 链式赋值 / SHA-256 / SHA-512 / TEA-XTEA-RC4 / String-switch / try-with-resources /
  try-finally-loop / 数值转换重命名 / iinc 宽化 / 循环计数器 slot 复用 / 嵌入式赋值 / Class 字面量 /
  int 类别收窄 / guava IntMath(gcd·log2·floor-sqrt·pow·isPrime·checkedAdd) / 严格 Hex 编解码(收窄+校验异常) /
  boolean-int 槽复用形态 / **空前导 case 的 tableswitch (Base32/Base64 EOF 形态)** /
  **对象-null 守卫后接底部测试循环 (Md5Crypt salt 形态)** /
  **短路 `(A&&B)||C` 中 C 为内联数组可变参调用 (DoubleMetaphone.conditionC0 形态)** /
  **if/else 双臂赋值且循环后读的逃逸槽 (Nysiis/Metaphone transcode 形态)** /
  **空-init Throwable holder 与异类局部跨支共槽 (twr primaryExc / catch-holder 形态)** /
  **null-init 槽仅允许首次类型采纳 (verbose twr `Throwable primaryExc=null` 提交 Throwable 后, 同槽被后继
  `Map.Entry e` 循环变量复用, 不相交活跃区间须分裂为两变量, DaitchMokotoffSoundex.<clinit> 形态)** /
  **switch case 内写、switch 后读的局部须前绑分裂出作用域 (fastjson2 DateUtils 手展开日期解析器形态, 每个 pattern case
  把规范化数字字符拷入一组局部, switch 后再校验)** /
  **单字符 `$` 标识符字段 (asm 混淆 MethodWriter 形态, javac 合法但 yak 文法词法器误当 Dollar token)** /
  **同槽三段不相交活跃区间 (if 臂 / else 臂 / if 后) 的跨作用域声明支配性 (FloatingDecimal / fastjson2 TypeUtils 形态)** /
  **if/else 双臂同类型同槽但跨 VarUid 的 phi (后继异类标量复用数组槽致 DFS 钳分裂两臂, fastjson2 ObjectReaderProvider 形态)** 等。已治本 bug 的
  文字记录按约定删除, 其承重 `Test*` + testdata 种子即永久档案。
- **OPCODE 解析覆盖门禁 `TestOpcodeParseCoverage`: 195/195 (100.0%)** (语料 126 class + 31 battery,
  命中 198 distinct opcode), 7 个文档化排除
  (jsr / jsr_w / ret / goto_w / wide / ldc_w / nop —— 均为 javac 不由源码产生或前缀修饰)。
- **全量工业语料健康度 (`TestM2StubReasons` `M2_INDUSTRY=1`, ~/.m2 全部 1107 jar, 每 jar 上限 120 类)**:
  **扫描 69149 类, ok 69146, 仅 5 处 stub (99.993% 干净)**, 仅剩 2 个失败桶:
  ① `post-decompile syntax validation failed` 3 处 (全在 asm-6.0_BETA `MethodWriter`, 单字符 `$` 标识符) —— **本轮已治本**;
  ② `ParseBytesCode failed: has circle` 2 处 (antlr `PredictionContext.toStrings` / commons-compress `TarUtils`) —— Bug AJ, 带标签 continue 跳 for-increment, 仍待重建。
  即治本 ① 后工业语料只余 Bug AJ 一类失败 (2 处)。
- **GA 里程碑 — guava `base` 整包「反编译→重编译→打回 jar→外部反射调用」= IDENTICAL (语义, 非仅编译)**:
  经生产 JarFS 路径重生 160 单元后整目录 `javac` (guava + 5 传递依赖上 classpath): **158/160 单元可重编译**
  (残留 2 单元 = Bug AH Group A, 临时手补 cast 后)。重编译产物覆盖回 overlay jar, 外部 `BaseProbe` 反射跑
  VerifyException / Predicates / Suppliers / **CaseFormat** / Ascii / Strings / Equivalence, 输出与原 jar
  **逐行 IDENTICAL**。其中 `CaseFormat` 此前 `StackOverflowError` (常量体 `super.convert` 被误渲染成
  `this.convert` 致无限递归) —— 这是**编译期测不出、必须运行**才暴露的语义 bug, 本轮已治本 (见下「本轮新增治本」
  第 4 项)。残留 2 条全部是 Group A「类型变量擦除成 Object 出现在非返回位置」(`PairwiseEquivalence` 的
  `Iterator var=it()` 应为 `Iterator<T>`、`Equivalence$Wrapper` 的 `Wrapper var=(Wrapper)o` 应为 `Wrapper<?>`);
  需把泛型类型沿数据流传播 (方法返回按接收者类型实参实例化), 属大特性, 见 Bug AH。
- **本轮新增治本 (codec 真实差分 + 工业语料新发现, 29 项, 均已锁回归种子, 种子即档案)**:
  - **iinc 同槽不相交活跃区间被 DFS 序污染: int 计数器槽被后继 long/float/double 复用 → 计数器 `i++` 误绑到出作用域的后继变量, `cannot find symbol` (Bug AL 收窄子族, fastjson2 -1)**
    (可用性/重编译, 单类反编译即可见, 整树重编译报 `cannot find symbol: variable varN`): fastjson2 `Fnv.hashCode64LCase` 的 slot5 先后承载 int 计数器 / int 字符 /
    `long` 哈希累加器三个不相交活跃区间; 前向模拟的单一全局 slot 表按 DFS 序把**后继 long 复生槽**泄漏回首循环计数器的 iinc, 使 `i++` 渲染成 `var5_1++`
    (绑到方法下方声明的 long 累加器), 在计数器处出作用域。治本是既有 iinc 到达定义修复 (`code_analyser.go` OP_IINC, `reachingSlotVersionByCategory`) 的**类别精化**:
    verifier 保证 iinc 槽在该点必为 int 类别, 故守卫由「仅引用类型泄漏」(`!refIsPrimitive(ref)`) 扩到「非 int 类别泄漏」(`!isIntCategoryNumeric(ref.Type())`, 覆盖
    long/float/double), 沿 Source 回walk 取最近到达定义、且**仅当其为 int 类别**才采纳 (引用情形回walk 命中的恒是 int 类别, 行为不变)。种子 pin 真实
    `testdata/regression/iinc_intcat_slot.class` (= fastjson2 `util/Fnv.class`), 承重 `TestIincIntCategorySlotRepairIsLoadBearing` (对 fastjson2 jar classpath 编译,
    信号=`cannot find symbol`: kill-switch `JDEC_IINC_REACHING_OFF=1` 时存储为 `var5_1++` 且报错、开启时回绑到 `var4++` 编译干净)。实测 **fastjson2 -1 (Fnv 整方法翻转干净;
    4-jar A/B 对比旧引用-only 基线 codec/guava/spring 零回归)**; 为「同槽活跃区间分割」大特性的一个可证 sound 子族 (姊妹杠杆 LOAD 侧见 Bug AL 根因方向)。
  - **类型变量字段存储漏铸 (Bug AH 汇点-cast 子族, return 侧之后第二个汇点) → `incompatible types: Object cannot be converted to K/V/E` (guava 整树 779→757 -22, fastjson2 -1, codec/spring 0 回归)**
    (可用性/重编译, 单类反编译即可见, 整树重编译报 `Object cannot be converted to K`): 形如 `private final K key;` 的同类字段, 字节码把字段类型擦除到其
    上界 (Object), 故构造器里 `this.key = var1.keys[var2]` (RHS 为裸 `Object[]` 元素读, 静态类型 Object) 缺源码本有的显式 `(K)` cast, javac 拒
    (guava CompactHashMap$MapEntry/EntryForKey、AbstractCache/AbstractLoadingCache 等)。治本与 `typeVarReturnCast` 同构、只作用于**同类字段存储**汇点:
    dumper 在 DumpMethods 前 (TypeParams 已知后) 从各字段的泛型 Signature (`TK;` 文本即判, 不渲染/不动 import 集) 建 `ClassContext.FieldTypeVars`
    (字段名→裸类型变量); `AssignStatement.String` 的 reassign 分支经 `typeVarFieldStoreCast` 探测 `this.field`/`ClassName.field` 左值命中该表、且 RHS
    非 null/非基本/未已渲染为该变量时, 裹 `(K)` cast。仅同类字段 (其类型变量唯本类已知) 触发, 跨类字段/实参侧仍需被调方泛型签名解析 (见 Bug AH 未决)。
    种子 pin 真实 `testdata/regression/typevar_field_store.class` (= guava CompactHashMap$MapEntry.class), 承重测试
    `TestTypeVarFieldStoreCastIsLoadBearing` (源码级: kill-switch `JDEC_NO_TYPEVAR_FIELD_CAST=1` 时存储为裸 `this.key = var...`、开启时出现 `this.key = (K) (`;
    因该单元是扁平 `Outer$Inner`、javac 17 独立编译会在 Flow$AliveAnalyzer 崩溃故不用 javac 信号, 重编译收益由整树 A/B `TestScratchJarErrDelta` 度量)。
    实测 **guava 整树 779→757 (-22)**, fastjson2 -1, codec/spring 无回归。
  - **类型变量数组汇点漏铸 (Bug AH 汇点-cast 子族, 数组臂; return 侧 + 字段存储侧两入口同治) → `incompatible types: Object[] cannot be converted to E[]/T[]` (guava 整树 757→731 -26, fastjson2 -1, codec/spring 0 回归)**
    (可用性/重编译, 单类反编译即可见): 形如 `public <E> E[] toArray(E[] var1){ return coll.toArray(var1); }` —— `Collection.toArray(E[])` 擦除返回 `Object[]`, 而方法
    返回类型是 `E[]`, 源码本有 `(E[])` cast; 又如 `final E[] rest;` 字段被 `this.rest = (Object[]) checkNotNull(var2)` 存入, 同缺 `(E[])`。治本是字段存储/return
    两个既有汇点的**数组泛型自然延伸**: 字段签名解析放宽到 `[TK;`/`[[TK;` (`dumper.go` 剥前导 `[` 后判 `TK;`, `FieldTypeVars` 存渲染型 `E[]`/`E[][]`);
    `typeVarReturnCast` 新增 `arrayTypeVar` 触发 (返回型是裸类型变量数组 `E[]`, 由 `isArrayOfTypeParam` 判定), 与裸类型变量同样无条件铸, 但加「RHS 必须是数组或
    Object」守卫确保 `(E[]) value` 合法 (擦除后 `E[]`=`Object[]`, unchecked 但语义保持)。种子 pin 真实 guava `Lists$OnePlusArrayList.class` (字段存储) +
    `LocalCache$AbstractCacheSet.class` (return), 承重 `TestTypeVarArrayCastIsLoadBearing` (源码级双向 pin, kill-switch `JDEC_TYPEVAR_RET_CAST_OFF` /
    `JDEC_NO_TYPEVAR_FIELD_CAST`)。实测 **guava 整树 757→731 (-26: return 侧 4 单元 + 字段侧 2 单元 + 级联)**, fastjson2 -1, codec/spring 无回归。
  - **类型变量实参侧伪上行 cast 抑制 (Bug AH 实参侧, 与汇点-cast 互补的「该删的 cast」方向) → `incompatible types: Comparable/Object cannot be converted to C/K/E/V/N` (guava 整树 731→699 -32, fastjson2/codec/spring 0 回归)**
    (可用性/重编译, 单类反编译即可见): 形如 `abstract class AbstractRangeSet<C extends Comparable>` 的 `contains(C var1){ return this.rangeContaining(var1)!=null; }` 调用同类
    `rangeContaining(C var0)` —— 描述符把形参擦除到上界 (Comparable), 反编译器据「实参渲染类型 (类型变量 C) ≠ 形参描述符类型 (Comparable)」合成伪 cast
    `rangeContaining((Comparable)(var1))`; 但类型变量值在字节码里本就**不带 checkcast** 入栈 (擦除即上界), 故源码无需任何 cast, 该上行 cast 反而在 javac 绑定到更精确的泛型
    签名后被拒 (`Comparable cannot be converted to C`)。治本 (`expression.go` `ArgumentStrings` 的 cast 合成点经 `suppressTypeVarArgCast`): 当**实参静态类型是在域类型变量
    (`IsTypeParam(arg.RawType().Name)`) 且形参期望类型不是类型变量** (即被擦成具体上界) 时, 抑制该 cast。收窄判据保证 sound: 显式源码 downcast 会把实参静态类型改成被 cast
    的具体类 (不再是类型变量), 故不会误抑制真实 cast; 形参仍是类型变量的 `<X> m(X)` 形态保留原 cast。种子 pin 真实 guava `AbstractRangeSet.class`, 承重
    `TestTypeVarArgCastSuppressionIsLoadBearing` (源码级双向 pin, kill-switch `JDEC_NO_TYPEVAR_ARG_NOCAST`: 关闭出现 `rangeContaining((Comparable)(`, 开启为裸
    `rangeContaining(var1)`)。实测 **guava 整树 731→699 (-32)**, fastjson2/codec/spring 无回归。注: 实参是**具体类**而非类型变量的同族 (如 enum `state().compareTo((Enum)(State.X))`
    伪上行到 `Enum`) 不被本治本覆盖 (实参非类型变量), 仍属 Bug AH 实参侧余量。
  - **签名多态 (`@PolymorphicSignature`) `MethodHandle.invoke`/`invokeExact` 调用漏铸返回类型 → `incompatible types: Object cannot be converted to <T>` (fastjson2 -30, 本轮单点最大 1-错族)**
    (可用性/重编译, 单类反编译即可见, 整树重编译报 `Object cannot be converted to BiFunction/LongFunction/...`): `MethodHandle.invoke`/`invokeExact`
    在源码层声明 `Object invoke(Object...)` 且带 `@PolymorphicSignature` —— javac 为每个调用点合成专属描述符 (`invokeExact:()Ljava/util/function/BiFunction;`),
    故字节码返回类型是**真实类型**, 但**源码可见返回类型恒为 Object**, 原始源码必带显式 `(BiFunction) handle.invokeExact()` cast。反编译器从 (真实) 描述符
    读返回类型, 遂渲染成 `BiFunction var3 = handle.invokeExact()` 无 cast, javac 拒。治本: `FunctionCallExpression.String` 经
    `polymorphicSignatureCastType` 探测 `java.lang.invoke.MethodHandle.invoke/invokeExact` 且描述符返回非 `void`/`Object` 时, 把整调用裹回
    `(T)(...)` cast (`renderCall` 出原调用)。形态恒为 `LambdaMetafactory...getTarget().invokeExact()` (JSONReader$BigIntegerCreator/JdbcSupport/DoubleToDecimal 等)。
    种子 pin 真实 `testdata/regression/polysig_invokeexact.class` (= fastjson2 JSONReader$BigIntegerCreator.class), 承重测试
    `TestPolymorphicSignatureCastIsLoadBearing` (对 fastjson2 jar classpath 编译, 信号=`cannot be converted to BiFunction`:
    kill-switch `JDEC_NO_POLYSIG_CAST=1` 时 present、开启时消失且源码出现 `(BiFunction)(`)。实测 **fastjson2 整树 746→716 (-30)**,
    guava/codec/spring 无回归 (A/B `TestScratchJarErrDelta`)。
  - **if/else 平行 phi「孤儿读」异渲染类型 LUB 子族 (cast 守卫) → `cannot find symbol: variable varN` (fastjson2 ObjectWriters.fieldWriterList / JSONStreamReaderUTF{8,16}.readLineObject 形态, fastjson2 -5、guava -3)**
    (可用性/重编译, 单类反编译即可见, 整树重编译报 cannot find symbol): 一个 jvm 槽在 if/else (可嵌套) 多臂各以**不同渲染类型**首声明
    (`ParameterizedType var3` vs `ParameterizedTypeImpl var3`; 或三臂 `Object`/`List`/`Object var2`), join 后仅经**显式 cast** 读
    (`(Type)(var3)` / `(T)(var2)`)。`parallelArmDeclHoist` 仅在两臂渲染类型 token 一致时合并, 异类型留待公共超类型设施 (本反编译器
    无跨类层级解析), 故各臂保留己声明、join 后读出作用域 → cannot find symbol。真 LUB 需跨类层级无法求得, 故治本**不猜** join 类型:
    仅当该槽**每一处非声明用法都是显式 cast** (或裸赋值左值) 且无任一声明为标量基本类型时触发 —— 此形态下 `Object varN = null;`
    **必然**类型正确 (各臂存储接纳任意引用、各读 downcast), 且各臂存储仍保留自身 RHS 类型。新增 dumper 文本 pass
    `hoistCastGuardedEscapedLocals` (在 `addMissingGeneratedLocalDecls` 之前): 按缩进 (dumper 每层一 tab) 探测「声明全在更深内层、
    却有更浅层 cast 读」的逃逸槽, 把各内层 `T varN = rhs` 降级为 `varN = rhs`、注入单条 `Object varN = null;`。任何其它用法
    (成员访问 `varN.f`、下标 `varN[i]`、未 cast 实参/返回、算术/关系) 令 Object 不 sound → 该槽**不碰** (文件保留其原有单错, 零回归)。
    种子 pin 真实 `testdata/regression/cast_escape_phi_orphan.class` (= fastjson2 ObjectWriters.class), 承重测试
    `TestCastEscapeHoistIsLoadBearing` (对 fastjson2 jar classpath 编译, 信号=孤儿读 `variable var3` 的 cannot-find-symbol:
    kill-switch `JDEC_CAST_ESCAPE_HOIST_OFF=1` 时 present、开启时消失且源码出现 `Object var3 = null;`)。实测
    **fastjson2 整树 751→746 (-5)、guava 782→779 (-3)**, codec/spring 无回归 (A/B `TestScratchJarErrDelta`)。
  - **try/catch 值-兜底 phi 到达定义未合并 → 读出来的恒是 null 默认支、真值被丢且类型错 (Bug AN, 语义+可用性, fastjson2 -8) —— FieldReader{Double,Float,...}Func 族**
    (语义错: 编译期外加 `Object cannot be converted to Double/V`, 运行期读到的恒为 null): 一个 JVM 槽在 try 体内被赋真值 (`v = readDouble()`)、
    在 catch 处理器内被赋 `= null` 兜底, try 后读 (`function.accept(obj, v)`) —— 同一逻辑变量。但 catch 仅异常时执行**不支配** try store,
    `code_analyser.go` 的 null-adopt 支配门 (`blockNullAdopt`) 遂阻止采纳, try store 铸出**另一个**异型变量 (`Double` vs catch 的 `Object`),
    try 后读经单一全局 slot 表 (DFS 序) 绑到 null 那支 → 真值丢失 + javac 报 `Object→Double`。二者 VarUid 不同, rewriter
    `prebindSharedTryCatchSlots` (按 VarUid 分组) 亦无法救。治本: store 处加 `reachingTrySlotPhiMerge`(内联实现) —— 当 `slotDefPhiReachesLoad`
    (一个被 try store 与 null-init def **双向到达**的下游 load) 证明二者汇入同一变量时, 放开 `blockNullAdopt`, 续用支配定义并采纳真值类型 (`Double var4;`)。
    **复用已有可信 phi 判据**, 故不相交复用反例 (twr `Throwable primaryExc=null` 后续复用为无关循环变量, DaitchMokotoffSoundex) **无公共下游 load、phi 不过、仍拆分** (承重 `TestTwrSlotReuseNullAdoptOnceIsLoadBearing` 仍绿)。
    种子 pin 真实 `testdata/regression/try_slot_phi_merge.class` (= fastjson2 FieldReaderDoubleFunc.class), 承重测试
    `TestTrySlotPhiMergeIsLoadBearing` (信号: kill-switch `JDEC_TRY_SLOT_PHI_MERGE_OFF=1` 时 `Object→Double` present、开启时消失且源码出现单一 `Double var4`)。
    实测 **fastjson2 整树 759→751 (-8)**, guava/codec/spring 无回归 + 65 battery 语义 round-trip 全绿 (无静默语义损伤) + 确定性保持。
    **后续**: BigIntegerCreator.<clinit> 一类 (entry `=null`、try `=invokeExact()`、if 后 `=new X()`) 的**编译错**根因经查实为上一项「签名多态 invokeExact 漏 cast」
    (try store 的 `BiFunction var3 = invokeExact()` 缺 `(BiFunction)` cast), 已随该项治本 (fastjson2 -30), 此处不再遗留编译错。其槽分裂 (三 def 未并)
    属纯**语义保真**残留 (invokeExact 真值落入死局部、最终用 `new X()`), 二者运行等价、不阻编译, 归 Bug AL 语义保真长尾。
  - **空-body void 方法被整体丢弃 → 子类不 override 抽象方法 / API 缺失 (fastjson2 -60、guava -47, 本轮单点最大杠杆)**
    (可用性/重编译, 单类反编译即可见, 整树重编译报 `is not abstract and does not override abstract method` / `cannot find symbol`):
    `DumpMethods` 对「body 反编译为空」的非构造、非抽象/接口/枚举方法, 一旦其返回类型为 `void` 即 `continue` **整条丢弃**。
    但 javac 对 `void f(){}` (字节码仅一条 `return`) 是**忠实的空 override**: 形如 `ObjectWriterBaseModule$VoidObjectWriter`
    的空 writer 其唯一真方法 `void write(JSONWriter,Object,Object,Type,long){}` 被丢后, 该类无任何方法 → javac 报
    「is not abstract and does not override abstract method write(..) in ObjectWriter」; 更广地, 任何空-body 的接口/抽象方法
    no-op 实现被丢都会令子类缺 override 或调用点缺 API。治本: 新增 `methodBodyIsTriviallyEmpty` —— 仅当方法字节码全由
    `nop`(0x00) 填充加**恰一条** `return`(0xb1) 构成 (即真·空 void body) 才保留发出 `void f(..){}`; 该 `{nop,return}` 集判定
    sound (任何带操作数的 opcode 自身不在此集, 其存在即被检出并排除, 故只保留真空体, 反编译丢失了真实内容的「假空」仍按旧逻辑丢弃)。
    种子 pin 真实 `testdata/regression/empty_void_override.class` (= fastjson2 VoidObjectWriter.class), 承重测试
    `TestEmptyVoidOverrideEmittedIsLoadBearing` (对 fastjson2 jar classpath 编译, 信号=`does not override abstract method write`:
    kill-switch `JDEC_NO_EMIT_EMPTY_VOID=1` 时 present 且源码无 `void write(`、开启时该 override 出现且编译干净)。
    实测 **fastjson2 整树 819→759 (-60)、guava 829→782 (-47)**, codec/spring 无回归 (A/B `TestScratchJarErrDelta`)。
  - **抽象方法的 varargs 末参渲染成「数组类型 + ...」(`Feature[]...`) 而非「元素类型 + ...」(`Feature...`) → 子类
    重写报「is not abstract and does not override」(fastjson2 JSONPath.set 抽象 varargs 整族, 整树 -8 / 翻转 6 文件)**
    (可用性/重编译, 单类反编译即可见, 整树重编译暴露子类不 override): javac 把 varargs 末参编成数组 descriptor (`[J` /
    `[LFeature;`) 并置 ACC_VARARGS。**拼接式具体方法 / lambda SAM / stub 三条渲染路径都正确剥离一层数组维度** (用
    `pt.ElementType().String()+"..."`), 唯独 dumper.go 的**抽象方法专用**分支 (`paramsNewStr=="" && abstractMethod`)
    漏剥离, 直接 `t.String()`(完整数组类型) 拼 `...` → `Feature[]...`。`Feature[]...` 是 `Feature[]` 的 varargs
    (descriptor `[[LFeature;`), 与子类忠实渲染的 `Feature...` (descriptor `[LFeature;`) **不再 override-equivalent**,
    故 javac 判子类未重写抽象方法 → 整族子类全部不可编译。治本: 抽象分支镜像其它三路, 末参 varargs 且 `t.IsArray()` 时
    用 `t.ElementType().String(funcCtx)+"..."`。种子 inline `varargsAbstractOverrideSource` (顶层 sibling 抽象基类
    `VAOBase` 声明 `combine(long,long,long...)` + 具体子类 `VAOImpl` 重写, 避开 `$` 嵌套噪声), 承重测试
    `TestVarargsAbstractMethodRenderIsLoadBearing` (多类整树 round-trip, kill-switch `JDEC_VARARGS_ABSTRACT_FIX_OFF=1`
    跑负向: 关闭即复现 `VAOImpl is not abstract and does not override abstract method combine(long,long,long[]...)`).
    实测 **fastjson2 整树 839→831, 干净文件 552→558 (翻转 JSONPath/JSONPath$RootPath/JSONPath$PreviousPath/
    JSONPathSingle/JSONPathTyped/JSONPathTypedMulti/JSONPathCompilerReflect$SingleNamePathTyped 6 个)**, codec/guava/spring 无回归。
  - **final 字段初始化器误把构造器局部 (碰撞改名 `varN_M`) 上提成字段 initializer → `cannot find symbol: variable varN_M` (fastjson2
    SymbolTable.hashCode64 / FactoryFunction.function 形态, 整树 -5 / 翻转 2 文件)** (可用性/重编译, 单类反编译即可见, 整树重编译报
    cannot find symbol): final 字段在构造器尾部由一个**同槽活跃区间分裂**而被改名为 `varN_M` (如 `var7_1`) 的局部赋值
    (`this.hashCode64 = var7_1;`)。`dumper.go` 的 `canHoistFieldInitializer` 守卫用 `localSlotRefRe` 探测 RHS 是否引用反编译生成局部,
    若引用则**禁止上提** (局部不在字段 initializer 作用域内); 但旧正则 `\bvar\d+\b` **匹配不到** `var7_1` —— `_` 是 word 字符, 故
    `\b` 词界在 `var7` 与 `_1` 之间不成立, 整个 `var7_1` 不被视作局部引用 → 守卫漏判 → 赋值被误上提成 `private final long hashCode64 = var7_1;`,
    而 `var7_1` 是构造器作用域局部, 字段 initializer 处不可见。治本: `localSlotRefRe` 放宽为 `\bvar\d+(?:_\d+)*\b`, 同时匹配 `varN`
    与碰撞改名 `varN_M` (`var7_1`/`var5_1` 等), 使守卫保守化、把这类赋值留在构造器体内 (blank final + 构造器内赋值, 合法 Java)。
    种子 pin 真实 `testdata/regression/final_field_renamed_local.class` (= fastjson2 SymbolTable.class, 合成 javac-17 battery 无法稳定
    复现同槽活跃区间分裂产出的 `varN_M`, 故按 twr-class 先例直接 pin 真类), 承重测试 `TestFinalFieldRenamedLocalHoistIsLoadBearing`
    (反编译 SymbolTable + 手写最小 `Fnv` stub 一起 javac, kill-switch `JDEC_FIELD_HOIST_RENAMED_LOCAL_OFF=1` 跑负向: 关闭即复现
    `SymbolTable.java:13: cannot find symbol: variable var7_1`)。实测 **fastjson2 整树 831→826, 干净文件 558→560 (翻转 SymbolTable/
    FactoryFunction)**, codec/guava/spring 无回归。
  - **if/else 平行 phi「孤儿读」(同槽跨 VarUid、两臂各自首声明、join 后被读) → `cannot find symbol: variable varN` (fastjson2
    FieldWriterListFunc.writeValue 等, 整树 -8 / 翻转 2 文件)** (可用性/重编译, 单类反编译即可见, 整树重编译报 cannot find symbol):
    一个 jvm 槽被复用为两个**不同逻辑变量** (不同 VarUid), 各自在 if/else 两臂顶层首声明 (`ObjectWriter var10 = var5` vs
    `var10 = getItemWriter(..)`), join 后又被读 (`var10.write(..)`)。`ifHoistDeclarations` 按 VarUid 分组, 两臂 VarUid 不同故
    从不配对 → 各臂保留自己的 `ObjectWriter var10 = ...`, 而 join 后的读 (反编译器内部绑到**第三个 merge id**、但渲染成同槽名
    `var10`) 无支配声明 → javac 报 cannot find symbol。治本: 新增 `parallelArmDeclHoist` (在 `hoistSwitchDeclarations` 晚期 pass、
    紧随 `ifHoistDeclarations`), 纯**按名**、不动任何 id —— dumper 端到端按渲染名绑定 (`addMissingGeneratedLocalDecls` 按 `varN`
    token 去重、javac 本身按名绑), 故只需在 if 前提一条支配性 `T varN;`、把两臂降级成 `varN = ...`: 唯一存活的声明令该槽名「已声明」,
    两臂赋值与孤儿读全渲染 `varN` 并按名绑到它, 缺声明兜底网见名已声明遂不再注入。**join 类型用「两臂渲染声明类型 token 一致」判定**
    (`renderedArmDeclType`, 不用 `ref.Type()` —— 本 pass 早于 dumper 末段 RHS 定型, 某臂 stale `ref.Type()` 仍是 Object 却能渲染
    `ObjectWriter var10 = objectWriterValued`); 渲染一致即反编译器自证「一条 `T varN;` 同时接纳两臂存储且支持 join 后用法」, 裸声明
    取自天然带该类型的那一臂。真正不同的渲染类型 (LUB 场景: ParameterizedType vs ParameterizedTypeImpl、Long vs BigDecimal) **不碰**
    —— 早期试过对不同类型一律 widen 到 Object, 实测 fastjson2 **+10** (臂内类型相关用法 `varN.foo()` 被 Object 打断), 已撤。种子 pin 真实
    `testdata/regression/parallel_arm_phi_orphan.class` (= fastjson2 FieldWriterListFunc.class), 承重测试
    `TestParallelArmPhiOrphanHoistIsLoadBearing` (对 fastjson2 jar classpath 编译, 信号=孤儿读 `variable var10` 的 cannot-find-symbol:
    kill-switch `JDEC_PARALLEL_ARM_HOIST_OFF=1` 时present、开启时消失; 单类隔离编译的 `JSONWriter$Feature` `$` 嵌套噪声属 Bug AD 无关)。
    实测 **fastjson2 整树 827→819, 干净文件 560→562 (翻转 FieldWriterListFunc 等)**, codec/guava/spring 无回归。
  - **内联 lambda body 自有局部与外层方法 varN 命名冲突 → 重编译报 `variable varN is already defined in method` (fastjson2
    ObjectWriterCreatorASM / 任意捕获 forEach 形态; Bug AL lambda-inlining 子族)** (可用性/重编译, 整库 round-trip 暴露):
    javac 把每个 lambda 编成独立 `lambda$...` 合成方法, 其 body 局部从**全新 jvm 槽命名空间** (slot0,slot1,...) 起算;
    反编译把该 body 作为箭头表达式**内联**回外层方法, 这些槽遂渲染成 var0,var1,... —— 与外层方法自己的形参/局部 (也是
    var0,var1,...) **同名**。Java 禁止 lambda body 内声明的局部遮蔽在作用域内的外层局部, 故朴素内联产出
    `int var0 = ...;` 遮蔽外层 `var0` 形参, 重编译失败。治本: `renameLambdaBodyLocals` 把每个 lambda body 自有局部
    抬入私有 `lv<seq>_N` 命名空间 (seq 为 `lambdaLocalSeq` 自增, 每个内联 body 唯一); 捕获变量在 body 内是**占位符**,
    由调用点在 rename **之后**替换为外层 varN, 故 rename 只动真正的 lambda 局部、不碰捕获 (实测 fastjson2 ON 输出 0 个
    `cannot find symbol: lvN` 即证)。种子 battery `LambdaLocalShadowsCapture.java` (零捕获 IntSupplier, body 局部 acc/i
    落 slot0/1 与外层 compute 形参/局部 var0/var1 同号, 进 `TestCodecSemanticsRoundTrip` 差分门禁), 承重测试
    `TestLambdaLocalRenameIsLoadBearing` (kill-switch `JDEC_NO_LAMBDA_LOCAL_RENAME=1` 跑负向: 关闭即复现
    `variable var0 is already defined in method compute(int)`)。**度量 (关键, 见下「整库度量方法学订正」)**: 该治本消除
    fastjson2 整库 **41 个 `already defined`** (其单文件隔离编译 ObjectWriterCreatorASM 267→250 错, 真实改善), 但整树
    `javac` 总错数反而 492→852 —— 此为**遮蔽解除 (un-masking)**: 旧版那 41 个 `already defined` 令 javac 放弃归属对应
    方法体, 把其中既有的 Bug AL/AH 缺陷一并压住; 治好遮蔽后这些缺陷 (约 +270 `cannot find symbol`) 现形。即治本正确且为
    那些文件达 GA 的**前置条件**, 整树总数上升是诚实暴露、非回归 (per-file 隔离 recompile 率 ON=OFF=344/681, 该治本
    本身不翻转任何文件, 但为后续 Bug AL/AH 治本铺路)。
  - **switch case 内写、switch 后读的局部未前绑分裂 → 声明被困在 case 内、switch 后读出作用域 (fastjson2
    DateUtils 手展开日期解析器形态, 整库重编译 -72, 本轮单点最大杠杆)** (可用性/重编译, 整库 round-trip 暴露,
    重编译报 `cannot find symbol: variable varN`):
    `rewriteVar` 对 switch 的所有 case 复用**同一个** sub-scope, 故各 case 的写已收敛到单一 id; 但 switch 之后的
    **读**位于父作用域, 仍持槽的原始 (未铸) id。case 写的铸后 id 与读的原始 id 不一致, 故 `switchHoistDeclarations`
    的 identity「switch 后读」探针与 `placeCrossScopeDeclarations` 都看不到该读 —— case 内的 `T x = ...` 声明被困在 case
    里, switch 后的读遂出作用域。典型形态: fastjson2 `DateUtils.parseLocalDateTime` 把每个 pattern case 的规范化日期/
    时间数字字符拷入一组局部 (`char y0=cs[0]; ...`), switch 之后统一做 `'0'..'9'` 校验与整型换算。治本: 仿照既有
    `prebindEscapingIfElseSlots` / `prebindEscapingSwitchSlots` —— 在下降进 case 前, 把「case 内写且 switch 后读」的槽
    (按 VarUid 归组, 即同一逻辑变量; 栈模拟在槽因类型复用时铸新 ref/VarUid, 故真正的不相交槽复用不会被合并) 前绑到一个
    父作用域新 id: case 写经 hasNamed 路径就地复用之, 并记 origId→newId 让父作用域延迟 ReplaceVar 重定向 switch 后的读,
    全部引用收敛到一个 id; `switchHoistDeclarations`/`placeCrossScopeDeclarations` 随即把单个 `T x;` 提到 switch 前。
    严格门禁: 按 VarUid 归组 (避免合并异类槽复用) + 必须 switch 后被引用 (origId sentinel 改名探针, 排除同名异 id 的无关
    局部) + 跳过 switch 前已被外层作用域绑定的槽 (普通局部仅在 case 内再赋值)。种子 battery `SwitchCaseLocalReadAfter.java`
    (switch 各 case 内写 a/b/c, switch 后校验读取, 进 `TestCodecSemanticsRoundTrip` 差分门禁), 承重测试
    `TestSwitchCaseLocalReadAfterIsLoadBearing` (kill-switch `JDEC_SWITCH_PREBIND_OFF=1` 跑负向: 关闭即复现
    `cannot find symbol`)。jasperreports `ExcelAbstractExporter.getTextAlignHolder` 回归用例的两个对齐局部因此从
    碰撞改名 `var2`/`var2_1` 变为更干净的 `var2`/`var3` (仍提为方法顶 bare 声明, 已同步更新断言)。实测 **fastjson2 整 jar
    重编译错误 -72 (564→492)**, codec/guava/spring 无回归。
  - **null-init 引用槽仅允许「首次」类型采纳 → verbose try-with-resources 合成 `Throwable primaryExc=null` 与后继
    `Map.Entry e` 循环变量共用同槽时被合并成单一误型变量 (commons-codec DaitchMokotoffSoundex.<clinit> 形态, fastjson2 主导杠杆)**
    (可用性/重编译, 整库 round-trip 暴露, 重编译报 `Throwable cannot be converted to Map.Entry` + `cannot find symbol: addSuppressed`):
    旧版 javac 的 try-with-resources 脱糖产出一个独立的 `Throwable primaryExc = null` 方法级槽 (现代 javac 直接复用 catch 形参,
    不复现), 该槽在 twr 合成 catch 里提交为 Throwable (`primaryExc = t; ... primaryExc.addSuppressed(..)`), twr 结束后 javac 把同槽
    复用给后继 `for (Map.Entry e : ...)` 的循环变量。`JavaRef.IsNullInitialized()` 判据是「Val 仍是 null 字面量」, 而
    `ResetVarType` 只改声明类型、**不改 Val**, 故该 ref 终生被视作 null-init —— `AssignVarGuarded` 遂让它**反复采纳**: 先采纳
    Throwable, 后又采纳 Map.Entry, 把两个活跃区间不相交的不同变量塌缩到单一声明 `Map.Entry var1 = null`, 于是 `var1 = <throwable>`
    与 `var1.addSuppressed(..)` 对 Map.Entry 非法。治本: `JavaRef` 增 `nullTypeAdopted` 标志, `AssignVarGuarded` 的 null-adopt 捷径
    只在**尚未采纳过**时生效, 首次采纳后置位; 此后同槽的不兼容 store 落入铸新变量 (块作用域局部) —— primaryExc 保持 Throwable,
    循环变量分裂为独立局部。与既有 `nullInitDefDominates` 支配门 (处理 null-init 在**异支**、首次采纳即跨支误并) 互补:
    支配门管「首次采纳即在异支」, 本守卫管「提交后再次复用」。种子 battery `TwrSlotReuseNullAdopt.java` (手写 verbose twr 脱糖 +
    嵌套块释放槽 + 循环变量多次使用迫其入槽, 用现代 javac 即复现旧形态), 承重测试 `TestTwrSlotReuseNullAdoptOnceIsLoadBearing`
    (kill-switch `JDEC_NO_NULL_ADOPT_ONCE=1` 跑负向); 既有 `TestNullInitSlotReuseIsLoadBearing` 重构为四组合 (gate-only /
    guard-only / both-off / both-on) 同时隔离两机制各自承重。实测 **fastjson2 整 jar 重编译错误 -39 (608→569)**, **codec -3 (4→1)**,
    guava/spring 无回归。
  - **嵌入赋值孤儿局部的合成声明类型推断错误 → 引用 `==`/`!=` 被误判 int + `obj.getClass()` 值未恢复 Class (fastjson2 JSONWriter.checkAndWriteTypeName 的 objectClass 形态)** (可用性/重编译, 单类反编译即可见, 重编译报 `bad operand types` + `int cannot be dereferenced` + `getTypeName(Object)` 不兼容):
    `(c = obj.getClass()) != type` (字节码 `... getClass; dup; astore; ... if_acmpeq`) 的 dup-collapse 丢掉 `c` 的独立声明,
    交由 `dumper.go` 文本安全网 `addMissingGeneratedLocalDecls` 合成。两处推断缺陷叠加: ① `generatedLocalLooksInt` 的裸比较
    基模式把 `==`/`!=` 与关系运算 (`< > <= >=`) 捆在一起当 int 类别, 致 `(c) != (HashMap.class)` 这种**引用相等比较**被误判 int
    (而 Java 的 `==`/`!=` 对引用合法, 仅当右操作数是**数值字面量**才证明 int —— 文档既有意图, 基模式却违背); ② 即便不误判 int,
    `inferGeneratedLocalRefType` 也无法从 `obj.getClass()` (实例方法调用, `bareCallRHSRe` 因前导 `obj.` 不匹配) 恢复类型 →
    默认 `Object c`, 后续 `c.getName()`/`getTypeName(c)` 不可编译。治本: ① 基模式拆分 —— 关系运算仍恒 int, 相等运算仅当右操作数
    为数值字面量 (`\(?-?\d`) 才 int; ② `inferGeneratedLocalRefType` 新增 `.getClass()→Class` 恢复 (getClass 恒返回 java.lang.Class,
    裸 Class 对所有用法都可重编译)。种子 battery `EmbeddedAssignGetClass.java` (循环条件嵌入 `(c = objs[i].getClass()) != exclude`、
    体内 `c.getName()`, 进 `TestCodecSemanticsRoundTrip` 差分门禁), 承重测试 `TestEmbeddedAssignGetClassIsLoadBearing`
    (kill-switch `JDEC_NO_EMBED_ASSIGN_REF=1` 跑负向: 关闭即退回 `Object c` → `c.getName()` cannot find symbol)。实测 fastjson2
    整 jar 重编译错误 -8 (685→677), codec/guava/spring 无回归。
  - **窄基本类型字段/局部「再赋值」(非声明) 由非常量 int 类别表达式写入时未补窄化 cast → `possible lossy conversion from int to char` (fastjson2 JSONWriter.quote / JSONReaderUTF8 字符写入形态)** (可用性/重编译, 单类反编译即可见, 重编译报 lossy conversion):
    `this.quote = single ? '\'' : '"';` —— javac 把该条件式定型为 char (两臂均 char 字面量), 降级成「分支压入 char 常量
    (JVM 操作数栈无 char 类别, 按 int 压栈) + `putfield ... C`」, putfield 隐式截断故**不发 i2c**; 反编译遂把两臂恢复成 int
    字面量、条件式定型 int。原样渲染即 `this.quote = single ? 39 : 34`, 在赋值上下文被 javac 拒 (JLS 5.2; 非常量条件式不是
    常量表达式, 不享 `char q = 39;` 的常量收窄豁免)。根因: `AssignStatement.String` 的**非声明 (reassignment) 分支**此前
    不做窄化, 而声明分支 (`narrowingInitCast`) 与数组元素写 (`arrayStoreRHS`) 早已补 cast。治本: 在非声明分支镜像同一
    `narrowingInitCast` —— 当 LHS 为 char/byte/short 且 RHS 为 int 类别时渲染 `(char)/(byte)/(short)` cast (即 store opcode
    本就执行的截断, 行为等价; 已是 char/byte/short 或带 i2c/i2b/i2s 的值类型非 int, 自动不动)。种子 battery
    `NarrowFieldReassign.java` (条件式写 char/byte/short 字段, 进 `TestCodecSemanticsRoundTrip` 差分门禁), 承重测试
    `TestNarrowFieldReassignCastIsLoadBearing` (kill-switch `JDEC_NO_NARROW_REASSIGN_CAST=1` 跑负向: 关闭即复现 lossy
    conversion)。实测 fastjson2 整 jar 重编译错误 -14 (699→685), codec/guava/spring 无回归。
  - **lazy-init 引用槽首个 store 被单次使用折叠丢弃 → slot 声明陷在 if 臂内、臂后读出作用域 (commons-codec DaitchMokotoffSoundex / 任意 `Map<K,List<V>>` 累加器形态)** (原 Bug AK, 可用性/重编译, 单类反编译即可见, 重编译报 `cannot find symbol: varN`):
    极常见 `List r = map.get(k); if (r == null) { r = new ArrayList(); map.put(k, r); } r.add(v);` 惯用法 ——
    首个 store 携**声明类型** (List, 来自 map.get), if 臂内第二个 store 携**子类型** (ArrayList)。`AssignVarGuarded`
    见类型不同且该槽非 null-init / 非形参 / 非 int 类别, 为臂内 store 铸新块作用域 id, 臂后 `r.add` 经单一全局
    slot 表 (DFS 序) 绑到臂内 id 渲染出作用域; 臂内 store 又因看似单次使用被折叠丢弃。治本: `code_analyser.go`
    新增 `reachingRefSlotPhiMerge` —— 经 phi (一个被「fall-through 定义」与「臂内 store」双向到达的下游 load) 证明
    两个 store 实为同一变量, 续用支配性的 `List var3` 定义, 使臂内变成普通 `var3 = new ArrayList()` 重赋值, 每处读
    都在作用域内。**结构门收窄, 只治 lazy-init 签名**: 支配定义须紧跟**对自身槽的 null 检查** (`defFollowedBySelfNullCheck`,
    排除 fastjson2 那种 `Object value` 多类型分派槽), 且臂内值须是**新建对象** (`new X()` 非数组, 保证 X 可赋给变量声明
    类型故续用支配定义类型恒类型安全)。种子 `lazyinit_slot_decl_dropped.class` (承重测试
    `TestDecompileSyntaxRegression/lazyinit_slot_decl_dropped.class` + `TestRefSlotPhiMergeIsLoadBearing`,
    kill-switch `JDEC_REF_SLOT_PHI_MERGE_OFF=1` 跑负向: 关闭即复现 `ArrayList var3` 拆分 + 臂后 `cannot find symbol`)。
    实测整 jar 重编译错误改善: commons-codec +2, fastjson2 +6, guava/spring 无回归。
  - **boolean 默认 init (`iconst_0; istore`) 与同槽 boolean 赋值未合并 → 拆成 `int varN=0` + 内嵌 `boolean varM`, 外部读出作用域 (commons-codec Metaphone.regionMatch / MatchRatingApproachEncoder 形态)**
    (可用性/重编译, 单类反编译即可见, 重编译报 `cannot find symbol: varM`): `boolean b=false; if(...){ b=expr; } return b;` ——
    javac 把默认 init 编成 `iconst_0; istore S`, 其压栈常量在 JVM 操作数栈上是 int (栈无 boolean 类别), 故默认 def 被定型
    int、if 臂内 boolean 赋值被定型 boolean。`AssignVarGuarded` 的整型类别合并**刻意排除 boolean** (Java 禁 int↔boolean
    转换), 两个同槽 def 不合并 → 拆成孤立 `int var4=0` + if 臂内 `boolean var5=...`, `return var5` 落在 if 外 →
    `cannot find symbol`。源码语法本身在不相交支上不矛盾, 故只有「反编译→重编译」暴露。治本: `code_analyser.go` 新增
    基于到达定义 + phi/diamond 的合并 `reachingBoolDefaultMerge` (配套 `reachingSlotStoreOps` 取到达定义的定义 opcode、
    `slotStoreValue` 记录每个 store 提交的值、`slotDefPhiReachesLoad` 做前向可达 + 后向到达定义双向校验) —— 仅当某 boolean
    store 的**唯一**到达定义是 int 0/1 字面量、且二者前向都汇入同一下游 load (phi/diamond, 证明是同一逻辑变量) 时, 才把
    默认 def 的 ref 与其 0/1 字面量一并改型为 boolean (字面量 0→false/1→true) 并续用, 使两臂共用单一 `boolean var4`。
    反例 `int a=0; use(a); boolean b=cond; return b;` 因 a/b 无公共 load (phi 测试不过) **绝不合并**, 故不会把
    `use(a)` 误编成 boolean。实测 commons-codec `Metaphone.regionMatch` 正确产出单一 `boolean var4=false`。种子
    `bool_default_init_merge.class` (承重测试 `TestDecompileSyntaxRegression/bool_default_init_merge.class`,
    kill-switch `JDEC_BOOL_DEFAULT_MERGE_OFF=1` 跑负向: 关闭即复现 `int var4=0` + `var5` split → 不可编译)。
  - **同槽多段不相交活跃区间的跨作用域声明被「存在即跳过」误判而不上提 → 兄弟臂用出作用域 (fastjson2 TypeUtils / JDK FloatingDecimal 形态)**
    (可用性/重编译, 单类反编译即可见, 重编译报 `cannot find symbol: varN`): 一个 JVM 槽被三个互不重叠活跃区间的源局部
    复用 (if 臂一个 int、else 臂另一个 int、if 之后第三个 int), minted-id 合并把三段并成同一 `VariableId`; 其声明落在
    if 臂, 而 if **之后**还有一处不相交的再声明在块顶层。`placeCrossScopeDeclarations` 旧逻辑用 `isDeclaredAtTopLevel`
    (只问「该 id 在本块顶层是否存在声明」) 判定已被覆盖而跳过上提, 但那处块顶层声明在词法上**位于** else 臂使用之**后**,
    并不支配它 → else 臂 `var4 = ...` 出作用域。现加 `topLevelDeclDominatesAllUses` + `blockHasUncoveredRef`:
    **作用域感知**地判定当前声明摆放是否让 id 的每处引用都在词法作用域内 —— 逐语句线程一个「至此已声明」标志, 简单声明
    `T id [= ...]` 在本块其后置位、每个嵌套子块按其出现点继承该标志; 故「兄弟臂用、声明只在另一臂」判为未覆盖 (上提),
    而「每个作用域各自声明同名」(VarFold `if(...){int v=1;...} int v=2;...` 双段合法) 判为已覆盖 (**不**上提)。引用按 id
    **身份**比对 (哨兵改名复用渲染路径, 同名异槽绝不误判)。仅当全部使用已被覆盖才跳过; 否则把一条裸 `T varN;` 上提到
    支配全部使用的块顶、把各处内联声明降为普通赋值。仅扩大作用域, 永远是合法 Java 且语义不变。种子 battery
    `SlotReuseDisjointRanges` (do-while 内 if/else 三段同槽复用), 承重测试 `TestCrossScopeDeclDominanceIsLoadBearing`,
    kill-switch `JDEC_NO_CROSS_SCOPE_DOMINATE` 跑负向 (关闭即 else 臂 `cannot find symbol` → round-trip 失败)。
    回归保护: `TestDecompiler/VarFold` 黄金用例锁住「双段各自声明不得被上提」这一边界。实测 fastjson2 整 jar 重编译
    错误 834→794、`varN_N` 类 cannot-find-symbol 72→30。
  - **单字符 `$` 标识符 (混淆字段/方法名) 被后置语法安全网误降级为 stub (asm-6.0_BETA `MethodWriter` 形态)**
    (可用性/round-trip, 单类反编译即可见, 重编译/外部调用才暴露「方法体丢失」): 混淆器会把字段/方法重命名成字面
    `$` (JVM 与 javac 均合法的标识符, 已用 `int $; this.$=...` 实测 javac 通过)。反编译忠实产出 `this.$`,
    但 yak 自带的 Java 文法词法器把**孤立** `$` (未粘连其它标识符字符) 词法化成专用 `Dollar` token (供 `${...}`
    插值, 见 `JavaParser.g4`) 而非 `IDENTIFIER`, 致 `this.$` 解析失败 → `degradeInvalidMethods` 把整段合法方法体
    降级成 `throw new RuntimeException` stub、并把 `$` 字段一并丢弃 → 重编译后外部调用直接抛异常 (语义全失)。
    旧 `isDollarIdentifierValidatorGap` 仅覆盖 `$` 作**方法调用** (`.$(`) 一种, 不覆盖字段读写/赋值。现于
    `validateJavaSyntaxWithBudget`: 首次校验失败且源码含孤立 `$` 时, 用 `neutralizeStandaloneDollarForValidation`
    把**孤立** `$` (前后均非标识符字符, 故 `Outer$Inner` / `val$x` / `$$` 这类已是合法 IDENTIFIER 的一律不动)
    在**字符串/字符字面量/注释之外**替换成普通标识符后复校; 复校通过才判定为合法 (仅校验用, 产出源码绝不改动)。
    因只在「主校验已失败 + 仅孤立 `$` 缺陷」时接受, 其它任何真实语法错误都会在中性化副本里幸存 → 仍降级, 故绝不
    放过真损坏代码。实测 asm `MethodWriter` 3 个被 stub 的方法 (`visitParameter`/`a()`/`a(ByteVector)`) 全部恢复
    真体、0 stub; asm jar 复扫该桶清零。种子 battery `DollarIdentField` (含 `$` 与 `$$` 双字段, 承重测试
    `TestDollarIdentifierValidationToleranceIsLoadBearing`, kill-switch `JDEC_NO_DOLLAR_IDENT_VALIDATE` 跑负向:
    关闭即 `$` 方法被 stub → round-trip 抛 `undecompilable method body`)。
  - **JDK8 try-with-resources 渲染成同 try 两个 `catch(Throwable)` (非法 Java, 重编译报「已捕获到异常错误 Throwable」)**
    (语义/编译, 单类反编译即可见但只有重编译暴露非法性): JDK8 内联 twr 脱糖里, `catch(Throwable t){primaryExc=t;throw t}`
    捕获器的保护区被外层 `Throwable` 清理 (`any`) 捕获器覆盖 (二者在字节码里是嵌套, 运行时先 inner 再 outer),
    旧 dumper 把两个处理器平铺成同一 try 的两个 `catch(Throwable)` —— Java 禁止一个 try 声明两个同类型处理器。现于
    dumper 新增 `mergeNestedSameTypeCatches`: 对**相邻同类型**且前者以 `throw <自身catch变量>` 无条件 rethrow 收尾的
    捕获器对, 按运行时顺序拼接 (前者体去掉尾部 rethrow ++ 后者体), 折成单一捕获器。因「同类型双 catch」只可能由
    反编译合成 (合法 Java 写不出), 故只会修复、绝不误并用户处理器; 三个以上 (嵌套 twr) 逐对左折。法线路径 close
    仍留在 try 体 (只跑一次), 异常路径 close 在合并后的 catch 里 —— 语义等价。实测 bm `Rule.parseRules` twr 该 try
    现可 round-trip。种子 `twr_jdk8_single_resource.class` (JDK8 编译, 承重测试 `TestTwrDuplicateCatchMergeIsLoadBearing`,
    kill-switch `JDEC_NO_CATCH_MERGE` 跑负向: 关闭即「已捕获到异常错误 Throwable」不可编译)。
  - **空-init 引用槽被兄弟分支异类局部按 DFS 顺序「收养」, 致 `Throwable` 槽被定型成 `String` (twr primaryExc / catch-holder 形态)**
    (语义/编译, twr 重编译会报 `Throwable cannot be converted to String` + `cannot find symbol getMessage/addSuppressed`):
    一支里 `Throwable holder=null` (twr 合成 primaryExc, 或 catch 里赋值的 holder, **空-init 存储支配后续真实赋值**),
    另一不相交支里异类局部 (如 `String acc`) 被 javac 打进**同一 JVM 槽**。前向模拟的「空-init 收养」(空-init 存储后
    遇到具体引用赋值就复用同 id, 保「单变量」惯用法) 是**无条件**的, 故 DFS 先走到 `String` 支时, 它收养了 primaryExc
    的 null 初值 → 两者并成一个 `String` 变量, 之后 primaryExc 的 `addSuppressed`/`= caught Throwable` 在该 String 上
    不可编译。源码语法在不相交支上不矛盾, 故只有「反编译→重编译」能暴露。现新增 `nullInitDefDominates` (code_analyser.go):
    对每个 `*STORE`, 仅当其要收养的空-init 定义 D **支配** (反向 CFG BFS, 把 D 当墙, 不穿过 D 能否抵达方法真入口) 当前
    存储 N 时才允许收养; 不支配 (即在不相交支) 则置 `blockNullAdopt`, 经 `AssignVarGuarded` 改铸新 id、令两变量按各自
    类型分裂。回环 back-edge / 多入口经「D 为墙」语义正确处理。实测修复 bm `Rule.parseRules` 槽定型 (其 twr 双 catch
    仍待回糖, 见 Bug AE)。副作用是改善了 `xmlbeans_qnamehelper` (推出更准的 `catch(UnsupportedEncodingException)`)。
    种子 `NullInitSlotReuse` (承重测试 `TestNullInitSlotReuseIsLoadBearing`, kill-switch `JDEC_NULLADOPT_REACH_OFF`
    跑负向测试: 关闭即 `Throwable→String` 不可编译)。
  - **if/else 双臂赋值且分支后被「循环+返回」读的逃逸槽, 被并入紧随其后的循环计数器, 致读错变量 (Nysiis/Metaphone transcode 形态)**
    (语义正确性, 编译期测不出): 某基本类型局部 (如 `int kind`) 在 if/else **两臂各赋值一次** (占一个 JVM 槽 S、两段活跃区),
    随后的循环 READ 它并自增**自己**的计数器 (槽 S+1, 活跃区与 S 第二段重叠 → 必为不同槽), return 再读它一次。
    旧实现里每臂各 mint 一个 id、分支后的读保留槽原始 (未 mint) id, 且因臂内 mint 在子作用域、**从不推进父作用域命名计数器**,
    导致紧随的循环计数器拿到与该局部**同一个 varN** → 重编译源码把计数器读成了那个局部, 静默算错。源码仍可编译 (同为基本类型),
    故只有「反编译→重编译→运行对比」能暴露 (commons-codec `Nysiis`/`Metaphone` transcode 每次编码被截断)。现于 rewriteVar
    处理 IfStatement 前新增 `prebindEscapingIfElseSlots`: 对**两臂都赋值且 if 后被读** (按 VariableId 身份探测, 非渲染名)
    且两臂类型一致的槽, 提前在父作用域 mint 单一 id (占用一个命名槽 → 后续兄弟局部不再撞名)、令两臂走 hasNamed 复用它、
    记 origId→newId 让本作用域 deferred ReplaceVar 重定向所有 if 后读, 并标 `reused` 让声明被提升到 if 之前。实测对真实
    `ColognePhonetic.colognePhonetic` 生效。种子 `IfElseEscapingSlot` (承重测试 `TestIfElseEscapingSlotIsLoadBearing`,
    kill-switch `JDEC_IFELSE_PREBIND_OFF` 跑负向测试: 关闭即复现指纹分叉)。
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
  - **`ACC_STRICT` 方法被渲染成非法关键字 `strict` 致整方法 (含 stub) 被丢弃 (JDK FloatingDecimal 拷贝形态)**
    (可用性, 单类反编译即可见, 方法在输出里直接消失): `getMethodAccessFlagsVerbose` 把 `0x0800` 映射成字面
    `strict`, 产出 `public strict double doubleValue()` —— `strict` 不是合法修饰符 token, 文法报
    `no viable alternative at input 'strict double'`, `degradeInvalidMethods` 主校验失败后连降级 stub 也继承
    了这个坏修饰符 (签名本身就不可表示) → 走到「drop」分支整方法被丢 (实测 beetl `FloatingIOWriter.doubleValue()/
    floatValue()` 的 `[WARN] un-representable as valid Java, dropping it`)。正字是 `strictfp` (文法
    `classOrInterfaceModifier` 已含, 方法/类通用), 忠实回写 `ACC_STRICT` 位且重编译 round-trip 一致。现把
    maskMap 该项改成 `strictfp` 并删除遗留的空 `else if verbose == "strict"` 死分支。种子
    `strictfp_method.class` (`--release 8` 编译以确保 ACC_STRICT 真被置位; Java17+ 默认严格已不再产生该位),
    承重测试 `TestDecompileSyntaxRegression/strictfp_method.class` + 根因单测 `TestMethodAccessFlags`。
  - **嵌入赋值 (DupStore) 孤儿局部「无声明 + 跨兄弟分支异类同名碰撞」→ `cannot find symbol` (commons-codec Metaphone.metaphone 形态)**
    (可用性/重编译, 单类反编译即可见, 重编译报 `cannot find symbol: varN`): 源码 `if (s==null || (n=s.length())==0){...}`
    的嵌入赋值 `(n=s.length())` (字节码 `... length; dup; istore_N; ifne`) 被 `code_analyser.go` dup-collapse 折成不透明
    `CustomValue` (其 ReplaceFunc 为 nil), 故 `n` **没有 DeclareStatement**: 既被声明上提器忽略, 又对 dumper 身份级碰撞
    重命名器 (只认声明) 不可见; 当其 slot 派生名 `varN` 与后续不同槽异类局部 (如 `StringBuilder`) 同名时不触发重命名 →
    `(varN=...)` 无声明且与 `StringBuilder varN` 撞名。治本: dup-collapse 时把嵌入赋值 LHS 记入
    `Decompiler.EmbeddedAssignDeclRefs`; `parser.go` 在 `RewriteVar` 后调 `rewriter.SynthesizeUndeclaredEmbeddedAssignDecls`,
    为「无声明孤儿」在方法首部合成裸 `T varN;`, 使其对碰撞重命名器可见 (后续同名兄弟被改名 `varN_1` 并同步改引用)。
    判据**刻意收窄, 只治本签名**: 仅当孤儿名与一个「不同 VariableId 且渲染类型不兼容」的已有声明撞名时才合成; 同名同类型
    (合法链式赋值 `int b=a=1`, 否则会把同一变量拆成两个, 见 `TestDecompiler/ContinuousAssign`) 与无任何同名声明的普通孤儿
    (其类型由字符串安全网 `JDEC_NO_EMBED_ASSIGN_*` 恢复) 一律不动。种子 `embedded_assign_cond_naming.class`
    (承重测试 `TestDecompileSyntaxRegression/embedded_assign_cond_naming.class`, kill-switch `JDEC_EMBED_ASSIGN_DECL_OFF=1`
    跑负向: 关闭即复现无声明+撞名)。实测 commons-codec 整 jar 重编译错误 10→8 (Metaphone 2 个错误清零)。
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
  - `return outside method` 7: **已治本** (本轮) —— `<clinit>` 静态初始化块里 JDK8 twr 脱糖残留的尾部
    `return;` (静态/实例初始化块禁止 return), 现于 dumper 渲染时丢弃块尾 void return。种子
    `clinit_twr_return.class` + `TestDecompileSyntaxRegression`, kill-switch `JDEC_NO_CLINIT_RETURN_DROP`。
  - boolean↔int 局部误定型 6: **已全部治本** —— String-switch 幻影 var (load+store 两侧) 见 Bug AI;
    boolean 默认 init (iconst_0) 与同槽 boolean 赋值未合并 (Metaphone.regionMatch) 本轮经到达定义 phi 合并治本
    (见「本轮新增治本」首项), 种子 `bool_default_init_merge.class` 即档案。

- **本轮权威重编译度量 (生产 JarFS 路径 + 包目录 + 传递依赖上 classpath, 整目录一次性 `javac --release 8`)** ——
  纠正了此前 report-v2 的两大度量噪声 (① 扁平文件名导致 `class X should be in X.java` 误报; ② 缺依赖导致
  `@Beta`/`@DoNotMock`/errorprone/checker 等注解类被记成 `cannot find symbol`/`package does not exist`)。修正后**真实
  decompiler 错误数** (单元数 / decErr=0 全部成功反编译):
  - **spring-core 5.3.27: 14 错 / 950 单元 (≈0.015/单元, 已达「非常好」)** —— GA 候选。
  - **commons-codec 1.15: 1 错 / 106 单元 (本轮 4→1: Bug AL null-adopt-once 治本 -3)** —— **已基本清零**, 余 1 为
    `DaitchMokotoffSoundex` 的三元 LUB (`ArrayList var = cond ? new ArrayList() : Collections.emptyList()` 应为 `List`, 归 Bug AH)。
  - **guava 28.2-android: 699 错 / 1877 单元 (empty-void-emit 治本 829→782 -47, typevar-field-store-cast 779→757 -22, typevar-array-cast 757→731 -26, typevar-arg-nocast 731→699 -32; 整树遮蔽下界)** —— **主导杠杆 = Bug AH 泛型类型变量沿数据流传播** (≈400 条:
    `Object→K/E/V/N/T/C/CAP` ≈260 + `type argument not within bounds` ≈84 + `inference variable has incompatible
    bounds` ≈75); 其余 `cannot find symbol` (含 sun.misc 等 JDK 内部 + 少量真缺陷)、`bad type in
    conditional expression` 12。即 guava 推 GA 必须做 Bug AH。**本轮已啃下 Bug AH 的三个「汇点-cast」入口 (return 侧 `typeVarReturnCast`、
    字段存储侧 `typeVarFieldStoreCast`、数组臂 `arrayTypeVar`/`isArrayOfTypeParam`) 与实参侧伪上行 cast 抑制 (`suppressTypeVarArgCast`, 实参为类型变量子族),
    本轮合计 guava -80; 余下 `Object→K` 多为「实参是具体类的伪上行」(enum `compareTo((Enum)...)`, 需被调方泛型签名解析) 与局部泛型恢复
    (`Iterator<? extends K> it = iterable.iterator()` 需把接收者类型实参代入方法返回签名做推断), 后者是真正的泛型推断引擎大特性。**
  - **fastjson2 2.0.43: 整树 javac 总错数 = 716 (ifelse-parallel-phi 治本 852→839, abstract-varargs 治本 839→831,
    final-field-renamed-local 治本 831→826, parallel-arm-phi-orphan 治本 827→819, empty-void-emit 治本 819→759 -60,
    try-slot-phi-merge 治本 759→751 -8, cast-escape-hoist 治本 751→746 -5 / 同治本 guava 782→779 -3,
    polysig-invokeExact-cast 治本 746→716 -30), 整树「完全干净文件」
    见 `TestScratchWholeTreePerFile`** ——
    **注: 整树「干净文件数」是比「总错数」更可靠的 GA 指标 —— 总错数受 javac 遮蔽, 但「0 错=干净」恒真。**
    **下一杠杆 (按 ROI):**
    **① `break outside switch or loop` 结构化 goto-到-尾合并 (Bug AM, FieldWriterDate/FieldReaderDateTimeCodec/ASMUtils/MethodWriter 等)** ——
    `else{ break; }` 实为跳到方法尾的非结构化 goto, 且公共尾块 (字段赋值/return) 被误划入 then 支; 属 CFG 结构化重建, 风险高。
    (注: 旧「① try-region 值槽子类型 widen 变体」之 BigIntegerCreator/JdbcSupport/DoubleToDecimal **编译错根因查实为签名多态 invokeExact 漏 cast, 已随本轮治本**,
    其槽分裂仅余语义保真长尾, 不再列为编译杠杆。)
    **② if/else 平行 phi「孤儿读」的 LUB 子族 (两臂渲染类型**不同**): **cast 守卫子族已治本** (见上「本轮新增治本」, `hoistCastGuardedEscapedLocals`,
    每处非声明用法皆 cast 时安全 widen `Object` —— ObjectWriters/JSONStreamReaderUTF{8,16} 等); **残留**仅「非 cast 读」子族 (join 后对 varN 直接
    成员访问/未 cast 实参, 真需公共超类型设施求 LUB, 如 EnumSchema var8 三臂 Integer/Long/BigInteger→Number)。+ `Object cannot be converted to X` 缺 cast 子族 (归 Bug AH)。**
    lambda-local rename 治本前整树显示 492 (其中 41 个 `already defined`), 治本后那 41 个 `already defined` 清零但整树升至 852 ——
    **这不是回归而是遮蔽解除**: 旧的 `already defined` 令 javac 放弃归属对应方法体, 把方法内既有的 Bug AL/AH 缺陷一并压住,
    治好遮蔽后约 +270 `cannot find symbol` 现形。**主导杠杆仍是局部变量声明/槽位数据流 (Bug AL) + 泛型 (Bug AH)**:
    cannot-find-symbol 450 条中, ObjectWriterCreatorASM 单文件 115 条 (命名局部 fieldClass/features/fieldName/format/
    fieldType 等沿 lambda body/活跃区间出作用域), 余分布 JSONReaderJSONB 46 / JSONReaderUTF8 28 / JSONReaderASCII 19 /
    CSVReaderUTF8 19 等密集分派多槽复用方法; 次为泛型 (`name clash ... same erasure` = `FieldWriter` 包歧义 internal.asm
    vs writer, 归 Bug AH)、控制流 `break outside switch or loop` (Bug AM)。即 fastjson2 推 GA 仍须夯实 Bug AL 命名局部
    出作用域 + Bug AH 泛型/包歧义 + Bug AM。
  - 度量复现: `SCRATCH=1 [PROFILE_JAR=guava] go test -run TestScratchProfile ./common/javaclassparser/tests/` (错误类目
    直方图, **整树, 受遮蔽**) / `TestScratchSymbolDrill` (cannot-find-symbol 细分) / `SCRATCH=1 KILL_SWITCH=<X>
    TestScratchJarErrDelta` (某 kill-switch 的整树错误前后差, **delta 受遮蔽**) / `SCRATCH=1 PROFILE_JAR=<X>
    [KILL_SWITCH=<Y>] TestScratchPerFileIso` (**逐文件隔离编译, 免遮蔽; 但扁平内部类 `Outer$Inner` 不能独立编译故绝对率
    偏悲观, delta 可信**)。详见 `tests/zz_jar_recompile_profile_test.go` (含 `jarDeps` 依赖 classpath 表)。
  - **⚠ 整树度量遮蔽 (本轮关键方法学发现)**: 「整目录一次性 javac」的总错数**严重受 javac 错误遮蔽影响, 不可靠**: 一个真实
    含 267 错的文件 (ObjectWriterCreatorASM, 逐文件隔离实测) 在整树编译里可能只报 1 错 —— javac 遇某些错即放弃归属该文件
    余下代码, 故总数取决于"哪些文件先失败、连带压住谁", 与反编译质量非单调。**推论: 此前所有整树 delta (-72/-39/-14 等)
    亦为遮蔽敏感的下界, 非精确值。** 逐文件隔离 (`TestScratchPerFileIso`) 免此遮蔽: 本轮实测**生产路径逐文件 recompile 率
    (绝对悲观, 扁平内部类计入失败) = fastjson2 50.5% (344/681) / guava 52.5% (986/1877) / spring 61.6% (585/950) /
    codec 68.9% (73/106)**; lambda-local rename 的逐文件 delta = 0 (该治本本身不翻转文件, 只降单文件错密度, 为前置条件)。
    今后治本以**逐文件隔离 delta** 为准绳, 整树直方图仅用于"找最大杠杆文件/类目"。
  - **方法学订正**: report-v2 的「重编译率%」用「Yak 每扁平内部类 1 单元」的高分母, 与 CFR/VF 不可直接比; 上面改用**绝对
    真实错误数** (deps 上 classpath、写回包目录), 更能驱动「哪个 bug 清掉收益最大」的 GA 决策。

- **本轮交叉对比 (Yak vs CFR 0.152 vs Vineflower 1.10.1, 同一 harness, `TestYakDecompilerCrossComparison`,
  CFR/VF jar 见 `/tmp/decompilers/`)** —— 复现: `CROSS_PK=1 CFR_JAR=.. VINEFLOWER_JAR=.. PK_JARS=.. PK_OUT=..
  go test -run TestYakDecompilerCrossComparison`。结果 (per-file javac + 仅 jar 上 classpath + flat `$` 单元):
  - **性能 (反编译墙钟, 越低越好)**: codec Yak并发 0.18s vs CFR 1.58s vs VF 1.60s; fastjson2 3.41s vs 8.19s vs 9.46s;
    spring 1.75s vs 4.26s vs 3.91s。**Yak 并发快 2.4-8.8x**, 完整性 100% (0 stub / 0 err), 三者中唯一全量产出。
  - **重编译率 (同 harness, 三者公平可比)**: codec **Yak 97% (103/106) > CFR 82% ≈ VF 97%**;
    fastjson2 **Yak 16% / CFR 12% / VF 79%**; spring **Yak 35% (342/978) > CFR 24% < VF 39%**。
  - **关键解读 (绝对%被 harness 严重压低, 仅相对排名可靠)**: 该 harness 用「逐文件 javac + -sourcepath 全树」, 故
    ① **级联**: 一个热点类 (JSONReader/JSONWriter/TypeUtils/Nullable) 反编译有缺陷, 经 sourcepath 拖垮所有引用它的单元;
    ② **缺可选依赖** (javax.annotation / kotlin / commons-logging / aspectj / sun.misc.Unsafe) 被误记成 decErr。二者对
    **三个工具一视同仁**, 故相对排名 (Yak>CFR, VF≥Yak) 可靠; 但绝对%远低于 Yak 自家「整目录 batch + deps 上 classpath」
    度量 (spring 14 错 / codec 1 错 / fastjson2 492 错), 后者才是真实单元缺陷率。**结论: Yak 全面快于 CFR/VF 且完整性满分;
    正确性 spring/codec 已与 VF 同档 (codec 余 1 = DaitchMokotoffSoundex 三元 LUB, 归 Bug AH), 唯 fastjson2 仍落后 VF —— 与绝对度量
    一致地指向局部变量数据流 (Bug AL) 为 fastjson2 推 GA 的主导杠杆 (本轮 Bug AN② 削 70 + Bug AL null-adopt-once 削 39 + Bug AL switch-prebind 削 72)**。
  - codec 当时距 100% 的 3 个单元 (`Rule.java:228` 匿名子类访父类私有字段、`DaitchMokotoffSoundex.java:85` 构造器
    实参误绑) **均已治本** (Bug AN ①②); DaitchMokotoffSoundex.<clinit> 的 twr 槽复用 (Throwable/Map.Entry 共槽) 本轮亦经
    Bug AL null-adopt-once 治本。dep-aware 度量 codec 现仅 1 错 (DaitchMokotoffSoundex 三元 LUB, 归 Bug AH)。

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
- **多段不相交活跃区间 (多个 store) 的跨作用域支配性已治本** (本轮, 见「本轮新增治本」第 1 项):
  同槽被 if 臂 / else 臂 / if 后三段各一个源局部复用、且 if 后还有一处块顶层再声明遮蔽时,
  `placeCrossScopeDeclarations` 旧「存在即跳过」误判。现 `topLevelDeclDominatesAllUses` 仅当顶层声明词法上先于
  全部使用才跳过, 否则上提裸声明。种子 `SlotReuseDisjointRanges` / `TestCrossScopeDeclDominanceIsLoadBearing` /
  kill-switch `JDEC_NO_CROSS_SCOPE_DOMINATE`。
- **残留 A (boolean `T v = false; if(...){ v = x; } return v;`) 已治本** (本轮): 其唯一已知复现
  commons-codec `Metaphone.regionMatch` 实为**槽误命名/分裂** (`iconst_0` 默认 init 被定型 int, 与同槽 boolean
  赋值分裂成 `int var4=0` + `boolean var5`), 现经 Bug AI 的 `reachingBoolDefaultMerge` 到达定义 phi 合并治本,
  默认 def 改型 boolean 后两臂共用单一 `boolean var4` —— 默认 init **不再被丢/误型**。种子
  `bool_default_init_merge.class` 即档案。若后续在**非 boolean** 基本类型上发现真正的「默认 init store 被丢」形态,
  再以「带 init 上提」治本 (须配 definite-assignment, 避免 `variable might not have been initialized`)。
- 残留 B (if/else 双臂均赋值且分支后被读): 与残留 A 同族但双臂均赋值。注: 其历史复现
  `Metaphone` / `ColognePhonetic` transcode 形态已由「本轮新增治本」的 `prebindEscapingIfElseSlots`
  (if/else 双臂逃逸槽预绑定) 覆盖。

- **残留 C 已治本 (本轮, 见「本轮新增治本」) — 嵌入赋值 (DupStore) 变量在短路条件内被铸造、跨兄弟分支读取时
  「无声明 + 命名碰撞」**: 源码用嵌入赋值缓存 `(n = s.length())` (字节码 `... length; dup; istore_N; ifne`),
  且 `n` 在 `s == null || (n = s.length()) == 0` 短路 `||` 里赋值、在后续 `if (n == 1)` 兄弟分支读取。结构化器把
  `||` 拆成嵌套 if 后, 该槽的嵌入赋值落在**内层 if 的条件**里 (渲染成 `(var4 = var1.length())`), 而非一条
  `AssignStatement`。根因: 嵌入赋值变量经 `code_analyser.go` dup-collapse 折成不透明 `CustomValue` (其 `ReplaceFunc`
  为 nil), 它**没有 `DeclareStatement`** —— 故 (1) 声明上提器找不到可上提的声明、不会为它补 `int varN;`; (2) dumper 的
  同名碰撞重命名器只认 `DeclareStatement`, 对这个仅以「条件内嵌入赋值」存在的变量不可见, 于是它与同 `Id()` 深度
  (分支 `Horizontal()` 同深) 的后续槽 (如 `StringBuilder`) 撞同名 `var4` 却不触发重命名 → `cannot find symbol: var4` /
  类型冲突。最小复现 (JDK8 `-g:none`):
  `String f(String s){ boolean hard=false; int n; if(s==null||(n=s.length())==0){return "";} if(n==1){return s.toUpperCase();} char[] c=s.toUpperCase().toCharArray(); StringBuilder a=new StringBuilder(40); StringBuilder b=new StringBuilder(10); a.append(c); return b.append(a).toString(); }`
  种子 `tests/testdata/regression/embedded_assign_cond_naming.class` (+ `.java.txt`)。真实复现: commons-codec
  `Metaphone.metaphone` (`txtLength` 嵌入赋值)。**治本**: `code_analyser.go` 在 dup-collapse 时把嵌入赋值 LHS 局部
  记入 `Decompiler.EmbeddedAssignDeclRefs`; `parser.go` 在 `RewriteVar` 之后调用 `rewriter.SynthesizeUndeclaredEmbeddedAssignDecls`,
  为这些「无声明孤儿」在方法首部合成裸 `T varN;` 声明, 使其对**身份级**碰撞重命名器可见 (后续同名兄弟被改名 `varN_1`)。
  判据**刻意收窄, 只治 residual-C 签名**: 仅当孤儿的 slot 派生名与一个「不同 VariableId、且渲染类型不兼容」的已有声明
  撞名时才合成 (例 `int` n vs `StringBuilder` var4)。两种情形故意不动: (a) 同名 + **同类型** = 合法链式赋值
  `int b = a = 1`, 合成第二个声明会被重命名器把同一变量拆成两个而破坏程序 (`TestDecompiler/ContinuousAssign` 回归);
  (b) 同名但**无任何声明**的普通孤儿, 其类型由字符串安全网 (`inferGeneratedLocalRefType` / `generatedLocalLooksInt`,
  kill-switch `JDEC_NO_EMBED_ASSIGN_*`) 已恢复, 不与之争抢以保持其承重。承重: 新种子 + kill-switch
  `JDEC_EMBED_ASSIGN_DECL_OFF=1`。commons-codec 整 jar 重编译错误 10 → 8 (Metaphone 2 个错误清零)。
- 其它待复核复现: commons-codec `MatchRatingApproachEncoder` (本轮复核**已干净**, 重编译通过) /
  `DaitchMokotoffSoundex` (lazy-init 槽声明被丢已治本, 见「本轮新增治本」首项; 残留属 Bug AH 泛型 + Bug AE twr 槽定型) /
  `bm.Rule` (残留属 Bug AD 嵌套 `$` 私有访问)。

### Bug AJ (新登记, 工业语料 STOP_ON_FIRST 命中) — 标号 `continue` 跳向 for 自增锁存被误渲成普通 `continue`

- 症状 (两形态, 同一根因):
  - **无限 for (`for(init; ; incr)`) + `continue 外层`**: 反编译报 `ParseBytesCode failed: has circle`, 整方法降级成
    stub。结构化把自增锁存 (incr, 例 `perm++`) 当成「被多前驱汇聚、不受任一体节点支配」的合并点, 进而把它误判为
    **循环出口** (endNode): 既丢掉 `continue` 分支 (渲染成空 `if(...){}`), 又把 do-while 的出口边接回锁存, 锁存自身
    再回边到 header → 2 节点环 (DoWhile -> incr -> DoWhile), `ToStatements` 判定 "has circle"。
  - **有限 for (`for(i=0;i<n;i++)`) + `continue 外层`**: **能编译但死循环** (编译期测不出、必须运行才暴露的语义 bug)。
    标号 `continue 外层` 被渲成**普通 `continue`**, 目标变成内层循环 → `i` 不自增、谓词不前进 → 无限循环。
- 根因: 反编译器循环模型只产 `do{...}while(true)` + break/continue, **从不重建 `for` 自增子句**, 也**不识别「跳向
  外层循环 latch/自增节点」即标号 continue** (LoopJmpRewriter 的标号-continue 只在 `next` 是外层循环 *header*
  (WhileNode) 时触发; 跳向 latch 不匹配)。javac 把 `continue` 编成跳到 incr (latch), 故二者皆走不到标号路径。
- 已验证的尝试 (本轮, 已回退, 不留半成品): 「自增 latch 尾复制」预处理 (按前驱拆分合并锁存, 每条 `continue` 路径
  各得一份 incr+回边) 能消除 has-circle 且让无限 for 编译通过, **但** dup 落在已被 IfRewriter 结构化的 if 体内,
  其回边在任何 LoopJmpRewriter 给它打标号前就被 if 结构化丢弃 → 仍渲成无标号 continue → stub 变 hang (更糟)。
  且该预处理对任意多前驱 latch 生效, 会把现有「有限 for+continue」从静默 hang 一并卷入, 回归面过大。结论: 需先做
  **标号 continue 正确渲染**(识别 continue→latch、按需给目标循环建 label) 或 **for 自增子句重建**, 再谈 latch 拆分;
  属较大特性, 须配 kill-switch + 全量(工业)回归(注意 hang 风险, 跑前先确认能编译再运行带 timeout)。
- 最小复现 (JDK8 编译):
  - 无限: `static String[] f(int n,int seed,int cur){ List l=new ArrayList(); outer: for(int perm=0;;perm++){ ...
    while(p>0&&p!=n){ ...; if(idx>=p){continue outer;} ... } ...; if(last)break; } return ...; }` (即 antlr-runtime
    `org.antlr.v4.runtime.atn.PredictionContext.toStrings(Recognizer,PredictionContext,int)` 的形态)。
  - 有限: `outer: for(int i=0;i<n;i++){ while(p>0){ ...; if(p>100){continue outer;} ...; if(p==7)break; p--; } sum+=i; }`。
- 复现命令: `DIAG_FILE=<antlr4-runtime-4.2.jar 解出的 PredictionContext.class> go test -run TestDiagDecompileClass -v
  ./common/javaclassparser/tests/` (工业扫描 `M2_INDUSTRY=1 STOP_ON_FIRST=1` 在 25071 类中**唯一**命中此条)。
- 指标背景: a-c 前缀 120 jar 全绿 (24490/24490, 0 partial); 工业语料 25071 类仅此 1 条 partial (graceful stub),
  其余 25070 全 OK。即除本条外工业面通过率 ~99.996%。

### Bug AD (残留) — 非标准库外部依赖 / 同 jar 跨包嵌套类型按 `$` 引用

- JDK/标准库子集已治本 (`Map.Entry` 等); 同 jar 跨包扁平 `$` import 也已治本 (见承重测试)。以下残留仍未治:
  - **非标准库外部依赖 jar** 的嵌套类型 (如 `com.google.errorprone.*$*`, `org.checkerframework.*$*`):
    这些在 classpath 上是真正嵌套的, 应渲染 `.` + 外层 import; 但单类入口无法区分「同 jar 自有扁平单元」与
    「依赖 jar 外部类型」(两者都可能是 `com.google.*` 前缀), 故未扩到非标准库包。注: 该残留**不会**让原本可编译
    的单元退化 (扁平 `$` 引用在旧外层 import 下本就不可解析), 仅是这类单元仍编不过。
- 复现: guava 引用 `com.google.errorprone.annotations.*` 嵌套类型的单元。

- **已治本 (本轮, String-switch 幻影 var, load + store 两侧)**: fastjson2 15% 重编译率的最大单一根因 ——
  `switch(s){case "X":...}` 脱糖成 `String s; int idx=-1; switch(s.hashCode()){case H: if(s.equals("X")) idx=N;} switch(idx){...}`,
  当第二段 case 体复用 slot1/2/3 作引用槽时, 单一全局 slot 表被 DFS 顺序污染, 致首段 `s.equals("X")` 接收者读成
  幻影 `var4.equals(...)` (应 `var2.equals`), 写匹配号 `idx=N` 被铸成新 `int varK = N` (应 `var2 = N`, 否则
  `switch(idx)` 永命中 default、编译过但语义死)。治本: `code_analyser.go` 新增基于到达定义 (reaching definition)
  的读修复 `reachingSlotVersionGeneral` (load 侧, 当解析到的 ref 非任一到达定义且唯一到达定义存在时改用之) +
  写修复 `reachingStoreVersion` (store 侧, 当全局版本类型不匹配而唯一到达定义类型匹配时, 经 `StackSimulation.SetVar`
  先装回正确 ref 再走 `AssignVarGuarded`, 令同型续写复用而非铸幻影)。承重种子 `strswitch_slot_reuse.class`
  (`TestDecompileSyntaxRegression`, load 侧, kill-switch `JDEC_SLOT_READ_REACHING_OFF`); store 侧承重为真实
  fastjson2 `TypeUtils.loadClass` (`DIAG_JAR` + kill-switch `JDEC_SLOT_STORE_REACHING_OFF`, 该法难以最小化成合成种子)。
- **boolean 默认 init 与同槽 boolean 赋值未合并 (Metaphone.regionMatch) 已治本** (本轮): 基于到达定义 + phi/diamond 的
  `reachingBoolDefaultMerge` —— 仅当 boolean store 的唯一到达定义是 int 0/1 字面量、且二者前向汇入同一下游 load 时,
  才把默认 def 改型为 boolean。详见「本轮新增治本」首项, 种子 `bool_default_init_merge.class` 即档案
  (kill-switch `JDEC_BOOL_DEFAULT_MERGE_OFF`)。

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
- **本轮已啃下的子族 (均带承重 + kill-switch, 见上「本轮新增治本」)**: ① return 侧汇点-cast (`typeVarReturnCast`),
  ② 同类字段存储汇点-cast (`typeVarFieldStoreCast`), ③ 数组臂汇点-cast (`isArrayOfTypeParam`, return+字段两入口),
  ④ **实参侧伪上行 cast 抑制** (`suppressTypeVarArgCast`: 实参静态类型是类型变量时, 描述符擦除到上界产生的伪 cast 一律抑制 —— 这是「该删的 cast」方向,
  与汇点-cast「该补的 cast」互补)。四项合计 guava 整树 -80。
- 治本方向 (余量, 大特性, 须谨慎, 高回归风险): **真正的有限泛型实例化引擎** —— 当局部 = `recv.m(...)` 且 recv
  有已知泛型类型、m 的 Signature 返回引用了 recv 的类型参数时, 用实例化后的泛型类型给局部定型 (而非 raw); checkcast
  目标同理按目标上下文补实参。须先有承重用例与 kill-switch, 避免误扩到无法静态判定的场景 (那比编译报错更危险)。
  另一独立余量: 实参是**具体类**而非类型变量的伪上行 (enum `state().compareTo((Enum)(State.X))` 把实参上行到 `Enum`,
  需被调方泛型/桥接签名解析才能判定该删), `suppressTypeVarArgCast` 刻意不覆盖 (实参非类型变量, 无法本地证 sound)。
- 复现: guava `PairwiseEquivalence` / `Equivalence$Wrapper` (局部泛型恢复); `AbstractService$*Guard` (enum compareTo 桥接实参伪上行)。

### Bug AE — try-with-resources 嵌套双资源命名 / 同包 enum 引用 (本轮复核: 两残留均已不复现, 已加守卫)

- twr 双 `catch(Throwable)` 合并 (`mergeNestedSameTypeCatches`) + twr 槽定型 (`NullInitSlotReuse`/`nullInitDefDominates`)
  早前已治本, 种子 `twr_jdk8_single_resource.class` / `NullInitSlotReuse` 即档案。
- **本轮复核 (嵌套双资源命名)**: 重新构造 JDK8 两资源 `ByteArrayInputStream` twr (含高槽位变体, 多前导局部把资源
  推到 slot9/slot11) 反编译, **每个资源的法线路径内联 close 副本接收者均与其声明同名** (`var11.close()` / `var9.close()`,
  不再出现历史的 `var10.close()` 幻影撞名)。两个合成输出 (TwrTwoRes / TwrTwoRes2) 经 JDK8 `javac` **均重编译通过**,
  bm `Rule.parseRules` 整方法的所有 close 接收者亦正确。该残留**已被早前的命名冲突解析 + nullInit 修复连带消除**。
  已加守卫种子 `twr_two_resource_naming.class` (`TestDecompileSyntaxRegression`, 断言 `var11.close()`/`var9.close()`
  存在且无 `var10.close()` 幻影)。
- **本轮复核 (同包 enum 引用)**: bm `Rule` 反编译中 `NameType`/`RuleType` 等同包顶层 enum 均正确按**类型名**引用
  (`NameType.class` / `NameType.values()` / `RuleType.RULES` / `EnumMap((Class)(NameType.class))`), 无 `cannot find
  symbol`; 单类残留仅 `Languages$LanguageSet`/`Rule$Phoneme` 这类 `$` 嵌套引用, 属 Bug AD 而非本条。该残留不复现。
- 备注: 完整 twr **回糖** (折回 `try (R r=..){..}`) 仍是可选大特性; 当前产出合法且语义等价的「手写式 twr」, 满足 round-trip。

### Bug AL (新登记, 本轮 fastjson2 度量主导) — 局部变量声明/槽位数据流残留 (undeclared `variable X` + 槽误型)

- 本轮已治本子族 (从本族切出, 见上「本轮新增治本」):
  - **嵌入赋值孤儿局部 `(c = obj.getClass()) != type` 的合成声明类型** —— `generatedLocalLooksInt` 引用 `==`/`!=` 误判 int +
    `inferGeneratedLocalRefType` 未恢复 getClass→Class, 已治 (-8)。
  - **null-init 引用槽仅允许首次类型采纳 (verbose twr `Throwable primaryExc=null` 与后继 `Map.Entry e` 共槽合并)** ——
    `JavaRef.nullTypeAdopted` + `AssignVarGuarded` null-adopt-once, 已治 (**fastjson2 -39 / codec -3**)。承重
    `TestTwrSlotReuseNullAdoptOnceIsLoadBearing` + 重构后的 `TestNullInitSlotReuseIsLoadBearing` (四组合隔离支配门与本守卫)。
  - **switch case 内写、switch 后读的局部前绑分裂 (fastjson2 DateUtils 手展开日期解析器形态)** ——
    `prebindEscapingSwitchSlots` (rewriter, 仿 if/else / try-catch 前绑), 已治 (**fastjson2 -72, 本轮单点最大杠杆**)。承重
    `TestSwitchCaseLocalReadAfterIsLoadBearing` (kill-switch `JDEC_SWITCH_PREBIND_OFF`)。
  - **内联 lambda body 自有局部与外层方法 varN 命名冲突 → `variable varN is already defined in method`** ——
    `renameLambdaBodyLocals` (dumper, 把 lambda body 局部抬入私有 `lv<seq>_N` 命名空间, 捕获占位符在 rename 后由调用点替换故不受影响),
    已治 (**fastjson2 整库消除 41 个 `already defined`; ObjectWriterCreatorASM 单文件隔离 267→250**)。承重
    `TestLambdaLocalRenameIsLoadBearing` (kill-switch `JDEC_NO_LAMBDA_LOCAL_RENAME`)。**注: 此治本解除遮蔽后整树总数升至
    852 (见上 fastjson2 度量「⚠ 整树度量遮蔽」), 为诚实暴露, 非回归。**
  - **if/else 平行 phi「孤儿读」异渲染类型 LUB 子族 (cast 守卫): 每处非声明用法皆显式 cast 时安全 widen `Object`** ——
    `hoistCastGuardedEscapedLocals` (dumper 文本 pass, 在 `addMissingGeneratedLocalDecls` 之前; 按缩进探测「声明全在更深内层、却有更浅层 cast 读」
    的逃逸槽, 把内层 `T varN = rhs` 降级为 `varN = rhs`、注入 `Object varN = null;`; 排除标量基本类型声明与任何成员访问/下标/未 cast/算术用法),
    已治 (**fastjson2 751→746 -5 / guava 782→779 -3**)。承重 `TestCastEscapeHoistIsLoadBearing` (kill-switch `JDEC_CAST_ESCAPE_HOIST_OFF`,
    pin fastjson2 ObjectWriters.class, 信号=孤儿读 `variable var3` 的 cannot-find-symbol)。
  - **if/else 双臂同类型同槽但跨 VarUid 的 phi (DFS 钳后槽复用致两臂铸不同 VarUid) 未合并 → 各臂各留声明、分支后读出作用域
    (fastjson2 ObjectReaderProvider.<init> 的 `long[] acceptHashCodes` 形态)** —— `prebindEscapingIfElseSlots` 只合并
    **共享 VarUid** 的两臂; 但当某槽 (典型: 引用/数组) 在 if/else 后被复用给**异类标量** (如 `long extra` 复用数组 astore 槽,
    bytecode 实测 `astore_3` 后 `lstore_3`) 时, javac 的 DFS 降级先探一臂的 fall-through 与那处后继标量复用、再回溯另一臂,
    栈模拟遂把槽表钳成 `long` 并为第二臂的 `long[]` store **铸全新 ref/VarUid** —— 同类型同槽但 VarUid 不同, 滑过既有前绑。
    治本: `prebindParallelTypedIfElseDefs` (rewriter, 在 `prebindEscapingIfElseSlots` 之后跑) —— 探测「两臂同 slot 同渲染类型
    但 VarUid 不同、且恰有一处引用逃逸 if/else」的形态, 铸单一父作用域新 id、把两臂引用与逃逸读全部 remap 到它、记 `reused`
    令 `placeCrossScopeDeclarations` 提一条裸 `T varN;` 到 if 前。种子 battery `IfElseParallelArrayPhi.java` (块作用域
    `data`/`total` 释放槽迫 `extra` 复用数组槽以触发 DFS 钳, 进 `TestCodecSemanticsRoundTrip`), 承重
    `TestIfElseParallelArrayPhiIsLoadBearing` (kill-switch `JDEC_IFELSE_PARALLEL_PREBIND_OFF` 跑负向: 关闭即复现
    `cannot find symbol` + `long cannot be dereferenced` + `array required but long found`)。度量: **整树 fastjson2 -13
    (852→839, cannot-find-symbol 441→430)**; **逐文件隔离 delta = 0** (受影响文件如 ObjectReaderProvider 仍有其它残留错故未翻转,
    本治本只降单文件错密度, 为该文件达 GA 的前置条件)。
  - **同槽不相交活跃区间被 DFS 序污染: int 计数器槽被后继 long/float/double 复用, iinc 误绑到出作用域的后继变量
    (fastjson2 `Fnv.hashCode64LCase`: slot5 先后承载 int 计数器 / int 字符 / long 哈希累加器; 首循环计数器 `i++` 渲染成
    `var5_1++` 绑到方法下方声明的 long 累加器, 在计数器处出作用域报 `cannot find symbol: var5_1`)** —— iinc 到达定义修复
    (verifier 保证 iinc 槽在该点必为 int 类别) 从「仅引用类型泄漏」扩到「long/float/double 类别泄漏」: 守卫 `!refIsPrimitive(ref)`
    改为 `!isIntCategoryNumeric(ref.Type())`, 沿 Source 回walk 取最近到达定义、且**仅当其为 int 类别**才采纳 (引用情形行为不变,
    因其回walk 命中的恒是 int 类别)。已治 (**fastjson2 -1, Fnv 整方法翻转干净; 4-jar A/B 对比旧引用-only 基线 codec/guava/spring
    零回归**)。承重 `TestIincIntCategorySlotRepairIsLoadBearing` (javac 编译真实 Fnv.class, kill-switch `JDEC_IINC_REACHING_OFF`
    双向: 关闭即复现 `var5_1++` 的 `cannot find symbol`, 开启回绑到 `var4++`)。**注: 这是「同槽不相交活跃区间分割」大特性的一个收窄
    可证 sound 子族; 天然姊妹杠杆是 LOAD 侧 (ILOAD 系列读到 long/float/double 复生槽), 现走热路径风险更高, 列为下一杠杆。**
- **if/else 平行 phi「孤儿读」子族 —— 同渲染类型部分已治本 (见上「本轮新增治本」parallel-arm-phi-orphan, 整树 827→819, 翻转 2 文件);
  残留仅「异渲染类型」LUB 子族 (≈4 文件)**。形态: if/else 两臂各在自己臂内声明同一 JVM 槽的局部 (`var<slot>`), 该槽在 if/else **之后**被读
  (phi 合并); 合并后的「读」绑定到**第三个 id** (既非 if 臂 def 的 id 也非 else 臂 def, 而是未构造的 phi 结果 id), 但**渲染成同槽名** `var<slot>`。
  治本走 dumper 端到端**按名绑定**这一事实: `parallelArmDeclHoist` (rewrite_var.go) 在 `hoistSwitchDeclarations` 晚期 pass 里, 当两臂顶层各首声明同名
  `var<slot>` (跨 VarUid)、且 `statementsReadName(afterSts, "var<slot>")` 命中时, 在 if 前提一条裸 `T var<slot>;`、把两臂降级成 `var<slot> = ...` ——
  唯一存活声明令该名「已声明」, 两臂赋值与孤儿读全按名绑到它, 无需动任何 id (`addMissingGeneratedLocalDecls` 按名去重, 见名已声明不再注入)。
  **已治: 两臂渲染声明类型 token 一致的子集** (`renderedArmDeclType` 判定, 而非 `ref.Type()` —— 本 pass 早于 dumper 末段 RHS 定型, 某臂 stale
  `ref.Type()` 仍是 Object 却渲染 `ObjectWriter var10 = ...`; 一致即类型安全, join 类型即该 token, 裸声明取自天然带该类型的臂)。代表已翻转:
  `FieldWriterListFunc.writeValue` (两臂渲染均 `ObjectWriter var10`, 读 `var10.write(..)`)。
  **已治 (cast 守卫子族)**: `ObjectWriters.fieldWriterList` (`ParameterizedType` vs `ParameterizedTypeImpl`, 读 `(Type)(var3)`)、
  `JSONStreamReaderUTF{8,16}` (`Object`/`List`/`Object`, 读 `return (T)(var2)`) 等「每处非声明用法皆显式 cast」者, 由 `hoistCastGuardedEscapedLocals`
  安全 widen `Object` (见上「本轮已治本子族」)。早期试过「异类型一律 widen 到 `Object`」实测 fastjson2 **+10** (臂内类型相关用法 `varN.foo()` 被 Object
  打断) 已撤; 新 pass 用「**全用法皆 cast** + 缩进逃逸 + 排除标量基本类型/成员访问/下标/算术」紧门, 故只在 widen 必然 sound 时触发, A/B 零回归。
  **未治 (非 cast 读子集)**: join 后对 varN 直接成员访问或作未 cast 实参者 —— `EnumSchema` (`Integer`/`Long`/`BigInteger`, 读 `this.items.add(var8)` 未 cast 实参,
  另伴随读被误提到比 var8 活跃区更浅的作用域, 属更深的作用域重建)、`ObjectReaderImplGenericArray` (读 `var6_1.add(var7)` 成员访问)。这些真需
  **type-LUB / 公共超类型设施** (反编译器当前无类层级/可赋值性查询, `rg 'Assignable|Superclass|commonSuper'` 命中 0) 才能给出比 `Object` 更具体、
  支撑成员访问的父类型, 属大特性, 暂留。
  实现须配 kill-switch + 全量 (codec 差分 + fastjson2/guava/spring 整树基线) 回归, 治本前必须确认整树错数**下降**而非因孤儿读上升。
  复现: `SCRATCH=1 PROFILE_JAR=fastjson2 MAXERR=1 TOPN=40 go test -run TestScratchWholeTreePerFile ./common/javaclassparser/tests/`
  (看 1-err 直方图与 closest-to-flipping 列表)。
  以下为**剩余**残留 (整树计数受遮蔽, 以下取逐文件隔离视角):
- 症状 (lambda-rename 解除遮蔽后, fastjson2 整树 `cannot find symbol` 升至 450, 但整树计数受遮蔽不可靠; 按出错文件
  集中度: ObjectWriterCreatorASM 115 / JSONReaderJSONB 46 / JSONReaderUTF8 28 / JSONReaderASCII 19 / CSVReaderUTF8 19
  等密集分派多槽复用 + lambda 重方法为主):
  - `cannot find symbol: variable X` (X 既含 `varN` 也含**带调试名的命名局部** fieldClass/features/fieldName/format/
    fieldType/ordinal 等, 在某分支/lambda body 声明、被兄弟分支或块外读 → 出作用域)。ObjectWriterCreatorASM 单类即 115 条,
    现为本族最大杠杆文件, 属 lambda body + 活跃区间复合形态, 需完整活跃区间分割。
  - `int cannot be dereferenced` 15 + `incompatible types: int cannot be converted to T` 44: 引用槽被同槽 int 局部
    污染后, 读成 int 再 `.method()`/赋给引用 → 误型 (与 Bug AI String-switch 幻影 var 同族, 但形态更广;
    例 `ObjectReaderImplMapTyped.readObject` 的 `var9` 既作 `Map` 又 `var9++` 当计数器, map 与 i 折叠成同名)。
  - `variable varN is already defined in method` 14: 同名声明在同作用域出现两次 (声明上提/槽合并把两段并成一个 id
    却各留一处声明)。
  - `incompatible types: T cannot be converted to T` 94 (例 `JSONPathSegment→JSONPath`, `Class→Field`): 读到错误的
    同槽变量。
- 根因方向: 现有「到达定义 + phi 合并」基础设施 (`reachingSlotVersionGeneral` / `reachingStoreVersion` /
  `reachingBoolDefaultMerge` / `reachingRefSlotPhiMerge` / `reachingSlotVersionByCategory` 的 iinc 修复) 覆盖了若干
  特定形态, 但 fastjson2 这类**密集分派 + 多槽复用**的方法仍有残留: 需把局部变量识别从「单一全局 slot 表 + DFS 序」升级
  为**真正的到达定义/活跃区间分割** (同槽不相交活跃区间 → 各自独立变量、声明摆到支配全部读的位置)。属用户要求的
  「活跃性 + 到达定义 + phi 数据流 pass」核心夯实目标, 高回归风险, 须配 kill-switch + 全量 (codec 差分 + 三 jar 基线) 回归逐步推进。
  本轮已切出并治本 iinc 侧的「int 类别槽被 long/float/double 复生槽污染」子族 (见上「本轮已治本子族」iinc-intcat 项)。
  **下一收窄子族 (姊妹): LOAD 侧 int 类别修复** —— `reachingSlotVersionOnMismatch` 当前只判「引用 vs 基本型」类别,
  ILOAD 系列读到 long/float/double 复生槽 (两者皆基本型) 漏判; 治法对称 (新增 `isIntCategoryLoadOpcode` + `isIntCategoryNumeric(better)`
  守卫), 但 LOAD 是热路径、整树度量受遮蔽噪声 ±3, 须独立 4-jar A/B 严格证零回归后再并入。
- 复现: `SCRATCH=1 PROFILE_JAR=fastjson2 go test -run TestScratchSymbolDrill ./common/javaclassparser/tests/`;
  单类例 `JSONPath` / `ObjectWriterCreator` / `FieldWriter`。
- 注: DaitchMokotoffSoundex.<clinit> 的 twr `Throwable`/`Map.Entry` 共槽形态 (此前本族最小种子) **本轮已治本**
  (见上「本轮新增治本」null-adopt-once 项), codec 因此 4→1; 余 1 是三元 LUB (归 Bug AH)。本族剩余主要是 fastjson2 那类
  密集分派 + 多槽复用方法 (DateUtils / ObjectReaderImplMapTyped / JSONPath), 仍需把单一全局 slot 表升级为真正的活跃区间分割。

### Bug AM (新登记, 本轮 fastjson2 度量) — `break` / `continue` 结构化越界 (无外层循环/switch)

- 症状: `break outside switch or loop` 22 (例 `ObjectWriterProvider.getObjectWriterInternal` 末尾 `}else{ break; }`
  落在没有任何外层循环的 if/else 里)。结构化器在重建大方法 (含内层 switch/do-while 混合) 时, 把某条本应是
  `return`/落空的边渲染成 `break`, 而其外层循环已被结构化消解 → `break` 无目标。
- 根因方向: 控制流结构化 (循环/switch 边界识别 + break/continue 目标绑定) 在复杂嵌套下边界判定不足; 与 Bug AJ (标号
  continue/for 自增重建) 同属循环结构化族, 但本条是 break 落到无循环上下文。须配 kill-switch + 工业回归 (注意 hang 风险)。
- 复现: `SCRATCH=1 PROFILE_JAR=fastjson2 DUMP_CLASS=ObjectWriterProvider.class go test -run TestScratchDumpClass
  ./common/javaclassparser/tests/` 看 `getObjectWriterInternal`。

### Bug AN — 匿名子类局部按合成名定型 (① 已治本) + 构造器实参「新绑定局部」改名漏传 (② 已治本)

- **① 匿名子类局部访问父类私有字段 — 已治本 (本轮)**:
  - 症状 (`Rule.java:228`, 仅整库 sourcepath round-trip 暴露): `Rule$2 var13 = new Rule$2(..); String x = var13.pattern;`。
    `pattern` 在父类 `Rule` 是 `private`, 私有成员**不被子类继承**, javac 报 `pattern has private access in Rule`。
    源码本是 `Rule r = new Rule(..){..}` (匿名子类), 局部声明类型应是父类 `Rule`。
  - 治本方案 (单类路径即可, 无需跨类 super 信息): `code_analyser.go` 新增 `castAnonSubclassReceiverForOwnField`,
    在 `OP_GETFIELD` 渲染时, 若 getfield 的 owner == 当前类名, 且 receiver 静态类型是合成匿名子类 `Owner$N`
    (`strings.HasPrefix(jc.Name, ClassName+"$")`), 则插入 `((Owner)recv).field` 显式上行 cast。仅用 getfield CP 的
    owner + receiver 静态类型判定, 严格限定到「自身私有字段经合成子类访问」一种形态, 不会给继承公有字段过度加 cast。
  - kill-switch: `JDEC_NO_PRIV_FIELD_CAST`。承重测试: `TestAnonSubclassOwnPrivateFieldCastIsLoadBearing`
    (battery `testdata/codec/AnonSubclassOwnPrivateField.java`, 多类匿名子类, ON/OFF 双跑, OFF 必须 javac 失败)。
- **② 构造器实参「新绑定局部」改名未传播 — 已治本 (本轮, 原误判为内部类合成形参, 实为通用改名漏传)**:
  - 症状 (`DaitchMokotoffSoundex.java:85`): `new DaitchMokotoffSoundex$Rule(var10,var11,var9,..)` 第 3 实参渲染成
    数组 `var9` 而非应有的局部 `var12`, javac 报 `incompatible types: String[] cannot be converted to String`。
  - **真实根因 (经 DMS_TRACE_LOAD / rewrite-var / var-fold 三路 trace 定位)**: 数据流**正确** (第 3 实参绑定到
    slot12 的 ref-16), 纯属**改名漏传**。`RewriteVar` 对一个新绑定局部 (`bind` 路径, rewrite_var.go:595) 用「拷贝
    JavaRef + 给副本 SetName + 记 `idReplaceMap[oldId]=newId` + 在 scope defer 里 `statement.ReplaceVar`」改名;
    但 `new T(..)` 的**构造器实参只活在 `NewExpression.ArgumentsGetter` 这个仅供渲染的闭包里**,
    `NewExpression.ReplaceVar` 只遍历 `Length/Initializer`, **够不到闭包内的实参**。于是声明被改成 `var12`、调用点
    仍用旧的 slot 派生名 `var9` (因 `VariableId` 名是树深度派生, ref-16 与数组 ref-12 同处深度 9, 撞名), 编译器把
    `var9` 绑成数组。
  - 治本方案: `NewExpression` 增 `ConstructorCall *FunctionCallExpression` 反向引用 (invokespecial `<init>` 处
    `v.ConstructorCall = funcCallValue`), `NewExpression.ReplaceVar` 追加遍历 `ConstructorCall.Arguments` (**只遍历
    Arguments, 不碰 Object**——Object 即该 NewExpression 自身, 否则自循环)。这样改名能抵达构造器实参。
  - kill-switch: `JDEC_NO_CTOR_ARG_REPLACE`。承重测试: `TestCtorArgFreshLocalRenameIsLoadBearing`
    (battery `testdata/codec/CtorArgFreshLocalRename.java`, 顶层 sibling 类避开 `$` 引用噪声, ON/OFF 双跑,
    OFF 必报 `String[] cannot be converted to String`)。
  - **影响面 (dep-aware 整库度量, 通用改名修复)**: codec 6→4, **fastjson2 677→607 (−70)**, guava 835→829 (−6),
    spring 14→14。fastjson2 收益显著, 是本轮单点最大杠杆。
- ② 修后 `DaitchMokotoffSoundex` 仅余 4 个其它缺陷 (line 203 conditional 类型、line 296/319 cannot find symbol、
  line 313 Throwable→Entry), 均属 slot 复用/catch 形参其它子问题, 归并入 Bug AL 处理。

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
