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
- 本阶段: slot 复用自引用初始化**已治本修复** + 去掉 CODEC_STRICT skip 变硬门禁 +
  battery 扩展 (SHA-1/SHA-256/Adler-32/Base64 解码), 全部 byte-for-byte 通过, 待提交。
- 下一轮: 修后自增数组下标 `arr[i++]` BUG -> 扩展 HMAC/Base32 -> 推进真实库 round-trip。
