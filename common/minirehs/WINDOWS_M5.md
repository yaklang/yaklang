# minirehs Windows M5 验收与性能评估

2026-10-18，PR [#5321](https://github.com/yaklang/yaklang/pull/5321)，目标 main。
按用户最新范围完成 T00–T17 的 Windows/amd64 阶段。Linux/amd64、macOS/arm64 原生性能和独立审查由用户暂缓；三平台 M5、生产回放和生产上线仍待后续验证。

## 结果和身份

保留已验证的编译分析复用、线性宽度分析、临时存储精简、C arena、短任务调度及单成员存在性完成策略。
完整 MITM 在默认16逻辑核 Q1/Q16/Q32 的耗时分别改善约8.38%/6.86%/10.63%；收益按场景列出，不相乘，不构造生产权重。
GetRequest 4941条编译约339.87ms，默认镜像加载69.87ms，文件1,454,565B。
运行时内存仍较大：单库构造后进程 Private 约233MiB，旧新双库及各自Scratch约367MiB。

运行源码来自 `0d68094624745d074fe1ac5ca96e5f3da7682456`；本报告所在的T17提交只增加验收测试、测量标记和文档。
main核对为 `4373b3010a96bab70647471b28c655e6322fc778`。T01的eb74d7d0b是历史组合基线，不能当作纯正确性A1。
本次运行策略A/B使用同一个冻结二进制：Reference在读者启动前关闭T15/T16两个资格，Integrated开启两者。
因此能归因这两个运行策略的组合，不能将整轮T00–T17归为单变量或统一倍率。
冷构造另外对比实际T15父二进制 `14b80dbdf4b4998287836a881de18c650831f1c2`。

机器：Intel i5-13490F，10物理核/16逻辑处理器；Windows/amd64，Go1.22.12，clang22.1.8，CGO_ENABLED=1，GOMAXPROCS=16。
本次默认全16LP、未改进程亲和性；物理频率、后台应用和SMT活动未锁定。GOMAXPROCS不限制C线程总数，没有声称OS配额或C线程硬上限。
测量均为串行执行的五轮独立进程；对照交替顺序。编译、测试、profile与结果计时分开。
表中中位数是各进程测量结果的中位数；全部配对变化及原始轮次保留，未使用显著性或生产SLO保证。

冻结二进制SHA256：`bdb2e2691d1ac565657a5fc069ad0276a6b66d8c330922c722aef1fd50105738`。源文件逐项哈希见 `T17-frozen/manifest.json`。

## 完整语义验收

使用原始有序GetRequest4941条、MITM89原始条目中87条可编译规则；ID33/43的编译失败显式保留，不隐去错误。
1332条原始记录共5,247,590有效输入字节；容器SHA256为
`b51943a8378353dbd15d0d0d363d7a48d25b8a8a4775ba0d5b98aa8dced42ef4`。
严格读完整语料，缺失/截断/尾部残留均失败，不接受普通辅助入口的first80或skip。

追加固定HTTP/稀疏/密集/零输入、空/32B/128B、Unicode/NUL/非法UTF、64KiB首/中/尾稀疏输入，共1344个输入。
两规则集、存在性/定位、Compile/Load、复用Scratch及第一次/中间回调早停全部验证。
原始表达式oracle只复用规则和每规则既有exact-span fallback策略：stdlib RE2 Longest或现有Yak/pcre2-lite；不复用minirehs的NFA、预过滤器或verifier。
新缓存oracle与旧逐调用oracle另有普通单元测试。

两个策略各保存5376行完整输出和6951行早停输出、各36981次完整比较；两策略JSONL逐字节一致，另有24654次跨策略Compile/Load有序比较。无结果、顺序、重复次数、错误或完成状态差异，原输入哈希不变。
额外48组完整/早停比较覆盖1MiB密集输出（定位65536个结果）、1MiB首部singleton+尾部assert、PCRE上下文、129位置及nullable。
格式矩阵15案例/360组覆盖旧image1–5、image6 dense/packed×压缩/不压缩×两种模式，包含lazy确认、别名隔离、未知功能/版本、损坏计数、等指纹不同完整规则、缓存一次刷新及回滚隔离。
实际旧v1 reader明确拒绝新v6，当前reader读取旧新镜像；旧blob3 DFA安全回退原NFA，旧非法UTF计划使用完整subject安全路径。

普通全包minirehs/embedded/trafficguard、三轮专项race、amalg路径及vet通过；最终提交的普通回归与CI状态另记录到交接证据。
300匹配成员、64/65与128/129边界、全局非重叠选择、最小trigger端点、native/PCRE失败、早停屏蔽后续错误、Close/reload/旧新owner及scalar对照沿用已修复的普通测试。
完整1332语料的本次大矩阵运行在native Windows；没有把amalg专项测试称为另一平台的完整性能验收。

## 默认16LP运行策略对照

Q为闭环客户端数量，没有额外应用排队；两阶段分开：第一阶段不逐请求计时，测吞吐、进程CPU（含C）和Go分配；第二阶段用QPC测服务P50/P99。
Go1.22 Windows共享时钟曾把短请求量化为0，故不用于这些分位数。QPC频率10MHz，时钟调用对开销逐轮记录、未扣除；其分配不混入第一阶段CPU。
GetProcessTimes的API单位是100ns，但实际CPU记账更粗；特别是小CPU差值须结合全部轮次看待。

| 完整记录场景 | Q | 耗时变化 | CPU/输入B变化 | 服务P50变化 | 服务P99变化 |
| --- | ---: | ---: | ---: | ---: | ---: |
| MITMCorpus | 1 | -8.38% | +1.96% | -2.81% | -8.18% |
| MITMCorpus | 16 | -6.86% | -3.15% | +0.73% | -4.39% |
| MITMCorpus | 32 | -10.63% | -5.06% | -1.34% | -7.89% |
| MITMShortCorpus | 1 | -18.86% | -33.33% | -24.63% | -18.84% |
| MITMShortCorpus | 16 | -2.70% | -2.95% | -6.67% | -38.33% |
| GetRequestShortCorpus | 1 | -11.59% | -25.00% | -15.62% | +1.05% |
| GetRequestShortCorpus | 16 | -5.81% | -1.27% | -5.43% | -0.90% |
| GetRequestHTTP | 1 | -0.91% | -7.89% | +0.08% | +3.05% |
| GetRequestHTTP | 16 | +0.31% | -0.67% | -0.05% | -10.09% |

完整MITM使用全部1332记录；ShortCorpus是全部193条原始<512B记录，未裁切单条输入，占54567/5247590输入B。
完整MITM单路CPU+1.96%是保留的成本，不能宣称每个场景都节省CPU。HTTP单路P99+3.05%也保留；这里的P99包含Go/OS调度，不是生产排队延迟。
受影响完整MITM场景耗时均改善至少5%，目标服务P99没有超过5%预算；不受影响HTTP墙时最大+0.31%。短MITM高并发耗时仅改善2.70%，不单列为达到5%的成功场景。
193条短记录和小PCRE的局部收益不能代表完整生产分布。

| 场景 | Q | Integrated req/s | MB/s（十进制） | CPU ns/B | 服务P50 µs | 服务P99 µs |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| MITMCorpus | 1 | 4875.5 | 19.21 | 77.42 | 55.40 | 4448.80 |
| MITMCorpus | 16 | 29159.2 | 114.88 | 94.44 | 82.70 | 5832.50 |
| MITMCorpus | 32 | 35367.0 | 139.33 | 100.40 | 117.90 | 8974.70 |
| MITMShortCorpus | 1 | 77804.7 | 22.00 | 44.74 | 10.10 | 29.30 |
| MITMShortCorpus | 16 | 702243.5 | 198.55 | 73.68 | 16.80 | 50.20 |
| GetRequestShortCorpus | 1 | 66813.4 | 18.89 | 53.69 | 10.80 | 57.80 |
| GetRequestShortCorpus | 16 | 610144.0 | 172.51 | 87.25 | 17.40 | 88.00 |
| GetRequestHTTP | 1 | 7467.6 | 6.81 | 146.40 | 128.30 | 236.80 |
| GetRequestHTTP | 16 | 67580.8 | 61.63 | 231.88 | 210.80 | 422.40 |

## 公共API控制与冷构造代价

同一冻结二进制的公共Group对照（两资格同时关/开）：

| Group场景 | Reference µs | Integrated µs | 中位数变化 | 配对变化中位数 |
| --- | ---: | ---: | ---: | ---: |
| AnchoredPCRE | 3.848 | 1.129 | -70.66% | -70.30% |
| GetRequest | 140.790 | 136.000 | -3.40% | +0.18% |
| SmallLiteral | 0.557 | 0.560 | +0.54% | +1.17% |

Group小literal+0.54%，分配不变；GetRequest两种聚合方式分别-3.40%与+0.18%，视为波动，不记作确定改善。小PCRE约70.66%改善仍是特定短规则集。

新DB构造/首次HTTP912扫描使用实际父二进制14b80dbdf与Integrated对照，五轮P16：

| 场景 | Parent ms | Integrated ms | 变化 |
| --- | ---: | ---: | ---: |
| GetRequest/LoadAndFirstScan | 72.282 | 73.647 | +1.89% |
| GetRequest/LoadImage | 76.594 | 75.355 | -1.62% |
| MITM/LoadAndFirstScan | 2.582 | 2.560 | -0.86% |
| MITM/LoadImage | 2.483 | 2.566 | +3.34% |

MITM Load自身+3.34%、GetRequest Load+首扫+1.89%保留在代价表内，属于一次构造的5%调查预算。
T16原P4冷构造观测MITM Load+首扫2.415→2.530ms（+4.73%，约114µs）同样保留；本次不同P/时段的-0.86%不用于抹去原成本。
此处“冷”指每次新建DB和首次Scan；DLL、分配器与文件页缓存可能已热，不是物理冷盘或完整进程启动P99。

## 编译、镜像与磁盘缓存

以下为另一组绝对测量的五轮中位数，不能与上表跨时段相除声称单项加速。

| 操作（存在性模式） | GetRequest4941 ms | MITM87 ms |
| --- | ---: | ---: |
| Compile | 339.8672 | 11.5152 |
| LoadImage | 69.8713 | 2.6220 |
| LoadAndFirstScan | 70.6984 | 2.6791 |
| CacheHit | 69.5660 | 2.5455 |
| MarshalImage | 80.4591 | 1.6764 |
| ReadImageFile | 0.2148 | 0.0284 |
| WriteImageFile | 1.7130 | 0.9997 |
| LoadPackedImage | 63.1689 | 2.2353 |
| LoadDenseImage | 41.9939 | 1.8643 |
| CacheMiss（完整重建、序列化、同步替换） | 431.3630 | 14.9442 |

GetRequest镜像加载约比Compile快4.86倍，MITM约4.39倍；它们是本机该规则集的操作比值，不是整个程序的启动保证。
GetRequest Compile累计Go分配约443.22MB/524.66万次，Load约123.65MB/11.78万次；累计分配不是保留内存。
缓存一次命中相对重新编译节省约270ms；一次完整缓存miss比直接Compile多约91ms。适合在规则稳定时持久缓存并复用已打开的DB。
不要在每条请求上反复LoadOrCompile/Marshal：运行热路径应复用DB及正确拥有的Scratch。

ReadImageFile是热页缓存读。WriteImageFile包含现有临时文件、Sync及原子替换，未模拟断电或物理冷SSD/HDD。
已生成镜像的同步写盘约1.71ms；序列化约80.46ms，写入者应把编码与存储同步分开评估。命中时主要成本是解码、Go恢复和C构造。
无新压缩库或RPC序列化依赖，使用项目已有zstd；先结构精简再压缩。

| 尺寸（实际字节） | GetRequest4941 | MITM87 |
| --- | ---: | ---: |
| 默认packed+zstd镜像 | 1,454,565 | 26,402 |
| packed未压缩镜像 | 4,177,702 | 60,323 |
| 稠密未压缩镜像 | 32,082,562 | 306,930 |
| 内存中的稠密native blob | 30,165,009 | 255,778 |

GetRequest先从32.08MB结构整理到4.18MB（约减少86.98%），再压缩到1.45MB/1.387MiB（相对稠密文件约减少95.47%）。
使用共享字符串/重复行、稀疏位集、变长整数及可恢复ASCII映射；文件尺寸问题已处理到这一量级。
稠密native blob仍为30.17MB，运行表及Go对象还要另外占内存。T12减少临时复制，T13减少分配次数，都不等于把这30MB运行表示消掉。
当前image6/blob4/MVP2；六规则嵌入示例为884B，SHA256为
`b7a4549d3399f76d1a51008a40dc00d190afe7ff80ce11a43fa956382fd7e327`。

## 保留内存、reload和关闭

每规则集三个新进程、同一冻结二进制的独立诊断；每阶段显式GC/scavenge。Private是进程私有提交，WorkingSet是当前工作集，Peak是进程生命周期峰值，GoHeapAlloc单列，三者不相加。
API字段依据 [GetProcessMemoryInfo](https://learn.microsoft.com/en-us/windows/win32/api/psapi/nf-psapi-getprocessmemoryinfo) 和 [PROCESS_MEMORY_COUNTERS_EX](https://learn.microsoft.com/en-us/windows/win32/api/psapi/ns-psapi-process_memory_counters_ex)。

| 规则集 / 阶段 | Private MiB | 当前WorkingSet MiB | 生命周期Peak MiB | GoHeapAlloc MiB | goroutine |
| --- | ---: | ---: | ---: | ---: | ---: |
| GetRequest / baseline | 20.75 | 19.48 | 20.23 | 1.85 | 5 |
| GetRequest / compiled_post_gc | 233.21 | 231.18 | 289.57 | 145.57 | 5 |
| GetRequest / old_and_new_db_post_gc | 366.45 | 364.19 | 397.19 | 250.80 | 5 |
| GetRequest / old_and_new_with_warm_owned_scratch | 367.23 | 365.05 | 397.19 | 251.40 | 13 |
| GetRequest / closed_and_unreachable_post_gc_scavenge | 33.75 | 32.27 | 397.19 | 1.89 | 5 |
| MITM / baseline | 20.72 | 19.51 | 20.26 | 1.87 | 5 |
| MITM / compiled_post_gc | 26.36 | 25.78 | 31.96 | 4.61 | 5 |
| MITM / old_and_new_db_post_gc | 29.38 | 28.77 | 35.46 | 6.99 | 5 |
| MITM / old_and_new_with_warm_owned_scratch | 29.74 | 29.19 | 35.46 | 7.14 | 13 |
| MITM / closed_and_unreachable_post_gc_scavenge | 23.75 | 23.50 | 35.46 | 1.90 | 5 |

双库阶段包含Compile旧库、Load新库及保留的镜像；并非只量一份native表。关闭Scratch、等待worker结束、关闭DB并放弃引用后，Go堆与goroutine回落。
Windows Private/WorkingSet未精确回到初值，可能包含分配器/runtime/DLL缓存；没有将剩余差额全部称为泄漏或将不下降的生命周期Peak当泄漏。
此诊断不是覆盖所有并发数的内存峰值保证，高并发活跃Scratch仍需按实际负载容量规划。

实际当前blob的独立native构造探针：GetRequest请求32,933,513B/4953次分配，MITM331,274B/132次；Close后受跟踪组件请求字节及live分配均归零。
相对T13前GetRequest44489次降为4953次，但多39536B所有权指针；不宣称C requested字节下降。
统计不含Go持有blob、Teddy/PCRE及分配器/runtime，不是进程RSS。GetRequest follow中1557804/2173366个word为零（71.68%），值得在后续任务评估运行时稀疏表示的代价。

## 默认计划、边界与未启用假设

| 已启用计划 | 资格/预算与回退 |
| --- | --- |
| T04/T05语义与错误修复 | Engine全局非重叠、完整上下文PCRE确认、非法UTF安全路由；错误与此前回调有序可观察，早停仍屏蔽不可观察的后续错误 |
| T06 subject及Group所有权 | 同次扫描只读共享subject；Group idle Scratch为min(GOMAXPROCS,4)，超4MiB数据缓冲完成扫描后淘汰；活跃借用者不截断、不排队限流，Close停止接收并等待借用者 |
| T09/T10存储与DFA正确性 | 只省严格为空的条件follow；DFA初态为空已消费集，保留所有已证实消费者；旧blob3 DFA回退原NFA |
| T11编译分析 | 每次Compile/每规则作用域共享exact AST；拼接前后宽度由O(k²)改为O(k)，溢出回原路径；无全局AST缓存 |
| T12/T13内存构造 | 合并派生表及临时blob复制精简；C基础表九次分配改一个受检查的arena，扩展表仍有所有权；不改变image6/blob4 |
| T14缓存身份 | 指纹仅快速拒绝；完整顺序、ID、flags、表达式相同才命中；不匹配一次刷新，显式LoadImage不替换用户规则 |
| T15短任务调度 | 记录<512B且GOMAXPROCS>=4，只将gate/anchor/bi任务调用线程执行，仍走原结果消息及错误/回调/join阶段；长记录、较低P及ScanBatch沿原路径 |
| T16单成员完成 | 仅公共存在性、单成员/W1、exact RE2、无断言/文本锚/legacy不安全，并证明原始/merged表等价；首次接受只结束该执行器，晚assert照常完成；定位/宽/多成员/forced-scalar/combined回原路径 |

T07通用range batch实际GetRequest资格覆盖为0，MITM可省调用太少，未启用。
T08新SIMD/lean循环只有合成局部大收益、未达真实场景门槛，全部原型运行代码已移除。
T15统一Serial使完整MITM慢9%–23%，统一Budget无稳定5%收益且一格P99+10.7%，均删除；没有隐藏lease或根计数。
T16跨度优化在实际W1无法减少word；GetRequest两个互斥尾锚成员也不支持一般成员退休，因此未添加元数据/动态mask。
更大IR/DAG框架未被剩余解析占比支持；探索X01–X06未强制实施。

## 剩余瓶颈与进一步算法空间

八个独立4秒阶段profile在所有无仪表计时之后运行。Go采样只能把C耗时定位到桥和调用者；没有原生指令、cache miss、PMU或Cgo单次延迟分解。

1. GetRequest稀疏输入的prefilter调用者约97.46%采样CPU；密集输入约67.25%，NFA确认约26.25%。下一步优先比较稀有必要因子/第二必要条件、共同前缀候选确认，而不是继续削几十纳秒边界调用。必须保留必要性证明、UTF和上下文安全；密集真命中还受输出量约束。
2. Load的packed展开约25.91%，Go表恢复约23.55%，GC drain约24.46%采样CPU。子项有嵌套，不能相加。直接packed→C或更少重复物化可能降低启动CPU/峰值，但须证明Go/native/scalar/DFA/debug各消费者和所有权，再作为新格式/实现任务验收；本轮没有留下半成品decoder。
3. Compile中的NFA构造约33.48%、Glushkov约26.58%、buildAlphabet约16.67%，GC drain约35.92%采样CPU；同样存在重叠。下一步可研究等价字符类构建、构造期指针/临时对象更少的布局，避免无界全局缓存。已经修掉拼接宽度O(k²)，并未把所有NFA构造降到线性。
4. 定位完整MITM约808ms，明显高于存在性273ms；profile里完整verifier约39.37%，native find_all约34.78%，PCRE2约21.26%。必须支付完整结果枚举和上下文确认，存在性singleton不能套给定位。
5. 稠密follow空间最坏O(P·ceil(P/64))，reach为O(A·ceil(P/64))。磁盘压缩能利用零/重复行，运行时压缩会改变随机访问及活跃状态处理成本；需以真实热路径试验，不能从1.45MB文件直接推算同等RAM或吞吐。

以密集输入prefilter约67.25%的采样CPU为模型，假设该部分理想2倍且其他CPU成本固定，Amdahl估计总CPU上限约1.51倍；这是模型估计，不是墙时实测。单项倍率不能相乘，跨Q/模式/规则集也不能相加成“平均生产收益”。
AC字面量扫描的工作取决于输入及触发/输出量；通用正则热路径还取决于活跃位置、异常边、候选确认和PCRE特性，没有宣布与规则数无关的统一O(L)。

## 发布准备与回滚

本阶段交付Windows准备证据，没有执行生产影子、canary、合并或生产发布。
后续接入机器后，补Linux/macOS原生完整语义和五轮计时，再做用户安排的独立审查。生产实际输入、存在性/定位/早停权重及SLO确定后，预注册CPU、P99、错误和内存阈值，先完整输出shadow/replay，再分阶段扩大。
任何输出/错误差异必须阻断；不能为性能回退已修复的错误传播、UTF、DFA或缓存身份语义。
需要回滚运行策略时可使用已验证14b80dbdf reader/二进制，仍保留image6/blob4与T03–T14正确性；候选/稳定缓存路径独立。先停止接收并等待旧库读者/Scratch，再关闭旧DB。禁止回退到带已知DFA/缓存/错误语义缺口的旧reader。

依赖、go.mod/go.sum、CI workflow相对main无变化。外部正则只使用现有VillanCh/go-pcre2-lite v0.1.6；其`regexp2`目录是该库兼容入口。
Essential Tests原有普通包矩阵包含`./common/minirehs/...`，未增加job或OS判断。新完整语料/内存导出默认关闭，普通回归保留新增资格/错误/边界/生命周期测试。

## 复现和证据索引

完整raw、源冻结、原始配对变化和任务交接位于用户本地性能计划的`evidence/T17/`；没有将语料/大体积原始日志上传GitHub。
核心文件：`T17-frozen/manifest.json`、`T17-correctness-summary.json`、`T17-runtime-summary.json`、`T17-absolute-summary.json`、`T17-components-summary.json`、`T17-resources-all.json`及各子目录完整JSONL/日志/profile。
所有Windows脚本对本轮实际路径和冻结binary做显式记录；移机先调整路径、记录工具链和机器，重新构建，不将跨编译视为原生测量。

普通测试：

```sh
CGO_ENABLED=1 go test -count=1 -timeout=5m ./common/minirehs/... ./common/crep/trafficguard
```

完整语义导出（为两计划设置不同目录；变量为测试专用，不是生产运行开关）：

```sh
CGO_ENABLED=1 MINIREHS_FULL_CORPUS=1 MINIREHS_EVIDENCE_DIR=/local/evidence/integrated \
  MINIREHS_SMALL_WORKER_MODE=ShortInline MINIREHS_SINGLETON_MODE=Singleton \
  go test ./common/minirehs -run '^TestProgramSystemCorpus$' -count=1 -timeout=10m
```

Reference对应`MINIREHS_SMALL_WORKER_MODE=Reference MINIREHS_SINGLETON_MODE=Reference`。Windows PowerShell使用`$env:名称='值'`设置环境。
绝对性能入口为`BenchmarkRuntimeScenarios`、`BenchmarkProgramSystemCorpus`、`BenchmarkProgramGroupReuse`、`BenchmarkCompilationImage`；先运行完整正确性，再冻结二进制，五轮分别启动新进程，串行运行，不和profile/编译并跑。

## 完整热扫描矩阵（五轮中位数）

RuntimeScenarios的Literal1000为1000条带唯一字面量的合成正则，PCRE128为128条指定lookahead规则；它们不能代表任意纯literal或PCRE负载。
HTTP912=912B，Sparse16K=16423B，DenseHTTP16K=17408B，Zero16K=16384B；各条原始输入完整保留。

| 规则/模式/输入或API | µs/op | MB/s | Go B/op | Go allocs/op |
| --- | ---: | ---: | ---: | ---: |
| ProgramGroupReuse/AnchoredPCRE | 1.123 | 22.27 | 256 | 9 |
| ProgramGroupReuse/GetRequest | 132.954 | 6.86 | 1,248 | 8 |
| ProgramGroupReuse/SmallLiteral | 0.556 | 17.99 | 224 | 7 |
| ProgramSystemCorpus/Exist | 272623.550 | 19.25 | 24,642,528 | 7,193 |
| ProgramSystemCorpus/Locate | 807852.400 | 6.50 | 24,722,056 | 3,994 |
| RuntimeScenarios/GetRequest/Exist/DenseHTTP16K | 3028.907 | 5.75 | 18,432 | 1 |
| RuntimeScenarios/GetRequest/Exist/HTTP912 | 132.308 | 6.89 | 1,024 | 1 |
| RuntimeScenarios/GetRequest/Exist/Sparse16K | 41.869 | 392.24 | 0 | 0 |
| RuntimeScenarios/GetRequest/Exist/Zero16K | 409.033 | 40.06 | 0 | 0 |
| RuntimeScenarios/GetRequest/Locate/DenseHTTP16K | 3144.690 | 5.54 | 18,432 | 1 |
| RuntimeScenarios/GetRequest/Locate/HTTP912 | 140.198 | 6.51 | 1,024 | 1 |
| RuntimeScenarios/GetRequest/Locate/Sparse16K | 42.278 | 388.45 | 0 | 0 |
| RuntimeScenarios/GetRequest/Locate/Zero16K | 405.973 | 40.36 | 0 | 0 |
| RuntimeScenarios/Literal1000/Exist/DenseHTTP16K | 4.726 | 3683.30 | 0 | 0 |
| RuntimeScenarios/Literal1000/Exist/HTTP912 | 0.398 | 2291.98 | 0 | 0 |
| RuntimeScenarios/Literal1000/Exist/Sparse16K | 4.623 | 3552.26 | 0 | 0 |
| RuntimeScenarios/Literal1000/Exist/Zero16K | 4.461 | 3672.80 | 0 | 0 |
| RuntimeScenarios/Literal1000/Locate/DenseHTTP16K | 4.599 | 3785.40 | 0 | 0 |
| RuntimeScenarios/Literal1000/Locate/HTTP912 | 0.396 | 2302.32 | 0 | 0 |
| RuntimeScenarios/Literal1000/Locate/Sparse16K | 4.884 | 3362.80 | 0 | 0 |
| RuntimeScenarios/Literal1000/Locate/Zero16K | 4.420 | 3706.66 | 0 | 0 |
| RuntimeScenarios/MITM/Exist/DenseHTTP16K | 904.425 | 19.25 | 18,459 | 1 |
| RuntimeScenarios/MITM/Exist/HTTP912 | 32.925 | 27.70 | 1,024 | 1 |
| RuntimeScenarios/MITM/Exist/Sparse16K | 75.644 | 217.11 | 18,440 | 1 |
| RuntimeScenarios/MITM/Exist/Zero16K | 72.018 | 227.50 | 16,384 | 1 |
| RuntimeScenarios/MITM/Locate/DenseHTTP16K | 906.677 | 19.20 | 18,451 | 1 |
| RuntimeScenarios/MITM/Locate/HTTP912 | 29.859 | 30.54 | 1,024 | 1 |
| RuntimeScenarios/MITM/Locate/Sparse16K | 73.487 | 223.48 | 18,439 | 1 |
| RuntimeScenarios/MITM/Locate/Zero16K | 72.376 | 226.37 | 16,384 | 1 |
| RuntimeScenarios/PCRE128/Exist/DenseHTTP16K | 4.745 | 3668.40 | 0 | 0 |
| RuntimeScenarios/PCRE128/Exist/HTTP912 | 0.382 | 2387.65 | 0 | 0 |
| RuntimeScenarios/PCRE128/Exist/Sparse16K | 31.934 | 514.29 | 48 | 2 |
| RuntimeScenarios/PCRE128/Exist/Zero16K | 4.427 | 3701.18 | 0 | 0 |
| RuntimeScenarios/PCRE128/Locate/DenseHTTP16K | 4.742 | 3671.33 | 0 | 0 |
| RuntimeScenarios/PCRE128/Locate/HTTP912 | 0.387 | 2356.91 | 0 | 0 |
| RuntimeScenarios/PCRE128/Locate/Sparse16K | 29.439 | 557.87 | 48 | 2 |
| RuntimeScenarios/PCRE128/Locate/Zero16K | 4.478 | 3658.84 | 0 | 0 |
