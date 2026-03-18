# ssa2llvm

`ssa2llvm` 负责把 YakSSA lowering 到 LLVM IR，并进一步产出：

- LLVM IR（`.ll`）
- 汇编（`.s`）
- 目标文件（`.o`）
- 原生可执行文件

链接阶段默认会接入 `common/yak/ssa2llvm/runtime/libyak.a` 和 `libgc`。当前调用 ABI 已统一为单参数 `InvokeContext`，普通函数调用、stdlib dispatch、`go` 异步调用都走同一套协议。

## 依赖安装

在 Ubuntu/Debian 上执行：

```bash
./common/yak/ssa2llvm/scripts/install_deps_ubuntu.sh
```

脚本会安装 `ssa2llvm` 运行和测试所需依赖（LLVM、clang 相关头文件、zlib/zstd、libgc）。

## 构建运行时静态库

链接原生二进制前，需要先准备 `libyak.a`：

```bash
./common/yak/ssa2llvm/scripts/build_runtime_go.sh
```

产物位置：

- `common/yak/ssa2llvm/runtime/libyak.a`

兼容脚本仍保留在：

- `common/yak/ssa2llvm/runtime/build_runtime_go.sh`

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

如果发布 CLI 时不想依赖外置 `libyak.a`，可以先打包嵌入资源：

```bash
./common/yak/ssa2llvm/scripts/build_runtime_embed.sh
go build -tags ssa2llvm_gzip_embed -o ./ssa2llvm ./common/yak/ssa2llvm/cmd
```

这会在 `common/yak/ssa2llvm/runtime/embed/` 下生成：

- `ssa2llvm-runtime.tar.gz`
- `ssa2llvm-runtime-src.tar.gz`

### 3. 现场编译 runtime：`--stdlib-compile`

发布版 CLI 没有本地 yaklang 源码目录时，可在编译 Yak 文件时现场恢复 runtime 源码并构建：

```bash
./ssa2llvm compile demo.yak --stdlib-compile
```

该模式会释放 `ssa2llvm-runtime-src.tar.gz`，再执行 `go build -buildmode=c-archive` 生成临时 `libyak.a`，最后进入 clang 链接阶段。

## 测试与验证

建议在 worktree 内准备独立 DB 目录再跑测试：

```bash
mkdir -p .db
export YAKIT_HOME="$PWD/.db"
go test ./common/yak/ssa2llvm/... -count=1
```

如果出现 `runtime library not found`，通常说明还没先构建 `common/yak/ssa2llvm/runtime/libyak.a`。

## 机制文档

- dispatch id 与 stdlib/libyak：`common/yak/ssa2llvm/docs/dispatch-and-stdlib.md`
- `InvokeContext`、函数调用与 goroutine：`common/yak/ssa2llvm/docs/context-call-and-goroutine.md`
- `defer` / `panic` / `recover` / `try-catch-finally`：`common/yak/ssa2llvm/docs/error-handling.md`
- GC 与 shadow object：`common/yak/ssa2llvm/docs/gc-mechanism.md`

## 关键目录

- `common/yak/ssa2llvm/compiler`：LLVM lowering、linker、wrapper、调用与错误处理
- `common/yak/ssa2llvm/runtime`：`libyak.a`、dispatch ABI、Go runtime glue
- `common/yak/ssa2llvm/tests`：从 IR 到最终二进制运行结果的集成测试
- `common/yak/ssa2llvm/obfuscation`：SSA/LLVM obfuscator 注册与实现
