# 架构

## 分派轴

Reify 不使用“操作系统 × 架构”的目录矩阵。每个维度单独建模、单独分派。

| 维度 | 类型 | 分派方式 |
| --- | --- | --- |
| 输入种类 | `core.InputKind` | Artifact 字段 |
| 文件格式 | `core.Format` | `Catalog` 按 Format 选择 Loader |
| 操作系统 | `core.OperatingSystem` | Target 字段，由格式头部推导 |
| 指令集 | `core.Architecture` | `Catalog` 按 Architecture 选择 Parser |
| 字节序 | `core.ByteOrder` | ByteReader 的读取模式 |
| 指令操作 | `core.Opcode` | LowIR 指令的 Kind |

格式不等于系统。ELF 也描述 BSD 和 Solaris 映像，PE 也描述固件，Mach-O 的 OS 需要平台加载命令才能确定。头部没有给出依据时，OS 保持 `unknown`。

架构也不等于格式。同一个 AMD64 parser 可以处理 ELF 或 PE 中的代码区域；loader 只负责给出 Target 和代码范围。

## 两级注册表

`loader.Default` 是进程级只读 Catalog，构造时显式注册：

- 三个 loader：`fileformat.ELF`、`fileformat.PE`、`fileformat.MachO`。
- 十个 parser：386、amd64、arm、arm64、riscv32、riscv64、mips、mips64、ppc、ppc64。

不使用 `init()` 自动注册，不在运行时修改 `Default`。需要自定义组合时调用 `loader.NewCatalog(loaders, parsers)`；重复 Format 或重复 Architecture 在构造时报错。

按 Kind 直接取实例：

```go
format := core.CheckFormat(prefix)          // 魔数 → Format
l, ok := loader.Default.Loader(format)      // Format → Loader
p, ok := loader.Default.Parser(core.ArchAMD64) // Architecture → Parser
```

ByteReader 属于单次输入，不进入全局表。它由 `path` 或调用方提供的 `io.ReaderAt` 创建，字节序在读取头部后确定。

## 加载与解析调用链

```text
path
  │  os.Stat / os.Open（只接受普通文件）
  ▼
*os.File
  │  core.NewByteReader(name, src, size)
  ▼
ByteReader（边界 + 字节序）
  │  Match(prefix) 选择 Loader
  ▼
Loader.Check / Load
  │  Target、Image、Diagnostics
  ▼
Artifact（Reader + Loader + Parser）
  │  EntryRegion / RegionAt
  ▼
Region（虚拟地址 + 文件偏移 + 长度）
  ├─ Parser.Check + Disassemble → 汇编文本
  ├─ Parser.Check + Parse       → LowIR (Function → Block → Instruction)
  │                                  ├─ 文本 / JSON
  │                                  └─ lowir2ssa → YakSSA → ssa2source → 伪源码
```

`Catalog.Open` 借出调用方的 source，`Catalog.OpenFile` 打开文件并把所有权交给 Artifact。两者返回的 Artifact 都记录实际使用的 reader、loader 和 parser。

## 格式层

`loader/fileformat` 汇总三种格式；每种格式各有一个独立包：

| Loader | Format | 目标平台来源 | 说明 |
| --- | --- | --- | --- |
| `ELF` | `elf` | ELF OS ABI、机器字段、类别 | 支持 32/64 位和大端 |
| `pe.Loader` | `pe` | 机器字段、可选头子系统 | PE32 与 PE32+；导出表、导入表、`.pdata` |
| `macho.Loader` | `macho` | CPU 字段 | thin 与 universal；`LC_MAIN`、`LC_FUNCTION_STARTS`、符号表 |

ELF 额外读取 `.dynsym`、`.symtab`、`.rela.plt` 和 `.rela.dyn`，把 PLT 入口与 GOT 槽映射到导入符号名，并记录定义符号的地址。PE 读取导出表、导入表与 `.pdata` 异常目录。Mach-O 读取 `LC_MAIN`、`LC_FUNCTION_STARTS`、符号表与导入符号，并在 universal 文件中选择一个切片。

COFF 与 MZ 共享同一个前两个字：PE 映像的 DOS stub 在 0x3c 处记录签名偏移，普通 MZ 可执行没有这个偏移。PE loader 只在偏移落在 DOS 头之后时才认领输入，因此三种格式不会互相覆盖。

ar 归档没有单一地址空间。loader 为每个成员分配互不重叠的基址，把成员报告的每个地址平移该基址；文件偏移保持绝对，因为 Artifact 的 Reader 就是归档文件本身。没有成员能被任何 loader 解析时，`Check` 返回错误，而不是给出零位宽 Target。COFF 头没有魔数，靠机器字段与节数判定。

PE 映像的目录项 14 是 COM descriptor，只有 .NET 程序集填写。命中时 loader 拒绝输入：托管映像在原生机器码的位置存放 IL 字节码，按 x86 解码会产出看似合法但无意义的结果。

格式层只读头部、节表和符号表，不反汇编，不执行输入，也不判断文件是否可信。

`Check` 解析完整头部并返回 `core.CheckResult`；`Load` 构建 Artifact。`Match` 只看魔数，用于候选选择。

## 架构层

| 架构 | 状态 | 解码器 |
| --- | --- | --- |
| AMD64 | 已实现 | `golang.org/x/arch/x86/x86asm`（64 位模式） |
| 386 | 已实现 | 同一套 x86 解码器（32 位模式），实现在 `loader/arch/internal/x86` |
| ARM64 | 已实现 | `golang.org/x/arch/arm64/arm64asm`，外加 PAC 编码解码 |
| ARM | 已实现 | `golang.org/x/arch/arm/armasm`，ARM 模式 |
| RISCV32、RISCV64、MIPS、MIPS64、PPC、PPC64 | 注册骨架 | 未实现 |

### AMD64

`loader/arch/amd64/` 是独立 parser 包。它把连续的 64 位 x86 机器码转换为 LowIR，并可选地渲染汇编文本。解析单位是 `ParseRequest` 给出的一个代码区域。当前一个区域产出一个 Function，不要求等于整个文件或一个已恢复的函数。

处理步骤：

1. 用 `x86asm.Decode` 逐条解码，显式处理 `ENDBR64` 和 `ENDBR32`。
2. 线性扫描整个区域，记录指令地址和长度。
3. 在直接跳转目标、条件跳转的下一条指令、直接调用目标、控制转移后的下一条指令处切分基本块。
4. 把每条指令提升为 LowIR，直接跳转和条件跳转写入 `Targets`。
5. 将当前区域封装为一个 Function，Function 内保存基本块。
6. 为没有控制转移的块补一个到下一块的 `jump`；区域末尾补 `trap` 和 `block-falls-off-region` 诊断。

解码器从 `ParseRequest.Reader` 的分段 ByteReader 中增量读取，不接收整段 `[]byte`。`Parse` 与 `Disassemble` 都使用同一套解码器和诊断；前者构建 Function，后者输出地址、机器字节和可切换的 AT&T/Intel 语法助记符。

已覆盖的指令族：

- 数据移动：`MOV`、`MOVZX`、`MOVSX`、`MOVSXD`、`LEA`。
- 算术与逻辑：`ADD`、`SUB`、`AND`、`OR`、`XOR`、`INC`、`DEC`、`NEG`、`NOT`、`IMUL`、`MUL`、`DIV`、`IDIV`、`SHL`、`SHR`、`SAR`。
- 比较与条件：`CMP`、`TEST`、`BT`、`Jcc`、`SETcc`、`CMOVcc`。
- 控制流：`JMP`、`CALL`、`RET`、`LEAVE`、`INT3`、`UD2`、`HLT`。
- 栈：`PUSH`、`POP`、`PUSHFQ`、`POPFQ`。
- 扩展与转换：`CDQE`、`CWDE`、`CDQ`、`CQO`、`CWD`、`XCHG`。
- 向量移动与按位操作：`MOVAPS`、`MOVUPS`、`MOVDQA`、`MOVDQU`、`MOVAPD`、`MOVUPD`、`MOVD`、`MOVQ`、`PXOR`、`XORPS`、`XORPD`、`ANDPS`、`ANDPD`、`ORPS`、`ORPD`、`PUNPCKLQDQ`。

表示约定：

- 通用寄存器在 LowIR 中是显式 `read_register` / `write_register`。部分寄存器使用原名称，例如 `al`、`eax`、`rax`；转换到 YakSSA 时按别名合并为带位宽类型的变量。普通分支汇合沿用变量；循环头插入 loop-carried Phi。
- `rflags` 是伪寄存器。算术、逻辑、比较和测试指令写入它；`Jcc`、`SETcc` 和 `CMOVcc` 从它读取。`JCXZ`、`JECXZ` 和 `JRCXZ` 例外，它们直接比较 `cx`、`ecx` 或 `rcx`。
- 多字节 NOP（`0f 1f`）的 ModRM/SIB 只编码指令长度，CPU 不访问内存。汇编文本统一显示为 `nop`，机器字节保留原始长度，LowIR 是 `nop`。
- 内存地址是 `pointer` 值。基址寄存器先转换为指针，索引和位移作为整数偏移相加。
- RIP 相对地址在解码时计算为 `下一条指令地址 + 位移`。
- 段覆盖保留为 `x86.segment.<name>` 内建操作，不假定段基址为零。
- 未覆盖的指令保留为 `x86.<mnemonic>` 内建操作，并产生 `unsupported-instruction` 诊断。

限制：

- 线性扫描不区分代码和数据。`.text` 中的跳转表或嵌入数据可能被解码成指令。
- 没有符号表、异常表或调用约定恢复。`CALL` 只记录目标，不展开参数和返回值。
- 浮点、AVX 和大多数 SSE 算术尚未建模。它们以 `intrinsic` 和 `unsupported-instruction` 诊断保留。
- `SYSCALL` 和 `SYSENTER` 以 `intrinsic` 保留，不产生 `unsupported-instruction`。`IRET` 系列提升为 `return`。
- `LOCK` 前缀会保留诊断，但原子顺序没有建模。
- 不执行输入，不做动态验证。

### 386 与共享 x86 实现

`loader/arch/internal/x86` 保存 x86 解码、块构造、提升与边界发现的唯一实现。`loader/arch/amd64` 与 `loader/arch/x86` 都只是薄封装：前者传入 `Profile64`，后者传入 `Profile32`。

`Profile` 只描述两种模式的差异：

| 字段 | AMD64 | 386 |
| --- | --- | --- |
| `Mode` | 64 | 32 |
| 默认位宽 | 64 | 32 |
| 栈指针 / 帧指针 | `rsp` / `rbp` | `esp` / `ebp` |
| 指令指针 | `rip`（支持 RIP 相对寻址） | 无 |
| 累加器 | `rax` / `rdx` | `eax` / `edx` |
| 标志伪寄存器 | `rflags` | `eflags` |
| `LOOP` 计数寄存器 | `rcx` | `ecx` |
| 栈槽宽度 | 8 字节 | 4 字节 |
| 指针值位宽 | 64 | 32 |

解码、基本块切分、条件分支、调用与跳转、`x86.flags.*` / `x86.cond.*` 的命名在两种模式下完全一致，因此 LowIR 形状一致，只有名称与位宽不同。

### ARM64

`loader/arch/arm64/` 是第二个独立 parser 包。流程与 AMD64 相同：线性解码、切分基本块、逐条提升、区域封装为一个 Function。区别在解码器和表示约定。

处理步骤：

1. 用 `arm64asm.Decode` 逐个 4 字节字解码；指针认证编码由本包按 ARM ARM 字段布局直接解码，因为 `golang.org/x/arch` 不覆盖 `+pauth`。
2. 记录指令地址；AArch64 指令定长 4 字节，切块只需要地址集合。
3. 在直接跳转目标、条件分支的两条后继、终止指令的下一条指令处切分基本块。
4. 把每条指令提升为 LowIR；寄存器名统一小写，例如 `x0`、`w1`、`sp`。
5. 为没有控制转移的块补 `jump`；区域末尾补 `trap` 与 `block-falls-off-region` 诊断。

已覆盖的指令族：

- 数据移动：`MOV`、`MOVZ`、`MOVN`、`MOVK`、`ADR`、`ADRP`。
- 算术与逻辑：`ADD`/`ADDS`、`SUB`/`SUBS`、`ADC`/`ADCS`、`SBC`/`SBCS`、`AND`/`ANDS`、`BIC`、`ORR`、`ORN`、`EOR`、`EON`、`LSL`/`LSLV`、`LSR`/`LSRV`、`ASR`/`ASRV`、`MUL`、`MADD`、`MSUB`、`SDIV`、`UDIV`、`NEG`、`MVN`。
- 位域：`SBFIZ`、`SBFX`、`UBFIZ`、`UBFX`、`BFI`、`BFXIL`、`SBFM`、`UBFM`、`BFM`。
- 比较与条件：`CMP`、`CMN`、`TST`、`CCMP`、`CCMN`、`CSEL`、`CSINC`、`CSINV`、`CSNEG`、`CSET`、`CSETM`、`CINC`、`CINV`、`CNEG`。
- 控制流：`B`、`B.cond`、`BL`、`BR`、`BLR`、`RET`、`CBZ`、`CBNZ`、`TBZ`、`TBNZ`。
- 内存：`LDR`、`LDRB`、`LDRH`、`LDRSW`、`LDRSB`、`LDRSH`、`LDUR` 系列、`STR`、`STRB`、`STRH`、`STUR` 系列、`LDP`、`STP`；偏移、前索引与后索引三种寻址。
- 指针认证：`PACIA`/`PACIB`/`AUTIA`/`AUTIB` 及 `*Z*` 变体、`RETAA`/`RETAB`、`BRAA`/`BLRAA`。
- 未建模的 SIMD 与浮点：保留为 `arm64.<mnemonic>` 内建操作，带一个按寄存器宽度的结果。

表示约定：

- `nzcv` 是伪寄存器；标志由 `arm64.flags.<op>` 写，条件由 `arm64.cond.<cc>` 读。
- 零寄存器（`wzr`/`xzr`）读取为常量 0；写入被丢弃，不产生 `write_register`。
- 前后索引寻址同时产生访问和基址回写；`stp`/`ldp` 的第二个槽位地址是基址加 8 或 16。
- `udf #0` 与其他 `0x0000????` 字（链接器留下的内嵌数据）提升为 `trap`，操作数是 `arm64.udf<imm>`，不产生 `decode-error`。

限制：

- 线性扫描不区分代码和数据；字面池与跳转表可能被解码，但会落入 `udf` 分支成为 `trap`。
- 不支持 Thumb 或 AArch32；`Check` 拒绝非 64 位或 `Mode` 非 `arm64` 的 Target。
- 浮点与 SIMD 的通道语义尚未建模，只保留寄存器级数据流。
- `golang.org/x/arch` 缺少部分 ARMv8.3+ 编码；本包只补了指针认证一族，其余仍报 `decode-error`。

### ARM

`loader/arch/arm/` 处理 32 位 ARM（ARM 模式），用 `armasm.Decode(..., armasm.ModeARM)` 解码。Thumb 模式没有被 `golang.org/x/arch` 支持，`Check` 对 `Mode == "thumb"` 返回错误，而不是静默按 ARM 解码。

处理步骤与 ARM64 相同：线性解码、按跳转目标与终止指令切块、逐条提升。

已覆盖的指令族：

- 数据移动：`MOV`、`MVN`、`MOVW`、`MOVT`。
- 算术与逻辑：`ADD`/`ADC`/`SUB`/`SBC`/`RSB`/`RSC`、`AND`/`ORR`/`EOR`/`BIC`、`MUL`/`MLA`/`MLS`、`UMULL`/`UMLAL`/`SMULL`/`SMLAL`、`SDIV`/`UDIV`。
- 移位：`LSL`、`LSR`、`ASR`、`ROR`、`RRX`。
- 比较：`CMP`、`CMN`、`TST`、`TEQ`。
- 位域与扩展：`UBFX`、`SBFX`、`BFC`、`BFI`、`UXTB`、`UXTH`、`SXTB`、`SXTH`、`RBIT`。
- 控制流：`B`、`BL`、`BX`、`BLX`、`SVC`、`BKPT`。
- 内存：`LDR`/`LDRB`/`LDRH`/`LDRSB`/`LDRSH`/`LDRD` 与 `STR` 系列，偏移、前索引、后索引三种寻址；`LDM`/`STM` 及别名 `PUSH`/`POP`。

表示约定：

- 寄存器名 `r0`..`r12`、`sp`(r13)、`lr`(r14)、`pc`(r15)；`cpsr` 是伪寄存器。
- 每条指令的低四位是条件码；非 `AL` 的指令读 `arm.cond.<cc>`。条件分支写成 `branch`，两个后继都在 `Targets` 里。
- `bx lr` 提升为 `return`；`bl` 与 `blx` 是 `call`。
- `push`/`pop` 展开为逐个 4 字节存储和加载，并分别调整 `sp`。
- 标志由 `arm.flags.<op>` 写，条件由 `arm.cond.<cc>` 读，二者都由 `lowir2ssa` 识别成比较谓词。
- 未建模的协处理器、NEON 与 VFP 指令保留为 `arm.<mnemonic>` 内建操作，并产生 `unsupported-instruction`。
- 无法解码的字保留为 `trap`（`arm.data`），同时产生 `decode-error`。

限制：

- 只支持 ARM 模式；Thumb 与 Thumb-2 未实现。
- 大端 ARM（BE8/BE32）未验证；`armasm` 固定按小端读取。
- NEON、VFP 与协处理器指令没有建模。
- 字面池在 `.text` 中可能被当成指令，也可能以 `trap` 形式保留。

### 其他架构

`loader/arch` 为其余六种 Architecture 保留单独文件，不放进已实现的四个包。它们满足：

- `Available() == false`。
- `Check` 校验架构、位数和执行模式；字节序由 ByteReader 处理。
- `Parse` 返回 `ErrDecoderUnavailable`，不伪造 LowIR。
- 没有实现 `Disassembler`，所以汇编模式返回 `ErrNoDisassembler`。

- `Available() == true`。
- `Check` 校验架构、位数和执行模式。
- `Parse` 解码为 LowIR；`Disassemble` 输出汇编文本；`DiscoverBoundaries` 参与函数边界发现。
- `internal` 只放被两个以上架构共用的实现：`x86` 服务 amd64、386、8086，`riscv` 服务 32/64，`ppc`、`mips` 同理，`ehframe` 与 `diag` 服务全部架构。只有一种架构使用的包直接放在 `loader/arch/<name>`，与 `arm64` 相同。

S390X 与 LOONG64 各自只有一个变体，因此没有共享内核，实现直接放在各自的架构包里。S390X 的指令长度由前两 bit 决定：`0` 是 2 字节，`3` 是 6 字节，其余是 4 字节，全部大端。反汇编文本手工拼装操作数，因为库的 `String()` 会把位移渲染成带括号的内存操作数。LOONG64 定长 4 字节小端；`LU32I.D`/`LU52I.D` 提升成 mask 加 or，库把分支偏移预先左移 16 位，`branchTarget` 右移还原。

8086 与 386、AMD64 共用 `loader/arch/internal/x86`。区别只在 `Profile`：16 位模式用 `sp`、`bp`、`ax`、`cx` 与 `flags`，并选择 `x86asm` 的 16 位解码模式。

## Target 与 Image

`core.Target` 描述 OS、Architecture、地址位数、字节序、ABI、机器标识和执行模式。`core.Image` 描述对象类型、基址、入口、节表和符号。二者都是具体结构。

节记录同时保留文件范围和虚拟范围：

- `FileOffset` 相对输入起点。
- `FileSize` 为 0 表示零填充节。
- `VirtualAddress` 是映像地址，不是运行时重定位地址。
- `MemorySize` 可以大于 `FileSize`。

PE 的入口和节地址由 ImageBase 加 RVA 得到；ELF 与 Mach-O 的地址直接来自头部。

## 渲染阶段

| 阶段 | 入口 | 输入 | 当前状态 |
| --- | --- | --- | --- |
| 汇编 | `core.Disassembler` | `ParseRequest` | AMD64、386、ARM64 与 ARM 已实现 |
| LowIR | `core/asm2lowir` | `Artifact`、区域 | 已实现；含符号补名 |
| LowIR 文本 | `lowirtext.Format` | `*core.LowIR` | 已实现 |
| YakSSA | `core/lowir2ssa` | `*core.LowIR` | 已实现 |
| YakSSA 文本 | `lowir2ssa.Text` | `*ssa.Program` | 已实现 |
| 伪源码 | `core/ssa2source` | `*ssa.Program` | 已实现 |

`Disassembler` 是 parser 的可选能力，通过 `asm2lowir.Disassemble` 调用；没有该能力的 parser 返回 `ErrNoDisassembler`。

`loader/fileformat` 只解析文件头和符号表，`loader/arch` 负责指令解码、块划分和调用目标发现；`asm2lowir` 用 `BoundaryHints` 把两者串起来，并按函数边界调用 parser，得到普通 LowIR。`lowir2ssa` 先运行可选的 LowIR pass（默认删除 stack canary 和栈帧），再把值、算术、比较和控制流转换成 YakSSA，把通用寄存器和内存槽提升为带位宽类型的变量。只有汇编 `call` 生成 YakSSA `Call`；intrinsic 保留为类型化标记。转换后 `core/ssa2source/pass` 应用 C 函数定义并重写字符串参数。`ssa2source` 只渲染已有 YakSSA 程序。这些包都是独立包，不参与 loader 与 parser 的分派。

详细映射见 [SSA 文档](ssa.md)。

## 后续 source 重写

当前 `ssa2source` 只生成 inspection 用的伪源码，没有 AST。后续如果需要
把结果重写成 Python、Go 等语言，应另加 `rewrite` 层，读取 source/结构，
而不是把语言级重写塞进 `ssa2source`。该层暂未实现。

## 所有权

| 对象 | 所有者 | 规则 |
| --- | --- | --- |
| `io.ReaderAt` | 调用方或 Artifact | `Open` 借用；`OpenFile` 由 Artifact 关闭 |
| `ByteReader` | Artifact | 单次输入，边界固定 |
| `Loader`、`Parser` | Catalog | 无状态，可复用 |
| `ParseRequest.Reader` | Artifact | Parse 期间只读借用，区域有界 |
| `ParseResult` | 解析器 | 结果字节必须自行拥有 |
| LowIR | 解析器 | 函数、块、值、效果和顺序不可隐式改写 |
| `*ssa.Program` | 转换包 | 调用方持有；转换不执行输入 |

`Artifact.Close` 只关闭带所有权标记的 reader。借用 source 即使实现 `io.Closer`，也不会被关闭。

## 失败边界

- 魔数不匹配：`ErrUnsupportedFormat`。
- 多个 loader 同时匹配：`ErrAmbiguousFormat`。
- 头部损坏或被截断：格式解析错误。
- 架构没有注册 parser：诊断 `parser-unavailable`。
- parser 不接受该 Target：诊断 `parser-incompatible`。
- parser 已注册但没有解码器：诊断 `decoder-unavailable`。
- parser 返回的 LowIR 没有 Function：`Parse` 返回错误。
- parser 没有实现 `Disassembler`：`ErrNoDisassembler`。
- LowIR 无法映射的操作：`lowir2ssa-unsupported-op` 诊断，保留为不透明调用。
- YakSSA 块没有终止指令：转换补一个空 `return`。
- 地址范围没有文件数据：`ErrRegion`。
- 区域中出现无法解码的字节：诊断 `decode-error`；已有指令仍返回。
- 指令没有对应提升规则：诊断 `unsupported-instruction`，并生成 `intrinsic`。
- 块到达区域末尾：补 `trap`，诊断 `block-falls-off-region`。
- 诊断最多保留 64 条；后续诊断折叠为一条 `diagnostics-truncated`。

格式加载成功不等于指令解析成功。未知信息和未实现能力都显式保留，不转换成零值或默认语义。

## 参考规范

- [Go debug/elf](https://pkg.go.dev/debug/elf)
- [golang.org/x/arch/x86/x86asm](https://pkg.go.dev/golang.org/x/arch/x86/x86asm)
- [Microsoft PE/COFF 格式](https://learn.microsoft.com/en-us/windows/win32/debug/pe-format)
- [Intel ENDBR32 指令](https://www.felixcloutier.com/x86/endbr32)
- [Intel ENDBR64 指令](https://www.felixcloutier.com/x86/endbr64)
- [LLVM LangRef](https://llvm.org/docs/LangRef.html)

[返回 README](../README.md)
