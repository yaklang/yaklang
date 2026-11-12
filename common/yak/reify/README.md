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
                                                     ├── Disassemble ── 汇编文本
                                                     │
                                                   LowIR ── function → block → instruction
                                                     │
                                lowir2ssa ── YakSSA ──┴── ssa2source ── 伪源码
```

## 目录

```text
common/yak/reify/
├── core/                  # 公共契约与共享数据
│   ├── kind.go            # InputKind、Format、OS、Architecture、ByteOrder
│   ├── target.go          # Target
│   ├── reader.go          # ByteReader（按字节序读取，边界受限）
│   ├── image.go           # Image、Section
│   ├── symbol.go          # Symbol、SymbolAt
│   ├── artifact.go        # Artifact、Region
│   ├── loader.go          # Loader 接口与 CheckResult
│   ├── parser.go          # Parser、Disassembler 接口与结果
│   ├── lowir.go           # LowIR、Function、Block、Instruction、Value、Operand
│   ├── lowirtext/         # LowIR 文本编解码
│   ├── asm2lowir/         # 机器码 → LowIR；边界与符号编排
│   ├── symbol/            # Symbol + funcdefs/ C 函数签名
│   ├── lowir2ssa/         # LowIR → YakSSA；pass/ 下为 LowIR 变形
│   ├── ssa2source/        # YakSSA → 伪源码；pass/ 下为 SSA pass
│   └── validate.go        # Target、Image、Artifact 校验
├── reify.go               # 统一 API：load、lift、pass options
├── loader/
│   ├── catalog.go         # 全局 Catalog：注册、检测、打开、检查
│   ├── fileformat/        # ELF、PE、Mach-O Loader
│   │   └── elf/           # ELF Loader、符号与 BoundaryHints
│   └── arch/              # 按 Architecture 注册的 Parser
│       ├── arch.go        # 注册表与其他架构骨架
│       └── amd64/         # AMD64 parser、函数边界与解码
│           ├── parser.go
│           ├── decode.go
│           ├── disasm.go
│           ├── lift.go
│           └── regs.go
├── cmd/reify/             # 命令行入口
└── docs/                  # 架构、接口、LowIR、SSA 和命令文档
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
"$OUT/bin/reify" lift --mode asm --address 0x2590 --size 48 /bin/true
"$OUT/bin/reify" lift --mode lowir --address 0x2590 --size 48 /bin/true
"$OUT/bin/reify" lift --mode ssair --address 0x2590 --size 48 /bin/true
"$OUT/bin/reify" lift --mode source --address 0x2590 --size 48 /bin/true
"$OUT/bin/reify" lift --mode asm --asm-syntax intel --address 0x2590 --size 48 /bin/true
"$OUT/bin/reify" lift --mode all --func main /bin/ls
"$OUT/bin/reify" lift --mode source --address 0xmain --size 0x40 /bin/ls
"$OUT/bin/reify" lift --json --mode lowir /bin/true
```

`lift` 默认输出 `asm`。默认恢复并解码全部已发现函数；`--section` 选择单个节，`--func` 选择一个函数，`--address/--size` 选择地址区间。`--mode` 选择 `asm`、`lowir`、`ssair`、`source` 或 `all`。默认使用 Intel 语法；`--asm-syntax att` 切换 GNU/AT&T。用 `--address` 和 `--size` 可以限定一个连续代码区域。两个参数必须同时给出。

`ssair` 先执行 LowIR 转换，再输出 YakSSA 文本。`source` 在 `ssair` 之后输出伪源码。两者都不是可编译源码。

批量导出四种模式：

```bash
common/yak/reify/scripts/lift-all.sh -o /mnt/data/scan-outputs/reify-lift /bin/true
```

脚本按 `1asm`、`2lowir`、`3ssair`、`4source` 顺序生成 `<name>.1asm.txt`、`<name>.2lowir.txt`、`<name>.3ssair.txt`、`<name>.4source.txt` 和 `version`。无 `-a/-s` 时输出到 `<output>/all`。`version` 记录版本、提交哈希和输入摘要。

示例程序在 `test/examples/`：`hello.c` 是最小入口；`linked_list.c` 含结构体、成员访问、循环、条件、函数调用、malloc 和 free；`control_flow.c` 用 `classify` 与 `accumulate` 演示 if/else 与 for。

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
result, err := asm2lowir.Parse(ctx, artifact, asm2lowir.WithRegion(entry), asm2lowir.WithExact())
```

## 当前能力

- ELF、PE 和 thin Mach-O 的头部、目标平台、入口与节元数据。
- ELF 导入导出：`.dynsym`、`.symtab`、`.rela.plt`/`.rela.dyn`，映射 PLT 与 GOT 地址到符号名。
- 调用目标与函数名解析：`puts`、`__libc_start_main`、`main`；无符号时保留 `sub_0x<地址>`。
- C 函数定义表：返回类型、参数类型和可变参数。
- SSA pass：`apply-function-define` 设置调用类型，`infer-types` 传播指针类型并渲染 `NULL`，`redefine-type-value` 把字符串参数换成字符串常量。
- 源码常量默认十六进制；字符串地址和符号地址带注释。
- 按 Format 选择 loader，按 Architecture 选择 parser。
- AMD64 解码器：通用寄存器、内存访问、算术与逻辑、比较与条件跳转、调用与返回、栈操作、部分 SSE 移动与按位操作。
- AMD64 汇编文本：按地址、机器字节输出；`--asm-syntax att|intel` 切换语法。
- AMD64 LowIR：一个代码区域提升为一个 Function，包含基本块、值、操作、目标与效果。
- LowIR → YakSSA：值、算术、比较和控制流进入 YakSSA；通用寄存器和内存槽提升为带位宽类型的变量；只有汇编 `call` 生成 YakSSA `Call`。
- 内置 stack canary 与标准栈帧检测，默认删除；可用 `--lowir-pass` 自定义 pass 列表。
- ELF 函数边界恢复：符号表、`.eh_frame`、`init/fini_array`、PLT、入口、递归下降和尾部扫描；边界来源写入 `ssair`/`source` 函数头。
- YakSSA → 伪源码：重构 `if`/`else` 和 `while`；展开循环 Phi；不输出块标签、`goto` 或 `phi`。
- 命令行只读检查，不执行输入。

## 当前边界

- 除 AMD64 外的架构 parser 只有注册骨架，`Parse` 返回 `ErrDecoderUnavailable`。
- AMD64 使用线性扫描，不依赖符号表恢复函数边界。一个 `ParseRequest` 区域当前产出一个 Function。`.text` 中的数据或填充也可能被当作指令。
- 未支持的 AMD64 指令保留为 `intrinsic`，并产生 `unsupported-instruction` 诊断。它们不会改变已支持的语义，也不会被静默丢弃。
- 解码在区域末尾截断时返回部分 LowIR，并产生 `decode-error` 诊断。
- LowIR 到 YakSSA 的转换在循环头插入 loop-carried Phi；普通分支汇合沿用变量。不做内存 SSA、语言级类型恢复和调用图恢复。
- 一个解析区域可以恢复多个 Function；边界来源和统计由 `asm2lowir.Result` 返回。
- 内部函数调用按被调函数实际读取的寄存器裁剪 arity；已知 C 签名优先。
- 汇编与 LowIR 报告相同诊断；`unsupported-instruction` 是提升阶段的限制，不是解码失败。
- Fat Mach-O、Java、Python 与 VM 字节码尚未接入。
- 标准库 `debug/*` 解析器不是对抗性输入沙箱。

## 文档

- [架构](docs/architecture.md)：分派轴、调用链与所有权。
- [接口](docs/interfaces.md)：ByteReader、Loader、Catalog、Artifact、Parser。
- [LowIR](docs/lowir.md)：具体表示、操作约束与效果。
- [SSA](docs/ssa.md)：LowIR 到 YakSSA、伪源码和限制。
- [命令](docs/cli.md)：命令、JSON、返回码与验证范围。
