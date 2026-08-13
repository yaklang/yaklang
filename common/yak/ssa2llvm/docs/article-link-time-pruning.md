# 一个 hello world 为什么是 85MB——以及它还能小到多少

## —— 给 Go c-archive 做链接期外科手术

> 本文记录 yaklang `ssa2llvm` AOT 编译器的一次真实工程实践：如何让一份"完整"的 Go c-archive 运行时（`libyak.a`），在每次编译 yak 脚本时由链接器自动丢弃该脚本用不到的模块代码。
>
> 所有数据均在本机实测，环境与复现命令见文末。文中同样如实记录了这套方案**没能**解决的部分，并把"还能更小多少、怎么做"逐条量化——那才是更有价值的部分。

---

## 0. 先看结论

我们做了一套构建期 ELF 拆节 + 链接期补丁的方案，效果是：

| 同一个 `print("hello")` 脚本 | 产物体积 | 相对全量 |
|---|---|---|
| 不裁剪（保留全部 9 个模块组） | 114,990,776 B（109.66 MiB） | — |
| 链接期裁剪后（编译器实际选择：0 个模块） | 89,260,920 B（85.13 MiB） | **−24.54 MiB（−22.4%）** |
| *（同一脚本，走"重建运行时"的另一条路径）* | *30,319,248 B（28.91 MiB）* | *−80.75 MiB（−73.6%）* |

第三行是本文最该被认真对待的一行：**同一个脚本、同样的输出，另一条路径只要 28.91 MiB。**

所以 85 MiB 不是物理下限，它只是**这套链接期方案的边界**。原因很具体：链接期裁剪能删掉模块的机器码，却删不掉它们在 `.gopclntab`、`.data.rel.ro` 里的元数据。有多离谱？在这个 hello world 产物里：

- 172,764 个函数的元数据中，**28.9% 来自 TypeScript AST 前端**，算上 PHP、Java、Python、JSP、Freemarker、antlr 等，**60.7% 的函数属于语言前端**；
- 而这些前端的机器码，恰恰是我们刚刚删掉的那 12.49 MiB `.modtext.ssafront`。

**代码删了，元数据一个字节没删。** 这就是 85 MiB 的真正来源。本文后半部分（§5、§7）会把这块的可回收量逐项量化，并给出继续往下走的操作路线。其中两条已经做完并实测：折叠 pclntab 的符号化数据（R1）再省 **14.63 MiB**；按依赖数据拆开公共模块组（R0）让纯计算脚本再省 **8.88 MiB**。两者正交，叠加后一个编解码脚本是 **94.50 → 70.99 MiB**。最彻底的一条实测能到 28.91 MiB。

---

## 1. 问题不是"Go 不会删代码"

`ssa2llvm` 的目标是把 yak 脚本编译成原生可执行文件（AOT），并且编译器本身要自包含：LLVM IR 生成、目标代码发射、静态链接全部 in-process 完成，运行时是嵌进编译器二进制里的 `libyak.a`（`go build -buildmode=c-archive`）+ `libgc.a` + crt + 静态 libc。产物是零外部依赖的静态可执行文件，`env -i` 下可直接运行。

于是有了一个矛盾：

- 编译器无法预知用户脚本会用到哪个 yaklib 模块（可能是 `print`，可能是 `poc.HTTP`，可能是 `ssa.Parse`），所以 `libyak.a` 必须**完整**；
- 但任何一个具体脚本只会用到其中很小一部分。

第一反应是"Go 链接器不会做死代码消除吧？"——**恰恰相反，它会**。`cmd/link` 有完整的 deadcode flood fill：从入口 roots 出发沿重定位标记可达符号，不可达的整个丢掉（见 [`src/cmd/link/internal/ld/deadcode.go`](https://github.com/golang/go/blob/master/src/cmd/link/internal/ld/deadcode.go)）。

真正的问题是**时机错位**：

1. `-buildmode=c-archive` 的 DCE roots 是**导出函数**。我们给每个模块生成了一个导出的注册函数：

```go
//export yak_register_module_ssa
func yak_register_module_ssa() {
	runtimeRegisterYaklibModule("ssa", ssaapi.Exports)
	runtimeRegisterYaklibModule("ssa", ssaproject.Exports)
	runtimeRegisterYaklibModule("ssa", ssaconfig.Exports)
}
```

   每个模块都从某个导出函数可达，所以 Go 的 DCE 一个也删不掉——这是对的，因为构建 archive 时确实还不知道谁会被用到。

2. 等到脚本编译期，我们终于知道了 used set，但此时**不能再跑一次 Go 链接器**。那意味着要求终端用户安装 Go 工具链，并且每编译一个脚本等上几分钟重新构建整个运行时。这与"自包含编译器"的前提直接冲突。

3. 此刻手上只剩 `lld`。而 `lld --gc-sections` 的粒度是 **section**，Go 的 c-archive 把整个运行时的机器码放在**一个** `.text` 里。等于"要么全留，要么全丢"。

顺带排除两个常见的想当然：

- `-ffunction-sections` 对 Go 代码不生效。它是 C/C++ 编译选项，通过 `CGO_CFLAGS` 只能作用于 cgo 的 C 部分，Go 编译器自己产出汇编，不受它控制。
- `strip -s` 只能去掉 ELF symtab 和 DWARF。Go 运行时依赖的 pclntab、typelink、moduledata、textsectmap 都在只读数据里，删了程序就起不来。

所以问题可以精确地重述为：

> **会做 DCE 的那个工具（Go 链接器）在错误的时间点已经下班了；还在上班的那个工具（lld）粒度不够。**

整套方案本质上就是**在事后把 lld 缺的那个粒度补出来**。

---

## 2. 为什么不选更笨但更稳的办法

这是本方案最该被质疑的地方，所以先正面回答。

| 备选方案 | 为什么没选 |
|---|---|
| 脚本编译期调用 `go build` 重建运行时 | 要求用户装 Go 工具链；每次编译等分钟级。与自包含前提冲突。（代码里保留了这条 legacy 路径，仅用于有完整工具链的环境） |
| 每个模块单独打一个 c-archive，按需链接 | **硬阻断**。每个 c-archive 自带一整份 Go runtime 和 `moduledata`，两份不能共存于同一个进程 |
| 预构建若干"档位"的 `libyak.a`（minimal / net / full） | 真正的对手方案。它更简单、零 ELF 风险，而且能用 Go 自己的 DCE 连元数据一起删（这点比本方案强）。代价是组合爆炸：模块任意组合是 2^N，每档几十到上百 MB 的分发成本。若未来模块数收敛到个位数且用户场景集中，这个方案值得重新评估 |
| 压缩 / 自解压 | 与本方案正交，减分发不减内存，可叠加 |
| 接受现状 | 行业基线确实如此：`deno compile` 的产物官方文档给出的典型值约 70 MiB，因为整个 JS 运行时都被打进去了（[Deno Docs: deno compile](https://docs.deno.com/runtime/reference/cli/compile/)）。但我们的分发场景对单脚本产物体积敏感 |

结论：**在"编译器自包含 + 单一完整 archive + 链接期决策"这三个前提下，ELF 拆节是唯一能自动化的路径。** 如果哪天前提变了（比如接受多档位预构建），方案就该重选。

---

## 3. 方案：把链接器缺的粒度补出来

分两个阶段，中间以"模块化的 `libyak.a`"为分界线。

```text
构建期（一次性）
  go build -buildmode=c-archive
        ▼
  libyak.a（完整，go.o 的 .text 是一整块）
        │  elfsplit：符号→模块归类，把 .text 拆成 .text + .modtext.<m>*
        ▼
  libyak.a（完整，但按模块分节）

链接期（每次编译脚本）
  编译器给出 used set（YaklibDependencies）+ 依赖闭包
        │  patch：未用模块的重定位指向 stub、inittask 标 done、textmap 清引用
        ▼
  lld --gc-sections   → 未用模块的 .modtext.<m> 整节被丢弃
        │  链接后修复：SortFinalTextMap（排序 textsectmap、修 minpc/maxpc/ftab）
        ▼
  脚本专属的静态可执行文件
```

### 3.1 used set 不是猜出来的

先说方案里我认为最关键、也最容易被忽略的一点。

JS bundler 的 tree-shaking、ProGuard/R8 的裁剪，都要靠启发式分析去**猜**哪些代码用不到，猜错了就是线上炸。而 `ssa2llvm` 不需要猜：编译器在生成 IR 时，本来就必须把 `codec.EncodeBase64` 解析成一个具体的 dispatch 调用，所以模块依赖是**编译过程的天然副产品**：

```go
deps := comp.YaklibDependencies()   // {"codec": ["EncodeBase64"]}
```

再叠加一层显式的依赖闭包（任何模块被使用则需要 `shared`；`poc` 需要 `cli`；`ssa` 需要 `ssafront`），就得到最终的保留集。另外还有一道 `ValidatePrunedRuntimeDependencies` 把关：脚本用到了裁剪运行时不支持的方法，直接在编译期报错，而不是产出一个运行时才崩的二进制——fail-closed。

一句话：**编译器天生知道精确的 used set，缺的只是链接器能利用这个信息的粒度。**

### 3.2 构建期：elfsplit 把一整块 .text 拆开

`cmd/elfsplit`（约 1200 行）是一个 ELF 重写工具：输入 `go.o`，输出按模块分节的 `go_split.o`。它要做五件事，每一件单独拿出来都是坑：

**（1）符号归类。** 把每个 `main.<pkg>.<func>` 按包路径前缀映射到 yaklib 模块。注意模块 ≠ Go 包：`ssa` 模块由 `ssaapi` + `ssaproject` + `ssaconfig` 三个包合成，`yakit` 模块由 `runtime/shim` 实现，而 `common/yak/yaklib` 一个包同时是多个模块的导出来源。所以不能按 Go 包边界切。

**（2）代码搬迁。** 每个模块的代码复制进自己的 `.modtext.<m>`，基础 runtime 留在压缩后的 `.text`。这里不是"删除"——archive 依然完整，只是从"一个连续段"变成"多个可独立 GC 的段"。

**（3）重定位搬家。** 原 `.rela.text` 里每条 RELA 的 `r_offset` 指向 `.text` 内某处。代码搬走后重定位必须跟着搬到对应的新节，否则 lld 会把重定位作用到别的字节上。

**（4）直接分支重写。** 这是最脆的一环。Go 编译出的代码里，函数之间的直接跳转在汇编层面是相对位移，**没有**对应的 ELF 重定位。代码搬家后，同模块内的相对分支必须重算，跨模块分支要合成一条 `R_X86_64_PC32` 交给 lld：

```go
// 用 x86asm 逐条解码，识别直接分支（x86asm.Rel）和 RIP-relative 内存操作数
// （Go 用 leaq sym(%rip) 取函数地址，open-coded defer 也依赖它）
oldTarget := fn.oldOff + pos + uint64(inst.Len) + disp
// 同模块：直接写新位移；跨模块：生成合成 RELA；已有 ELF 重定位的位置跳过
```

线性反汇编本身是不可靠的（符号大小可能包含不可解码的填充字节），代码里的兜底是"解不出来就跳过继续扫，找不到目标 placement 就不动"。**这是整个方案里正确性最依赖经验判断的地方**，后面 §6 会展开。

**（5）textsectmap。** Go 的 `moduledata.textsectmap` 负责把 PC 映射回物理地址，是 `findfunc`/traceback/pprof 的基础。elfsplit 为每个保留代码块生成一条记录，编码进 `.data.rel.ro.yaktextmap` + 对应的重定位节，末尾再放一个哨兵让 `maxpc` 覆盖全部文本。

### 3.3 链接期：让 lld 真的敢删

拆完节还不够。直接 `--gc-sections` 会发现一个模块也删不掉，因为到处都是指向它的引用。patch 阶段做三件事：

**（1）重定位中和。** 所有指向"被移除模块"符号的重定位，改指到一个保留的 no-op 函数 `main.yakUnusedModuleStub`，并按重定位宽度修正 addend（PC32/PLT32 补 −4 等）。

为什么不直接清零？因为清零 PC-relative 重定位是危险的：保留代码里若有 `leaq moduleFunc(%rip), %rax`，清零后 `%rax` 拿到的是垃圾地址，运行时间接调用直接崩。指向 stub 至少让悬空函数指针变成安全的 no-op。

实测规模：hello world 那一档，**239,176 条重定位被改指 stub**。

**（2）inittask 标记完成。** Go 启动时会顺序执行 `..inittask` 数组里的每个任务。模块代码删了但任务记录还在，直接跑会跳到已删除的代码上。做法是扫描重定位，凡是落在某个 inittask 对象范围内、且指向被移除模块函数的，把 `state` 字段写成 `2`（完全初始化）。

按"重定位目标"匹配而不是按包名匹配是刻意的——模块名和包名并不对应。

**（3）textmap 清引用。** textmap 的重定位指向模块函数符号，这会让 lld 认为模块被引用而拒绝 GC。所以指向被移除模块的记录直接清零。

### 3.4 链接后：还要再修一次

`lld` 布局后 `.modtext.*` 的物理地址顺序不一定与原始虚拟地址顺序一致，而 Go 的 `pcToOffset` 依赖 baseaddr 单调递增。`SortFinalTextMap` 在最终二进制里按物理地址稳定排序写回，同时把 `minpc/maxpc` 扩到 `[text, etext)`，并修正 pclntab ftab 的哨兵。

最后是一个收敛循环：基础 runtime 的 init 图有可能仍然引用某个模块，lld 把它留下了，但 patch 已经清了它的 textmap——结果是"代码在、PC 查不到"。`MissingRetainedModules` 检查最终 ELF，发现遗漏就把模块加回 used 重新链接，最多 8 次。

---

## 4. 实测

### 4.1 受控 A/B：同一个目标文件，只改保留集

要回答"裁剪到底省了多少"，唯一严格的做法是**控制变量**：同一个已编译的脚本目标文件、同一份 archive、同一个链接器，只改 used-module 集合。

| 档位 | 保留模块 | 产物体积 | 链接耗时 |
|---|---|---|---|
| `pruned` | （编译器实际选择：无） | 89,260,920 B（85.13 MiB） | 1.299 s |
| `+shared` | `shared` | 98,788,520 B（94.21 MiB） | 1.243 s |
| `−ssa` | 除 `ssa`/`ssafront` 外全部 | 98,849,272 B（94.27 MiB） | 1.399 s |
| `full` | 全部 9 组（等价于不裁剪） | 114,990,776 B（109.66 MiB） | 1.566 s |

**裁剪收益 = 25,729,856 B = 24.54 MiB = 22.4%。**

一个漂亮的交叉验证：产物体积差（25,729,856）与保留的 `.modtext` 节大小之和（25,728,903）只差 **953 字节**，正好是节对齐填充。这说明裁剪删掉的**精确地**就是模块机器码，没有多删也没有少删。

链接耗时 1.2–1.6 秒，收敛循环在这些用例上都是一次通过——性能不是这套方案的瓶颈。

### 4.2 三层证据

只看体积是不够的，体积变小也可能是把程序弄坏了。所以每个产物都过三层证据：

**A. 行为**——`env -i` 空环境下运行，输出符合预期：

```
$ env -i ./pruned.bin
hello world 123
x=1
```

**B. 内容**——用 `debug/elf` 直接检查 `.modtext.*` 节是否存在且可执行。这一层检查的是 **section 级**证据，而不是符号表：`-s` 会剥离 symtab，pclntab 里也可能残留名字，**代码本体在不在只能看节**。

**C. 体积**——记录精确字节数，并断言跨脚本的大小序（print < poc < ssa），防止裁剪退化成全保留。

### 4.3 panic 路径仍然正确

这是我最在意的一项验证。整套 textsectmap/ftab/minpc 的修复工作，保护的就是 Go 运行时的 PC 查找；一旦修错，panic 打印栈、`runtime.Callers`、pprof 会全部崩掉，而且是那种"平时看不出来、出事时才发现"的崩法。

拿一个会越界 panic 的脚本走完整裁剪路径（最终产物 **0 个 `.modtext` 节**，238,667 条重定位被中和）：

```
$ env -i GOTRACEBACK=all ./panic_probe.bin
before
[yak-runtime] panic: index "3" out of range
[yak-runtime] panic: index "3" out of range
0
after
```

panic 被正确捕获、报告，执行继续（报告打印两次是 AOT 运行时自身的行为，与裁剪无关）。

另一道防线是 Go 启动时的 `moduledataverify`：它会校验 pclntab 函数表的排序，以及 `maxpc` 与 ftab 末项经 `textAddr` 解析后的一致性——而 `textAddr` 的解析要经过我们重写过的 `textsectmap`。产物能启动到 `schedinit` 之后，本身就说明这套表是自洽的。这也正是 §3.4 里必须修正 ftab 哨兵 `entryoff` 的原因。

---

## 5. 剩下的 85 MiB 是什么

裁剪之后 hello world 仍然是 85.13 MiB。它由什么构成？（下表为文件内实际占用的节，`.bss`/`.noptrbss` 是 NOBITS 不占文件，已排除）

| 节 | 大小 | 占产物 | 说明 |
|---|---|---|---|
| `.gopclntab` | 43,949,436 B（41.91 MiB） | **49.2%** | Go 的 PC→行号/函数元数据表 |
| `.data.rel.ro` | 18,365,160 B（17.51 MiB） | **20.6%** | 需重定位的只读数据：类型描述符、itab、moduledata 等 |
| `.text` | 16,669,284 B（15.90 MiB） | 18.7% | 基础 runtime 机器码（Go runtime + AOT runtime + libc/libgc） |
| `.rodata` | 7,496,796 B（7.15 MiB） | 8.4% | 只读常量 |
| `.data` | 1,264,380 B | 1.4% | |
| `.noptrdata` | 1,239,937 B | 1.4% | |
| `.eh_frame` | 246,144 B | 0.3% | |
| 合计 | 89,231,137 B | 99.97% | |

**元数据（pclntab + data.rel.ro）占了 69.8%，机器码只占 18.7%。**

更说明问题的一个观察：`.gopclntab` 在裁剪产物和全量产物里**字节数完全相同**（43,949,436）。也就是说，pclntab 仍然在描述那些机器码已经被删掉的函数。textmap 清零保证了这些 PC 查不到（它们本来也不该存在），但那 41.91 MiB 的表还原封不动地躺在产物里。

这就是本方案的边界：**它是一套"代码裁剪"方案，而产物的大头不是代码。**

但"不是代码"不等于"不能删"。下面把这 41.91 MiB 拆开看。

### 5.1 pclntab 不是一张符号表，但它有一半是

一个常见误解是把 `.gopclntab` 当成"给逆向工程用的符号表，删了就行"。**不能删**：Go 运行时用它做栈展开，而栈展开是 **GC 精确扫描栈、栈增长/拷贝、panic/recover、抢占**的前提。Go 团队的 Ian Lance Taylor 在 golang-nuts 上说得很直接——移除 pclntab 会让"大多数有实际规模的 Go 程序无法正确工作"，但同时他也指出"**原则上可以删掉其中一部分，比如函数名，后果没那么严重**"（[golang-nuts: runtime.pclntab stripping](https://groups.google.com/g/golang-nuts/c/hEdGYnqokZc/m/ltonS9eAAwAJ)）。

按 `pcHeader` 把这 41.91 MiB 拆成子表，就能看清哪部分是命脉、哪部分只是给人看的：

| 子表 | 大小 | 占 pclntab | 性质 |
|---|---|---|---|
| `funcnametab`（函数名字符串） | 15,334,904 B（14.62 MiB） | 34.9% | **仅符号化** |
| `filetab`（源文件名） | 195,480 B | 0.4% | **仅符号化** |
| `cutab`（编译单元→文件索引） | 157,432 B | 0.4% | **仅符号化** |
| `pctab`（pc-value 变长表） | 7,084,560 B（6.76 MiB） | 16.1% | 混合：`pcsp` 必需，`pcfile`/`pcln` 仅符号化 |
| `ftab` + `_func` + funcdata | 21,176,988 B（20.20 MiB） | 48.2% | **运行时必需**（含 GC stackmap 指针） |

**纯符号化数据合计 15,687,816 B = 14.96 MiB，占 pclntab 的 35.7%，占整个产物的 17.6%。** 这部分即使全部抹掉，GC、栈增长、panic/recover 也照常工作，代价只是 traceback 里没有函数名和行号。`pctab` 里的 `pcfile`/`pcln` 还能再挖一块，这里保守地整块算作必需。

顺带说清楚工具链现状，免得走弯路：

- **Go 官方没有提供任何裁剪 pclntab 的开关。** `-ldflags="-s -w"` 只处理 ELF symtab 和 DWARF，完全不碰 pclntab。
- 社区提过 [golang/go#36555](https://github.com/golang/go/issues/36555)（给链接器加 `-stripfn` 之类的 flag），有人实现了补丁，但**至今没有合并进主干**；[golang/go#36313 "runtime: pclntab is too big"](https://github.com/golang/go/issues/36313) 则是上游对这个问题的长期跟踪 issue，讨论里提到在某些 Google 内部二进制中"多达 50% 的 pcln 被字符串占用"。
- 生态里能用的是 [garble](https://github.com/burrowers/garble)：它走的是**改写运行时源码 + 重新编译**的路子（比如把 traceback 打印函数掏空），而不是对成品二进制做后处理。

结论：这条路可行，但**必须自己做**，而且要么改链接器、要么做 ELF 后处理——正好是本文方案已经在做的事情。

### 5.2 真正的大头：给已经删掉的代码保留的元数据

把产物里 172,764 个函数按包前缀分桶，结果非常刺眼（这是一个 **hello world**）：

| 包前缀 | 函数数 | 占比 | 仅函数名占用 |
|---|---|---|---|
| `common/yak/typescript/frontend/ast` | 49,977 | **28.9%** | 5,006,066 B |
| `common/yak/php/parser` | 10,323 | 6.0% | 907,975 B |
| `common/yakgrpc/ypb`（*非前端，见下*） | 9,804 | 5.7% | 810,364 B |
| `common/yak/java/parser` | 8,693 | 5.0% | 776,421 B |
| `common/yak/ssa` | 6,154 | 3.6% | 407,803 B |
| `common/syntaxflow/sf` | 5,136 | 3.0% | 436,813 B |
| `common/yak/antlr4go/parser` | 4,882 | 2.8% | 418,925 B |
| `common/yak/antlr4c/parser` | 4,864 | 2.8% | 441,086 B |
| `common/yak/python/parser` | 4,162 | 2.4% | 350,282 B |
| …（antlr4yak / freemarker / ssaapi / jsp …） | | | |
| **语言前端类合计** | **≈104,832** | **≈60.7%** | |

（合计一行只统计语言前端相关包，不含 `ypb`。）

一个只打印 hello world 的程序，产物里 60.7% 的函数元数据属于 TypeScript AST、PHP/Java/Python/JSP/Freemarker 语法分析器。而这些前端的机器码，正是我们在 §4.1 里刚刚删掉的 12.49 MiB `.modtext.ssafront`。

**我们删掉了代码，却把描述这些代码的元数据一字不落地留下了。** 这不是 Go ABI 的固有成本，这是本方案目前的一个缺口。

顺便还暴露出一个独立的问题：`common/yakgrpc/ypb`（gRPC 的 protobuf 定义）有 9,804 个函数，而且它在**两种构建里都存在**——在下文的对照组 `p1.bin` 里，它占到了全部函数的 23.4%。也就是说它被算进了"基础 runtime"，任何脚本都躲不掉。AOT 产物是否真的需要完整的 gRPC 接口定义，这是一个值得单独确认的问题。

### 5.3 对照组：让 Go 链接器自己来会怎样

同一个仓库里其实还有另一条路径：`BuildPrunedRuntimeArchiveFromSourceTreeWithDeps`——按脚本的依赖重新生成运行时源码，然后**重新跑一遍 `go build`**。它需要 Go 工具链 + clang，所以自包含模式没有采用它。

`build/p1.bin` 就是这条路径的产物（依据：`go version -m` 显示的模块路径与 AOT 运行时相同，但依赖列表短得多，且不含任何 `.modtext` 节，说明它来自一棵 import 更少的源码树而非拆分后的 archive）。把它和本文方案的产物放在一起：

| | 链接期裁剪（本文方案） | 重建运行时（Go 自己 DCE） |
|---|---|---|
| 产物体积 | 89,260,920 B（85.13 MiB） | **30,319,248 B（28.91 MiB）** |
| 函数数量 | 172,764 | **41,892** |
| `.gopclntab` | 41.91 MiB | **11.63 MiB** |
| `.data.rel.ro` | 17.51 MiB | **5.44 MiB** |
| `.text` | 15.90 MiB | **8.09 MiB** |
| `.rodata` | 7.15 MiB | **2.07 MiB** |
| 运行输出 | `hello world 123 / x=1` | `hello world 123 / x=1` |
| 编译期依赖 | 无（完全自包含） | Go 工具链 + clang，分钟级 |

差距是 **56.22 MiB（−66%）**，而且每一个节都在缩小——因为 Go 链接器的 DCE 是从符号图上做的，删函数的同时，它的 pclntab 条目、类型信息、字符串**一起消失**。

这就把问题定性得很清楚了：

> **不是"元数据删不掉"，而是"能删元数据的那个工具（Go 链接器），我们没让它参与"。**

需要说明的是，28.91 MiB 也不是终点。同类的"嵌入完整运行时"产品也在这个量级——`deno compile` 官方文档给出的典型值约 70 MiB，而 Deno 正在试验用 QuickJS 替换 V8，把 hello world 从约 65 MiB 压到约 35 MiB。**当运行时本身成为主体时，最有效的手段是换更小的运行时。**

### 5.4 另一个发现：模块粒度严重不均

把每个模块节的真实大小列出来：

| 模块节 | 大小 |
|---|---|
| `.modtext.ssafront` | 12,492,427 B（11.91 MiB） |
| `.modtext.shared` | 9,527,468 B（9.09 MiB） |
| `.modtext.ssa` | 3,648,888 B（3.48 MiB） |
| `.modtext.cli` | 32,020 B |
| `.modtext.http` | 25,732 B |
| `.modtext.yakit` | 2,212 B |
| `.modtext.os` | 52 B |
| `.modtext.poc` | 52 B |
| `.modtext.codec` | 52 B |

`os`、`poc`、`codec` 各只有 **52 字节**——那只是它们的注册函数。真正的实现代码全都落在 `shared` 里（`schema`/`lowhttp`/`net-http`/`gorm` 的依赖闭包）。

后果很直接：**裁不裁 `poc` 只差 52 字节**；而 `shared` 的规则是"只要用了任何一个 yaklib 模块就必须保留"。所以一个只调用 `codec.EncodeBase64` 的脚本，要为 9.09 MiB 的 `shared` 买单。§4.1 的 A/B 表里 `+shared` 和 `−ssa` 两档只差 60,752 字节，就是这个事实的直接体现：除 `shared` 外的六个模块加起来只有 58.7 KiB。

结论：**当前 9 个模块组里，真正有裁剪意义的只有 3 个**（`ssafront`、`shared`、`ssa`），它们贡献了全部 24.54 MiB 收益中的 99.8%。下一步真正该做的是把 `shared` 这个大包袱拆开，而不是继续增加模块数量。§7 的 R0 就是这件事，已经做完：公共核心从 9.09 MiB 降到约 1 MiB。

---

## 6. 边界与已知风险

写给打算照着做的人。这套方案的适用范围比它看起来窄得多。

**（1）只支持 linux/amd64。** patch 与 textmap 修复带 `//go:build linux`，分支重写依赖 x86 指令解码。arm64（含 Apple Silicon 与 ARM 服务器）、macOS、Windows 全部不支持，且移植不是小工作量。

**（2）深度耦合 Go 内部 ABI。** `moduledata` 布局、`textsectmap` 结构、pclntab ftab 编码、`inittask.state = 2` 这些都是 Go 运行时的**非稳定内部实现**，没有兼容性承诺。每次升级 Go 都必须重新验证。本文数据基于 Go 1.26.5（GOEXPERIMENT `nodwarf5`）+ LLVM 22.1.8。

**（3）线性反汇编的可靠性边界。** §3.2 第 4 点的分支重写是最脆的一环。当前的安全论证是：函数体内不含数据、跳转表走 rodata 重定位由 lld 处理、无法确定目标的候选一律不改。但"解码失败继续逐字节扫描"这条兜底路径在理论上能构造出误判，一次误判就是运行时随机跳转。这一块值得单独做 fuzz/差分验证。

**（4）依赖闭包算错的后果，以及两道防线。** 这是整套方案最容易出错的地方：闭包漏一个模块，被中和的调用就会跳进 stub。最初的 stub 是 no-op——**什么都不做地返回**，调用方拿着未初始化的返回寄存器继续跑，问题在任意远的地方才暴露。现在有两道防线。

**运行期：per-module stub。** 每个组生成一个自己的 stub，命中即 panic 并报出组名：

```
panic: yaklib module was pruned at link time but is still reachable: ssafront
  (the compiler's used-module closure is missing it)
```

这一条改完当天就抓到了真问题：加进 `str` 模块后，`common/yak/yaklib` 的 init 调用 `goja.New`，而 goja 属于 `ssafront`。换成 no-op stub 的话，这就是一个"JS 引擎没初始化"的幽灵 bug。

**构建期：启动路径闸门。** 更好的是根本不让这种 archive 被构建出来。elfsplit 现在会做一遍可达性分析：从每个包 init 出发，沿调用边前向遍历，命中"当前上下文不保证存在"的组就报错并给出路径：

```
elfsplit: 2 start-up path(s) reach code that would be pruned:
  common/yak/yaklib.init -> github.com/dop251/goja.New -> .modtext.ssafront
  common/cybertunnel/tpb.file_tunnel_proto_init
    -> google.golang.org/protobuf/internal/filetype.Builder.Build -> .modtext.sharednet
```

这个闸门有三处细节，少一处就会误报或漏报：

- **必须看直接分支，不能只看重定位。** 第一版只扫 `.rela.text`，结果 `yaklib.init -> goja.New` 这种普通 `call` 完全看不见——同一个 `.text` 内的直接调用没有重定位，它只存在于机器码里。分支重写那一趟本来就在逐条解码指令，顺手把边收下来即可。
- **只算控制转移，不算取地址。** `lea sym(%rip)` 把函数地址存进变量不等于会执行它，init 里定义的闭包（`init.func1`）大多是注册用的回调。把取地址也算成调用，19 条报告里有 13 条是误报；只认 `x86asm.Rel` 分支和 `R_X86_64_PLT32` 之后，剩下的每一条都是真的。
- **可达性要对"上下文"敏感。** 一个在 ssa 组里的 init，只在保留了 ssa 的产物里运行，它调用公共核心完全没问题；同样的调用出现在基础 `.text` 里就是 bug——基础代码在什么都不保留时也要跑。所以遍历按"起点所在的组"分别做一遍，安全性判据是集合包含：保留 A 是否蕴含保留 B。

**（5）`--icf=safe` 在这里是安慰剂（已移除）。** 链接参数里原本带着 `--icf=safe`，但 ld.lld 的 safe ICF 依赖 clang `-faddrsig` 产生的 `.llvm_addrsig` 节；该节缺失时 lld 保守地把所有符号视为 address-significant，即不折叠（见 [MaskRay: Explain GNU style linker options](https://maskray.me/blog/2020-11-15-explain-gnu-linker-options#icfall-and---icfsafe)）。实测 `go.o` 中不存在 `.llvm_addrsig`，所以它对 Go 代码不可能生效。

   更需要警惕的是反面：**假如它真的生效了，反而是危险的。** 两个相同的 Go 函数被折叠到同一地址后，textmap 里指向被折叠符号的记录会解析到另一个函数，traceback 归属直接错乱。Go 自己的链接器不做这种折叠是有原因的。

**（6）进程内 lld 报错即崩溃——诊断必须前移。** 有几个脚本编译时会直接把编译器进程打成 SIGSEGV，Go traceback 里只有 `go-llvm/lld.go:46`，看不出任何原因。把 `-x` 打出的链接命令拿到命令行上用同版本 `ld.lld` 重跑一遍，一句话就说清楚了：

```
ld.lld: error: undefined symbol: yak_register_module_str
>>> referenced by ssa2llvm-out.o:(main)
```

脚本用了 `str`，而这个 `libyak.a` 是按 `os,poc,cli,http,codec,yakit,ssa` 构建的，根本没有这个模块。**和裁剪毫无关系**，是一个普通的"模块没编进去"。但进程内 lld 在报告这个错误的过程中会带着整个进程一起死，用户看到的就成了一个无从下手的崩溃堆栈。

既然错误对象无法在崩溃后补救，诊断只能前移到调用 lld 之前：读脚本目标文件里未定义的 `yak_register_module_*` 符号，和 archive 的符号索引比对。用符号索引（ar 的首个 `/` 成员，几十 KB）而不是 200 MB 的对象体，成本可以忽略。现在的输出是：

```
Error: the embedded runtime has no yaklib module "str"; it was built with: cli, codec, http, os, poc, ssa, yakit
rebuild it including the missing module(s):
  SSA2LLVM_EMBED_MODULES=cli,codec,http,os,poc,ssa,yakit,str bash common/yak/ssa2llvm/scripts/build_yaklib.sh
```

这里有个容易走偏的地方：我最初拿编译器算出的 used set 去比对，报出来的却是 `"shared, str"`——因为 used set 里混着 `shared` 这类只做拆分、不做注册的组。**问哪个符号真的未定义，比问我以为需要什么更准确**，而且这正是 lld 会做的判断。

**（7）panic 路径没有自动化回归。** §4.3 的验证是手工做的；现有的 panic 相关测试走的是未拆分的 debug 运行时。裁剪产物的 traceback 正确性目前只有启动期 `moduledataverify` 这一道自动防线。

---

## 7. 还能更小：五条路线与操作方案

85.13 MiB 不是终点。把 §5 的数据摊开，可回收的量是明确的，路线也是明确的——区别只在难度和代价。

| 路线 | 预计可回收 | 难度 | 主要代价 |
|---|---|---|---|
| **R0** 拆 `shared` 模块组 | 最多 9.09 MiB | 低 | 只是工程量，不碰任何 ELF 结构 |
| **R1** 折叠 pclntab 符号化数据 | **已实现，实测 14.63 MiB**（84.62 → 69.99 MiB） | 中 | traceback 丢函数名（行号仍在） |
| **R2** pclntab 条目级裁剪 | 在 R1 之上再约 15 MiB | **高** | 触碰 GC 关键结构，隐性错误风险高 |
| **R3** 预构建档位 archive | **实测可达 28.91 MiB 量级** | 中 | 分发体积 × 档位数 |
| **R4** 更小的运行时 / 分层运行时 | 数量级 | 高 | 语义兼容性 |

R1 和 R2 加起来约 30 MiB，正好等于 §5.3 里 pclntab 的超额部分（41.91 − 11.63 = 30.28 MiB），所以两者不重复计算。但即便全做完，产物也只到 55 MiB 左右，仍高于 p1.bin 的 28.91 MiB——差额在 `.data.rel.ro`（12.07 MiB）、`.text`（7.81 MiB）、`.rodata`（5.08 MiB）里，**那部分是类型描述符和 itab，后处理工具动不了，只有 Go 链接器能去掉**。这就是 R3 不可替代的原因。

下面按"我会先做哪个"的顺序展开。

### R0：拆 `shared`（已实现，纯计算脚本 −8.88 MiB）

§5.4 量化过：`shared` 是"用了任何一个 yaklib 模块就必须保留"的 10.00 MiB。一个只调 `codec.EncodeBase64` 的脚本要为它全额买单。

结论先放这里：

| 脚本 | 拆分前 | 拆分后 |
| --- | ---: | ---: |
| `println("hello")`（不用任何模块） | 84.62 MiB | 84.62 MiB |
| `codec.EncodeBase64` / `yakit.Info` | 94.50 MiB | **85.62 MiB** |
| `http.Get` | 94.50 MiB | 94.50 MiB |

纯计算脚本 **−8.88 MiB（−9.4%）**，且现在离"什么模块都不用"的地板只差 1.00 MiB。用网络的脚本不变——它本来就需要那些代码。

**分组不能靠手写清单。** 我第一版想按"网络栈 / 数据库 / schema"人工分类，量了一下就放弃了：`shared` 的 10 MiB 摊在 139 个包上，最大的一块只有 0.58 MiB，没有可以整块切走的东西。

真正能回答"哪些包该分到一起"的是依赖数据。对每个模块的 AOT 入口包跑 `go list -deps`，然后**按"哪几个模块的闭包包含这个包"给包分类**——同一个类里的包永远同生共死，天然就是一个段：

```
360 个包  ssa                          （只有 ssa 用）
278 个包  cli+http+poc+ssa             （除 codec/os/str 外都用）
197 个包  aotlib+cli+http+poc+ssa      （全都用）
 95 个包  http+poc+ssa
 61 个包  aotlib+cli+http+poc+ssa+yakit
```

`aotlib` 是 codec/os/str 这些纯计算模块在 AOT 下共用的 shim 包。含 `aotlib` 的类是真正的公共核心，剩下的类都可以按需保留。落到实现上是一个新段：核心 `shared` 从 10.00 MiB 缩到 1.01 MiB，其余 8.81 MiB 进 `sharednet`（脚本用到 cli/http/poc/ssa 中任意一个才保留），另有一部分并入已有的 `ssafront`（保留条件与它完全相同：脚本用了 ssa）。

（这里的 MiB 是 elfsplit 按 archive 内函数符号求和得到的，与 §5.4 表里同一组在链接产物中的 9.09 MiB 是两把不同的尺子，差值是对齐与 lld 的取舍。）

**这里有一条必须遵守的不变式**，我是踩了三次才把它写清楚的：

> 每一个依赖类都必须落到某个组，而且该组的保留条件不能比这个类更宽松。

理由是传递性：如果包 `p` 只被模块集合 `S` 触达，那么 `p` import 的每个包都至少被 `S` 触达，因而落在一个"`p` 在时它一定也在"的组里。反过来，只要有一个类被漏掉、留在了永远保留的核心里，它的依赖却被分到了可裁剪的组，核心就会在启动时调用一个已被替换成 stub 的函数。我漏掉 `ssa`-only 那 360 个包时，正是这样炸的：

```
github.com/yaklang/yaklang/common/cybertunnel/tpb.file_tunnel_proto_init
  -> google.golang.org/protobuf/internal/filetype.Builder.Build -> .modtext.sharednet
```

`tpb` 只有 ssa 会用，却因为不在任何分组规则里而留在核心；它的 protobuf 依赖被分到了 `sharednet`。于是 `codec` 脚本一启动就 panic。

还有两个来自数据的约束，不加就会出事：

1. **只有原本就在 `shared` 里的包可以被移动。** 生成器用当前 `.modtext.shared` 的符号表作为白名单。第一版没有这个限制，把 `log/slog` 和 `common/utils` 分了出去——它们在基础 `.text` 里，基础代码随时会调。
2. **核心里"不属于任何模块闭包"的包，其依赖要钉在核心。** 这些包是被基础运行时 import 进来的，init 无条件执行，所以它们需要的东西必须一起留下。

**顺带修好的一个存量 bug。** 分组做完后 `http` 脚本立刻 panic：`cli` 被裁掉但仍可达。查依赖数据一目了然——`common/utils/cli` 的类是 `cli+http+poc+ssa`，即 http/poc/ssa 的代码都会调用 cli 模块的实现；而编译器的闭包里只写了 `poc → cli`。这个洞在 fail-loud stub 之前是**静默的**：调用直接返回，然后脚本拿着未初始化的返回值继续跑。

顺带一提，这类"闭包规则"必须进构建缓存的 key。改完规则第一次实测毫无变化，因为缓存命中了旧产物——规则决定了产物里保留哪些模块，它就是产物的一部分。

### R1：折叠 pclntab 的符号化数据（已实现，实测 −14.63 MiB）

这一条已经写完并实测。结论先放这里：

| 产物 | 体积 | `.gopclntab` |
| --- | ---: | ---: |
| 折叠前 | 84.62 MiB | 41.93 MiB |
| 折叠后 | **69.99 MiB** | **27.30 MiB** |

同一个 hello world 脚本，**−14.63 MiB（−17.3%）**，运行结果逐字节一致。这个数字和 §5.3 里"约 15 MiB 是纯符号化数据"的静态估算对得上。

**关键设计选择：放在构建期（elfsplit）而不是链接期。** 函数名的裁剪**不依赖 used set**——不管脚本用哪些模块，语言前端那 5 MB 的函数名都不需要出现在任何产物里。放在构建期意味着一次性成本、所有产物受益，而且 elfsplit 已经具备完整的 ELF 段表/符号表重写能力。

**实际做法**（`cmd/elfsplit/pclntab.go`，比原计划更简单）：

1. 读 `.gopclntab` 的 `pcHeader`，校验 magic 与五个子表偏移单调递增；
2. 把每个 `_func` 的 `nameOff` 清零，而不是"指向一个共享空字符串"——`funcnametab[0]` 本身就是空串，省掉一次分配；
3. 从段里删掉 `funcnametab` 的字节，`pcHeader` 中其后的 `cuOffset`/`filetabOffset`/`pctabOffset`/`pclnOffset` 全部减去被删长度；
4. 建一个同样大小的 `SHT_NOBITS` 段 `.yakfuncnames`，把 `moduledata.funcnametab` 指过去。**不占文件字节**，但保留了合法的长度——内联帧的名字偏移无法枚举，它们会落进这块全零区域，读到空串而不是越界。

原计划里的 `filetab`/`cutab`（合计 0.34 MiB）没做：收益只有 `funcnametab` 的四十分之一，却要额外改写每个 `_func` 的 `cuOffset` 和 `pcfile`。不值。

**踩到的坑（这条值得单独记）**：`moduledata` 里指向 pclntab 的引用不止那五个 slice 头。Go 链接器还会把 `moduledata.findfunctab` 指到 pclntab **末尾之后**的一块区域。第一版按字段名逐个修 addend，漏掉了它——`findfunctab` 于是指偏 14.6 MiB，`findfunc()` 返回错误的 `_func`，程序在第一次栈增长时死于 `fatal error: missing stackmap`。

修法是不要按字段枚举，而是**扫描所有重定位节里指向 `runtime.pclntab` 的每一条**，按 addend 落在被删区间的哪一侧统一处理：

```go
patched := img.retargetSymbolAddends(pclntabSym, func(addend uint64) (uint32, uint64) {
	switch {
	case addend < funcnameOffset: // pcHeader 本身，不动
		return pclntabSym, addend
	case addend < cuOffset: // funcnametab 指针，改指 NOBITS 段
		return namesSym, addend - funcnameOffset
	default: // 其余全部前移
		return pclntabSym, addend - delta
	}
}) 
```

教训是通用的：**做二进制重排时，"我知道有哪些引用"是最危险的假设**。按符号扫一遍重定位表，让工具自己找出全部引用，比按结构体字段枚举可靠得多。

**代价**：traceback 里没有函数名（行号仍在，因为 `pctab` 未动）。因此默认关闭，用 `SSA2LLVM_FOLD_FUNCNAMES=1` 打开：

```sh
SSA2LLVM_FOLD_FUNCNAMES=1 bash common/yak/ssa2llvm/scripts/build_yaklib.sh
```

**已验证**：hello world 输出一致；用到模块（`cli`）的脚本正常；数组越界 panic 仍被 yak 运行时捕获并继续执行（§4.3 的验收标准）。

**下一步**：那 14.63 MiB 现在以 `.yakfuncnames` 的形式留在 `.bss` 里（不占文件，只占从未被写过的零页，RSS 影响约等于零）。如果把内联树里的名字偏移也一并清掉，这块 NOBITS 可以缩到一页——那才是真正的"零成本"。

### R2：pclntab 条目级裁剪（收益最大，但我会最后做）

`ftab` + `_func` + funcdata 那 20.20 MiB 里，绝大部分属于机器码已被删除的函数（§5.2 的 60.7%）。理论上可以把这些函数的 `ftab` 条目、`_func` 结构、以及它们独占的 `pctab` 片段一起删掉。

**为什么难**：`ftab` 必须保持按 `entryoff` 排序、`nfunc` 必须与 `pcHeader` 一致、`minpc/maxpc` 必须与首尾项吻合、`funcdata` 里的 GC stackmap 指针必须继续有效——这已经等于**重写一个 pclntab 链接器**。而且失败模式极其隐蔽：改错了不会立刻崩，而是在某次 GC 扫栈或某次 panic 时崩。

**如果要做，建议的中间形态**：不删条目，而是把被裁模块的所有函数**合并到一条"已裁剪"记录**上——保持 `ftab` 结构与排序不变，只回收它们各自的 `_func` 与 `pctab` 片段。这样约束条件少得多，收益也能拿到大部分。

**前置条件**：必须先有 R1 的基础设施（pcHeader 重排 + moduledata slice 改写）和一套真正的回归测试（GC 压力 + panic + `runtime.Callers`），否则不要开工。

### R3：预构建档位 archive（有实测证据的一条路）

§5.3 的 `p1.bin` 已经证明了上限：**同一个脚本，28.91 MiB，函数数从 172,764 降到 41,892。** 因为让 Go 链接器参与后，代码和元数据是一起消失的。

问题只是"不能在用户机器上跑 `go build`"。但这不等于"不能用 Go 链接器"——**可以在 CI 上预先构建若干档位的 `libyak.a`**：

- 定义 3–4 档，例如 `core`（无 yaklib）/ `net`（poc/http/codec/cli）/ `full`（含 ssa 与语言前端）；
- CI 为每档跑一次 `go build -buildmode=c-archive`，Go 的 DCE 在这一步就把代码和元数据一起删干净；
- 编译器按脚本的 used set 选择**能覆盖需求的最小档**，档内再用本文的链接期裁剪做二次收敛。

**分发成本怎么解决**：不必把所有档都塞进编译器二进制。Deno 就是现成的先例——`deno compile` 并不自带运行时，而是按目标平台从 CDN 拉取对应的 `denort` 并缓存（[Deno Docs](https://docs.deno.com/runtime/reference/cli/compile/)）。同样的做法可以让编译器只内嵌最小档，其余档按需下载并缓存，既保住"开箱即用"，又不让二进制线性膨胀。

**R3 与本文方案不是二选一。** 档位解决"元数据"，链接期裁剪解决"档内的模块代码"，两者叠加才是最优解——这也是我现在倾向的终局形态。

### R4：换更小的运行时

如果目标是把量级从"几十 MB"降到"几 MB"，那么在 Go c-archive 这个前提下无解——Go 运行时 + GC + 反射元数据就是这个体量。真正的解法是给纯计算类脚本准备一个不依赖完整 yaklang 运行时的执行路径。Deno 用 QuickJS 替换 V8 把 hello world 从 65 MiB 压到 35 MiB，走的就是这条路。这属于产品层面的决策，不在本文范围内，但值得放在路线图上。

### 综合建议

R0 与 R1 已经做完，两者正交，可以叠加：

| 脚本 | 原始 | +R0 | +R1 | R0+R1 |
| --- | ---: | ---: | ---: | ---: |
| 不用模块 | 85.13 | 84.62 | 70.50 | **69.99** |
| 纯计算（codec/yakit） | 94.50 | 85.62 | 79.87 | **70.99** |

（粗体为实测值，其余按 R1 的 −14.63 MiB 折算。R0+R1 的组合已联合实测：两个脚本都精确落在折算值上，说明两条路线互不重叠——R0 省的是代码，R1 省的是元数据。）

"不用模块"那一行 R0 只省了 0.51 MiB，因为它本来就一个模块都不引用，整个 `shared` 组在拆分前就已经被 GC 掉了；R0 的收益全部落在"用了少数几个模块、却要为整个公共闭包买单"的脚本上。

R0 的收益上限受限于"公共核心已经只剩 1.01 MiB"，继续切下去没有肉了；**下一件该做的是 R3**——它是唯一有实测数据支撑能到 28.91 MiB 的路径，而且与上面两条正交。

**R2 放到最后**，并且只在有了 GC/panic 回归测试之后再动。

---

## 8. 复现

下面用到的分析工具都在 `common/yak/ssa2llvm/cmd/` 下，先一次性构建：

```bash
for t in elfsplit linkbench groupsize moduledeps pclntabinfo pclnrefs; do
  go build -o build/tools/$t.bin ./common/yak/ssa2llvm/cmd/$t
done
```

```bash
# 1. 提取 archive 中的 go.o，查看按模块拆分后的节
llvm-ar x common/yak/ssa2llvm/runtime/embed/assets/libyak.a go.o
llvm-readelf -S --wide go.o | grep -E 'modtext|\.text'

# 2. 受控 A/B：同一个脚本目标文件，只改保留的模块集合
#    （linkbench 直接调用 compiler.CompileObjectToBinarySCWithPatch）
./build/tools/linkbench.bin <script>.o build/linkbench \
  'pruned=' \
  'shared=shared' \
  'nonssa=os,poc,cli,http,codec,yakit,shared' \
  'full=os,poc,cli,http,codec,yakit,ssa,shared,ssafront'

# 3. 产物体积构成（排除 NOBITS）
llvm-readelf -S --wide pruned.bin | sed 's/^ *\[ *[0-9]*\] *//' \
  | awk 'NF>=6 && $2=="PROGBITS" {printf "%-20s %12d\n", $1, strtonum("0x" $5)}' \
  | sort -k2 -rn

# 4. 行为验证
env -i ./pruned.bin
env -i GOTRACEBACK=all ./panic_probe.bin

# 5. 拆解 .gopclntab 子表 + 按包前缀统计函数分布
#    （解析 pcHeader，区分"仅符号化"与"运行时必需"）
./build/tools/pclntabinfo.bin pruned.bin full.bin p1.bin

# 6. R1：带函数名折叠重建 archive，再比对体积
#    注意 libyak.a 是 go:embed 进编译器的，改完 archive 必须重建编译器
SSA2LLVM_FOLD_FUNCNAMES=1 bash common/yak/ssa2llvm/scripts/build_yaklib.sh
go build -o build/tools/ssa2llvm ./common/yak/ssa2llvm/cmd/ssa2llvm
./build/tools/ssa2llvm compile hello.yak -o fold.bin

# 7. 列出所有指向 runtime.pclntab 的重定位（折叠前必须先看这个）
./build/tools/pclnrefs.bin go.o

# 8. R0：看某个模块组的字节分布在哪些包上（判断"能不能整块切走"）
./build/tools/groupsize.bin go.o .modtext.shared 4

# 9. R0：按"哪几个模块的闭包包含这个包"给包分类，并生成分组表
#    MODULEDEPS_FILTER 限定只有当前 shared 里的包可以被移动
#
#    ⚠️ 这一步必须喂"未拆分"的 go.o。分组表一旦生成并生效，.modtext.shared
#       里就只剩核心那 1 MiB 了，拿拆过的产物再跑一遍会得到一个不断塌缩的
#       分组。要重新生成，先用旧分组重建 archive：
#       AOT_SPLIT_MODULES='os,poc,cli,http,codec,yakit,ssa,shared,ssafront' \
#         bash common/yak/ssa2llvm/scripts/build_yaklib.sh
GROUPSIZE_PACKAGES=1 ./build/tools/groupsize.bin go.o .modtext.shared 20 > shared_allowed.txt
MODULEDEPS_FILTER=shared_allowed.txt \
MODULEDEPS_GO=common/yak/ssa2llvm/cmd/elfsplit/sharedgroups_generated.go \
  ./build/tools/moduledeps.bin \
    aotlib=github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/aotlib \
    cli=github.com/yaklang/yaklang/common/utils/cli \
    poc=github.com/yaklang/yaklang/common/utils/lowhttp/poc \
    http=github.com/yaklang/yaklang/common/yak/yaklib/yakhttp \
    yakit=github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/shim \
    ssa=github.com/yaklang/yaklang/common/yak/ssaapi

# 10. R0 的 A/B：纯计算脚本 vs 用网络的脚本（改完闭包规则必须重建编译器）
go build -o build/tools/ssa2llvm ./common/yak/ssa2llvm/cmd/ssa2llvm
./build/tools/ssa2llvm compile t_codec_only.yak -o codec.bin   # 85.62 MiB
./build/tools/ssa2llvm compile t_http.yak       -o http.bin    # 94.50 MiB

# 11. R0 + R1 联合（当前树默认已含 R0，再打开 R1 开关即可）
SSA2LLVM_FOLD_FUNCNAMES=1 bash common/yak/ssa2llvm/scripts/build_yaklib.sh
go build -o build/tools/ssa2llvm ./common/yak/ssa2llvm/cmd/ssa2llvm
./build/tools/ssa2llvm compile t_codec_only.yak -o both.bin    # 70.99 MiB
```

**环境**：linux/amd64，Go 1.26.5（GOEXPERIMENT `nodwarf5`），LLVM 22.1.8，分支 `feature/ssa2llvm/shrik_yaklib_size` @ `7303582dc`。

---

## 9. 结语

这套方案的链条是：

> **问题**：完整 c-archive 无法按脚本裁剪
> → **根因**：会做 DCE 的 Go 链接器在构建 archive 时就下班了，链接期只剩粒度不够的 lld
> → **构建期**：elfsplit 按模块拆节，把 lld 缺的粒度补出来
> → **链接期**：重定位指 stub、inittask 标 done、textmap 清引用，让 `--gc-sections` 真的敢删
> → **链接后**：排序 textsectmap、修 ftab/minpc/maxpc、收敛保留集
> → **结果**：22.4%（24.54 MiB），链接耗时 1.2–1.6 s，行为/内容/体积三层证据齐备

但更值得写下来的是它的**边界**，以及边界之外还有多少空间：

- 产物仍有 85 MiB，其中 69.8% 是元数据。**我们删掉了代码，却把描述这些代码的元数据一字不落地留下了**——一个 hello world 里，60.7% 的函数元数据属于 TypeScript/PHP/Java/Python 语法分析器。
- 这里面 **14.63 MiB 是纯符号化数据**，删掉不影响 GC 与 panic。这一条已经实现并实测：84.62 → **69.99 MiB**（R1）。
- 模块分组本身也留了钱：一个只做编解码的脚本原本要为 9 MiB 的公共依赖闭包买单，按 `go list -deps` 的依赖类重切之后是 **94.50 → 85.62 MiB**（R0）。
- 两条叠加起来是正交的，联合实测把同一个编解码脚本从 **94.50 MiB 压到 70.99 MiB（−24.9%）**——R0 省代码、R1 省元数据，互不重叠。
- 而同一个脚本走"让 Go 链接器参与"的路径，实测只要 **28.91 MiB**（R3）。所以 85 MiB 从来不是物理下限，只是这套链接期方案的边界。

**结论不是"到此为止"，而是"到此为止的是这一种方法"。** 链接期裁剪解决的是"档内的模块代码"，元数据要靠让 Go 链接器提前参与来解决，两者正交、可以叠加——这才是终局形态。

对于任何打算给 Go 二进制瘦身的人，有四条经验可能比本文的具体方案更有用：

1. **先量一量你的产物里到底有多少是代码。** 我们花了大量精力做代码裁剪，事后才量清楚代码只占五分之一。
2. **优先想办法让 Go 自己的链接器参与。** 它删函数时会连 pclntab 条目、类型信息、字符串一起删；任何后处理工具都做不到这一点，只能追着补。
3. **分组不要手写，让依赖数据来定。** `shared` 那 10 MiB 摊在 139 个包上，没有可以整块切走的东西；而 `go list -deps` 一跑，"哪几个模块需要这个包"的等价类立刻把边界画了出来，还顺带保证了"依赖一定和使用者同生共死"这条不变式。
4. **让错误在构建期、带着名字发生。** 这套方案里每一个真正难查的 bug——静默返回的 stub、init 期调用被裁掉的组、被缓存掩盖的闭包改动——都是因为失败发生得太晚或太安静。补上 per-module stub、启动路径可达性闸门、链接前的模块存在性检查之后，同类问题现在都是一条带路径的构建错误。
