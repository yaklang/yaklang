# defer、panic、recover 与 try-catch-finally 机制说明

本文档说明 `ssa2llvm` 当前的错误处理实现。它不是走 LLVM 原生异常展开，而是把 Yak 的 `defer`、`panic`、`recover`、`try-catch-finally` 降成普通 CFG 和 `InvokeContext.Panic` 槽位上的协议。

## 1. 语法到 YakSSA 的表示

Yak 语法定义在：

- `common/yak/antlr4yak/YaklangParser.g4`

语法树到 YakSSA 的构建逻辑在：

- `common/yak/yak2ssa/builder_ast.go`
- `common/yak/ssa/cfg.go`

当前相关语法入口：

- `try { ... } catch err { ... } finally { ... }`
- `defer ...`
- `panic(expr)`
- `recover()`

对应 SSA 构建方式：

- `buildTryCatchStmt` 调用 `TryBuilder`
- `buildDeferStmt` 支持 defer 一个 `Call` / `recover` / `panic`
- `buildPanicStmt` 生成 `ssa.Panic`
- `buildRecoverStmt` 生成 `ssa.Recover`

`TryBuilder` 会生成：

- `ErrorHandler`
- `ErrorCatch`
- `TryStart / TryCatch / TryFinally / TryDone` 相关块

catch 里的异常变量本质上先是一个 `Undefined` 值，之后在 LLVM lowering 阶段被解释为“从当前函数的 panic 槽读值”。

## 2. LLVM 侧的总体策略

准备逻辑在：

- `common/yak/ssa2llvm/compiler/error_handling_internal.go`

编译函数前，`prepareErrorHandling` 会先把 SSA 里的 handler 信息整理成几张表：

- `activeHandlerByBlock`：某个 block 当前生效的 handler
- `catchBodyByHandler`：handler 对应的第一段 catch body
- `catchTargetByBlock`：catch body 跑完后要跳去哪里（`finally` 或 `done`）
- `exceptionValueIDs`：哪些 SSA value id 代表 catch 异常变量

这里有一个很重要的实现选择：

- **异常值不单独分配栈槽**
- **异常值直接映射到 `InvokeContext.Panic`**

所以：

- `getValue(...)` 如果发现当前 value id 在 `exceptionValueIDs` 里，
- 就不会读普通 SSA cache，
- 而是直接 `load ctx.panic`。

相关代码：

- `common/yak/ssa2llvm/compiler/ops.go`

## 3. `panic` 怎么 lowering

实现位于：

- `common/yak/ssa2llvm/compiler/ops_panic.go`

`compilePanic` 的流程是：

1. 计算 panic 参数；
2. 写入 `ctx.panic`；
3. 看当前 block 是否有生效的 error handler；
4. 有 handler 时跳到对应 catch body；
5. 没有 handler 时，如果函数定义了 `DeferBlock`，先跳到 defer；
6. 否则直接 `ret void`。

这表示当前实现里的 “panic 传播” 本质上是：

- **当前函数内** 用 CFG 跳转进入 catch/defer；
- **函数边界上** 用 `ctx.panic` 保存最后的异常值；
- 不是原生栈展开，也不是 LLVM `landingpad` 模型。

## 4. `recover` 和 catch 变量怎么取值

`recover()` 的 lowering 也很直接：

- `common/yak/ssa2llvm/compiler/ops_panic.go`

`compileRecover` 只做一件事：

- `load ctx.panic`

catch 变量的处理本质上也是一样的：

- YakSSA 给 catch 参数一个单独的 value id；
- 但编译器不会真的给它分配独立值；
- `getValue` 发现它是 exception id 后，直接返回 `ctx.panic` 当前值。

因此当前语义里：

- `recover()`
- `catch err` 里的 `err`

最终都共享同一个 panic 槽。

## 5. `defer` 怎么接到 return / panic 路径

`return` 的 lowering 在：

- `common/yak/ssa2llvm/compiler/ops.go`

核心策略是：

- 普通 `return` 先把结果写入 `ctx.ret`；
- 如果函数存在 `DeferBlock`，不立即 `ret void`；
- 而是统一 branch 到 defer block；
- defer 执行完之后再进入最终返回路径。

`panic` 走未捕获路径时也使用同一个策略：

- 先把 panic 值写入 `ctx.panic`
- 再跳 `DeferBlock`

所以当前实现里，`defer` 是一个 **函数级统一收尾块**：

- 所有显式 return 走它；
- 所有未被本地 catch 吸收的 panic 也走它。

这也是 `defer { x = recover() }` 能工作的基础：因为跳进 defer 前，`ctx.panic` 已经写好了。

## 6. `try-catch-finally` 的控制流

SSA 侧的 `TryBuilder` 会先把控制流拼出来：

- try body 正常结束时跳到 `finally`（如果有）或 `done`
- catch body 执行完也跳到 `finally` 或 `done`
- `finally` 结束后再进 `done`

对应实现：

- `common/yak/ssa/cfg.go`

LLVM 编译器本身不重新发明这套 CFG，只做两件事：

- 根据 block 识别当前激活的 handler；
- 在 `panic` 时补一条 branch 到 catch body。

当前实现还有限制：

- 一个 handler 下如果有多个 catch，编译器当前只记录第一段 catch body；
- 也就是说，SSA 结构支持多 catch，但 LLVM lowering 目前按“单 catch 入口”处理。

## 7. 当前行为边界

这套方案现在是可用的，但需要明确它的边界：

- 它是 **函数内 CFG lowering**，不是原生异常展开；
- 当前不会在每次普通函数调用返回后自动检查 callee 的 `panic` 槽并继续向上抛；
- invoke runtime 会把 host panic 写回当前 `ctx.panic`，并打印到 `stderr`；
- 但跨调用边界的 host/runtime panic 仍然没有完整接到 Yak 的 catch/recover 控制流；
- `recover()` 当前只是“读取当前 context 的 panic 槽”，没有完整的 Go 栈语义。

相关 runtime 代码：

- `common/yak/ssa2llvm/runtime/runtime_go/invoke.go`
- `common/yak/ssa2llvm/runtime/runtime_go/yak_lib.go`

## 8. 后续重构建议

如果后续要把错误处理做得更可维护，建议围绕 `InvokeContext` 抽象继续收口：

- 明确 `panic`、`recover`、`defer` 的 context 状态机，而不是在多个 lowering 点散落条件分支；
- 给 catch/finally/defer 建统一的 function-level metadata 结构，避免继续扩散多个 map；
- 如果未来需要跨函数传播，再在 call ABI 层显式定义“panic 返回协议”，而不是隐式依赖局部 CFG。
