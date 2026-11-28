# Reify 逆向引擎评估、缺陷分析与加固测试报告

> **评估基线**：`yaklang` (`origin/main` `5ab65cfdd52daf4f8ece45650b7822b2bfba8e2d`)，Windows 11 x86_64，Go 1.22.12，Clang/LLVM 22 + MinGW GCC；交叉编译覆盖 Windows PE (amd64)、Linux ELF (amd64/arm64)、macOS Mach-O (arm64)。  
> **文档说明**：所有测试样本（几十 KB 到 928 MB 的二进制语料）均保存在工作区外临时评估目录中验证，未随代码提交，以免仓库体积膨胀；本报告总结实测数据、定位到的算法/可扩展性缺陷、已落地的修复与新增单测。

---

## 1. 评估结论摘要

1. **架构完备性**：`common/yak/reify` 已形成清晰的分层管线（`loader/fileformat` → `loader/language` → `loader/arch` → `core/asm2lowir` → `core/lowir2ssa` → `core/ssa2source`），支持 PE / ELF / Mach-O / COFF / MZ / Unix AR 与 13 种架构，并能与 `ssa` / `ssadb` / `SyntaxFlow` 打通。
2. **可扩展性与性能（修前 vs 修后）**：
   - **修前**：存在多处隐藏的 $O(N^2)$ 与重复全节读取问题，导致 45–72 MB DLL（`libclang-cpp.dll`、`libLLVM-22.dll`）、333–528 MB Go 二进制（`reify.exe`、`yak_mitm_perf.exe`）及 1500 块控制流图触发 **60s/90s 超时或长达 51–53s 的卡顿**；全量 Go 程序提升（12 MB `go.exe`、146 MB 剥离 Go PE）在 `ssair`/`source` 阶段普遍超时。
   - **修后**：通过索引化 PE `.pdata` / `core.Image`、引入跨 8 架构共用的 `boundindex` $O(\log N)$ 边界索引、`golang` 节区与可执行段字节缓存、`--func` 有大小符号直达路径、消除 `c.blockOrder` 状态泄漏以及以 Cooper-Harvey-Kennedy 迭代支配树/后支配树替代 $O(N^2)$ 集合迭代，核心场景取得 **14x–100x** 加速：
     - `inspect`：`libclang-cpp.dll` (45.24 MB) 60s 超时 → **0.586s**；`reify.exe` (333.51 MB) 51.376s → **3.688s**；`yak_mitm_perf.exe` (528.49 MB) 60s 超时 → **5.713s**；**928.49 MB** Go PE 压力测试仅需 **8.091s**。
     - 单函数精确提取（`--func`）：`reify.Lift` (333.51 MB Go) 90s 超时 → **3.087s**；`main.main` (528.49 MB) 90s 超时 → **4.515s**；`928.49 MB` PE `--func main.main` **5.042s**；1500 块 `-O0` CFG 函数 `big_cfg_1500` 从 >60s 超时降至全模式 **20.380s**（纯 `ModeSource` `pprof` 实测仅 **629.2ms**）。
     - `go.exe` (12.36 MB) 全量汇编反汇编由 55.795s 降至 **7.335s**，全量伪源码在 90s 内成功完成（**85.551s**，生成 50.23 MB 伪源码）；146.46 MB `reify_stripped.exe` 全量反汇编在 **76.381s** 内完成（产出 802.4 MB 汇编）。
3. **关键算法还原准确性**：针对 C (`-O0` / `-O2` / stripped) 与 Go (PE amd64、ELF amd64/arm64、Mach-O arm64) 的 RC4、XTEA、CRC32、SHA-256 轮函数与二分查找进行了源码级核对，修复了 8 类影响算法语义正确性的关键缺陷（`-O2` 单块自循环被展开并丢失循环、栈数组索引写 `s[i] = byte(i)` 被误判为帧槽并静默丢弃、`x86.rol`/`ror` 与 `div`/`idiv` 及 `arm64.ubfx`/`ubfiz`/`sbfx`/`sbfiz`/`ror` 退化为 `0x0 /* intrinsic */`、Go `sib`/`dib` 8 位寄存器别名缺失、`/* forward declaration */` 插入在函数签名上方等）。
4. **鲁棒性与稳定性**：对 10 类对抗/畸形样本（空文件、4B/64B 截断头、错乱 `e_lfanew` / `e_shoff`、损坏 PE DataDirectory、随机垃圾字节 `.text` 等）进行 `inspect` + `lift --mode source` 模糊验证，**修前修后均无 Go runtime panic**；PE 导出表计数现已使用 `uint64` 与上限保护（`1 << 20`）防止溢出或 OOM。

---

## 2. 多尺度性能评估（修前 vs 修后）

### 2.1 CLI 元数据检查、单函数反编译与全程序提升对比

| 测试项（样本大小） | 修前状态 / 耗时 | 修前峰值内存 | 修后状态 / 耗时 | 修后峰值内存 | 主要优化点 |
| --- | --- | --- | --- | --- | --- |
| `inspect:libclang-cpp.dll` (45.24 MB) | `TIMEOUT(60s)` (60.07s) | 214.5 MB | `OK` **0.586s** | 192.3 MB | PE `.pdata` $O(N)$ 去重 + 区间读取 |
| `inspect:libLLVM-22.dll` (72.53 MB) | `TIMEOUT(60s)` (60.02s) | 314.7 MB | `OK` **0.711s** | 274.5 MB | PE `.pdata` + 符号索引 |
| `inspect:reify_stripped.exe` (146.46 MB) | `OK` 52.597s | 821.5 MB | `OK` **2.183s** | 859.4 MB | 节区缓存 + `goFuncSize` 缓存 |
| `inspect:reify.exe` (333 MB) | `OK` 51.376s | 922.9 MB | `OK` **3.688s** | 860.8 MB | 同上 |
| `inspect:yak_mitm_perf.exe` (528.49 MB) | `TIMEOUT(60s)` (60.06s) | 1852.5 MB | `OK` **5.713s** | 1233.9 MB | PE + Go 元数据路径去重与索引 |
| `inspect:big_928mb_go.exe` (928.49 MB 压力样本) | — | — | `OK` **8.091s** | — | 基于 528 MB PE 扩展至 ~928 MB 验证 |
| `func:big_cfg_O0:big_cfg_500` | `OK` 19.420s | 201.8 MB | `OK` **2.440s** | 179.5 MB | CHK 迭代支配树/后支配树 |
| `func:big_cfg_O0:big_cfg_1500` (`--mode all`) | `TIMEOUT(60s)` (60.06s) | 474.5 MB | `OK` **20.380s** | 442.4 MB | `ModeSource` `pprof` 仅 **629.2ms** |
| `func:reify_stripped:reify.Lift` (146 MB) | `TIMEOUT(90s)` (90.04s) | 811.2 MB | `OK` **1.705s** | 775.4 MB | 非 ARM `--func` 有大小符号直达 |
| `func:reify_full:reify.Lift` (333 MB) | `TIMEOUT(90s)` (90.05s) | 961.9 MB | `OK` **3.087s** | 872.0 MB | 非 ARM `--func` 有大小符号直达 |
| `func:yak_mitm_perf:main.main` (528 MB) | `TIMEOUT(90s)` (90.07s) | 1677.5 MB | `OK` **4.515s** | 1063.8 MB | 非 ARM `--func` + Go 缓存 |
| `func:big_928mb_go:main.main` (928.49 MB) | — | — | `OK` **5.042s** | — | 接近 1 GB 规模下仍稳定 5s 内完成 |
| `whole:go_win_amd64:asm` (1.92 MB) | `OK` 1.936s | 119.7 MB | `OK` **0.884s** | 94.9 MB | `boundindex` + `strings.Builder` |
| `whole:go_win_amd64:ssair` (1.92 MB) | `OK` 11.619s | 1155.3 MB | `OK` **8.646s** | 1191.1 MB | 修复 `blockOrder` 泄漏 + dominator |
| `whole:go_win_amd64:source` (1.92 MB) | `OK` 12.354s | 1028.8 MB | `OK` **7.277s** | 931.1 MB | CHK 支配树 + 标识符一次扫描 |
| `whole:go_exe_12MB:asm` (12.36 MB) | `OK` 55.795s | 574.2 MB | `OK` **7.335s** | 452.8 MB | `boundindex` + `strings.Builder` |
| `whole:go_exe_12MB:source` (12.36 MB) | `TIMEOUT(90s)` (90.47s) | 5563.8 MB | `OK` **85.551s** | 9309.9 MB | 成功输出 50.23 MB 伪源码 |
| `whole:reify_stripped_146MB:asm` (146.46 MB) | `TIMEOUT(90s)` (90.04s) | 822.7 MB | `OK` **76.381s** | 4618.2 MB | 成功输出 802.4 MB 汇编文本 |

> **说明**：146 MB / 12 MB 二进制的全量 `lowir` / `ssair` / `source`（数十万个函数同时构建完整 YakSSA `Program` 与百万级指令对象）受限于内存与 GC（参见 `go_exe_12MB` 峰值 9–14 GB、`reify_stripped_146MB` 超出 20 GB 后 90s 超时），建议对百兆级程序使用 `--func` 或 `--section` / `--address --size` 做按需提升。

### 2.2 CPU / Heap Profile (`pprof`) 热点观察

使用 `reifyprof` 采样分析修后瓶颈：
- **528 MB `yak_mitm_perf.exe` `inspect`**：CPU 4.38s，其中 Go 标准库 `debug/pe.(*File).ImportedSymbols` 累计产生 **7.81 GB（89.69%）** 瞬态堆分配（因标准库内部读取包含导入表的 `.idata`/`.rdata` 节区字节）。自身业务代码在 `.pdata`、`core.Image` 与 `pclntab` 上耗时极低。
- **1,500 块 `-O0` 函数 `big_cfg_1500` (`ModeSource`)**：修前 CPU >185s 卡死在 `ssa2source.(*structurer).computeDominators`；修后整条 `Lift` 链路降至 **629.2ms**（其中 `lowir2ssa` 320ms、`ssa2source.Render` 100ms、其余为 GC 扫描）。
- **全量 `go_algos_win_amd64.exe` (`ModeSource`, 1,626 函数 / 23,938 基本块 / 513,837 指令)**：总计 **6.80s**，主要开销在 `OrderedMap.ForEach` 遍历全程序函数及 `cleanSource` 文本清洗（47.17% / 20.32%）。

---

## 3. 关键算法反编译正确性审计

在 C（Clang `-O0` / `-O2` / stripped，PE amd64）与 Go 1.22.12（PE amd64、ELF amd64/arm64、Mach-O arm64，有符号与 `-s -w` 剥离）上对经典加密/校验/检索算法与真实 Yaklang 函数进行源码还原对比：

| 算法 / 目标函数 | 修前关键缺陷 | 修后验证结果 |
| --- | --- | --- |
| **XTEA (`xtea_encrypt`, `-O2` PE amd64)** | 单块 `do-while` 自循环未识别（`structure.go` 与 `mergephi.go` 显式跳过 `header == tail`），循环体被展开复制一份放进 `if (v154 != 0x0)` 且丢失循环 | 还原为完整 `while (true) { ... v93 = v25 + 0x9e3779b9; ... if (!(v165 != 0x0)) break; }`，32 轮 Feistel 结构与 `sum` / `(v << 4 ^ v >> 5) + v` 更新完整保留 |
| **SHA-256 (`sha256_round_ops`, `-O2` PE amd64)** | `ROR`/`ROL` 指令在 `lowir2ssa` 中成为不透明标记，6 处核心旋转全部输出 `0x0 /* x86.rol */`，`Sigma0` / `Sigma1` 完全丢失 | 全部还原为精确位宽的无符号移位与按位或（如 `(v8 << 0x1a & 0xffffffff) \| (v8 >> 0x6)`），与 `Ch` / `Maj` 共同组成准确轮函数 |
| **Go RC4 (`main.GoRC4Crypt`, PE amd64 & ELF/Mach-O)** | (1) `s[i] = byte(i)` 循环被误判为 `memorySlotFrame` 并丢弃 `OpStore`；(2) `x86.idiv.rem` 输出 `0x0 /* x86.idiv.rem */`；(3) `sib`/`dib` 缺失产生 `v25 = undefined`；(4) `/* forward declaration */` 插在函数头上方；(5) `loopPhi` 对 `current == nil` 生成空 `phi []` | S-box 初始化 `*(uint8_t *)(frame + v80 + 0x18) = (uint8_t)v80` 完整保留；`i % len(key)` 精准还原为 `(int64_t)v115 % (int64_t)arg1`；无 `undefined`；所有前向声明位于 `{` 之后；PRGA 交换与 XOR 输出正确 |
| **Go XTEA / CRC32 (ELF & Mach-O arm64)** | `arm64.ubfx` 未降级，所有位域抽取全部输出 `0x0 /* arm64.ubfx */` | 正确降级为逻辑右移与位掩码（如 `v19 >> 0x5 & 0x7ffffff`、`v30 >> 0x1 & 0x7fffffff`），核心运算全还原 |
| **RC4 (`rc4_crypt`, `-O2` PE amd64) & 二分查找 (`binary_search`)** | `-O2` 紧凑循环同样受自循环缺失与部分展开影响 | KSA、PRGA 双重循环及 `mid = low + (high-low)/2` 控制流准确结构化 |
| **Yaklang 真实函数 (`reify.Lift` 333 MB, `main.main` 528 MB)** | 90s 超时无法产出 | 1.7s–4.5s 内产出完整伪源码，保留全局符号（`os.Stderr`、`os.Args`、`fmt.Fprintln`、`os.Exit`）与分支结构 |

---

## 4. 已发现并修复的核心缺陷清单

### Group 1：可扩展性与性能缺陷

1. **PE 加载器 `.pdata` 二次方扫描与不必要节区分配 (`loader/fileformat/pe/pe.go`)**：
   - `parsePEExceptionTable` 为每个 `.pdata` 条目调用 $O(N)$ `image.SymbolAt`，大型程序出现 $O(N^2)$ 退化；`peReadRVA` 调用 `pe.Section.Data()` 分配整节内容；`parsePEExports` 使用有符号 `int` 与 `uint32` 乘法，存在畸形导出表计数溢出/OOM 风险。
   - **修复**：引入 `uint64` 溢出检查与 `maxExportEntries = 1 << 20` 上限；`peReadRVA` 优先通过 `ByteReader.Bytes` 做精确边界切片读取；`parsePEExceptionTable` 预先构建地址哈希表完成 $O(N)$ 去重。同时改进 `parsePECOFFSymbols` 保留外部/静态数据对象符号（`SymbolObject`）。
2. **符号表与重定位查询线性扫描 (`core/image.go`)**：
   - `SymbolAt`、`SymbolByName`、`InferredSymbolSize`、`RelocationAt` 均为全切片线性扫描。
   - **修复**：新增惰性构建的 `byName`、`byMangled`、带前缀最大结束地址的区间有序数组 `sizedSymbols`/`sizedMaxEnd`、去重地址表 `sortedAddrs` 及 `relocByAddr`，并在 `SetSymbols` 时统一失效重建，将查找降为 $O(1)$ / $O(\log N)$。
3. **8 架构边界扫描器的 $O(N^2)$ 复杂度 (`loader/arch/internal/boundindex` 及 `loader/arch/*/boundaries.go`)**：
   - `x86`/`arm64`/`arm`/`loong64`/`s390x`/`mips`/`ppc`/`riscv` 的 `nextBoundary` 与 `finishRanges` 在几十万符号上触发全表遍历；`x86.scanGap` 传入 `end - address` 造成尾部候选区间互相重叠；`arm.insideExisting` 同样线性扫描全表。
   - **修复**：抽取公共 `boundindex.Index`（批量阈值排序合并 + 二分查找），将 `finishRanges` 改为排序切片邻接差值 $O(N)$ 计算，修复 `scanGap` 与 `arm.insideExisting`。
4. **Go 语言插件重复读取百兆 `.rdata` 与海量函数尾部逐个 `ReadAt` (`loader/language/golang/{golang.go,moduledata.go}`)**：
   - `Recover` 与 `recoverModuleData` 对 PE `.rdata` 各读取一遍数百 MB 节区；`parseGoTable` 对每个函数调用 `goFuncSize` 触发十万次以上系统调用。
   - **修复**：在单次恢复流程内缓存候选数据节字节及可执行段字节，`goFuncSize` 直接切片内存判断对齐填充。
5. **非 ARM `--func` 盲扫全程序与汇编字符串二次方拼接 (`core/asm2lowir/asm2lowir.go`)**：
   - 指定 `--func` 时仍全量扫描所有边界；`Parse`/`Disassemble` 循环中使用 `+=` 拼接文本。
   - **修复**：改用 `strings.Builder`；对于非 ARM（无需扫描 Thumb 奇地址位）且可解析出正尺寸符号的 `--func` 请求直接返回单函数边界。
6. **函数间 `converter` 状态泄漏 (`core/lowir2ssa/lowir2ssa.go`)**：
   - `convertFunction` 未重置 `blockOrder`、`memoryIndex`、`stackStores`、`frameValues`、`spDerived`，多函数提升导致 `blockOrder` 累计增长并产生重复 Phi 边与误分类。
7. **$O(N^2)$ 支配集与后支配集计算 (`core/ssa2source/structure.go`)**：
   - 原实现为每个基本块维护完整可达块 `map[int64]bool` 集合并做不动点集合交集，导致大 CFG（500–1500 块）严重超时；`findLoops` 出口块选择依赖 Go map 随机迭代顺序。
   - **修复**：改用迭代 DFS 逆后序 + Cooper-Harvey-Kennedy 迭代支配树算法，结合支配树欧拉序（`domEntry`/`domExit`）实现 $O(1)$ `dominates` 查询，并在逆 CFG 上构建后支配树 `ipdom`；`findLoops` 按 `s.order` 确定性选定出口块。

### Group 2：算法与语义正确性缺陷

1. **x86 8 位低字节寄存器别名缺失 (`core/frame.go`)**：补充 `"spb"`、`"bpb"`、`"sib"`、`"dib"` 到 `x86ClassicAliases`，消除 Go 二进制常见的 `v25 = undefined`。
2. **栈数组/切片存储误判为标量帧槽 (`core/lowir2ssa/memory.go`)**：将粗粒度的 `spDerived || frameDerived` 检查替换为递归校验 `isFixedFrameAddress`，仅将形如 `[sp/fp ± const]` 的固定标量帧槽标记为 `memorySlotFrame`，使 `[rsp + rdx + 0x18]` 等变址栈数组访问保持 `memorySlotOther` 并保留完整 `OpStore`。
3. **单块自循环与空 Phi 防护 (`mergephi.go`, `lowir2ssa.go`, `structure.go`)**：不再忽略 `target == block.ID` 的自环回边；`loopPhi` 在入口无定义（`current == nil`）时不再发出零边空 `phi []`；循环头读窄寄存器保留 `view.narrow()` 截断；`emitLoop` 为单块自循环生成无重复体 `while (true) { ... if (!(cond)) break; }`。
4. **核心加密与位操作 Intrinsics 降级 (`lowir2ssa/{intrinsics.go,signed.go}`)**：支持 `x86.rol`/`x86.ror`、`arm64.ror`/`arm64.rorv`/`arm64.extr`（双源相等）、`arm64.ubfx`/`ubfiz`/`sbfx`/`sbfiz`，并在校验高位为常数 `0`（`x86.div`）或低位算术右移 `bits-1`（`x86.idiv`，匹配 `CDQ`/`CQO`/`CWD`）的前提下安全降级为无符号/有符号 `OpDiv` / `OpMod`（`orderedBinaryFolds` 补充 `OpMod` 与 `MinInt64 / -1` 防护）；不满足前提时保留不透明 `intrinsic`，确保严谨性。
5. **伪源码清洗缺陷 (`core/ssa2source/ssa2source.go`)**：`valueUsedSeen` 始终保持 `OpStore`/`OpLoad`/`OpSpill` 活跃；修复 `cleanSource` 的 `insertAt` 逻辑以识别 C 签名 `) {`，将前向声明放在函数体内部；`inlineFrameBaseAliases` 使用 `sourceValue` 词边界替换，防止替换 `v1` 时破坏 `v10`/`v11`；`pruneUnusedParameters` 改为单次标识符扫描取代每参数编译正则。
6. **跨平台单测兼容 (`cmd/reify/main_test.go`, `test/examples_test.go`, `test/save_audit_test.go`)**：适配 Windows PE 产物格式校验、`gcc -o` `.exe` 后缀、多格式入口点与符号查询（使原仅在 Linux ELF 运行的 CLI 与 E2E 测试在 Windows 也能完整跑通）。

---

## 5. 鲁棒性与仍存在的已知边界

### 5.1 对抗性畸形输入验证

针对 10 个极端边界输入运行 `reify inspect --json` 与 `reify lift --mode source`，**全部安全处理并产出结构化错误/诊断，无任何崩溃**：
- 截断或非法头样本（`empty.bin`、`short_4b_{elf,macho,mz}.bin`、`short_64b_pe.bin`、`pe_bad_lfanew.exe`、`elf_bad_shoff.elf`、`algos_truncated_half.exe`）：稳定返回 `EOF` / `unsupported input format` / `mz` 越界错误。
- `algos_huge_datadir.exe`（畸形超大导出/DataDirectory）：边界裁剪后正常返回元数据与 87 个函数伪源码，未 OOM。
- `elf_junk_text.elf`（合法 ELF 头 + 随机字节 `.text`）：识别 212 个扫描函数边界并发出 444 条诊断，未 panic。

### 5.2 已知局限（未在本次 PR 范围内修改）

1. **百兆级全程序 SSA 构建内存压力**：对 >100 MB 二进制全程序开启 `lowir`/`ssair`/`source` 会一次性常驻数十万个函数的 IR/SSA 对象（约 20+ GB），建议结合 `--func`、`--section` 或 `--address --size` 使用，或后续将管道改为按函数流式渲染。
2. **Go 运行时与高级抽象**：`runtime.morestack` 序言在部分 Go 函数中仍会呈现为外层循环条件或局部栈检查 `undefined` 变量；接口动态派发、协程调度、多返回值及浮/向量（SSE/NEON）语义仍保持不透明或局部寄存器级建模。
3. **标准库 `debug/pe` 导入表读取开销**：对导入节点与重定位极大的 PE 文件，Go 标准库 `pe.File.ImportedSymbols()` 会产生较多的瞬态字节切片拷贝。
4. **PE `.rdata` 硬上限**：出于防畸形 OOM 保护，`maxNamedPclntabSize` 上限为 **512 MiB**；若极少数静态数据单节 `.rdata` 超过 512 MiB 的 Go PE 二进制，将主动拒绝加载 Go 符号表。

---

## 6. 复现与回归测试验证

```sh
gofmt -l common/yak/reify
go vet -mod=mod ./common/yak/reify/...
go test -mod=mod -count=1 -timeout 20m ./common/yak/reify/...
go build -mod=mod ./common/yak/reify/cmd/reify
```

本次补充的核心回归测试覆盖：
- `core/image_section_test.go`：`TestImageIndexedLookupsAndInvalidation`
- `core/frame_test.go`：`TestAliasOfX86`（补充 `spb`/`bpb`/`sib`/`dib`）
- `loader/fileformat/pe/pe_test.go`：`TestPEOversizedExportCountsRejectedSafely`、`TestPEManyPdataEntriesScaleCleanly`
- `loader/arch/internal/boundindex/index_test.go`：`TestIndexNextAndFlush`
- `loader/arch/internal/x86/boundaries_test.go`：`TestDiscoverBoundariesManySymbolsFastAndNonOverlappingGap`
- `core/lowir2ssa/lowir2ssa_test.go`：`TestConvertResetsConverterStateBetweenFunctions`
- `core/lowir2ssa/intrinsics_test.go`：`TestLowerX86RotateIntrinsics`、`TestLowerX86DivAndIdivSoundness`、`TestLowerARM64BitfieldAndRotate`、`TestIndexedStackStoreNotElidedAndSingleBlockSelfLoop`
- `core/ssa2source/structure_test.go`：`TestStructurerLargeDiamondCFGPerformance`、`TestSingleBlockSelfLoopNoBodyDuplication`、`TestInlineFrameBaseAliasesWordBoundary`、`TestCleanSourceInsertsForwardDeclInsideFunctionBody`
