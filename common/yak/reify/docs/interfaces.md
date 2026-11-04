# 接口

本文描述 `core`、`loader` 和 `loader/arch` 的公开契约。签名以源码为准。

## ByteReader

定义：[core/reader.go](../core/reader.go)。

```go
reader, err := core.NewByteReader(name, src, size)
ordered, err := reader.WithOrder(core.LittleEndian)
```

ByteReader 是单次输入的字节视图。它限制所有读取不超过声明大小，并为一个字节序提供多字节整数读取。

| 方法 | 说明 |
| --- | --- |
| `Name() string` | 输入名称 |
| `Size() int64` | 声明大小 |
| `Kind() ByteOrder` | 当前读取字节序 |
| `Source() io.ReaderAt` | 借出的底层 source |
| `WithOrder(ByteOrder)` | 返回共享 source 的新视图 |
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
| `Parse(ctx, region)` | 用 artifact 的 parser 解析区域 |
| `Close()` | 释放拥有的 source |

`RegionAt` 要求整个范围落在同一个节的文件数据内。零填充区域、跨节范围和溢出地址都返回 `ErrRegion`。

`Parse` 要求 Artifact 已有 parser。没有 parser 时返回 `ErrNoParser`。调用前会再次执行 `Parser.Check`；解析结果必须包含 LowIR，且 LowIR 的 Target 必须与 Artifact 一致，否则返回错误。

## Image 与 Section

定义：[core/image.go](../core/image.go)。

```go
type Image struct {
    ObjectType string
    Base       uint64
    Entry      *uint64
    Sections   []Section
}
```

`EntryPoint` 的布尔返回值区分“地址为零”和“没有入口信息”。`SectionAt` 返回包含地址的节。

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
- `Check` 校验位数、字节序和执行模式；不匹配时返回错误。
- `Available` 表示 `Parse` 是否具备解码能力。
- `Parse` 接收 Target、首地址和连续代码字节。

`ParseRequest.Code` 是只读借用。`ParseResult` 只包含 `IR` 和 `Diagnostics`；解析器不返回汇编记录。需要保留机器字节时，解析器必须把字节复制进 LowIR 常量或诊断。

## 架构实现状态

定义：[loader/arch](../loader/arch)。

`arch.All()` 返回十个 parser 实例。

`AMD64` 的实现：

- `Available() == true`。
- `Check` 要求 64 位、小端、`ArchAMD64`。
- `Parse` 用 `x86asm` 解码，构建基本块，并输出 LowIR。
- 支持的指令族和限制见[架构文档](architecture.md#amd64)。

其余九个架构仍是骨架：

- `Available() == false`。
- `Check` 校验 Target 的位数、字节序与模式。
- `Parse` 返回 `ErrDecoderUnavailable`。

实现某个架构时替换该类型的 `Parse`，并让 `Available` 返回 true。接口和注册方式不变。

## LowIR

定义：[core/lowir.go](../core/lowir.go)。

LowIR 是具体结构，不是接口。`ParseResult.IR` 是指向 `core.LowIR` 的指针。直接调用 `Parser.Parse` 时，nil 表示解析器没有产出表示；`Artifact.Parse` 要求结果非 nil，且 Target 与 Artifact 一致。

详细约束见 [LowIR 文档](lowir.md)。

## 变换边界

定义：[rewriter/pass.go](../rewriter/pass.go)。

```go
type Pass interface {
    Name() string
    Rewrite(context.Context, *ssa.Program) error
}
```

Pass 面向已有 YakSSA 程序。LowIR 到 YakSSA 的转换尚未实现；转换层需要保留位宽、效果和来源，不能只按操作名称映射。

[返回 README](../README.md)
