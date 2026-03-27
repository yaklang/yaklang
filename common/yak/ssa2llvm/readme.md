# ssa2llvm

`ssa2llvm` 负责把 YakSSA lowering 到 LLVM IR，并继续产出汇编、目标文件和原生可执行文件。当前主路径是 **AOT 编译 + `libyak.a` 运行时链接**，不再依赖早期的 JIT-only 试验路径。

当前统一约定：

- 调用 ABI 统一为单参数 `InvokeContext`
- 普通 Yak 函数、builtin/stdlib、runtime shadow method、`go` 异步调用都经由 `yak_runtime_invoke`
- 复杂对象通过 Go object + shadow object + Boehm GC 混合模型运行
- 测试以“最终二进制可运行且输出正确”为准，不再依赖 hook/stub 复制运行时行为

## 依赖安装

在 Ubuntu/Debian 上执行：

```bash
./common/yak/ssa2llvm/scripts/install_deps_ubuntu.sh
```

脚本会安装 `ssa2llvm` 运行和测试所需依赖（LLVM、clang、zlib/zstd、libgc 等）。

## 构建运行时静态库

链接原生二进制前，需要先准备 `libyak.a`：

```bash
./common/yak/ssa2llvm/scripts/build_runtime_go.sh
```

产物位置：

- `common/yak/ssa2llvm/runtime/libyak.a`

## 构建 CLI

```bash
go build -o ./ssa2llvm ./common/yak/ssa2llvm/cmd
```

常用命令示例：

```bash
./ssa2llvm compile demo.yak
./ssa2llvm compile demo.yak --emit-llvm
./ssa2llvm compile demo.yak --emit-asm
./ssa2llvm compile demo.yak -c
./ssa2llvm run demo.yak
```

## 运行时交付模式

### 1. 默认模式：链接本地 `libyak.a`

适合源码仓库内开发与测试。编译前先执行 `build_runtime_go.sh` 即可。

### 2. 嵌入 runtime 到 CLI

```bash
./common/yak/ssa2llvm/scripts/build_runtime_embed.sh
go build -tags ssa2llvm_gzip_embed -o ./ssa2llvm ./common/yak/ssa2llvm/cmd
```

会在 `common/yak/ssa2llvm/runtime/embed/` 下生成：

- `ssa2llvm-runtime.tar.gz`
- `ssa2llvm-runtime-src.tar.gz`

### 3. 现场编译 runtime：`--stdlib-compile`

```bash
./ssa2llvm compile demo.yak --stdlib-compile
```

该模式会释放 `ssa2llvm-runtime-src.tar.gz`，再执行 `go build -buildmode=c-archive` 生成临时 `libyak.a`，最后进入 clang 链接阶段。

## 当前覆盖的关键能力

- 普通函数调用、递归调用、`go` 异步调用
- `defer` / `panic` / `recover` / `try-catch-finally`
- `sync.NewWaitGroup` / `NewSizedWaitGroup` / `NewLock` / `NewMutex` / `NewRWMutex`
- `sync.NewMap` / `NewPool` / `NewOnce` / `NewCond` 构造入口
- Go shadow object 反射方法分发
- `make([]int)` / `make([]int, n)` / `append(a, x)` 这类 slice 基础能力
- closure freevalue / parameter capture 基线

## 测试与验证

建议在 worktree 内准备独立 DB 目录再跑测试：

```bash
mkdir -p .db
export YAKIT_HOME="$PWD/.db"
go test ./common/yak/ssa2llvm/... -count=1
```

如果出现 `runtime library not found`，通常说明还没先构建 `common/yak/ssa2llvm/runtime/libyak.a`。

## 机制文档

- builtin ID、stdlib 与 runtime shadow method：`common/yak/ssa2llvm/docs/dispatch-and-stdlib.md`
- `InvokeContext`、函数调用、closure binding 与 goroutine：`common/yak/ssa2llvm/docs/context-call-and-goroutine.md`
- `defer` / `panic` / `recover` / `try-catch-finally`：`common/yak/ssa2llvm/docs/error-handling.md`
- GC、shadow object、async roots：`common/yak/ssa2llvm/docs/gc-mechanism.md`

## 关键目录

- `common/yak/ssa2llvm/compiler`：SSA → LLVM lowering、wrapper、call/error lowering、函数级编译上下文
- `common/yak/ssa2llvm/runtime`：`libyak.a`、ABI 定义、Go runtime glue、embed 资源
- `common/yak/ssa2llvm/tests`：从 IR 到最终二进制输出的集成测试
- `common/yak/ssa2llvm/obfuscation`：SSA/LLVM obfuscator 注册与实现
