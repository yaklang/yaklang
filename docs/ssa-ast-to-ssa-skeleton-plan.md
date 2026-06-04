# SSA 编译：用「AST→SSA 骨架 + 小闭包钩子」替代第二遍整树 AST 解析

> 目标分支：`enhance/ssa/use_lazybuild_replace_second_ast`
> 文档定位：方案/计划（实现前的 RFC），落地后再补充 `ssa-lazy-root-build.md`。

## 1. 背景

当前编译管线（`common/yak/ssaapi/ssa_compile_fs.go`）分阶段执行：

- `f1_pre_handler`：逐文件 `ParseAST` 得到 ANTLR AST，调用 `PreHandlerProject` 构建“粗定义/骨架”，并为每个需要编译的文件注册一个 **top-level root build 闭包**（`RegisterRootTopLevel`），闭包内捕获了 **整份文件 AST**（`ast := fileContent.AST`），随后把 `fileContent.AST = nil`。
- `f3_main_build`：`RunRootBuilds()` 顺序执行这些闭包，闭包内再调用 `BuildFromAST(ast, rootBuilder)` 完成真正的构建。
- 函数体/类方法体则通过 `Function.AddLazyBuilder` / `Blueprint.AddLazyBuilder` 注册延迟构建闭包，闭包内捕获对应的 AST 子树（如 Go 的 `fun.Block()`、Java 的 `i.MethodBody()`）。

历史动机：先看到“系统全貌”（pass1 收集跨文件定义），再做真正的 build（pass2 按定义解析），从而解决跨文件依赖。`LazyBuilder` 就是为这套两遍机制服务的。

## 2. 问题：内存没有真正下降

用户的判断是对的。本分支相对 `main` 的改动，**只是把整份文件 AST 从 `fileContents []*FileContent` 切片，搬到了 `RegisterRootTopLevel` 的闭包里**。AST 的生命周期没有缩短：

- `f1` 结束时所有文件都已 `ParseAST`，每个文件的整树 AST 都被某个 root-build 闭包（或函数/方法的 lazy 闭包）强引用着。
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
        注册一个 lazy 钩子，钩子内 **只持有该体所需的最小、且与父树断开的 AST 片段**
        （或片段的源码 span，见 §4）

——此处该文件的整树 AST 不再被任何对象引用，可被 GC——

pass2（由 SSA 骨架/定义驱动的 lazy 展开）
  └─ RunRootBuilds + Finish：按需执行各 lazy 钩子，从「骨架 + 小片段」构建函数体 SSA，
        不再有任何“整份文件 AST”级别的二次遍历。
```

对应用户的话：「第一次解析将 AST 变成 SSA 表达的结构；第二次直接从 SSA 解析；中间 AST 只剩 lazybuild 里单独的小闭包钩子」。

两个硬约束：

1. **文件级整树 AST 不得被 pass2 持有**（top-level 闭包不能再捕获整树）。
2. **lazy 钩子持有的 AST 片段必须与父树断开**（否则父指针把整树钉住，§2.1）。

## 4. 候选机制（钩子里到底放什么）

“pass1 之后整树要能释放，pass2 还能构建函数体”，三种实现各有取舍：

### 方案 A：剥离子树（detached subtree）— 推荐作为第一阶段落地

在注册每个体的 lazy 闭包前，把对应子树从父树上“剪断”：

- 子树自身 `SetParent(nil)`；
- 从其父节点的 `children` 里移除该子树（`RemoveLastChild` 或按索引删），断开反向可达；
- 闭包仅捕获这个**自包含**子树。

pass1 走完整树后，文件根节点没有任何外部引用 → 整树（除被剪下的若干小子树外）立即可回收；被剪下的小子树各自独立存活，符合“只剩小闭包钩子”。

- 优点：改动局部、风险低、不需要重新解析、不需要新 IR；直接命中用户描述。
- 代价：仍持有“函数体这部分”的 ANTLR 节点（比整树小得多）；需要确认各语言 build 函数体时不会再向上 `GetParent()` 访问父级上下文（若有，需要在 pass1 先把所需父级信息固化进骨架/`store`）。
- 关键依赖：`BaseParserRuleContext` 暴露 `SetParent`/`RemoveLastChild`/`GetParent`，已确认存在。

### 方案 B：源码 span 重解析（offset re-parse）— 推荐作为第二阶段优化

pass1 只记录每个体的源码区间（`ctx.GetStart()/GetStop()` → byte offset，结合 `MemEditor` 源码）。pass1 后 **整树 100% 释放**。pass2 lazy 钩子触发时，对该 span 重新做一次**局部解析**得到小 AST，再 build。

- 优点：pass1 后内存几乎归零（只剩 offset 整数 + 已 emit 的 SSA）；内存最优。
- 代价：pass2 需要为每个体做一次局部 antlr 解析（CPU 换内存）；局部解析需要正确的上下文规则入口（function/method/block 各自的 parser 规则），每语言需适配；行列/Range 偏移要做基准平移以保证定位准确。

### 方案 C：完整前端 IR 序列化（AST→自定义紧凑 IR）

pass1 把每个体翻译成一套自定义、紧凑、与 ANTLR 解耦的中间结构，pass2 从该 IR 构建。

- 优点：彻底摆脱 ANTLR 生命周期；可序列化、可缓存、利于未来并发与增量。
- 代价：等于给每种语言重写一套前端 IR 与两套遍历，工作量与回归风险最大。本计划**不作为近期目标**，仅作为长期方向记录。

### 选型建议

分两步走：**先做方案 A**（用最小改动把内存峰值从“全工程整树”降到“全工程函数体子树”，并消除整树二次遍历与重复 build），**再对热点语言/大体量文件引入方案 B**（把残留的函数体 AST 也换成 offset，逼近内存下限）。方案 C 留作长期架构演进。

## 5. 实施路线（分阶段、可独立验证）

### 阶段 0：基线与度量（先做，避免“盲改”）

1. 选 1~2 个大工程样本（如大型 Java/TS 仓库）。
2. 在 `parseProjectWithFS` 内或通过 `pprof` 采集：`f1` 结束时、`f3` 中段、`Finish` 后的 `HeapInuse`；以及 `f1/f3` 墙钟时间。
3. 记录基线，作为后续每阶段的对比口径。

### 阶段 1：消灭“整树 top-level 闭包”，top-level 与骨架在 pass1 内联完成

目标：`RegisterRootTopLevel` 不再捕获整份文件 AST。

1. 把“文件级 top-level”拆成两类：
   - **定义/声明**（包名、类型壳、函数/方法签名、全局变量名、import）：pass1 单次遍历内**立即 emit 成 SSA 骨架**，跨文件可见。
   - **可执行 top-level 语句块**（init、文件级语句、含跨文件引用的全局变量初始化）：当作“伪函数体”，注册成 lazy 钩子（与函数体同等待遇），**只捕获该语句块子树**，不捕获整树。
2. 删除/退役 pass2 中“整树 `BuildFromAST`”的 top-level 路径；root build 阶段只跑“函数 / blueprint / top-level 语句块 / helper”这些 SSA 自有节点。
3. 修复 §2.2 的重复 build：明确每个体的构建只发生在唯一一处（pass1 emit 骨架，pass2 emit 体），删除 `PreHandlerProject` 里多余的 `prog.Build` 整树调用。

涉及文件（主）：
- `common/yak/ssaapi/ssa_compile_fs.go`（`f1` 注册逻辑、移除整树闭包）
- `common/yak/ssa/root_builder.go`、`common/yak/ssa/program.go`（root build 节点种类与注册）
- 各语言 `builder.go` / `builder_ast.go` / `builder_prehandler.go` 的 `PreHandlerProject` 与 `BuildFromAST` 边界划分

### 阶段 2：lazy 钩子子树剥离（方案 A），释放文件级整树

目标：pass1 走完一个文件后，该文件整树可被 GC。

1. 在 `ssa` 层提供一个语言无关的工具，例如：

```go
// common/yak/ssa/ast_detach.go
type DetachableAST interface {
    SetParent(antlr.Tree)
}

// DetachAST 把子树从父树剪断：清父指针 + 从父 children 移除，
// 使子树自包含、可独立存活，父树整体可回收。
func DetachAST(node antlr.ParserRuleContext) antlr.ParserRuleContext {
    if node == nil { return nil }
    if p, ok := node.GetParent().(antlr.ParserRuleContext); ok && p != nil {
        // 从 p.children 中移除 node（按引用比对）
        removeChild(p, node)
    }
    node.SetParent(nil)
    return node
}
```

2. 各语言在 `AddLazyBuilder` 前，把要捕获的体（`fun.Block()` / `i.MethodBody()` / top-level 语句块）先 `DetachAST` 再捕获：

```go
body := ssa.DetachAST(fun.Block())
newFunc.AddLazyBuilder(func() {
    ... // 只用 body，不再向上 GetParent()
    b.buildBlock(body, true)
})
```

3. 审查并消除 lazy 闭包内对“父级上下文”的依赖：凡 build 体时需要的父级信息（外层类型参数、包名、import、外层作用域），必须在 pass1 固化进 `store`/骨架，不能在 pass2 通过 `GetParent()` 回溯。这是阶段 2 的主要工程量与风险点。

涉及文件（每语言各一处或数处 `AddLazyBuilder`）：
- `common/yak/go2ssa/builder_ast.go:171,752,912`
- `common/yak/java/java2ssa/visit_package_n_class.go:127,229,543,620,889`、`visit_interface.go:34`
- `common/yak/typescript/ts2ssa/build_from_ast.go:1030,2794,2895,2971,3045,4251,4267`、`auxiliary.go:411`
- `common/yak/php/php2ssa/builder.go:157`、`visit_cls.go:253,293,320`、`visit_func.go:33`
- `common/yak/python/python2ssa/visit_class.go:279`
- `common/yak/ssa/ssa_globalBulePrint.go:28`

### 阶段 3（可选，热点优化）：方案 B offset 重解析

仅对“函数体特别大 / 文件特别多”的语言开启。

1. lazy 钩子改为捕获 `(editorURL, startOffset, stopOffset, 规则入口枚举)`，不持有任何 AST 节点。
2. 钩子触发时用 `MemEditor` 取出该 span 源码，调用语言专用的“局部解析”入口（function/method/block 级 parser 规则）得到小 AST，build 后即丢弃。
3. 校正 Range：以 span 起点为基准做行列/offset 平移，保证 SSA 指令定位与全文件一致。

### 阶段 4：清理与文档

1. 删除已死代码：`buildFileContent`、`collectCompileTargets`（若阶段 1 后不再使用）、`Program.VisitAst` 兼容空函数等。
2. 更新 `docs/ssa-lazy-root-build.md`，与本计划合并/对齐。
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

- **父级依赖泄漏（最大风险）**：剥离子树后 pass2 若仍 `GetParent()`，将 panic 或定位错乱。缓解：阶段 2 前用 grep 审查各 `AddLazyBuilder` 闭包体内是否调用 `GetParent`/外层 `ctx`；逐语言、逐文件灰度。
- **跨文件依赖回归**：把“可执行 top-level”降级为 lazy 后，需确保其在所有定义注册完成后才执行（保持在 `Finish`/root build 阶段，而非 pass1）。
- **Range/定位回归**：方案 B 偏移平移易错，需专门的定位单测。
- **回滚策略**：每阶段独立成 commit；阶段 2/3 用语言级开关（如 builder 上的 `detachAST` / `offsetReparse` flag）控制，可单语言回退。

## 8. 验证

1. **正确性**：全量跑各语言 `*_test.go` 与 SyntaxFlow 规则验证；重点回归 `common/yak/ssa/root_builder_test.go`、各语言 `test/` 用例。
2. **内存**：用阶段 0 的样本与口径对比 `f1`/`f3` 峰值 `HeapInuse`，目标——阶段 1 显著下降、阶段 2 降到“函数体子树量级”、阶段 3 逼近“仅 SSA + offset”。
3. **速度**：对比编译墙钟，确认消除整树二次遍历/重复 build 后 `parseTime` 下降；方案 B 需确认局部重解析的 CPU 增量可接受。
4. **基准**：补充一个可复现的 bench（大工程 fixture），纳入回归。

## 9. 落地顺序小结

1. 阶段 0 度量基线。
2. 阶段 1：top-level 内联 + 去整树闭包 + 去重复 build（先拿到“结构正确、无整树二次遍历”的版本）。
3. 阶段 2：方案 A 子树剥离（拿到内存主收益）。
4. 阶段 3：方案 B 热点语言 offset 重解析（拿到内存下限）。
5. 阶段 4：清理与文档。
