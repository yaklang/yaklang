# 语言层

本文描述 `loader/language` 与它引入的 `core` 契约。签名以源码为准。

## 为什么是第三层

Rust 与 Go 的产物仍是 ELF、Mach-O 或 PE。容器没有变，变的是容器内部：

| 内容 | C / C++ 产物 | Go 产物 | Rust 产物 |
| --- | --- | --- | --- |
| 函数符号 | `.symtab`、`.dynsym` | `.symtab` 与 `.gopclntab` 并存；剥离后只剩 pclntab | `.symtab`，v0 或 legacy 编码 |
| 运行时 | 由 libc 提供 | 自带调度器、GC、栈增长 | 自带 panic/unwind、分配器 |
| 调用约定 | 平台 ABI | ABIInternal：`RAX, RBX, RCX, RDI, RSI, R8, R9, R10, R11` | 不保证稳定，简单标量接近平台 ABI |
| 异常/展开 | `.eh_frame` / `.pdata` | 每个函数自带栈增长检查 | `.eh_frame` 加 panic 表 |
| 动态分派 | 函数指针 | 接口调用读 itab | trait 对象调用读 vtable |

因此 `Format` 与 `Architecture` 两条分派轴都不适合承载“语言”。语言层是独立的第三条轴：

```text
path ── ByteReader ── Format ── Loader ── Artifact
                                            │
                                  language.Inspect（语言层）
                                            │  识别编译器
                                            │  恢复符号
                                            │  改写可读名
                                            │  给函数挂 ABI 覆盖
                                            ▼
                                  asm2lowir ── LowIR ── lowssa ── 伪源码
```

`Loader` 仍然只认识容器，`Parser` 仍然只认识指令集。语言层不解析头部、不解码指令，也不注册新的 `Format` 或 `Architecture`。

## 接口

定义：[core/language.go](../core/language.go)、[loader/language/language.go](../loader/language/language.go)。

```go
type Detector interface {
    Name() string
    Detect(*core.Artifact) *core.LanguageReport
}

type SymbolRecoverer interface {
    Recover(*core.Artifact) ([]symbol.Symbol, error)
}

func Inspect(*core.Artifact) *core.LanguageReport
```

`Inspect` 在 `Catalog.OpenAs` 校验 Artifact 之后调用，位置见 [loader/catalog.go](../loader/catalog.go)。调用发生在 `Image.SymbolsAt` 建立地址索引之前，因此新增符号不需要重建索引。

检测结果写入 `Artifact.Language`：

```go
type LanguageReport struct {
    Language   Language      // unknown、c、go、rust
    Confidence string        // certain、likely、none
    Runtime    string        // 例如 go1.27.4 或 rustc version 1.97.1
    Evidence   []LanguageEvidence
    Registers  *RegisterModel // 仅当语言 ABI 已建模
    Notes      []string       // 未能恢复的能力
}
```

置信度只有两级。`certain` 表示找到编译器拥有的结构：pclntab、buildinfo、可解码的 Rust 符号。`likely` 表示只有弱证据：`.comment` 里的 rustc 版本。`unknown` 表示没有证据，**不等于 C**。

## 符号模型

`core/symbol.Symbol` 增加两个字段：

```go
type Symbol struct {
    Name     string // 展示名，例如 main.Add、rust_hello::add
    Mangled  string // 原始名，例如 _RNvCs4FrXkKnA8Dq_10rust_hello3add
    Address  uint64
    Size     uint64
    Kind     SymbolKind
    Imported bool
    Library  string
    Source   string // pclntab、rust-demangle；空表示容器符号表
}
```

- `Image.SymbolByName` 同时匹配 `Name` 与 `Mangled`，所以 `--func` 可用任一形式。
- VM 索引与展示名在冲突时加地址后缀。demangle 会省略 crate hash，两个单态化函数可能得到同一展示名；`normalizeRustSymbols` 检测到冲突后写成 `name@0xADDRESS`，不会把两个函数合并成一个 SSA 函数。
- `MergeSymbols` 只在地址上没有命名函数时追加 pclntab 符号。`.symtab` 已给出名字时保留其大小，只补 `Source=pclntab` 作为来源标记。

## 函数级 ABI

`core.Function` 增加一个可选覆盖：

```go
type Function struct {
    // ...
    Facts *ArchFacts `json:"facts,omitempty"`
}

func (ir *LowIR) FactsFor(function Function) ArchFacts
```

`asm2lowir` 只对同时满足以下条件的函数设置 `Facts`：

1. 检测语言是 Go；
2. Go 版本与架构支持寄存器 ABI（amd64 ≥ 1.17，arm64 ≥ 1.18）；
3. 该地址上有 `Source=pclntab` 且不是 `.abi0` 的符号。

同一个进程里并存 cgo、汇编与 ABI0 函数，它们必须继续使用平台 ABI，因此不能用整份 Artifact 的字段覆盖。`lowir2ssa` 按函数读取 `FactsFor`，`lowirtext` 用一行 `function-facts: {json}` 保留该覆盖，缺省时回退到架构默认值。

Go ABIInternal 还取消普通整型 callee-save 寄存器。`Facts.Frame.CalleeSaved` 只保留帧指针，避免把 `RBX` 当作被调用者保存。

## 复用了什么，新写了什么

复用：

| 能力 | 现状 |
| --- | --- |
| ELF / PE / Mach-O 节、入口、`.eh_frame`、`LC_FUNCTION_STARTS`、`.pdata` | 已有 loader，不改 |
| 函数边界发现 | 已有 `BoundaryDiscoverer`；pclntab 符号直接进入符号表即可 |
| 指令解码与 lift | 已有 amd64 / arm64 parser，不改 |
| 调用目标命名 | 已有 `annotateSymbols`，自动使用 demangle 后的名字 |

新写：

| 能力 | 位置 |
| --- | --- |
| 语言识别与证据收集 | `loader/language/language.go` |
| Go pclntab 定位与函数恢复 | `loader/language/go.go` |
| pclntab 尾部对齐填充剥离 | `loader/language/go.go`，`goFuncSize` |
| Go buildinfo 版本 | 标准库 `debug/buildinfo`，无新依赖 |
| Go ABIInternal 寄存器模型 | `loader/language/go.go` |
| Rust 符号识别与 demangle | `loader/language/rust.go`，依赖 `github.com/ianlancetaylor/demangle` |
| 原始名/展示名双字段 | `core/symbol`、`core/image.go` |
| 函数级 ABI 覆盖 | `core/lowir.go`、`core/asm2lowir`、`core/lowir2ssa`、`core/lowirtext` |

pclntab 解析直接使用标准库 `debug/gosym`：`NewLineTable` 识别 Go 1.2 / 1.16 / 1.18 / 1.20+ 四种格式，`NewTable(nil, lineTable)` 从 funcnametab 取名字。新写的部分只有定位与校验：

- 优先按节名 `.gopclntab`、`<segment>,__gopclntab`；
- PE 与外部链接场景回退到 `.rdata`、`.data.rel.ro`，按魔数扫描；
- 校验 `nfunc` 上界、每个 `Entry/End` 落在可执行节内，任一失败即拒绝，不把错误表当作有效结果。

Rust demangle 使用纯 Go 实现，不调用外部 `rustfilt` 或 `c++filt`。依赖在 `go.mod` 中固定版本。

pclntab 只记录下一个函数入口，不记录精确大小。`goFuncSize` 在函数尾部最多回退 64 字节，剥掉 x86 的 `int3`/`nop` 填充与 arm64 的 `nop`/零字填充，得到函数体长度。实测三个平台都与反汇编一致：

## 能力边界

已实测（2026-11-18，命令与输出见下节）：

| 场景 | 状态 |
| --- | --- |
| Go 未剥离，ELF / PE / Mach-O | 识别、恢复、lift 通过 |
| Go 剥离（`-s -w`），ELF / PE / Mach-O | 识别、恢复、lift 通过；名字来自 pclntab |
| Rust v0 未剥离 | 识别、demangle、lift 通过；可直接调用被 demangle 的 `rust_hello::add` |
| Rust legacy（`_ZN...17h...E`） | demangle 单元测试通过 |
| Go 接口调用 | 不解析 itab，调用目标保持间接调用 |
| Rust trait 对象调用 | 不解析 vtable，调用目标保持间接调用 |
| Go goroutine、栈增长、defer/recover、GC 写屏障 | 不重建 |
| Go 多返回值、浮点与聚合参数 | 未建模；`Return` 只有 `RAX`/`X0` |
| Go ABI0、cgo、跨 ABI 调用 | 不建模；这些函数保留平台 ABI |
| 剥离的 Rust 产物 | 无法恢复函数名；实测 `rust (likely)`，只有 `.comment` 版本证据 |
| Go 函数精确大小 | pclntab 只给到下一个入口；尾部填充剥离后与代码体一致，超过 64 字节的填充不剥离 |
| Rust panic/unwind 控制流 | 不重建，异常边不进入 CFG |

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
| `TestRustDemangle` | v0、legacy、Mach-O 前导下划线、非 Rust 名字拒绝 |
| `TestLanguageUnknownAndMalformedMetadata` | 未知输入保持 unknown；截断与超大计数被拒绝 |
| `TestRustNamesKeepIdentityAndRebuildIndex` | 展示名冲突消歧、原名可查、重复 Inspect 幂等 |
| `TestLanguageNamesAndFunctionFactsRoundTrip` | 含逗号/箭头/空格的 Rust 名与函数级 ABI 的文本往返 |
| `TestLanguageFactsArePerFunction` | Go ABI 不落到 C 函数、ABI0 函数或未标记地址 |
| `TestGoFuncSizeTrimsOnlyAlignmentFill` | 只剥尾部填充；整段填充至少保留 1 字节；未知架构不动 |

`FuzzGoPCLNTab` 对 pclntab 头部做模糊测试，要求不 panic。