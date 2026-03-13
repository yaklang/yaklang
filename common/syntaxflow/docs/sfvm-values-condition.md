# SFVM Values / Condition / Anchor 设计说明（中文）

本文说明 SFVM 运行时在 `Values`、`Condition`、`Anchor`、`NativeCall` 四块的当前设计目标、数据流与关键约束：

- `Values`：SFVM 栈里永远压 `sfvm.Values`，叶子永远是 `sfvm.ValueOperator`
- `Condition`：`?{}` / `?()` 统一走 anchor-scope（按 slot 分组的执行作用域），支持 `&&` / `||` / `!` 等逻辑组合
- `Anchor`：在 anchor-scope 内用 anchor bits 保持“结果属于哪个 source slot”的可回推映射
- `NativeCall`：单值/分组两种执行模型都由 sfvm 包装，业务层不直接操心 anchor 分组协议

总体目标是让 SFVM 在绝大多数地方保持“拍平的 Values 流”，只在条件作用域内引入必要的分组语义（通过 anchor bits 实现）。

## 快速索引（代码位置）

- `common/syntaxflow/sfvm/values.go`：`sfvm.Values`、`NewValues`、`MergeValues/Remove/Intersect` 等容器语义
- `common/syntaxflow/sfvm/value_op.go`：`sfvm.ValueOperator`（原子值接口）
- `common/syntaxflow/sfvm/frame.go`：`OpAnchorScopeStart/End` 执行、anchor-scope 生命周期管理
- `common/syntaxflow/sfvm/anchor_bitvector.go`：anchor 分配/恢复、anchor 合并、mask 回推辅助函数
- `common/syntaxflow/sfvm/condition_exec.go`：`ConditionEntry`、mask/candidate 模式、比较结果归一化
- `common/syntaxflow/sfvm/native_call.go`：`ValueSingleNativeCall` / `ValuesNativeCall` 包装与分组调度
- `common/yak/ssaapi/values.go`：`ssaapi.Value.Hash()` 对临时值返回 `("", false)`，避免临时结果被错误去重

## 1. 背景：为什么会出问题

SFVM 是一个“基于栈的值过滤 VM”。很多语句是天然拍平的，比如：

- `a.b`：从 `a` 投影到 `b`
- `a?{.b}`：筛出“存在 `.b` 的 `a`”

但当过滤条件需要“按 source 分组”时，纯拍平会丢失来源信息，典型例子：

- `a?{.*<len>==2}`：`*` 会展开子项，`<len>`/比较希望按每个 `a` 单独计算
- `a?(*<len>==3)`：`?()` 里的表达式也是过滤，但语法不同（本质同一套 anchor-scope 机制）

如果没有一种机制把“子结果 -> 原始 source slot”映射回去，最后就只能得到“全局拍平后的 len/比较”，导致条件错误。

## 2. Values 重构：从类型系统里移除“隐式拍平/合并”

### 2.1 核心规则

当前实现强制三条硬规则：

1. `sfvm.ValueOperator` 只表示原子值（atomic），不再表示“列表值”
2. `sfvm.Values` 是 SFVM 运行时唯一容器（栈里永远压的是 `Values`）
3. `ssaapi.Values` 仍是 API 层的容器，不混入 SFVM 的运行时类型系统，只在边界显式转换

这解决的是旧模型的根本问题：`ValueList` 既像容器又像值，导致“什么时候会拍平/什么时候会合并/什么时候会追加前序”都隐藏在类型方法里，栈语义不可预测。

### 2.2 运行时形态

现在 SFVM 的运行时形态非常简单：

- 栈元素类型固定为 `sfvm.Values`
- 叶子类型固定为 `sfvm.ValueOperator`

并且 `ValueOperator` 的主要实现是：

- `*ssaapi.Value`
- `*ssaapi.Program`
- `*ssaapi.ProgramOverLay`

### 2.3 `NewValues` vs `MergeValues`

实现里一个非常重要的语义拆分：

- `sfvm.NewValues(...)`：保序、保重复（不去重）
- `sfvm.MergeValues(...)`：显式归并/去重（并且合并 anchor bits）

原因是：条件作用域依赖“source slot 的宽度和位置”。如果 `NewValues` 在创建时就去重，会直接破坏 slot 对齐（尤其是 source 内存在重复值时）。

`MergeValues` 的去重 key 来源大致为：

1. `GetId()`（SSA 稳定 id）
2. `Hash() (string, bool)`（稳定 hash）
3. fallback 到 `%T:%p`（对象地址）

因此一个很关键的配套约束是：

- 临时/提取类结果（例如 file-filter 生成的临时 const）不应当提供“稳定 hash”，否则会被错误合并

这也是为什么 `ssaapi.Value.Hash()` 现在只对 `id > 0` 的稳定 SSA 值返回 hash；对 `id <= 0` 的临时值返回 `("", false)`，避免 file-filter 结果被错误去重。

### 2.4 ssaapi 边界

`ssaapi.Values` 与 `sfvm.Values` 的转换是显式的：

- `ssaapi.ToSFVMValues`
- `ssaapi.FromSFVMValues`

这保证了 SFVM 的栈/条件/anchor 逻辑只需要处理一种容器类型，不再出现“某些地方是 list，某些地方是 value”的隐式分支。

## 3. Anchor：只在 anchor-scope 内维护“来源映射”

### 3.1 Anchor 的作用

Anchor 的作用不是“分组返回结果”，而是维护一种映射关系：

- 一个派生出来的值，属于哪些 source slot（可以是 1 个，也可以是多个）

SFVM 用 `ValueOperator.GetAnchorBitVector()/SetAnchorBitVector()` 在每个叶子上挂一个 bitvector：

- bitvector 的第 `i` 位表示“此值来自 source 的第 `i` 个 slot”

这样，哪怕中间经过了多层 `.b.c.*...`，最终都能把匹配结果映射回 source 的 mask。

### 3.2 Anchor 的生命周期

Anchor 只在 anchor-scope 里启用，且由同一个 opcode 控制：

- `OpAnchorScopeStart` / `OpAnchorScopeEnd`

这点很重要：没有“单独的 anchor opcode”，anchor 只是 anchor-scope 的内建能力。

实现上，scope 开始时会：

1. 取当前 source（栈顶 `sfvm.Values`）
2. 给 source 每个 slot 分配本地 anchor（写入一个“当前 scope 专用”的 bit-range）
3. 保存并在 scope 结束时恢复原先的 anchor bits（避免跨语句泄漏）

这个生命周期只允许由 anchor-scope 驱动：

- `assignLocalAnchorBitVector` / `restoreAnchorBitVector` 是 sfvm 内部机制
- search / topdef / nativecall / ssaapi 业务逻辑都不应该自己显式调这对函数

### 3.3 为什么需要“anchorBase”（嵌套 scope 的关键）

一个规则里经常出现嵌套的 anchor-scope，例如：

- `URL?{<getCall>?{.openStream()}}`

这里的内层 `?{...}` 会对 `<getCall>` 的结果（call 列表）再次做过滤。如果在内层 scope 里“直接覆盖” anchor bits，那么 call 列表就会丢失它原本携带的“来自哪个 URL slot”的信息，导致外层 `OpFilter` 无法再把内层结果映射回 `URL`。

为了解决这个问题，每个 anchor-scope 都有两个关键参数：

- `anchorBase`：当前 scope 的 bit-range 起始偏移
- `anchorWidth`：当前 scope 的 source slot 宽度（`len(source)`）

并且采用**可叠加**策略：

- scope start：在 `[anchorBase, anchorBase+anchorWidth)` 这个范围内写入本地 anchor bits
- 同时保留（OR 合并）已有的 anchor bits，让值可以同时携带“外层 provenance + 内层 provenance”

嵌套时，`anchorBase` 通过父 scope 叠加得到（父 range 的尾部开始），保证不同 scope 的 bit-range 不重叠。

伪代码（scope start）：

```text
base = 0
if hasParentScope:
  base = parent.base + parent.width
width = len(source)

for slot i in [0..width):
  oldBits = source[i].bits
  localBits(i) = { base + i }          // duplicates: union all their slots
  source[i].bits = localBits(i) OR oldBits
```

scope end 会把 `source[i].bits` 恢复为 `oldBits`（避免 scope 内写入的本地 bits 泄漏到外层/后续语句）。

### 3.4 本地 anchor 的分配策略（含重复值）

当 source 里同一个逻辑值出现多次（例如重复引用/重复 slot），本地 anchor 会取并集：

- slot 0 和 slot 2 都是同一个对象/同一个 SSA id
- 那么该值的 anchor bits 是 `{0,2}`

这样才能保证“宽度”和“位置”语义不被去重破坏，也能让后续结果映射回正确的多个 slot。

### 3.5 Anchor bits 如何传播

传播规则很简单：当某个 source 值派生出新值时，把 source 的 anchor bits OR 到派生值上。

当前统一使用一个公开 API：

- `MergeAnchor(source, dst...)`

也就是：

- 单值派生时，`MergeAnchor(parent, child)`
- 一对多派生时，`MergeAnchor(source, result...)`

这点很重要：以前“单值合并”和“结果列表合并”分成两个名字，代码里不好搜，也容易让语义边界变散。现在统一成一个入口，后续审查/迁移都只看 `MergeAnchor`。

用代码/数学表达就是：

```text
// 单值
child.bits |= parent.bits

// 一对多
for r in result:
  r.bits |= source.bits
```

同时要注意：**native call 也必须遵守这条规则**。否则像 `<scanInstruction>` 这种“从 BasicBlock 派生指令列表”的操作，派生值没有 anchor bits，就无法在 `?{!<scanInstruction>}` 这种表达式里正确映射回源 BasicBlock。

现在 anchor 传播的主要落点已经收敛到两类框架能力：

1. `RunValueOperatorPipeline`
2. `ValueSingleNativeCall` / `ValuesNativeCall`

也就是说，后续如果是“逐 value 派生结果”的场景，应优先通过 pipeline/wrapper 统一处理，而不是在某个 search/topdef/nativecall 里单独再写一遍 anchor merge 逻辑。

### 3.6 条件 mask 如何从结果反推 source

当比较/过滤产生的 cond 数组长度与 source 宽度不一致时，SFVM 会做归一化：

1. 用结果值的 anchor bits 去标记 source mask

这使得“中间展开/拍平后的候选值集合”仍然能被正确投影回“source slot 的布尔 mask”。

约束：在 mask-mode 的 anchor-scope 内，所有参与回推的值都必须携带 anchor bits；缺失属于实现 bug，VM 会直接报 `CriticalError`，而不是回退做猜测匹配。

这里的关键点是：标记 mask 时只看当前 scope 对应的 bit-range（`anchorBase..anchorBase+anchorWidth`），避免被其它 scope 的 anchor bits 干扰。

回推规则（在当前 scope 范围内投影）：

```text
mask[i] = true  <=>  (anchorBase + i) in value.bits
```

### 3.7 `ConditionEntry` 的两种模式

Condition 栈条目有两种模式：

- `ConditionModeMask`：常规模式，维护与 source slot 对齐的 `[]bool`
- `ConditionModeCandidate`：优化/兼容模式，主要用于“source 是单例且不自然产生 mask”的场景（例如 program/overlay）

对大多数 `len(source) > 1` 的情况，走 mask 模式；对某些 `len(source) == 1` 且 `ShouldUseConditionCandidate()` 的情况，走 candidate 模式。

### 3.8 谁不应该直接碰 anchor scope

当前实现里，下面这些事情都不应该出现在 `ssaapi` 的业务逻辑里：

- 手动判断 `ActiveAnchorScope()`
- 手动按 scope 做计数/分组
- 手动做 local anchor 的 assign/restore
- 为了某个单独的 operator/nativecall 再写一套“如果在 condition 里就特殊处理”的协议

这些都属于 sfvm 的职责，而不是业务 nativecall 的职责。

## 4. NativeCall 分层：single vs grouped

### 4.1 为什么 nativecall 也必须纳入框架层

nativecall 的问题和普通 value operator 一样：

- 它也会派生新值
- 它也可能需要“按 source slot 分组”执行

如果让 `ssaapi` 自己判断当前是不是处在 condition 里，再决定怎么处理 anchor，就会把 scope 协议暴露到业务层，最终造成：

- search 一套
- topdef 一套
- nativecall 一套
- ssaapi 甚至自己再补一套

这正是需要避免的。

### 4.2 `ValueSingleNativeCall`

`ValueSingleNativeCall` 用于“一个输入 leaf 独立地产生结果”的场景，例如：

- `string`
- `strlower`
- `strupper`
- `regexp`
- `name`
- `scan*`

它的语义是：

- 输入 `Values` 拍平为 leaf
- 每个 leaf 并发执行
- wrapper 统一负责结果收集，以及在 anchor-scope 内的 anchor 传播

因此这类 nativecall 不需要自己关心当前是否在 condition 里。

### 4.3 `ValuesNativeCall`

`ValuesNativeCall` 用于“语义上必须按组求值”的场景，例如：

- `len`
- `slice`

它的规则是：

- 普通模式下：整个输入当成一组
- anchor-scope 下：按当前 source slot 自动分组
- 分组顺序必须和 source slot 一致
- 空组也要保留，不能偷偷跳过

空组必须保留这一点很关键。像 `<len>` 这种 grouped nativecall，在 condition 中语义上需要对每个 slot 都给出结果；没有结果和结果为 0 不是一回事。

### 4.4 grouped nativecall 的所有权边界

`ValuesNativeCall` 解决的不是“业务逻辑”，而是“框架如何把 grouped 语义正确地放到 condition 里运行”。

因此：

- grouped nativecall 的分组逻辑必须放在 sfvm wrapper 里
- `ssaapi` 只表达“拿到一组值后要算什么”

这也是为什么 `ssaapi` 不应该再直接使用：

- `ActiveAnchorScope()`
- `CountValuesByAnchorScope()`
- `GroupValuesByAnchorScope()`

是否在 condition 中、怎样按 root/anchor 分组、结果怎样补回 slot anchor，这些都应该是 sfvm 的封装。

## 5. `?{}` / `?()` 语义：统一都是 anchor-scope（anchor 开启）

两种写法的本质是一致的：都是进入一个 anchor-scope，在 scope 内跑过滤表达式，然后把条件应用回 scope 入口处的 source。

区别不在“是不是 call”，而在“你在 scope 内把 source 展开到了什么层级”：

- `a?{.b}`：在 scope 内对 `a` 做 `.b`，然后把匹配映射回每个 `a`
- `a?(.b || .c)`：同样的条件机制，只是语法是 `?()`（函数式写法）

### 5.1 call-wide vs per-arg 的差异（你之前提的例子）

下面两种写法语义不同，且这种差异是被保留的：

- `a?(opcode:param && have:a)`：以“一个 call”作为过滤对象（call-wide）
- `a?(*?{opcode:param && have:a})`：先把一个 call 的参数展开成多个 slot，再做过滤（per-arg）

直观理解：

- call-wide：对“整个参数集合”求条件（存在某个 param，且存在某个 have:a 就算匹配）
- per-arg：要求“同一个参数 slot 同时满足多个条件”

Anchor 的作用是让 per-arg 这种“展开后再过滤，再映射回 call”能稳定工作。

## 6. 一个具体例子：没有 anchor 会失败的场景

假设 source 是两个对象：

- `a1` 有 1 个成员
- `a2` 有 2 个成员

规则：`a?{.*<len>==2}`

期望结果：只返回 `a2`。

如果没有 anchor，`a.*` 会被拍平成 3 个成员，然后 `<len>` 看到的是 3，无法得出“a2 的 len 是 2”。

有 anchor 时流程是：

1. scope start：给 `a1/a2` 分配本地 slot（0/1）并写入 anchor bits
2. `*` 展开：来自 `a1` 的子成员携带 `{0}`，来自 `a2` 的子成员携带 `{1}`
3. `<len>`/比较：计算结果携带对应 slot 的 anchor bits
4. 归一化：把匹配结果映射回 source mask 得到 `[false, true]`
5. 应用 mask：返回 `a2`

这就是 anchor 机制存在的根本理由。

## 7. 需要长期保持的约束（Invariants）

1. 栈里永远是 `sfvm.Values`，叶子永远是 `sfvm.ValueOperator`
2. `NewValues` 不做去重（保序、保重复）
3. `MergeValues` 才是显式归并点，并负责合并 anchor bits
4. Anchor 只在 anchor-scope 内启用，且必须通过 `anchorBase/anchorWidth` 做 bit-range 隔离；scope 结束必须恢复 source 的原始 anchor bits
5. `MergeAnchor` 是唯一公开的 anchor 合并入口
6. `ssaapi.Values` 不参与 SFVM 运行时类型系统，只做边界转换
7. `ValueSingleNativeCall` / `ValuesNativeCall` 是 nativecall 的唯一框架层分法
8. `ssaapi` 不直接读取 anchor scope，也不自己实现 scope 内部分组协议

只要这些约束不被破坏，condition/anchor 的行为就会稳定且可推理。

## 8. 扩展规范（新增功能时怎么落点）

### 8.1 新增 value operator

如果一个功能本质上是：

- 从一个 `ValueOperator` 派生出新的 `Values`

那么优先考虑走 `RunValueOperatorPipeline`，而不是在某个独立点手写 anchor merge 或 scope 判断。

### 8.2 新增 nativecall

先判断语义属于哪一类：

- 单 leaf 语义：`ValueSingleNativeCall`
- 分组语义：`ValuesNativeCall`

如果要在 `ssaapi` 里显式判断“当前是不是在 condition 中”，通常就说明分层已经不对了。

### 8.3 新增 condition / anchor 相关能力

如果一个需求涉及下面任意一项：

- scope 生命周期
- slot anchor 分配/恢复
- result 到 source mask 的回投
- grouped nativecall 的 condition 内分组协议

优先落在：

- `frame.go`
- `condition_exec.go`
- `anchor_bitvector.go`
- `values.go`
- `native_call.go`

而不是先在业务层打补丁。

### 8.4 文档怎么写

后续继续补文档时，优先描述：

- 谁负责什么
- 哪些 API 是唯一入口
- 哪些行为是框架保证，不需要业务层重复实现

不要再把中间态 workaround 当成最终设计写进规范里。
