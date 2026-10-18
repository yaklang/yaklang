# minirehs 性能计划执行记录

所有后续工作继续在 [PR #5321](https://github.com/yaklang/yaklang/pull/5321) 上推进，目标分支为 main。
各任务使用独立提交和实验基线，不把组合收益当作单项严格 A/B。

## 当前范围

用户在 2026-10-18 明确要求先完成 Windows 内容，暂不寻找其他机器或审查入口。
当前目标为 Windows/amd64、CGO_ENABLED=1、现有 clang 和 VillanCh/go-pcre2-lite v0.1.6。
Linux/amd64、macOS/arm64 原生性能和独立审查保留待补；Windows 阶段完成不代表三平台 M5 验收。
不新增第三方依赖，不新增 workflow、job 或操作系统判断。

## 身份和材料核对（T00）

- 研究基线：8911cf7d8bd75d327919f3f4e41f217dbd5dd173，独立工作副本保留。
- 合并后的 main：4373b3010a96bab70647471b28c655e6322fc778。
- 本轮起点：27100ce303e0c10d22d00ccab401de6167435fe4，已 rebase；与原 7c940fa98 的代码树相同。
- 五项既有实现：共享 PCRE subject、连续 LimEx 派生表、完整 bracket 放宽、局部 gate、anchored 首字节 mask。
  它们已在本 PR 中，后续卡片复验已有实现，不重复记为新优化。
- 实际有序规则：GetRequest 4941 条；MITM 原始 89 条，失败项单独导出，不静默删除。
- 本地语料：1332 条、5,247,590 字节有效记录内容；容器文件另含每条长度字段。
- 生产存在性/定位/早停权重、生产 SLO 尚未提供，不构造所谓平均生产收益。
- 既有测量包含正确性与性能的组合修改，不能直接作为后续 correctness-only A1。
- T00 起点为 portable image v4、native blob v3；身份任务没有改变格式或算法。

本轮可选诊断入口按现有普通测试组织，不改变 CI：

```sh
CGO_ENABLED=1 MINIREHS_EVIDENCE_DIR=/local/evidence/T00 \
  go test ./common/minirehs -run '^TestProgramCorpusManifest$' -count=1 -v
```

该入口导出 Go 实际读取的原始有序 Pattern、完整编译报告，以及每条 MITM 规则的编译错误。
默认不写文件；本地证据包含原始语料身份，不自动上传到任何服务。

## 推进顺序

T01 建立路由和阶段归因；T02 补完整元组、重复、错误与早停差分；随后以 T03–T05 建立正确性基线。
T06–T16 的每项优化先验证反例，再进行单变量 A/B；没有收益的假设可以否定。
T17 汇总 Windows 系统验证，其他平台和独立审查仍明确列为待补证据。

## Windows 阶段检查点

- T02：192 组完整结果对；comparator 可识别漏 ID、错偏移、多重复、哨兵、吞错、完成和顺序差异。
  独立 oracle 直接使用完整 subject 的 RE2 / 现有 Yak wrapper，另验证有界 PCRE 资源错误。
- T03：修复测试用途 native record batch 的 256 成员截断。255/256/257/300、多记录归属、非法偏移、
  scratch 重用、实际 Go blob 的容量 0/1 和 native 分配失败已验证；普通包、amalgamation、专项 race 通过。
  没有将该接口接入生产 Scan，没有改变 image/blob 布局。
- T04：Engine 恢复每候选一次完整 subject 的全局非重叠选择；localized gate 从全部事件取得最小终点，
  不要求 Teddy 事件排序。另修复 RuneError 字面量对非法 UTF 的漏报。
  portable image 语义版本升为 v5，旧 v1–v4 原样加载/导出，非法 UTF 时保守完整确认；cache 自动重建一次。
  nullable RE2 fallback 原有的精确 span 例外保持不变。

这些是阶段证据；T04 完整回归、T05 错误贯通及后续性能实验仍按实际任务记录推进，尚未声称 M5 完成。

## 执行错误接口（T05 Windows 已验证）

`Database.Scan` / `ScanBatch` 的执行失败可用 `errors.As` 提取 `*ScanError`，包含规则、记录、
阶段、类别及 incomplete。原绑定未暴露的数字错误码保持 nil，原错误仍可用 `errors.Is/As` 检查。
失败前的回调不撤销；Batch 的错误按原 record 顺序重放，之前的 handler=false 不观察后错。

Group 的可靠入口为 `MatchErr`、`MatchStringErr`、`MatchBytesErr`、`FindErr`、`MatchedIndexesErr`、
`MatchedPatternsErr`、`CountErr`、`ScanErr`。集合/计数入口可在 error 同时返回已观察前缀，不能当作完整结果。
旧无 error 签名保留成功行为，执行失败时 panic(ScanError)，不再伪装 NoMatch；`MatchAny` 返回错误。

```go
indexes, err := group.MatchedIndexesErr(data)
if err != nil {
    // indexes 只包含失败前已观察到的规则；按调用方的错误路径处理。
    return err
}
_ = indexes
```

Close 前由调用方收拢扫描，不在 handler 内关闭同一 DB；关闭后 Scan、ScanBatch、NewScratch 返回 ErrClosed。
PCRE2、lazy verifier、native 分配和 dispatcher 错误均已贯通。C 的 -1/-2/-3 分别保留为
invalid dispatch、overflow、out of memory；已启动 worker 在所有返回路径收拢，再按回调顺序报告。
多条 native 结果槽使用有符号状态编码，负值先作为错误处理，不能用非零直接当作命中。
trafficguard 改用 MatchedIndexesErr；候选过滤失败时对全部原规则执行其权威提取器，避免把部分候选当完整结果。

当前生产配置保留 VillanCh/go-pcre2-lite v0.1.6 的默认 match/depth 资源策略，没有新设规则或输入限制。
已有表达式内的 PCRE2 LIMIT 指令随表达式保存在镜像中；Compile/Load 都验证同样的执行错误。
专项测试使用现有公开 SetMatchLimits，在对象被扫描线程使用之前配置；该操作会重编译并替换后端，
不允许对正在扫描的实例动态调用。MatchTimeout 或在外层 select 返回不能取消正在执行的 C 匹配；
本实现不声称支持这种取消，也不访问 PCRE2 私有句柄或新增范围接口。

独立 C 故障探针使用真实 Go blob，覆盖 16 条 native 路径的 38 次 malloc/calloc 失败及临时缓冲释放；
普通 Go 回归另验证状态桥接、Compile/Load、GOMAXPROCS=1/2、早停、Batch 的 record 归属与错误后复用。
这两类证据分开记录，没有把 Go 状态注入冒充 C 内存分配故障。
池高水位和 Group 常驻 worker 所有权的 T06 结论见下文，T15/T17 继续系统复验；尚未声称完整 M5 验收。

## T06 Windows 资源与分配检查点

成功 native Scan 先返回 nil，避免每条记录分配 8 B 的 errors.As 目标；五轮交错 A/B 中，
Sparse/Zero 从 8 B/1 alloc 降为 0，HTTP 从 1032 B/2 alloc 降为 1024 B/1 alloc。
耗时差异较小且有噪声，不将这一改动宣传成明显吞吐提升。

Group 的 sync.Pool 曾留下无法枚举关闭的永久 worker。现改为可排空的有界 idle cache，
最多保留 min(GOMAXPROCS,4) 个 Scratch；并发扫描不受此 idle 数量限制。
Close 拒绝新借用，等待已借出的 Scratch 完成归还/清理，关闭 idle worker 后关闭 DB；
同一 Group 的 handler 内不能调用 Close。GC finalizer 作为漏掉显式 Close 时的兜底。
直接 Database.Close 仍要求调用方先收拢扫描。

每个缓存根按实际 capacity 限制 4 MiB 的 data-sized Go buffer；包括历史锚定区间和内部 worker，
规则规模元数据、不可变自动机及 PCRE2 自身缓存不在这个口径内，因此不是整个进程的内存上限。
预算超出后仍完整处理本条输入，再关闭 Scratch 而不缓存；没有输入截断或规则过滤。
真实 600 KB 密集输入的末尾独立标记也已验证。

生命周期修复并非纯加速：五轮 Group warm A/B，SmallLiteral 519.7→576.6 ns，
AnchoredPCRE 3.122→3.421 µs，GetRequest 131.35→138.31 µs；分配数相同。
保留实际负例和桌面测量噪声，后续系统矩阵继续核对，不恢复已知 worker 泄漏。

原有安全 subject 共享单独复验，没有重复实现：16 KiB 合成输入、1/8/64 个 PCRE 候选，
1 个候选无明显收益；8 个时 Go 分配 147584→18560 B，64 个时 1180672→19456 B。
这是前端 bytes→string 复制的资格测量，不能当作真实业务的 64 倍提速。
输入覆盖、并发、早停、错误后重用、GC、Close 与真实 burst 回归及 race 均通过。
R 级 generation 清零未启用：当前 profile 的大头是 C 扫描，先优化已定位的消费路径。

## T08 Windows 实验决议：保留原 lean 单字递推

实验只改变无断言、单 word 的 C 存在性递推：以 shift 推进连续链，仅枚举活跃异常源，
复用原 follow 和两个已有标量字段，不增加逐位置表。实验补丁未合入默认生产代码。
1391 个 GetRequest、62 个 MITM 真实 frontend 表通过 2,975,744 次 native/native/Go 状态步对照；
另有 P=1/63/64、自环、密集异常、锚点、Unicode/NUL/非法 UTF 的输入逐步对照。
完整 1332 条 MITM 语料及四类输入、两种报告模式、stop=0/1/2，共 8040 组有序完整结果无差异。

Windows i5-13490F、Go 1.22.12、clang 22.1.8、GOMAXPROCS=2，两个相同源代码二进制仅以
C 编译宏选择递推，五轮串行交错测量（桌面频率/负载未固定）：

| 场景 | 原递推中位数 | LimEx 中位数 | 耗时变化 |
| --- | ---: | ---: | ---: |
| GetRequest HTTP912 存在性 | 132.12 µs | 130.27 µs | -1.40% |
| GetRequest DenseHTTP 存在性 | 2.951 ms | 3.014 ms | +2.13% |
| MITM 全 1332 记录存在性 | 301.95 ms | 294.81 ms | -2.36% |
| MITM 全 1332 记录定位 | 852.28 ms | 834.90 ms | -2.04% |
| 合成 64 位置长链、16 KiB | 404.74 µs | 19.30 µs | -95.23% |

真实场景没有达到预注册的 5% 收益门槛，且 GetRequest 定位 Zero16K 中位数回退 6.71%。
合成长链约 21 倍的收益不能替代真实负载证明，故撤回实验实现，保持原递推；不增加默认分支/元数据。
实际 Go blob 的 native requested 字节和分配次数完全相同，组件 Close 均归零；这不是进程 RSS 结论。
Load 两变体 83.30→84.46 ms，其中派生步骤在两变体共同存在，不能据此单独归因派生开销。
没有 PMU、请求 P99 或生产权重；其他平台和独立审查仍按用户要求暂缓。
完整补丁、源码哈希、二进制身份、结果与原始 230 行测量保存在本地计划 evidence/T08，供后续不同负载复验。

## T09 Windows 条件表空间修补

单 word 断言的条件 source mask 必须等于有效 condFollow 位置的并集；native 和文件加载器均拒绝
负 count、越界位置或 mask 不一致。count=0 且 mask=0 时不分配 condFollowEval；first/accept 表保留。
三处单 word reader 原本只在 prev & mask 非零时读取 follow 表，热循环无需增加判断；free(NULL) 保持对称。

实际 Go blob 的 C requested 当前/峰值/累计请求一致减少：GetRequest 25,600 B/一次分配，
MITM 47,616 B/三次分配。有条件后继的表保留，不将这一修补宣称为 30 MB 级缩减。
P=1/16/63/64/65/128、bit63 条件边、全部 64 种 truth code、非法字段、Compile/Load、
定位/存在性/early-stop、多 reader 并发，以及所有相关 native reader 已验证。
八个实际 frontend 小产物的全部构造分配点故障都明确失败并释放组件请求，不能据此声称整个进程 RSS 归零。

五轮串行 A/B 的 16 个 warm 运行场景最大耗时回退 2.25%，不声称吞吐有明显改善。
单次 Load 中位数 +3.14%，一次预定的 5x/轮复核变为 -0.71%；Load+首扫分别 +0.13% 和 -2.57%。
两组原始数据都保留，桌面计时波动限制了对小变化的解释；主要收益是确定少分配、少持有全零表。
64 组实际/独立参考及有序 Compile/Load 完整结果对无差异，普通全包、trafficguard、专项 race、
amalgamation 与 vet 通过。镜像 v5/blob v3 不变，合法旧镜像仍可原样加载/导出；无新依赖或 CI 改动。

## T10 第一阶段：DFA 初态与持久计划正确性

已在实际 Go producer 和 native Compile/Load 中复现 `[ab][cd]` 错配单字符 `c`、
`[ab]{2}` 错配单字符 `b`；默认 merged Scan 的 NFA 是独立的正确消费路径。
新 DFA 以空的已消费位置集开始，每步先注入 first 再消费；builder 明确拒绝断言、首/尾文本锚和多 word。

native blob4 标识修正后的 DFA 初态；blob3 的 DFA 通过已有字段标为旧表，单条/scalar/many/DFA-batch
都安全回退原 NFA，不新增结构体字段或重解析旧规则。image6 更新磁盘缓存身份，旧 image1–5 仍可
加载并逐字节导出，旧 cache 只刷新一次。MVP1 必须恢复 blob3，MVP2 恢复 blob4，外部/内部版本一致性均验证。
embedded 例子重新生成后为 884 B；这项正确性修复不计为性能优化。

普通 Windows 全包/trafficguard、专项 race5、native/amalgamation 与 vet 均通过。
已保存旧 image5/blob3 的实际 producer 反例，普通测试强制全部相关消费者、非法 UTF、空输入、
63/64 位边界、dense/packed 再序列化、旧 cache 刷新与无编译 Load。

## T10 第二阶段：消费者清单与构造辅助存储

实际 Compile 产物记录了 native、Go reverse/location fallback、verifier、debug 和 DFA 消费资格。
GetRequest 没有 optional DFA；MITM 规则 73/77 共 61 状态、62,525 B。
两种报告模式下，四类输入及全部 1332 MITM 记录的普通 Scan 没有调用这两张 DFA；
但 standalone native、显式 DFA-batch 和强制验证确有消费者，故保留表及其正确 NFA 回退，
不引入复杂消费者框架，也不以默认路径未调用作为删表证据。原 per-pattern NFA 的 Go 反向、定位和 fallback 仍保留。

只删除构造时未读取的 stateKeys 列表、重复的 BFS queue，以及返回对象中固定未使用的 nsym/isByte。
stateSets 按 discovery ID 遍历维持原 BFS 顺序；状态预算拒绝路径、状态编号、转移内容和输出均保持。
相对于已修正的 63a33cd95，五轮独立串行 A/B：规则 73 构造约 345.7→338.5 µs、少 20 alloc/约 2.8 KB；
规则 77 约 244.9→237.1 µs、少 17 alloc/约 1.3 KB。全部 DFA 表保留，因此镜像大小没有相应缩减。
完整 MITM Compile 13.56→13.71 ms（+1.10%），不声称整体编译提速；Load 初测 +3.37%，
一次独立 Load-only 100x 复核 +0.44%，两组保留。没有 PMU/P99/生产权重。
两种真实规则集、两种报告模式的四份镜像逐字节相同；64 完整结果对无差异。
构造变更后的普通 DFA/state-budget、race、amalgamation、vet 也已验证；未删除被强制路径需要的表。

## T11 第一批：编译期 exact AST 参数复用

仅显式 MVS Compile 暂存第一次 RE2 分析已经生成的 simplified AST，按 compiledPattern 实例隔离，
传给 NFA 和 literal-window 构造；处理每条规则时移除映射。没有全局缓存、可执行 regexp 共享、
发布后 AST 字段或宽度/位置算法变更。Auto 仍是 Engine，不保留这些树；标准库内部编译解析仍在。

相对于 c9392a5af，用同二进制 Reference/Reuse 路径、五轮串行独立进程（Go1.22.12/clang22.1.8/GOMAXPROCS2）：

| 指标 | Reference | Reuse |
| --- | ---: | ---: |
| GetRequest Compile 中位数 | 445.76 ms | 396.49 ms（-11.05%） |
| GetRequest 累计 Go 分配 | 503.93 MB / 5,550,747 alloc | 475.18 MB / 5,251,182 alloc |
| MITM Compile 中位数 | 14.35 ms | 13.30 ms（-7.33%） |
| GetRequest 显式包内 Parse 次数 | 14,828 | 6,152 |
| MITM 显式包内 Parse 次数 | 322 | 161 |

NFA 尝试次数仍为 4941/87；这是参数复用的增量，不能用旧 T01 的不同测量时点拼接更大百分比。
单次带 hook/10ms HeapAlloc 采样的诊断中，GetRequest 峰值约 224→232/238 MB，GC 后约152.5 MB持有量相同；
另三轮独立 fresh-process 资源测量的 Private Bytes 采样峰值中位数为 441.6→370.5 MB，Working Set 为439.8→371.6 MB。
这些是不同口径且受 GC/桌面负载影响，采样峰值不是真实精确峰值，不能把 Go/C 请求与 RSS 相加。
主要结论是解析/累计分配确定减少，未据一次堆采样推断进程内存回退，也不声称生产 P99 或加权收益。

四份真实镜像逐字节相同，48 组跨变体有序完整结果无差异；普通测试另对独立 whole-subject oracle，
覆盖重复 ID/flags、exact/PCRE、nullable/assert/Unicode/非法 UTF、counted-repeat 四个独立位置、AST 只读、
错误顺序与并发 Compile。默认启用后 full minirehs39.021s/embedded/trafficguard、race5、amalgamation 和 vet 通过。
镜像仍为 image6/blob4；下一批的测量与范围决议见下。

## T11 第二批：局部 concat 宽度事实

启用第一批后的独立编译 profile 中，剩余非首次显式解析最多占采样 CPU 的 2.86%。
按任务卡的 5% 门槛，本轮拒绝扩大成 raw/superset IR、全局 DAG 或无消费者的 nullable/位置缓存。
保留原始表达式、flags、exact/superset 和 verifier 的隔离；不共享消费位置或可执行 regexp。

window/biCover 对宽 concat 原本为每个子项重算左右兄弟宽度，现以局部 prefix/suffix 计算，
将每个 concat 的兄弟求和从 O(k²) 降至 O(k)。head 使用同顺序滚动前缀。
短 buffer 放栈上，大 concat 使用一次临时连续 int backing；视图容量严格封口。
若宽度为负或所有有界宽度之和可能溢出，则回原求和，保持原 addSat 的行为与错误时机。
递归子树宽度及最终 NFA O((P+S)W) 仍存在，不声称整个编译为线性。

相对于第一批，五轮串行同二进制 Reference/Facts 测量：GetRequest Compile 392.20→383.68 ms（-2.17%），
MITM 13.06→13.12 ms（+0.45%）。512 段 `[ab]token`（3072 消费位置）的单独窗口分析
6.01 ms→38.44 µs，整条 Compile 16.51→4.45 ms；合成分析收益不能替代实际规则收益。
该大分析多 18,432 B/一次临时分配，小例子分配不变；GetRequest 累计分配增加约 147 KB（0.03%）。

四份实际镜像逐字节一致，48 组有序完整输出无差异。普通测试另用独立 whole-subject oracle，
覆盖长 concat 正匹配、counted repeat、Unicode/FoldCase、非法 UTF、断言、无界宽度、溢出退路与别名边界。
默认启用后 full minirehs39.134s/embedded/trafficguard、race5、amalgamation、vet 均通过。
没有镜像/缓存格式变化；T11 的 Windows 范围以这两批局部优化和明确拒绝大框架的决议完成。
Linux/macOS 与独立审查仍按用户要求暂缓。

## T12：剩余 merged 派生表与 blob 临时复制

T00 已有的 per-pattern initScalar 连续布局和 imageRows 基础 backing 保留，不重复认领收益。
本次只补仍逐位置 clone 的 merged buildLimEx：先只读数出异常行，再一次分配保留行，精确容量视图防止 append 污染邻行。
实际 GetRequest merged 仅 7 位置且无异常行，MITM 为 56 位置、34 异常行，故这部分实际空间收益很小。
512 位置合成纯链的派生从 516→4 alloc、18.72→7.03 µs；稠密分支从516→5 alloc、18.92→13.89 µs。

原 blob builder 同时保留每个 unit 的编码缓冲再复制进最终 blob，且扩展字段会触发扩容。
现在检查长度算术，精确测量后直接写最终缓冲；独立 encodeUnit 同样按全部可选字段精确分配。
u32 offset 的表示溢出明确失败，不能静默截断；没有新增规则数或输入长度限制。
仍保留 flattened Go units、Go NFA 与 C 运行表，本批不改变消费者、wire 或持有量。

相对于7fcd31ee8，五轮串行 fresh-process A/B：GetRequest blob 构造29.21→22.56 ms（-22.76%），
累计104.23→72.12 MB；完整 Compile387.46→374.54 ms（-3.33%），累计475.34→443.22 MB。
MITM blob3.50→3.45 ms，完整 Compile12.44→12.64 ms（+1.63%）；不隐藏小幅负例。
这是本卡两项改动组合的整体结果，组件加速不相乘。
原生 dense blob 仍为30,165,009 B（GetRequest）、255,778 B（MITM），本批减少的是临时复制而非文件或 C 运行表尺寸。

保留冻结父版 encoder 做完整字节差分；逐边重建、零行/纯链/稠密、63/64/65和127/128/129边界、
全部扩展组合、算术溢出、容量别名、独立 whole-subject oracle、Compile/Load/early-stop 均通过。
四份实际镜像逐字节相同，48 组跨变体完整有序结果相同。
full minirehs40.866s/embedded/trafficguard、race5、amalgamation、vet 通过；无新依赖/CI/镜像格式变化。
其余平台和独立审查依用户授权暂缓。

## T13：C 基础数组 arena 试点

保留所有已证明有消费者的 Go NFA、reverse、fallback 和 optional DFA。选择固定基础数组作小范围试点：
每个 unit 的四个位向量、follow、reach、cuts、ASCII 映射和 posPat 由九次 malloc 改为一个 C 自有块。
先检查 count×stride、累加、unit 边界和结构尺寸，再按 u64/i32 顺序布局；显式小端解码，不借用 Go 指针。
断言、LimEx 和 DFA 扩展仍分别拥有内存，失败/Close 只释放基础块一次；没有 wire、semantic 或 cache 身份变化。

实际 GetRequest 的 C requested 分配次数44,489→4,953，字节32,893,977→32,933,513；
多39,536 B来自每个 unit 的所有权指针，不能宣称 C requested 空间下降。MITM 分配828→132。
这些是组件请求统计，不包括分配器开销、PCRE/Teddy，也不是进程 RSS。
十个实际 frontend 小产物的全部277个构造失败点及两个非法尺寸产物验证失败清理归零；
未逐点穷举大型 GetRequest 的全部构造分配。

相对于21c8f410a，五轮串行独立进程500ms测量，GetRequest native open/close16.83→14.64 ms（-12.98%），
MITM57.56→29.73 µs（-48.35%）。完整 Load 初测GetRequest73.25→75.66 ms（+3.29%），
按预注册规则仅做一次独立Load-only1s复核，为84.93→79.94 ms（-5.88%）；两组都保留。
对应首个HTTP912扫描初测+1.78%、复核-2.89%；MITM完整Load初测-1.52%、复核-3.20%，
其首扫复核+1.03%。桌面计时/进程状态波动限制了整体收益结论，主要确定收益是更少分配及原生构造改善。

分项调用不是可相加的 CPU 分区。另一次独立 Load profile 中 packed展开30.28%、Go表恢复28.57%、
native open9.38%（含于cgocall，非原生栈）仍显示进一步减少重复恢复的空间；重叠子项不相加。
本卡按约定止于一个稳定表示的 arena 试点；直接 packed→C decoder 是另有门禁的追加工作，尚未实施，
现有 Go 表的消费证明也不支持大范围删除。不能以首扫后移或未物化字段不验证来声称加速。

四份实际镜像逐字节相同，48组完整有序结果无差异。普通 full minirehs40.556s/embedded/trafficguard、
专项race5、amalgamation和vet通过；额外新旧DB并发读取、输入覆盖、关闭旧owner后继续读新owner及lazy回归通过。
最终只给扫描外的只读资格助手补KeepAlive，并加入reload测试；不改变测量的Load/Scan路径。
其他平台和独立审查仍按用户要求暂缓，未宣称三平台或生产验收。

## T14：集中兼容矩阵与完整缓存身份

发现并复现缓存只比较64位指纹的缺口：让两个完整、CRC正确的压缩镜像具有同一指纹后，
请求可能得到另一套表达式或ID；flags/顺序差异也会被当作命中。这里是人为构造相同指纹，未声称找到自然FNV碰撞。
现在指纹只作快速拒绝，解码规则元数据后逐项比较顺序、ID、完整flags和原始表达式，
再展开大型表/打开C内核。局部复用原flag前缀，避免每规则拼接完整字符串；不解析/编译正则。
不匹配按原缓存失效路径重新编译一次，随后正常命中；显式LoadImage仍按传入镜像加载，无隐式换规则。

修复改变缓存身份验证，不改变有效持久计划或字节布局，保留image6/blob4/MVP2。
T03的内部batch接口、T11局部分析复用、T12临时布局和T13私有arena均不额外失效合法缓存；
T04/T10的持久正确性分别由image5、image6/blob4处理，旧错误DFA使用原NFA，旧非法UTF计划有安全完整subject退路。
未知version/mandatory feature明确不兼容，损坏/计数溢出明确错误；LoadOrCompile只对可重建缓存走重编译。

15个实际格式/模式案例、8类输入、全输出及两种early-stop共360组对照全部通过；
全输出对独立oracle，early-stop保持该DB完整输出的有序前缀。包含image1–5原始fixture，
及image6 dense/packed×压缩/未压缩×存在性/定位；逐个核验实际格式、无编译Load及输入/导出隔离。
四类相同指纹错配、八种flags与内联flags合法命中、未知版本/功能/计数的一次刷新、stable/candidate独立路径回滚也通过。
实际A0(image1)旧二进制读新image6明确ErrIncompatible，新reader读旧/新镜像完整结果相同。
旧二进制仅作兼容证据；回滚使用保留正确性修复的reader和先前验证过的正确计划，不能恢复已知旧错误。

五轮独立进程合法缓存成本测量：GetRequest CacheHit77.22→75.49ms、未改变的LoadImage控制79.65→77.60ms；
MITM分别2.784→2.692ms、2.551→2.580ms。未分辨明显的额外耗时，不将波动记作提速。
GetRequest中位Go分配次数约多7次，累计字节变化不足1KB；不增加每规则完整prefixed字符串分配。
普通全包minirehs38.674s/embedded.201s/trafficguard.441s，专项race3、amalgamation与vet通过。
首次全包命令误开启研究证据模式，两个入口因缺参数拒绝；该失败保留，随后清环境的普通全包和单独矩阵均通过。
无新依赖/CI工作流变更，其他平台及独立审查仍按用户要求暂缓。

## T07：范围批处理资格门槛

先复验生产Scan结束后实际scratch中的batch任务，未新增执行接口或生产hook。
完整1332记录加4个固定输入、两规则集和两模式共5344条扫描记录，保存完整回调和原始idx/lo/hi/W。
GetRequest存在性阶段共有8619个任务，全部带文本锚，因此卡片首版no-assert/no-text-anchor/W≤8资格覆盖为零。
MITM只有341个合格任务，分布于285/1336记录、每记录最多4个；连续同窗口32任务/64KiB块最多减少50次边界调用。
定位模式不列入首版资格；未把前面的windowable、anchor、bi-anchor或PCRE任务混入范围batch覆盖。

五轮独立进程空执行测量，单条native包装+dispatch约63.84ns；32条重复调用约2.029µs、已有many约162.5ns。
这是空函数任务微基准，既不是纯Cgo延迟，也不是业务提速。MITM少量可合并调用不足以支持新增通用range ABI，
GetRequest首版资格又为空，故在S0覆盖门槛拒绝默认激活，不开发闲置原型，不新增错误/资源路径。
原逐任务执行保持；T15可在range batching关闭下推进。普查及专项race通过，完整记录和40行微测量留存。

普查中GetRequest这些任务有6786个为单word（其余W=2–5）。核对T08原始补丁后确认其已经保留
first注入及anchor/accept并覆盖单word文本锚，故这些统计不构成新的资格变化，不重复原负实验。
不将本卡未实现的range接口当成已有能力，后续按真实任务分布推进调度预算。

上一轮性能结论及边界见 [RUNTIME_PERFORMANCE.md](RUNTIME_PERFORMANCE.md)。
