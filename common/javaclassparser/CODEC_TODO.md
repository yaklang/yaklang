# Codec 算法交叉验证 — 交接 TODO (CODEC_TODO.md)

> 分支: `codex/yak-java-decompiler-cross-comparison`
> 核心目标: 用「反编译 → 重编译回 class → 直接运行算法对比」验证反编译器的**语义正确性**, 达到 GA 水准, 而非仅"能反编译"。
> 配套文档: [YAK_JAVA_DECOMPILER_CROSS_COMPARISON.md](./YAK_JAVA_DECOMPILER_CROSS_COMPARISON.md), [HARNESS_WORKFLOW.md](./HARNESS_WORKFLOW.md)
>
> 本文件只记录**尚未修复**的缺陷与待扩展项; 已治本的 bug 不再保留文字记录 —— 其回归测试 (testdata
> 种子 + `Test*` 守卫) 即永久档案。新发现的 bug 必须在此登记 (症状 / 根因 / 复现 / 规避现状)。

---

## 当前状态

- 真实库差分调用 (guava 28.2-android): 反编译 → 重编译成 overlay → classpath 覆盖原 jar →
  用 `GuavaProbe` 反射跑算法对比指纹。**18 个类 (IntMath / Ints / Longs / UnsignedInts /
  UnsignedLongs / Ascii / Strings + 各自内部类) overlay 重编译 exit 0, GuavaProbe 输出与原 jar
  byte-identical (同 MD5)**。本轮借此挖出并治本 5 个长尾 bug (注解 boolean/char 渲染成 int、
  varargs 形参被注入 `Object varN=null` 幽灵声明、引用型形参被同类型子类重赋值时被影子拆分成块作用域
  局部、泛型父类型/接口签名被擦除致抽象方法未覆盖、数值转换 (l2i/i2c...) CustomValue 未转发
  ReplaceVar 致 `(int)x` 强转保留过期变量名)。按约定均不留文字记录, 其回归测试即档案。
- 差分门禁 `TestCodecSemanticsRoundTrip`: **45 个自托管 battery 全绿** (byte-for-byte round-trip);
  新增 `IincWidenDecl` (承重负向锁定 Bug W「iinc 目标 slot 声明过窄」治本: baload 喂入的 int 局部被
  推断成 byte, 随后 `b += 256`/`b -= 200` 被 javac 编成 `iinc_w`(javac 只对真 int 局部发 iinc,
  byte/char/short 复合赋值走 iadd+i2b/i2c/i2s), 非 ±1 iinc 反糖成 `b = b + 256` 对 byte 声明是可能精度
  丢失; 治本: iinc 处把 narrow int 类别 slot 声明宽化为 int, 关 `JDEC_IINC_WIDEN_OFF` 即报
  `possible lossy conversion from int to byte`, 由 `TestIincWidenDeclIsLoadBearing` 守卫, 真实命中
  commons-codec `Base64.encode`);
  新增 `LoopCounterSlotReuse` (承重负向锁定 Bug X「循环计数器 slot 被循环后 byte[] 复用致 iinc 绑错
  化身」治本: javac 把已死的循环计数器槽复用给循环后的 byte[] (Base64.decode 形态), 单一全局
  slot->ref 表在 DFS 顺序里先访问循环后的 byte[] 存储, 致 `GetVar(slot)` 在循环内 iinc 处返回 byte[]
  化身, `i++` 渲染成 `someByteArray++`; 治本: iinc 处沿 Source 回溯到 int 类别的到达定义 (复用 load
  失配修复的同一回溯), 关 `JDEC_IINC_REACHING_OFF` 即报 `bad operand type byte[] for unary operator
  ++`, 由 `TestLoopCounterSlotReuseIsLoadBearing` 守卫);
  新增 `EmbeddedAssignDecl` (锁定「dup-collapse 嵌入式赋值致 int 局部无声明被安全网猜成 Object」治本:
  `(v = a[i]) == 0` 等值比较整型字面量 / `(v = a[i]) < limit` 关系比较, 关 `JDEC_NO_EMBED_ASSIGN_INT`
  即退化报 Object/int 不兼容, 由 `TestEmbeddedAssignDeclIntIsLoadBearing` 承重守卫);
  新增 `JumpEnteredTryCatch` (承重负向锁定「跳转进入的 try-catch 被丢弃」治本: guard-return 后接 try /
  try 作循环体 / try 作 else 分支体 / guard 后接 catch checked-exception; 关闭
  `JDEC_TRY_JUMP_ANCHOR_OFF` 即报 `unreported exception UnsupportedEncodingException` 或运行期异常逃逸,
  由 `TestJumpEnteredTryCatchAnchorIsLoadBearing` 守卫, 真实命中 commons-codec `QCodec.encode/decode`);
  新增 `StaticInitForwardRef` (承重负向锁定「<clinit> 字段初始化器越过副作用语句被提升」治本: SAFE 先 new
  再被 set() 循环改写, DERIVED=SAFE.clone() 作为 clinit 最后一步且声明在 SAFE 之前; 提升会 forward-ref +
  clone 空集; 关闭 `JDEC_NO_CLINIT_HOIST_BARRIER` 即报 `illegal forward reference`, 由
  `TestClinitHoistBarrierIsLoadBearing` 守卫, 真实命中 commons-codec `URLCodec`);
  新增 `ClassLiteralRendering` (锁定 ldc Class 字面量渲染治本: `String.class.getName()` 实例调用接收者 /
  `nameOf(Double.class)` 字面量作实参 / `int[].class`+`byte[][].class` 数组字面量 / 当前类自有字面量
  `ClassLiteralRendering.class.getName()` 实例调用, 同时静态调用 `Integer.parseInt` 保持裸类名不退化为
  `Integer.class.parseInt`; 真实命中 commons-codec `ColognePhonetic`;
  另含「Class 字面量存入局部多次读取」形态 `Class<?> c = Long.class; c.getName()/c.isPrimitive()/
  c.getSimpleName()` —— 捕获局部必须定型为 `java.lang.Class` 而非被引用类 `Long`, 否则成员读取报
  `cannot find symbol`; 关闭 `JDEC_NO_CLASSLIT_SLOT_TYPE` 即声明成 `Long c = Long.class;` 重编译失败,
  由 `TestClassLiteralSlotTypeIsLoadBearing` 承重守卫);
  新增 `IntCategoryNarrowing` (承重负向锁定本轮 4 个 int 计算类别治本: ① 单 slot 条件重赋值合并
  decodeOctet 形态、② byte 初值 + int 重赋值的声明宽化 getUnsignedOctet 形态、③ JLS 5.6.2 二元数值
  提升 byte/short/char 算术结果为 int、④ int 值存入 byte[] 重新补 `(byte)` 收窄转换; 关闭
  `JDEC_INTCAT_REASSIGN_SPLIT`/`JDEC_NO_BINNUM_PROMOTE` 即失败, 证明承重);
  含 `ControlFlowAlgorithms` (自然 sieve 守卫嵌套循环 + 2D 带标签 break/continue)、
  `SortingAlgorithms` (bubble/insert/select/quick/heap, 嵌套循环 + 递归 + 短路条件)、
  `ChainedAssignAlgorithms` (2/3/4 槽链式赋值 + 数组元素链 + 链后独立修改 + 终结链 +
  三元赋值副作用回读 `(cond)?(x=a):(x=b)` 两侧读位置, 锁定 Bug T 求值顺序治本)、
  `Sha256Algorithms` / `Sha512Algorithms` (从零实现, 分别压 32 位 int 与 64 位 long 位运算族,
  rotr / `>>>` / 调度数组 / 64-80 轮压缩, sanity 锁定空串摘要)、
  `CipherAlgorithms` (TEA / XTEA / RC4 加解密自校验, 回绕 int 算术 + 字节置换)、
  `StringSwitchAlgorithms` (String-switch 两段 hashCode+equals 降级 + char-switch fall-through;
  含「单方法内 String-switch 临时槽被后续 int 复用」合并形态, 锁定 Bug S slot 读取版本归属治本) 与
  `TryWithResourcesAlgorithms` (try-with-resources 降级: 单资源 / 多资源一 try 反序 close /
  twr+catch / twr+catch+finally(循环体, 正常退出后跑 finally 再 return) / 嵌套 twr(外层循环体里再套
  twr), 含 `Throwable primary` + `addSuppressed` 抑制机; 后两者锁定 Bug U「异常处理器边被循环结构化
  误当正常退出边」治本) 与
  `TryFinallyLoopAlgorithms` (非 twr 的纯 try/catch/finally + 循环体: for/while 循环正常退出后跑
  finally、嵌套循环带标签 break、long 累加、finally 体自身含循环, 锁定 Bug U 在非 twr 场景的同源治本) 与
  `ConversionRenameAlgorithms` (mirror guava UnsignedLongs.toString: 嵌套 else 分支里算出的 long
  被 `(int)` 强转消费, 深度碰撞重命名后强转操作数变量名串号, 锁定数值转换 ReplaceVar 转发治本)。
- 注解默认值 `default <value>` 治本 (承重 `TestAnnotationDefaultIsLoadBearing`): `@interface` 元素的
  `AnnotationDefault` 属性此前被当 `UnparsedAttribute` 丢弃, 反编译出的注解声明缺 `default false` 等子句,
  致**任何省略该元素的使用点**重编译失败 (`annotation @X is missing a default value for the element 'serializable'`)。
  这正是 guava 重编译率被压低的主因 (`@GwtCompatible`/`@GwtIncompatible` 几乎覆盖全 guava)。治本: 解析
  `AnnotationDefault` 的 element_value (复用 `ParseAnnotationElementValue`)、并在 dumper 抽象方法处补
  `default <value>` (复用抽出的 `formatAnnotationElementValue`, 覆盖 Z/I/s/c/e/[ 全标签); 同时捕获原始字节
  保 marshal 字节级一致。关 `JDEC_ANNO_DEFAULT_OFF` 即丢 default 重编译失败, 由承重测试守卫。
  实测 guava yak-syntax 重编译率 7%(131/1892) → 30%(574/1892)。
- 标准库嵌套类型点号渲染治本 (承重 `TestStdlibNestedDotIsLoadBearing` + 自动 round-trip battery
  `StdlibNestedDot`): 对 **外部标准库** (`java.*`/`javax.*`/`jdk.*`/`sun.*`/`com.sun.*`/`org.w3c.*`/
  `org.xml.*`/`org.ietf.*`/`org.omg.*`) 的嵌套类型引用, 此前按 Yak 自有扁平单元的二进制名 `Map$Entry` 渲染,
  而这些类型在 classpath 上只以真正嵌套的 `Map.Entry` 存在, 源码无法用 `Map$Entry` 解析 →
  `cannot find symbol: class Map$Entry`。这是 guava/spring 重编译的**最大单一阻断点** (仅 guava 全量批量
  编译就有 706 处 `Map$Entry` "cannot find symbol")。治本: `class_context.ShortTypeName` 对标准库包的嵌套
  类型改用 `binaryNestedNameToSource` 输出点号形式 (`Map.Entry`), import 仍带外层类 (`import java.util.Map;`)。
  标准库包永不会是 Yak 单元, 故 100% 安全; 自有扁平单元 (同 jar) 仍保持 `$` 以匹配扁平声明。关
  `JDEC_STDLIB_NESTED_DOT_OFF` 即退回 `Map$Entry` 重编译失败, 由承重测试守卫。
- 泛型方法零参返回类型还原治本 (承重 `TestGenericMethodReturnIsLoadBearing` + 自动 round-trip battery
  `GenericMethodReturn`): 方法 Signature `()TK;` 经 `ParseMethodSignature` 解析得 (nil 形参, K 返回),
  旧 `sigParams != nil` 闸门恰好跳过零参方法, 使 `getKey()`/`getValue()`/`iterator()` 等被擦除为
  `Object` 返回, 无法覆盖泛型接口方法 (`return type Object is not compatible with K`)。这是「标准库嵌套点号」
  修好后**新暴露**的 guava 次大阻断 (AbstractMapEntry / Maps$* / Multimaps$* 等)。治本: `dumper.go`
  方法签名覆盖改以 `sigRet != nil` 为闸门 (形参仍按数量匹配且仅在非 nil 时覆盖), 零参泛型返回得以还原为
  K/V; `<...>` 形参方法仍回退 descriptor (见下方残留说明)。关 `JDEC_METHOD_SIG_RET_OFF` 退回旧闸门即重编译
  失败, 由承重测试守卫。
- 内部类自由类型变量注入治本 (承重 `TestInnerClassTypeVarIsLoadBearing`): 非静态内部/局部/匿名类被 Yak
  扁平成顶层 `Outer$Inner` 单元后, 其从外层继承的类型变量 (K/V/E) 丢失声明 →
  `cannot find symbol: class K` (guava Multimap/Table/cache 内部类族此前约 2000 处)。治本: 在 `DumpClass`
  扫描本单元父类型签名 + 字段签名里实际引用的 `T<name>;` 变量, 给扁平类补声明 `<K, V>`; 仅限**本身不声明
  任何形参**的纯继承内部类 (否则声明/引用 arity 失配致 `wrong number of type arguments`)。关
  `JDEC_INNER_TYPEVAR_OFF` 即丢声明重编译失败。残留 (并入 Bug AD/V 跨类整体重建): ① 内部类构造器合成的
  外层实例形参使 Signature 形参计数比 descriptor 少 1, 致捕获形参 (如 `K owner`) 仍擦除为 `Object`; ②
  `this$0` 字段擦除为裸外层类型 (无 Signature), 经其读外层泛型字段得 `Object`; ③ 只引用外层变量**子集**
  的内部类, 注入 arity 可能与外层引用点 arity 失配。
- 泛型返回值类型变量强转治本 (承重 `TestTypeVarReturnCastIsLoadBearing` + 自动 round-trip battery
  `TypeVarReturnCast`): 上面「零参泛型返回还原」把 `max()` 正确定型为返回类型变量 `T` 后, 方法体若 `return`
  一个被推断成**擦除 bound** (`Comparable`) 的局部, 重编译报 `incompatible types: Comparable cannot be
  converted to T` (字节码把 `()TT;` 擦除到 bound, 局部即 bound 类型)。这是「零参泛型返回还原」在 gated
  `Generics` 语料引入的回归。治本: `ReturnStatement.String` 在「方法返回类型是类作用域类型变量 (经
  `ClassContext.TypeParams` 识别) 且返回值静态类型为不同引用类型」时补 `return (T) (expr)` 无检查强转
  (与 CFR/Fernflower 一致, 行为等价); `TypeParams` 由 `DumpClass` 用类形参 + 注入的自由变量填充。关
  `JDEC_TYPEVAR_RET_CAST_OFF` 即丢强转重编译失败, 由承重测试守卫。
- OPCODE 解析覆盖门禁 `TestOpcodeParseCoverage`: **195/195 (100.0%)** (语料 126 class + 31 battery,
  命中 198 distinct opcode), 7 个文档化排除
  (jsr / jsr_w / ret / goto_w / wide / ldc_w / nop —— 均为 javac 不由源码产生或前缀修饰)。
- 全量 `go test ./common/javaclassparser/...` 全绿。

---

## 未修复缺陷 (下一轮治本目标)

### Bug V — enum 高级 idiom 跨类重建 (需多类整体反编译能力, 非单类 bug)

当前架构 (`javaclassparser.Decompile` + jar parser) 对每个 `.class` **独立**反编译并各自落一个
`.java`, 没有「把内部类折叠进外层类」的整体重建能力。两类 enum idiom 因此产出**不可编译**输出:

1. **enum 常量体 (constant-specific class body)** —— 形如
   `enum Op { ADD { long apply(...){...} }, MUL { ... }; abstract long apply(...); }`。
   javac 把每个带体常量降级成合成子类 `Op$1`/`Op$2`(带 `ACC_ENUM` 且 `extends Op`)。
   - 症状: 反编译 `Op` 得到 `abstract long apply(...)` 但常量 `ADD/MUL` 无内联体 →「ADD 未覆盖
     抽象方法」; 反编译 `Op$1` 得到 `class Op$1 extends Op` →「枚举类型不可继承」。两者**单类都无法
     编译**, 唯一正确写法是把 `Op$N` 的方法体折叠回常量声明。
   - 复现: `/tmp/grt/EnumBody.java` (最小); 真实命中 guava `LongMath$MillerRabinTester`。
   - 规避现状: `dumper.go` 已把合成子类降级成普通 class (去 `enum` 关键字), 但 `extends <enum>`
     仍非法。彻底治本需新增「外层 + 全部内部类一起喂入」的反编译入口, 在 dumper 里按 `<clinit>`
     的 `ADD = new Op$1("ADD",0)` 映射把子类体内联进常量、并抑制独立 `Op$N` 输出。

2. **enum-switch 脱糖** —— `switch(enumVar)` 被 javac 降级成合成类 `Outer$1` 内的
   `$SwitchMap$<Enum>[] ` 序数映射数组 + `enumVar.ordinal()` 查表。
   - 症状: 反编译保留低层 `Outer$1.$SwitchMap$...[v.ordinal()]` 形态而非还原 `switch(enum)`。
   - 现状: 当外层与合成 `Outer$1` 两个 class **同时**反编译并一起编译时可正常编译运行 (实测), 故
     列为「美观/idiom 复原」低优, 非阻断正确性。真实整 jar 反编译会同时产出两文件。

> 两者同源: 都需要**跨类整体重建** (multi-class folding) 能力, 是下一个独立大特性, 不做半成品式
> 改动以免破坏现有逐类架构与全量回归。

### Bug Y — 分支/条件内赋值的目标局部声明作用域过窄, 被读出作用域 (int 嵌入式赋值已治, 引用形态残留)

- 已治 (int 嵌入式赋值): `if ((var4 = expr) == 0)` / `(v = a[i]) < n` 这类「dup-collapse 形成的嵌入式
  赋值致变量无独立声明」, 安全网原先猜成 `Object v = null` 致 int 存储与算术读取重编译失败; 现在
  `generatedLocalLooksInt` 识别「嵌入式赋值参与关系比较 / 与整型字面量等值比较」补成 `int v = 0`。
  回归 battery `EmbeddedAssignDecl` + 承重测试 `TestEmbeddedAssignDeclIntIsLoadBearing`
  (关 `JDEC_NO_EMBED_ASSIGN_INT` 即退化报 Object/int 不兼容)。
- 残留 (引用嵌入式赋值): `(s = parts[i]) != null` 后接 `s.length()` → 安全网仍补 `Object s = null`,
  致引用方法调用 `cannot find symbol`。根因: dumper 末端的字符串安全网 (`addMissingGeneratedLocalDecls`)
  丢失了真实 ref 类型, 只能靠启发式; 引用类型无法从字符串可靠反推。治本需把真实类型从
  `ParseBytesCode` 的 statementList 透传到安全网 (varN→type 映射), 属跨层管线改动。
- 残留 (纯分支声明): `if(...) v=x; ... use(v);` 中 v 声明被限制在 if 块内 →
  `cannot find symbol`。需把声明提升到支配作用域 / 跨分支合并同槽局部 (与 Bug W 同源)。
- 复现: commons-codec `Metaphone` / `MatchRatingApproachEncoder` / `DaitchMokotoffSoundex` /
  `bm.Rule`。

### Bug AC — 控制流结构化产出 'return outside method'

- 症状: 某些控制流形态下出现位于方法体外的 `return`, javac 报 "return outside method"。
- 根因: 控制流结构化在特定 (含 switch/嵌套 try) 形态下越过方法边界生成语句。
- 复现: commons-codec `DaitchMokotoffSoundex` / `cli.Digest`。

### Bug AD (残留) — 非标准库外部依赖 / 同 jar 跨包嵌套类型按 `$` 引用

- JDK/标准库子集已治本 (见当前状态「标准库嵌套类型点号渲染」, `Map.Entry` 等), 占 guava `cannot find
  symbol` 的最大头 (706 处)。以下两类残留仍未治:
  1. **非标准库外部依赖 jar** 的嵌套类型 (如 `com.google.errorprone.*$*`, `org.checkerframework.*$*`):
     这些在 classpath 上也是真正嵌套的, 应渲染 `.`; 但单类入口无法区分「同 jar 自有扁平单元」与「依赖 jar
     外部类型」(两者都可能是 `com.google.*` 前缀), 故未扩到非标准库包。
  2. **同 jar 跨包**嵌套类型 (如 base 包引用 collect 的 `Multiset$Entry`): 自有扁平单元需保持 `$`,
     与 §「Bug V / 根因 B」同源, 属跨类整体重建特性。
- 复现: guava `Multimaps$Keys$1` (`Multiset$Entry`)、commons-codec `DaitchMokotoffSoundex`。

### Bug AG (残留) — 方法级形参类型变量 `<...>` 签名未还原

- 零参泛型返回 (`()TK;`) 已治本 (见当前状态「泛型方法零参返回类型还原」)。残留: 方法**自带形参类型变量**的
  签名 (`<T:...>(...)...`, 如 `<T> T[] toArray(T[] a)`) 仍不还原。
- 原因有二: (1) `ParseMethodSignature` 遇到 sig 首字符 `<` 直接返回 (nil,nil) 回退 descriptor;
  (2) 即便解析了形参与返回, dumper 目前不渲染方法级 `<T>` 声明 (仅 `ParseMethodSignatureTypeParams` 提取但
  未接入渲染), 贸然代入 `T foo(T)` 会因 `T` 未声明而编译失败。故需「解析 `<...>` + 渲染方法级 `<T>` 声明 +
  形参/返回代入」三者一并落地, 属下一轮独立特性 (风险集中在方法头渲染, 建议带 kill-switch)。
- 影响: guava 含方法级泛型的工具方法 (`<T> T[] toArray`, `<E> ImmutableList<E> of(...)` 等) 仍可能不可重编译;
  但类级类型变量 (K/V/E) 的方法此前已随零参修复覆盖大部分常见集合接口。
- 复现: guava `collect/ObjectArrays` (`<T> T[] newArray(...)`)。

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
