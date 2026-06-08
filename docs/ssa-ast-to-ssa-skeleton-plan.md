# SSA 编译：ANTLR 原地瘦身 + lazy 子树闭包

> 目标分支：`enhance/ssa/use_lazybuild_replace_second_ast`
> 文档定位：当前落地方案与后续计划。

## 0. 2026-06-05 落地结论

本轮实现方向已经调整为：

- **不重新解析 AST**：offset/span 重解析会增加 CPU，并且 lazy 触发时仍会产生额外 AST，当前不采用。
- **不做每语言小 AST / 统一 AST**：每语言都维护一套紧凑 AST 成本过高，统一 AST 也会引入新的前端模型风险。
- **保留 ANTLR context 类型和 children**：lazy builder 继续使用原 visitor/build 逻辑。
- **在 range 已经计算后原地瘦身 ANTLR 节点**：清掉 parent、ctx.parser、parser ATN cache、exception、token source/input stream；保留 token text/offset/line/column。
- **缩短 lazy/deferred 调度引用生命周期**：deferred build task 执行后释放自身载荷，`StoreFunctionBuilder` 从小对象树改为字段快照。
- **文件级收尾只做轻量切根**：真正被 lazy 闭包捕获的子树才递归 `DetachAST`；整文件 root children 只用 `DetachASTRootChildren` 切 parent/parser，不再递归复制整文件 token text。
- **降低瘦身自身开销**：ANTLR unexported 字段访问用类型字段索引缓存，避免每个 AST 节点反复 `FieldByName`。

这条线的目标不是完全消灭 lazy 子树本体，而是在不牺牲解析 CPU 的前提下，把 lazy 子树从整文件 parent/parser/input-stream 图里剥离出来。

## 1. 背景

当前编译管线（`common/yak/ssaapi/ssa_compile_fs.go`）分阶段执行：

- `f1_pre_handler`：逐文件 `ParseAST` 得到 ANTLR AST，调用 `PreHandlerProject` 构建“粗定义/骨架”；对仍需要文件级收尾的语言，只注册 `RegisterFileBuild` / `RegisterDeferredBuild` 这类 SSA 自有任务，不再捕获整份文件 AST。
- `f3_main_build`：`RunDeferredBuilds()` 顺序执行剩余的 SSA 自有任务，再由 `Finish()` 展开函数 / blueprint lazy builder。
- 函数体/类方法体则通过 `Function.AddLazyBuilder` / `Blueprint.AddLazyBuilder` 注册延迟构建闭包，闭包内捕获对应的 AST 子树（如 Go 的 `fun.Block()`、Java 的 `i.MethodBody()`）。

历史动机：先看到“系统全貌”（pass1 收集跨文件定义），再做真正的 build（pass2 按定义解析），从而解决跨文件依赖。`LazyBuilder` 就是为这套两遍机制服务的。

## 2. 问题：内存没有真正下降

用户的判断是对的。旧实现相对 `main` 的改动，**只是把整份文件 AST 从 `fileContents []*FileContent` 切片，搬到了文件级闭包里**。AST 的生命周期没有缩短：

- `f1` 结束时所有文件都已 `ParseAST`，每个文件的整树 AST 都被某个文件级闭包（或函数/方法的 lazy 闭包）强引用着。
- `f3` 才开始逐个执行并释放。
- 因此在 `f1` 结束、`f3` 进行到一半之前，**全工程所有文件的 AST 同时存活**，峰值内存 = 所有文件 AST 之和。大工程下这就是内存爆点。

### 2.1 真正的根因（关键）

除了“整树闭包”，还有一个被忽略的放大器：**ANTLR Go runtime 的父指针**。

- `BaseRuleContext.parentCtx`（`rule_context.go:44`）让每个子节点持有指向父节点的引用，`SetParent/GetParent` 即操作它。
- 这意味着：**只要 lazy 闭包捕获了任意一个子树（哪怕只是一个方法体 `MethodBody`），通过 `parentCtx` 链就把整份文件 AST 全部钉住，无法回收。**

所以即便我们“释放”了 `fileContent.AST` 这个根引用，只要还有函数/方法的 lazy 闭包存在，整树依然不会被 GC。这就是“内存没下降”的物理原因。

### 2.2 附带问题：部分语言重复 build

`go2ssa` 的 `PreHandlerProject` 在 pass1 已经调用了 `prog.Build(ast,...) → BuildFromAST`，pass2 的 top-level 闭包又调用一次 `BuildFromAST`；C 语言已在文档里记录了类似“重复 lazy builder / 重复 SSA 输出”的规避。两遍都做整树遍历，CPU 也翻倍，正是用户说的“AST 解析太久拖慢编译”。

## 3. 目标架构

把管线从「parse → 整树存活 → 第二遍整树 build」改为「**一次整树遍历产出 SSA 骨架 + 每个待延迟单元的小钩子，文件级整树随即释放**」：

```
pass1（逐文件，单次遍历后立即释放该文件整树 AST）
  ├─ 直接 emit 跨文件可见的 SSA「骨架/定义」：
  │     package/namespace、类型/Blueprint 壳、函数/方法签名、全局变量名、import 关系
  └─ 对每个“可执行体”（函数体、方法体、init、文件级 top-level 语句块）
        注册一个 lazy 钩子，钩子内 **只持有该体所需的、原地瘦身后的 ANTLR 子树**

——此处该文件的整树 AST 不再被任何对象引用，可被 GC——

pass2（由 SSA 骨架/定义驱动的 lazy 展开）
  └─ RunDeferredBuilds + Finish：按需执行各 lazy 钩子，从「骨架 + 瘦身子树」构建函数体 SSA，
        不再有任何“整份文件 AST”级别的二次遍历。
```

对应用户的话：「第一次解析将 AST 变成 SSA 表达的结构；第二次直接从 SSA 解析；中间 AST 只剩 lazybuild 里单独的小闭包钩子」。

两个硬约束：

1. **文件级整树 AST 不得被 pass2 持有**（top-level 闭包不能再捕获整树）。
2. **lazy 钩子持有的 AST 片段必须与父树断开**（否则父指针把整树钉住，§2.1）。

## 4. 落地机制（钩子里到底放什么）

当前只采用一种机制：**保留 ANTLR 子树，但在捕获边界和 range 热路径做原地瘦身**。

### 4.1 ANTLR 子树原地瘦身

注册 lazy 闭包前，对闭包要捕获的子树调用 `ssa.DetachAST`，底层进入 `antlr4util.SlimParserTree`：

- 保留 generated context 类型和 downward children，原 visitor/build 逻辑继续可用；
- 保留 start/stop token 的 text、offset、line、column；
- 清理 rule/terminal parent link，lazy builder 不能再依赖 `GetParent()` 回溯；
- 清理 generated context 的 `parser` 字段、parser ATN simulator cache、context exception；
- 对可达 token 先固化 `GetText()`，再替换空 source pair，断开 lexer/input stream；
- range 计算完成后，通过 `SlimRangeToken` / `SlimParserNode` 在 `SetRange` 热路径清当前节点。

这种方式不移除父节点 children，也不重建 AST。释放成立的关键是：文件根 AST 不再被 pipeline/file closure 持有；lazy 捕获到的子树没有 parent/parser/input-stream 反向链，不能把整文件图继续钉住。

文件级收尾使用更轻的 `DetachASTRootChildren`：只切 root direct children 的 parent/parser/exception，不递归固化 token text。它只用于“所有 lazy 保留子树已经显式 `DetachAST` 捕获之后”的收尾，避免把即将不可达的整文件 AST 再加工一遍。Go/C 仍保留原顶层递归兜底，直到各自捕获点全部显式瘦身后再切换。

### 4.2 不采用的方向

- **不做源码 span 重解析**：会让 lazy build 触发额外 parser CPU，并且会短时产生新 AST；当前目标是同时减少 CPU 和内存，不接受这条线。
- **不做每语言小 AST**：每种语言维护一套小 AST 工作量过大，且和现有 visitor/build 体系割裂。
- **不做统一 AST**：统一模型难以覆盖各语言语义和现有 parser context API，风险超过本轮收益。

### 4.3 辅助内存优化

- deferred build task 执行后释放自身 `LazyBuilder` 引用；
- `StoreFunctionBuilder` 由“小 FunctionBuilder 对象树”改为字段快照，减少每个 lazy 闭包保存 builder 状态时的小对象滞留；
- `DeferredBuildCount()` 保留注册总数，用于测量统计，执行成功后 deferred build 调度表可以清空。
- `antlr4util` 缓存 unexported 字段索引，light-detach 复测中 `reflect.(*structType).FieldByNameFunc` alloc 下降约 466 MB、`reflect.(*structType).Field` 下降约 262 MB。

## 5. 实施路线（分阶段、可独立验证）

### 阶段 0：基线与度量（先做，避免“盲改”）

1. 选 1~2 个大工程样本（如大型 Java/TS 仓库）。
2. 在 `parseProjectWithFS` 内或通过 `pprof` 采集：`f1` 结束时、`f3` 中段、`Finish` 后的 `HeapInuse`；以及 `f1/f3` 墙钟时间。
3. 记录基线，作为后续每阶段的对比口径。

### 阶段 1：消灭“整树 top-level 闭包”，top-level 与骨架在 pass1 内联完成

目标：文件级 deferred build 不再捕获整份文件 AST。

1. 把“文件级 top-level”拆成两类：
   - **定义/声明**（包名、类型壳、函数/方法签名、全局变量名、import）：pass1 单次遍历内**立即 emit 成 SSA 骨架**，跨文件可见。
   - **可执行 top-level 语句块**（init、文件级语句、含跨文件引用的全局变量初始化）：当作“伪函数体”，注册成 lazy 钩子（与函数体同等待遇），**只捕获该语句块子树**，不捕获整树。
2. 删除/退役 pass2 中“整树 `BuildFromAST`”的 top-level 路径；deferred build 阶段只跑“函数 / blueprint / 文件级语句块 / helper”这些 SSA 自有节点。
3. 修复 §2.2 的重复 build：明确每个体的构建只发生在唯一一处（pass1 emit 骨架，pass2 emit 体），删除 `PreHandlerProject` 里多余的 `prog.Build` 整树调用。

涉及文件（主）：
- `common/yak/ssaapi/ssa_compile_fs.go`（`f1` 注册逻辑、移除整树闭包）
- `common/yak/ssa/deferred_build.go`、`common/yak/ssa/program.go`（deferred build 节点种类与注册）
- 各语言 `builder.go` / `builder_ast.go` / `builder_prehandler.go` 的 `PreHandlerProject` 与 `BuildFromAST` 边界划分

### 阶段 2：lazy 钩子子树瘦身，释放文件级整树

目标：pass1 走完一个文件后，该文件整树可被 GC。

1. 在 `antlr4util` / `ssa` 层提供语言无关的瘦身工具：

```go
// common/yak/antlr4util/detach.go
SlimParserTree(node) // recursive: parent/parser/cache/token source/input stream
SlimParserNode(node) // non-recursive: SetRange hot path
SlimToken(token)     // keep text/range, drop source/input
```

2. 各语言在 `AddLazyBuilder` 前，把要捕获的体（`fun.Block()` / `i.MethodBody()` / top-level 语句块）先 `DetachAST` 再捕获：

```go
body := ssa.DetachAST(fun.Block())
newFunc.AddLazyBuilder(func() {
    ... // 只用 body，不再依赖向上 GetParent()
    b.buildBlock(body, true)
})
```

3. 在 `SetRange` / 语言自定义 range 入口中，先计算 range，再调用 `SlimRangeToken`，避免清理 token source/input stream 后影响定位。
4. 审查并消除 lazy 闭包内对“父级上下文”的依赖：凡 build 体时需要的父级信息（外层类型参数、包名、import、外层作用域），必须在 pass1 固化进 `store`/骨架，不能在 pass2 通过 `GetParent()` 回溯。这是阶段 2 的主要工程量与风险点。

涉及文件（每语言各一处或数处 `AddLazyBuilder`）：
- `common/yak/go2ssa/builder_ast.go:171,752,912`
- `common/yak/java/java2ssa/visit_package_n_class.go:127,229,543,620,889`、`visit_interface.go:34`
- `common/yak/typescript/ts2ssa/build_from_ast.go:1030,2794,2895,2971,3045,4251,4267`、`auxiliary.go:411`
- `common/yak/php/php2ssa/builder.go:157`、`visit_cls.go:253,293,320`、`visit_func.go:33`
- `common/yak/python/python2ssa/visit_class.go:279`
- `common/yak/ssa/ssa_globalBulePrint.go:28`

### 阶段 3：缩调度与 builder 状态

1. deferred build task 执行后释放自身载荷，避免已完成任务继续保留 `LazyBuilder`。
2. `StoreFunctionBuilder` 保存字段快照，不再为每个 lazy 闭包构造一棵小 `FunctionBuilder/Function/anValue/anInstruction` 对象树。
3. 文件级收尾改成轻量切根，递归 token text 固化只发生在真实 lazy 捕获子树上。
4. 重测 pprof，确认 `StoreFunctionBuilder`、`NewLazyBuilder`、deferred task、反射字段访问相关对象是否下降。

### 阶段 4：清理与文档

1. 删除已死代码：旧整树 top-level pass2 路径、仅用于兼容的空函数等。
2. 更新 `docs/ssa-deferred-build.md`，与本计划合并/对齐。
3. 移除调试日志。

## 6. 各语言适配要点

| 语言 | pass1 骨架应 emit | pass2 lazy 体 | 主要风险 |
|------|------------------|---------------|---------|
| Go | package、类型、函数签名、全局名、import | 函数体 `Block`、init | `PreHandlerProject` 当前直接整树 build，需先拆分；泛型 `tpHandler` 依赖父级 |
| Java | 包/类壳、方法签名、字段、annotation 元数据 | 方法体、构造/析构、static/normal method | 方法体大量依赖外层 `class`/`store`，需固化 |
| TypeScript | namespace/class 壳、函数签名 | 函数体、类成员、默认构造 | 闭包数量多（8+ 处），命名空间嵌套父级依赖 |
| PHP | class、函数签名 | 函数/方法体 | class 与 method 双层 lazy 嵌套 |
| Python | class 壳、方法签名 | 方法体 | 同作用域成员引用，方法体 lazy |
| C | 函数/类型前置信息 | 非函数 top-level | 已有“pre-handler 只注册、runtime 跳过重复”的特例，需与新模型对齐 |

通用前置：在 `PreHandlerBase`/`Builder` 接口层约定“pass1 只产骨架、pass2 只产体”的契约，避免各语言再各自直接 `prog.Build` 整树。

## 7. 风险与回滚

- **父级依赖泄漏（最大风险）**：瘦身后 pass2 若仍依赖 `GetParent()`，将拿不到父上下文或定位错乱。缓解：用 grep 审查各 `AddLazyBuilder` 闭包体内是否调用 `GetParent`/外层 `ctx`；逐语言、逐文件灰度。
- **跨文件依赖回归**：把“可执行 top-level”降级为 lazy 后，需确保其在所有定义注册完成后才执行（保持在 `Finish`/deferred build 阶段，而非 pass1）。
- **Range/定位回归**：必须先拿 range 再瘦 token；用专门单测覆盖清理后 range 仍可用。
- **IR 计数差异/非确定性**：spring-boot 测量中 instructions 有小幅变化；同一 light-detach 实现两次测量为 868159 / 868206，需要用小样例 IR diff 和规则回归确认不是漏建。
- **峰值 RSS 短峰**：light-detach retained heap 下降，但 `/usr/bin/time` 峰值 RSS 未稳定低于 release-store。`FilesHandler` 的并发 read/ParseAST pipeline 仍可能让多份 `FileContent(Content+AST)` 同时在途，下一步应单独评估 backpressure、buffer 上限和大项目并发上限。
- **回滚策略**：每阶段独立成 commit；ANTLR 瘦身、deferred task release、builder snapshot 可以分别回退。

## 8. 验证

1. **正确性**：全量跑各语言 `*_test.go` 与 SyntaxFlow 规则验证；重点回归 `common/yak/ssa/deferred_build_test.go`、各语言 `test/` 用例。
2. **内存**：用阶段 0 的样本与口径对比 `f1`/`f3`/`f4` `HeapInuse`、结束 RSS、`/usr/bin/time` 峰值 RSS。
3. **速度**：对比编译墙钟，确认不重新解析、不整树二次 build 后 `parseTime` 没有明显回退。
4. **IR diff**：固定小样例做同实现多次编译的 IR 稳定性检查，再比较 skeleton/release-store/light-detach 的差异。
5. **基准**：补充一个可复现的 bench（大工程 fixture），纳入回归。

## 9. 落地顺序小结

1. 阶段 0 度量基线。
2. 阶段 1：top-level 内联 + 去整树闭包 + 去重复 build（先拿到“结构正确、无整树二次遍历”的版本）。
3. 阶段 2：ANTLR 子树原地瘦身（拿到 token/input/parser 保留链收益）。
4. 阶段 3：deferred task release + builder 状态字段快照 + 文件级轻量切根 + 反射字段缓存（减少调度、lazy 状态和瘦身实现开销）。
5. 阶段 4：清理与文档。
6. 下一阶段：f1 read/ParseAST pipeline 的在途对象和 buffer/backpressure，专门解决 retained heap 之外的峰值 RSS 短峰。
