# SSA2LLVM GC 机制说明

本文档说明 `ssa2llvm` 当前使用的 GC 机制实现。

## 总览

`ssa2llvm` 采用 Go 对象 + C 侧影子对象（shadow object）的混合模型：

- 对象真实状态保存在 Go 对象中。
- Go 侧通过 `cgo.NewHandle(obj)` 生成 `handleID`，用它保持对象可达。
- LLVM/C 侧不直接持有 Go 指针，只持有一个很小的影子对象；影子对象里保存 `handleID`。
- 影子对象由 Boehm GC（`libgc`）分配和追踪，因此 C/LLVM 栈上的可达性可被正确识别。
- 当影子对象不可达时，Boehm finalizer 被触发，回调到 Go 执行 `handle.Delete()`。
- `handle.Delete()` 后，如果 Go 侧没有其他引用，该对象即可被 Go GC 回收。

这形成了完整闭环：Go 创建对象并交给 `cgo.Handle` 保活，C 侧用 Boehm 管理可达性，Boehm 回收时再释放 `cgo.Handle`，最终允许 Go GC 回收对象。

## 运行时组件

- 运行时静态库：`common/yak/ssa2llvm/runtime/libyak.a`
- Go 运行时代码：`common/yak/ssa2llvm/runtime/runtime_go/yak_lib.go`
- Finalizer 代理（C）：`common/yak/ssa2llvm/runtime/runtime_go/c_stub.c`

核心导出 API：

- `yak_runtime_new_shadow`
- `yak_runtime_get_field`
- `yak_runtime_set_field`
- `yak_runtime_dump_handle`
- `yak_runtime_gc`

## 对象分配与生命周期

1. Go 侧（例如 stdlib 实现）创建 Go 对象，并存入 `cgo.Handle` 得到 `handleID`。
2. Go 侧调用 `yak_runtime_new_shadow(handleID)`，用 `GC_malloc` 分配影子对象内存（当前为 16 字节：magic + handleID）。
3. 影子对象里保存 `handleID`（`uintptr_t`），同时在 Go 侧的 `shadowHandles` map 里登记一份（避免对潜在的无效指针解引用）。
4. 通过 `GC_register_finalizer` 注册 Boehm finalizer。
5. 影子对象被回收时：
   - C 回调 `yak_finalizer_proxy` 被触发；
   - 回调进入 Go 的 `yak_internal_release_shadow`；
   - 由 `yak_host_release_handle` 删除对应 `cgo.Handle`，释放 Go 侧引用。

## 编译与链接

- 运行时构建脚本：
  - `common/yak/ssa2llvm/scripts/build_runtime_go.sh`
- 最终二进制链接（`CompileLLVMToBinary`）包含：
  - `-lyak`（运行时库）
  - `-lgc`（Boehm GC）
  - `-lpthread -ldl`

## 生成程序中的 GC 触发

在 `compiler/api.go` 中，会注入 `main` wrapper。执行完 Yak 入口函数后，会调用 `yak_runtime_gc()`：

- `GC_gcollect()`：触发 Boehm GC 回收
- `runtime.GC()`：触发 Go GC 回收
- 短暂 sleep：用于测试场景下让 finalizer 调度更稳定

## Ubuntu/Debian 依赖

可通过脚本安装：

- `./common/yak/ssa2llvm/scripts/install_deps_ubuntu.sh`

当前依赖包：

- `llvm-dev`
- `libclang-dev`
- `zlib1g-dev`
- `libzstd-dev`
- `libgc-dev`

## 边界与注意事项

- 该机制只覆盖“走影子对象路径”的对象生命周期管理，其他所有权模型需要单独设计。
- `GCLOG` 日志主要用于调试和测试，不应作为生产环境稳定输出约定。
