# MINI_VECTOR_SCAN_IMPL —— 自托管多正则引擎实现方案（前端 Go + 纯 C 运行期内核）

> 代号 **mvscan**（mini vectorscan）。目标：做出一个**运行期内核为纯 C、像 sqlite 一样几乎完全
> 平台无关 / CPU 无关（兼容 + 退化）、靠算法提升效率的自托管多正则存在性引擎**，让它在
> Windows / Linux / macOS 三端都有一席之地。
>
> 本版相对初版的关键调整（依据最新反馈）：
> 1. **接受编译期依赖 `regexp/syntax`（RE2）与 `regexp2`（PCRE 系）作为 AST 来源**，不再自写 C parser。
>    为此设计一个 **Unify Regexp AST** 中间表示承载两种语法树（见第 3 节）。
> 2. **被丢弃的性能/CPU 相关组件不是永久删除**，而是登记"未来回收触发条件"，需要时再拿回来（见第 5 节）。
> 3. **bridge 不删**：在 mvscan 通过全部一致性 + 性能验收之前，保留 `vectorscan_bridge.go` /
>    `vectorscan_stub.go` / 相关测试，作为性能与正确性的对照基线。

## 0. 一个必须先讲清的查证结论（影响架构）

落地"用 re/re2 的语法块构建 unify AST"前，已实地核对 yaklang 依赖（`dlclark/regexp2 v1.11.0`）：

- **`regexp/syntax`（标准库 RE2）**：AST 类型 `*syntax.Regexp` 及其 `Op / Rune / Sub / Min / Max /
  Flags` 字段**全部公开、可遍历**。是 unify AST 的可靠来源。
- **`dlclark/regexp2 v1.11.0`**：`RegexTree` 虽导出，但其 `root *regexNode`、`regexNode` 结构、
  所有字段（`t/children/str/set/ch/m/n`）以及 `nodeType` 常量**全部 unexported**；该 `syntax` 子包
  对外仅暴露 `Write / RegexTree.Dump() / Prefix / BmPrefix / ReplacerData`。**外部包无法以编程方式
  遍历 regexp2 的 AST**。
- 另一个本质事实：regexp2-only 的构造（lookaround / backreference）属于**非正则语言**，即使拿到其
  结构也**无法进入 Glushkov NFA**（数学边界，非实现缺陷）。

因此 unify AST 的定位是：
- **RE2 来源**：`*syntax.Regexp` → unify AST → 既可建 NFA、也可统一提字面量。直通无障碍。
- **regexp2 来源**：因 AST 封闭 + 非正则，**不进 NFA 核**；unify AST 在这里只承担"统一字面量提取 +
  分流标注"，最终匹配仍由 `regexp2` 兜底验证（沿用现有 always-on 路径）。其字面量获取**已定为：
  起步走近似提取（路线 B，零 fork、零维护），并在前端预留 vendor 增强（路线 A）接口按需升级**（见 3.4）。

## 1. 目标与非目标

### 1.1 架构定位：双层

- **前端（Go，编译期，冷路径，一次性）**：解析（`regexp/syntax` 优先，`regexp2` 兜底）→ **Unify AST**
  → Glushkov NFA → 字面量提取 → Teddy 分桶 → 序列化为**平台无关只读 blob**。复用 yaklang 现有成熟件。
- **后端（纯 C，运行期，热路径，sqlite-like）**：加载 blob（零拷贝）→ Teddy 预过滤（SIMD/标量）→
  bit-parallel NFA 验证 → 存在性命中。**这一层才是"纯 C99、自托管、平台/CPU 无关、可 amalgamate、
  可独立分发"的核心。**

> "纯 C 像 sqlite" 精确落在**运行期内核**：它是性能关键、需要到处编译、需要 SIMD/标量退化、需要被任意
> 语言通过 FFI 调用的部分。编译期是一次性冷路径，用 Go 复用成熟 parser 不损害"运行期可独立分发"，反而
> **砍掉自写 C parser 这一最大正确性风险与最大工时**。

### 1.2 目标（必须达成）

1. **运行期内核纯 C99**：可合并为单一 `mvscan.c` + `mvscan.h`（amalgamation），任何 C99 编译器
   （MSVC / MinGW / GCC / Clang）一条命令编译，**无需 CMake / Ragel / Colm / Boost / 任何第三方**。
2. **自托管运行期**：C 后端只认 `blob + data`，不依赖 libhs、不依赖系统正则、不依赖 Go 运行时。
3. **平台无关**：不使用 OS 专有 API、不依赖线程库、不依赖动态加载；三端行为一致。
4. **CPU 无关（兼容 + 退化）**：靠算法取得主要收益，SIMD 是可选加速。x86_64 标量基线→运行时探测
   SSSE3→AVX2；arm64 NEON；其它架构纯标量。任何 SIMD 路径都有等价标量孪生；探测不到一律退化。
5. **性能**：真实 MITM 规则集 + 真实流量上达 **~100x（相对 StdlibLoop）**，追平/超越当前 bridge(87x)；
   纯标量退化档显著快于逐条匹配。
6. **语义**：**多正则存在性匹配**（block）。命中以 `From/To=-1` 上报，与现有 vectorscan/regexp2-only
   后端一致。
7. **可验证**：与 stdlib RE2 oracle 做逐记录命中规则 ID 集合差分，必须完全一致；随机正则 fuzz；跨架构 CI。

### 1.3 非目标（明确不做，理由见第 5 节）

- 不做精确偏移 / leftmost-longest / 捕获组 / start-of-match。
- 不做 streaming / vectored（只 block）。
- 不做 backreference / 任意 lookaround 的 NFA 化（数学边界）；这类仍走 regexp2 兜底。
- 不追求全量 PCRE / 全量 Hyperscan 兼容；不做 Chimera。
- 不做 Hyperscan 跨架构 / 带版本协商的完整数据库序列化；但**做一个精简的平台无关 blob 用于前后端解耦**
  （顺带可落盘缓存，见 7.3）。

## 2. 总体架构

```
┌──────────────────────────── 前端 (Go, 编译期, 一次性) ────────────────────────────┐
                          ┌─ regexp/syntax (RE2, AST 公开) ─┐
 patterns[] ─► 解析分流 ──┤                                  ├─► Unify AST ─┐
                          └─ regexp2 (PCRE系, AST 封闭/非正则)┘              │
                                                                            ▼
   ┌──────────────────────── Unify AST 统一消费 ────────────────────────────────┐
   │  regular 子树 ─► Glushkov NFA ─► (per-pattern 小 bit-NFA)                    │
   │  全部子树     ─► 字面量提取 ─► Teddy 分桶 (bucket→patternID)                 │
   │  non-regular  ─► 标注 fallback (regexp2 验证, 可带字面量预过滤)              │
   └────────────────────────────────────────────────────────────────────────────┘
                                       │ 序列化
                                       ▼
                          平台无关只读 blob ([]byte, POD/小端/对齐)
└───────────────────────────────────────────────────────────────────────────────────┘
                                       │ cgo 传入
┌──────────────────────────── 后端 (纯 C, 运行期, 热路径) ──────────────────────────┐
 blob ─► 零拷贝解析 ─► Teddy 预过滤(SIMD/标量+退化) ─► 候选(pos,bucket)              │
                                                          │                          │
        always-on(无字面量正则) bit-NFA 全程运行 ◄────────┘  触发 per-pattern bit-NFA │
                                                             命中→标记ID(去重)→可提前停 │
└───────────────────────────────────────────────────────────────────────────────────┘
```

两个区别于 Hyperscan 的关键简化：**per-pattern 小 bit-NFA + 字面量触发**（非合并巨型 NFA）；
**Teddy 只做预过滤定位候选**，真伪由 bit-NFA 判定（只假阳、绝无假阴）。

## 3. Unify Regexp AST 设计（本版核心）

### 3.1 为什么需要它

- 两个解析来源（RE2 / regexp2）结构迥异；下游（Glushkov、字面量提取、分流）若各写一套会重复且易漂移。
- 用一个**统一中间表示**，让"建 NFA"和"提字面量"对来源无感，并显式携带"是否含非正则构造"的标注，
  从而干净地把每条 pattern 分流到 **NFA 核** 或 **regexp2 兜底**。
- 它是**前端 Go 内部**的 IR，**不进 C**（C 只吃 blob）。

### 3.2 节点定义（Go）

```go
type UnifyOp uint8

const (
    UOpEmpty     UnifyOp = iota // 空 (可空匹配)
    UOpLiteral                  // 字面字节串 (已按 caseless 规整)
    UOpClass                    // 字符类 [..]、.、\d\w\s 等 -> 字节集合
    UOpConcat                   // 顺序连接
    UOpAlternate                // a|b
    UOpStar                     // x*       (greedy 标记仅记录, 存在性下不影响命中集)
    UOpPlus                     // x+
    UOpQuest                    // x?
    UOpRepeat                   // x{m,n} (n=-1 表示无上界)
    UOpAnchor                   // ^ $ \A \z \b \B (类型见 AnchorKind)
    UOpNonRegular               // lookaround / backref 等: 不可进 NFA, 仅占位
)

type UnifyNode struct {
    Op       UnifyOp
    Lit      []byte       // UOpLiteral
    Class    *ByteClass   // UOpClass: 256-bit 字节集合 (UTF-8 按字节)
    Sub      []*UnifyNode // 复合节点子树
    Min, Max int          // UOpRepeat
    Greedy   bool         // 记录但存在性匹配下不区分
    Anchor   AnchorKind   // UOpAnchor
    // 来源标注: 决定该子树能否进 Glushkov
    Regular  bool         // 该子树及其后代是否全为正则构造
}
```

`ByteClass` 用 `[4]uint64`（256 位）表示字节集合，`.`/`\d`/`[^...]` 等都规约成它，
让 Glushkov 的 `reach[c]` 构建与 caseless 折叠统一处理。

### 3.3 从 `regexp/syntax`（RE2）映射

`syntax.Parse(expr, syntax.Perl).Simplify()` 后逐节点转换（标准库 `Op` 稳定可靠）：

| `syntax.Op` | UnifyNode | 说明 |
|---|---|---|
| `OpEmptyMatch` | `UOpEmpty` | |
| `OpLiteral` | `UOpLiteral` | `Rune[]`→字节；`FoldCase` 标志触发 caseless 规整 |
| `OpCharClass` | `UOpClass` | rune range → 字节集合（>0x7F 走 UTF-8 字节展开） |
| `OpAnyChar` / `OpAnyCharNotNL` | `UOpClass` | 全字节 / 除 `\n` |
| `OpStar/OpPlus/OpQuest` | `UOpStar/Plus/Quest` | `NonGreedy`→`Greedy=false` |
| `OpRepeat` | `UOpRepeat` | `Min/Max`（`Max<0` 无上界） |
| `OpConcat/OpAlternate` | `UOpConcat/Alternate` | |
| `OpCapture` | 透明剥离（取 `Sub[0]`） | 存在性不需要分组 |
| `OpBeginText/EndText/BeginLine/EndLine/WordBoundary/NoWordBoundary` | `UOpAnchor` | |
| `OpNoMatch` | 整条标记不可命中 | |

来自 RE2 的子树 `Regular=true`，可全程进 Glushkov。

### 3.4 从 `regexp2`（PCRE 系）映射 —— 两条可选路线

事实：v1.11.0 的 regexp2 AST 不可遍历，且 lookaround/backref 非正则。对仅 regexp2 可编译的 pattern：

- **路线 A（推荐，精确）**：在 yaklang 维护一个**最小增强的 regexp2 syntax**（vendor 一份，或向上游加
  `func (t *RegexTree) Walk(fn)` / `ToUnify()`），把节点结构以只读方式暴露，转成 UnifyNode（含 non-regular
  占位）。yaklang 本就深度依赖 regexp2，增量可控；收益是能从 lookaround 外层的"必经字面量"精确提指纹。
- **路线 B（近似，零 fork）**：不碰 regexp2 内部。对 pattern 文本做**轻量字面量近似提取**（剥除
  `(?=...)`/`(?<=...)`/`\1` 等包装后，用 `regexp/syntax` 解析可解析的骨架来取必需字面量）；提不出就归
  `always-on`。**最终匹配一律用 regexp2 验证**，故语义绝对正确，路线只影响"预过滤收益"高低。

**决策（已定）：起步采用路线 B** —— 零 fork、零维护、语义绝对正确（regexp2 兜底），仅在预过滤收益上
保守；前端预留路线 A 的转换接口，待真实 MITM 中 regexp2-only 占比与 profile 显示收益不足时再升级到 A。

无论 A/B：regexp2-only pattern 的 UnifyNode 顶层带 `Op=UOpNonRegular` 标注，**不进 NFA 核**，只用于
字面量提取；运行期由 Go 侧 regexp2 做存在性验证（可被 Teddy 命中触发，减少全量扫描）。

### 3.5 统一下游

- **字面量提取**：对任意来源的 UnifyNode 走同一套"必需字面量 OR 集"算法（移植现有 `literal.go` 思路，
  改吃 UnifyNode）。
- **Glushkov**：只接受 `Regular==true` 的整树；含 `UOpNonRegular` 的整条 pattern 走 regexp2 兜底。
- **分流**：`regular + 有字面量`→Teddy 触发 bit-NFA；`regular + 无字面量`→always-on bit-NFA；
  `non-regular`→always-on regexp2（尽量带字面量预过滤）。

## 4. 保留什么 & 为什么

| 组件 | 位置 | 作用 | 为什么保留 |
|---|---|---|---|
| **Go 解析前端（syntax+regexp2）** | 前端 | 文本→AST | 复用成熟实现，**砍掉自写 C parser 的最大风险/工时**；用户已接受该依赖。 |
| **Unify AST** | 前端 | 统一两来源 IR | 让建 NFA / 提字面量 / 分流对来源无感（见第 3 节）。 |
| **Glushkov NFA 构造** | 前端 | AST→ε-free NFA | ε-free + 入边同符号，使运行期退化为几条位运算，是高效根基。 |
| **字面量提取** | 前端 | 抽必需字面量 OR 集 | 决定预过滤收益；命中稀疏时跳过绝大多数数据。移植现有 `literal.go`。 |
| **bit-parallel NFA 执行 (LimEx-lite)** | 后端 C | bitset 位运算推进活跃集 | Hyperscan 性能内核精华；避免逐状态查表，天然 SIMD 友好。 |
| **Teddy SIMD 预过滤** | 后端 C | 多字面量指纹一次扫多字节 | "多串匹配比多正则快两个数量级"；100x 主要来自这里。复用 `teddy.c` 经验升级。 |
| **bounded repeat 展开** | 前端→后端 | `a{m,n}` 展开为有限状态 | 覆盖真实规则常见量词，避免引入 Castle。 |
| **per-pattern 触发编排** | 后端 C | 字面量→触发哪些 NFA | compile-then-scan 的落地，避免合并巨型 NFA。 |
| **CPU 运行时分发 + 标量退化** | 后端 C | SSE/AVX2/NEON/scalar | "CPU 无关（兼容+退化）"机制；沿用 `teddy.c` 的 `#if defined` 架构隔离 + 运行期探测。 |
| **平台无关 blob** | 前后端契约 | 解耦 Go 前端 / C 后端 | 让 C 后端只认字节、可独立分发、可落盘缓存。 |

## 5. 丢弃什么 & 为什么（含"未来回收触发条件"）

> 强调：下面与**性能 / CPU**相关的丢弃项**不是永久删除**，而是登记回收条件。一旦真实负载触发，按表回收。

| 丢弃项 | HS 里的作用 | 为什么现在可丢 | **未来回收触发条件** |
|---|---|---|---|
| **自写 C parser** | 文本→AST | 改用 Go syntax/regexp2，省最大风险/工时 | 若要让 C 后端脱离 Go 完全独立分发给非 Go 项目（远期） |
| **Rose 完整图分解** | 子串+FA 挂图边，极致编排 | 存在性用"字面量→pattern 触发"最朴素编排即可达标 | 当编排粒度不足、需子串级触发以压低误触发率时 |
| **FDR** | >300 字面量大规模多串 | 规则数百级，Teddy 足够 | 当字面量规模膨胀到 Teddy bucket 饱和、误报率升高时 |
| **DFA 族 (Sheng/McClellan/Gough)** | 小状态组件转 DFA 更快 | 只用 bit-NFA 一种模型，省整套 DFA 构建/最小化 | 当少数热点 pattern 的 bit-NFA 成为 profile 瓶颈、且其状态数小适合转 DFA 时 |
| **Castle** | bounded repeat 专用引擎 | NFA 内展开近似（状态可控） | 当大量大区间 repeat 导致状态爆炸、展开成本不可接受时 |
| **SOM (start of match)** | 精确起点 | 存在性不需要 | 若业务从"打标"升级到"需要精确定位/取证" |
| **streaming / vectored** | 跨块流状态 | 只做 block；MITM 整段可得 | 若出现超大流式数据、无法整段载入内存时 |
| **capturing / leftmost-longest** | 子匹配/精确语义 | 存在性只需命中与否 | 同 SOM：需要子匹配内容时 |
| **boost::graph** | 图算法 | 丢上述后只剩 follow 集，bitset/数组即可 | 仅当回收 Rose/DFA 且其图算法复杂到值得引入时（优先自写精简图操作） |
| **Ragel + Colm** | 生成 parser | 用 Go 现成 parser | 同"自写 C parser"回收时才需要（且更可能仍用手写） |
| **Chimera / PCRE 兼容** | PCRE 混合 | 超出存在性目标；regexp2 兜底 | 若需在 C 后端内直接支持 backref/lookaround（基本不会） |
| **fat-runtime 复杂调度** | 单库多 ISA + ifunc | 用"函数指针 + 运行期探测一次"更简单 | 当需要单二进制内多 ISA 共存且分发开销敏感时 |

> 主干一句话：只留"统一编译 + 一次扫描 + 位并行验证"。这正是存在性场景能用 ~1/N 代码逼近 HS 性能的原因。

## 6. 核心数据结构

### 6.1 前端（Go）：Unify AST + 编译产物

见 3.2 的 `UnifyNode`。编译产物（待序列化）：每条 pattern 的小 NFA（`first/accept/follow/reach`）、
Teddy 掩码与 `bucket→patternID`、`alwaysOn` 列表、`patternID→源类型(NFA/regexp2)`。

### 6.2 后端（纯 C）：从 blob 解析的只读结构 + scratch

```c
typedef struct {                 /* 一条 pattern 的小 bit-NFA (Glushkov 位并行) */
    const uint64_t *first;       /* 起始可达 position 集 (nword) */
    const uint64_t *accept;      /* 到达即命中集 */
    const uint64_t *follow;      /* nstate 行, 每行 nword */
    const uint64_t *reach;       /* 256 行, 每行 nword: reach[c] 接受 c 的 position */
    int32_t  nstate, nword;
    uint8_t  anchoredStart, anchoredEnd;
} mvs_nfa;

typedef struct {                 /* Teddy: nibble 指纹 + bucket→pattern */
    uint8_t loMask[32], hiMask[32];
    const int32_t *bucketOff, *bucketPat;
    int32_t nbucket, fpLen;
} mvs_teddy;

typedef struct {                 /* 不可变, 多线程只读共享; 字段指向 blob 内部 (零拷贝) */
    const uint8_t *blob; size_t blobLen;
    mvs_teddy teddy;
    const mvs_nfa *nfas; int32_t nNfa;
    const int32_t *triggerNfa;   /* 有字面量 pattern -> 验证哪个 nfa */
    const int32_t *alwaysOn; int32_t nAlwaysOn;
    const int32_t *patID;        /* nfa/regexp2 槽位 -> 调用方 PatternID */
    int32_t npat;
} mvs_db;

typedef struct {                 /* 每线程一份, 非并发安全 */
    uint64_t *cur, *nxt;         /* bit-NFA 活跃集双缓冲 */
    uint8_t  *hitset;            /* patternID 去重位图 */
    int32_t  *candPos, *candBucket;
    uint8_t  *lower;             /* ASCII 小写缓冲 (caseless 复用) */
} mvs_scratch;
```

## 7. 核心算法

### 7.1 前端：解析、Unify、Glushkov（Go）

1. 对每条 pattern：先 `regexp/syntax.Parse`（成功→RE2 路径，全 regular）；失败→regexp2 路径（见 3.4）。
2. 转 Unify AST；提字面量；regular 整树送 Glushkov。
3. **Glushkov**：自底向上算各节点 `nullable/first/last/follow`，给每个"消费字符的叶子"分配 position；
   产出 `reach[c]/first/accept/follow[]`。保证 ε-free 且入边同符号。

### 7.2 后端：bit-parallel NFA 执行（性能核心，C）

无锚搜索（隐含前缀 `.*?`）核心递推：

```
active = first
for each byte c in window:
    succ = OR( follow[p] for all p set in active )   // 后继并集
    active = succ & reach[c]                          // 仅保留接受 c 的后继
    if (active & accept): 命中, 记录 ID, break        // 存在性可提前停
    active |= first                                   // 无锚: 每位置可作新起点
```

- `nstate ≤ 64`：单 `uint64`，`succ` 遍历 active 置位（`while(a){p=ctz(a);succ|=follow[p];a&=a-1;}`），
  置位通常很少，极快。
- `nstate > 64`：分 `nword` 处理；`succ` 并集是热点 → 后续用 LimEx 的 "limited shift + exception" 优化并 SIMD 化。
- **退化优先**：先朴素正确（任意架构可跑），SIMD/shift 作为加速档叠加。

### 7.3 平台无关 blob 格式（前后端契约）

- 固定 magic + version header；其后是定长头 + 偏移表 + 各段数据（NFA 表 / Teddy / bucket / patID）。
- **小端固定、字段 8 字节对齐**，C 侧零拷贝按偏移取指针（不做反序列化拷贝）。
- 与机器无关（不写指针、不写 Go 内存布局）。**用途**：cgo 传入；副产品是可落盘缓存（编译一次多次加载）。
- MVP 可先 cgo 直接传内存（紧耦合 Go，快速跑通），目标态收敛到 blob（C 后端真正独立）。

### 7.4 Teddy SIMD 预过滤（C）

沿用 `teddy.c` 的 "lo/hi nibble + PSHUFB 查表" 骨架，升级为完整 Teddy：取字面量前 1~4 字节做指纹，
分 ≤8（SSSE3）/≤16（AVX2 Fat Teddy）bucket；一次 16/32 字节，PSHUFB 得"每位置命中哪些 bucket"
位掩码，移位对齐多字节指纹；输出 `(pos,bucket)` 候选，真伪由 7.2 验证。参照 BurntSushi
`aho-corasick` 的 `src/packed/teddy`。

### 7.5 bounded repeat（前端展开）

`a{m,n}` 在 Glushkov 前展开为 `m` 必经 + `n-m` 可选 position（上限如 n≤200，超限：拒绝或交 regexp2）。
`a{m,}` = m 必经 + 闭包。避免专用 repeat 引擎。

### 7.6 运行期编排（C）

```
清空 hitset
Teddy 预过滤(data) -> 候选 (pos,bucket)[]
for 候选: for patternID in bucket:
    若已命中 跳过; 否则在 pos 邻域触发该 pattern bit-NFA; 命中→set hitset, 上报 Match{ID,-1,-1}
for nfa in alwaysOn: 全程跑 bit-NFA; 命中→上报
（regexp2-only: Go 侧 always-on 验证, 可由其字面量被 Teddy 命中触发以减少全量扫描）
handler 返回 false 即整体提前停
```

## 8. CPU 无关：兼容 + 退化（C 后端）

沿用并推广 `teddy.c` 范式：

1. **编译期架构隔离**：`#if defined(__x86_64__)/__aarch64__/else` 三分支，SIMD 分支用架构宏自我隔离，
   **任意架构都能编译**（未知架构只编标量）。
2. **运行期探测一次**：x86 用 `__builtin_cpu_supports` 或 `cpuid`（MSVC `__cpuid`）探 SSSE3/AVX2；
   arm64 NEON 基本恒有。结果缓存。
3. **函数指针分发**：`scan_fn = pick(features)`，从 `{avx2,ssse3,neon,scalar}` 选一；探测失败一律落
   `scalar`。比 fat-runtime 的 ifunc 更简单、可移植。
4. **标量孪生**：每个 SIMD kernel 都有逐字节标量实现，**结果逐位一致**（差分覆盖）。
   口号：**有 SIMD 就快，没有也对、也仍比逐条快**。

## 9. 平台无关：Win / Linux / macOS（C 后端）

- **纯 C99**，只用 `<stdint.h><stddef.h><string.h><stdlib.h>`；不碰 OS API、不依赖 pthread/dlopen/文件。
- **内存**：只用 `malloc/free`，提供可选分配器钩子 `mvs_set_allocator`。
- **字节序/对齐**：blob 小端固定；SIMD 用非对齐 load（`loadu`）。
- **编译器矩阵**：GCC/Clang/**MSVC**(`__cpuid`,`<intrin.h>`,`_mm_*`)/MinGW；intrinsic 用 `#if defined(_MSC_VER)` 兼容分支。
- **amalgamation**：`tools/amalgamate` 拼为单 `mvscan.c`+`mvscan.h`，宿主"丢两个文件即可编"。
- **Windows 立足点**：相对 bridge 的最大胜势——Vectorscan 在 Windows 支持薄弱，纯 C99 的 mvscan 在三端
  同等对待（标量保证可用，x86 上仍可 SSSE3/AVX2 加速）。

## 10. 与 minirehs 集成

- 新增后端 `BackendMVS`（`minirehs.go` 枚举 + `backend.go` 选择），实现 `backendImpl`/`compiledDB`。
- `compile`：前端把每条 `compiledPattern` 经 Unify AST 编译；`regular` 进 NFA blob，`non-regular`
  （现有 `regexp2Verifier`/`!v.exact()`）标记 regexp2 兜底（复用现路径，不丢规则）。
- cgo 绑定纯 C 后端（范式同 `prefilter_cgo.go` 调 `teddy.c`：Go 薄封装，C 出力），传 blob + data。
- 语义存在性，命中 `Match{ID,-1,-1}`，与现有 vectorscan 后端一致，可共用差分测试。
- **CGO_ENABLED=0 / 未带 tag**：`BackendMVS` 优雅退化为现有纯 Go 引擎（stub + selectBackend，同 vectorscan）。
- **bridge 保留**：mvscan 通过全部验收前 `vectorscan_bridge.go` 等不动，作对照基线。

## 11. 正确性保证

1. **三级 oracle**：① stdlib RE2 为最终 oracle；② 前端 Go 内置一个**参考执行器**（按 7.2 在 Go 跑
   bit-NFA），作为"算法规格"与 C 后端逐位对照；③ C 后端各 SIMD 档互相逐位对齐。
2. **差分一致性**：合成随机正则 + 真实 MITM 规则，逐记录比较命中 ID 集合（复用
   `consistency_test.go` / `vectorscan_test.go` 框架）。
3. **fuzz**：复用 `fuzz_test.go` 随机 RE2 生成器差分。
4. **内存安全**：C 侧 ASan/UBSan + 边界 corpus。
5. **跨架构 CI**：linux/amd64、linux/arm64、darwin/arm64、windows/amd64 编译+测试；QEMU 兜其它架构标量。

## 12. 性能目标与验证

- 基准：复用 `BenchmarkMITMRealTraffic` / `BenchmarkEngineVsVectorscan` / `BenchmarkSyntheticScale`，
  对照 StdlibLoop / 纯 Go 引擎 / **bridge(vectorscan)** / **mvscan**。
- 目标：mvscan ≥ **100x**（相对 StdlibLoop）；标量退化档显著快于逐条。
- 每阶段退出标准（见 13）均带可量化性能 + 一致性门槛。

## 13. 分阶段计划与工时

> 因砍掉自写 C parser、前端复用 Go，**总估从初版 ~118 人日下调到 ~60~72 人日（单人熟练，±30%）**。
> 最大风险转移到：bit-NFA 热点调优、blob 契约稳定、regexp2 字面量路线（3.4）。

| 里程碑 | 内容 | 退出标准 | 估算 |
|---|---|---|---|
| **M1 前端 + Go 参考执行器** | syntax/regexp2→Unify AST→Glushkov→字面量/Teddy 分桶 + Go 内置 bit-NFA 参考执行器 | 与 stdlib oracle 差分全过；量化"纯算法 x" | ~16 人日 |
| **M2 C 标量后端 + blob** | blob 格式 + C 解析 + bit-NFA(标量) + Teddy(标量) + 编排 | 与 Go 参考执行器逐位一致；真实 MITM 一致性全过；标量已显著快 | ~18 人日 |
| **M3 SIMD 加速档** | Teddy/NFA 的 SSSE3/AVX2/NEON + 运行期分发 + 标量孪生一致 | 各档逐位一致；达 **100x** | ~18 人日 |
| **M4 平台收敛** | MSVC/MinGW + amalgamation + 跨架构 CI + ASan/fuzz | 四平台编译测试绿；单文件 amalgamation 可用 | ~12 人日 |
| **M5 接入与切换** | `BackendMVS` 接入 + 基准固化 + 文档；达标后**移除 bridge** | 默认/退化/加速全绿；验收通过 | ~8 人日 |

> M1 的 Go 参考执行器**不是一次性原型**：它既量化纯算法上限，又作为 C 后端的可执行规格 + 永久差分参照，
> 大幅降低 C 调试成本，是本路线相对初版"先扔原型再写 C"的实质优化。

## 14. 风险与缓解

| 风险 | 影响 | 缓解 |
|---|---|---|
| bit-NFA 大状态热点 | 达不到 100x | per-pattern 小 NFA 优先；Teddy 跳过；LimEx shift/SIMD；少数大正则交 regexp2 |
| regexp2 字面量获取（3.4） | 预过滤收益低 | 路线 A 精确 / 路线 B 近似；二者语义都正确（regexp2 兜底），先 B 后按需 A |
| blob 契约漂移 | 前后端不兼容 | magic+version；契约测试；M2 即冻结 |
| 跨平台/编译器差异 | Win 编译失败 | MSVC 适配提前到 M2 验证一次；纯 C99 + intrinsic 兼容分支 |
| 内存安全(C) | 崩溃=分发事故 | ASan/UBSan/fuzz；分配器钩子；零拷贝边界校验 |
| Glushkov 正确性 | 漏/误报 | Go 参考执行器作规格；全程 oracle 差分 + fuzz |

## 15. 文件结构（规划）

```
common/minirehs/
  unify_ast.go        Unify AST 定义 + syntax/regexp2 → UnifyNode 转换 + 分流
  glushkov.go         UnifyNode(regular) -> 小 bit-NFA (first/last/follow/reach)
  literal_unify.go    基于 UnifyNode 的统一字面量提取 (移植现有 literal.go)
  blob.go             编译产物 -> 平台无关只读 blob ([]byte) 序列化
  mvs_cgo.go          [tag minirehs_mvs] cgo 绑定 + BackendMVS (传 blob, 收命中)
  mvs_refexec.go      Go 参考执行器 (bit-NFA), 作 C 后端差分规格/oracle
  mvs_stub.go         [!minirehs_mvs] newMVSBackend()->nil, 退化为引擎
  mvs_test.go         一致性/退化/并发/早停 (复用现有框架)
  native/mvscan/
    mvscan.h          C API (mvs_db/mvs_open/mvs_scan/mvs_close + 分配器钩子)
    blob.c            blob 零拷贝解析
    nfa_exec.c        bit-parallel 执行 (标量) + LimEx-lite
    nfa_exec_x86.c    SSSE3/AVX2 kernel (target 属性)
    nfa_exec_neon.c   arm64 NEON kernel
    teddy.c           多字面量 SIMD 预过滤 (由现有 teddy.c 升级)
    dispatch.c        运行期 CPU 特性探测 + 函数指针选择
    mvscan.c          (amalgamation 产物, 由 tools 生成)
  tools/amalgamate/   拼 native/mvscan/*.c -> 单文件 mvscan.c/.h
  vectorscan_bridge.go / vectorscan_stub.go   ← 移植达标前保留, 作对照基线
```

## 16. 术语

- **Unify AST**：承载 `regexp/syntax` 与 `regexp2` 两来源的统一中间表示（前端 Go 内部）。
- **Glushkov NFA**：ε-free、入边同符号的位置自动机，位并行执行的基础。
- **LimEx**：Hyperscan 的 bit-NFA（limited 转移 + exception）；本方案取其"位并行"精华做精简版。
- **Teddy**：少量字面量的 SIMD 预过滤（nibble 指纹 + PSHUFB 查表）。
- **blob**：平台无关只读字节布局，前端 Go 产出、后端 C 零拷贝消费的解耦契约。
- **存在性匹配**：只判定规则是否命中（`From/To=-1`），不求精确位置。
- **兼容 + 退化**：SIMD 有则用、无则落标量；保证"能编译、结果对、仍快于逐条"。
```
