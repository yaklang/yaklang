# Reify

Reify 是 Yaklang 的程序表示接入框架。它把文件格式、目标平台和指令集分开建模，用统一的 Artifact 连接文件加载、架构解析和低层程序表示。

## 设计轴

| 维度 | 类型 | 实现单位 |
| --- | --- | --- |
| 输入种类 | `core.InputKind` | native、bytecode、source |
| 文件格式 | `core.Format` | 一个 Loader 对应一种格式 |
| 操作系统 | `core.OperatingSystem` | Target 字段，不是分派键 |
| 指令集 | `core.Architecture` | 一个 Parser 对应一种架构 |
| 字节序 | `core.ByteOrder` | ByteReader 的读取模式 |
| 指令操作 | `core.Opcode` | LowIR 指令的 Kind |

Loader 和 Parser 使用两条独立的分派路径。ELF、PE、Mach-O 各自有 loader；AMD64 已有解码器，其余架构仍是注册骨架。操作系统、ABI 和执行模式属于 Target，随请求传给 parser。

```text
path ── os.File ── ByteReader ── Format ── Loader ── Artifact
                                                     │
                                   Reader + Loader + Parser + Target + Image
                                                     │
                                          Region / EntryRegion
                                                     │
                                    Architecture ── Parser
                                                     │
                                                   LowIR
```

## 目录

```text
common/yak/reify/
├── core/                  # 公共契约与共享数据
│   ├── kind.go            # InputKind、Format、OS、Architecture、ByteOrder
│   ├── target.go          # Target
│   ├── reader.go          # ByteReader（按字节序读取，边界受限）
│   ├── image.go           # Image、Section
│   ├── artifact.go        # Artifact、Region、解析入口
│   ├── loader.go          # Loader 接口与 CheckResult
│   ├── parser.go          # Parser 接口与解析结果
│   ├── lowir.go           # 具体 LowIR、Block、Instruction、Value、Operand
│   └── validate.go        # Target、Image、Artifact 校验
├── loader/
│   ├── catalog.go         # 全局 Catalog：注册、检测、打开、检查
│   ├── native/            # ELF、PE、Mach-O 三个 Loader
│   └── arch/              # 按 Architecture 注册的 Parser
│       ├── amd64.go       # AMD64 入口
│       ├── amd64_decode.go
│       ├── amd64_lift.go
│       ├── amd64_regs.go
│       └── 其他架构骨架
├── rewriter/pass.go       # YakSSA 变换接口
├── cmd/reify/             # 命令行入口
└── docs/                  # 架构、接口、LowIR 和命令文档
```

## 快速使用

在 Yaklang 工作树根目录运行，例如 `feature-decompile/`：

```bash
OUT=/mnt/data/scan-outputs/reify
mkdir -p "$OUT/bin"
go build -o "$OUT/bin/reify" ./common/yak/reify/cmd/reify

"$OUT/bin/reify" loaders
"$OUT/bin/reify" parsers
"$OUT/bin/reify" inspect --json /bin/true
"$OUT/bin/reify" check /bin/true
"$OUT/bin/reify" lift --json --address 0x2590 --size 48 /bin/true
"$OUT/bin/reify" lift --json /bin/true
```

`lift` 默认解码 `.text` 节。用 `--address` 和 `--size` 可以限定一个连续代码区域。两个参数必须同时给出。

代码中的调用链：

```go
artifact, err := loader.Default.OpenFile(ctx, path)
if err != nil {
    return err
}
defer artifact.Close()

entry, err := artifact.EntryRegion(48)
if err != nil {
    return err
}
result, err := artifact.Parse(ctx, entry)
```

## 当前能力

- ELF、PE 和 thin Mach-O 的头部、目标平台、入口与节元数据。
- 按 Format 选择 loader，按 Architecture 选择 parser。
- AMD64 解码器：通用寄存器、内存访问、算术与逻辑、比较与条件跳转、调用与返回、栈操作、部分 SSE 移动与按位操作。
- AMD64 LowIR：显式寄存器读写、内存读写、标志寄存器、基本块与控制流目标。
- 命令行只读检查，不执行输入。

## 当前边界

- 除 AMD64 外的架构 parser 只有注册骨架，`Parse` 返回 `ErrDecoderUnavailable`。
- AMD64 使用线性扫描，不依赖符号表恢复函数边界。`.text` 中的数据或填充也可能被当作指令。
- 未支持的 AMD64 指令保留为 `intrinsic`，并产生 `unsupported-instruction` 诊断。它们不会改变已支持的语义，也不会被静默丢弃。
- 解码在区域末尾截断时返回部分 LowIR，并产生 `decode-error` 诊断。
- LowIR 到 YakSSA 的转换尚未实现；`rewriter` 只声明 YakSSA 变换接口。
- Fat Mach-O、Java、Python 与 VM 字节码尚未接入。
- 标准库 `debug/*` 解析器不是对抗性输入沙箱。

## 文档

- [架构](docs/architecture.md)：分派轴、调用链与所有权。
- [接口](docs/interfaces.md)：ByteReader、Loader、Catalog、Artifact、Parser。
- [LowIR](docs/lowir.md)：具体表示、操作约束与效果。
- [命令](docs/cli.md)：命令、JSON、返回码与验证范围。
