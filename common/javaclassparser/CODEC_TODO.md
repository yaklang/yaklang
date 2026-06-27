# Codec 算法交叉验证 — 交接 TODO (CODEC_TODO.md)

> 分支: `codex/yak-java-decompiler-cross-comparison`
> 核心目标: 用「反编译 → 重编译回 class → 直接运行算法对比」验证反编译器的**语义正确性**, 达到 GA 水准, 而非仅"能反编译"。
> 配套文档: [YAK_JAVA_DECOMPILER_CROSS_COMPARISON.md](./YAK_JAVA_DECOMPILER_CROSS_COMPARISON.md), [HARNESS_WORKFLOW.md](./HARNESS_WORKFLOW.md)
>
> 本文件只记录**尚未修复**的缺陷与待扩展项; 已治本的 bug 不再保留文字记录 —— 其回归测试 (testdata
> 种子 + `Test*` 守卫) 即永久档案。新发现的 bug 必须在此登记 (症状 / 根因 / 复现 / 规避现状)。

---

## 当前状态

- 差分门禁 `TestCodecSemanticsRoundTrip`: **31 个自托管 battery 全绿** (byte-for-byte round-trip);
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
  finally、嵌套循环带标签 break、long 累加、finally 体自身含循环, 锁定 Bug U 在非 twr 场景的同源治本)。
- OPCODE 解析覆盖门禁 `TestOpcodeParseCoverage`: **195/195 (100.0%)** (语料 126 class + 31 battery,
  命中 198 distinct opcode), 7 个文档化排除
  (jsr / jsr_w / ret / goto_w / wide / ldc_w / nop —— 均为 javac 不由源码产生或前缀修饰)。
- 全量 `go test ./common/javaclassparser/...` 全绿。

---

## 未修复缺陷 (下一轮治本目标)

> Bug S (String-switch 复用槽 slot 读取版本归属串号)、Bug T (含赋值副作用的三元值折叠后求值
> 顺序被破坏)、Bug U (try-with-resources/try-finally 循环体正常退出需流向 try 之后延续时, 异常
> 处理器边被循环结构化误当正常退出边 —— `LoopJmpRewriter` 把 catch 边改写成 `break` 致 catch 丢失
> + `Exception` 占位符泄漏; `searchCircleEndNode` 把 catch 后继当 loop out-edge 致 `loopEnd` 串号、
> 循环丢 `break` 死循环) 均已治本, 文字记录按约定删除。其回归测试 (`StringSwitchAlgorithms` 合并
> 形态 / `ChainedAssignAlgorithms` 三元赋值回读 / `TryWithResourcesAlgorithms` 的 `readLinesGuarded`
> 与 `nestedResources` 两方法) 即永久档案。

当前无已知未修复缺陷; 下一轮以「待扩展覆盖」为主, 继续在真实 `.m2` 语料上探测新长尾。

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
