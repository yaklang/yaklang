# LowIR 到 YakSSA 与源码

Reify 提供三层表示，每层都可以挂可选的 pass：

| 包 | 输入 | 输出 | 作用 |
| --- | --- | --- | --- |
| [core/asm2lowir](../core/asm2lowir) | `*core.Artifact`、区域 | `*core.LowIR` | 调用 arch parser，补函数名与符号 |
| [core/lowir2ssa](../core/lowir2ssa) | `*core.LowIR` | `*ssa.Program` | 机器语义进入 YakSSA |
| [core/ssa2source](../core/ssa2source) | `*ssa.Program` | 文本 | 可读的伪源码 |

`core/lowir2ssa/pass` 和 `core/ssa2source/pass` 是各自的 pass 注册表。
`asm2lowir` 只负责解析和补符号，不做 LowIR 变形。
[reify.go](../reify.go) 把它们串成 `Lift`，命令行只解析参数并调用它。

调用链：

```text
Artifact + Region
  │  asm2lowir.Parse（arch parser + 符号）
  ▼
LowIR (Function → Block → Instruction)
  │  lowir2ssa.Convert
  ▼
*ssa.Program
  ├─ lowir2ssa.Text        → YakSSA 文本
  └─ ssa2source.Render     → 伪源码
```

`lowir2ssa.Convert` 返回 `Result{Program, Diagnostics}`。诊断记录转换过程中被迫放宽的语义。

## 值映射

| LowIR | YakSSA |
| --- | --- |
| `function` | `Program.NewFunction("sub_0x<地址>")` |
| `block` | `Function.NewBasicBlock` |
| `constant` | `EmitConstInst` |
| `copy` | 直接复用源值 |
| `add`、`sub`、`mul`、`udiv`、`sdiv` | `EmitBinOp` |
| `and`、`or`、`xor`、`shl`、`lshr`、`ashr` | `EmitBinOp` |
| `eq`、`ult`、`slt` | `EmitBinOp`（`eq`、`lt`） |
| `neg`、`not` | `EmitUnOp` |
| `phi` | LowIR 显式 Phi 折为变量；循环头自动生成 Phi |
| `select` | 类型化标记值 |
| `zext`、`sext`、`trunc` | `TypeCast` |
| `load` | 内存变量读取 |
| `store` | 内存变量赋值 |
| `call` | `Call`（唯一生成调用的操作） |
| `intrinsic` | 类型化标记值 |
| `jump` / `branch` / `return` | `EmitJump` / `EmitIf` / `EmitReturn` |
| `trap` | `EmitPanic`，随后 `return` |

`operand` 的映射：

- `OperandValue` 查值表。
- `OperandConstant` 变成整数常量。超过 64 位的常量保留为十六进制文本，并产生 `lowir2ssa-wide-constant` 诊断。
- `OperandRegister` 按寄存器变量读取。
- `OperandSymbol` 变成字符串常量，用于调用参数。

## 类型

每个值带位宽类型，不使用笼统的 `number`：

| LowIR `ValueType` | YakSSA 类型 |
| --- | --- |
| integer 8/16/32/64 | `i8`、`i16`、`i32`、`i64` |
| integer 1 | `i1` |
| pointer 64 | `ptr64` |
| float 32/64 | `f32`、`f64` |
| 未知宽度 | `any` |

部分寄存器因此可以靠类型区分：`al` 写入 `i8`，`ah` 写入 `i8` 并合并到第 8 到 15 位，`ax` 是 `i16`，`eax` 是 `i32`，`rax` 是 `i64`。

## 寄存器

通用寄存器变成 YakSSA 变量。普通分支汇合不插 Phi；循环头插入 loop-carried Phi。

| LowIR | YakSSA |
| --- | --- |
| `read_register` | 读取变量当前值；宽度不足时截断 |
| `write_register` | 给变量赋新值；32 位写零扩展，8/16 位写合并高位 |
| 普通汇合 | 沿用最近一次赋值 |
| 循环头 | 插入 Phi 并回填 back-edge 值 |

别名按 64 位规范寄存器合并：

| 规范寄存器 | 别名 |
| --- | --- |
| `rax` | `al`、`ah`、`ax`、`eax` |
| `rbx`、`rcx`、`rdx` | `bl`、`bh`、`bx`、`ebx` 等 |
| `rsi`、`rdi`、`rbp`、`rsp` | `sil`、`dil`、`bpl`、`spl` 等 |
| `r8` 到 `r15` | `r8b`、`r8w`、`r8d`、`r8l` 等 |
| `rflags` | `eflags`、`flags` |
| `xmm0` 到 `xmm15` | 读写整个 128 位值 |

读取窄寄存器用 C 风格类型转换表达截断，例如 `eax` 读成 `(uint32_t)rax`。部分寄存器写按位合并。例如写 `ah` 会保留 `rax` 的其他 56 位：

```text
cleared = old and-not 0xff00
inserted = (value shl 8) and 0xff00
merged = cleared or inserted
```

普通汇合块读取最近一次写入的值，不做路径敏感分析。循环头例外：转换器按逆后序先输出循环头，再输出循环体，建立 loop-carried Phi，并在 back-edge 输出后补齐第二个操作数。

`rflags` 是变量，但标志语义仍由内建操作表达：`x86.flags.<op>` 产生新标志值，`x86.cond.<cc>` 读取标志值并产生条件。

## 内存与调用

调用是唯一生成 YakSSA `Call` 的操作：

- 直接调用：目标是 `sub_0x<地址>` extern 桩。桩没有函数体，文本输出跳过它。
- 间接调用：目标是计算出的值。
- 符号调用：目标是同名 extern 函数。
- 参数：读取自上一次 call 之后写入的 `rdi`、`rsi`、`rdx`、`rcx`、`r8`、`r9`，按最高写入槽位截断。缺少的低槽传入未定义值。栈参数尚未恢复。

内存不生成调用：

- `store`：按地址表达式分组的变量赋值。内部变量按地址分组，源码统一显示为 `v<id>`。
- `load`：读取同一变量。没有已写入值时返回携带地址值 ID 的 load 标记；SSAIR 写作 `load(t<id>)`，source 渲染成 `*(type *)(address)`，例如 `*(uint32_t *)(v60)`。
- 地址表达式按结构比较。不做别名分析。

其他操作不生成调用：

- `intrinsic`：类型化标记值，例如 `(uint64_t)0 /* x86.flags.add */`。
- `select`：标记值 `select`。
- 未支持操作：标记值 `<kind>`，并产生 `lowir2ssa-unsupported-op` 诊断。
- `trap`：`panic` 后 `return`。
- `nop`：不产生指令。

因此：

- 寄存器与内存槽都是变量；普通汇合不插 Phi，循环头插入 loop-carried Phi。
- 只有汇编 `call` 对应 YakSSA `Call`。
- 不推断函数签名。间接调用只保留目标值。
- 未建模的 intrinsic 保留为可见标记，不伪装成调用。

## 函数定义

[core/symbol/funcdefs](../core/symbol/funcdefs) 保存内置 C 函数签名，每个语言一个文件，当前是 [c.go](../core/symbol/funcdefs/c.go)。

```go
type Definition struct {
    Name     string
    Params   []Param
    Return   Type
    Variadic bool
}
```

类型覆盖 `void`、`int`、`uint`、`size_t`、`pointer`、`string`、`struct` 和 `float`。没有签名的函数保持 LowIR 推断类型。

## SSA passes

转换后按顺序执行两个 pass：

1. `apply-function-define`：匹配 extern 调用目标，设置调用返回类型和已知参数类型。
2. `redefine-type-value`：把字符串参数地址替换为字符串常量。可变参数在地址可解析为字符串时也替换。

完整管线：

```text
LowIR
  ├─ remove-canary
  └─ remove-stackframe
  ▼
YakSSA
  ├─ apply-function-define
  └─ redefine-type-value
  ▼
source
```

`pass.RedefineTypeValue` 需要地址到字符串的读取函数，CLI 传入 `Artifact.StringAt`。

### 循环 Phi

`for` 循环示例：

```text
b3_1:
  v30 = phi(0x0, v67)
  v31 = phi(0x0, v60)
  v39 = v30 <= 0x4
  if (v39) goto b4_3; else goto b2_2
b2_2:
  v55 = classify(v30 * 0x4)
  v60 = v31 + v55
  v67 = v30 + 0x1
  goto b3_1
b4_3:
  return v31
```

循环头先建立 Phi，循环体使用 Phi；back-edge 输出后把 `v67` 和 `v60` 追加为第二个操作数。这是 YakSSA 文本的结构。`ssa2source` 会继续把 Phi 展开成变量赋值。

## 控制流

- `jump` 只在目标块存在时连边。目标缺失时产生 `lowir2ssa-incomplete-jump`，并补 `return`。
- `branch` 需要条件和两个目标。目标不足时产生 `lowir2ssa-incomplete-branch`；只有一个目标时仍生成 `If`，另一侧为空。
- 每个块都以终止指令结束。块没有终止指令时补一个空 `return`，保证 CFG 完整。
- 跳到无环 return-only 尾部的 `jump` 改成显式 `return rax`，避免共享 epilogue 覆盖分支返回值。
- `Function.Finish()` 在转换结束时调用，填充入口块、退出块和返回类型。

### 控制结构还原

`ssa2source` 先按逆后序排列基本块，再用支配树识别自然循环，用后支配树选择 `if` 的汇合点。循环头的 Phi 在源码层展开为循环前赋值和 back-edge 前赋值，因此源码不出现 `goto` 或 `phi`。

这套结构只处理可约简 CFG。实现参考 Ghidra 的 `CollapseStructure` / `BlockWhileDo`；remill 的 `ControlFlowStructuring` 只分类指令流，不负责反编译结构化。

## 文本形式

`lowir2ssa.Text` 输出带类型的 YakSSA 文本：每个赋值写成 `<type> t<id> = ...`，同一函数的 `=` 按最宽左列对齐。块名使用下划线，例如 `entry_0`、`b3_1`。它与 `ssa2source` 都跳过 extern 调用目标桩，只显示真实转换结果。

`ssa2source.Render` 生成伪源码。它把 CFG 重构为 `if`/`else` 和 `while`，不输出块标签、`goto` 或 `phi`。循环头的 Phi 展开为循环前赋值和 back-edge 前赋值：

```c
function accumulate() {
    v30 = 0x0;
    v31 = 0x0;
    while (v30 <= 0x4) {
      v55 = classify(v30 * 0x4);
      v60 = v31 + v55;
      v67 = v30 + 0x1;
      v30 = v67;
      v31 = v60;
    }
    return v31;
}
```

`call` 的返回值写入 `rax`。`return` 没有显式值时返回当前 `rax`。

### 类型推导

`ssa2source` 折叠冗余类型转换和恒等运算：`x ^ x`/`x - x` 为 `0x0`，`x & x`/`x | x` 为 `x`，乘一、加零、移位零消去。`0x0` 在指针上下文仍输出 `NULL`。

`pass.InferTypes` 在函数定义 pass 之后运行：

- `malloc`、`calloc`、`realloc` 的返回值标记为 `ptr64`。
- `free`、`memset`、`strncpy` 等指针参数把实参标记为 `ptr64`。
- 指针加减、`Phi`、内部函数返回值和 `Return` 传播指针类型。
- 指针比较中的常量 `0` 标成指针，source 输出 `NULL`。
- 内部函数调用根据被调函数实际读取的参数寄存器裁剪 arity；已知 C 签名优先。

函数入口的 `rsp`/`rbp` 建模为 `entry_rsp`/`entry_rbp`，不再使用 `undefined` 哨兵。`remove-stack-frame` 后无 user 的纯值不输出；仍被使用的 `undefined` 带 `/* undefined source */`。

推导只处理指针，不猜结构体布局，也不做完整的内存 SSA。

类型转换输出 C 风格转换：`i8`/`i16`/`i32`/`i64` 对应 `uint8_t`/`uint16_t`/`uint32_t`/`uint64_t`，`ptr` 对应 `void *`，`i1` 对应 `bool`。常量上的类型转换直接折叠，不生成临时定义。未建模的 intrinsic 输出 `0x0 /* x86.flags.add */`。

源码变量统一使用 `v<id>`，不暴露寄存器名或内存槽名。常量和类型转换结果在使用处内联。

source 只在值有 SSA user 时输出 `v<id> =`。没有 user 的调用直接输出 `puts(...);`；纯值和类型转换只在被使用时内联。SSA 本身仍保留常量、转换和调用结果。

常量默认十六进制。字符串、布尔和浮点按各自类型显示。常量地址命中可打印字符串时追加 `/* "..." */`；命中符号时追加 `/* 名称 */`。

伪源码不是可编译源码，也不还原语言级类型。控制结构只还原可约简的 `if`/`else` 和 `while`；不可约 CFG 可能丢失路径。源码不出现块标签、`goto` 或 `phi`。

## Stack frame

`lowir2ssa` 默认删除标准栈帧：

- 函数开头保存的 callee-saved 寄存器：`push rbp`、`push rbx`、`push r12` 到 `r15`
- `mov rbp, rsp`
- `sub rsp, N` 与 `and rsp, N` 分配/对齐
- `leave`
- 结尾的 callee-saved `pop`
- 配对的 `add rsp, N`

检测按机器指令地址分组，允许前几条无关指令穿插。默认删除。栈帧删除不重写
rbp 相对地址；需要保留时通过 pass 选项改为自定义 LowIR pass 列表。

## Stack canary

`lowir2ssa` 内置 stack-protector 检测。它识别：

- `%fs:0x28` 的 TLS canary 读取。
- 把 canary 写入栈槽。
- 从同一栈槽重载并和 TLS canary 比较。
- 比较后的条件分支，以及只调用失败函数的块。

识别成功后，整段检查被删除，分支改为直接跳到成功路径，失败块在无引用时删除。死掉的地址计算由 DCE 清理。线性解码会把下一个函数并入失败块，因此检测到 `call` 后不再要求失败块有终止指令。

默认删除。需要保留时通过自定义 LowIR pass 列表跳过 `remove-canary`。

## 诊断

| 代码 | 含义 |
| --- | --- |
| `lowir2ssa-unsupported-op` | 操作没有专用映射，保留为不透明调用 |
| `lowir2ssa-missing-operand` | 操作的操作数不足 |
| `lowir2ssa-missing-value` | 引用了未定义的值 |
| `lowir2ssa-missing-condition` | 分支没有条件 |
| `lowir2ssa-incomplete-jump` | 跳转目标不在当前 Function |
| `lowir2ssa-incomplete-branch` | 分支目标不足两个 |
| `lowir2ssa-multiple-results` | 多结果操作只保留第一个结果 |
| `lowir2ssa-wide-constant` | 超过 64 位的常量转成十六进制文本 |
| `lowir2ssa-missing-target` | 无法创建未定义目标 |

诊断不改写输入。转换结果与诊断一起返回，调用方决定是否接受不完整结果。

## 限制

- 一个 LowIR Function 对应一个 YakSSA Function。跨函数调用图尚未恢复。
- 寄存器已提升为变量；普通汇合不插 Phi。循环头使用 Phi。
- 类型只保留位宽，不做语言级类型恢复。
- 内存未提升为 SSA 状态。
- 部分寄存器按位合并，未模拟标志位的部分写规则。
- 伪源码不还原 `if`、`while` 等控制结构。
- 不执行输入，不做动态验证。

`rewriter` 包已删除。需要变换时直接对 `*ssa.Program` 编写 Pass，在本包之外实现。

[返回 README](../README.md)

## SSAIR 文本

`common/yak/ssa/ssairtext` 同时提供 SSAIR 文本的格式化和解析。
`FormatProgram` 输出稳定文本，`Parse` 把文本读回 `*ssa.Program`；
`lowir2ssa.FormatSSAIR` 只是转发到 `ssairtext.Format`。方言示例：

```text
sub_0x1000 <i64> (1) left, <i64> (2) right
type: (i64, i64) -> i64
entry_0:
    <i64> t3 = <i64> t1 add <i64> t2
    ret <i64> t3
```

支持函数头与参数、`type:` 行、命名块、常量、二元运算、`type_cast`、调用、
`If`/`jump` 和 `ret`。未知形式直接报错，不做静默跳过。

`ssairtext` 的测试只做文本 → 程序 → 文本的往返比较，不依赖
`ssa2source`，因此失败能定位到格式本身。

阶段测试使用 embedded JSON：

```text
common/yak/ssa/ssairtext/testdata/base.ssair  # SSAIR 文本往返
core/lowir2ssa/testcases/*.json           # LowIR → SSAIR/source
core/ssa2source/testcases/*.json          # SSAIR → source
loader/arch/amd64/testdata/*.json         # 机器码 → 精确 LowIR
```

AMD64 用例的 `lowir` 是完整期望文本，不做片段匹配；
SSAIR 用例是普通文本，不使用 JSON 转义。
