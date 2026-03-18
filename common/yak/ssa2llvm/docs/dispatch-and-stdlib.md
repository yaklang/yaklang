# Dispatch ID 与 stdlib/libyak 机制说明

本文档说明 `ssa2llvm` 里“stdlib 调用”的整体机制：编译器如何把 `println/print/os.Getenv/poc.*` 等函数调用 lowering 为一个统一的 runtime dispatcher（通过 `dispatch id`），以及最终如何与 `libyak.a` 链接并在二进制里运行。

---

## 1. 设计目标：把 stdlib 变成一个入口

`ssa2llvm` 的目标之一是减少最终二进制里可读/可枚举的导出符号数量，并且让编译器侧的调用 lowering 稳定、可扩展。

因此 stdlib 调用不直接链接到一堆 `yak_stdlib_xxx` 符号，而是统一为一个 dispatcher：

- `yak_runtime_dispatch(ctx)`（见 `common/yak/ssa2llvm/runtime/runtime_go/stdlib.go`）

编译器只需要把“我要调用哪个 stdlib 函数”编码为一个稳定的整数 ID，然后把参数打包进 `InvokeContext` 即可。

---

## 2. dispatch id：稳定的函数编号

dispatch id 定义在：

- `common/yak/ssa2llvm/runtime/dispatch/ids.go`

里面包含：

- `dispatch.FuncID`：函数 ID 的类型（`int64`）
- `dispatch.DispatcherSymbol`：dispatcher 符号名（当前为 `yak_runtime_dispatch`）
- 一组稳定的常量 ID（`IDPrint/IDPrintln/IDOsGetenv/...`）

### 稳定性约束

这些 ID 一旦发布，就应该保持稳定；否则旧的 IR/二进制可能会调用到错误的 runtime 分支。

---

## 3. 编译器侧：从函数名绑定到 DispatchID

编译器的 name→binding 映射在：

- `common/yak/ssa2llvm/compiler/externs.go`

`ExternBinding` 里有 `DispatchID dispatch.FuncID` 字段：

- 当 `DispatchID != 0` 时，该函数调用会走 stdlib dispatcher lowering。
- 例如 `println/print/printf/os.Getenv/poc.*` 都在默认 bindings 中绑定了对应的 ID。

一个典型流程：

1. SSA 里出现 `Call` 指令，callee 名称解析为 `println`
2. 查到 `ExternBinding{DispatchID: dispatch.IDPrintln}`
3. lowering 为：构造 `InvokeContext(kind=Dispatch, target=IDPrintln, args=...)`，然后调用 `yak_runtime_dispatch(ctx)`

相关 lowering 代码：

- `common/yak/ssa2llvm/compiler/ops_call_dispatch.go`

---

## 4. Runtime 侧：yak_runtime_dispatch(ctx) 如何执行

runtime dispatcher 定义在：

- `common/yak/ssa2llvm/runtime/runtime_go/stdlib.go`

核心步骤（概念上）：

1. 读取 `ctx.kind`，必须是 `abi.KindDispatch`
2. 读取 `ctx.target`，把它当作 `dispatch.FuncID`
3. 读取 `ctx.argc` 和参数数组
4. `switch(id)` 调到具体实现（如 `stdlibPrintln/stdlibOsGetenv/...`）
5. 将返回值写回 `ctx.ret`

注意：

- 目前所有值在 LLVM 侧统一用 `i64` 表示；runtime 需要按照约定对“被标记的指针值”做解码（详见下一节）。

---

## 5. Print 类调用的指针 Tag 与 Roots

为了在 LLVM 中把“指针值/字符串”塞进 `i64`，并且让 runtime 能识别哪些参数应该当作字符串/指针来解释，`ssa2llvm` 对部分 stdlib（例如 `print/println/printf/yakit.*`）使用指针 tag 机制：

- `yakTaggedPointerMask = 1<<62`（见 runtime 侧实现）
- 编译器在 lowering 时，若某个参数 SSA 类型被判定为“指针样式”（string/object/map/slice/struct/ptr），且该 stdlib 需要 tag，则会对参数做 `or mask`。

同时，为了解决 **Boehm GC 只能扫描 C/LLVM 的可达性，无法扫描 Go 栈** 的问题，`InvokeContext` 还带有 `roots` 区域：

- 对于被 tag 的指针参数，编译器会把 **未 tag 的原始指针** 写入 `roots[i]`，以便 Boehm GC 能看到真实指针，避免误回收。

这部分布局定义在：

- `common/yak/ssa2llvm/runtime/abi/abi.go`

---

## 6. libyak.a：runtime 如何进入最终二进制

`ssa2llvm` 的 runtime 以静态库形式提供：

- `common/yak/ssa2llvm/runtime/libyak.a`

该库主要由 `runtime/runtime_go/*.go`（以及少量 C glue）构建得到，导出给 LLVM/clang 链接使用。

构建脚本：

- `common/yak/ssa2llvm/scripts/build_runtime_go.sh`

链接逻辑：

- `common/yak/ssa2llvm/compiler/linker.go`（通过 `clang` 把 `.ll` + `libyak.a` + `-lgc -lpthread -ldl ...` 链到一起）

---

## 7. 如何新增一个 stdlib dispatch 函数

最小步骤：

1. 在 `common/yak/ssa2llvm/runtime/dispatch/ids.go` 增加一个稳定的 `FuncID`
2. 在 `common/yak/ssa2llvm/runtime/runtime_go/stdlib.go` 的 `switch(id)` 里实现对应分支，并通过 `ctxSetRet` 写回返回值
3. 在 `common/yak/ssa2llvm/compiler/externs.go` 增加 name→`DispatchID` 的绑定
4. 如需对参数做 tag（例如打印类），在 `common/yak/ssa2llvm/compiler/ops_call_dispatch.go` 的 `shouldTagStdlibArgPointers` 增加对应 ID

