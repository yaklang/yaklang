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

### 2. Codec 算法语义验证 harness (已 commit, 见下)

- **自包含算法 battery**: `tests/testdata/codec/CodecAlgorithms.java`
  覆盖: MD5 (RFC 1321), CRC32, CRC32C, MurmurHash2, MurmurHash3 x86_32, XXHash32, Base64,
  MD5-crypt ($1$ 密码哈希, 1000 轮混合 + base64 打包)。
  **全部算法已对照标准库验证正确**: MD5/CRC32 与 Python `hashlib`/`binascii` 一致;
  md5Crypt 与 commons-codec `Md5Crypt` **12/12 一致**。

- **差分执行测试**: `tests/codec_semantics_test.go` 的 `TestCodecSemanticsRoundTrip`。
  流程: javac 编译 battery → 生成 golden fingerprint → Yak 反编译 → javac 重编译 → 运行比对。
  这是能捕获**语法验证捕获不到的静默计算错误**的最强 oracle。

---

## 当前阻塞缺陷 (下一轮优先)

### BUG: 多分支方法内 local-slot 复用导致声明重命名未传播到引用

**症状**: 反编译产出非法 Java, javac 报 "variable varN might not have been initialized"。
具体: 声明点渲染成 `int var17_1 = ...` (重命名加 `_M` 后缀), 但引用点仍用旧名, 形成自引用初始化。
在 md5() 和 xxHash32() 方法触发。

**复现**: `CODEC_STRICT=1 go test -run TestCodecSemanticsRoundTrip -count=1 -v ./common/javaclassparser/tests/`

**根因定位**:
- `dumper.go` 的 `declareLocalInScope()` (~1422 行) 在 slot 名冲突时给声明点的 `VariableId` 调用
  `SetName("varN_M")`。
- 但引用点的 `JavaRef` 持有**不同的 `VariableId` 实例**, SetName 不传播。
- 根源在 `code_analyser.go`: 同一 slot 在不同作用域/分支被合并为同名变量时, 声明点和引用点的
  `JavaRef.Id` 是独立实例 (`rewrite_var.go` 的 `newRef.Id = newId`)。
- 最小复现: 同一方法内, 一个 if/else 两个分支各自首次赋值给同一 slot 的变量, 外层循环复用该 slot。

**已验证的相关逻辑**:
- `DoWhileStatement.ReplaceVar` / `IfStatement.ReplaceVar` **递归**更新 body, 没问题。
- 最小单循环复现 (SelfInit) **正常** (slot 只用一次, 无冲突, 不触发重命名)。
- 把每个表初始化循环抽到独立方法 (本阶段已做) **规避**了 CRC32/CRC32C 的问题。

**修复方向** (供接力参考):
1. **方向 A (治本)**: 在 `declareLocalInScope` 重命名后, 把新名字传播到所有引用**同一 slot**
   的 JavaRef (需要 JavaRef -> slot 的映射, 或在 Id 共享处修复)。
2. **方向 B (治本, 推荐)**: 修 `code_analyser` 的 slot 合并逻辑, 确保同一 slot 在所有引用点
   共享**同一个 VariableId 指针**, 这样 SetName 自动传播。
3. **方向 C (兜底)**: 若治本风险大, 在 dumper 输出阶段扫描自引用初始化
   (`int x_M = ...; x = x ...`), 把引用重写为重命名后的名字。但这是症状修补, 治本更好。

**验证**: 修好后, `CODEC_STRICT=1 go test -run TestCodecSemanticsRoundTrip` 应通过
(去掉 test 里的 CODEC_STRICT skip, 变成硬门禁)。

---

## 待办: 扩展覆盖 (向 GA 推进)

### 算法覆盖扩展 (修好上面的 BUG 后)
- [ ] SHA-256 (FIPS 180-4) — 加入 battery
- [ ] SHA-1 — 加入 battery
- [ ] HMAC-MD5 / HMAC-SHA256 — 加入 battery
- [ ] Base32 编解码
- [ ] Base64 解码 (目前只测了编码)
- [ ] UnixCrypt (DES crypt) — commons-codec 有, 复杂度高
- [ ] Sha2Crypt (SHA-512 crypt)
- [ ] Adler-32

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
- [ ] `XXHash32`: `var1_1 might not have been initialized` (同上面的 slot 复用 BUG)

---

## 如何运行

```bash
# 本地快回归 (<=30s, 主闸门, CI 也跑这个)
go test ./common/javaclassparser/...

# 跑 codec 算法差分 (默认 skip 已知缺陷; CODEC_STRICT=1 强制运行并断言)
CODEC_STRICT=1 go test -run TestCodecSemanticsRoundTrip -count=1 -v ./common/javaclassparser/tests/

# 跑大型交叉对比 PK (需要 CFR/Vineflower jar + 语料, 见 YAK_JAVA_DECOMPILER_CROSS_COMPARISON.md §7)
CROSS_PK=1 CFR_JAR=... VINEFLOWER_JAR=... go test -run TestYakDecompilerCrossComparison ./common/javaclassparser/tests/
```

## 提交节奏 (用户要求: commit + push + 下一轮)
本阶段: `fbb6cbf36` 已提交 3 个 fix + 回归。codec harness + battery + 本 TODO 待提交 (见 git status)。
下一轮: 修上面的 slot 复用 BUG -> 去掉 CODEC_STRICT skip -> commit + push -> 扩展算法/库覆盖。
