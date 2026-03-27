# Builtin ID、stdlib 与 runtime shadow method 机制说明

本文档说明 `ssa2llvm` 当前的 builtin/runtime 调用模型：编译器如何把 `println/print/os.Getenv/poc.*`、`sync.*`、`append` 等调用 lowering 为统一的 builtin ID 分发，以及 Go shadow object 方法如何通过单独的 runtime shadow method 路径执行。

## 1. 设计目标

当前的目标不是导出一堆独立 runtime 符号，而是统一收敛到：

- `yak_runtime_invoke(ctx)` 作为唯一公共运行时入口
- `InvokeContext.Kind + Target(FuncID)` 作为 builtin/runtime dispatch 信息

这样做的好处：

- 编译器侧调用 lowering 只有一套入口
- builtin 增量扩展只需要加稳定 ID 和 runtime 分支
- 最终二进制里的公开调用面更小

## 2. builtin ID 定义

builtin ID 现在定义在：

- `common/yak/ssa2llvm/runtime/abi/abi.go`

这里统一包含：

- `FuncID`
- 一组稳定的 builtin ID 常量
- `InvokeContext` header layout

当前已覆盖：

- `print / printf / println`
- `yakit.*`
- `os.Getenv`
- `poc.*`
- `sync.NewWaitGroup / NewSizedWaitGroup / NewLock / NewMutex / NewRWMutex`
- `sync.NewMap / NewPool / NewOnce / NewCond`
- `append`
- runtime shadow method dispatch

这些 ID 一旦发布，应保持稳定；否则旧 IR/旧二进制会调用到错误分支。

## 3. 编译器侧绑定

name → builtin ID 的绑定在：

- `common/yak/ssa2llvm/compiler/externs.go`

`ExternBinding` 当前有两个主要用途：

- `DispatchID != 0`：走 builtin ID dispatcher
- `Symbol != ""`：走 callable extern

例如：

1. SSA `Call` 解析到 `println`
2. 命中 `ExternBinding{DispatchID: abi.IDPrintln}`
3. 编译器构造 `InvokeContext(kind=Dispatch, target=IDPrintln, args=...)`
4. 调用 `yak_runtime_invoke(ctx)`

对应 lowering 代码：

- `common/yak/ssa2llvm/compiler/ops_call.go`
- `common/yak/ssa2llvm/compiler/ops_call_context_internal.go`

## 4. runtime 侧如何执行 builtin

runtime dispatcher 位于：

- `common/yak/ssa2llvm/runtime/runtime_go/invoke.go`
- `common/yak/ssa2llvm/runtime/runtime_go/stdlib.go`
- `common/yak/ssa2llvm/runtime/runtime_go/sync_runtime.go`
- `common/yak/ssa2llvm/runtime/runtime_go/slice_runtime.go`

其中 sync 构造优先复用：

- `common/yak/yaklib/sync.go`

这样可以和 Yak 解释执行侧的 sync 语义保持一致，并直接复用 `yaklib` 中的公开实现。

当 `ctx.kind == abi.KindDispatch` 时，runtime 会：

1. 从 `ctx.target` 读取 `abi.FuncID`
2. 从 `ctx.argc` 与参数区读取参数
3. 在 `switch(id)` 中进入对应 builtin 实现
4. 把返回值写回 `ctx.ret`

当前 builtin 里，`append` 已经是一个真实 runtime builtin，而不是测试 stub 或 CLI 特判。

## 5. runtime shadow method

Go shadow object 的方法调用与普通 stdlib 不同：

- 它不是 Yak object-factor 本地方法
- 它也不是一堆按类型手写 `switch`

当前路径是：

- 编译器把这类调用 lowering 为 `abi.IDRuntimeShadowMethod`
- runtime 在 `runtime_method.go` 中通过反射：
  - 找 method
  - 解码参数
  - 处理 variadic
  - 规范化返回值/错误

这条路径当前主要服务于：

- `sync.NewLock()` / `NewMutex()` / `NewRWMutex()` 返回的 shadow object
- `sync.NewWaitGroup()` / `NewSizedWaitGroup()` 返回的 shadow object

Yak 自己的 object-factor `a.set()` / `a.get()` 仍然走本地函数调用 lowering，不经过这条 runtime shadow method 路径。

## 6. 指针 tag 与 roots

为了让 LLVM 侧仍然用 `i64` 传值，同时又能正确传递字符串/对象指针，`ssa2llvm` 对部分 builtin 参数使用 tagged pointer 机制。

当前关键点：

- 编译器对需要保活/按指针解释的参数加 tag
- 原始未 tag 指针写入 `InvokeContext.Roots`
- runtime 解码 tagged pointer 后恢复 Go/shadow object 或 C string

相关定义：

- `common/yak/ssa2llvm/runtime/abi/abi.go`
- `common/yak/ssa2llvm/compiler/ops_call_context_internal.go`

## 7. 如何新增 builtin

最小步骤：

1. 在 `common/yak/ssa2llvm/runtime/abi/abi.go` 增加稳定 `FuncID`
2. 在相应 runtime 文件实现逻辑，并在 `invoke.go` 的 dispatcher 中接入
3. 在 `common/yak/ssa2llvm/compiler/externs.go` 增加 name → `DispatchID` 绑定
4. 如参数需要 tagged pointer，在 `shouldTagStdlibArgPointers` 中补该 ID

如果新增的是 **Go shadow object 方法**，优先复用 `runtime_method.go` 的反射分发，而不是继续在 `invoke.go` 里加类型特判。
