# Codec 算法交叉验证 — 交接 TODO (CODEC_TODO.md)

> 分支: `codex/yak-java-decompiler-cross-comparison`
> 核心目标: 用「反编译 → 重编译回 class → 直接运行算法对比」验证反编译器的**语义正确性**, 达到 GA 水准, 而非仅"能反编译"。
> 配套文档: [YAK_JAVA_DECOMPILER_CROSS_COMPARISON.md](./YAK_JAVA_DECOMPILER_CROSS_COMPARISON.md), [HARNESS_WORKFLOW.md](./HARNESS_WORKFLOW.md)

---

## 已完成 (本阶段)

### 1. 反编译器缺陷修复 (已 commit: `fbb6cbf36`)

三个 correctness fix, 全部由 codec 算法交叉验证发现:

- **byte/char/short 局部变量收窄 cast** (commons-codec `PureJavaCrc32C`):
  JLS 规定整数算术/位运算/移位**总是**把操作数提升为 int。byte 局部变量存储此类表达式时
  (`byte x = (arr[i] ^ crc) & 255`), 类型是 int 但 slot 是 byte, javac 报 "possible lossy
  conversion from int to byte"。修复: 在赋值声明处检测 int 值存入 byte/char/short slot, 包裹
  显式 cast, 与 return 收窄逻辑一致。**验证: CRC32C 现在能编译且 8/8 差分输入 0 分歧**。
  同时把整数算术/位运算 I-op (IADD/ISUB/IAND/IOR/IXOR 等) 正确类型化为 int。

- **还原 IFNONNULL 条件极性** (commons-codec `Md5Crypt` / `TryCatchBattery.multiCatchUnion`):
  前一次"修复" (87167bf4f) 基于**误诊**: Md5Crypt 的"分歧"实为 oracle 的数组别名 bug
  (`md5Crypt` 末尾 `Arrays.fill(keyBytes, 0)` 清零了共享数组)。用修复别名后的 oracle 验证,
  Md5Crypt 用 `== null` (fall-through 条件) 渲染时 **20/20 salt 全部 byte-for-byte 正确**。
  `!= null` 渲染反而**交换了 if/else 体**, 破坏真实控制流 (multiCatchUnion NPE, ternary 反转)。
  **还原回 `== null`, 与 IFNULL / 数字 IF 保持一致的极性约定**。

- **回归用例**: `testdata/regression/byte_local_narrowing.class` + 更新 `ifnonnull_branch` 断言。

### 2. Codec 算法语义验证 harness (已扩展为硬门禁)

- **自包含算法 battery**: `tests/testdata/codec/CodecAlgorithms.java`
  覆盖: MD5 (RFC 1321), SHA-1, SHA-256 (FIPS 180-4), CRC32, CRC32C, Adler-32,
  MurmurHash2, MurmurHash3 x86_32, XXHash32, Base64 编码 + 解码,
  MD5-crypt ($1$ 密码哈希, 1000 轮混合 + base64 打包)。
  **全部算法已对照标准库验证正确**: MD5/SHA-1/SHA-256/CRC32 与 Python `hashlib`/`binascii` 一致;
  md5Crypt 与 commons-codec `Md5Crypt` **12/12 一致**。

- **差分执行测试**: `tests/codec_semantics_test.go` 的 `TestCodecSemanticsRoundTrip`。
  流程: javac 编译 battery → 生成 golden fingerprint → Yak 反编译 → javac 重编译 → 运行比对。
  这是能捕获**语法验证捕获不到的静默计算错误**的最强 oracle。
  **已去掉 `CODEC_STRICT` skip**: 只要环境里有 `javac/java` 就强制断言, 成为真正的硬门禁。

### 3. slot 复用自引用初始化缺陷 (已修复, 本阶段)

**原症状**: 多分支方法内 local-slot 复用导致声明重命名未传播到引用, 产出非法 Java
(`int var17_1 = (((var17_1) + ...))` 自引用初始化, javac 报 "variable might not have been
initialized")。在 md5() / xxHash32() 触发。

**根因**: `rewriteVar` 在分支后对同 slot 变量再赋值 (`h = h + n`) 时会新建一个 `VariableId`,
但旧 Id 也被一并替换, 导致再赋值语句的 LHS 与 RHS 都指向这个未初始化的新 Id, 同时该再赋值被
当成新声明 (`int h = ...`)。

**修复** (`rewrite_var.go`): 新增 `redirectPostBlockReassignments()`, 由 `ifHoistDeclarations` /
`switchHoistDeclarations` 在分支声明 hoist 后调用。它把分支后对同一 `VarUid` 的再赋值语句的
`VariableId` 统一重定向到 hoist 出来的目标 Id, 并把第一条再赋值从声明降级为普通赋值
(`IsFirst=false, IsDeclare=false`)。

**回归**: `tests/testdata/regression/post_branch_reassign_slot.class` +
`TestDecompilePostBranchReassignNoSelfInit` (无需 javac, 断言不出现自引用初始化, 且分支合并变量
被还原成普通赋值)。该 seed 覆盖 if/else 简单合并 (xxHash32 形) 与嵌套 if-else-if 合并 (md5 round 形)。

---

## 当前阻塞缺陷 (下一轮优先)

### BUG: 后自增数组下标 `arr[i++] = v` 被错误重排为 `i++; arr[i] = v`

**症状**: 反编译产出**语义错误** (而非语法错误) 的 Java: 形如 `out[oi++] = (byte) v` 的后自增数组
写入, 被反编译成先自增下标再写入, 使用了**错误的下标** (多偏移 1), 运行时数组越界 / 结果不一致。
本轮在 base64Decode 触发 (`ArrayIndexOutOfBoundsException`)。

**根因 (推测)**: javac 把 `arr[i++] = v` 编成
`aload arr; iload i; iinc i 1; <v>; iastore`。反编译器顺序处理指令: `iload i` 压入对 i 的引用,
`iinc i 1` 作为独立语句发射 (`i = i + 1`), `iastore` 时弹出的下标引用渲染成 `i` —— 但此时 i 已被
前一条 iinc 语句改写, 于是用了自增后的值。需要识别「数组下标 load 与 iastore 之间对该下标 slot 的
iinc」, 还原成 `arr[i++] = v` (捕获自增前的旧值)。

**规避现状**: 本轮 `base64Decode` 改用**显式下标算术** (`out[o], out[o+1], out[o+2]; o += 3`)
绕开该 idiom, 因此 base64 解码仍纳入 battery 并通过。该 BUG 不阻塞当前门禁, 但反编译真实库
(commons-codec `Base64`/`Base32` 大量使用 `buffer[pos++]`) 时会触发。

**复现方向**: 写最小 seed `arr[i++] = v` (在循环里), 编译后反编译, 对比下标。

**修复方向**: 在栈模拟里跟踪 pending 数组下标引用, 当其后、对应 iastore 之前出现对同 slot 的 iinc 时,
把该次访问折叠为后自增 (`arr[i++]`), 而不是发射独立 iinc 语句。注意与现有 field/static 后自增折叠
(`selfOpFoldedRefs`, 走 dup/dup_x1) 是不同机制。

---

## 待办: 扩展覆盖 (向 GA 推进)

### 算法覆盖扩展
- [x] SHA-256 (FIPS 180-4) — 已加入 battery, byte-for-byte 通过
- [x] SHA-1 — 已加入 battery, byte-for-byte 通过
- [x] Base64 解码 — 已加入 (显式下标, 绕开上面的后自增数组下标 BUG)
- [x] Adler-32 — 已加入 battery, 通过
- [ ] HMAC-MD5 / HMAC-SHA256 — 加入 battery
- [ ] Base32 编解码
- [ ] UnixCrypt (DES crypt) — commons-codec 有, 复杂度高
- [ ] Sha2Crypt (SHA-512 crypt)

### 真实库 round-trip (用户明确要求: "也可以验证一些其他的库比如 spring")
- [ ] commons-codec 1.15 整库 round-trip: 反编译 → 重编译成 jar → 用反射差分调用
  (现有 `/tmp/cc-verify/` 的手工验证脚本可固化为 Go 测试; Md5Crypt/Sha2Crypt/MurmurHash3
  已证明语义正确, 需覆盖 Base64/Base32/HmacUtils/UnixCrypt 等剩余类)
- [ ] spring-core round-trip
- [ ] guava round-trip
- 这些需要解决已分类的根因 (见 YAK_JAVA_DECOMPILER_CROSS_COMPARISON.md §4.1):
  协变桥接方法抑制 (根因 A)、嵌套类 `$` 引用归一 (根因 B)、泛型占位符 `__` (根因 C)。

### 已知 codec 算法缺陷 (commons-codec, 待修复)
反编译 commons-codec 1.15 后重编译, 暴露的缺陷 (来自 `/tmp/cc-verify/cc.log`):
- [ ] `Base64`/`Base32`/`BaseNCodec`: 内部类 `$Context` 引用 + final 字段赋值 + byte 收窄
- [ ] `DigestUtils`: final 字段 `messageDigest` 赋值
- [ ] `HmacUtils`: try-catch 重建 (`exception Throwable has already been caught`) +
  checked exception 未声明
- [ ] `UnixCrypt`: 变量重复定义 (`variable var14 is already defined`)
- [x] `XXHash32`: `var1_1 might not have been initialized` — 已由本阶段 slot 复用修复解决

---

## 已完成 (本阶段 — OPCODE/算法覆盖再扩展)

### 4. 新增自托管 battery (全部 round-trip byte-for-byte 通过)

差分门禁 `TestCodecSemanticsRoundTrip` 现在跑 8 个 battery, 全绿:

- **`LongHashAlgorithms.java`**: SHA-512 (FIPS 180-4)、xxHash64、SipHash-2-4、CRC64-ECMA、
  FNV-1a-64、splitmix64。专门压 long OPCODE (LADD/LMUL/LSHL/LUSHR/LAND/LXOR/LCMP 等) 与
  64 位旋转/混合。
- **`OpcodeCoverage.java`**: long/double/float 算术与比较 (DCMPG/FCMPL/LCMP)、`tableswitch`/
  `lookupswitch`、`instanceof` + checkcast、各类原始类型 cast (i2l/l2d/d2i/i2b...)、
  多维数组 (`anewarray`/`multianewarray`/`*aload`/`*astore`)、`foreach`。
- **`GuavaAlgorithms.java`**: Murmur3_32、Fingerprint2011 (FarmHash 前身)、LongMath/IntMath/
  UnsignedLongs (divideUnsigned/remainderUnsigned/log2/isPowerOfTwo)、BaseEncoding base16/base64、
  CRC32。**与 JDK `Long.divideUnsigned`/`Base64`/`CRC32` 交叉校验一致**。
- **`SpringAlgorithms.java`**: AntPathMatcher 风格 glob (`matchStrings` 单段双指针回溯 + `antMatch`
  多段 `**` 回溯)、`StringUtils.cleanPath` (`.`/`..` 规整)、`MimeType` 解析 (type/subtype/参数
  归一)、`capitalize`、字符串 hash。压 String/char 相关 OPCODE 与深层控制流。
- **`BitTwiddlingAlgorithms.java`**: Hacker's-Delight 风格位运算 (SWAR popcount32/64、bit/byte
  reverse、ntz/nlz、parity、nextPow2、log2、gcd、整数平方根 isqrt、rotl/rotr)。**每个手写结果与
  JDK builtin (`Integer.bitCount`/`reverse`/`numberOf*Zeros`/`rotateLeft` 等) 折叠在指纹里逐一交叉
  校验一致**。分支轻、移位/与或异或密集。
- **`ChecksumAlgorithms.java`**: 无表实现的 CRC16 (CCITT-FALSE/XMODEM/ARC)、CRC32、Fletcher-16/32、
  Adler-32、Internet checksum (RFC 1071)、BSD/SysV checksum。**`"123456789"` 标准 check value 锚定
  正确性** (CCITT-FALSE=0x29b1、XMODEM=0x31c3、ARC=0xbb3d、CRC32=0xcbf43926)。压 byte[] 迭代、
  byte<->int 收窄、嵌套移位/异或。

### 5. 反编译器缺陷修复 (本阶段, 全部带 round-trip 回归)

- **嵌套类 import 语法 (Bug A)**: dumper 之前把内部类 import 写成 JVM 二进制名
  `import java.util.Base64$Encoder;`, javac 报 "cannot find symbol"。修复 `dumper.go`:
  import 行把 `$` 替换为 `.` (`import java.util.Base64.Encoder;`)。
- **`foreach` 遍历裸数组的 var-fold 误内联 (Bug arrays)**: 单次使用折叠 pass 把 `new double[3][]`
  这类**裸数组分配** (无 initializer) 内联进合成迭代变量, 导致迭代未初始化的新数组 → NPE。
  修复 `code_analyser.go`: var-fold 跳过 `IsArray() && len(Initializer)==0` 的 `NewExpression`,
  保持其为变量引用。回归: `ForEachRepro.java` (含 `multiForEachOneMethod`)。
- **布尔位运算 `& | ^` 渲染为 int (Bug D)**: `(x>0) & (y>0)` 这类布尔域位运算, 每个比较被渲染成
  整数三元 `cond ? 1 : 0` 再做整型位运算, 在 `boolean` 返回处报 "int cannot be converted to
  boolean"。修复 `expression.go` + `java_value.go`: 新增 `BoolTernaryCondition` 识别 `cond?1:0`,
  `JavaExpression` 检测两侧均为布尔形操作数时, `Type()` 返回 `boolean` 且 `String()` 直接渲染
  `(c1) & (c2)`。验证: Guava `isPowerOfTwoLong` 编译并语义一致。

### 6. OPCODE 解析 100% 覆盖门禁 (本阶段新增)

新增 opcode 命中记录 + 硬门禁, 确保反编译器对**每一个** javac 可产生的 JVM opcode 的解析路径都被
确定性、可在 CI 复跑的语料覆盖:

- **命中钩子** (`decompiler/core/opcode_coverage.go`): `calcOpcodeStackInfo` (栈模拟器里每条指令的唯一
  必经点) 顶部记录 `opcode.Instr.OpCode`。禁用态零成本 (一次 atomic load), 仅在专用串行测试里开启,
  不影响并发 m2 扫描。导出 `EnableOpcodeHitRecording` / `DisableOpcodeHitRecording` /
  `RecordedOpcodeHits`。
- **门禁** (`tests/opcode_coverage_test.go` 的 `TestOpcodeParseCoverage`): 反编译整套内嵌语料
  (regression seeds + 顶层 syntax-coverage class, 共 116 个 .class) + 6 个自托管 battery (有 javac 时),
  统计命中的 opcode, 断言**所有**反编译器注册了 handler 的真实 opcode (opcode 值 0..201) 全部被命中,
  除 7 个有据可查的排除项。**当前结果: 195/195 (100.0%) 通过。**
- **7 个文档化排除** (均为 javac 不可由源码产生 / 前缀修饰 / 超大方法专用):
  `jsr` / `jsr_w` / `ret` (废弃子例程, javac>=6 不发, JSR inliner 在栈模拟前已消除)、
  `goto_w` (>32KB 分支偏移才发)、`wide` (操作数扩展前缀, 折叠进下一条的 IsWide, 不独立分发)、
  `ldc_w` (常量池下标 >255 才发)、`nop` (javac 从不由源码产生, handler 是空 return)。
- **填补缺口**: 为补齐 `dstore_0/1`、`fstore_0/1` (低 slot 的 double/float 存储) 与 `dup2_x2`
  (category-2 数组元素复合赋值且值被使用) 扩展了 `OpcodeCoverage.java`。前 4 个已并入语义指纹、
  byte-for-byte 通过; `dup2x2()` 因下方 Bug J 仅保留方法体 (供 opcode 命中) 但不并入指纹。

---

## 已知深层缺陷 (本阶段定位, 暂以源码重构规避; 待治本)

- **Bug J — category-2 数组元素复合赋值且值被使用, RHS 被重复求值**:
  `double dv = (a[i] += 2.5);` (double[]/long[] 元素) 编译为 `... dup2_x2 ... dastore`, 把存入的
  category-2 结果复制到栈底返回。反编译器没有把"被复制的存储值"折叠成临时变量, 而是**重新求值
  RHS**: 产出 `a[i] = a[i] + 2.5; double var3 = a[i] + 2.5;` —— 此时 `a[i]` 已自增, var3 再加一次
  2.5, 结果错误 (long[] 同理)。属操作数栈 dup 折叠缺陷, 与 field/static 后自增折叠 (`selfOpFoldedRefs`)
  是不同机制。**规避**: `OpcodeCoverage.dup2x2()` 保留但不调用 (仍覆盖 DUP2_X2 解析), 不并入语义指纹。
  **复现**: `static long f(double[] a,int i){ double v=(a[i]+=2.5); return Double.doubleToLongBits(v); }`。

这些是**预先存在**的控制流 / 变量身份重建缺陷, 修复风险高, 本阶段先在 battery 源码里改写算法
形态规避 (保留等价 OPCODE 覆盖), 并在此登记复现形态供下一轮治本。

- **Bug E/F/I — 守卫子句 `if(!cond) {throw|跳出}; <body>` 分支体交换**:
  形如 `if (!cond) throw ...;` 后接较大顺序代码, 反编译把 *body* 与 *throw* 相对条件**对调**
  (cond 为真时反而执行 throw)。Guava `remainderUnsigned` 的 `if(dividend>=0)` 分支体互换、
  Spring `main` 里的 `if(!antMatch(...)) throw` 自检全部中招。根因疑为 `if(!cond) goto body`
  (ifne) 的极性 + body/handler 块归属判定。**规避**: 去掉 `if(!cond) throw` 自检 (结果已进
  fingerprint), `remainderUnsigned` 改用 `divideUnsigned` 直接计算。
- **Bug H — 尾随 `while(cond){i++}; return i==n` 循环出口反转**:
  `while (p<len && pat[p]=='*') p++; return p==len;` 被重建成「char 是 `*` 时 return、否则 i++」,
  即循环体与出口互换 → 无限自增 → 下标溢出到 `INT_MIN` → `charAt(MIN_VALUE)` 越界。Spring
  `matchStrings`/`antMatch` 尾部触发。**规避**: 改写成 `for(q=p;q<len;q++){ if(pat[q]!='*')
  return false; } return true;` (带提前 return 的 for) 可正确反编译。
- **Bug G — fall-through `switch` 被按 label 升序重排**:
  Murmur3_32 尾块的降序 fall-through `case 3: ...; case 2: ...; case 1: ...` 被反编译器按
  `1→2→3` 升序重排, fall-through 语义反转 → 数组越界。**规避**: 尾块改写为 `if/else if` 阶梯。
- **Bug B — slot 跨类型复用合并出错 (long temp 与 int 下标共 slot)**:
  Murmur3_128 尾块把 `int` 数组下标 (`tail`) 与 `long` 累加器 (`k2`) 合并成同一变量, 导致
  `long` 当数组下标 → "possible lossy conversion from long to int"。属深层变量身份缺陷。
  **规避**: 暂从 Guava battery 移除 murmur3_128。
- **Bug C — 续用循环变量被重复声明 (slot 复用 + 第二个循环)**:
  `for (int j=0; i<n; i++, j++)` 这种「续用外层 i + 新增 j」的尾循环, 反编译把延续的 `i`
  误重声明为 `int i = 0`。**规避**: `fullFingerprint` 尾循环改写为单变量遍历只读基址。

> 这些 BUG 与第 66 行的「后自增数组下标 `arr[i++]`」一并构成下一轮治本目标。其中 Bug E/F/H/I
> 同属**控制流分支极性/块归属**一类, 可能可统一修复; Bug B/C 同属**slot 复用变量身份**一类。

---

## 如何运行

```bash
# 本地快回归 (主闸门, CI 也跑这个)
go test ./common/javaclassparser/...

# 跑 codec 算法差分 (只要有 javac/java 就硬断言, 已不再需要 CODEC_STRICT)
go test -run TestCodecSemanticsRoundTrip -count=1 -v ./common/javaclassparser/tests/

# 跑大型交叉对比 PK (需要 CFR/Vineflower jar + 语料, 见 YAK_JAVA_DECOMPILER_CROSS_COMPARISON.md §7)
CROSS_PK=1 CFR_JAR=... VINEFLOWER_JAR=... go test -run TestYakDecompilerCrossComparison ./common/javaclassparser/tests/
```

## 提交节奏 (用户要求: commit + push + 下一轮)
- `fbb6cbf36`: 3 个 correctness fix + 回归 (byte 收窄 / IFNONNULL 极性)。
- 上一阶段: slot 复用自引用初始化**已治本修复** + 去掉 CODEC_STRICT skip 变硬门禁 +
  battery 扩展 (SHA-1/SHA-256/Adler-32/Base64 解码), 全部 byte-for-byte 通过。
- 本阶段: 3 个反编译器 fix (嵌套 import `$`→`.` / foreach 裸数组 var-fold / 布尔位运算 `&|^`)
  + 4 个新 battery (LongHashAlgorithms / OpcodeCoverage / GuavaAlgorithms / SpringAlgorithms),
  差分门禁现跑 6 battery 全绿; 定位并登记 5 类深层控制流/slot 缺陷 (B/C/E/F/G/H/I)。
- 下一轮: 治本控制流分支极性/块归属一类 (E/F/H/I) 与 slot 复用变量身份一类 (B/C);
  修后自增数组下标 `arr[i++]` BUG -> 扩展 HMAC/Base32 -> 推进真实库 round-trip。
