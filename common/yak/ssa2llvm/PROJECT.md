# ssa2llvm Project Notes

## 目标

`ssa2llvm` 的目标是把 YakSSA 编译为 LLVM IR，并进一步生成：

- LLVM IR
- 汇编
- 目标文件
- 原生可执行文件

当前主路径是 **AOT 编译 + `libyak.a` 运行时链接**，而不是早期的 JIT-only 实验路径。

## 目录结构

```
common/yak/ssa2llvm/
├── cmd/           # CLI：compile / run / obfuscators
├── compiler/      # SSA → LLVM lowering、linker、wrapper、InvokeContext ABI
├── docs/          # 机制文档
├── obfuscation/   # SSA / LLVM obfuscator
├── runtime/       # libyak.a、dispatch ids、runtime glue、embed 资源
├── scripts/       # 依赖安装、runtime 构建、CLI 构建脚本
├── tests/         # 从 IR 到最终二进制运行结果的集成测试
├── trace/         # 外部命令 trace
├── types/         # 类型与布局辅助
└── STATUS.md      # 当前实现状态
```

## 编译流水线

1. 前端把源码解析为 YakSSA；
2. `compiler` 预创建 LLVM function / basic block / phi；
3. 各类 SSA 指令 lowering 到 LLVM；
4. 注入 `main` wrapper，把 Yak 入口函数接成原生程序入口；
5. 根据命令选择输出 `.ll` / `.s` / `.o` / 可执行文件；
6. 需要链接时，通过 `clang` 把产物和 `common/yak/ssa2llvm/runtime/libyak.a`、`libgc` 接起来。

## 当前核心设计

### 1. 单参数 `InvokeContext` 调用 ABI

所有调用面统一成：

- 编译后的 Yak 函数
- extern hook
- stdlib dispatcher
- `go` 异步启动入口

都围绕 `InvokeContext` 工作。这样普通调用、dispatch 调用、异步调用共享同一套参数/返回值/异常槽协议。

参考：

- `common/yak/ssa2llvm/compiler/invoke_ctx_internal.go`
- `common/yak/ssa2llvm/compiler/invoke_ctx_calling_internal.go`
- `common/yak/ssa2llvm/runtime/abi/abi.go`

### 2. stdlib 统一 dispatch

标准库函数不直接导出成大量 runtime 符号，而是先绑定稳定的 `dispatch.FuncID`，再统一调用：

- `yak_runtime_dispatch`

参考：

- `common/yak/ssa2llvm/compiler/externs.go`
- `common/yak/ssa2llvm/compiler/ops_call_dispatch.go`
- `common/yak/ssa2llvm/runtime/runtime_go/stdlib.go`

### 3. `go` 语句走 runtime spawn

Yak 的 `go` 语句会把 `ssa.Call.Async` 置位，编译后统一走：

- `yak_runtime_spawn(ctx)`

runtime 再根据 `ctx.kind` 选择执行 callable 还是 dispatch，并用 C 侧 root 链表让 Boehm GC 在异步期间仍能看到 `ctx`。

参考：

- `common/yak/yak2ssa/builder_ast.go`
- `common/yak/ssa2llvm/runtime/runtime_go/spawn.go`
- `common/yak/ssa2llvm/runtime/runtime_go/ctx_root.c`

### 4. `defer` / `panic` / `recover` / `try-catch-finally`

当前不是用 LLVM 原生异常机制，而是把错误处理 lower 成：

- `InvokeContext.Panic`
- YakSSA CFG
- 函数级 `DeferBlock`

参考：

- `common/yak/ssa2llvm/compiler/error_handling_internal.go`
- `common/yak/ssa2llvm/compiler/ops_panic.go`
- `common/yak/ssa2llvm/compiler/ops.go`
- `common/yak/ssa/cfg.go`

### 5. Go 对象与 Boehm GC 的混合对象模型

运行时里的复杂对象由 Go 持有真实状态，LLVM/C 侧只持有 shadow object；Boehm 回收 shadow object 时再反向释放 Go handle。

参考：

- `common/yak/ssa2llvm/runtime/runtime_go/yak_lib.go`
- `common/yak/ssa2llvm/runtime/runtime_go/c_stub.c`

## 测试策略

当前测试不是只校验 IR 是否生成，还会尽量走到：

- 编译到汇编/目标文件/可执行文件
- 链接 `libyak.a`
- 运行最终二进制
- 校验退出码和输出内容

重点测试目录：

- `common/yak/ssa2llvm/tests/go_stmt_test.go`
- `common/yak/ssa2llvm/tests/complex_syntax_test.go`
- `common/yak/ssa2llvm/tests/extern_hook_test.go`
- `common/yak/ssa2llvm/tests/print_test.go`
- `common/yak/ssa2llvm/tests/struct_test.go`

## 维护建议

后续继续演进时，优先遵守下面几条：

- 新调用能力优先扩 `InvokeContext`，不要重新引入多套函数参数 ABI；
- 新 stdlib 能力优先补 `dispatch id + runtime dispatcher`，不要散落很多导出符号；
- defer/panic/recover 相关状态优先收束到统一 metadata/上下文结构，不要继续扩散大量平行参数；
- 测试保持“最终二进制可运行且输出正确”为准，而不是只看 IR 生成成功。

## 相关文档

- `common/yak/ssa2llvm/docs/dispatch-and-stdlib.md`
- `common/yak/ssa2llvm/docs/context-call-and-goroutine.md`
- `common/yak/ssa2llvm/docs/error-handling.md`
- `common/yak/ssa2llvm/docs/gc-mechanism.md`
