# ssa2llvm Status

## 当前基线

`ssa2llvm` 当前已经从“基础 SSA → LLVM 实验”演进为一条完整的 AOT 路径：

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
- `main` wrapper 自动注入

### 调用 ABI

- 单参数 `InvokeContext` 调用协议
- 编译后的 Yak 函数调用
- extern hook 调用
- stdlib dispatch 调用
- `go` 异步调用与等待

### 语法与控制流

- 常量、算术、比较
- `if` / `for` / `return`
- 普通函数调用、递归调用
- `ParameterMember` / side-effect 相关 lowering 基础路径
- `defer`
- `panic`
- `recover`
- `try-catch-finally`

### 运行时

- stdlib dispatcher
- Go 对象 + shadow object + Boehm GC 混合模型
- `waitAllAsyncCallFinish()`
- runtime 嵌入与 `--stdlib-compile` 模式

### 测试

已有测试覆盖：

- 基础算术与控制流
- 函数调用与递归
- 多语言前端输入
- print / stdlib / extern hook
- `go` 语句
- 复杂语法
- struct / interop / obfuscation
- 编译缓存与 runtime 嵌入

## 当前限制

### 值表示仍然偏 `i64`

- LLVM 侧目前依然以 `i64` 作为主值表示；
- 字符串、对象、复杂值依赖指针编码、shadow object、tag/root 协议；
- 这使 ABI 简洁，但类型语义仍需要继续细化。

### 错误处理是 CFG lowering，不是原生异常展开

- 当前 `panic`/`recover` 建立在 `InvokeContext.Panic` 上；
- `defer` 通过函数级 `DeferBlock` 收口；
- 不是 LLVM `landingpad` / 栈展开模型；
- 多 catch 目前按单 catch 入口处理。

### 异步错误传播仍然较保守

- goroutine 内部的 Go runtime panic 会被 runtime 捕获并打印；
- 不会自动回流成上层 Yak `catch` 值；
- 普通调用边界上的跨函数 panic 传播协议还可以继续加强。

## 推荐验证方式

```bash
mkdir -p .db
export YAKIT_HOME="$PWD/.db"
./common/yak/ssa2llvm/scripts/build_runtime_go.sh
go test ./common/yak/ssa2llvm/... -count=1
```

如果要验证最终产物链路，建议再额外执行一次：

```bash
go build -o ./ssa2llvm ./common/yak/ssa2llvm/cmd
./ssa2llvm run demo.yak
```

## 后续建议

- 继续围绕 `InvokeContext` 收口调用与错误处理 metadata；
- 补强复杂对象、blueprint、member、side-effect 的覆盖与测试；
- 如果未来要做更强的异常语义，再单独设计跨函数 panic 传播协议；
- 维持“最终二进制可运行且输出正确”的测试标准，不只验证 IR。
