# SSA2LLVM GC 机制说明

本文档说明 `ssa2llvm` 当前使用的 GC 机制实现。

## 1. 总览

`ssa2llvm` 采用 Go 对象 + C 侧 shadow object 的混合模型：

- 对象真实状态保存在 Go 对象里
- Go 侧通过 `cgo.NewHandle(obj)` 持有对象
- LLVM/C 侧不直接持有 Go 指针，只持有 shadow object
- shadow object 由 Boehm GC（`libgc`）分配和追踪
- shadow object 被回收时，再反向删除 Go handle

这样可以同时满足：

- C/LLVM 栈上的可达性由 Boehm 负责
- Go 对象生命周期由 `cgo.Handle` 间接管理

## 2. 运行时组件

核心位置：

- `common/yak/ssa2llvm/runtime/libyak.a`
- `common/yak/ssa2llvm/runtime/runtime_go/yak_lib.go`
- `common/yak/ssa2llvm/runtime/runtime_go/c_stub.c`
- `common/yak/ssa2llvm/runtime/runtime_go/ctx_root.c`

关键导出 API：

- `yak_runtime_new_shadow`
- `yak_runtime_get_field`
- `yak_runtime_set_field`
- `yak_runtime_gc`
- `yak_runtime_wait_async`
- `yak_runtime_make_slice`

## 3. shadow object 生命周期

1. runtime 创建 Go 对象，并放入 `cgo.Handle`
2. 调用 `yak_runtime_new_shadow(handleID)`，通过 `GC_malloc` 分配 shadow object
3. shadow object 内保存 magic + handleID
4. Go 侧 `shadowHandles` map 也登记一份，避免对野指针解引用
5. `GC_register_finalizer` 注册 Boehm finalizer
6. shadow object 被回收时：
   - C 回调 `yak_finalizer_proxy`
   - 进入 Go 的 `yak_internal_release_shadow`
   - 最终调用 `yak_host_release_handle` 删除 `cgo.Handle`

## 4. 当前哪些值走 shadow object

当前主要包括：

- sync 系列 runtime object
- runtime 里的 slice object
- 运行时返回的复杂对象
- 被 tagged pointer 协议包装并需要跨边界传递的 Go 值

Yak object-factor 自己的纯 Yak 对象不一定都走 shadow object；shadow object 主要用于 runtime/Go 持有真实状态的值。

## 5. roots 与 tagged pointer

LLVM 侧当前仍大量使用 `i64` 传值，因此：

- 某些参数会被编码成 tagged pointer
- 为了让 Boehm GC 看到真实未 tag 指针，会把原始指针写入 `InvokeContext.Roots`

这主要影响：

- print / printf / println
- append 等会跨 runtime 边界返回复杂值的 builtin
- 某些 runtime shadow object 调用参数

## 6. async ctx roots

`go` 异步调用时，`InvokeContext` 不能只依赖当前 C/LLVM 栈存活。

因此 runtime 在 C 侧维护了一条 ctx root 链：

- async 调用启动前把 `ctx` 挂入 root 链
- goroutine 结束后再移除 root
- root 挂住期间，`ctx` 及其 `Roots` 都对 Boehm 可见

这保证了异步期间不会把仍在执行的上下文对象误回收。

## 7. main wrapper 中的 GC 触发

`main` wrapper 执行完 Yak 入口函数后，会调用：

- `yak_runtime_wait_async()`
- `yak_runtime_gc()`

其中 `yak_runtime_gc()` 会做：

- `GC_gcollect()`
- `runtime.GC()`
- 一个很短的 sleep，让测试环境里 finalizer 更稳定

这也是为什么很多集成测试能稳定看到 `Releasing handle` 日志。

## 8. 依赖与构建

依赖安装：

```bash
./common/yak/ssa2llvm/scripts/install_deps_ubuntu.sh
```

运行时构建：

```bash
./common/yak/ssa2llvm/scripts/build_runtime_go.sh
```

关键依赖通常包括：

- `llvm-dev`
- `libclang-dev`
- `zlib1g-dev`
- `libzstd-dev`
- `libgc-dev`

## 9. 注意事项

- `GCLOG` 主要用于调试和测试，不应作为稳定输出协议
- 当前 GC 机制只覆盖走 shadow object 路径的值
- 如果未来引入新的复杂值模型，必须先明确它是否纳入 shadow object / root 可达性协议
