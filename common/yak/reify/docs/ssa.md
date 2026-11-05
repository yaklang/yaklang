# LowIR 到 YakSSA 与源码

Reify 在 LowIR 之后提供两个转换包：

| 包 | 输入 | 输出 | 作用 |
| --- | --- | --- | --- |
| [core/lowir2ssa](../core/lowir2ssa) | `*core.LowIR` | `*ssa.Program` | 机器语义进入 YakSSA |
| [core/ssa2source](../core/ssa2source) | `*ssa.Program` | 文本 | 可读的伪源码 |

两者都是普通包，不是接口骨架。`core/stage.go` 只保留契约：

```go
type SSAIRBuilder interface {
    BuildSSAIR(context.Context, *LowIR) (*ssa.Program, error)
}

type SourceRenderer interface {
    RenderSource(context.Context, *ssa.Program) (string, error)
}
```

调用链：

```text
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
| `phi` | 折为变量赋值（产生诊断） |
| `select` | `reify.select` 调用 |
| `zext`、`sext`、`trunc` | `reify.zext` 等调用 |
| `jump` / `branch` / `return` | `EmitJump` / `EmitIf` / `EmitReturn` |
| `trap` | `reify.trap` 调用，随后 `return` |

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

通用寄存器变成 YakSSA 变量，不再是 `reify.read-register` 调用。不插入 Phi。

| LowIR | YakSSA |
| --- | --- |
| `read-register` | 读取变量当前值；宽度不足时截断 |
| `write-register` | 给变量赋新值；32 位写零扩展，8/16 位写合并高位 |
| 控制流汇合 | 沿用最近一次赋值，不建 Phi |

别名按 64 位规范寄存器合并：

| 规范寄存器 | 别名 |
| --- | --- |
| `rax` | `al`、`ah`、`ax`、`eax` |
| `rbx`、`rcx`、`rdx` | `bl`、`bh`、`bx`、`ebx` 等 |
| `rsi`、`rdi`、`rbp`、`rsp` | `sil`、`dil`、`bpl`、`spl` 等 |
| `r8` 到 `r15` | `r8b`、`r8w`、`r8d`、`r8l` 等 |
| `rflags` | `eflags`、`flags` |
| `xmm0` 到 `xmm15` | 读写整个 128 位值 |

读取窄寄存器用类型转换表达截断，例如 `eax` 读成 `cast(i32, rax)`。部分寄存器写按位合并。例如写 `ah` 会保留 `rax` 的其他 56 位：

```text
cleared = rax and-not 0xff00
inserted = (value shl 8) and 0xff00
rax = cleared or inserted
```

变量形式不区分路径。汇合块读取最近一次写入的值，不做 Phi 合并，也不做路径敏感分析。这是有意的取舍：输出更接近源代码，但不是严格 SSA。

`rflags` 是变量，但标志语义仍由内建操作表达：`x86.flags.<op>` 产生新标志值，`x86.cond.<cc>` 读取标志值并产生条件。

## 内存与调用

内存、调用和不能直接映射的操作保留为对 `reify.*` 伪函数的调用：

| LowIR | YakSSA 调用 |
| --- | --- |
| `load` | `reify.load(address)` |
| `store` | `reify.store(address, value)` |
| `call` | `reify.call(target)` 或 `reify.call_0x<地址>` |
| `intrinsic` | 按符号名调用，例如 `reify.intrinsic.x86.flags.add` |
| `select` | `reify.select(condition, true, false)` |
| `zext`、`sext`、`trunc` | `reify.zext` 等调用 |

`reify.*` 不是真实函数。它们是共享的伪函数，用来给 YakSSA 一个可分析的调用点。所有伪函数使用保留前缀 `reify.`，文本输出会跳过它们的函数体。

因此：

- 寄存器已提升为变量；汇合处不插 Phi。
- 内存没有提升为 SSA 状态，`load`/`store` 的地址是普通值，不做别名分析。
- 不推断函数签名。`call` 只保留目标值。
- 未支持操作变成 `reify.op_<kind>` 调用，并产生 `lowir2ssa-unsupported-op` 诊断。

## 控制流

- `jump` 只在目标块存在时连边。目标缺失时产生 `lowir2ssa-incomplete-jump`，并补 `return`。
- `branch` 需要条件和两个目标。目标不足时产生 `lowir2ssa-incomplete-branch`；只有一个目标时仍生成 `If`，另一侧为空。
- 每个块都以终止指令结束。块没有终止指令时补一个空 `return`，保证 CFG 完整。
- `Function.Finish()` 在转换结束时调用，填充入口块、退出块和返回类型。

## 文本形式

`lowir2ssa.Text` 直接使用 `ssa.Function.DisAsm(ssa.DisAsmDefault)`。它与 `ssa2source` 都跳过 `reify.*` 伪函数，只显示真实转换结果。

`ssa2source.Render` 生成伪源码，保留 SSA 名字、块标签和调用：

```text
function sub_0x2590() {
entry-0:
  rbp = undefined;
  rsp = undefined;
  v6 = reify.intrinsic.x86.endbr64();
  v8 = cast(i32, rbp);
  v9 = cast(i32, rbp);
  v10 = v8 ^ v9;
  rbp = cast(i64, v10);
  v14 = reify.intrinsic.x86.flags.xor(v8, v9, v10);
  v20 = reify.load(rsp);
  ...
}
```

伪源码不是可编译源码，也不还原语言级类型和控制结构。它只把 SSA 图压平成易于对照机器指令的文本。

## Stack canary

`lowir2ssa` 内置 stack-protector 检测。它识别：

- `%fs:0x28` 的 TLS canary 读取。
- 把 canary 写入栈槽。
- 从同一栈槽重载并和 TLS canary 比较。
- 比较后的条件分支，以及只调用失败函数的块。

识别成功后，整段检查被删除，分支改为直接跳到成功路径，失败块在无引用时删除。死掉的地址计算由 DCE 清理。线性解码会把下一个函数并入失败块，因此检测到 `call` 后不再要求失败块有终止指令。

默认删除。调用方可以选择保留：

```go
result, err := lowir2ssa.Convert(ctx, ir, lowir2ssa.WithKeepStackCanary())
```

命令行对应 `--keep-stack-canary`。

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
- 寄存器已提升为变量；汇合处不插 Phi，也不区分路径。
- 类型只保留位宽，不做语言级类型恢复。
- 内存未提升为 SSA 状态。
- 部分寄存器按位合并，未模拟标志位的部分写规则。
- 伪源码不还原 `if`、`while` 等控制结构。
- 不执行输入，不做动态验证。

`rewriter` 包已删除。需要变换时直接对 `*ssa.Program` 编写 Pass，在本包之外实现。

[返回 README](../README.md)
