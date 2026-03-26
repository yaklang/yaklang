# InvokeContext、函数调用与 go routine 执行机制

本文档说明 `ssa2llvm` 当前的统一调用 ABI：为什么所有函数调用都收敛为一个 `InvokeContext`，以及同步调用、stdlib dispatch、`go` 语句和最终 `main` wrapper 是怎么串起来的。

## 1. 统一成单参数 ABI 的原因

`ssa2llvm` 现在不再给每个 Yak 函数生成一套“真实参数列表” ABI，而是统一成：

- 编译后函数签名：`void fn(i8* ctx)`
- 统一运行时入口：`yak_runtime_invoke(void* ctx)`

这样做的目的有几个：

- 编译器侧只有一套调用 lowering 逻辑；
- 普通 Yak 函数、extern hook、stdlib dispatch 都能走同一个调用面；
- 以后扩展更多 metadata 时，不需要不断改函数签名；
- `go` 异步调用可以直接复用同步调用构造好的上下文对象。

## 2. InvokeContext 布局

布局定义在：

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

- `KindCallable` 表示 `Target` 是函数指针；
- `KindDispatch` 表示 `Target` 是 `dispatch.FuncID`；
- `Flags` 目前使用：
  - `FlagAsync`：表示是否异步执行；
  - `FlagPanicTaggedPointer`：表示 `Panic` 槽里保存的是未 tag 的指针，需要在读取时补 tag；
- `Ret` 是返回值槽；
- `Panic` 是当前函数的异常/`panic` 槽；
- `Roots` 是给 Boehm GC 看的原始指针数组，用来保活被 tag 过的对象参数。

## 3. Callee 侧怎么取参数

编译后的 Yak 函数入口会从 `InvokeContext` 反序列化参数，代码在：

- `common/yak/ssa2llvm/compiler/invoke_ctx_internal.go`

关键点：

- `bindParamsFromContext` 从 `Args` 区依次读取 `fn.Params`；
- 对 YakSSA 的 `ParameterMembers`，继续按顺序从 `Args` 后半段读取；
- 读取出的值直接放进编译器 `Values` 表，后续指令像读普通 SSA 值一样使用。

这意味着 **编译后的 Yak 函数调用** 会把 `Args` 编码为：

- 前半段：普通参数 `inst.Args`
- 后半段：参数成员 `inst.ArgMember`

对应逻辑在：

- `common/yak/ssa2llvm/compiler/ops_call.go`

## 4. Caller 侧怎么构造上下文

调用方侧的公共 helper 在：

- `common/yak/ssa2llvm/compiler/invoke_ctx_calling_internal.go`

核心流程是：

1. `allocInvokeContext(argc, ...)` 申请一块连续内存；
2. `initInvokeContext(...)` 写入 header；
3. 把参数写进 `Args`；
4. 如有需要，把未 tag 的原始指针写进 `Roots`；
5. 调用目标函数或 dispatcher；
6. 从 `ctx.ret` 读回返回值。

### 4.1 普通函数/extern hook 调用

普通 Yak 函数、extern hook、fallback 符号都会先被解析成 `KindCallable` 的 `ContextCallSpec`：

- `common/yak/ssa2llvm/compiler/ops_call.go`
- `common/yak/ssa2llvm/compiler/ops_call_context_internal.go`

同步调用路径：

- `kind = KindCallable`
- `target = ptrtoint(fn)`
- `call yak_runtime_invoke(ctx)`
- `load ctx.ret`

### 4.2 stdlib 调用

stdlib 不直接链接成一堆导出符号，而是编码成 `KindDispatch`，再走统一 invoke 入口：

- `common/yak/ssa2llvm/compiler/ops_call.go`
- `common/yak/ssa2llvm/compiler/ops_call_context_internal.go`
- `common/yak/ssa2llvm/runtime/runtime_go/stdlib.go`

调用时：

- `kind = KindDispatch`
- `target = dispatch.FuncID`
- `call yak_runtime_invoke(ctx)`
- `load ctx.ret`

打印类函数会对指针参数打 tag，并把原始指针放进 `Roots`，避免 Boehm GC 看不到真实对象。

## 5. `go` 语句怎么执行

Yak 语法层把 `go f(...)` 先转成一个普通 `Call`，然后只额外打上 `Async = true`：

- `common/yak/yak2ssa/builder_ast.go`

LLVM lowering 后：

- 所有异步调用统一先构造 `InvokeContext`
- 再把 `ctx.flags` 置为 `FlagAsync`
- 最后统一调用 `yak_runtime_invoke(ctx)`

相关实现：

- `common/yak/ssa2llvm/compiler/ops_call.go`
- `common/yak/ssa2llvm/compiler/ops_call_context_internal.go`
- `common/yak/ssa2llvm/runtime/runtime_go/invoke.go`

### 5.1 为什么 `spawn` 前要把 ctx 挂到 root 链表

goroutine 启动后，真实执行点已经不在当前 LLVM/C 栈上了。为了让 `ctx` 在异步执行期间仍然被 Boehm GC 看到，runtime 在 C 侧维护了一条 root 链：

- `common/yak/ssa2llvm/runtime/runtime_go/ctx_root.c`

过程是：

1. `yak_runtime_invoke(ctx)` 发现 `ctx.flags & FlagAsync != 0`
2. 再调用内部的异步执行路径，把 `ctx` 挂到 root 链表；
3. root 节点由 Boehm GC 管理；
4. goroutine 结束后调用 `yak_ctx_root_remove(handle)`；
5. 整个异步执行期间，`ctx` 及其 `Roots` 都保持可达。

### 5.2 goroutine 内部如何决定执行什么

goroutine 内部不会再重新走 async 分支，而是直接进入实际执行逻辑。`yak_runtime_invoke` 再根据 `ctx.kind` 分发：

- `KindCallable`：调用 `yak_invoke_callable(fn, ctx)`；
- `KindDispatch`：调用 `invokeDispatch(ctx)`；

其中 `yak_invoke_callable` 只是一个很薄的 C glue：

- `common/yak/ssa2llvm/runtime/runtime_go/invoke_callable.c`

### 5.3 异步返回值怎么处理

当前 `go` 语句不向调用点返回结果：

- 编译器把异步 `Call` 的结果值写成 `0`；
- `main` wrapper 在入口函数返回后会自动等待所有异步 goroutine；
- 如果 Yak 代码里需要更细粒度的同步，应使用 `sync.NewWaitGroup()` / `sync.NewSizedWaitGroup()`。

相关实现：

- `common/yak/ssa2llvm/runtime/runtime_go/invoke.go`
- `common/yak/ssa2llvm/runtime/runtime_go/stdlib.go`

## 6. `main` wrapper 怎么把入口函数接成可执行程序

最终的二进制入口不是 Yak 函数本身，而是编译器生成的 `main`：

- `common/yak/ssa2llvm/compiler/main_wrapper_internal.go`

wrapper 的流程：

1. 创建一个 `argc=0` 的 `InvokeContext`；
2. `kind = KindCallable`，`target = entry function pointer`；
3. 调用 `yak_runtime_invoke(ctx)`；
4. 从 `ctx.ret` 取回返回值；
5. 如配置要求，打印返回值；
6. 调用 `yak_runtime_gc()` 做一次 Boehm/Go 双 GC；
7. 截断 `ret` 为进程退出码。

## 7. 对后续重构的含义

当前统一 ABI 的价值在于：

- 新增调用类型时，优先扩 `Kind` 和 context header，而不是再加一套新参数表；
- 需要新增调用 metadata 时，优先放进 `Reserved` 或版本化 header；
- 异步、stdlib、普通函数共享同一套上下文结构，后续重构只要围绕 `InvokeContext` 做，不需要分别维护多套调用协议。
