# 命令行

入口：[cmd/reify/main.go](../cmd/reify/main.go)。命令用于查看注册项、加载元数据、检查头部，以及把 AMD64 代码区域提升为 LowIR。

## 构建

在 Yaklang 工作树根目录执行，例如 `feature-decompile/`：

```bash
OUT=/mnt/data/scan-outputs/reify
mkdir -p "$OUT/bin"
go build -o "$OUT/bin/reify" ./common/yak/reify/cmd/reify
```

## 命令

| 命令 | 参数 | 输出 |
| --- | --- | --- |
| `loaders` | `--json` 可选 | 按 Format 注册的 loader |
| `parsers` | `--json` 可选 | 按 Architecture 注册的 parser 与解码器状态 |
| `inspect` | `<file>`，`--json` 可选 | Artifact、Target、Image、节和诊断 |
| `check` | `<file>`，`--json` 可选 | 头部检查、loader 和 parser 选择 |
| `lift` | `<file>`，`--json` 可选 | AMD64 LowIR |
| `help` | 无 | 用法 |

`lift` 的区域选择：

- 默认解码 `.text` 节。
- `--section NAME` 选择其他节。
- `--address ADDRESS --size SIZE` 解码指定映像地址和字节数。两个参数必须同时给出。

标志可以放在文件名之前或之后。输入文件只读打开，结果写入标准输出。

```bash
"$OUT/bin/reify" loaders
"$OUT/bin/reify" parsers --json
"$OUT/bin/reify" inspect --json /bin/true
"$OUT/bin/reify" check /bin/true
"$OUT/bin/reify" lift --json --address 0x2590 --size 48 /bin/true
"$OUT/bin/reify" lift --section .text /bin/true
```

## loaders 与 parsers

`loaders` 的 JSON 是 `core.LoaderInfo` 数组：

```json
[
  {"kind": "elf", "name": "ELF", "description": "ELF images; the target OS comes from the ELF OS ABI"},
  {"kind": "macho", "name": "Mach-O", "description": "thin Mach-O images; entry points are not recovered"},
  {"kind": "pe", "name": "PE", "description": "PE images; the target OS comes from the PE subsystem"}
]
```

`parsers` 的 JSON 是 `core.ParserInfo` 数组：

```json
[
  {"kind": "386", "name": "386", "available": false},
  {"kind": "amd64", "name": "amd64", "available": true},
  {"kind": "arm", "name": "arm", "available": false}
]
```

AMD64 有解码器；其他架构仍是注册骨架。

## inspect

文本输出包含：

```text
Scope: metadata only; no disassembly or SSA
Input: "/bin/true"
Size: 43320 bytes
Loader: ELF
Format: elf
Target: unknown/amd64, 64-bit, little-endian, abi ELFOSABI_NONE, machine EM_X86_64
Parser: amd64 (available)
Object: ET_DYN
Header entry: 0x2590
SECTION       FILE OFFSET  FILE SIZE  VIRTUAL ADDRESS  MEMORY SIZE
...
```

JSON 结构：

```json
{
  "scope": "metadata-only",
  "artifact": {
    "origin": {"name": "/bin/true", "size": 43320},
    "kind": "native",
    "format": "elf",
    "target": {
      "os": "unknown",
      "architecture": "amd64",
      "bits": 64,
      "byte_order": "little",
      "abi": "ELFOSABI_NONE",
      "machine": "EM_X86_64"
    },
    "image": {
      "object_type": "ET_DYN",
      "entry_point": 9616,
      "sections": []
    }
  },
  "parser": {"kind": "amd64", "name": "amd64", "available": true}
}
```

节数组和空诊断在示例中省略。`Target.OS` 为 unknown 时，表示 ELF OS ABI 没有给出系统信息，不代表宿主系统。

## check

`check` 解析完整头部并报告所选 loader 与 parser，不构建 Artifact。

```json
{
  "scope": "metadata-only",
  "check": {
    "path": "/bin/true",
    "size": 43320,
    "loader": {"kind": "elf", "name": "ELF", "description": "..."},
    "kind": "native",
    "format": "elf",
    "target": {"os": "unknown", "architecture": "amd64", "bits": 64, "byte_order": "little"},
    "object_type": "ET_DYN",
    "sections": 28,
    "parser": {"kind": "amd64", "name": "amd64", "available": true}
  }
}
```

文本输出末行固定为 `No instructions or SSA were checked.`。

## lift

`lift` 默认解码 `.text`。文本输出给出区域、块数、指令数、值数和诊断数：

```text
Scope: amd64 LowIR
Region: address=0x2590 size=48
Blocks: 2
Instructions: 59
Values: 37
Diagnostics: 1
Note [block-falls-off-region]: block at 0x25b6 reaches the end of the decoded region
```

JSON 结构：

```json
{
  "scope": "amd64-lowir",
  "region": {"address": 9616, "offset": 9616, "size": 48},
  "ir": {
    "target": {
      "os": "unknown",
      "architecture": "amd64",
      "bits": 64,
      "byte_order": "little",
      "abi": "ELFOSABI_NONE",
      "machine": "EM_X86_64"
    },
    "blocks": [
      {
        "id": 1,
        "address": 9616,
        "instructions": [
          {
            "kind": "intrinsic",
            "operands": [{"kind": "symbol", "name": "x86.endbr64"}],
            "address": 9616
          }
        ]
      }
    ]
  },
  "diagnostics": [
    {"code": "block-falls-off-region", "message": "block at 0x25b6 reaches the end of the decoded region"}
  ]
}
```

示例只显示第一条指令。实际结果包含完整块和指令。

解码行为：

- 区域按线性顺序解码。遇到无法解码的字节时停止，并返回已经产生的 LowIR。
- 无法解码时产生 `decode-error` 诊断；区域末尾截断也走这条路径。至少解码一条指令时命令返回 0。
- 未支持的指令产生 `unsupported-instruction` 诊断，并在 LowIR 中保留 `x86.<mnemonic>` 内建操作。
- 块到达区域末尾时补 `trap`，并产生 `block-falls-off-region` 诊断。

## 平台与地址解释

- ELF：OS 来自 OS ABI，架构来自机器字段与类别，字节序来自数据编码。
- PE：OS 来自子系统，地址由 ImageBase 加 RVA 得到；固件子系统为 unknown。
- Mach-O：OS 暂为 unknown，入口不恢复，诊断 `entry-point-unavailable`。
- 缺少入口字段时不输出 `entry_point`；这不表示文件没有入口。
- 入口和节地址是映像地址，不是 ASLR 后的运行时地址。
- `lift` 只解释 AMD64。其他架构返回 `ErrDecoderUnavailable`。

## 返回码

| 返回码 | 含义 |
| --- | --- |
| `0` | 帮助、列表、检查、加载或部分解码完成 |
| `1` | 文件、格式、区域、解析、取消或输出错误 |
| `2` | 命令或参数错误 |

`check` 只验证头部和节范围。`lift` 只做静态解码，不执行输入，也不证明文件安全或语义完整。`debug/*` 解析器不是对抗性输入的安全沙箱；不可信文件应在资源隔离环境中处理。

## 开发验证

使用项目指定脚本：

```bash
RUN=/mnt/data/scan-outputs/reify-test
mkdir -p "$RUN/yakit" "$RUN/tmp"
export YAKIT_HOME="$RUN/yakit"
export SSA_DATABASE_RAW="$RUN/ssadb.db"
export TMPDIR="$RUN/tmp"
scripts/ssa-test.sh -count=1 ./common/yak/reify/...
```

测试覆盖 ByteReader、Image/Artifact 校验、区域映射、三个格式 loader、Catalog 注册与分派、parser 目标校验、AMD64 解码与提升，以及 CLI 的 lift 路径。

## 参考资料

- [Go debug/elf](https://pkg.go.dev/debug/elf)
- [Go debug/pe](https://pkg.go.dev/debug/pe)
- [Go debug/macho](https://pkg.go.dev/debug/macho)
- [golang.org/x/arch/x86/x86asm](https://pkg.go.dev/golang.org/x/arch/x86/x86asm)
- [Microsoft PE/COFF 格式](https://learn.microsoft.com/en-us/windows/win32/debug/pe-format)

[返回 README](../README.md)
