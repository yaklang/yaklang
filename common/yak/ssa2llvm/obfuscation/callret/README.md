# callret Mechanism Note

本文档描述 `callret` 在 `ssa2llvm` 里的机制定位、变换路径、最终产物形态，以及一组真实样例证据。

这份文档的定位是“机制说明”，不是实验报告。文中的样例、IR 片段和汇编片段只用于证明这个机制在当前实现里实际会产出什么，而不是把样例本身当成机制定义。

文档整理时间：`2026-04-02T12:02:15+08:00`  
样例验证时间：`2026-04-01T22:43:27+08:00`

## 1. 机制定位

`callret` 是 `ssa2llvm` 当前的一个 **hybrid obfuscator**。

它不引入新的执行域，也不是新的编译后端；它仍然工作在现有 `SSA -> LLVM -> asm -> binary` 路径里。它的目标不是改变 Yak 程序的语义，而是把“普通 Yak 内部函数调用链”重写成更难直接阅读和恢复的控制流形态。

当前它的战略定位应该理解为：

- 它是一个 `obf`
- 它是 native 路径上的 Layer 1 primitive
- 它负责打掉清晰的内部函数边界和调用图
- 它不是最终的主防线，也不负责提供完整的高强度对抗能力

## 2. 机制边界

`callret` 当前只处理 **普通 Yak 内部 direct call**。

它不负责：

- runtime/extern/builtin 调用本身的混淆
- 新建另一套用户可见 ABI
- 把程序变成解释执行
- 把所有控制流都自动改写到受保护执行域

当前实现里明确跳过的场景包括：

- `async`
- `unpack`
- `drop-error`
- `ellipsis`
- call result 用户超出 continuation 区域
- phi-sensitive 的 continuation/value restore 场景

因此，这个机制的正确理解是：

- 它是“普通内部调用的结构性改写”
- 不是“全程序任意调用的普适混淆”

## 3. 在编译流水线里的位置

`callret` 是三阶段 obfuscation 管线中的 hybrid pass：

1. `StageSSAPre`
2. `StageSSAPost`
3. `StageLLVM`

其中 `callret` 真正起作用的是后两段：

- `StageSSAPost`
  - 把普通 Yak 内部调用改写成显式协议
- `StageLLVM`
  - 把协议 helper lowering 成真实的 LLVM IR 和最终汇编

对应代码位置：

- `common/yak/ssa2llvm/obfuscation/callret/callret.go`
- `common/yak/ssa2llvm/obfuscation/callret/callret_llvm.go`

## 4. SSA 阶段到底做了什么

### 4.1 只保留入口函数作为最终主体

`StageSSAPost` 会收集当前程序里的普通 Yak 内部函数，把非入口函数的 basic blocks 合并到入口函数里，然后清掉 `program.Funcs` 中其他函数，只保留入口函数继续进入后续编译。

这一步的直接效果是：

- 最终编译出的 LLVM 函数不再保留原来的 `leaf` / `mid` / `top` 之类清晰内部函数边界
- 程序主体更像一个大的单函数状态机

### 4.2 参数与自由变量改成 value stack 输入

被合并进入口函数的 callee，不再通过原来那套正常的函数入参绑定接收参数。

当前实现会在函数入口前部把参数、自由变量、成员输入等统一改写为从 value stack 里 pop 出来。

这样做的结果是：

- 原本“call 时传入、callee 入口接收”的结构在 SSA 上被抹平
- 调用边不再直接表现为一条普通函数调用边

### 4.3 调用点改成 push + jump

普通内部调用点会被改写为：

1. 先把 continuation 区域仍然需要的 live values 压到 value stack
2. 再把 invoke args 按协议压到 value stack
3. 把 continuation block id 压到 call stack
4. 删除原始 call 指令
5. 用 jump 直接跳到 callee entry block

这一步是整个机制的核心。原来的“call callee, wait ret”关系，变成了显式的：

- value transport
- continuation encoding
- control transfer

### 4.4 return 改成 push result + ret dispatch

普通 return 不再直接返回给 caller。

当前实现会把 return 改成：

1. 把返回值压到 value stack
2. 从 call stack pop 一个 continuation id
3. 再把这个 continuation id 压回 value stack
4. 删除原始 return
5. jump 到统一的 ret dispatch block

随后 ret dispatch block 会：

- 先取出 ret id
- 如果 ret id == 0，进入最终 exit
- 否则按 continuation block id 链式比较并跳转到对应 continuation

因此，原来的“call/ret 结构”在 SSA 层被替换成了：

- jump
- explicit stack protocol
- central return dispatcher

## 5. LLVM 阶段到底做了什么

SSA 阶段不会直接把协议变成最终汇编，它只是插入四个 helper 调用标记：

- `__yak_obf_vs_push`
- `__yak_obf_vs_pop`
- `__yak_obf_cs_push`
- `__yak_obf_cs_pop`

LLVM 阶段再把这四类 helper 进一步 lowering 成显式状态。

### 5.1 在函数入口创建两块 obf stack

当前默认会在函数入口生成两块 `i64` 数组：

- value stack：`65536`
- call stack：`65536`

同时生成两个栈顶指针：

- `obf_vs_sp`
- `obf_cs_sp`

### 5.2 push/pop helper 变成真实的 load/store

`StageLLVM` 会逐条扫描 LLVM call instruction。

如果被调用目标是上述四个 helper 之一，就直接把它替换成：

- 读 sp
- 计算元素地址
- store/load
- 更新 sp

随后把 helper 自身删掉。

所以最终 IR 和最终汇编里，不再保留那四个 helper 符号，而是只剩：

- `alloca`
- `load`
- `store`
- `add/sub`
- `cmp`
- `br/jmp`

## 6. 最终产物形态

从最终逆向视角看，`callret` 做出来的不是“另一个调用约定版本的函数调用”，而是一个显式状态机。

当前样式可以概括为：

- 一个巨大的入口函数
- 两块本地 stack 数组
- 一套 value stack 协议
- 一套 call stack 协议
- 一个 ret-id dispatcher
- 大量由 continuation 驱动的块间跳转

它最直接打掉的是：

- 清晰的内部函数边界
- 清晰的内部调用图
- 从符号名和 call edge 直接跟读逻辑的路径

## 7. 这项机制真正提供的收益

当前这版 `callret` 的收益主要是 **结构性收益**，不是“语义隐藏”。

它最有价值的地方是：

- 让内部函数不再自然地出现在最终二进制里
- 让普通静态阅读者和基于函数图的分析先失去抓手
- 让控制流恢复必须先理解 continuation 协议和两个显式 stack

但它当前还不是“非常强”的原因也很清楚：

- 保护范围不是全覆盖
- ret dispatch 仍然是可识别协议
- stack 模式当前比较固定
- 真正的计算语义并没有被彻底改写，只是换了控制流载体

因此更准确的定位是：

- `callret` 适合作为 Layer 1
- 它应该和后续更强的 native obf / protected 机制叠加
- 不应该单独承担整套高强度抗逆向目标

## 8. 真实样例证据

下面这组证据来自一次直接使用 `common/yak/ssa2llvm/cmd` 跑出来的最小样例。它们只用来证明这个机制在当前实现里“确实变成了什么样”，不构成机制定义本身。

### 8.1 样例语义未变

样例函数调用链：

```yak
leaf = () => { return 7 }
mid = () => { return leaf() + 8 }
top = () => { return mid() + leaf() }
check = () => { return top() + mid() }
```

验证结果：

- 普通版二进制：`exit=37`
- `callret` 版二进制：`exit=37`

说明这个机制当前是结构改写，不是语义改写。

### 8.2 普通版 IR 仍然保留内部函数边界

普通版里仍然能直接看到：

- `@leaf`
- `@mid`
- `@top`
- `@check`

而且 `mid` / `check` 中依然能看见通过 `InvokeContext + yak_runtime_invoke` 发起的正常内部调用路径。

典型片段：

```llvm
define void @leaf(ptr %ctx_6) {
bb_7:
  %0 = getelementptr i64, ptr %ctx_6, i64 6
  store i64 7, ptr %0, align 4
  ret void
}
```

### 8.3 callret 版 IR 只剩一个主体函数

`callret` 版里只剩一个大的 `check`，并且入口一开始就是两块 obf stack：

```llvm
define void @check(ptr %ctx_25) {
bb_40:
  %obf_vs_sp = alloca i64, align 8
  %obf_cs_sp = alloca i64, align 8
  store i64 0, ptr %obf_vs_sp, align 4
  store i64 0, ptr %obf_cs_sp, align 4
  %obf_vs = alloca i64, i64 65536, align 8
  %obf_cs = alloca i64, i64 65536, align 8
```

后续可见显式 continuation id：

```llvm
store i64 44, ptr %obf_ptr14, align 4
store i64 55, ptr %obf_ptr29, align 4
```

以及统一 ret dispatch：

```llvm
%val_94 = icmp eq i64 %obf_pop34, 0
%val_98 = icmp eq i64 %obf_pop34, 44
%val_102 = icmp eq i64 %obf_pop34, 50
```

### 8.4 callret 版汇编体现为单函数状态机

最终汇编里，最显眼的两个证据是：

1. `check` 入口申请了接近 `1 MiB` 的局部栈帧
2. 内部逻辑主要由显式数组读写和 ret-id 比较链构成

典型片段：

```asm
0000000000caece0 <check>:
  sub    rsp,0xfffa0
  mov    QWORD PTR [rsp+0xfff98],0x0
  mov    QWORD PTR [rsp+0xfff90],0x0
```

以及 continuation id 写入：

```asm
mov    QWORD PTR [rsp+rax*8-0x70],0x2c
mov    QWORD PTR [rsp+rax*8-0x70],0x32
mov    QWORD PTR [rsp+rax*8-0x70],0x37
```

以及 ret-id dispatch：

```asm
cmp    rax,0x0
cmp    rax,0x2c
cmp    rax,0x32
cmp    rax,0x37
```

### 8.5 证据产物位置

本次样例产物保存在当前 worktree 的：

- `.codex-tmp/ssa2llvm-callret-demo/out/normal.ll`
- `.codex-tmp/ssa2llvm-callret-demo/out/callret.ll`
- `.codex-tmp/ssa2llvm-callret-demo/out/normal.bin`
- `.codex-tmp/ssa2llvm-callret-demo/out/callret.bin`
- `.codex-tmp/ssa2llvm-callret-demo/out/normal.core.ir.txt`
- `.codex-tmp/ssa2llvm-callret-demo/out/callret.core.ir.txt`
- `.codex-tmp/ssa2llvm-callret-demo/out/normal.core.asm.txt`
- `.codex-tmp/ssa2llvm-callret-demo/out/callret.core.asm.txt`

这些文件是证据材料，不是机制定义的一部分。

## 9. 当前实现限制

当前 README 需要明确承认这些限制：

- 它只处理普通 Yak 内部 direct call
- 它当前仍然能被模式识别
- 它并没有把真实计算语义全部隐藏掉
- 它更适合作为后续更强机制的前置层

因此，后续演进方向应该是：

- 完善 `callret v2`
- 增加 dispatcher family
- 增加 stack/continuation 多样化
- 与更强的 protected 机制叠加

## 10. 应如何阅读这份机制

看 `callret` 时，建议把它理解成：

- 一个 native obf
- 一个结构重写器
- 一个 continuation + explicit stack protocol
- 一个 Layer 1 primitive

不要把它理解成：

- 另一套完整 VM
- 新后端
- 全程序保护方案
- 已经足以单独对抗高强度逆向的终态机制
