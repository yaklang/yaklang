# ssa2llvm Project Notes

## 目标

`ssa2llvm` 的目标是把 YakSSA 编译为 LLVM IR，并进一步生成：

- LLVM IR
- 汇编
- 目标文件
- 原生可执行文件

当前主路径是 **AOT 编译 + `libyak.a` 运行时链接**。

## 目录结构

```text
common/yak/ssa2llvm/
├── cmd/           # CLI：compile / run / obfuscators
├── compiler/      # SSA → LLVM lowering、wrapper、call/error lowering、函数级编译上下文
├── docs/          # 机制文档
├── obfuscation/   # SSA / LLVM obfuscator
├── runtime/       # libyak.a、ABI 定义、Go runtime glue、embed 资源
├── scripts/       # 依赖安装、runtime 构建、CLI 构建脚本
├── tests/         # 从 IR 到最终二进制运行结果的集成测试
├── trace/         # 外部命令 trace
├── types/         # 类型与布局辅助
└── STATUS.md      # 当前实现状态
```

## 编译流水线

1. 前端把源码解析为 YakSSA
2. 编译器预创建 LLVM function / basic block / phi
3. 在函数级上下文里完成 SSA 指令 lowering
4. 注入 `main` wrapper，把 Yak 入口函数接成原生程序入口
5. 根据命令选择输出 `.ll` / `.s` / `.o` / 可执行文件
6. 通过 `clang` 把产物与 `common/yak/ssa2llvm/runtime/libyak.a`、`libgc` 链接

## 当前核心设计

### 1. 单参数 `InvokeContext` 调用 ABI

所有调用面统一成：

- 编译后的 Yak 函数
- stdlib / builtin dispatcher
- Go shadow object 方法调用
- `go` 异步启动入口

都围绕 `InvokeContext` 工作。这样普通调用、builtin 调用、异步调用共享一套参数/返回值/异常槽协议。

关键位置：

- `common/yak/ssa2llvm/compiler/invoke_ctx_internal.go`
- `common/yak/ssa2llvm/compiler/invoke_ctx_calling_internal.go`
- `common/yak/ssa2llvm/runtime/abi/abi.go`

### 2. builtin ID 与 runtime shadow method

当前 runtime ABI 里统一定义：

- context header layout
- flags / kinds
- builtin IDs

普通 stdlib/builtin 通过 `FuncID` 进入 dispatcher；Go shadow object method 则使用独立的 shadow method builtin target，再在 runtime 中通过反射实际调用。

sync 系列构造当前直接复用 `common/yak/yaklib/sync.go` 中的实现，而不是在 `ssa2llvm` runtime 内重复维护一份语义。

关键位置：

- `common/yak/ssa2llvm/compiler/externs.go`
- `common/yak/ssa2llvm/compiler/ops_call_context_internal.go`
- `common/yak/ssa2llvm/runtime/runtime_go/invoke.go`
- `common/yak/ssa2llvm/runtime/runtime_go/runtime_dispatch.go`
- `common/yak/ssa2llvm/runtime/runtime_go/runtime_object.go`
- `common/yak/ssa2llvm/runtime/runtime_go/runtime_sync.go`
- `common/yak/yaklib/sync.go`

### 3. function compile context

不是所有 lowering metadata 都应该挂在 `Compiler` 上。当前已引入函数级上下文，承载：

- 当前函数
- invoke context 参数
- return block
- error handling metadata

这让 closure/freevalue、panic/defer 之类的函数内状态不再污染全局 compiler 状态。

关键位置：

- `common/yak/ssa2llvm/compiler/function_compile_context.go`
- `common/yak/ssa2llvm/compiler/compiler.go`

### 4. closure binding / freevalue

closure 调用点会按稳定顺序把 freevalue binding 追加进 `InvokeContext.Args`；callee 侧在函数入口从相同顺序回填 freevalue SSA 值。

关键位置：

- `common/yak/ssa2llvm/compiler/call_binding_internal.go`
- `common/yak/ssa2llvm/compiler/invoke_ctx_internal.go`
- `common/yak/ssa2llvm/compiler/ops_call.go`

### 5. slice runtime 路径

当前已经实现最小 slice 运行时闭环：

- `make([]int)` / `make([]int, n)`
- 索引读写
- 越界 panic
- `append(slice, x)` builtin

slice 在 runtime 中用 shadow object 表示，而不是简单的 8-byte placeholder。

关键位置：

- `common/yak/ssa2llvm/compiler/ops_memory.go`
- `common/yak/ssa2llvm/runtime/runtime_go/runtime_slice.go`

### 6. defer / panic / recover / try-catch-finally

当前不是用 LLVM 原生异常机制，而是 lower 成：

- `InvokeContext.Panic`
- YakSSA CFG
- 函数级 `DeferBlock`

关键位置：

- `common/yak/ssa2llvm/compiler/error_handling_internal.go`
- `common/yak/ssa2llvm/compiler/ops_panic.go`
- `common/yak/ssa2llvm/compiler/ops.go`

### 7. Go 对象与 Boehm GC 的混合对象模型

runtime 里的复杂对象由 Go 持有真实状态，LLVM/C 侧只持有 shadow object；Boehm 回收 shadow object 时再反向释放 Go handle。

关键位置：

- `common/yak/ssa2llvm/runtime/runtime_go/yak_lib.go`
- `common/yak/ssa2llvm/runtime/runtime_go/c_stub.c`
- `common/yak/ssa2llvm/runtime/runtime_go/ctx_root.c`

## 测试策略

测试不是只校验 IR 是否生成，而是尽量走到：

- 编译到汇编/目标文件/可执行文件
- 链接 `libyak.a`
- 运行最终二进制
- 校验退出码和输出内容

当前重点测试包括：

- `common/yak/ssa2llvm/tests/go_stmt_test.go`
- `common/yak/ssa2llvm/tests/complex_syntax_test.go`
- `common/yak/ssa2llvm/tests/interop_test.go`
- `common/yak/ssa2llvm/tests/runtime_error_test.go`
- `common/yak/ssa2llvm/tests/loop_gc_test.go`

## 维护建议

- 新调用能力优先扩 `InvokeContext` / builtin IDs，不要重新引入多套参数 ABI
- 新 runtime method 逻辑优先放到独立 runtime 文件，不要继续混在 `invoke.go`
- 函数内 metadata 优先放进 `functionCompileContext`
- 测试优先走真实 runtime 路径，不要再复制一套 stub runtime

## 相关文档

- `common/yak/ssa2llvm/docs/dispatch-and-stdlib.md`
- `common/yak/ssa2llvm/docs/context-call-and-goroutine.md`
- `common/yak/ssa2llvm/docs/error-handling.md`
- `common/yak/ssa2llvm/docs/gc-mechanism.md`
