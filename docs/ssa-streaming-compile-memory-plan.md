# SSA 编译：编译单元粒度 + 依赖拓扑序的单遍流式编译（大工程内存上界方案）

> 目标分支：`refactor/ssa/compile_step_shrink_ast`
> 前置文档：`docs/ssa-ast-to-ssa-skeleton-plan.md`（ANTLR 原地瘦身 + lazy 子树）、`docs/ssa-deferred-build.md`、`common/yak/ssaapi/docs/ssa_ircode_cache_mechanism.md`
> 本文定位：在上一轮“AST 瘦身”之后，从根上解决 2G 级工程 OOM 的**结构性**问题。借鉴传统编译器“分离编译”，把内存上界与项目总规模解耦。

---

## 0. 决策记录（已拍板）

1. **主方案 = 编译单元粒度（按包/目录）+ 依赖拓扑序的单遍流式编译**；体级重解析仅作为“单个超大单元”的局部兜底，不是默认路径。
2. **动态依赖语言（PHP、JS/TS）兜底**：按目录粗切单元，目录间无序，但**每个单元编译完立即释放该单元 AST**；跨单元前向引用沿用现有 virtual-lib / `fixImportCallback` 惰性解析。
3. **依赖边抽取按语言选择**：显式依赖语言（Go/Java/C/Python）用**轻量扫描 import/include**（不建全 AST）产出依赖图；其余语言不单独建图，**复用单元解析过程中发现的依赖**喂给 `UpStream`。

---

## 1. 现状与根因

主流程在 `common/yak/ssaapi/ssa_compile_fs.go: parseProjectWithFS`，6 阶段：f1 并发解析**全工程** + `PreHandlerProject` 建骨架并 `AddLazyBuilder` 捕获**体**子树；f3 `RunDeferredBuilds` 统一展开所有体；f4 finish；f5 IR 刷库。

- IR（instruction）在 DB 模式下由 `ProgramCache` 按 TTL/容量 spill，**已是有界**。
- 但被 lazy 闭包捕获的**“体”AST 子树是普通闭包，不 spill、不提前释放**，从 f1 注册一直存活到 f3 执行到它。

**根因（与项目规模线性、无上界）：**

```
peak_AST ≈ Σ_over_all_functions_methods_toplevel ( body_subtree_i )
```

`DetachAST`（`common/yak/ssa/ast_detach.go`）只切断“一个体通过 parentCtx 钉住整文件”的放大效应，**无法缩小体子树自身**，更无法减少“全工程体同时存活”的数量。所以 2G 工程必然 OOM。

---

## 2. 当前分支评估：保留地基，替换编排

| 归类 | 内容 | 处置 |
|---|---|---|
| 基础设施（保留并复用） | IR cache spill / lazy reload / save 确认（`ProgramCache`）、deferred build 调度、`StoreFunctionBuilder` 字段快照、heap 度量（`YAK_SSA_HEAP_LOG`）、并发 read/parse 窗口（`astBuildWindowSize`）、`DetachAST`（降级为单元内多体隔离） | 直接复用 |
| 编排骨架（需替换） | “f1 解析全工程 + 攒所有体闭包 → f3 统一展开”——无内存上界 | 替换为单元粒度流式编排 |

结论：**当前分支是好地基不是终点**。基础设施继续用；“全工程一次性 prehandler + 全量攒体”这个编排必须替换，而不是继续缝补。

---

## 3. 传统编译器经验 → 本项目映射

C/C++、Go、Rust、Java 都**不把整个项目 AST 同时拿在手里**：以**包/模块/翻译单元**为粒度、**按依赖拓扑序逐个编译**；依赖单元以**紧凑导出信息**（export data / .class / crate metadata / 头文件）可见，而非重新加载 AST；**编完一个单元就释放它的 AST**。

惊喜：**本项目架构已具备一半条件**。

| 传统编译器 | 本项目现状（已存在） |
|---|---|
| 翻译单元 / 包 | `Application` 下的 `UpStream` library 子程序（`createSubProgram`/`NewLibrary`/`GetOrCreateLibrary`）——包→library 已成立 |
| 依赖单元的导出信息 | library 的骨架 + IR（IR 已可 spill DB / lazy reload；library 间共享同一 `Cache`） |
| 链接 / 跨单元解析 | `GetLibrary`（查 `app.UpStream` / `prog.UpStream`）+ `fixImportCallback` + virtual-lib |

迁移要点：

1. **不要全局视野，只要依赖视野** → 单元拓扑序，而非“看全工程”。
2. **依赖用紧凑导出信息，不用 AST** → library 骨架/IR（可 spill DB）。
3. **编译单元用完即弃** → 单元编译完释放 AST，内存界 = 最大单元。
4. **环 = 一起编译** → SCC 合并成一个单元。
5. **语言相关的只有“切单元 + 抽依赖”** → 用 interface 下放各语言；引擎语言无关。

---

## 4. 目标与硬约束

目标：

- **内存上界与项目总规模解耦**：峰值 ≈ `O(最大单元 AST + IR cache 窗口)`，而非 `O(全工程体 AST)`。
- **单遍解析**：正常路径不重复解析（重解析仅超大单元兜底）。
- 跨文件/跨包 import、类型、继承解析结果不回归。

硬约束：

1. 编译单元 U 的体 build 时，其依赖单元的**骨架/导出符号必须可见**（拓扑序保证；无序兜底用惰性 virtual-lib）。
2. 任意时刻常驻“体 AST”有界（= 当前单元/SCC，而非全工程）。
3. 体 build 不依赖 `GetParent()`：父级上下文（外层类型、包名、import、外层作用域）在单元解析时固化。
4. 复用 `ProgramCache` IR spill、`UpStream`/library、deferred build；不另起炉灶。

---

## 5. 方案：单元粒度 + 拓扑序的单遍流式编译

把“parse 全工程 → 攒所有体 → 统一展开”改为“**逐单元：解析→建骨架+体→刷 IR→释放 AST→下一个单元**”。

```
S0  扫描文件 → 各语言 PartitionUnits 切编译单元（默认=目录/包）
S1  各语言 UnitDependencies 抽依赖边（显式语言轻量扫 import；动态语言跳过）
S2  引擎建依赖图 → 求 SCC（合并环）→ 拓扑排序
S3  for each 单元(或SCC) in 拓扑序:
       解析该单元文件（只此一遍）
       单元内部：现有“骨架 → 体 lazy”两阶段，但只针对本单元文件
       emit IR → spill DB
       【释放整个单元的 AST】
       后续单元引用它时，从已持久化骨架/IR(可 DB reload)解析，不碰 AST
S4  Finish / Blueprint 构造析构 / 元数据 / IR flush（沿用 f4/f5/f6）
```

### 5.1 显式依赖语言（Go / Java / C / Python）

- `UnitDependencies` 用**轻量扫描**（只识别 `import` / `package` / `#include` 行，正则或 import-only 子解析），成本远低于建全 AST，**不构成第二次全量解析**。
- 拓扑序保证：编译单元 U 时，其依赖单元已编译并持久化，U 的体直接解析到依赖符号。
- 内存上界 = 最大单个单元（或最大 SCC）的 AST。Go 包级无环，SCC 几乎都是单包，最理想。

### 5.2 动态依赖语言（PHP / JS/TS）兜底（决策 2 + 3）

- 不建精确依赖图（`SupportsDependencyOrder()==false`）。`PartitionUnits` 按**目录粗切**单元。
- 引擎按**确定性目录顺序**逐单元编译，**每个单元编译完立即释放该单元 AST**。
- 跨单元前向引用（引用尚未编译单元的符号）沿用现有 **virtual-lib + `fixImportCallback`** 惰性解析；依赖关系“边解析边发现”喂给 `UpStream`（决策 3 的“复用单元解析结果”）。
- 内存上界 = 最大单个目录单元 AST；骨架（小）随全工程累积常驻，体 AST 不累积。
- 已知取舍：无序模式下前向跨单元解析精度依赖惰性机制，弱于“全骨架先行”。这些语言工程通常远小于 2G 级 Java/C 巨仓，可接受（见 §8 风险）。

### 5.3 超大单元兜底（重解析降级为局部）

若单个单元/SCC 自身过大（罕见：一个包就 2G，或巨型 SCC），单元内重新出现“体 AST 同时存活”。此时**仅对该单元**启用体级兜底：

- 单元内先建骨架并丢弃体 AST，再按文件流式重解析、build 体（即上一版“两遍重解析”，但范围缩到一个单元）；或
- 对该单元按子目录再细分。

由阈值（单元字节数 / 文件数）触发，默认关闭，配置可调。

---

## 6. 多语言接口设计（语言相关下放，引擎语言无关）

引擎（`ssaapi` / `ssa`）负责：`Partition → 抽依赖 → SCC + 拓扑 → 逐单元驱动 → 单元后释放 AST`。语言只实现“切单元 + 抽依赖”，单元内编译复用现有 `PreHandlerProject` / `BuildFromAST`（输入文件集从“全工程”缩成“一个单元”）。

接口草案（挂在 `ssa.Builder` / `PreHandlerBase`）：

```go
type CompileUnit struct {
    Key   string   // 单元标识（包名/目录路径）
    Files []string // 该单元文件清单
}

type UnitRef struct {
    From string // 依赖方单元 Key
    To   string // 被依赖单元 Key
}

type UnitPartitioner interface {
    // 把文件清单切成编译单元；语言决定粒度
    // Go/Java/Python: 按 package(目录); C/C++: 目录或文件; PHP/JS: 按目录
    PartitionUnits(files []string, fs FileSystem) []CompileUnit

    // 是否能给出可靠依赖图；false 时引擎走无序+目录粗切兜底(§5.2)
    SupportsDependencyOrder() bool

    // 仅当 SupportsDependencyOrder()==true：轻量抽依赖边(不建全 AST)
    UnitDependencies(units []CompileUnit, fs FileSystem) []UnitRef
}
```

引擎驱动伪码：

```go
units := lang.PartitionUnits(files, fs)
var order [][]CompileUnit            // 每项是一个 SCC（通常单元素）
if lang.SupportsDependencyOrder() {
    edges := lang.UnitDependencies(units, fs)
    order = topoSortSCC(units, edges) // 环并入同一 SCC
} else {
    order = deterministicDirOrder(units) // §5.2 无序兜底
}
for _, scc := range order {
    parseUnit(scc)        // 只此一遍
    buildSkeletonAndBodies(scc)
    flushIR(scc)
    releaseUnitAST(scc)   // 关键：单元后释放
}
```

各语言落地：

| 语言 | 单元粒度 | `SupportsDependencyOrder` | 依赖抽取 | 备注 |
|---|---|---|---|---|
| Go | package(目录) | true | `import` 显式、极廉价 | 包级无环，SCC 多为单包，最理想 |
| Java | package | true | `package` + `import` | 可能包间环 → SCC 合并；方法体外层 class 上下文需固化 |
| C | 目录/文件 | true | `#include` | 头文件天然是导出信息 |
| Python | 目录/模块 | true | `import` | 相对明确 |
| PHP | 目录 | false | 复用单元解析发现 | autoload 动态 → §5.2 兜底 |
| JS/TS | 目录/模块 | false | 复用单元解析发现 | 动态 require → §5.2 兜底 |
| C++/C#(未来) | 目录/namespace | true(预期) | include / using | 接口已留 |

关于 ANTLR：解析成本低、内存高 → 本方案“单遍解析 + 单元后立即释放”正好对症：同时存活的 AST 始终只有一个单元大小；显式语言的依赖抽取走轻量扫描，不引入第二次全量解析。

---

## 7. 与现有机制衔接

- **library / UpStream**：单元 = library 子程序，已存在；本方案补“拓扑序驱动 + 单元后释放 AST”。
- **跨单元解析**：拓扑序下依赖已编译，从 `UpStream` 命中或 DB reload（注意：`GetLibrary` 当前对“不在内存的库”返回 nil，DB reload 路径目前被注释——拓扑序下依赖库骨架可保持常驻，无序兜底沿用 virtual-lib；若要支持依赖库骨架 DB 卸载，需打开该 reload 路径，留待后续）。
- **IR cache**：完全复用，单元内 build 出的 IR 照常 spill。
- **deferred build / LazyBuilder**：保留，作用域从“全工程”收敛到“单元内”。
- **DetachAST**：从“跨文件防钉住”降级为“单元内多体隔离”，可逐步退役。
- **增量编译 / overlay**：单元骨架天然适合做“接口指纹”，与 `FileHashMap`/overlay 不冲突；本轮不扩展。

---

## 8. 风险与回滚

- **单元划分 / 依赖抽取不准（显式语言）**：轻量扫描漏边导致拓扑序错 → 体 build 时依赖未就绪。缓解：扫描保守宁可多连边；漏边时回退到惰性 virtual-lib 解析（与 §5.2 同机制）兜底，不致编译失败。
- **无序模式跨单元前向引用精度（PHP/JS）**：弱于全骨架先行。缓解：确定性目录序 + 骨架常驻 + 惰性解析；用规则回归确认无明显漏报。
- **巨型 SCC**：内存兜底触发重解析/再细分；阈值可配。
- **父级依赖泄漏**：体 build 仍 `GetParent()`/外层闭包取父级会失效。缓解：grep 审查各 `AddLazyBuilder` 体闭包，父级上下文固化进单元级 store；逐语言灰度。
- **正确性回归**：IR diff（固定样例多次编译稳定）+ 各语言用例 + SyntaxFlow 规则。
- **回滚**：保留旧“全工程 f1/f3”路径作为开关，逐语言切换；每语言独立 commit 可回退。

---

## 9. 实施路线（分阶段、可独立验证）

### 阶段 0：基线度量
`YAK_SSA_HEAP_LOG=1` + `YAK_SSA_HEAP_PROFILE_DIR` 采集现状 f1/f3/f4 `HeapInuse`、峰值 RSS、墙钟；选含一个会 OOM 的 2G 级样本（可用 cgroup 限内存复现）。

### 阶段 1：引擎侧单元驱动框架（语言无关）
- 定义 `CompileUnit` / `UnitRef` / `UnitPartitioner`；引擎实现 `topoSortSCC` / `deterministicDirOrder` / 逐单元驱动 + 单元后 `releaseUnitAST`。
- `parseProjectWithFS` 改造成单元循环；保留旧全工程路径作开关。

### 阶段 2：Go 打通端到端（显式语言样板）
- Go `PartitionUnits`=按包目录；`UnitDependencies`=轻量扫 `import`；`PreHandlerProject` 拆“骨架 only”，体改单元内 lazy；`importMap`/泛型/包名固化进单元 store。
- 验证：Go 全量用例 + 规则；IR diff 稳定；大 Go 工程内存对比阶段 0。

### 阶段 3：无序兜底打通（PHP 或 JS/TS 之一）
- `SupportsDependencyOrder()==false` 路径：目录粗切 + 确定性序 + 单元后释放 + 惰性跨单元解析。
- 验证：动态引用样例不回归。

### 阶段 4：超大单元内存兜底
- 阈值触发单元内体级重解析/子目录细分；config + 环境变量旋钮，默认关闭。

### 阶段 5：推广到 Java / C / Python / 其余
- 逐语言实现 `PartitionUnits` / `UnitDependencies` 与父级上下文固化，删除对应旧全工程体捕获路径。

### 阶段 6：清理与文档
- 退役/收敛 `DetachAST` 跨文件用途、旧路径开关；合并更新本文与 `ssa-deferred-build.md`。

---

## 10. 验证

1. **正确性**：`scripts/ssa-test.sh` 各语言用例 + SyntaxFlow 规则；重点 `common/yak/ssa/deferred_build_test.go` 及各语言 `test/`。
2. **内存（核心验收）**：2G 样本峰值不再随项目规模线性增长、不再 OOM；峰值 ≈ 最大单元量级。
3. **速度**：墙钟对比，确认单遍 + 轻量依赖扫描相对现状无明显回退。
4. **IR diff**：固定样例多次编译 IR 计数稳定。
5. **基准**：补可复现大工程 fixture bench，纳入回归。

---

## 11. 一句话总结

不再“看全工程”，改“看依赖”：把工程按包/目录切成编译单元，显式语言按依赖拓扑序、动态语言按目录确定性序，**逐单元单遍解析、build、刷 IR，然后立即释放该单元 AST**，依赖通过已持久化的 library 骨架/IR 解析。用传统编译器“分离编译、AST 用完即弃”的思路，把内存上界从“全工程体 AST”压到“最大单个单元”，且不引入第二次全量解析。
