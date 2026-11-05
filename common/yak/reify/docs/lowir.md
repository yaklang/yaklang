# LowIR

LowIR 是 Reify 的机器级程序表示。它是具体结构，不是接口：所有 parser 产出相同的 Function、Block、Instruction、Operand 和 Effect 形状。

声明位于 [core/lowir.go](../core/lowir.go)。架构 parser 负责把机器码转换为这些结构；core 不包含 ISA 解码表、寄存器别名表或提升规则。

## 结构

```go
type LowIR struct {
    Target    Target
    Functions []Function
}

type Function struct {
    ID      FunctionID
    Address uint64
    Blocks  []Block
}

type Block struct {
    ID           BlockID
    Address      uint64
    Instructions []Instruction
}

type Instruction struct {
    Kind     Opcode
    Operands []Operand
    Results  []Value
    Targets  []BlockID
    Effects  Effect
    Address  uint64
}
```

一个 Function 对应 parser 处理的一个连续代码范围。当前 AMD64 parser 对一个 `ParseRequest` 区域产出一个 Function；后续可以通过符号、调用目标或异常表扩展函数发现，不要求一个区域等于整个文件。

`Instruction.Kind` 是统一的指令种类。它不使用接口方法，也不包装原始 ISA 指令；操作数、结果、控制目标和效果都在同一结构上表达。

## 标识与顺序

- `FunctionID` 的作用域是一个 LowIR；`BlockID` 和 `ValueID` 的作用域是一个 Function。
- 零表示缺失标识。
- 值采用单次定义，使用位置通过 `ValueID` 引用定义。
- 基本块内的指令顺序有意义，必须保持。
- 基本块以控制转移指令结束。
- `Instruction.Targets` 在控制转移中表示同一 Function 内的后继块，在 Phi 中表示前驱块。
- Phi 的 `Targets` 与 `Operands` 一一对应。

寄存器和内存不是隐式可变的临时值。它们通过读写操作和效果位表达。

## 类型与常量

```go
type ValueType struct {
    Kind TypeKind
    Bits uint16
}
```

- `Kind` 取 integer、float、pointer 或 opaque。
- `Bits` 是值宽或地址宽，保留机器精度。
- 整数符号不属于位模式本身；有符号与无符号语义由操作决定，例如 SDiv/UDiv、SLess/ULess、AShr/LShr。

`Operand` 是带标签的载荷：

| Kind | 有效载荷 |
| --- | --- |
| `OperandValue` | `Value` 与类型 |
| `OperandConstant` | `Literal` 位模式与类型 |
| `OperandRegister` | `Name` 与类型 |
| `OperandSymbol` | `Name`，用于调用目标或具名语义 |

常量 `Literal` 使用低有效字节在前的规范编码，与目标内存字节序无关。有效位数由 `ValueType.Bits` 决定。

寄存器名称在目标架构内解释。部分寄存器更新、标志位和别名关系必须显式保留，不能用同名变量赋值代替。

## 操作

| 操作族 | Opcode | 约束 |
| --- | --- | --- |
| 值与合并 | `constant`、`copy`、`phi`、`select` | 类型、定义和前驱关系一致 |
| 寄存器 | `read-register`、`write-register` | 明确寄存器与宽度 |
| 内存 | `load`、`store` | 地址、值类型和效果完整 |
| 算术 | `add`、`sub`、`mul`、`udiv`、`sdiv`、`neg` | 按位宽解释，除法保留异常效果 |
| 位运算 | `and`、`or`、`xor`、`not`、`shl`、`lshr`、`ashr` | 移位数与位宽规则明确 |
| 比较 | `eq`、`ult`、`slt` | 结果为一位整数 |
| 宽度转换 | `zext`、`sext`、`trunc` | 源与目标位宽可追溯 |
| 调用 | `call`、`intrinsic` | 保留目标、参数、结果和效果 |
| 控制流 | `jump`、`branch`、`return`、`trap` | 目标和异常边界明确 |
| 空操作 | `nop` | 不改变值、内存或控制流 |

### 载荷约定

| 操作 | Operands | Results | Targets |
| --- | --- | --- | --- |
| Constant | 一个常量载荷 | 一个定义 | 空 |
| Copy / 宽度转换 | 一个值 | 一个定义 | 空 |
| 二元算术 / 比较 | 左值、右值 | 一个定义 | 空 |
| Neg / Not | 一个值 | 一个定义 | 空 |
| ReadRegister | 寄存器操作数 | 一个定义 | 空 |
| WriteRegister | 寄存器操作数、写入值 | 空 | 空 |
| Load | 地址值 | 读取值 | 空 |
| Store | 地址值、写入值 | 空 | 空 |
| Phi | 各前驱输入值 | 一个合并定义 | 与输入对应的前驱块 |
| Select | 条件、真值、假值 | 一个选择结果 | 空 |
| Nop | 空 | 空 | 空 |
| Call / Intrinsic | 目标或语义符号、参数 | 返回值列表 | 空 |
| Jump | 区域内直接跳转时为空；间接跳转或区域外直接跳转时为目标值 | 空 | 区域内目标块 |
| Branch | 一位条件值；区域外目标追加为目标地址 | 空 | 已知目标块，true 目标在前 |
| Return | 返回值列表 | 空 | 空 |
| Trap | 原因符号或参数 | 空 | 空 |

内存访问宽度由读取结果的类型或写入值的类型确定。地址和写入值是不同操作数。

`intrinsic` 用于不能由通用操作准确表示的行为。浮点环境、原子顺序等语义必须有明确约定，不能只靠类型名或效果位推定。

### AMD64 约定

AMD64 parser 使用以下约定：

- 通用寄存器通过 `read-register` 和 `write-register` 读写，名称保留机器宽度，例如 `al`、`eax`、`rax`。
- `rflags` 是伪寄存器。算术、逻辑、比较和测试指令写它；`Jcc`、`SETcc` 和 `CMOVcc` 读它。`JCXZ`、`JECXZ` 和 `JRCXZ` 直接比较计数寄存器，不读 `rflags`。
- 条件值由 `x86.cond.<cc>` 产生，输入是 `rflags`，结果是一位整数。
- 标志更新由 `x86.flags.<op>` 表示，输入是参与运算的值，结果写回 `rflags`。
- 内存地址是 `pointer` 值。基址、索引、位移和段覆盖分别保留；RIP 相对地址在解码时折算为绝对地址。
- 未覆盖的指令以 `x86.<mnemonic>` 内建操作保留，并带有 `unknown` 效果。

## 效果

```go
type Effect uint32
```

| 位 | 含义 |
| --- | --- |
| `read-memory` | 读取内存 |
| `write-memory` | 写入内存 |
| `read-register` | 读取寄存器 |
| `write-register` | 写入寄存器 |
| `call` | 可能调用外部代码 |
| `may-trap` | 可能产生机器异常 |
| `volatile` | 访问不可消除或重排 |
| `atomic` | 具有原子语义 |
| `unknown` | 行为未完整建模 |

效果位用于暴露依赖，不替代操作本身的语义。跨越内存访问、调用或异常边界的变换需要额外证明。未知行为必须保留为显式效果和诊断，不能默认为纯计算。

LowIR 不声称仅凭效果位完成别名分析、MemorySSA 或并发语义验证。

## 来源

`Instruction.Address` 记录对应的机器指令地址。一个机器指令可以产生多个 LowIR 指令；同一条机器指令产生的合成操作沿用该指令地址。块尾补的 `jump` 或 `trap` 使用块边界地址。

`Parse` 只输出 LowIR。汇编文本由可选的 `Disassembler` 单独渲染，不是 LowIR 的语义来源。LowIR 文本由 `core.FormatLowIR` 渲染，JSON 是机器可读形式。

## 文本形式

`core.FormatLowIR` 输出稳定的逐行表示，用于人工检查：

```text
target: unknown/amd64 (64-bit, little-endian)
function f1 @ 0x2590
block b1 @ 0x2590
  0x2590  intrinsic @x86.endbr64
  0x2594  v1:i32 = read-register %ebp [read-register]
  0x2594  v2:i32 = read-register %ebp [read-register]
  0x2594  v3:i32 = xor v1, v2
  0x2594  write-register %ebp, v3 [write-register]
  ...
```

文本保留函数、块、值 ID、类型、操作、操作数、目标和效果。它不是新的存储格式，也不改变 JSON 契约。

## 与 YakSSA 的关系

LowIR 聚焦机器精度与执行效果；YakSSA 面向更高层程序关系与现有分析生态。

[core/lowir2ssa](../core/lowir2ssa) 已提供转换：

- 值、算术、比较和宽度转换进入 YakSSA 值。
- 通用寄存器提升为带位宽类型的变量；不插入 Phi。
- 内存、调用、intrinsic 和未支持操作保留为 `reify.*` 调用。
- 控制流转换成 `jump`、`if` 和 `return`。

转换不做内存 SSA、语言级类型恢复和调用图恢复，并提供可选的 stack canary 删除。映射与诊断见 [SSA 文档](ssa.md)。

实现新的提升规则时必须保留：

- 整数与指针位宽。
- 内存、寄存器和异常效果。
- 控制流目标。
- 指令来源地址。

[返回 README](../README.md)
