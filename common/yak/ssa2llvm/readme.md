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

## 快速验证

```bash
go test ./common/yak/ssa2llvm/...
```

如果出现 `runtime library not found`，通常是还没先执行 runtime 构建脚本。

## 文档

- GC 机制说明：`docs/gc-mechanism.md`
