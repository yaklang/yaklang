# Yaklang 解析性能：SLL vs LL 与两阶段解析优化

本文记录 Yaklang 编译前端（ANTLR4）在大脚本下的性能瓶颈定位、根因分析、
两阶段解析（SLL 优先 / LL 回退）的实现方式，以及优化前后的基准对比。

相关代码：

- 语法：`common/yak/antlr4yak/YaklangParser.g4`
- VM 编译器解析入口：`common/yak/antlr4yak/yakast/visitor.go` 中的 `Compiler` / `parseProgramTwoStage`
- SSA 前端解析入口：`common/yak/yak2ssa/builder.go` 中的 `FrontEnd`（复用 `antlr4util.ParseASTWithSLLFirst`）
- 两阶段解析公共实现：`common/yak/antlr4util/sll_first_parse.go`
- 基准与正确性测试：`common/yak/antlr4yak/perf_investigation_test.go`

---

## 1. 问题现象

实际使用中，编译几百 KB 的脚本（尤其是 `common/coreplugin/` 下的大插件）明显偏慢，
单个脚本编译达到数百毫秒。需要定位：慢在词法分析（Lexer）还是语法分析（Parser）。

## 2. 瓶颈定位：不是 Lexer，是 Parser（LL 全上下文预测）

分别对最大的几个 coreplugin 脚本单独计时 词法分析 / 语法分析(LL) / 语法分析(SLL) / 完整编译，
测量结果（`TestPerf_LexerVsParser`，DFA 预热后稳定态）：

```
script                          bytes   tokens |      lex |   parse(LL)   parse(SLL) | fullCompile
--------------------------------------------------------------------------------------------------
启发式SQL注入检测.yak            96542   14253 |  19.6ms |   683.9ms       9.6ms  |    376.7ms
Shiro 自定义检测.yak             27224    3661 |   3.2ms |    75.3ms       2.5ms  |     69.6ms
SSA 项目探测.yak                24898    6860 |   4.9ms |   277.4ms       2.9ms  |    131.4ms
Fastjson 综合检测.yak           24834    3813 |   3.4ms |   383.9ms       4.7ms  |    262.5ms
基础 XSS 检测.yak               21503    3164 |   3.8ms |   148.7ms       2.0ms  |    107.9ms
```

结论：

- **Lexer 很快且随规模线性增长**（96KB 文件仅 ~20ms），不是瓶颈。
- **Parser 在默认 LL（全上下文）预测模式下极慢**：96KB 文件耗时 683ms，
  是同一份输入在 SLL 模式下（9.6ms）的约 **70 倍**。其他文件 SLL 相对 LL 也有 **30~80 倍** 差距。

### 合成规模测试：LL 随规模急剧放大

用重复的“重表达式”行（混合二元运算、成员/下标调用、三元表达式）构造不同规模的脚本
（`TestPerf_ScalingSynthetic`）：

```
lines     tokens |      lex |   parse(LL)   parse(SLL) | fullCompile
--------------------------------------------------------------------
 200        8006 |   2.7ms |   166.9ms       5.3ms  |    12.5ms
 400       16006 |   3.3ms |   321.6ms       9.9ms  |    22.5ms
 800       32006 |   6.3ms |   661.8ms      19.6ms  |    48.1ms
1600       64006 |  12.8ms |  1285.1ms      39.7ms  |    97.8ms
3200      128006 |  23.6ms |  2604.4ms      77.9ms  |   261.2ms
```

LL 与 SLL 都随规模增长，但 LL 的**每 token 常数成本极高**（约为 SLL 的 30 倍以上），
在几百 KB 级别就会累积到秒级。

### 根因

`YaklangParser.g4` 中的 `expression` 规则是深度左递归、且备选分支非常多的规则
（一元运算、二元运算多档优先级、三元、管道、成员/下标/函数调用等）。
默认 LL 模式在遇到这类规则时会频繁触发昂贵的 **全上下文自适应预测（full-context AdaptivePredict）**，
对每个表达式都要做大量前瞻与合并计算，导致大脚本解析时间被放大数十倍。

## 3. 修复：两阶段解析（SLL 优先，LL 回退）

采用 ANTLR 官方推荐的 **two-stage parsing** 策略：

1. **阶段一**：`PredictionMode = SLL` + `BailErrorStrategy`。SLL 用单一（合并）上下文预测，
   速度快数十倍；一旦遇到 SLL 无法判定的歧义或语法错误，`BailErrorStrategy` 立即抛出
   `ParseCancellationException` 中止，不做任何错误恢复。
2. **阶段二（仅在阶段一失败时）**：回退到 `PredictionMode = LL` + 正常错误恢复策略与错误监听器，
   重新解析。既保证性能，又保证对真正语法错误的精确报告，以及对极少数 SLL 无法判定构造的正确处理。

关键实现细节：

- 生成的 parser 复用**全局静态 DFA**（`decisionToDFA`），阶段二新建 parser 不产生冷启动开销。
- 复用同一个已缓冲的 `CommonTokenStream`（`Seek(0)` 回绕），避免重复词法分析与重复上报词法错误。
- 通过环境变量 `YAK_ANTLR_SLL_FIRST=0` 可关闭 SLL 快路径，退回纯 LL（与历史行为一致），便于排查问题。
  该开关与 SSA 前端共用（`antlr4util.SLLFirstEnabled`）。

VM 编译器落地在 `yakast/visitor.go`：

- `Compiler` 调用新增的 `parseProgramTwoStage`，SLL 成功则直接返回，失败则回退 LL。
- SSA 前端（`yak2ssa`）此前已通过 `antlr4util.ParseASTWithSLLFirst` 采用同样策略；
  本次改动使 VM 编译器与 SSA 前端在解析策略上保持一致。

## 4. 正确性保证

两阶段解析的正确性前提是：**当 SLL 未报错地成功解析时，其解析树必须与 LL 完全一致**；
只有当 SLL 报错/中止（bail）时才回退 LL。测试 `TestPerf_SLLBailDiagnostic`
对全部 52 个 coreplugin 脚本逐一验证了这一不变式（未 bail 的脚本 SLL 树与 LL 树逐字符相等），
若出现“SLL 无错却与 LL 结果不同”会直接判定失败。

此外，`common/yak/antlr4yak` 的完整测试套件（含 `TestNewExecutor_*` 等运行时用例）在开启
两阶段解析后全部通过，行为无回归。

## 5. 优化前后对比（端到端编译）

对全部 52 个 coreplugin 脚本做端到端完整编译（`TestPerf_EndToEndAggregate`，DFA 预热后计时）：

| 模式 | 脚本数 | 总字节 | 总编译耗时 |
| --- | --- | --- | --- |
| `YAK_ANTLR_SLL_FIRST=0`（纯 LL，旧行为） | 52 | 408926 | **2.641s** |
| `YAK_ANTLR_SLL_FIRST=1`（两阶段，新行为） | 52 | 408926 | **1.636s** |

整体约 **38% 提速**。对能走 SLL 快路径的脚本（本集合中 32/52），单文件语法分析可获得
**30~80 倍** 的加速（见第 2 节 `parse(LL)` vs `parse(SLL)`）。

## 6. 语法消歧：让全部 coreplugin 都命中 SLL

早期版本中 `TestPerf_SLLBailDiagnostic` 显示 52 个 coreplugin 里有 **20 个会 bail 回退到 LL**，
其中包含体积最大的 `启发式SQL注入检测.yak`。经定位，全部 bail 都来自两处**共享前缀 + 后缀
自我重叠**的语法歧义（同一类问题），现已通过语法消歧解决，**52 / 52 全部命中 SLL，bail=0**。

### 6.1 赋值左值（下标/切片/映射）

历史语法：

```
leftExpression : expression (leftMemberCall | leftSliceCall) | Identifier ;
leftSliceCall  : '[' expression ']' ;   // 与 expression 的 sliceCall 单下标形式完全重叠
```

对 `a[0] = 1`，`a[0]` 既能作为独立表达式语句（`expression sliceCall`），又能作为赋值左值
（`expression` + `leftSliceCall`），两者共享前缀，真正的判别符是其后的 `=`。SLL 的单一（合并）
上下文近似无法穿过左递归 `expression` 的优先级循环看到 `=`，于是先选表达式语句分支、消费完
`a[0]` 后在 `=` 处报 no viable alternative，bail 回退 LL。成员赋值 `a.b = 1` 不受影响，是因为
`memberCall` 只有单一形式，而 `sliceCall` 的多个 `:` 备选把 SLL 的嵌套判定进一步拖垮。

消歧：让左值直接复用通用 `expression`，与右值走完全相同的 ATN 路径，判别点后移到
`expression` 结束后的单个 token：

```
leftExpression : Identifier | expression ;
```

`Identifier` 作为首选备选，保证裸标识符（如 `for i in x`）不会被 `expression` 贪婪吞掉后缀，
从而与 for-range 等规则保持既有行为。左值是否可赋值改由 visitor 依据 expression 子树判定
（成员 / 单下标 / 标识符），非法左值（`1 = 2`、`a[1:2] = x`）在语义阶段报错。

### 6.2 `go` / `defer` 的函数调用

历史语法 `functionCallExpr : expression functionCall ;` 与 expression 自身的 functionCall 后缀
重叠：对 `go f()` / `defer f()`，`f()` 既能被 `expression` 整体吞掉，又能拆成 `expression=f` +
外层 `functionCall=()`，同样是共享前缀 + 后缀自我重叠，SLL bail。消歧同样复用通用 `expression`：

```
functionCallExpr : expression ;
```

顶层是否为合法调用由 visitor 判定。注意 SSA 前端刻意“只构造不 emit”，由 `go`/`defer` 决定
emit 时机，因此从 expression 子树取出顶层 `functionCall` 再走 `buildFunctionCall`，而非直接
`buildExpression`（后者会 `EmitCall`，会导致重复发射）。

### 6.3 影响面

VM 编译器（`yakast`）与 SSA 前端（`yak2ssa`）的 visitor 已同步适配新的 `leftExpression` /
`functionCallExpr` 结构；“SLL 未 bail 时解析树必须与 LL 逐字符一致”的不变式仍然成立；
`common/yak/antlr4yak` 全量测试与 `yak2ssa` 测试通过，运行时行为（下标/映射/成员赋值、`++`、
`go`、`defer`、for-range）经源码引擎冒烟验证无回归。

> 历史备注：下述最小复现在旧语法下会 bail，现已全部 OK：
>
> ```
> a[0] = 1 / m[k] = v / a[0] := 1 / a[0], b = 1, 2 / a[i] = a[i] + 1
> go f() / defer f()
> ```

## 7. 如何复现基准

基准类测试默认跳过，避免拖慢常规 `go test`。开启方式：

```bash
# 定位瓶颈：Lexer vs Parser(LL) vs Parser(SLL) vs 完整编译
YAK_PARSER_BENCH=1 go test ./common/yak/antlr4yak/ -run TestPerf_LexerVsParser -v -count=1

# 合成规模测试，观察 LL 随规模放大
YAK_PARSER_BENCH=1 go test ./common/yak/antlr4yak/ -run TestPerf_ScalingSynthetic -v -count=1

# 端到端：两阶段(SLL 优先) vs 纯 LL 的总编译耗时对比
YAK_PARSER_BENCH=1 YAK_ANTLR_SLL_FIRST=1 go test ./common/yak/antlr4yak/ -run TestPerf_EndToEndAggregate -v -count=1
YAK_PARSER_BENCH=1 YAK_ANTLR_SLL_FIRST=0 go test ./common/yak/antlr4yak/ -run TestPerf_EndToEndAggregate -v -count=1

# 正确性守卫（始终运行）：SLL 未 bail 时与 LL 结果必须逐字符一致
go test ./common/yak/antlr4yak/ -run TestPerf_SLLBailDiagnostic -v -count=1
```

> 注：以上耗时为特定机器上的相对参考值，绝对数字会因硬件而异，重点看 LL 与 SLL 的**倍数关系**。
