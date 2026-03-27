# defer、panic、recover 与 try-catch-finally 机制说明

本文档说明 `ssa2llvm` 当前的错误处理实现。它不是走 LLVM 原生异常展开，而是把 Yak 的 `defer`、`panic`、`recover`、`try-catch-finally` lowering 为普通 CFG 和 `InvokeContext.Panic` 槽位上的协议。

## 1. YakSSA 表示

相关语法和 SSA 构建位置：

- `common/yak/antlr4yak/YaklangParser.g4`
- `common/yak/yak2ssa/builder_ast.go`
- `common/yak/ssa/cfg.go`

当前相关语法入口：

- `try { ... } catch err { ... } finally { ... }`
- `defer ...`
- `panic(expr)`
- `recover()`

SSA 层会生成：

- `ErrorHandler`
- `ErrorCatch`
- `ssa.Panic`
- `ssa.Recover`
- `DeferBlock` 和 try/catch/finally/done 相关块

## 2. 编译器侧总体策略

准备逻辑在：

- `common/yak/ssa2llvm/compiler/error_handling_internal.go`
- `common/yak/ssa2llvm/compiler/function_compile_context.go`

编译函数前会整理出函数级 metadata：

- `activeHandlerByBlock`
- `catchBodyByHandler`
- `catchTargetByBlock`
- `exceptionValueIDs`

这部分 metadata 现在属于函数级上下文，而不是全局 `Compiler` 字段。

## 3. panic 槽模型

当前实现的核心选择：

- 异常值不单独分配栈槽
- 异常值直接映射到 `InvokeContext.Panic`

因此：

- `getValue(...)` 发现某个 value id 属于 `exceptionValueIDs`
- 就直接读取 `ctx.panic`
- `recover()` 也是同样从 `ctx.panic` 取值

关键位置：

- `common/yak/ssa2llvm/compiler/ops.go`
- `common/yak/ssa2llvm/compiler/ops_panic.go`
- `common/yak/ssa2llvm/compiler/invoke_ctx_internal.go`

## 4. panic 的 lowering

`compilePanic` 的流程是：

1. 计算 panic 参数
2. 写入 `ctx.panic`
3. 看当前 block 是否有生效的 handler
4. 有 handler 时 branch 到对应 catch body
5. 没有 handler 时，如果函数定义了 `DeferBlock`，先跳到 defer
6. 否则直接 `ret void`

这意味着当前的 panic 传播本质上仍然是：

- **函数内** 靠 CFG 跳转进入 catch/defer
- **函数边界上** 靠 `ctx.panic` 保存最后异常值
- 不是原生栈展开，也不是 LLVM `landingpad`

## 5. recover 与 catch 变量

`recover()` 的 lowering 很直接：

- 读取 `ctx.panic`

catch 变量本质上也是一样：

- YakSSA 给 catch 变量一个单独 value id
- 编译器不为它单独分配真实存储
- `getValue` 识别后直接映射到 `ctx.panic`

因此当前语义里：

- `recover()`
- `catch err` 里的 `err`

最终都共享同一个 panic 槽。

## 6. defer 如何接入 return / panic 路径

`return` lowering 在：

- `common/yak/ssa2llvm/compiler/ops.go`

当前策略：

- 先把结果写入 `ctx.ret`
- 若函数存在 `DeferBlock`，不立即 `ret void`
- 统一跳到 defer block
- defer 执行完再进入最终返回块

未捕获 panic 也走同一策略：

- 先写 `ctx.panic`
- 再跳 `DeferBlock`

所以 `defer { x = recover() }` 能工作，是因为进入 defer 前 `ctx.panic` 已经写好。

## 7. try-catch-finally 控制流

SSA 侧 `TryBuilder` 已经先把 CFG 拼出来：

- try 正常结束 → finally / done
- catch 执行完 → finally / done
- finally 执行完 → done

LLVM 编译器不重建这套 CFG，只负责：

- 根据 block 查当前激活的 handler
- 在 `panic` 时补 branch 到 catch body

当前限制：

- 一个 handler 下如果有多个 catch，LLVM lowering 目前仍按单 catch 入口处理

## 8. runtime host panic 与边界

runtime 侧在：

- `common/yak/ssa2llvm/runtime/runtime_go/invoke.go`
- `common/yak/ssa2llvm/runtime/runtime_go/yak_lib.go`

当前 host/runtime panic 行为：

- runtime 会把 host panic 写回当前 `ctx.panic`
- 同时把信息打印到 `stderr`
- goroutine 内的 panic 也会被 runtime 记录并输出

但需要明确边界：

- 这仍然不是完整的跨函数异常传播系统
- 当前主要保障“函数内 CFG + 当前调用边界上的 panic 槽”
- 更强的跨调用边界 panic 协议仍然可以继续单独设计
