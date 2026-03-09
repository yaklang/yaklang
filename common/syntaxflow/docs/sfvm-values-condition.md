# SFVM Values / Condition / Anchor 设计说明（中文）

本文只覆盖这次重构里两块最关键的改动：

- `ValueList -> Values`：把 SFVM 运行时“单值/多值”的混用彻底拆开
- `Anchor`：在条件过滤作用域（`?{}` / `?()`）内稳定维护“结果属于哪个 source slot”的映射

目标是让 SFVM 在绝大多数地方保持“拍平的 Values 流”，只在条件作用域内引入必要的分组语义（通过 anchor bits 实现）。

## 快速索引（代码位置）

- `common/syntaxflow/sfvm/values.go`：`sfvm.Values`、`NewValues`、`MergeValues/Remove/Intersect` 等容器语义
- `common/syntaxflow/sfvm/value_op.go`：`sfvm.ValueOperator`（原子值接口）
- `common/syntaxflow/sfvm/frame.go`：`OpConditionScopeStart/End` 执行、anchor scope 生命周期管理
- `common/syntaxflow/sfvm/anchor_bitvector.go`：anchor 分配/恢复、anchor 合并、mask 回推辅助函数
- `common/syntaxflow/sfvm/condition_exec.go`：`ConditionEntry`、mask/candidate 模式、比较结果归一化
- `common/yak/ssaapi/values.go`：`ssaapi.Value.Hash()` 对临时值返回 `("", false)`，避免临时结果被错误去重

## 1. 背景：为什么会出问题

SFVM 是一个“基于栈的值过滤 VM”。很多语句是天然拍平的，比如：

- `a.b`：从 `a` 投影到 `b`
- `a?{.b}`：筛出“存在 `.b` 的 `a`”

但当过滤条件需要“按 source 分组”时，纯拍平会丢失来源信息，典型例子：

- `a?{.*<len>==2}`：`*` 会展开子项，`<len>`/比较希望按每个 `a` 单独计算
- `a?(*<len>==3)`：`?()` 里的表达式也是过滤，但语法不同（本质同一套 condition-scope 机制）

如果没有一种机制把“子结果 -> 原始 source slot”映射回去，最后就只能得到“全局拍平后的 len/比较”，导致条件错误。

## 2. Values 重构：从类型系统里移除“隐式拍平/合并”

### 2.1 核心规则

这次重构强制三条硬规则：

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

这次改动里一个非常重要的语义拆分：

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

## 3. Anchor：只在 condition-scope 内维护“来源映射”

### 3.1 Anchor 的作用

Anchor 的作用不是“分组返回结果”，而是维护一种映射关系：

- 一个派生出来的值，属于哪些 source slot（可以是 1 个，也可以是多个）

SFVM 用 `ValueOperator.GetAnchorBitVector()/SetAnchorBitVector()` 在每个叶子上挂一个 bitvector：

- bitvector 的第 `i` 位表示“此值来自 source 的第 `i` 个 slot”

这样，哪怕中间经过了多层 `.b.c.*...`，最终都能把匹配结果映射回 source 的 mask。

### 3.2 Anchor 的生命周期

Anchor 只在 condition scope 里启用，且由同一个 opcode 控制：

- `OpConditionScopeStart` / `OpConditionScopeEnd`

这点很重要：没有“单独的 anchor opcode”，anchor 只是 condition-scope 的内建能力。

实现上，scope 开始时会：

1. 取当前 source（栈顶 `sfvm.Values`）
2. 给 source 每个 slot 分配本地 anchor（写入一个“当前 scope 专用”的 bit-range）
3. 保存并在 scope 结束时恢复原先的 anchor bits（避免跨语句泄漏）

### 3.3 为什么需要“anchorBase”（嵌套 scope 的关键）

一个规则里经常出现嵌套的 condition-scope，例如：

- `URL?{<getCall>?{.openStream()}}`

这里的内层 `?{...}` 会对 `<getCall>` 的结果（call 列表）再次做过滤。如果在内层 scope 里“直接覆盖” anchor bits，那么 call 列表就会丢失它原本携带的“来自哪个 URL slot”的信息，导致外层 `OpFilter` 无法再把内层结果映射回 `URL`。

为了解决这个问题，每个 condition-scope 都有两个关键参数：

- `anchorBase`：当前 scope 的 bit-range 起始偏移
- `anchorWidth`：当前 scope 的 source slot 宽度（`len(source)`）

并且采用**可叠加**策略：

- scope start：在 `[anchorBase, anchorBase+anchorWidth)` 这个范围内写入本地 anchor bits
- 同时保留（OR 合并）已有的 anchor bits，让值可以同时携带“外层 provenance + 内层 provenance”

嵌套时，`anchorBase` 通过父 scope 叠加得到（父 range 的尾部开始），保证不同 scope 的 bit-range 不重叠。

### 3.4 本地 anchor 的分配策略（含重复值）

当 source 里同一个逻辑值出现多次（例如重复引用/重复 slot），本地 anchor 会取并集：

- slot 0 和 slot 2 都是同一个对象/同一个 SSA id
- 那么该值的 anchor bits 是 `{0,2}`

这样才能保证“宽度”和“位置”语义不被去重破坏，也能让后续结果映射回正确的多个 slot。

### 3.5 Anchor bits 如何传播

传播规则很简单：当某个 source 值派生出新值时，把 source 的 anchor bits OR 到派生值上。

SFVM 在很多“从 value 取下一跳”的地方会做一次 `mergeAnchorBitVectorToResult(result, source)`，确保派生出的 `result` 能携带来源信息。

同时要注意：**native call 也必须遵守这条规则**。否则像 `<scanInstruction>` 这种“从 BasicBlock 派生指令列表”的操作，派生值没有 anchor bits，就无法在 `?{!<scanInstruction>}` 这种表达式里正确映射回源 BasicBlock。

### 3.6 条件 mask 如何从结果反推 source

当比较/过滤产生的 cond 数组长度与 source 宽度不一致时，SFVM 会做归一化：

1. 优先用结果值的 anchor bits 去标记 source mask
2. 如果结果值没有 anchor bits，再用 value identity 回退匹配（id/指针 identity）

这使得“中间展开/拍平后的候选值集合”仍然能被正确投影回“source slot 的布尔 mask”。

这里的关键点是：标记 mask 时只看当前 scope 对应的 bit-range（`anchorBase..anchorBase+anchorWidth`），避免被其它 scope 的 anchor bits 干扰。

### 3.7 `ConditionEntry` 的两种模式

Condition 栈条目有两种模式：

- `ConditionModeMask`：常规模式，维护与 source slot 对齐的 `[]bool`
- `ConditionModeCandidate`：优化/兼容模式，主要用于“source 是单例且不自然产生 mask”的场景（例如 program/overlay）

对大多数 `len(source) > 1` 的情况，走 mask 模式；对某些 `len(source) == 1` 且 `ShouldUseConditionCandidate()` 的情况，走 candidate 模式。

## 4. `?{}` / `?()` 语义：统一都是 condition-scope（anchor 开启）

两种写法的本质是一致的：都是进入一个 condition scope，在 scope 内跑过滤表达式，然后把条件应用回 scope 入口处的 source。

区别不在“是不是 call”，而在“你在 scope 内把 source 展开到了什么层级”：

- `a?{.b}`：在 scope 内对 `a` 做 `.b`，然后把匹配映射回每个 `a`
- `a?(.b || .c)`：同样的条件机制，只是语法是 `?()`（函数式写法）

### 4.1 call-wide vs per-arg 的差异（你之前提的例子）

下面两种写法语义不同，且这种差异是被保留的：

- `a?(opcode:param && have:a)`：以“一个 call”作为过滤对象（call-wide）
- `a?(*?{opcode:param && have:a})`：先把一个 call 的参数展开成多个 slot，再做过滤（per-arg）

直观理解：

- call-wide：对“整个参数集合”求条件（存在某个 param，且存在某个 have:a 就算匹配）
- per-arg：要求“同一个参数 slot 同时满足多个条件”

Anchor 的作用是让 per-arg 这种“展开后再过滤，再映射回 call”能稳定工作。

## 5. 一个具体例子：没有 anchor 会失败的场景

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

## 6. 需要长期保持的约束（Invariants）

1. 栈里永远是 `sfvm.Values`，叶子永远是 `sfvm.ValueOperator`
2. `NewValues` 不做去重（保序、保重复）
3. `MergeValues` 才是显式归并点，并负责合并 anchor bits
4. Anchor 只在 condition-scope 内启用，且必须通过 `anchorBase/anchorWidth` 做 bit-range 隔离；scope 结束必须恢复 source 的原始 anchor bits
5. `ssaapi.Values` 不参与 SFVM 运行时类型系统，只做边界转换

只要这些约束不被破坏，condition/anchor 的行为就会稳定且可推理。
