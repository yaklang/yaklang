# Reify 设计与算法

本文说明当前 Reify 的实现：三层表示如何接力，`asm2lowir`、`lowir2ssa`、`ssa2source` 各自用什么算法，以及 `loader` / `arch` 为什么这样拆分。
格式契约见 [LowIR](lowir.md) 与 [SSA](ssa.md)，分派轴见 [架构](architecture.md)，本文不重复字段清单，重点写数据怎么变、判断依据是什么、边界在哪里。

## 1. 总体设计

### 1.1 一条流水线，四个稳定契约

```text
ELF / PE / Mach-O
        │  Loader：只解析文件格式
        ▼
     Artifact
  Reader + Image + Symbols + BoundaryHints + Target
        │  asm2lowir：只编排边界、区域和命名
        ▼
      LowIR
  机器精度、显式寄存器与内存效果
        │  lowir2ssa：只做语义降低
        ▼
    YakSSA Program
  变量、值、基本块、If / Jump / Phi
        │  ssa2source：只做结构化与打印
        ▼
   伪源码文本
```

四个契约各自独立：

| 契约 | 位置 | 责任 | 不负责 |
| --- | --- | --- | --- |
| `Loader` | [core/loader.go](../core/loader.go) | 识别格式、解析头部、节、符号、重定位 | 解码指令、划分函数 |
| `Parser` | [core/parser.go](../core/parser.go) | 把一段机器码变成 LowIR | 打开文件、理解节表 |
| `LowIR` | [core/lowir.go](../core/lowir.go) | 保存机器精度与执行效果 | 语言类型、调用图 |
| `ssa.Program` | `common/yak/ssa` | 承载现有分析生态 | 保留原始机器效果 |

这样拆分的目的不是“分层好看”，而是让每一层都能单独替换、单独测试：

- 新增文件格式只改 `loader/fileformat`。
- 新增指令集只改 `loader/arch`。
- 新增机器级化简只改 `core/lowir2ssa/pass`。
- 新增类型或源码级恢复只改 `core/ssa2source/pass`。
- 三层都能用文本 fixture 固定输出，不依赖真实二进制。

### 1.2 三个不变量

1. **不执行输入。** 所有结论来自静态解码、模式匹配和数据流近似。
2. **不静默丢弃。** 未建模的指令变成显式 intrinsic、opaque 标记或 diagnostic；转换不会把未知操作当成纯计算。
3. **来源可追溯。** LowIR 指令带机器地址；SSA pass 和文本渲染都保留原始值 ID 与地址常量注释。

---

## 2. asm2lowir：机器码到 LowIR

实现位于 [core/asm2lowir](../core/asm2lowir)。它不包含任何 ISA 解码表；解码由 `Artifact.Parser` 完成。

### 2.1 输入与输出

```go
type Result struct {
    IR            *core.LowIR
    Assembly      string
    Diagnostics   []core.Diagnostic
    Boundaries    []core.FunctionBoundary
    BoundaryStats core.BoundaryStats
}
```

输入是已经打开的 `Artifact`。`Artifact` 里同时有：

- `Reader`：按目标字节序读取、带范围检查的 `ByteReader`。
- `Image`：节、段、入口、符号。
- `Hints`：loader 收集的函数边界线索。
- `Parser`：按 `Target.Architecture` 选中的解析器。

`asm2lowir` 只做四件事：选边界、切区域、逐函数调用 parser、补名字与诊断。

### 2.2 区域选择与边界发现

`Parse` 先按选项决定“看哪一片代码”，再决定“片内有哪些函数”。

| 选项 | 选择的区域 | 边界来源 |
| --- | --- | --- |
| `WithFunction(name)` | 符号的 `Address/Size` | 该符号本身 |
| `WithRegion(region)` | 调用方给定 | parser 发现的边界过滤到区域内；没有则整段当一个函数 |
| `WithRegion + WithExact` | 调用方给定 | 整段恰好一个函数，不再发现 |
| `WithSection(name)` | 节的 `FileSize` | parser 发现的边界过滤到节内；没有则整节 |
| `WithAll()` | 所有可执行节 | parser 发现的全部边界；没有则每个可执行节一个边界 |

`discover` 的优先级很直接：

```text
指定函数名  ──► 符号查找
指定区域    ──► Exact ? 单边界 : parser 发现 → 过滤 → 回退单边界
指定节      ──► parser 发现 → 过滤 → 回退整节
全部        ──► parser 发现 → 回退所有可执行节
```

parser 通过可选接口 `BoundaryDiscoverer` 参与：

```go
type BoundaryDiscoverer interface {
    DiscoverBoundaries(reader *ByteReader, hints BoundaryHints, image *Image) (BoundaryResult, error)
}
```

如果 parser 不支持该接口，或发现过程报错，`asm2lowir` 回退到符号表里的本地函数符号。这个回退保证“没有边界发现器也能出结果”。

### 2.3 边界到文件区域

边界主要提供虚地址，大小可能为零，文件偏移也还没有。`boundaryRange` 按顺序决定大小：

1. 边界自带 `Size` 就用它。
2. 否则用下一个边界的地址减去当前地址。
3. 最后回退到所在可执行节的末尾。

随后 `boundaryRegion` 把虚地址映射到文件偏移，并检查是否越出当前选择范围。映射失败或区域为零长度时记一条 diagnostic 并跳过，不让一个坏函数终止整个文件：

- `empty-function`：边界没有文件支撑的字节。
- `boundary-region`：边界落在所选区域之外。

### 2.4 每个函数一次 ParseRequest

对每个边界：

```go
parsed, err := artifact.Parser.Parse(ctx, core.ParseRequest{
    Target:       artifact.Target,
    Address:      region.Address,
    Reader:       reader,          // 区域受限的视图
    Syntax:       config.Syntax,
    WithAssembly: config.WithAssembly,
})
```

然后给返回的每个 `Function` 重新编号，并写入 `Address`、`Size`、`Boundary`、`Name`：

- `ID` 在一个 LowIR 内全局递增。
- 块 ID 和值 ID 由 parser 在函数内生成。
- 边界未提供名字时，后续统一补 `sub_0x<地址>`。

逐函数解析有一个明确的工程收益：峰值内存由“最大函数”决定，而不是整个文件。注释里写得很清楚：`asm2lowir issues one request per function boundary, so memory scales with the largest function, not the file.`

代价是跨函数信息不可见。每个函数单独解码时，跳转到另一个函数会被视为“区域外目标”。因此当边界多于一个时，`filterCrossFunctionDiagnostics` 会过滤掉预期内的噪声：

- `branch-target-outside-region`
- `lowir2ssa-incomplete-branch`

这些诊断在单函数模式下仍然保留，因为它们可能代表真实问题。

### 2.5 命名与符号

两步收尾：

1. `annotateSymbols`：用 `Image.Symbols` 给函数和调用目标补符号名。直接调用常量地址如果命中符号，就从 `sub_0x...` 变成真实名字。
2. `fillFunctionNames`：仍为空的名字统一为 `sub_0x<entry>`。

`Disassemble` 复用同一套边界选择，只把 `Parse` 换成 parser 的可选 `Disassembler`。因此汇编文本和 LowIR 的函数切分一致，不会出现“反汇编看到 10 个函数、LowIR 只有 3 个”的错位。

### 2.6 parser 内部的块构造

`asm2lowir` 负责函数级切分，基本块切分在 parser 内完成。以 AMD64 为例：

1. 线性解码整个函数区域，得到地址到指令的映射。
2. 收集块首地址（leader）：函数入口、直接跳转/分支目标、条件跳转或 `jmp` 的下一条指令、终止指令的下一条指令、落在区域内的直接调用目标。
3. 按 leader 把线性指令序列切成基本块。
4. 目标落在区域内但不在指令边界时记 `branch-target-misaligned`；落在区域外时记 `branch-target-outside-region`。

parser 返回的 `LowIR` 通常只有一个函数，`asm2lowir` 再把它和边界信息合并。ARM64 使用同样的思路，只是解码器和控制转移判断不同。

### 2.7 结果形状

`asm2lowir` 的输出有明确的最小保证：

- 至少一个函数。
- 每个函数至少一个块。
- 每个块以控制转移结束是 LowIR 约定，不是 `asm2lowir` 的强制后处理；当前 AMD64/ARM64 lifter 会在块缺少终止符时补 `jump` 或 `trap`。
- `Instruction.Address` 指向来源机器指令；一条机器指令可产生多条 LowIR 指令，它们共享地址。

---

## 3. lowir2ssa：LowIR 到 YakSSA

实现位于 [core/lowir2ssa](../core/lowir2ssa)。它解决的是一个表示错配问题：

- LowIR 里寄存器、内存、标志位都是显式读写，指令有副作用。
- YakSSA 里需要“值”和“变量”，控制流是基本块，数据流用 SSA 值表达。

转换器的定位是**变量物化（variable materialization）**，不是完整反编译。它把机器状态映射成 SSA 变量，把可证明的纯计算映射成 SSA 值，把不能建模的部分保留成可见标记。

### 3.1 转换模型

| LowIR | YakSSA |
| --- | --- |
| 纯值结果（算术、比较、转换） | `ssa.Value` |
| 通用寄存器 | 一个规范 64 位变量，读当前值、写重新赋值 |
| 内存槽 | 按地址结构键共享的变量 |
| `rflags` | 伪变量 + intrinsic 记录 |
| 直接调用 | `ssa.Call` + extern 函数桩 |
| intrinsic / 未知操作 | 带标记的字符串常量，强制转换为结果类型 |
| `jump` / `branch` / `return` / `trap` | `Jump` / `If` / `Return` / `Panic + Return` |

关键取舍：**寄存器的合流点插 Phi**，按支配边界构造；内存槽仍用变量赋值表达。

- 寄存器：`planMergePhis` 收集每个寄存器被写入的块，求迭代支配边界，在每个汇合块插入一个 Phi，边数与基本块前驱数相同。填边在全部块转换完成后进行，按前驱顺序取“该前驱出口的值”。没有这一步，合流处只能取“线性顺序中最后一次写”，值会来自未执行的分支。
- 内存：仍用变量赋值。内存 SSA 需要别名分析，用“复制返回尾声”“折叠返回尾”等 pass 消除常见歧义。
- 标志寄存器是例外：条件读取按“比较值 id”查找比较信息，不参与合流 Phi。

### 3.2 两遍扫描

`convertFunction` 的顺序是固定的：

```text
建 SSA 函数与全部基本块
  → 收集每个 ValueID 的定义指令
  → 标记循环头
  → 收集循环携带的内存键
  → 在入口块绑定参数
  → 按 LowIR 块顺序逐条降低
  → Finish()
```

先建块再填指令，保证 `jump`/`branch` 可以前向引用尚未转换的块。
`defs` 映射让后面的地址结构键、flag 记录和返回值推断都能回溯定义。

### 3.3 值的降低

`convertInstruction` 是一张显式分派表，没有反射，也没有接口调用：

| LowIR | YakSSA |
| --- | --- |
| `constant` | 常量，类型来自 `ValueType` |
| `copy` | 同宽直接复用；跨宽或跨种类生成 `TypeCast` |
| `add/sub/mul` | `OpAdd/OpSub/OpMul` |
| `udiv/sdiv` | 统一 `OpDiv` |
| `and/or/xor/shl/lshr/ashr` | 对应二元操作 |
| `eq/ult/slt` | `OpEq/OpLt` |
| `neg/not` | `OpNeg/OpBitwiseNot` |
| `zext/sext/trunc` | `TypeCast` 到结果类型 |
| `phi` | 记录 diagnostic，取第一个输入 |
| `select` | opaque 标记（`cmov` 的语义不在此处展开） |
| 未支持操作 | opaque 标记 + `lowir2ssa-unsupported-op` |

`opaqueValue` 的实现值得注意：它不是一个假调用，而是“字符串常量 + 强制类型转换”。这样标记在 SSAIR 和源码里都可见，也不会被误当成有副作用的函数调用。

### 3.4 寄存器视图与部分写

#### 规范寄存器

`registerViewFor` 把 `al/ax/eax/rax` 统一到 `rax`，并记录：

```go
type registerView struct {
    canonical     string // rax
    width         uint16 // 8 / 16 / 32 / 64
    shift         uint16 // 位偏移
    canonicalBits uint16 // 64
}
```

- 读 `al`：`trunc(rax >> 0, 8)`
- 读 `ah`：`trunc(rax >> 8, 8)`
- 写 `eax`：按 x86 规则零扩展到 64 位。
- 写 `al`：读-改-写，保留高 56 位。

#### 部分写算法

宽度小于 32 的写不能直接赋值，否则会清掉同一规范寄存器的其他位。实现是标准的掩码合并：

```text
mask     = (1 << width) - 1
cleared  = current & ~mask
inserted = (value << shift) & mask
next     = cleared | inserted
```

三条保护：

1. 当前值不存在时，先创建一个 `undefined` 占位值，避免读到空。
2. 写入值本身是 `undefined` 时直接返回。YakSSA 在变量被真实值替换时会删除占位值；若把占位值赋给变量，早先的使用者会留下悬空引用。
3. 变量统一在入口块创建，保证定义支配所有使用。

这部分是 `lowir2ssa` 里最容易出错、也最值得单独测试的逻辑；fixture 目录专门有 `partial-register` 用例。

### 3.5 内存槽与地址结构键

LowIR 的 `load/store` 只给一个地址值，没有“这是哪个局部变量”的信息。转换器用 `valueKey` 构造结构键：

```text
const:0x10
reg:rbp
add(reg:rbp,const:fffffffffffffff8)
intrinsic:x86.segment.fs(const:28)
v17                      // 其他无法结构化的表达式
```

规则：

- `constant` → `const:<位模式>`
- `copy` → 被复制值的键
- `add` → `add(左键,右键)`
- `read_register` → `reg:<规范名>`
- `intrinsic` → `intrinsic:<符号>(参数键...)`
- 其他 → `v<ID>`

递归深度限制为 16，并带访问集合防止环。递归键让 `[rbp-8]` 的多次访问共享一个变量，即使每次地址值 ID 不同。

`memoryName` 把键映射成变量名：

- `const:` 键 → `mem_<常量>`
- 其他键 → 递增的 `mem_N`

这是**语法等价**，不是别名分析。两个不同表达式即使实际指向同一地址，也会得到两个变量；同一个表达式重复出现则共享变量。这个边界必须在实现新优化时记住：跨内存访问做变换需要额外证明。

当槽还没有当前值时，`emitLoad` 生成 `load:<地址值ID>` 标记常量，并把地址值登记为标记的使用者。这个“使用者边”让 `infer-types` 能把地址标成指针，源码渲染也能还原成：

```c
*(uint64_t *)(v12)
```

### 3.6 调用与参数捕获

#### 目标

`emitCall` 按操作数种类决定目标：

- 常量地址 → `sub_0x<地址>`
- 符号 → 符号名
- 其他值 → 间接调用目标

直接目标会创建一个 `IsExtern` 的函数桩。桩没有函数体，文本渲染会跳过它，但 `Call` 指令本身是真实的。

#### 参数

参数由两部分拼接：

1. `Instruction.Operands[1:]` 里显式列出的参数。
2. `captureArguments()` 捕获的寄存器参数。

捕获算法按 `Target.Architecture` 选择寄存器组：

```text
AMD64: names = [rdi, rsi, rdx, rcx, r8, r9], 返回值写 rax
ARM64: names = [x0..x7],                       返回值写 x0
last  = 自上次 call 以来被写过的最高槽位
args  = names[0..last] 的当前值
缺少的低位槽补 undefined
清零“已写”标记
```

只捕获到最高写过的槽位，是为了避免把无关的寄存器值全部塞进调用。显式操作数在前、寄存器参数在后，对应 lifter 已经写出的调用形状。

返回值按 ABI 写入返回寄存器：

```go
c.setResult(instruction, call)
c.assignRegister(c.returnRegister(), call)
```

`Variadic` 标记保存在 `Call.VerboseName()` 里，供后续 arity 裁剪跳过可变参数调用。

#### 边界

- 只实现 System V AMD64 与 AAPCS64 的整数参数寄存器；栈上传参和浮点寄存器未捕获。
- 调用后的 caller-saved 寄存器没有统一失效；后续写入会覆盖，未写入则沿用旧值。
- 间接调用没有调用图恢复，只保留目标值。

### 3.7 标志位与条件恢复

机器码的 `Jcc` 读 `rflags`，而不是直接读比较结果。如果直接降低，条件会变成“rflags 非零”，源码几乎不可读。转换器用两步恢复：

1. **记录**：lifter 把算术/比较/测试写成 `x86.flags.<op>` intrinsic，操作数是参与运算的值。转换器把它记成：

   ```go
   type flagInfo struct {
       name     string          // cmp / test / add / ...
       operands []core.ValueID
   }
   ```

2. **还原**：`x86.cond.<cc>` 查询 `rflags` 对应的记录：
   - `cmp`：直接用两个操作数生成比较。
   - `test`：先 `and`，再与 0 比较。
   - 其他：取最后一个操作数与 0 比较。
   - `s/ns`：与 0 比较后取反。

条件码映射：

| cc | 操作 |
| --- | --- |
| `e/z` | `==` |
| `ne/nz` | `!=` |
| `l/b/c` | `<` |
| `le/be` | `<=` |
| `g/a` | `>` |
| `ge/ae` | `>=` |

`JCXZ/JECXZ/JRCXZ` 不走 flags，lifter 直接生成计数寄存器与 0 的比较。

当前实现有一个已知近似：`l` 与 `b`、`g` 与 `a` 都映射到同一个比较操作，没有区分有符号与无符号语义。这能让源码保持可读，但会在极端边界值上偏离机器语义。写新的比较规则时必须显式处理这一点，不能默认 `OpLt` 就是无符号。

### 3.8 控制流

- `jump`：有一个块内目标就 `EmitJump`；间接跳转或没有目标时不伪造目标，直接结束当前块。
- `branch`：true 目标在前，false 目标在后。两个目标都已知时生成 `If`；只有一个目标时生成无 else 的 `If`，并记 `lowir2ssa-incomplete-branch`。
- `return`：显式操作数为空时取当前 `rax`。
- `trap`：生成 `Panic(原因符号)` 后再 `EmitReturn(nil)`。
- 每个块结束时调用一次 `EmitReturn(nil)` 作为兜底；已经有终止指令的块会忽略它。

这保证即使输入被截断，CFG 仍然是完整的，SSA 构造不会遇到“没有终止符的块”。

### 3.9 循环与循环携带变量

合流 Phi 由支配边界统一构造（见 3.1）。本节描述循环携带值的额外处理：内存槽还需要先找出“在循环里既被读又被写”的键。

#### 循环头识别

`markLoopHeaders` 只看块在函数中的线性顺序：

```text
对每条 jump/branch 的每个目标
  若目标块的位置 <= 当前块的位置
  则目标是循环头
```

这是“回边”近似，不计算支配关系。前置条件是 `reverse-postorder` 已经重排过块，否则头块可能排在后边。

#### 循环携带的内存键

`collectLoopCarried` 对每个循环头做：

1. 找到所有回到该头的边，取最后一个回边块的位置作为循环范围终点。
2. 扫描 `[header, last]` 内所有 `load` 和 `store` 的地址结构键。
3. 取两者的交集：既被读又被写的槽，才需要循环携带值。

只有内存需要预先收集；寄存器在循环头被读到时会按需建 Phi。

#### Phi 的创建与完成

```text
prepareLoopPhis(header)
  → 对每个循环携带的内存键创建 Phi
  → 初值边 = 进入循环前的变量值

循环体内读寄存器
  → 若当前是循环头，创建/复用 "reg:<name>" Phi

遇到回边
  → fillLoopCarried 把当前变量值追加为 Phi 的边
  → 把 Phi 赋回变量
```

两个防重复条件：

- `headerStore`：如果循环头本身已经写过该键，就不再建 Phi。头部的写覆盖了进入值，Phi 没有意义。
- `loopPhiDone`：同一个 Phi 只补一次回边值。

这套算法能处理常见的 `i++`、指针遍历、累加器。寄存器合流点另有支配边界构造（见 3.1），本节只补循环携带的内存槽：

- 寄存器合流 Phi 由 `planMergePhis` 覆盖；循环头同时是合流点。
- 只识别自然循环式的回边；不可归约循环、间接跳转、异常边无法可靠恢复。
- 循环嵌套依赖块顺序和递归处理，不保证所有形状都能还原。

### 3.10 类型映射

`typeFor` 只做机器精度到 SSA 类型的映射：

| LowIR `ValueType` | YakSSA 类型 |
| --- | --- |
| `integer/64` | `i64` |
| `integer/32` | `i32` |
| `integer/0` | `any` |
| `pointer/64` | `ptr64` |
| `pointer/0` | `ptr64` |
| `float/32` | `f32` |
| `float/64` | `f64` |
| `opaque` | `any` |

整数符号不在类型里，而在操作里；这与 LowIR 的设计一致。类型只用于打印、参数渲染和后续指针推断，不参与值语义判断。

### 3.11 LowIR pass 管线

默认顺序：

```text
remove-canary → duplicate-return-blocks → remove-stackframe
→ fold-return-tails → drop-unreachable-blocks → reverse-postorder
```

每个 pass 都只改 `core.Function`，不碰 SSA。

#### 架构事实：`core.ArchFacts`

pass 不按寄存器名字符串猜测架构。parser 把自己架构的事实以数据形式写进 `core.LowIR`，`lowir2ssa` 通过 `ir.Facts()` 交给每个 pass：

| 字段 | 内容 | 谁填 |
| --- | --- | --- |
| `Frame.StackPointer` | 栈指针寄存器名（`rsp`、`esp`、`sp`、`r15`、`r1`、`r3`） | 各架构 lifter |
| `Frame.FramePointer` | 帧指针寄存器名（可为空） | 同上 |
| `Frame.ReturnAddress` | 链接寄存器（`lr`、`x30`、`ra`、`r14`、`r1`） | 同上 |
| `Frame.Flags` | 标志伪寄存器 | 同上 |
| `Frame.CalleeSaved` | 被调用者保存寄存器 | 同上 |
| `Frame.StackGrowth` | 栈增长方向 | 同上 |
| `Registers.Naming` | 寄存器命名规则：`x86`（部分寄存器共享一个变量）、`aarch64`（Wn/Xn 共享）、`plain`（每个名字一个寄存器） | 同上 |
| `Registers.Width` | 默认整数寄存器宽度 | 同上 |
| `Registers.Arguments` / `Return` | 整数参数寄存器顺序、返回值寄存器 | 同上 |
| `Canary.Kind` / `Register` / `Offset` | 栈保护值从哪里读 | 同上 |

同一份事实也决定语义标记的命名：lifter 只写中立命名空间 `tls.base`、`flags.*`、`cond.*`，不写架构前缀。canary 的读取形态本身是架构知识（x86 是 `fs:0x28`，arm64 是 `tpidr_el0+0x28`，RISC 类目标经 GOT 取 `__stack_chk_guard`），所以由 lifter 识别并发出 `tls.base`，pass 只处理中立形态。

`internal` 只放被复用的实现；单变体架构（arm、arm64、s390x、loong64）直接在自己的包里。

每个 parser 实现 `core.ArchFactsProvider`，`Facts()` 一次性给出 frame、register 和 canary 三份事实。`asm2lowir` 把它们写到 `core.LowIR`，`lowir2ssa` 用它绑定入口寄存器、参数和返回值。`lowirtext` 以 `frame:`、`registers:`、`canary:` 三行承载同样的内容，所以文本往返不丢事实。

`lowir2ssa` 不再有按架构分支：入口寄存器由 `FrameModel.EntryRegisters()` 推导，参数与返回寄存器读 `Registers`，寄存器别名规则读 `Naming`。

#### `remove-canary`

识别 stack protector 模式，架构中立：

1. `tls.base` → 栈保护值；偏移取自 `ArchFacts.Canary.Offset`。两种形态：段基址把偏移折进标记（x86），线程指针在 `add` 里加偏移（arm64）。
2. canary 写入栈槽（store）。
3. 返回前从同一槽读回（load）。
4. 与 canary 比较（`sub/xor/cmp`）。
5. 比较结果经 `flags.*` / `cond.*` 进入分支。
6. 分支一侧是不返回的失败处理块（含 `call` 或 `trap`）。

命中后：

- 删除 canary 的 store、比较、flags/cond、检查分支。
- 把检查分支改写为直接跳到成功块。
- 删除只写 canary 的寄存器赋值。
- 做一次死代码删除和不可达块删除。

模式不完整时函数保持原样。这比“看到 `fs:0x28` 就删”安全，因为 TLS 访问也可能用于其他目的。

#### `duplicate-return-blocks`

目标：消除共享返回尾声带来的“最后一次写覆盖分支值”。

条件：

- 块以 `return` 结束。
- 块内没有 `call/store/branch/jump/trap`，即纯尾声。
- 至少两个无条件前驱。

动作：把尾声复制进每个前驱，删除原块。循环执行到没有可复制的尾声为止。

#### `remove-stackframe`

按“同一机器地址的指令组”分组，用 `ArchFacts.Frame` 的寄存器名识别：

- 被调用者保存寄存器或返回地址压栈。
- 帧指针建立（`mov rbp, rsp`、`mov x29, sp`、`mov r11, sp`）。
- 栈指针减去立即数的栈分配。
- 拾取帧指针的 `leave`。
- 弹栈恢复。
- 栈指针加回立即数的栈释放。

同一个实现服务 13 种架构，因为寄存器名来自 `FrameModel` 而不是字符串常量。删除序言和尾声，并给被删指令打 `EffectFrameRestore`。它**不重写帧指针相对局部变量**，所以删除后地址可能失去基址。随后的转换阶段按“是否启用了该 pass”在函数上记录：

- `FrameCleanup`：启用了框架清理。
- `FrameKept`：未启用，保留框架。

这两个标志会影响源码清理阶段是否做死赋值删除。

#### `fold-return-tails`

递归判断一个块是否“只会返回”，判断结果缓存并带访问集合防环：

- 以 `return` 结束，且块内没有会返回的调用；`abort/exit/_exit` 等不返回的调用不算。
- 以 `jump` 跳到 return-like 块，且块内没有会返回的调用。
- 以 `branch` 跳到两个 return-like 块。

然后对每条指向这类尾块的 `jump`，再确认尾块子图内没有回到自身的回边（`hasBackEdge`），才替换成 `return rax`。这样每个分支都有自己的返回，结构化和可读性都更好。

#### `reverse-postorder`

从入口 DFS，`branch` 的后继顺序反转（false 先走），得到后序再反转。未访问块按原顺序追加。

目的不是优化布局，而是保证循环头先于循环体出现。`markLoopHeaders` 和 `ssa2source` 的结构化都依赖这个顺序。

---

## 4. ssa2source：YakSSA 到伪源码

实现位于 [core/ssa2source](../core/ssa2source)。输出目标是**可检查的伪代码**，不是可编译源码：

- 保留 `vN` 值名和基本块结构痕迹。
- 保留隐式调用、load 标记和未知操作。
- 用 C 风格类型和表达式降低阅读成本。
- 不承诺语义等价于原始语言。

### 4.1 结构化算法

`structurer` 先建立 CFG 索引：

```go
blocks map[int64]*ssa.BasicBlock
order  []int64
succs  map[int64][]int64
preds  map[int64][]int64
```

后继优先取 `block.Succs`，为空时从最后一条 `If` / `Jump` 推导。

#### 支配与后支配

两者都是迭代数据流不动点：

```text
dom[entry] = {entry}
dom[n]     = {n} ∪ ⋂ dom[p]        p ∈ preds(n)

pdom[n]    = {n} ∪ ⋂ pdom[s]       s ∈ succs(n)
pdom[exit] = {exit}                // 虚拟出口 0
```

后支配引入虚拟出口 `0`，让多个 return 块有共同的后继。`ipdom[n]` 取后支配集合中“最大”的那个（集合大小最大），作为分支合流点。

这是结构化 `if/else` 的核心：**合流点 = 直接后支配点**。

#### 自然循环

对每条边 `tail → header`：

```text
若 header 支配 tail，则 header 是循环头，tail 是回边
循环体 = 从 tail 出发沿前驱反向可达的块集合
出口   = 循环体内某块指向体外的第一个后继
```

只识别自然循环。不可归约循环没有“头支配回边”的性质，会退化成普通分支输出，甚至需要前向声明兜底。

#### 区域发射

```go
emitRegion(id, indent, stop)
```

`stop` 是“不许进入”的块集合，典型成员是合流点、循环出口、循环头。`emitted` 防止重复输出；`rendered` 允许同一个块在必要时输出两次（`force`），用于分支落空后的重复发射。

主循环：

```text
遇到循环头 → emitLoop
遇到 If    → 先输出条件前的指令，再 emitIf
遇到 Jump  → 输出块内指令，再 emit 目标
其他       → 输出块内指令，再 emit 第一个后继
```

#### `if/else` 的 join 校验

`join = ipdom[block]`，但不是拿到就用。以下任一情况把 join 置 0：

- join 已经在 `stop` 里。
- join 已经输出过。
- true 或 false 分支无法在不穿过 stop、循环头、return、panic 的情况下到达 join。

join 有效时：

- `true == join`：输出 `if (!(cond)) { false 分支 }`。
- `false == join`：输出 `if (cond) { true 分支 }`。
- 其他：输出完整 `if/else`。
- 最后继续输出 join。

join 无效时：直接输出完整 `if/else`，不再输出合流后的代码。这避免把不可结构化 CFG 硬塞进错误缩进。

#### `while` 的发射

`emitLoop` 从循环头的 `If` 判断哪一侧在循环体内：

- true 在体内、false 在体外 → `while (cond)`，false 是出口。
- false 在体内、true 在体外 → `while (!(cond))`，true 是出口。
- 两侧都在体内 → 取 true 侧作为循环体入口，仍输出 `while (cond)`；出口使用自然循环分析找到的 `loop.exit`。
- 头块没有 `If`，也找不到体内后继 → `while (true)`。

发射顺序：

```text
循环头 Phi（进入值）
循环头指令（跳过条件指令）
while (...) {
    循环体，stop = {header, exit}
    回边 Phi（回边值）
}
出口块
```

Phi 渲染成普通赋值。循环携带 Phi 的边多于 1 条时，进入时取第一条边，回边时取最后一条边。
Phi 按“边数从多到少”排序，保证循环携带值先于可能引用它的单边别名输出。

### 4.2 指令渲染

`renderInstructionAt` 只处理非控制流指令：

| SSA | 输出 |
| --- | --- |
| 常量 | 在使用点内联 |
| 类型转换 | 在使用点内联 |
| 二元/一元 | `vN = expr;` |
| 调用 | `vN = f(args);` 或 `f(args);` |
| SideEffect | `vN = side-effect(...);` |
| Undefined | `vN = undefined;` |
| Parameter | `vN = parameter;` |
| Panic | `panic(reason);` |
| Return | `return ...;` |
| Phi/Jump/If | 交给结构化器 |

细节：

- 没有使用者的值不输出，减少噪声。
- `vN` 是程序全局 ID；最后统一重编号成函数内连续 ID。
- 二元表达式做恒等式折叠：`x^x=0`、`x-x=0`、`x&x=x`、`x|0=x`、`x+0=x`。
- 常量注释保留：`0x402000 /* "hello" */`。
- load 标记输出成 `*(T *)(address)`，而不是一个字符串常量。
- 类型名使用 C 风格：`uint64_t`、`int32_t`、`void *`、`float`、`double` 等；`i1` 输出 `bool`。无法确定的类型退回 `uint64_t`。

### 4.3 文本清理

`cleanSource` 解决结构化输出常见的四类瑕疵。

#### 1. 框架基址别名内联

```c
v3 = entry_rsp - 0x8;
...
*(v3 + 0x10)
```

把 `v3` 替换为 `entry_rsp`，删除赋值。只匹配“入口寄存器减常量”的形状。

#### 2. 死赋值删除

反复统计每个 `vN` 的使用次数，删除无使用者且无副作用的赋值。副作用判定只认：

- `Call`
- `Panic`
- `SideEffect`

如果函数标记为 `frame-kept`，跳过这一步。原因是保留框架时，某些看似无用的赋值可能承载局部变量语义。

#### 3. 前向引用声明

结构化后的文本可能出现“使用早于定义”：

- 不可归约 CFG。
- 循环头 Phi 在回边前被引用。
- 分支重复发射。

处理方式：

1. 先删掉一类会产生前向引用的赋值：赋值左侧的值没有使用者，且右侧引用了后面才定义的值。这类行保留下来只会制造前向引用。
2. 若仍存在前向引用，在函数开头插入 `T vN; /* forward declaration */`。
3. 最多迭代 4 轮。
4. 仍有前向引用则返回错误，而不是输出坏代码。

这是一道显式的质量门：宁可失败，也不输出无法解释的文本。

#### 4. 重编号

`renumberSource` 把程序全局 ID 映射成函数内连续 ID：

```text
参数（有使用者）
→ 各块 Phi
→ 各块指令
```

与 SSAIR 文本的编号顺序一致，便于并排对照两层输出。

### 4.4 SSA pass 管线

`core/ssa2source/pass` 里的 pass 在渲染前运行，默认顺序：

```text
apply-function-define → infer-types → trim-internal-calls
→ mark-void-returns → refine-return-types
→ simplify-partial-writes → redefine-type-value
```

#### `apply-function-define`

用 [core/symbol/funcdefs](../core/symbol/funcdefs) 的 C 签名表匹配调用目标：

- 设置调用返回类型。
- 非可变参数调用截断多余实参。
- 设置已知参数类型。
- 字符串参数重命名（`s`、`format` 等）。

#### `infer-types`

最多 12 轮不动点，来源：

- C 签名的指针返回和指针参数。
- 指针加减：`ptr ± int` 仍是指针。
- 指针与 0 比较：把 0 标成空指针。
- Phi：任一输入是指针，输出和所有输入都标指针。
- 类型转换：指针转换向内传播。
- load 标记：地址使用者标成指针。

不猜结构体布局，不创建结构体类型。

#### `trim-internal-calls`

对恢复出来的内部函数：

1. 统计被真正使用的形参最高索引，得到 `usedArity`。
2. 上限 6（System V 整数参数寄存器数量）。
3. 截断调用实参到该数量。

跳过已知 C 函数和可变参数调用。目的是去掉 `lowir2ssa` 因寄存器捕获多带的实参。

#### `mark-void-returns`

若函数的所有返回都是“弱返回”（空返回、`undefined` 或 load 标记，类型转换会穿透），且没有调用方使用返回值，则把函数标成 void，并清掉返回值和调用结果类型。

显式标量或指针返回不会被改成 void。

#### `refine-return-types`

从函数体实际返回推断类型：

- 全部返回指针 → `ptr64`
- 全部返回整数 → `i64`
- 混合 → `any`
- 没有返回 → `null`

然后同时更新函数类型和所有调用点的结果类型。

#### `simplify-partial-writes`

识别 `(old & ~0xff) | (low & 0xff)`（掩码固定为 `0xff`），替换为 `low` 的源值。它主要清理“字节写 + 零扩展”留下的掩码合并，让源码更接近 `v = (uint8_t)x`。

#### `redefine-type-value`

把字符串参数的常量地址替换成字符串常量：

1. 从 C 签名或内部函数推断出字符串参数位置。
2. 跟踪实参是否来自常量地址表达式。
3. 用 `Artifact.StringAt(address, 128)` 读取 NUL 结尾、可打印的 UTF-8 字符串。
4. 替换成 `ssa.ConstInst(string)`。

这一步把：

```c
puts(0x402000);
```

变成：

```c
puts("hello");
```

只处理可打印字符串；二进制数据、非 UTF-8、超过 128 字节的内容保持地址常量。

### 4.5 地址注释

`AnnotateArtifact` 在渲染阶段给地址常量加 verbose 注释：

- 命中可打印字符串 → `/* "..." */`
- 命中符号 → `/* symbol_name */`

注释不改变值，只帮助人工对照反汇编和节表。

---

## 5. loader / arch 的拆分

这部分是支撑结构，不是本文重点，但三层能否独立演进取决于它。

### 5.1 两个独立注册表

[loader/catalog.go](../loader/catalog.go) 里的 `Catalog` 同时持有：

```go
loaders map[core.Format]core.Loader
parsers map[core.Architecture]core.Parser
```

两个映射互不引用：

- 一个 `Format` 恰好一个 loader。
- 一个 `Architecture` 至多一个 parser。
- 操作系统、位数、字节序、执行模式都在 `Target` 里，不作为分派键。

这避免了“操作系统 × 架构”的目录矩阵：ELF 里的 ARM64 和 Mach-O 里的 ARM64 使用同一个 parser，只是 `Target` 不同。

### 5.2 打开流程

```text
NewByteReader
  → 读取前 64 字节（ProbeSize）
  → 每个 loader.Match(prefix)
  → 恰好一个匹配则选中；0 个 unsupported；多个 ambiguous
  → loader.Check 解析头部
  → loader.Load 构建 Artifact
  → 按 Target.Architecture 选 parser
  → ValidateArtifact
```

`Check` 和 `Open` 是两条路径：

- `Check` 只验证头部并报告将要使用的 parser，不解码。
- `Open` 构建完整 Artifact，包括节、符号、入口和 `BoundaryHints`。

parser 选择失败不会让打开失败，而是写入 diagnostic：

- `parser-unavailable`：没有注册该架构的 parser。
- `parser-incompatible`：parser 不接受该 Target（位数、模式不符）。
- `decoder-unavailable`：parser 已注册但还不能解码。

### 5.3 Loader 只做格式事实

`Loader` 接口：

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

Loader 解析 ELF/PE/Mach-O 的头部、节表、符号表、重定位和异常表，产出：

```go
type BoundaryHints struct {
    ExecSections   []Region
    FunctionStarts []uint64
    EHFrame        []byte
    EHFrameAddr    uint64
    InitTargets    []uint64
    PLTTargets     []PLTEntry
    GOTRelocs      map[uint64]uint64
}
```

这些字段全是“格式能提供的事实”，没有一条指令被解码。Loader 不知道 x86 的 `call` 有几个字节，也不知道 ARM64 的块怎么切。

### 5.4 Parser 只做指令集

`Parser` 接口：

```go
type Parser interface {
    Kind() Architecture
    Name() string
    Available() bool
    Check(Target) error
    Parse(context.Context, ParseRequest) (*ParseResult, error)
}
```

可选能力：

- `Disassembler`：输出汇编文本。
- `BoundaryDiscoverer`：从代码段发现函数边界。

当前状态：

| 架构 | Parser | 解码 | 边界发现 |
| --- | --- | --- | --- |
| AMD64 | `loader/arch/amd64` | 是 | 是 |
| ARM64 | `loader/arch/arm64` | 是 | 是 |
| 386 / ARM / RISCV32/64 / MIPS / MIPS64 / PPC / PPC64 | `loader/arch` | 否，返回 `ErrDecoderUnavailable` | 否 |

骨架 parser 不是占位符：它们实现 `Check`，明确拒绝不匹配的位数和模式，并让 `Available() == false`。这样“不支持的架构”和“架构匹配但解码器未实现”是可区分的。

### 5.5 AMD64 的边界发现顺序

`amd64.DiscoverBoundaries` 合并 8 种来源，按优先级去重：

```text
symbol > eh_frame > function_start > init_array
> plt > entry > call_target > scan
```

流程：

1. 加入符号表里的本地函数。
2. 解析 `.eh_frame` 的 FDE 起始地址。
3. 加入 Mach-O `LC_FUNCTION_STARTS`、PE 导出/异常表地址；两者都写入 `BoundaryHints.FunctionStarts`。
4. 加入 `.init_array/.fini_array` 目标。
5. 加入 PLT 桩。
6. 加入镜像入口。
7. 从已知边界出发走直接调用图：解码函数体，收集 `CALL` 目标（含 GOT 间接目标），把新目标入队。
8. 用下一个边界补齐零长度函数的大小。
9. 对未被覆盖的可执行间隙做尾部扫描：跳过 `0xcc/0x90/0x00`，按 16 字节对齐尝试解码，找到非 `nop/int` 起点就登记。
10. 再次补齐大小。

同一地址被多个来源报告时，优先级高的来源覆盖名字和大小，低优先级只补空缺。`BoundaryStats` 记录每种来源的数量和重复数，便于判断某个二进制主要靠扫描还是靠符号恢复。

ARM64 使用同样的来源顺序和调用图遍历，只是解码器不同。AArch64 定长 4 字节，所以 ARM64 的尾部扫描按 4 字节步进，并只接受像函数开头的指令（`STP`、`SUB`、`ADD`、`MOV`、`ADRP`、`B`）。两者的 `.eh_frame` 解析都调用 `loader/arch/internal/ehframe`，只取 FDE 范围，不执行 CFI。

### 5.6 为什么不让 Parser 解析文件格式

如果 Parser 自己读 ELF：

- 每个架构都要重复节表、重定位、异常表逻辑。
- 新增文件格式要改所有架构。
- 边界发现无法复用 loader 已经解析出的 `eh_frame`、`init_array`、PLT。

如果 Loader 自己解码指令：

- 新增架构要改所有格式。
- 边界发现和 ISA 解码纠缠，测试无法分层。

当前拆分的测试边界也很清楚：

- `loader/fileformat/*_test.go` 只验证头部、节、符号和 hints。
- `loader/arch/*/testdata/*.json` 验证机器码到 LowIR 的精确文本，以及栈帧、canary 与 ABI 的行为。
- `core/lowir2ssa/testcases/*.json` 只验证 LowIR 到 SSAIR/源码。
- `core/ssa2source/testcases/*.json` 只验证 SSAIR 到源码。

任何一层回归都能定位到具体契约，而不是“整个反编译输出不对”。

---

## 6. 不变量、限制与测试

### 6.1 LowIR 不变量

- `FunctionID` 在一个 LowIR 内唯一；`BlockID`、`ValueID` 在一个函数内唯一；0 表示缺失。
- 值单次定义，使用处通过 `ValueID` 引用。
- 块内指令顺序有意义；块以控制转移结束。
- `Targets` 在跳转/分支里是同一函数内的后继块；在 Phi 里是前驱块。
- 常量 `Literal` 用低有效字节在前的规范编码，与目标内存字节序无关。
- 寄存器名称保留机器宽度；部分寄存器更新、标志位和别名关系必须显式表达。

### 6.2 转换不变量

- 寄存器变量始终是 64 位规范变量；部分读写通过移位和掩码完成。
- 入口块创建变量，保证定义支配使用。
- 直接调用的目标一定有名字；没有符号就用 `sub_0x...`。
- 未知操作保留为可见标记，不会被删除。
- 每个块都有终止符。
- 循环 Phi 只在循环头创建，且只补一次回边。

### 6.3 当前限制

以下都是实现现状，不是待办清单；写新功能时需要显式处理：

| 限制 | 影响 |
| --- | --- |
| 无内存 SSA、无别名分析 | 不同表达式指向同一地址时不会合并变量 |
| 内存合流不插 Phi | 内存槽仍按变量赋值，需别名分析才能做内存 SSA |
| 有符号/无符号条件未区分 | `jl` 与 `jb` 生成同一个比较 |
| 只捕获 System V 整数参数寄存器 | 栈参数、浮点参数、其他 ABI 不完整 |
| 间接跳转不恢复目标 | 间接跳转后的块可能不可达或丢失 |
| 只识别自然循环 | 不可归约 CFG 退化成分支或前向声明 |
| 不做语言级类型恢复 | 只有 C 签名表和指针推断，没有结构体布局 |
| 不做调用图完整性验证 | 直接调用可命名，间接调用只有目标值 |
| 源码不可编译 | 输出保留 `vN`、load 标记和 forward declaration |

### 6.4 测试层次

```text
loader/arch/<arch>/testdata/*.json     机器码 → LowIR / 栈帧 / canary / ABI
core/lowir2ssa/testcases/*.json        LowIR → SSAIR / 源码
core/ssa2source/testcases/*.json       SSAIR → 源码
common/yak/ssa/ssairtext/testdata      SSAIR 文本往返
common/yak/reify/test/examples         gcc 编译真实 C 文件后跑三层
```

每个架构目录只有一个测试文件 `testdata_test.go`，内容是调用 `loader/arch/internal/archtest`。期望值全部写在 JSON 里，一个 fixture 一个对象：

| 字段 | 含义 |
| --- | --- |
| `name` | 用例名 |
| `address` | 区域起始地址 |
| `symbols` | 函数符号，给出时按符号切分函数 |
| `code` | 每行一个十六进制串加汇编注释，例如 `55 // push rbp` |
| `lowir` | 期望的 LowIR 文本，逐字匹配，不含事实头 |
| `frame_removed` | `remove-stackframe` 应删掉的指令数 |
| `canary_removed` | `remove-canary` 应删掉的指令数 |
| `params` | 转换后应绑定的参数个数 |
| `diagnostics` | 期望的诊断码，按顺序 |

每个计数都从独立的一次解析开始，因为两个 pass 互相影响：canary 的栈槽位于帧内。

`testdata/basic.json`、`blocks.json`、`functions.json` 覆盖解码；`frame.json` 覆盖栈帧与 ABI；`canary.json` 覆盖栈保护（amd64 的 `fs:0x28`、arm64 的 `tpidr_el0`）。

fixture 的字节来源有两种，都可复核：

- `llvm-mc` 汇编后从 ELF 对象取 `.text`（s390x、loong64、riscv、mips、powerpc、x86、arm 及其 64 位变体）。
- 手工编码 + `objdump -D -b binary -m i8086` 逐条反汇编核对（8086：llvm-mc 的 `.code16` 不可靠，仍按 32 位编码）。

每条用例的注释就是该字节串的汇编写法，所以期望值与真实汇编器或反汇编器一致。

fixture 覆盖的关键形状：

- `arithmetic-branch`：算术、比较、条件分支。
- `call-arguments` / `call-return-value`：寄存器参数捕获与 `rax` 返回。
- `intrinsic-marker`：未知 intrinsic 保留。
- `memory-and-call`：栈槽与调用混合。
- `partial-register`：字节/字/双字写合并。
- `register-variables`：寄存器变量与 `rax` 返回。
- `return-uses-rax`：隐式返回。
- `stack-frame`：框架清理（canary 删除另见 `pass/stackcanary_test.go`）。

---

## 7. 扩展点

### 新增架构

1. 在 `loader/arch/<arch>` 实现 `Parser`。
2. `Check` 明确位数、模式、字节序要求。
3. `Parse` 只输出 LowIR；不要读文件格式。
4. 如需文本反汇编，实现 `Disassembler`。
5. 如需函数切分，实现 `BoundaryDiscoverer`，优先使用 `BoundaryHints`，再走调用图和扫描。
6. 在 `arch.All()` 注册；实现 `Facts()` 给出 frame、registers 和 canary。
7. 补 `testdata/*.json`：`basic.json` 验证解码，`frame.json` 验证栈帧与 ABI。

### 新增文件格式

1. 在 `loader/fileformat/<format>` 实现 `Loader`。
2. `Match` 只读 64 字节前缀。
3. `Load` 产出 `Image`、`Symbol`、`BoundaryHints`，不解码指令。
4. 在 `fileformat.All()` 注册；补头部与 hints 测试。

### 新增 LowIR pass

1. 在 `core/lowir2ssa/pass` 注册 `Pass{Name, Description, Apply}`。
2. 只改 `core.Function`，保持块终止符和值引用有效。
3. 明确是否依赖块顺序；需要时放在 `reverse-postorder` 前或后。
4. 补一个 `testcases` 输入，验证 pass 前后 SSAIR。

### 新增 SSA pass

1. 在 `core/ssa2source/pass` 注册。
2. 只改 `*ssa.Program`，不要依赖源码文本。
3. 在 `DefaultNames()` 里给出顺序，并说明它依赖哪些前序 pass。
4. 补 SSAIR fixture，验证类型、参数或调用点变化。

### 修改转换器时必须保持

- 机器精度（整数/指针位宽）。
- 内存、寄存器、异常效果。
- 控制流目标。
- 指令来源地址。
- 未知操作的可见性。

---

## 8. 小结

当前 Reify 的算法骨架可以概括为四句话：

1. **Loader 读格式，Parser 读指令，`asm2lowir` 只负责把边界、区域和命名接起来。**
2. **`lowir2ssa` 用变量物化机器状态：寄存器按规范宽度合并，内存按地址结构键共享，标志位还原成比较，循环头才建 Phi。**
3. **`ssa2source` 用支配/后支配求合流点，用自然循环识别循环，用递归区域发射还原 `if/else` 与 `while`，再用清理和类型 pass 把文本压到可读。**
4. **所有阶段都保留未知、保留地址、保留诊断；宁可输出带标记的近似，也不静默猜测。**

[返回 README](../README.md)