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

- 三个 loader：`native.ELF`、`native.PE`、`native.MachO`。
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
  │  Parser.Check + Parse
  ▼
LowIR
```

`Catalog.Open` 借出调用方的 source，`Catalog.OpenFile` 打开文件并把所有权交给 Artifact。两者返回的 Artifact 都记录实际使用的 reader、loader 和 parser。

## 格式层

`loader/native` 为每种格式提供独立类型：

| Loader | Format | 目标平台来源 | 说明 |
| --- | --- | --- | --- |
| `ELF` | `elf` | ELF OS ABI、机器字段、类别 | 支持 32/64 位和大端 |
| `PE` | `pe` | 机器字段、可选头子系统 | PE32 与 PE32+ |
| `MachO` | `macho` | CPU 字段；OS 暂为 unknown | 只接受 thin Mach-O |

格式层只读头部和节表，不反汇编，不执行输入，也不判断文件是否可信。

`Check` 解析完整头部并返回 `core.CheckResult`；`Load` 构建 Artifact。`Match` 只看魔数，用于候选选择。

## 架构层

| 架构 | 状态 | 解码器 |
| --- | --- | --- |
| AMD64 | 已实现 | `golang.org/x/arch/x86/x86asm` |
| 386、ARM、ARM64、RISCV32、RISCV64、MIPS、MIPS64、PPC、PPC64 | 注册骨架 | 未实现 |

### AMD64

`loader/arch/amd64*.go` 把连续的 64 位 x86 机器码转换为 LowIR。解析单位是 `ParseRequest` 给出的一个代码区域，不要求等于整个文件或一个已恢复的函数。

处理步骤：

1. 用 `x86asm.Decode` 逐条解码，显式处理 `ENDBR64` 和 `ENDBR32`。
2. 线性扫描整个区域，记录指令地址和长度。
3. 在直接跳转目标、条件跳转的下一条指令、直接调用目标、控制转移后的下一条指令处切分基本块。
4. 把每条指令提升为 LowIR，直接跳转和条件跳转写入 `Targets`。
5. 为没有控制转移的块补一个到下一块的 `jump`；区域末尾补 `trap` 和 `block-falls-off-region` 诊断。

已覆盖的指令族：

- 数据移动：`MOV`、`MOVZX`、`MOVSX`、`MOVSXD`、`LEA`。
- 算术与逻辑：`ADD`、`SUB`、`AND`、`OR`、`XOR`、`INC`、`DEC`、`NEG`、`NOT`、`IMUL`、`SHL`、`SHR`、`SAR`。
- 比较与条件：`CMP`、`TEST`、`Jcc`、`SETcc`、`CMOVcc`。
- 控制流：`JMP`、`CALL`、`RET`、`LEAVE`、`INT3`、`UD2`、`HLT`。
- 栈：`PUSH`、`POP`、`PUSHFQ`、`POPFQ`。
- 扩展与转换：`CDQE`、`CWDE`、`CDQ`、`CQO`、`CWD`、`XCHG`。
- 向量移动与按位操作：`MOVAPS`、`MOVUPS`、`MOVDQA`、`MOVDQU`、`MOVAPD`、`MOVUPD`、`MOVD`、`MOVQ`、`PXOR`、`XORPS`、`XORPD`、`ANDPS`、`ANDPD`、`ORPS`、`ORPD`。

表示约定：

- 通用寄存器是显式 `read-register` / `write-register`。部分寄存器使用原名称，例如 `al`、`eax`、`rax`。
- `rflags` 是伪寄存器。算术、逻辑、比较和测试指令写入它；`Jcc`、`SETcc` 和 `CMOVcc` 从它读取。`JCXZ`、`JECXZ` 和 `JRCXZ` 例外，它们直接比较 `cx`、`ecx` 或 `rcx`。
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

### 其他架构

`loader/arch` 为其他九种 Architecture 保留独立文件。它们满足：

- `Available() == false`。
- `Check` 校验位数、字节序和执行模式。
- `Parse` 返回 `ErrDecoderUnavailable`，不伪造 LowIR。

实现某个架构时替换该类型的 `Parse` 并让 `Available` 返回 true。接口和注册方式不变。

## Target 与 Image

`core.Target` 描述 OS、Architecture、地址位数、字节序、ABI、机器标识和执行模式。`core.Image` 描述对象类型、基址、入口和节表。二者都是具体结构。

节记录同时保留文件范围和虚拟范围：

- `FileOffset` 相对输入起点。
- `FileSize` 为 0 表示零填充节。
- `VirtualAddress` 是映像地址，不是运行时重定位地址。
- `MemorySize` 可以大于 `FileSize`。

PE 的入口和节地址由 ImageBase 加 RVA 得到；ELF 与 Mach-O 的地址直接来自头部。

## 所有权

| 对象 | 所有者 | 规则 |
| --- | --- | --- |
| `io.ReaderAt` | 调用方或 Artifact | `Open` 借用；`OpenFile` 由 Artifact 关闭 |
| `ByteReader` | Artifact | 单次输入，边界固定 |
| `Loader`、`Parser` | Catalog | 无状态，可复用 |
| `ParseRequest.Code` | 调用方 | Parse 期间只读借用 |
| `ParseResult` | 解析器 | 结果字节必须自行拥有 |
| LowIR | 解析器 | 值、块、效果和顺序不可隐式改写 |

`Artifact.Close` 只关闭带所有权标记的 reader。借用 source 即使实现 `io.Closer`，也不会被关闭。

## 失败边界

- 魔数不匹配：`ErrUnsupportedFormat`。
- 多个 loader 同时匹配：`ErrAmbiguousFormat`。
- 头部损坏或被截断：格式解析错误。
- 架构没有注册 parser：诊断 `parser-unavailable`。
- parser 不接受该 Target：诊断 `parser-incompatible`。
- parser 已注册但没有解码器：诊断 `decoder-unavailable`。
- 地址范围没有文件数据：`ErrRegion`。
- AMD64 区域中出现无法解码的字节：诊断 `decode-error`；已有指令仍返回。
- AMD64 指令没有对应提升规则：诊断 `unsupported-instruction`，并生成 `intrinsic`。
- AMD64 块到达区域末尾：补 `trap`，诊断 `block-falls-off-region`。
- AMD64 诊断最多保留 64 条；后续诊断折叠为一条 `diagnostics-truncated`。

格式加载成功不等于指令解析成功。未知信息和未实现能力都显式保留，不转换成零值或默认语义。

## 参考规范

- [Go debug/elf](https://pkg.go.dev/debug/elf)
- [golang.org/x/arch/x86/x86asm](https://pkg.go.dev/golang.org/x/arch/x86/x86asm)
- [Microsoft PE/COFF 格式](https://learn.microsoft.com/en-us/windows/win32/debug/pe-format)
- [Intel ENDBR32 指令](https://www.felixcloutier.com/x86/endbr32)
- [Intel ENDBR64 指令](https://www.felixcloutier.com/x86/endbr64)
- [LLVM LangRef](https://llvm.org/docs/LangRef.html)

[返回 README](../README.md)
