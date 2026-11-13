# 接口

本文描述 `core`、`loader` 和 `loader/arch` 的公开契约。签名以源码为准。

## ByteReader

定义：[core/reader.go](../core/reader.go)。

```go
reader, err := core.NewByteReader(name, src, size)
ordered, err := reader.WithOrder(core.LittleEndian)
region, err := ordered.Slice(offset, size)
```

ByteReader 是单次输入的字节视图。它限制所有读取不超过声明大小，并为一个字节序提供多字节整数读取。`Slice` 返回共享 source 的有界视图，视图内的偏移从零开始。

| 方法 | 说明 |
| --- | --- |
| `Name() string` | 输入名称 |
| `Size() int64` | 声明大小 |
| `Kind() ByteOrder` | 当前读取字节序 |
| `Source() io.ReaderAt` | 借出的底层 source |
| `WithOrder(ByteOrder)` | 返回共享 source 的新视图 |
| `Slice(offset, size)` | 返回共享 source 的有界子视图 |
| `WithOwner()` | 标记 source 归 reader 所有 |
| `Close()` | 只关闭带所有权的 source |
| `CheckRange(offset, size)` | 检查范围是否越界或溢出 |
| `ReadAt` | 实现 `io.ReaderAt`，越界返回 EOF |
| `Bytes(offset, size)` | 返回范围副本 |
| `Uint8/Uint16/Uint32/Uint64` | 按当前字节序读取 |

未设置字节序时，多字节读取返回错误。8 位读取不受字节序影响。

## Loader

定义：[core/loader.go](../core/loader.go)。

```go
type Loader interface {
    Kind() Format
    Name() string
    Description() string
    Match(prefix []byte) bool
    Check(*ByteReader) (*CheckResult, error)
    Load(context.Context, *ByteReader) (*Artifact, error)
}
```

- `Kind` 对齐文件格式，是 Catalog 的键。
- `Match` 只检查前缀，不保证头部有效。
- `Check` 解析完整头部，返回 Target、对象类型、节数和诊断。
- `Load` 构建 Artifact，不执行输入，不解析指令。

`CheckResult` 包含 `Kind`、`Format`、`Target`、`ObjectType`、`Sections` 和 `Diagnostics`。

## Catalog

定义：[loader/catalog.go](../loader/catalog.go)。

```go
catalog, err := loader.NewCatalog(loaders, parsers)
loader.Default // 进程级只读实例
```

| 方法 | 说明 |
| --- | --- |
| `Loaders()` | 按 Format 排序的 loader 信息 |
| `Parsers()` | 按 Architecture 排序的 parser 信息 |
| `Loader(Format)` | 按 Format 取实例 |
| `Parser(Architecture)` | 按 Architecture 取实例 |
| `CheckFile(ctx, path)` | 头部检查，不创建 Artifact |
| `Check(ctx, name, src, size)` | 借出 source 的头部检查 |
| `OpenFile(ctx, path)` | 加载文件，Artifact 拥有 source |
| `Open(ctx, name, src, size)` | 加载借出的 source |

`NewCatalog` 拒绝 nil、空 Kind、未知 Kind 和重复 Kind。检测时调用已注册 loader 的 `Match`：零个匹配返回 `ErrUnsupportedFormat`，多个匹配返回 `ErrAmbiguousFormat`。

打开后，Catalog 按 `Target.Architecture` 选择 parser，并把选择结果写入 Artifact。选择失败时 Artifact 仍返回，诊断记录原因。

## Artifact

定义：[core/artifact.go](../core/artifact.go)。

```go
type Artifact struct {
    Origin Origin
    Kind   InputKind
    Format Format
    Target Target
    Image  *Image
    Diagnostics []Diagnostic
    Hints  *BoundaryHints
    Reader *ByteReader
    Loader Loader
    Parser Parser
}
```

`Reader`、`Loader`、`Parser` 不参与 JSON 序列化。Artifact 是“一次加载”的结果，包含从 path 走到 LowIR 所需的实例。

| 方法 | 说明 |
| --- | --- |
| `RegionAt(address, size)` | 虚拟地址 → 文件区域 |
| `EntryRegion(size)` | 从入口点开始的文件区域 |
| `RegionReader(region)` | 为区域建立有界 ByteReader |
| `StringAt(address, max)` | 读取地址处的 C 字符串 |
| `Close()` | 释放拥有的 source |

`RegionAt` 要求整个范围落在同一个节的文件数据内。零填充区域、跨节范围和溢出地址都返回 `ErrRegion`。

`Artifact` 只承载加载结果和区域映射，不执行解析。解析由 `asm2lowir.Parse` 完成：它调用 parser、切分函数并补符号。`Disassembly` 由 `asm2lowir.Disassemble` 调用 parser 的可选能力生成。

## Image 与 Section

定义：[core/image.go](../core/image.go)。

```go
type Image struct {
    ObjectType string
    Base       uint64
    Entry      *uint64
    Sections   []Section
    Symbols    []Symbol
}
```

`EntryPoint` 的布尔返回值区分“地址为零”和“没有入口信息”。`SectionAt` 返回包含地址的节。`SymbolAt` 先匹配精确地址，再匹配带大小的符号范围。`Artifact.StringAt` 读取文件中的 NUL 结尾可打印字符串。

`Symbol` 记录名称、地址、大小、种类、是否导入和来源库。ELF loader 从 `.dynsym`、`.symtab` 和重定位表填充；PLT 与 GOT 地址都映射到导入符号名。PE 与 Mach-O 暂为空。

Section 的 `FileSize == 0` 表示零填充。`Flags` 保留格式原义，不做跨格式归一化。

## Parser

定义：[core/parser.go](../core/parser.go)。

```go
type Parser interface {
    Kind() Architecture
    Name() string
    Available() bool
    Check(Target) error
    Parse(context.Context, ParseRequest) (*ParseResult, error)
}
```

- `Kind` 对齐 Architecture，是 Catalog 的键。
- `Check` 校验架构、位数和执行模式；不匹配时返回错误。字节序由 ByteReader 处理，不在 parser 中判断。
- `Available` 表示 `Parse` 是否具备解码能力。
- `Parse` 接收 Target、首地址和区域 ByteReader。

```go
type ParseRequest struct {
    Target  Target
    Address uint64
    Reader  *ByteReader
}
```

`ParseRequest.Reader` 是只读借用，范围就是当前区域，偏移从零开始。parser 用 `Bytes`、`ReadAt` 或 `Uint*` 读取；不要关闭 reader，也不要从 `Target.ByteOrder` 重新解释字节。`ParseResult` 只包含 `IR` 和 `Diagnostics`；汇编文本通过独立的可选接口返回。需要保留机器字节时，解析器必须把字节复制进 LowIR 常量或诊断。

```go
type Disassembler interface {
    Disassemble(context.Context, ParseRequest) (*Disassembly, error)
}

type Disassembly struct {
    Text         string
    Instructions int
    Diagnostics  []Diagnostic
}
```

`Disassembly.Text` 一行一条指令，格式由架构 parser 决定。AMD64 默认使用 Intel 语法，`DisassembleOptions.Syntax` 可切换 GNU/AT&T。

## 架构实现状态

定义：[loader/arch](../loader/arch)。

`arch.All()` 返回十个 parser 实例。

`AMD64` 的实现位于 [loader/arch/amd64](../loader/arch/amd64)，是独立包：

- `Available() == true`。
- `Check` 要求 64 位和 `ArchAMD64`；不检查 Target 的字节序。
- `Parse` 用 `x86asm` 解码，构建 Function 与基本块，并输出 LowIR。
- `Disassemble` / `DisassembleWithOptions` 复用解码结果，输出地址、机器字节和 AT&T/Intel 汇编。
- 支持的指令族和限制见[架构文档](architecture.md#amd64)。

其余九个架构仍是骨架：

- `Available() == false`。
- `Check` 校验 Target 的架构、位数与执行模式。
- `Parse` 返回 `ErrDecoderUnavailable`。

实现某个架构时替换该类型的 `Parse`，并让 `Available` 返回 true。接口和注册方式不变。

## LowIR

定义：[core/lowir.go](../core/lowir.go)。

LowIR 是具体结构，不是接口。`ParseResult.IR` 是指向 `core.LowIR` 的指针。LowIR 包含 `Functions`；每个 Function 包含 `Blocks`；每个 Block 包含 `Instructions`。直接调用 `Parser.Parse` 时，nil 表示解析器没有产出表示；`asm2lowir.Parse` 要求结果非 nil、至少一个 Function，且 Target 与 Artifact 一致。

详细约束见 [LowIR 文档](lowir.md)。

## SSA 边界

LowIR 的文本形式由 `lowirtext.Format` 提供，不属于 parser 契约。它保留函数、块、值、操作、目标和效果，JSON 仍是机器可读形式。

转换把通用寄存器和内存槽提升为带位宽类型的变量。普通分支汇合沿用变量；循环头插入 loop-carried Phi。只有汇编 `call` 生成 YakSSA `Call`；intrinsic 保留为类型化标记。`lowir2ssa` 默认运行完整 LowIR pass 列表，包含删除 stack canary 和栈帧。完整映射和诊断见 [SSA 文档](ssa.md)。

`rewriter` 包已删除。需要变换时直接对 `*ssa.Program` 编写 Pass。

[返回 README](../README.md)
