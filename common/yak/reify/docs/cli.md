# 命令行

入口：[cmd/reify/main.go](../cmd/reify/main.go)。命令用于查看注册项、加载元数据、检查头部，以及按阶段渲染 AMD64 代码区域。

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
| `lift` | `<file>`，`--mode`、`--json` 可选 | AMD64 汇编、LowIR、YakSSA 或伪源码 |
| `help` | 无 | 用法 |

`lift` 的 `--mode`：

| 模式 | 输出 | 状态 |
| --- | --- | --- |
| `1asm` / `asm` | 地址、机器字节和汇编；`--asm-syntax att|intel` | 已实现，默认 |
| `2lowir` / `lowir` | 块、值、操作、目标和效果 | 已实现 |
| `3ssair` / `ssair` | 带类型、对齐 `=` 的 YakSSA 文本 | 已实现 |
| `4source` / `source` | 伪源码 | 已实现 |
| `all` | 同一次解码输出四段，分隔为 `===== 1asm =====` 等 | 已实现 |

`lift` 的转换选项：

- `--keep-stack-canary`：保留 stack-protector 代码。默认删除。只影响 `ssair` 和 `source`。
- `--keep-stack-frame`：保留栈帧建立与销毁。默认删除。删除后不重写 rbp 相对地址。
- `--asm-syntax intel|att`：汇编语法，默认 `intel`；`att` 输出 GNU/AT&T 语法。
- `--func NAME`：反编译一个已恢复函数；不能和 `--address/--size` 同用。
- `--address` 也接受函数符号名（`sum_points` 或 `0xsum_points`）。

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
"$OUT/bin/reify" lift --mode 1asm --address 0x2590 --size 48 /bin/true
"$OUT/bin/reify" lift --mode 2lowir --address 0x2590 --size 48 /bin/true
"$OUT/bin/reify" lift --mode 3ssair --address 0x2590 --size 48 /bin/true
"$OUT/bin/reify" lift --mode 4source --address 0x2590 --size 48 /bin/true
"$OUT/bin/reify" lift --mode 1asm --asm-syntax intel --address 0x2590 --size 48 /bin/true
"$OUT/bin/reify" lift --mode 4source --func main /bin/true
"$OUT/bin/reify" lift --mode all --func main /bin/true
"$OUT/bin/reify" lift --json --mode 2lowir /bin/true
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

### asm

默认模式输出汇编。文本头部给出区域和指令数，随后一行一条指令：

```text
Scope: amd64 assembly
Region: address=0x2590 size=48
Instructions: 14
Diagnostics: 0
0x2590  f3 0f 1e fa               endbr64
0x2594  31 ed                     xor ebp, ebp
0x2596  49 89 d1                  mov r9, rdx
0x2599  5e                        pop rsi
...
```

JSON 结构：

```json
{
  "scope": "amd64-assembly",
  "mode": "asm",
  "region": {"address": 9616, "offset": 9616, "size": 48},
  "assembly": "0x2590  f3 0f 1e fa  endbr64\n...",
  "instructions": 14
}
```

空的 `diagnostics` 字段会省略。汇编由 parser 的可选 `Disassembler` 能力生成。`--asm-syntax att` 输出 GNU/AT&T 语法；没有该能力的架构返回 `ErrNoDisassembler`。

边界恢复按优先级合并：符号表、`.eh_frame`、`init/fini_array`、PLT、入口、直接/ GOT call 目标、尾部扫描。报告头会打印：

```text
Boundaries: symbol=13 eh_frame=8 init_array=2 plt=5 entry=1 call_target=0 scan=0 duplicates=10
```

`3ssair` / `4source` 的每个函数头带 `; boundary: symbol` 或 `// boundary: eh_frame`。

### lowir

`--mode 2lowir` 输出区域统计和 LowIR 文本：

```text
Scope: amd64 LowIR
Region: address=0x2590 size=48
Functions: 1
Blocks: 2
Instructions: 59
Values: 37
Diagnostics: 1
target: unknown/amd64 (64-bit, little-endian)
function f1 @ 0x2590
block b1 @ 0x2590
  0x2590  intrinsic @x86.endbr64
  0x2594  v1:i32 = read_register %ebp [read_register]
  ...
Note [block-falls-off-region]: block at 0x25b6 reaches the end of the decoded region
```

JSON 结构：

```json
{
  "scope": "amd64-lowir",
  "mode": "lowir",
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
    "functions": [
      {
        "id": 1,
        "address": 9616,
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
      }
    ]
  },
  "diagnostics": [
    {"code": "block-falls-off-region", "message": "block at 0x25b6 reaches the end of the decoded region"}
  ]
}
```

示例只显示第一条指令。实际结果按 `functions → blocks → instructions` 组织。

### ssair

`--mode 3ssair` 先构建 LowIR，再调用 `lowir2ssa.Convert`，最后输出 YakSSA 文本：

```text
Scope: amd64 SSair
Region: address=0x401136 size=24
Functions: 1
Diagnostics: 0
main
type: () -> null
entry_0: (<nil condition>)
	<i64>    t4  = <i64> 8
	<i64>    t5  = <i64> t3 sub <i64> 8
	<string> t6  = <string> x86.flags.sub
	<i64>    t7  = type_cast[i64] <string> x86.flags.sub
	<i32>    t8  = <i32> 4202500
	<i64>    t9  = type_cast[i64] <i32> 4202500
	<any>    t12 = call <any> puts (<i64> t9) binding[] member[]
	...
```

JSON 增加 `ssair` 字段，并保留 `ir` 与 `diagnostics`：

```json
{
  "scope": "amd64-ssair",
  "mode": "ssair",
  "region": {"address": 4198710, "offset": 4406, "size": 24},
  "ssair": "main\ntype: () -> null\n...",
  "ir": {"target": {}, "functions": []},
  "diagnostics": []
}
```

### all

`--mode all` 复用一次解码，按固定顺序输出四段；JSON 同时填充 `assembly`、`ir`、`ssair`、`source` 和 `boundary_stats`。

### source

`--mode 4source` 在同一个 YakSSA 程序上调用 `ssa2source.Render`：

```text
Scope: amd64 source
Region: address=0x401136 size=24
Functions: 1
Diagnostics: 0
function main() {
    puts("hello, reify");
    return 0x0;
}
```

JSON 增加 `source` 字段。伪源码不是可编译源码，也不还原语言级类型。控制结构还原为 `if`/`else` 和 `while`；循环 Phi 展开为循环前赋值和 back-edge 前赋值。源码不出现块标签、`goto` 或 `phi`。

数值类型保留位宽：`i8`、`i16`、`i32`、`i64`、`ptr64`、`i1`。部分寄存器由类型区分。普通汇合不插 Phi；循环头插入 loop-carried Phi，再由 source 展开。

源码中的常量默认是十六进制。可打印字符串地址和符号地址会追加注释。函数名和调用目标来自 ELF 符号表、PLT 与 GOT 重定位；没有符号时保留 `sub_0x<地址>`。

转换规则和诊断码见 [SSA 文档](ssa.md)。

## 批量导出

[scripts/lift-all.sh](../scripts/lift-all.sh) 用 `go run` 依次执行四种模式：

```bash
common/yak/reify/scripts/lift-all.sh -o /mnt/data/scan-outputs/reify-lift /bin/true
```

输出根由 `-o` 指定，默认 `./reify-out`。无 `-a/-s` 时写入 `<root>/all`；用 `-a ADDRESS -s SIZE` 只拆一个映像地址区间，例如单个函数，写入指定的 `-o` 目录。用 `-x intel|att` 切换汇编语法，默认 Intel。

| 文件 | 内容 |
| --- | --- |
| `<name>.1asm.txt` | 汇编 |
| `<name>.2lowir.txt` | LowIR |
| `<name>.3ssair.txt` | 带类型 YakSSA |
| `<name>.4source.txt` | 伪源码 |
| `version` | 版本、提交哈希、分支、时间、输入路径、SHA-256 与区间 |

任一模式失败时，脚本把错误写入对应文件，继续其余模式，最后返回 1。

示例二进制：[examples/hello.c](../examples/hello.c)。

```bash
gcc -O1 -fno-pie -no-pie -o /tmp/hello common/yak/reify/examples/hello.c
main_addr=$(nm -S --defined-only /tmp/hello | awk '$4 == "main" {print "0x"$1}')
main_size=$(nm -S --defined-only /tmp/hello | awk '$4 == "main" {print "0x"$2}')

common/yak/reify/scripts/lift-all.sh -o /tmp/hello-all /tmp/hello
common/yak/reify/scripts/lift-all.sh -o /tmp/hello-main -a "$main_addr" -s "$main_size" /tmp/hello
```

### 解码行为

- 区域按线性顺序解码。遇到无法解码的字节时停止，并返回已经产生的汇编或 LowIR。
- `asm` 和 `lowir` 是同一解码与提升管线的两个视图，报告相同诊断。`asm` 不构建 LowIR，但仍会执行提升检查。
- 无法解码时产生 `decode-error` 诊断；区域末尾截断也走这条路径。至少解码一条指令时命令返回 0。
- 未支持的指令产生 `unsupported-instruction` 诊断，并在 LowIR 中保留 `x86.<mnemonic>` 内建操作。
- 块到达区域末尾时补 `trap`，并产生 `block-falls-off-region` 诊断。

## 平台与地址解释

- ELF：OS 来自 OS ABI，架构来自机器字段与类别，字节序来自数据编码。
- PE：OS 来自子系统，地址由 ImageBase 加 RVA 得到；固件子系统为 unknown。
- Mach-O：OS 暂为 unknown，入口不恢复，诊断 `entry-point-unavailable`。
- 缺少入口字段时不输出 `entry_point`；这不表示文件没有入口。
- 入口和节地址是映像地址，不是 ASLR 后的运行时地址。
- `lift` 只解释 AMD64。其他架构的 `lowir` 返回 `ErrDecoderUnavailable`，`asm` 返回 `ErrNoDisassembler`。
- `ssair` 和 `source` 依赖 LowIR，因此也只对已实现解码器的架构有效。

## 返回码

| 返回码 | 含义 |
| --- | --- |
| `0` | 帮助、列表、检查、加载或部分解码完成 |
| `1` | 文件、格式、区域、解析、转换、取消或输出错误 |
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

测试覆盖 ByteReader、Image/Artifact 校验、区域映射、三个格式 loader、Catalog 注册与分派、parser 目标校验、AMD64 解码、汇编渲染与 LowIR 提升，以及 CLI 的 lift 模式。

## 参考资料

- [Go debug/elf](https://pkg.go.dev/debug/elf)
- [Go debug/pe](https://pkg.go.dev/debug/pe)
- [Go debug/macho](https://pkg.go.dev/debug/macho)
- [golang.org/x/arch/x86/x86asm](https://pkg.go.dev/golang.org/x/arch/x86/x86asm)
- [Microsoft PE/COFF 格式](https://learn.microsoft.com/en-us/windows/win32/debug/pe-format)

[返回 README](../README.md)
