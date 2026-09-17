# YakVM core extraction from PR #5013

本 PR 把协议性能分支中的 YakVM 核心改动独立到 main：缓存 Go 导出方法的不可变索引（256 个类型上限，永不缓存绑定 receiver）；为完全同步且私有的 VM 提供 frame 快路径；允许私有 VM 关闭 panic 的 Go 调试栈输出；修复共享 undefined 值的赋值元数据污染。普通 VM 的 goroutine frame 隔离、默认 panic 行为、Yak 错误信息及恢复语义保持原行为。

源版本：PR #5013 `43d9869da98267f9db0839cc4f431c9518c9c296`；12 个摘出文件与源版本逐字节一致。新增一个同步 frame 基准。核心提交 `dd3b16a7f1`，基线 main `a9f1feb2732812cf49e57859574d0e9fd43986f7`。没有 bin-parser/pcapx 代码、规则、语料或消费端启用逻辑；这些保留在 #5013。

## 可重复的独立性能结果

Apple M1 Max / macOS 14.1.2 / Go 1.22.12 darwin/arm64，GOMAXPROCS=4。前后独立 test binary；各项 500 ms，五轮交替 A/B、B/A；编译、测试不与计时同时运行。下表为中位数，正数表示变慢。共享开发机上的小变化不作为显著性能结论，超过 5% 的普通模式退化才进入复查。本轮普通模式控制项均未达到此阈值。

VMMemberLoop 一 op 是包含 1,000 次真实 Go 方法调用、断言和 VM 循环的已编译脚本；其他项为一次对应操作。编译在计时外，使用普通 goroutine-aware VM。**普通脚本循环耗时下降约 5.4%，不是协议解析整体加速比。**

| 普通模式 A/B | main ns/op | extracted ns/op | 耗时变化 | allocs/op |
|---|---:|---:|---:|---:|
| CallYakFunctionBackgroundContext | 27574.00 | 27546.00 | -0.10% | 441 → 441 |
| CallYakFunctionCancelableContext | 27759.00 | 27832.00 | +0.26% | 441 → 441 |
| AnonymousFunctionDirectAssignment | 8321.00 | 8371.00 | +0.60% | 34 → 34 |
| VMMemberLoop | 3797284.00 | 3590699.00 | -5.44% | 80027 → 76027 |
| ExecFrameLifecycle | 4929.00 | 4991.00 | +1.26% | 14 → 14 |
| NestedFrameLifecycle | 5183.00 | 5233.00 | +0.96% | 19 → 19 |
| IdleGetVar | 32.77 | 32.84 | +0.21% | 0 → 0 |

以下控制项在同一个新二进制内比较反射/缓存、普通/私有同步配置，衡量局部路径。同步 benchmark 一 op 是嵌套父子 frame、当前 frame 查找与同步 native callback 查找，**不是一般 Yak 脚本的整体加速比**。私有同步配置要求调用者保证所有执行和回调都在同一 goroutine，默认关闭，不能用于共享或异步 VM。

| 局部控制项 | 普通 ns/op | 优化 ns/op | 耗时变化 | B/op | allocs/op |
|---|---:|---:|---:|---:|---:|
| NativeMethodLookup | 362.60 | 136.60 | -62.33% | 192 → 72 | 6 → 2 |
| SynchronousFrameLifecycle | 43695.00 | 532.50 | -98.78% | 1496 → 984 | 25 → 15 |

完整五轮原始结果及验证摘要见 [performance-extraction-20260917.txt](testdata/performance-extraction-20260917.txt)。基线只复制 `member_method_benchmark_test.go` 测试文件，其余主线生产代码保持不变。

```sh
go test -c -o /tmp/engine.test ./common/yak/antlr4yak
go test -c -o /tmp/vm.test ./common/yak/antlr4yak/yakvm
# 在对应包目录运行，前后版本交替执行以下命令，每项五轮。
/tmp/engine.test -test.run '^$' -test.bench '^(BenchmarkVMMemberLoop|BenchmarkAnonymousFunctionDirectAssignment|BenchmarkCallYakFunctionBackgroundContext|BenchmarkCallYakFunctionCancelableContext)$' -test.benchtime 500ms -test.benchmem -test.cpu 4
/tmp/vm.test -test.run '^$' -test.bench '^(BenchmarkExecFrameLifecycle|BenchmarkNestedFrameLifecycle|BenchmarkIdleGetVar)$' -test.benchtime 500ms -test.benchmem -test.cpu 4
# 新二进制的局部控制项：
/tmp/vm.test -test.run '^$' -test.bench '^(BenchmarkNativeMethodLookup|BenchmarkSynchronousFrameLifecycle)$' -test.benchtime 500ms -test.benchmem -test.cpu 4
```

## 正确性与 race 边界

- `go test ./common/yak/antlr4yak/... -count=1 -timeout=10m`：全部通过，含 engine、VM、AST、DAP/LSP。
- YakVM 包完整 `-race`：通过。
- engine/VM 定向 race：方法绑定、并发 receiver 隔离、frame 生命周期、回调、closure、undefined 本地/全局赋值、panic 恢复及匿名函数绑定通过。
- 全 engine race 发现 4 个已有脚本 fixture 竞争：`TestNewExecutor4`、`TestNewExecutor4_Scope_Go`、`TestForLoopVar_Go122Semantics`、`TestNewExecutor4_OpGo_N`。在未修改 main 上用同样测试名逐项复现；栈显示测试中的 len/dump/迭代与脚本 goroutine 的普通 map 写入并发。不能声称全 engine race 通过；本拆分没有重写这些独立 fixture，也没有隐藏失败。

```sh
go test -race ./common/yak/antlr4yak/yakvm ./common/yak/antlr4yak -run 'Test(NativeMethod|Synchronous|UndefinedAssignment|BoundUndefined|VMPanicDebug|MemberMethod|.*Frame|.*Callback|.*Concurrent|.*Closure|AnonymousFunctionAssignment)' -count=1 -timeout=5m
# 在未修改 main 上复现已有 fixture 竞争（预期失败）：
go test -race ./common/yak/antlr4yak -run '^(TestNewExecutor4|TestNewExecutor4_Scope_Go|TestForLoopVar_Go122Semantics|TestNewExecutor4_OpGo_N)$' -count=1 -timeout=3m
```
