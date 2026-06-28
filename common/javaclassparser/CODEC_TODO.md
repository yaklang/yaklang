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
- **差分门禁 `TestCodecSemanticsRoundTrip`: 61 个自托管 battery 全绿** (byte-for-byte round-trip), 覆盖
  控制流 / 排序 / 链式赋值 / SHA-256 / SHA-512 / TEA-XTEA-RC4 / String-switch / try-with-resources /
  try-finally-loop / 数值转换重命名 / iinc 宽化 / 循环计数器 slot 复用 / 嵌入式赋值 / Class 字面量 /
  int 类别收窄 / guava IntMath(gcd·log2·floor-sqrt·pow·isPrime·checkedAdd) / 严格 Hex 编解码(收窄+校验异常) /
  boolean-int 槽复用形态 / **空前导 case 的 tableswitch (Base32/Base64 EOF 形态)** /
  **对象-null 守卫后接底部测试循环 (Md5Crypt salt 形态)** /
  **短路 `(A&&B)||C` 中 C 为内联数组可变参调用 (DoubleMetaphone.conditionC0 形态)** /
  **if/else 双臂赋值且循环后读的逃逸槽 (Nysiis/Metaphone transcode 形态)** /
  **空-init Throwable holder 与异类局部跨支共槽 (twr primaryExc / catch-holder 形态)** /
  **单字符 `$` 标识符字段 (asm 混淆 MethodWriter 形态, javac 合法但 yak 文法词法器误当 Dollar token)** /
  **同槽三段不相交活跃区间 (if 臂 / else 臂 / if 后) 的跨作用域声明支配性 (FloatingDecimal / fastjson2 TypeUtils 形态)** 等。已治本 bug 的
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
- **本轮新增治本 (codec 真实差分 + 工业语料新发现, 10 项, 均已锁回归种子, 种子即档案)**:
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
  - boolean↔int 局部误定型 6: String-switch 幻影 var (load+store 两侧) 已治本 (见 Bug AI); 残留仅
    boolean 默认 init (iconst_0) 与同槽 boolean 赋值未合并的形态 (Metaphone.regionMatch), 见 Bug AI 残留。

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
- 残留 A (单 store 分支声明 + 默认 init 被丢): `T v = dflt; if(...){ v = x; } return v;` —— javac 在 if 前有
  `store v=默认值` (如 `boolean var4=false`), 反编译**丢掉**该默认 init store, 只剩 if 臂内的赋值被当成声明,
  if 后 `return v` 读不到。此形态 v 只有一处 store (不在 `reused` 集) 且 `ifHoistDeclarations` 要求 ≥2 store, 两套
  上提器都不接。治本需「带 init 上提」(恢复默认值 store 并提到支配块) 或槽默认 init 不丢, 涉及 definite-assignment,
  须谨慎 (盲上提无 init 会触发 `variable might not have been initialized`)。复现: commons-codec `Metaphone.regionMatch`
  (注: 该处实际是**槽误命名**, 见 Bug AI —— boolean 结果 `istore_4` 被渲成 `var5`、`return var4` 渲成 `return var5`)。
- 残留 B (if/else 分支声明被读出): 与残留 A 同族但双臂均赋值。
- 复现: commons-codec `Metaphone` / `MatchRatingApproachEncoder` / `DaitchMokotoffSoundex` / `bm.Rule`。

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
- **残留 (boolean 默认 init 与同槽 boolean 赋值未合并, commons-codec Metaphone.regionMatch / MatchRatingApproachEncoder)**:
  `boolean r=false; if(...){ r=sub.equals(t); } return r;` —— slot4=`r`(boolean), 其默认 init `iconst_0; istore_4`
  在字节码层是 int 0, 被定型成 `int`; if 臂内 `equals` 结果 (boolean) 再 `istore_4`。`AssignVarGuarded` 的整型类别
  合并**刻意排除 boolean** (Java 禁止 int↔boolean 转换), 故 int-默认 与 boolean-赋值两个同槽 def **不合并** →
  拆成两变量: 孤立 `int var4 = 0` + if 臂内 `boolean var5 = ...`, 而 `return var5` 落在 if 外 → `cannot find symbol: var5`。
  最小复现 (JDK8, 已取): `boolean rm(StringBuilder sb,int i,String t){ boolean m=false; if(i>=0&&i+t.length()-1<sb.length()){ String s=sb.substring(i,i+t.length()); m=s.equals(t);} return m; }`
  反编译即得上述形态 (无需 LVT)。
- **根因 + 为何不强改**: 根因是 `iconst_0` 默认 init 被定型 int 而非 boolean, 致与同槽 boolean def 分裂。安全治本需
  **基于 phi/活跃性的合并** —— 仅当 int-0/1 默认 def 与 boolean def 同时到达某下游公共 load (即同一逻辑变量) 时才合并并
  改型为 boolean (并把字面量 0/1 渲成 false/true, 参 `arrayStoreRHS` 的布尔强制)。**不可用 store 期启发式**: 反例
  `int a=0; use(a); boolean b=cond; return b;` 里 a/b 是不相交两变量 (无公共 load), store 期无前向信息区分,
  盲合并会把 `use(a)` 编成 `use(boolean)` → 误编译。槽合并层另有「禁止过度拆分」回归 (`var3_1 leaked` 等) 把握平衡,
  贸然改触发之, 而 dumper 线性拆分跨分支可**静默错拆 (比编译错更危险)**。故需先建到达定义/活跃性 pass 再做, 与 Bug Y/AH 同族核心特性。
  影响面小 (整目录 ≈6-10 条 `int↔boolean`), 优先级低于 enum 簇, 已具备最小复现可作首个承重用例。

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
