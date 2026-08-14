# 让 libyak.a 在链接期按脚本自动瘦身

## —— ssa2llvm 的 ELF 拆分与链接期裁剪方案

> 分支：`feature/ssa2llvm/shrik_yaklib_size`（HEAD `6781126d9`）
> 日期：2026-08-13
> 范围：本文描述 yaklang 的 ssa2llvm AOT 场景下，如何让"完整"的 Go c-archive 运行时（`libyak.a`）在每次编译 yak 脚本时，由链接器自动丢弃该脚本用不到的 yaklib 模块代码。

---

## 1. 背景与问题

### 1.1 ssa2llvm 需要"单纯的二进制编译器"

ssa2llvm 的目标是把 yak 脚本编译成原生可执行文件（AOT）。它最终希望成为一个自包含的二进制编译器：

- 编译器内部通过 in-process go-llvm 完成 LLVM IR 生成、目标代码发射（替代 `llc`）和静态链接（替代 `clang`/`ld.lld` 子进程）；
- 运行时是嵌入在编译器二进制里的 `libyak.a`（Go `-buildmode=c-archive`）+ `libgc.a` + crt + 静态 libc；
- 目标产物是"零外部运行时依赖"的静态可执行文件，可以在 `env -i` 下直接运行。

相关实现：

- `common/yak/ssa2llvm/compiler/linker_selfcontained.go`：`newNativeTargetMachine` / `CompileModuleToObjectSC` / `linkStaticWithPatch`；
- `common/yak/ssa2llvm/compiler/compile_dispatch_selfcontained.go`：`prepareAndLinkBinary`。

### 1.2 依赖库必须"完整"

一个语言编译器无法预知用户脚本会用到哪个 yaklib 模块：可能是纯 `print`，可能是 `poc.HTTP`，可能是 `ssa.Parse`。因此 `libyak.a` 必须包含所有模块的完整实现——这是"对任意脚本可编译"的前提。

### 1.3 问题：完整库 × 单脚本产物 = 巨大二进制

Go 的 c-archive 有一个硬约束：整个 runtime 是**一个** Go 包（`package main`）编译出的**一个** `go.o`，所有函数都落在**单一 `.text` section** 里。于是：

- 链接器经典的 `--gc-sections` 只能按 section 丢弃，而这里只有一个巨型 `.text`，等于"要么全留、要么全丢"；
- `-ffunction-sections` 对 Go 代码不生效（Go 编译器自己产出汇编，不受该 C 编译选项控制）；
- `strip -s` 只能去掉 ELF symtab/DWARF，Go 运行时依赖的 pclntab、typelink、moduledata、textsectmap 都在 rodata 里，不能删。

结果：一个只 `print("hello")` 的脚本，产物也会携带 poc / cli / http / ssa 等全部模块的机器码。

**需要解决的核心问题**：

> 让 `libyak.a` 保持"完整"，但在每次链接时，根据脚本实际用到的模块，把未用模块的**真实代码**从最终产物里去掉，同时不破坏 Go 运行时的函数查找（findfunc / traceback / pprof / panic 栈）。

---

## 2. 为什么不能直接"链接时去符号"

第一直觉是"链接时删除不需要的符号"，但这条路走不通：

1. **符号 ≠ 代码块**。Go c-archive 的 `.text` 里函数之间互相引用、`inittask` 数组引用全部初始化函数、pclntab 的 ftab 记录每个函数的物理偏移。删一个符号名，代码还在；删代码，所有引用它的元数据必须同步修。
2. **模块 ≠ Go 包**。yaklib 的"模块"是语言层面的概念，不是 Go 包边界：
   - `ssa` 模块由 `ssaapi` + `ssaproject` + `ssaapi/ssaconfig` 多个 Go 包合成；
   - `yakit` 模块由 `ssa2llvm/runtime/shim` 实现；
   - 一个 Go 包（如 `common/yak/yaklib`）可能同时是多个模块的导出来源。
   所以不能按 Go 包粒度裁剪。
3. **Go runtime 元数据与物理地址强绑定**。pclntab 的 ftab 按函数偏移排序、`moduledata.textsectmap` 把虚拟地址映射回文件偏移。只要移动过代码，这些表就必须重写，否则 panic 打印栈、pprof、`runtime.Callers` 全部崩溃。
4. **init 任务不能"假装没有"**。Go runtime 启动时会顺序执行 `..inittask` 数组里的所有初始化任务。模块代码被删了，但任务记录还在，必须显式标记为已完成，否则 runtime 会跳到被删除的代码上。

结论：需要"ELF 级重写工具 + 链接期补丁 + Go runtime 元数据修复"三件套，而不是简单的符号过滤。

---

## 3. 总体设计

方案分两个阶段，中间以"模块化的 `libyak.a`"为分界线：

```text
构建期（一次性，build_yaklib.sh）
  runtime_imports_generated.go（per-module 注册导出）
        │  go build -buildmode=c-archive
        ▼
  libyak.a（完整，但 go.o 的 .text 是"一整块"）
        │  elfsplit：解析符号 → 按模块归类函数 → 拆 .text
        ▼
  libyak.a（完整，但 .text 被拆成 .text + .modtext.<m>*）

链接期（每次编译脚本）
  compiler 收集脚本实际使用的 yaklib 模块（YaklibDependencies）
        │  加上依赖闭包：shared / poc→cli / ssa→ssafront
        ▼
  patch.Patch（对 release 出来的 libyak.a 副本）
        │  把"未用模块"的重定位指到 stub、inittask 标记 done、
        │  清掉 textmap 里的模块引用
        ▼
  lld --gc-sections --icf=safe -s
        │  未用模块的 .modtext.<m> 整节被丢弃
        ▼
  SortFinalTextMap + MissingRetainedModules 收敛
        ▼
  脚本专属的静态可执行文件
```

核心文件：

| 阶段 | 文件 | 职责 |
|---|---|---|
| 构建期 | `common/yak/ssa2llvm/scripts/build_yaklib.sh` | 生成 imports → 编 c-archive → elfsplit → 收集 C 依赖 → 打包 assets |
| 构建期 | `common/yak/ssa2llvm/runtime/embed/permodule_runtime.go` | 生成按模块导出的 `runtime_imports_generated.go` |
| 构建期 | `common/yak/ssa2llvm/runtime/embed/module_registry.go` | 模块 → Go 包/导出表/轻量 AOT shim 的静态注册表 |
| 构建期 | `common/yak/ssa2llvm/cmd/elfsplit/main.go` | ELF 重写：把 `go.o` 的 `.text` 拆成 `.text` + `.modtext.<m>` |
| 链接期 | `common/yak/ssa2llvm/runtime/patch/elf_linux.go` | 重定位中和、inittask 标记、textmap 清理 |
| 链接期 | `common/yak/ssa2llvm/runtime/patch/final_textmap_linux.go` | 链接后修复 textsectmap/ftab/minpc/maxpc，检测残留模块 |
| 链接期 | `common/yak/ssa2llvm/compiler/linker_selfcontained.go` | patch → lld → 元数据修复 → 收敛重链 |

---

## 4. 构建期：调整 yaklib

### 4.1 模块注册从"全部 init"改为"按需注册"

原先 `runtime_imports_generated.go` 在 `init()` 里把全部模块注册进 runtime。改成 per-module 模式后（`WriteRuntimeImportsPerModule`，见 `runtime/embed/permodule_runtime.go`）：

```go
// 每个模块一个 C 导出函数，编译期只调用脚本实际使用的模块
//export yak_register_module_ssa
func yak_register_module_ssa() {
	runtimeRegisterYaklibModule("ssa", ssaapi.Exports)
	runtimeRegisterYaklibModule("ssa", ssaproject.Exports)
	runtimeRegisterYaklibModule("ssa", ssaconfig.Exports)
}

//export yakUnusedModuleStub
//go:noinline
func yakUnusedModuleStub() {}
```

要点：

- `init()` 为空：模块仍通过 import 触发 Go 的 `..inittask`，但**注册**推迟到编译期生成的调用；
- 未用模块的注册函数无人调用 → 在 lld GC 时整节丢弃；
- `yakUnusedModuleStub` 是给"被删除模块的悬空引用"准备的 no-op 目标（见 §6.2）。

### 4.2 轻量 AOT 导出表，避免 monolithic yaklib 被拖入

如果 `codec`、`os` 这类模块的导出表直接引用 `common/yak/yaklib` 包，那么编译 `libyak.a` 时整个 yaklang 前端栈（goja、ssaapi、各种语言前端……）都会被拖进 archive。

为此 `module_registry.go` 给模块增加了 `PrunedShim`：

```go
"codec": {
	ModuleName:   "codec",
	GoImportPath: "github.com/yaklang/yaklang/common/yak/yaklib",
	ImportAlias:  "yaklib",
	ExportExpr:   "yaklib.CodecExports",
	// AOT build only: lightweight table, keeps the monolithic yaklib out.
	PrunedShim: &ExportSource{
		GoImportPath: "github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/aotlib",
		ImportAlias:  "aotlib",
		ExportExpr:   "aotlib.CodecExports",
	},
},
```

`runtime/aotlib/codec.go` 手工维护一份轻量导出表，只依赖 `common/yak/yaklib/codec` 子包：

```go
var CodecExports = map[string]any{
	"EncodeBase64": codec.EncodeBase64,
	"DecodeBase64": codec.DecodeBase64,
	"Sha256":       codec.Sha256,
	// ...
}
```

同时 AOT 全局注册走 `runtime_globals_aot.go`（build tag `ssa2llvm_aot`），不 import `common/yak/yaklib` 与 yaklang builtin 包；非 AOT 构建保留原 `runtime_globals_full.go`。这样 archive 本身是"完整模块集、但无 monolith 包袱"的。

### 4.3 构建流水线（build_yaklib.sh）

```bash
# 1. 生成 per-module imports（aot 模式）
go run ./common/yak/ssa2llvm/runtime/embed/genfull --permodule --aot "${GENFULL_OUT}" ${MODULES//,/ }

# 2. 编译 c-archive。ffunction-sections 对 Go 无效，但 elfsplit 会自己做拆分
CGO_CFLAGS="-ffunction-sections" CGO_ENABLED=1 GOWORK=off \
  go build -tags ssa2llvm_aot -trimpath -ldflags="-s -w" \
  -buildmode=c-archive -o "${RUNTIME_DIR}/libyak.a" .

# 3. 提取 go.o → elfsplit 拆分 → 替换回 archive
llvm-ar x "${RUNTIME_DIR}/libyak.a" --output="${SPLIT_DIR}" go.o
elfsplit "${SPLIT_DIR}/go.o" "${SPLIT_DIR}/go_split.o" "${AOT_SPLIT_MODULES}"
llvm-ar x "${RUNTIME_DIR}/libyak.a" --output="${REPACK_DIR}"
cp "${SPLIT_DIR}/go_split.o" "${REPACK_DIR}/go.o"
llvm-ar rcs "${RUNTIME_DIR}/libyak.a" $(ls *.o)

# 4. 收集 cgo C 静态依赖（libpcap/libm/...）→ embed/extdeps
# 5. 收集 libgc/crt/libc/libgcc → embed assets
# 6. 生成 manifest_generated.go（每个文件的 SHA256，作为缓存 key）
```

默认模块集 `os,poc,cli,http,codec,yakit,ssa`，再加上拆分组 `shared` 与条件组 `ssafront`（仅当包含 `ssa` 时）。`shared` 是 poc/ssa 共享的依赖闭包（schema/lowhttp/net-http/gorm 等），`ssafront` 是 ssa 的语言前端。

---

## 5. 构建期核心：elfsplit —— 把"一整块 .text"拆成"模块节"

### 5.1 为什么必须自己拆 ELF

`go build -buildmode=c-archive` 产出的 `go.o` 是普通 ELF 可重定位文件，但整个 runtime 代码在一个 `.text` section 里，符号表里每个函数有名字、偏移和大小。要做链接期 DCE，就得把这些函数按"yaklib 模块"重新分组，搬进独立的 `.modtext.<module>` section：

- `.text`：基础 runtime（Go runtime + ssa2llvm AOT runtime 本身），任何脚本都保留；
- `.modtext.os` / `.modtext.poc` / `.modtext.ssa` / …：对应模块的机器码，链接期按需保留；
- `.zz_yak_text_end`：1 字节可执行哨兵，用来让 `runtime.etext` 落在所有文本末尾。

`cmd/elfsplit/main.go`（约 1200 行）就是这样一个 ELF 重写工具：输入 `go.o`、输出 `go_split.o`、参数是模块名列表。

### 5.2 模块归类：符号名 → 模块

把 c-archive 里每个 `main.<pkg>.<func>` 形式的目标函数，按包路径前缀映射到模块。Go 的包路径 `github.com/yaklang/yaklang/common/utils/lowhttp/poc` 与模块名 `poc` 的对应关系就是 `module_registry.go` 里那张注册表。

拆分的核心数据结构（`modFunc` / `codePlacement`）：

```go
type modFunc struct {
	symIdx int    // 符号表下标
	name   string
	oldOff uint64 // 原 .text 内偏移
	size   uint64
	newOff uint64 // 新 section 内偏移
	module string // 所属模块；"" 表示基础 runtime
}
```

### 5.3 代码搬迁

每个模块的代码被搬进自己的 `.modtext.<m>`，基础 runtime 保留在压缩后的 `.text` 里。注意这里不是"删除"——`libyak.a` 仍然完整，只是代码从"一个连续段"变成"多个可独立 GC 的段"。

```go
modDataByModule := make(map[string][]byte)
for _, mod := range modules {
	ranges := modGroups[mod]
	if len(ranges) == 0 {
		continue
	}
	var totalSize uint64
	for _, r := range ranges {
		totalSize = maxUint64(totalSize, r.newOff+r.size)
	}
	modData := make([]byte, totalSize)
	for _, r := range ranges {
		copy(modData[r.newOff:r.newOff+r.size],
			data[textOff+r.oldOff:textOff+r.oldOff+r.size])
	}
	modDataByModule[mod] = modData
}
```

### 5.4 重定位搬家

原 `.rela.text` 里的每条 RELA 记录，其 `r_offset` 指向 `.text` 内某个位置。代码搬走后，重定位必须跟着搬到对应的新 section，否则 lld 会把重定位错误地作用到别的字节上：

```go
source := findPlacement(sourceRanges, rOffset)
if source == nil {
	fmt.Fprintf(os.Stderr, "elfsplit: text relocation at %#x is outside retained code\n", rOffset)
	os.Exit(1)
}
entry := make([]byte, elf64RelaSize)
binary.LittleEndian.PutUint64(entry[0:], source.newOff+(rOffset-source.oldOff))
copy(entry[8:], relaData[base+8:base+16])
copy(entry[16:], relaData[base+16:base+24])
section := ".text"
if source.module != "" {
	section = ".modtext." + source.module
}
relocsBySection[section] = append(relocsBySection[section], entry...)
```

### 5.5 直接分支重写（PC-relative）

Go 编译出的代码里，函数之间的直接跳转/调用在汇编里是相对位移，**没有**对应的 ELF 重定位。代码搬走后，同一模块内的相对分支必须重新计算位移；跨模块分支则生成一条合成 RELA，交给 lld 解析。

`rewritePCRelativeBranches` 用 `golang.org/x/arch/x86/x86asm` 逐条解码每个函数：

- 识别 `x86asm.Rel`（直接分支）和 `RIP-relative Mem` 操作数（Go 用 `leaq sym(%rip)` 取函数地址，open-coded defer 也依赖它）；
- 计算 `oldTarget = fn.oldOff + pos + inst.Len + disp`；
- 同模块：直接写新位移 `newTarget - newPC`；
- 跨模块：生成合成 RELA（`R_X86_64_PC32`），由 lld 在最终布局确定后解析；
- 已有 ELF 重定位的位置跳过（交给 lld，避免双重修正）。

关键点：函数内部的 label（无独立符号）也按"所属函数整体搬迁"处理——只要源和目标都在同一输出 section，相对位移不变。

### 5.6 textsectmap：让 Go runtime 认识新布局

Go 的 pclntab 用 `moduledata.textsectmap` 把 PC 映射回物理地址。elfsplit 为每个保留代码块生成一条记录：

```go
type textMapEntry struct {
	vaddr  uint64 // 原 .text 内偏移（虚拟地址）
	end    uint64
	symIdx int    // 指向最终所在 section 的符号（.text 或函数自身）
	addend int64
}
```

编码成 `.data.rel.ro.yaktextmap` section + `.rela.data.rel.ro.yaktextmap` 重定位：

- 基础 runtime 的每个连续块：记录 `[oldOff, oldOff+size)`，重定位指向 `.text` 符号、addend 为块在压缩后 `.text` 里的偏移；
- 每个模块函数：记录 `[fn.oldOff, fn.oldOff+size)`，重定位指向函数自身符号（addend 0），因为 `writeSymbolPlacement` 已把符号值改成它在新 `.modtext.<m>` 里的物理偏移；
- 末尾哨兵：`[originalTextSize, +1) → runtime.etext`，让 `moduledataverify` 的 maxpc 覆盖全部文本。

> 重要：textmap 的重定位指向模块函数符号，这会让 lld 以为模块被"引用"而不肯 GC。所以链接期 patch 会把这些记录**清零**（见 §6.3），而不是靠 `--gc-sections` 的引用分析。

### 5.7 其他元数据同步

符号表每个函数符号的 `shndx`（所属 section）与 `st_value`（新偏移）都要更新；`.rela.modtext.<m>` 的 `sh_info` 指回目标 section。elfsplit 最后把新旧 section 表、符号表、重定位表完整写回，得到一个"功能等价、但按模块分节"的 `go.o`，再替换回 `libyak.a`。

---

## 6. 链接期：让 lld 真正删掉未用模块

`common/yak/ssa2llvm/compiler/compile_dispatch_selfcontained.go` 在每次编译脚本时收集模块依赖：

```go
deps := comp.YaklibDependencies()
for mod := range deps {
	usedModules = append(usedModules, mod)
}
if len(usedModules) > 0 {
	usedModules = append(usedModules, "shared")
}
if containsModule(usedModules, "poc") {
	usedModules = append(usedModules, "cli")
}
if containsModule(usedModules, "ssa") {
	usedModules = append(usedModules, "ssafront")
}
```

然后 `linkStaticWithPatch`（`linker_selfcontained.go`）执行"patch → lld → 修复 → 收敛"循环：

```go
for attempt := 0; attempt < 8; attempt++ {
	rp, err := assets.ReleaseTo(workDir)
	// 1. 对 release 出的 libyak.a 副本打补丁
	if err := patch.Patch(patch.Request{ArchivePath: rp.Libyak, UsedModules: used}); err != nil { ... }
	// 2. lld 链接：--gc-sections 真正丢弃 .modtext.<未用>
	linkArgs = append(linkArgs, "--gc-sections", "--icf=safe", "-s")
	if err := llvm.LinkExecutableStatic(in); err != nil { ... }
	// 3. 链接后修复 runtime 元数据
	if err := patch.SortFinalTextMap(binFile); err != nil { ... }
	// 4. 检查是否有"被 lld 保留但 textmap 不覆盖"的模块 → 重链
	missing, err := patch.MissingRetainedModules(binFile)
	...
}
```

### 6.1 为什么 patch 的是 archive 副本

`assets.ReleaseTo(workDir)` 把嵌入的 `libyak.a` 释放到编译 work dir，patch 修改的是这份**副本**。原始嵌入资产保持完整、不可变，manifest（SHA256）也稳定；每次脚本编译都从干净副本开始，不存在状态污染。

### 6.2 重定位中和：未用模块的引用指到 stub

`neutralizeModtextRelocs`（`runtime/patch/elf_linux.go`）扫描所有 RELA 节：

- 如果一条重定位引用的是"被移除模块"的符号：
  - 把它改指到 `main.yakUnusedModuleStub`（`//go:noinline` 的保留 no-op 函数）；
  - 按重定位宽度修正 addend（PC32/PLT32 补 `-4`，PC16 补 `-2`，PC8 补 `-1`），因为 PC-relative 的解析是 `S + A + 4`；
  - 绝对重定位 addend 保持 0。

为什么不直接清零？

> 清零 PC-relative 重定位是危险的：保留代码里如果有一条 `leaq moduleFunc(%rip), %rax`，清零后 `%rax` 会得到垃圾地址，运行时间接调用直接崩。指向 stub 则让"悬空函数指针"变成安全的 no-op。

```go
newInfo := (uint64(stubRaw) << 32) | (rInfo & 0xffffffff)
binary.LittleEndian.PutUint64(data[off+8:], newInfo)
switch elf.R_X86_64(rInfo & 0xffffffff) {
case elf.R_X86_64_PC32, elf.R_X86_64_PLT32:
	binary.LittleEndian.PutUint64(data[off+16:], ^uint64(3))
case elf.R_X86_64_PC16:
	binary.LittleEndian.PutUint64(data[off+16:], ^uint64(1))
case elf.R_X86_64_PC8:
	binary.LittleEndian.PutUint64(data[off+16:], ^uint64(0))
default:
	binary.LittleEndian.PutUint64(data[off+16:], 0)
}
```

### 6.3 inittask：被删模块的初始化任务标记 done

Go runtime 启动会执行 `..inittask` 数组里的每个任务。模块代码被删了，但任务记录还在——直接跑会跳到已删除的代码上。

`markInittasksDone` 的做法：找到所有 `main.<pkg>..inittask` 符号，扫描 RELA，凡是"重定位落在某 inittask 对象范围内、且指向被移除模块函数"的任务，把 `state` 字段写成 `2`（完全初始化）。这样 runtime 跳过它，不会碰已删除的代码。

```go
data[loc] = 2 // state = fully initialized
```

按"重定位目标"匹配而不是按包名匹配是刻意的：模块名 ≠ 包名（`yakit` 由 `runtime/shim` 实现），复合模块（`ssa`）包含多个包的多个任务。

### 6.4 textsectmap 重定位清零

`.rela.data.rel.ro.yaktextmap` 里指向被移除模块函数的记录直接清零（保留 `baseaddr=0` 条目）。这样：

- textmap 不会让 lld 误以为模块被引用而保留 section；
- `runtime.pcToOffset` 对未保留模块的 PC 查不到记录（这些 PC 本来就不该存在）。

### 6.5 链接后：SortFinalTextMap

elfsplit 的 textmap 记录按**原始 vaddr** 排序，但 lld 布局后 `.modtext.*` 的物理地址顺序不一定与虚拟地址顺序一致。Go runtime 的 `pcToOffset` 依赖"baseaddr 单调递增、找到第一个高于 PC 的项就停"。

`SortFinalTextMap` 在最终二进制里：

1. 找到 `.go.module` 里的 `moduledata`；
2. 读出 `textsectmap`，按 **baseaddr（物理地址）** 稳定排序后写回；
3. 把 `minpc/maxpc` 扩到 `[text, etext)`，否则 `findmoduledatap` 会拒绝模块函数 PC；
4. 把 pclntab ftab 的哨兵 `entryoff` 改指原文本末尾，保持 ftab 排序且让 `textAddr` 解析到 etext。

```go
sort.SliceStable(entries, func(i, j int) bool {
	return entries[i].baseaddr < entries[j].baseaddr
})
```

### 6.6 收敛：MissingRetainedModules

有时候基础 runtime 的 init 图仍然引用某个模块（比如 `shared` 里的包被其他保留代码间接引用），lld 会把它留下，但 patch 已经把它的 textmap 记录清零了——结果就是"代码在、PC 查找不到"，panic 栈会坏。

`MissingRetainedModules` 检查最终 ELF：

```go
for _, m := range retained {
	if !covered[m.name] {
		missing = append(missing, m.name)
	}
}
```

发现遗漏就把该模块加进 `used`，重新 patch + 链接（最多 8 次）。最终保留集与 textmap 覆盖严格一致。

---

## 7. 落地效果与验证

### 7.1 三类脚本的实测产物

`TestArtifactPruning_DualEvidence`（`tests/artifact_pruning_test.go`）用真实 CLI 编译三个代表性脚本，然后对产物做三层证据检查：

| 脚本 | 行为（env -i 运行） | 必须保留 | 必须不存在 | 大小 |
|---|---|---|---|---|
| `print_stdlib.yak` | 输出 "hello world 123" | （基础 runtime） | ssa / ssafront / poc / cli | 89,260,984 B |
| `ssa_go_parse.yak` | 输出 "hello-from-go" | ssa / ssafront / shared | poc / cli | 114,931,816 B |
| `poc_request.yak` | 输出 "0" | poc / cli / shared | ssa / ssafront | 98,822,280 B |

测试的三层证据：

```go
// A. behavior — 编译出的二进制在空环境下正常运行并输出期望内容
run := runProcess(t, bin, nil)
if run.ExitCode != 0 { ... }
if !strings.Contains(run.Output, tc.outputSub) { ... }

// B. size — 记录精确字节数
sizes[tc.script] = fi.Size()

// C. content — 用 debug/elf 直接检查 .modtext 节是否存在且可执行
mods := modtextModules(f)
for _, want := range tc.wantRetain {
	if !present[want] { ... }
	if !hasExecSection(f, ".modtext."+want) { ... }
}
for _, forbid := range tc.wantAbsent {
	if present[forbid] { ... }
}
```

加上跨脚本的大小序断言：`print < poc < ssa`，防止"裁剪退化成全保留"。

### 7.2 为什么是"真实代码缺席"，而不是"符号表缺席"

这个测试检查的是 **ELF section 级**证据：`debug/elf` 枚举的是 `SHT_PROGBITS` + `SHF_EXECINSTR` 的 `.modtext.*` 节。符号表（`.symtab`）可能被 `-s` 剥离、`.gopclntab` 里可能残留名字，但**代码本体在不在**只能看节。`wantAbsent` 断言的就是"该模块的代码节不存在"。

这也回答了"为什么 print 和 poc 大小差距不大"的疑惑：Go 运行时元数据（pclntab、typelink、itab、GC 位图、rodata 常量）本身就有几十 MB 的固定成本，代码裁剪只能砍掉"增量部分"。真正可比的证据是：

- 同一 libyak.a 下，print/poc/ssa 产物的 `.modtext.*` 集合不同；
- 大小严格满足 `print < poc < ssa`；
- 每个产物都能正常运行（包括 panic 栈、yakit 输出等依赖 PC 查找的功能）。

---

## 8. 边界与已知限制

1. **Go 全局元数据不能按模块删**。pclntab 是 Go 归档的固有限制：`runtime.pclntab` 是一个整体，类型/接口元数据（typelink、itab）由 Go runtime 自引用，不能按 yaklib 模块安全移除。本文方案删除的是**模块专属的机器码节**，元数据残留是 Go ABI 的代价，不是裁剪失效。
2. **`-s` 只影响 ELF symtab/DWARF**。Go 反射与 `runtime.Callers` 用的是 pclntab 而非 ELF symtab，所以剥离是安全的。
3. **跨模块引用必须精确**。`shared` 闭包、`poc→cli`、`ssa→ssafront` 的依赖规则如果漏掉，patch 会把必要引用指到 stub，导致保留模块运行时崩溃。`MissingRetainedModules` 的收敛循环只能兜住"被 lld 保留"的情况，拦不住"被错误中和"的情况——所以 `TestArtifactPruning_DualEvidence` 的行为断言是关键。
4. **旧 archive 兼容**。`neutralizeModtextRelocs` 找不到 stub 时会退回旧的清零行为（仍可链接，但运行时风险更高），保证迁移期 baseline 可用。
5. **只支持 ELF64/Linux**。`elf_linux.go` / `final_textmap_linux.go` 带 `//go:build linux`，其他平台无操作。

---

## 9. 结语

整个方案的链条是：

> **问题**：完整 c-archive 无法按脚本裁剪 → **根因**：Go 单一 `.text` 段无法 GC → **构建期解法**：elfsplit 按模块拆节 + per-module 注册导出 + 轻量 AOT shim → **链接期解法**：重定位指 stub、inittask 标 done、textmap 清引用 → **让 lld 真的删掉未用模块代码** → **链接后解法**：排序 textsectmap、修复 ftab/minpc/maxpc、收敛保留集 → **验证**：行为 + 大小 + section 三证据。

最终效果：`libyak.a` 保持完整（对任意脚本可编译），而每次链接只保留脚本实际用到的模块代码——"完整库"与"单脚本瘦身"不再互斥。
