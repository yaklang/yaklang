# 语言层

本文描述 `loader/language` 与它引入的 `core` 契约。签名以源码为准。

## 三条分派轴

Rust 与 Go 的产物仍是 ELF、Mach-O 或 PE。容器没有变，变的是容器内部：

| 内容 | C 产物 | Go 产物 | Rust 产物 |
| --- | --- | --- | --- |
| 函数符号 | `.symtab`、`.dynsym` | `.symtab` 与 `.gopclntab` 并存；剥离后只剩 pclntab | `.symtab`，v0 或 legacy 编码 |
| 运行时 | 由 libc 提供 | 自带调度器、GC、栈增长 | 自带 panic/unwind、分配器 |
| 调用约定 | 平台 ABI | ABIInternal 覆盖平台 ABI | 不保证稳定，简单标量接近平台 ABI |
| 字符串 | 指针 + NUL 结尾 | 指针 + 长度，不保证 NUL | 指针 + 长度，UTF-8，不保证 NUL |
| 动态分派 | 函数指针 | 接口调用读 itab | trait 对象调用读 vtable |

`Format` 与 `Architecture` 都不能承载“语言”。语言层是第三条轴，位于容器加载之后、指令解析之前：

```text
path ── ByteReader ── Format ── Loader ── Artifact
                                            │
                                  language.Inspect（语言层）
                                            │  识别编译器
                                            │  恢复符号与可读名
                                            │  给出平台 ABI 的语言覆盖
                                            │  给出字符串表示模型
                                            ▼
                                  asm2lowir ── LowIR ── lowssa ── 伪源码
```

`Loader` 只认识容器，`Parser` 只认识指令集。语言层不解析头部、不解码指令，也不注册新的 `Format` 或 `Architecture`。

每个原生产物都有语言。没有编译器证据时，默认语言是 `c`；它不增加任何覆盖，只表示“使用平台约定”。平台约定来自 `Architecture` 与 `OS` 的 profile：

- 参数寄存器：SysV AMD64 为 `RDI, RSI, RDX, RCX, R8, R9`，Win64 为 `RCX, RDX, R8, R9`，AArch64 为 `X0..X7`。
- 这些是平台约定，不是 C 独有属性。Go 与 Rust 可以覆盖它，覆盖范围按函数记录。

## 插件接口

定义：[core/language.go](../core/language.go)、[loader/language/language.go](../loader/language/language.go)。

```go
type Language interface {
    Kind() core.Language
    Detect(*core.Artifact) *core.LanguageReport
    Recover(*core.Artifact) ([]symbol.Symbol, error)
}
```

检测与恢复属于同一个所有者。拆成两个可独立注册的接口会产生“能识别但不能恢复”的标签，也会产生没有检测依据的恢复器。注册表按 `Kind()` 去重，每种语言一个实现：

```go
var languages = []Language{c.C{}, golang.Golang{}, rust.Rust{}}
```

C 层始终以最低置信度命中，因此 `Inspect` 一定返回一个语言；未知的 kind 也回退到 C 层。置信度排序为 certain > likely > none。

`Inspect` 在 `Catalog.OpenAs` 校验 Artifact 之后调用，位置见 [loader/catalog.go](../loader/catalog.go)。它先选置信度最高的检测结果，再调用该层的 `Recover`：

- Go：`debug/buildinfo` 读版本；按 `.gopclntab`、`<segment>,__gopclntab` 定位函数表，PE 与外部链接场景回退 `.rdata`、`.data.rel.ro`；用 `debug/gosym` 恢复名字、入口和大小。剥离 `.symtab` 后仍可用。
- Rust：识别 v0（`_R`）与 legacy（`_ZN...17h...E`）；`Recover` 把符号表里的编码名改写为可读名，原始名保留在 `Symbol.Mangled`。剥离符号表的 Rust 产物没有名字可恢复，只保留 `.comment` 证据。

`Recover` 返回的符号按 `(名字, 地址)` 合并：

- 同地址、同编码名或同展示名：改写展示名、来源与约定，保留容器给出的精确大小。
- 其他情况：追加。同名不同地址的函数是两个函数体，不合并。

一个 Go 镜像可以链接 Rust 代码。`Inspect` 在检测语言之外仍运行 Rust 命名，给这类符号可读名；这不改变检测出的语言，也不把 Rust ABI 应用到 Go 函数。

## 符号模型

`core/symbol.Symbol`：

```go
type Symbol struct {
    Name       string     // 展示名，例如 main.Add、rust_hello::add
    Mangled    string     // 原始名，例如 _RNvCs4FrXkKnA8Dq_10rust_hello3add
    Address    uint64
    Size       uint64
    Kind       SymbolKind
    Imported   bool
    Library    string
    Source     string     // pclntab、rust-demangle；空表示容器符号表
    Convention Convention // 空表示语言约定；platform 表示保留平台约定
}
```

- `Image.SymbolByName` 同时匹配 `Name` 与 `Mangled`，所以 `--func` 可用任一形式。
- Rust 的 demangle 会省略 crate hash。展示名冲突时写成 `name@0xADDRESS`，不会把两个单态化合并成一个 SSA 函数。
- `Convention` 让 `asm2lowir` 不必认识具体语言：它只检查语言是否提供覆盖，以及该函数是否被标记为 `platform`。

## ABI：平台默认与语言覆盖

默认调用约定属于平台，不属于 C 层。SysV/Win64/AArch64 的参数寄存器、栈方向与 callee-save 集合由 `Architecture` 与 `OS` 的 profile 给出；C 代码和手写汇编都遵循它。C 语言层因此不提供 `ABI`，只提供默认标签和字符串模型。Rust/Go 只覆盖自己恢复出的函数，不改变同一地址空间里的其他函数。

这样分层的原因是：把 ABI 放进 C 层会让非 C 语言的镜像丢失平台约定，也会让汇编函数随检测结果变化。放在架构 profile、由语言按函数覆盖，就不会出现这两种问题。

`LanguageReport.ABI` 是语言对平台约定的覆盖，不是完整模型：

```go
type ABI struct {
    Registers    RegisterModel // 参数与返回寄存器
    NoCalleeSave bool          // 该语言没有普通 callee-save 寄存器
}
```

`schema` 不变的平台字段（栈方向、帧指针、寄存器命名与位宽）继续来自架构 profile。语言只覆盖它确实不同的部分。

`asm2lowir` 把覆盖写成每函数的 `Function.Facts`，条件是：

1. 检测语言提供了 `ABI`（C 默认层没有）；
2. 该地址上有语言恢复出的函数符号（`Source` 非空）；
3. 该符号没有被标记为 `ConventionPlatform`。

Go 插件对 amd64 Go 1.17+、arm64 Go 1.18+ 给出 ABIInternal 覆盖：整数参数寄存器为 `RAX, RBX, RCX, RDI, RSI, R8, R9, R10, R11`（arm64 为 `X0..X15`），返回 `RAX`/`X0`，普通整型无 callee-save 寄存器，帧指针仍按自己的约定恢复。`.abi0` 包装函数标记为 `platform`；cgo 与汇编函数不在 pclntab 中，保持平台约定。

`lowir2ssa` 按函数读取 `FactsFor`，`lowirtext` 用 `function-facts: {json}` 保留该覆盖。同一进程内的 cgo、汇编与 ABI0 函数因此不会被错误地换成 Go 约定。

本文只描述模型。浮点、聚合参数与多返回值未建模：`Registers.Return` 只记录一个整数返回值。

## 字符串模型

`core.Strings` 是语言注册的字符串表示，带解码行为，不只是描述：

```go
type Strings struct {
    Layout       StringLayout   // nul-terminated 或 pointer-length
    Encoding     StringEncoding // bytes 或 utf-8
    ValidateUTF8 bool           // 不合法 UTF-8 是否报错
}
```

两种布局由 `core` 实现，语言只选择并注册：

| 布局 | 值 | 解码 |
| --- | --- | --- |
| `nul-terminated` | 指针 | 从指针读字节，遇到 NUL 停止；上限 4096 字节 |
| `pointer-length` | 指针 + 长度 | 读描述符（按产物字节序与字宽），再读指定长度；长度上限 4096 字节 |

语言注册：

| 语言 | Layout | Encoding | ValidateUTF8 |
| --- | --- | --- | --- |
| C（默认） | `nul-terminated` | `bytes` | 否 |
| Go | `pointer-length` | `utf-8` | 否（Go 允许非法字节） |
| Rust | `pointer-length` | `utf-8` | 是（`&str` 保证 UTF-8） |

`Language` 接口要求实现 `Strings() core.Strings`；`Inspect` 把该值写进 `LanguageReport.Strings`。解码统一调用 `Strings.Decode(artifact, address)`：

- `nul-terminated`：`address` 指向首字节。
- `pointer-length`：`address` 指向描述符；指针与长度按 `Target.Bits` 与 `Target.ByteOrder` 读取。

没有任何调用点按语言名或 kind 分支；两种模式的差别只存在于 `core` 的解码函数里。

命令行入口：

```sh
reify strings --at 0x47c000 bin/hello       # C 字节地址
reify strings --at 0x5200 bin/go_hello      # Go/Rust 描述符地址
reify strings --at greeting bin/hello       # 也接受符号名
```

限制：

- 解码需要一个已知地址。语言层不做数据流分析，不会自动从调用参数反推出字符串描述符。
- 不应用重定位。PIE 产物的描述符指针在文件里常为 0，解码会报 `zero pointer; relocations are not applied`。非 PIE 的 Go/Rust 样例可直接解码；要支持 PIE，需要先在加载阶段应用重定位。

## 包结构

语言层按语言分包，公共机制留在父包：

| 路径 | 职责 |
| --- | --- |
| `loader/language` | `Language` 契约、注册表、`Inspect`、符号合并与消歧 |
| `loader/language/c` | 默认层：C 标签、C 字符串模型、无 ABI 覆盖 |
| `loader/language/golang` | Go 检测、pclntab 恢复、goFuncSize、ABI 与字符串模型 |
| `loader/language/rust` | Rust 检测、v0/legacy 解码器、恢复、字符串模型 |

语言包都只依赖 `core` 与 `core/symbol`，父包按 `Kind()` 组装注册表。Go 的 pclntab 解析使用标准库 `debug/gosym`。

## Rust 解码器

`loader/language/rust` 自带解码器，不引入模块依赖，也不复制第三方实现。入口是 `Demangle`，只接受通过 `IsMangled` 检查的名字。

v0（RFC 2603）支持：

- 路径：crate、实现（`M`/`X`/`Y`）、嵌套路径（`N`，含闭包与 shim 命名空间）、泛型实例（`I`）、反向引用（`B`）。
- 标识符：可选消歧数、长度前缀、punycode 变体；空标识符按 rustc 的规则读作单个 `0`。
- 泛型参数：类型、生命周期（含 `G` binder）、常量。
- 类型：基本类型、数组与切片、元组、引用与裸指针、函数签名（含 `unsafe` 与 ABI）、`dyn` trait 与关联类型绑定。
- 常量：整数、布尔、字符、字符串字面量、引用、数组、元组、结构体值与占位符。
- 符号尾部：实例化 crate 与 `.llvm.<hash>` 后缀。

legacy 支持 `_ZN...17h<hash>E`，包含 `$...$` 转义；hash 必须有 16 个十六进制字符且至少 5 个不同数字，否则视为 C++ 名字。

限制：

- 解码上限：输入 64 KiB、输出 64 KiB、反向引用深度 64、解析步数 1M。
- 任何语法错误都会保留编码名，并在报告里计数；不会猜测。
- 不打印 crate 的 hash 后缀；常量表达式按 rustc 的语法打印，但不保证与 rustc 的括号形式逐字一致。

验证：`rust_hello` 的 583 个 Rust 符号全部解码；可选测试 `TestRealBinaryCorpus` 用 `REIFY_RUST_CORPUS=<elf>` 对任意 Rust 二进制统计解码率。

## 能力边界

已实测（2026-11-18）：

| 场景 | 状态 |
| --- | --- |
| Go 未剥离，ELF / PE / Mach-O | 识别、恢复、lift 通过 |
| Go 剥离（`-s -w`），ELF / PE / Mach-O | 识别、恢复、lift 通过；名字来自 pclntab |
| Rust v0 未剥离 | 识别、demangle、lift 通过；可用展示名或编码名选函数 |
| Rust legacy（`_ZN...17h...E`） | demangle 单元测试通过 |
| Go 接口调用 | 不解析 itab，调用目标保持间接调用 |
| Rust trait 对象调用 | 不解析 vtable，调用目标保持间接调用 |
| Go goroutine、栈增长、defer/recover、GC 写屏障 | 不重建 |
| Go 多返回值、浮点与聚合参数 | 未建模；`Return` 只有 `RAX`/`X0` |
| Go ABI0、cgo、跨 ABI 调用 | 保留平台约定 |
| 剥离的 Rust 产物 | 无法恢复函数名；实测 `rust (likely)`，只有 `.comment` 版本证据 |
| Go 函数精确大小 | pclntab 只给到下一个入口；尾部填充剥离后与代码体一致，超过 64 字节的填充不剥离 |
| 字符串值重建 | 部分实现：`strings --at` 按模型解码；不从参数自动反推地址 |

Go 接口调用的实测形态（`main.total`，`go_interface.go`）：

```text
v122 = *(void * *)((void *)v82) + 0x18;
v126 = *(void * *)(v122)(*(uint64_t *)(v105), arg1, *(void * *)(v122));
```

Rust trait 调用的实测形态（`rust_trait::total`）：

```text
v4 = (void *)arg1 + 0x18;
v8 = *(void * *)(v4)();
```

两者都是寄存器间接调用，LowIR 保留 `call` 指令，但没有可命名的目标。这是已知缺口，不是解析失败。

Go 函数大小实测（`main.Add`，`go_hello.go`，`-gcflags='-N -l'`）：

| 产物 | 符号表/元数据 | 剥离填充后 | `ret` 位置 |
| --- | --- | --- | --- |
| linux/amd64，未剥离 | `.symtab` 39，pclntab 64 | 39 | 0x47db86，长度 39 |
| linux/amd64，`-s -w` | pclntab 64 | 39 | 0x47db86，长度 39 |
| windows/amd64，未剥离 | `go tool nm` 64 | 39 | 0x14007c946，长度 39 |
| darwin/arm64，未剥离 | `go tool nm` 48 | 44 | 0x100074588，长度 44 |

未剥离的 windows 与 darwin 产物中，容器符号表把对齐填充计入大小。剥离填充后与 `RET` 指令位置一致。

## 证据与回归

样例源码在 [test/examples](../test/examples)，编译命令写在文件头与 README。

```sh
export GOCACHE=/mnt/data/cache/go-build GOMODCACHE=/mnt/data/cache/go-mod
export GOTMPDIR=/mnt/data/cache/go-tmp TMPDIR=/mnt/data/cache/go-tmp
RUN=/mnt/data/scan-outputs/rust-go-binaries; mkdir -p "$RUN"

scripts/ssa-test.sh -count=1 -timeout=10m ./common/yak/reify/...
REIFY_TEST_OUTPUT_DIR="$RUN/tests" scripts/ssa-test.sh -count=1 -timeout=10m ./common/yak/reify/test
```

测试覆盖：

| 用例 | 断言 |
| --- | --- |
| `TestGoBinariesIdentifyRecoverAndLift` | linux/amd64 未剥离与剥离、windows/amd64、darwin/arm64；识别、pclntab 恢复、lift、函数级 ABI |
| `TestRustBinaryIdentifyDemangleAndLift` | v0 demangle、展示名与编码名都能选函数、直接调用目标可读 |
| `TestDemangleV0AndLegacy` | v0、legacy、Mach-O 前导下划线、`.llvm` 后缀 |
| `TestV0RealForms` | 空标识符、嵌套闭包与消歧数、`K` 命名空间、闭包作泛型实参 |
| `TestDemangleRejectsNonRustNames` | C++ 与畸形名字被拒绝 |
| `TestMalformedInputDoesNotPanic` | 任意截断输入返回错误，不 panic |
| `TestExpandPunycode` | RFC 3492 示例与非法输入 |
| `TestRealBinaryCorpus` | 可选；`REIFY_RUST_CORPUS` 指向 ELF 时要求 ≥98% 解码率 |
| `TestDetectAndRecover` | Rust 检测、字符串模型、恢复的 Name/Mangled/Source/Size |
| `TestCommentOnlyIsLikelyAndRecoversNothing` | 只有 `.comment` 时为 likely，不伪造名字 |
| `TestVersionAndABI` | Go 版本解析、寄存器模型、无 callee-save、字符串模型 |
| `TestPclntabRejectsUnboundedCount` | 超大函数计数被拒绝 |
| `TestFuncSizeTrimsOnlyAlignmentFill` | 只剥尾部填充；整段填充至少保留 1 字节；未知架构不动 |
| `TestCDefaultAndMalformedMetadata` | 无编译器证据时为 C/none；坏 pclntab 回退 C 并保留失败说明 |
| `TestRustNamesKeepIdentityAndRebuildIndex` | 展示名冲突消歧、原名可查、重复 Inspect 幂等 |
| `TestMergeGoSymbolsKeepsExactSizeAndProvenance` | 容器精确大小保留，来源改写为 pclntab |
| `TestMergeKeepsSameNameAtDifferentAddresses` | 同名不同地址的函数体不被合并 |
| `TestMergeRenamesEncodedSymbolInPlace` | Rust 改名不新增重复符号 |
| `TestLanguageNamesAndFunctionFactsRoundTrip` | 含逗号/箭头/空格的 Rust 名与函数级 ABI 的文本往返 |
| `TestLanguageFactsArePerFunction` | 语言 ABI 不落到 cgo、平台约定或未标记地址 |
| `TestVersionAndABI` / `TestFuncSizeTrimsOnlyAlignmentFill` / `TestPclntabRejectsUnboundedCount` | Go 版本、ABI、填充剥离与坏表拒绝 |

`FuzzPCLNTab` 对 pclntab 头部做模糊测试，要求不 panic。