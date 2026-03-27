# ssa2llvm Status

## 当前基线

`ssa2llvm` 已经具备一条完整的 AOT 路径：

- YakSSA → LLVM IR
- LLVM IR → 汇编 / 目标文件 / 原生二进制
- 链接 `common/yak/ssa2llvm/runtime/libyak.a`
- 运行最终二进制并校验输出结果

## 已完成的核心能力

### 编译与链接

- LLVM module / function / basic block 预创建
- phi 预建与收尾解析
- `llc` / `clang` 产出 `.ll` / `.s` / `.o` / 可执行文件
- `libyak.a + libgc` 链接路径
- `main` wrapper 自动注入与 async wait

### 调用 ABI

- 单参数 `InvokeContext` 调用协议
- 普通 Yak 函数调用
- builtin/stdlib dispatch 调用
- Go shadow object 反射方法调用
- sync 系列构造入口对齐 yaklib sync 导出
- `go` 异步调用与自动等待
- closure binding / freevalue 基线注入

### 语法与控制流

- 常量、算术、比较
- `if` / `for` / `return`
- 普通函数调用、递归调用
- `ParameterMember` / side-effect 基础 lowering
- `defer`
- `panic`
- `recover`
- `try-catch-finally`
- `make([]T)` 与 `append(slice, x)` 的最小 slice 路径

### 运行时

- builtin ID dispatcher
- Go object + shadow object + Boehm GC 混合模型
- runtime shadow method 反射分发
- slice shadow object 运行时表示
- runtime 嵌入与 `--stdlib-compile` 模式

### 测试

当前测试重点覆盖：

- 基础算术与控制流
- 函数调用与递归
- `go` 语句与 sync 系列标准库
- 复杂 object-factor 场景
- closure freevalue / parameter capture
- `make([]int)` / `append` / 越界 panic
- 编译缓存与 runtime 嵌入

## 当前限制

### 值表示仍以 `i64` 为主

- LLVM 侧仍然以 `i64` 作为主值表示
- 字符串、对象、slice 等复杂值依赖 tagged pointer、shadow object、roots 协议
- ABI 简洁，但类型语义仍未完全细化到 LLVM 原生类型层

### 错误处理仍是 CFG lowering

- `panic`/`recover` 建立在 `InvokeContext.Panic` 上
- `defer` 通过函数级 `DeferBlock` 收口
- 不是 LLVM `landingpad` / 栈展开模型
- 多 catch 仍按单 catch 入口处理

### closure 目前是基线路径

- 目前已支持稳定顺序的 freevalue/binding 注入
- 复杂 object-factor + freevalue + side-effect 组合还需要继续补强
- function compile metadata 已经开始收口，但 compiler 文件布局还可以继续整理

## 推荐验证方式

```bash
mkdir -p .db
export YAKIT_HOME="$PWD/.db"
./common/yak/ssa2llvm/scripts/build_runtime_go.sh
go test ./common/yak/ssa2llvm/... -count=1
```

如需验证最终产物链路，再额外执行：

```bash
go build -o ./ssa2llvm ./common/yak/ssa2llvm/cmd
./ssa2llvm run demo.yak
```

## 后续建议

- 继续收口 compile/function/runtime 三层边界
- 继续把 compiler 内的机制文件按职责拆清楚
- 补强 slice / map / blueprint / member / side-effect 的真实运行覆盖
- 维持“最终二进制可运行且输出正确”的测试标准，不只验证 IR
