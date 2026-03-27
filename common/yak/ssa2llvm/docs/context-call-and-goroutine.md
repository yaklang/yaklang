# InvokeContext、函数调用、closure binding 与 goroutine 执行机制

本文档说明 `ssa2llvm` 当前的统一调用 ABI：为什么函数调用都收敛为一个 `InvokeContext`，同步调用、builtin dispatch、closure binding、`go` 异步调用和 `main` wrapper 是怎么串起来的。

## 1. 为什么统一成单参数 ABI

`ssa2llvm` 不再给每个 Yak 函数生成一套真实参数列表 ABI，而是统一成：

- 编译后函数签名：`void fn(i8* ctx)`
- 统一运行时入口：`yak_runtime_invoke(void* ctx)`

这样做的目的：

- 编译器只有一套调用 lowering 逻辑
- 普通 Yak 函数、builtin/stdlib、runtime shadow method 都能复用同一入口
- 后续扩元数据时不需要反复改函数签名
- `go` 异步调用直接复用同步调用构造好的上下文对象

## 2. InvokeContext 布局

定义位置：

- `common/yak/ssa2llvm/runtime/abi/abi.go`

当前 `InvokeContext` 是一段按 `i64` 对齐的连续内存：

- `[0] Magic`
- `[1] Version`
- `[2] Kind`
- `[3] Flags`
- `[4] Target`
- `[5] Argc`
- `[6] Ret`
- `[7] Panic`
- `[8] Reserved0`
- `[9] Reserved1`
- `[10...] Args`
- `[10+argc...] Roots`

其中：

- `KindCallable`：`Target` 是函数指针
- `KindDispatch`：`Target` 是 builtin `FuncID`
- `FlagAsync`：表示异步执行
- `FlagPanicTaggedPointer`：表示 `Panic` 槽里保存的是未 tag 指针，需要读出时补 tag
- `Roots`：给 Boehm GC 看的原始指针数组，用于保活被 tag 的对象参数

## 3. caller 侧如何构造上下文

调用侧 helper 在：

- `common/yak/ssa2llvm/compiler/invoke_ctx_calling_internal.go`
- `common/yak/ssa2llvm/compiler/ops_call_context_internal.go`

核心流程：

1. `allocInvokeContext(argc, ...)` 申请连续内存
2. `initInvokeContext(...)` 写 header
3. 把参数写入 `Args`
4. 必要时把未 tag 指针写入 `Roots`
5. 调用 `yak_runtime_invoke(ctx)`
6. 同步路径再从 `ctx.ret` 读回结果

### 3.1 普通函数 / 本地方法 / callable extern

- 本地 Yak 函数调用走 `KindCallable`
- object-factor 的本地方法调用也走 `KindCallable`
- direct extern symbol 绑定仍然走 `KindCallable`

相关代码：

- `common/yak/ssa2llvm/compiler/ops_call.go`

### 3.2 builtin dispatch

- `println` / `sync.NewWaitGroup` / `append` 等会被编码成 `KindDispatch`
- `target = abi.FuncID`
- runtime 再根据 `FuncID` 真正执行

## 4. callee 侧如何取参数

callee 入口读取逻辑在：

- `common/yak/ssa2llvm/compiler/invoke_ctx_internal.go`

当前读取顺序是稳定的：

1. `fn.Params`
2. `fn.ParameterMembers`
3. ordered freevalue bindings

这也是 closure/binding 能工作的基础：caller 和 callee 必须共享同一个稳定顺序。

## 5. closure binding / freevalue

当前 closure 基线实现位置：

- `common/yak/ssa2llvm/compiler/call_binding_internal.go`
- `common/yak/ssa2llvm/compiler/invoke_ctx_internal.go`
- `common/yak/ssa2llvm/compiler/ops_call.go`

机制是：

- call-site 按稳定排序把 `inst.Binding` 中的 freevalue 追加进 `Args`
- callee 在 `bindParamsFromContext` 阶段按同顺序回填 freevalue SSA ID

当前已经覆盖：

- outer local capture
- parameter capture
- 基础 object-factor closure 场景

## 6. function compile context

不是所有 lowering metadata 都应挂在 `Compiler` 上。当前已经把函数内状态收束进函数级上下文：

- 当前函数
- invoke context 参数
- return block
- error handling metadata

位置：

- `common/yak/ssa2llvm/compiler/function_compile_context.go`
- `common/yak/ssa2llvm/compiler/compiler.go`

## 7. `go` 语句怎么执行

Yak 前端把 `go f(...)` 先转成普通 `Call`，额外只打上 `Async = true`。

LLVM lowering 后：

- 先构造普通 `InvokeContext`
- 再把 `ctx.flags` 置为 `FlagAsync`
- 仍统一调用 `yak_runtime_invoke(ctx)`

runtime 侧在：

- `common/yak/ssa2llvm/runtime/runtime_go/invoke.go`
- `common/yak/ssa2llvm/runtime/runtime_go/ctx_root.c`

关键点：

- async 调用会把 `ctx` 挂到 C 侧 root 链表
- goroutine 结束后再移除 root
- `main` wrapper 在入口函数返回后统一 `yak_runtime_wait_async()`

## 8. main wrapper

`main` wrapper 在：

- `common/yak/ssa2llvm/compiler/main_wrapper_internal.go`

流程：

1. 创建 `argc=0` 的 `InvokeContext`
2. `kind = KindCallable`，`target = entry function pointer`
3. `yak_runtime_invoke(ctx)`
4. 读取 `ctx.ret`
5. 如配置要求打印返回值
6. `yak_runtime_wait_async()`
7. `yak_runtime_gc()`
8. 返回进程退出码
