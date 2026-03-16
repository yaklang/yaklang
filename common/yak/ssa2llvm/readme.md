# YAKSSA to LLVM

## 依赖安装

在 Ubuntu/Debian 上执行：

```bash
./scripts/install_deps_ubuntu.sh
```

脚本会安装 `ssa2llvm` 运行和测试所需依赖（LLVM、clang 相关头文件、zlib/zstd、libgc）。

## 生成 runtime

`ssa2llvm` 链接阶段依赖 `runtime/libyak.a`，在运行编译/测试前需要先构建它。

使用 Go 运行时实现：

```bash
./scripts/build_runtime_go.sh
```

运行时静态库中包含 `ssa2llvm` 编译出的 Yak 代码所需的基础运行时能力，以及部分 yak 标准库函数绑定（例如 `poc.*`）。

构建完成后可在这里看到产物：

- `common/yak/ssa2llvm/runtime/libyak.a`

兼容入口保留在：

- `runtime/build_runtime_go.sh`

## 将 runtime 嵌入到 CLI（可选）

如果你希望发布的 `ssa2llvm` CLI 不依赖外置的 `libyak.a`，可以把运行时归档打包进二进制，并在编译时自动释放到临时构建目录供 clang 链接。

生成嵌入资源（会在 `common/yak/ssa2llvm/runtime/embed/` 下生成 `ssa2llvm-runtime.tar.gz` 和 `ssa2llvm-runtime-src.tar.gz`）：

```bash
./scripts/build_runtime_embed.sh
```

嵌入资源生成后，构建 CLI（示例）：

```bash
go build -tags ssa2llvm_gzip_embed ./common/yak/ssa2llvm/cmd
```

## 现场编译 stdlib/runtime（`--stdlib-compile`）

当你使用发布版 CLI（没有 yaklang 项目目录）时，如果希望在编译 Yak 代码时现场编译 `libyak.a`，可以使用：

```bash
ssa2llvm compile demo.yak --stdlib-compile
```

该模式会把内置的 runtime 源码（`ssa2llvm-runtime-src.tar.gz`）释放到临时目录，并执行 `go build -buildmode=c-archive` 生成 `libyak.a`，随后再进入 clang 链接阶段。

## 快速验证

```bash
go test ./common/yak/ssa2llvm/...
```

如果出现 `runtime library not found`，通常是还没先执行 runtime 构建脚本。

## 文档

- GC 机制说明：`docs/gc-mechanism.md`
