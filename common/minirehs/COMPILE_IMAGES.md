# MVS 编译镜像、磁盘缓存与性能评估

`Database.MarshalImage` 将已经完成的 MVS 编译结果保存为 `.rehs`；`LoadImage` 恢复可扫描的数据库，
跳过正则解析、Glushkov 构造、字面量提取、自动机合并和 DFA 构造。
`LoadOrCompile` 在磁盘缓存缺失、规则变化或文件损坏时重新编译并替换缓存。

镜像保存正向 NFA、反向 NFA、合并 NFA、PatternID、字面量索引、路由和窗口参数，
也保存完整的 Go 必要条件过滤器。后者不能只靠 C blob 中的精简必要条件恢复，
否则纯 Go 的 merged 扫描会丢掉已有的预检查优化。

默认先做结构化精简，再用 zstd 压缩：共享字符串及重复表行、稀疏位集、变长整数，
并省去可由字符区间恢复的 ASCII 映射。无需调用方额外解压，旧版稠密镜像也能加载。

## 使用接口

```go
db, err := minirehs.LoadOrCompile(cachePath, patterns,
    minirehs.WithBackend(minirehs.BackendMVS),
    minirehs.WithReportLocation(false),
)
if err != nil {
    return err
}
defer db.Close()
scratch, err := db.NewScratch()
if err != nil {
    return err
}
defer scratch.Close()
return db.Scan(input, scratch, handler)
```

`LoadImage(data)` 不接受 Option，模式、最短字面量长度等编译选项来自镜像。
加载完成后可以修改或释放传入的缓冲区；数据库拥有自己的 blob 和 Go 数组。
同一数据库可并发扫描，每个扫描者独占 Scratch。Scratch、日志器和运行期 A/B 开关不进入镜像。

`MarshalImage` 对 Engine/Stdlib 返回 `ErrUnsupportedImage`；
`LoadOrCompile` 对这两种后端直接编译，不访问缓存文件。
当前 `Auto` 仍然选择 Engine，需要显式选择 `BackendMVS` 才会启用镜像缓存。

| 错误 | 含义 |
| --- | --- |
| `ErrUnsupportedImage` | 后端不支持导出，或输入超过当前格式可表示的范围 |
| `ErrIncompatibleImage` | 镜像/内核版本、功能位或确认器字节码不受本构建支持 |
| `ErrCorruptImage` | 魔数、校验和、长度、索引、位集、unit 或内核打开失败 |

加载失败只返回错误。自动重编译只发生在 `LoadOrCompile`。
缓存目录、读取、同步或替换失败也会返回错误，调用方可以据此决定自身的失败策略。

## 构建期生成与内嵌

生成器读取有稳定顺序的 JSON `[]minirehs.Pattern`：

```sh
go run ./common/minirehs/tools/rehs-image \
  -patterns rules.json -out rules.rehs
```

定位模式加 `-location`；最短字面量长度使用 `-min-literal`。构建期生成的模式必须与运行时用途相符。
`-check` 会重新编译并与提交的镜像逐字节比较，发现过期文件时失败。
生成器是独立命令，普通库测试不会执行它。

```go
//go:embed rules.rehs
var image []byte

func builtinDB() (minirehs.Database, error) {
    return minirehs.LoadImage(image)
}
```

完整示例及新鲜度测试在 `examples/embedded`：

```sh
go generate ./common/minirehs/examples/embedded
go test ./common/minirehs/examples/embedded
```

内置镜像失效应作为启动错误处理，并随规则/编译器一起重新生成发布。
用户配置规则适合走磁盘缓存。此 PR 提供库接口和内嵌示例；生产 fp 调用方需要在自己的规则生命周期中接入，
不会仅因升级 minirehs 自动改变现有 fp 的加载方式。

## 缓存一致性与磁盘行为

身份使用 FNV-64，按顺序包含每条规则的 ID、原始 Flags、表达式字节长度及原始字节，
再包含生效的最短字面量长度、编译功能位、镜像版本和 MVS blob 版本。
改表达式、ID、Flags、规则顺序或编译模式都会失效；日志器不影响身份。

命中缓存时只读取、校验和加载。未命中时通过同路径 `.lock` 的独占创建协调线程及进程，
拿到锁后重新检查缓存，避免等待者重复编译。编译或写入失败会清理本次的锁和临时文件。
进程被终止后可能遗留锁；等待上限为两分钟，之后返回错误。
应在确认写入进程已经结束后清除遗留锁，不能仅按文件年龄抢占正在编译的写入者。

写入使用同目录临时文件、0644 权限、`Sync`、关闭后替换。
Unix 使用 Rename；Windows 读句柄允许删除共享，支持的文件系统使用 FileRenameInfoEx 的 POSIX 替换语义，
旧读句柄继续读取旧文件，新读者打开新文件；旧系统/文件系统回退到有限重试的 MoveFileEx。
失败时保留原缓存。Windows 语义依据 [Microsoft 的 FILE_RENAME_INFORMATION 文档](https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/ntifs/ns-ntifs-_file_rename_information)。
这里只承诺文件内容同步和完整文件替换，不承诺所有文件系统在掉电后的目录项持久性。

缓存保存的是精简并压缩的镜像，不保存运行时对象或指针。没有 scratch 池化、mmap 或后台写盘。
一次冷缓存需要额外导出和同步写盘；以后每次命中只增加读文件成本。

## v1 格式

固定宽度整数小端；精简表的整数采用规范的无符号 LEB128，带符号值采用 zigzag。
文件最后的 IEEE CRC32 覆盖其之前的全部字节。CRC 用于发现损坏，不能认证文件来源。

头部仍为 32 字节。flags bit4 表示 zstd 封装：头部后是 u32 解压长度、一个完整内部镜像的
zstd 数据和外部 CRC32。内部镜像有自己的头部及 CRC；其身份和编译选项必须与外部一致，
不接受嵌套压缩。固定使用 1 MiB 窗口、单线程和 SpeedDefault；已有依赖提供纯 Go 编解码。
解压限制窗口及输出长度，不按伪造长度直接预分配巨量内存。不受益于压缩的内容保留未压缩形式。

以下是去掉压缩封装后的布局。flags bit5 表示精简表；没有此位的稠密 v1 镜像保持兼容。

| 顺序 | 内容 |
| --- | --- |
| 32 字节头部 | `REHS`、imageVersion=1、mvsBlobVersion=3、flags、minLiteralLen、patternHash、nPatterns |
| 按 pattern idx | ID、原始 Flags、verifierKind、route、specialized、winW、gateHead、winHead、winTail、最终 expr、literals |
| 字面量索引 | 全局 literals；每个字面量的 patIdx、litHead、litBiHead、litBiTail |
| 路由列表 | mergedCount、maxAnchorNword、assertAlwaysOn、assertAlwaysOnC/Go、otherAlwaysOn、specializedAlwaysOn |
| 反向 unit | count；每条的 idx 和带长度的 MPU1；稠密模式为原 encodeUnit 字节 |
| C blob | 带长度的 MVP1；稠密模式为原 MVS1 v3 blob |
| PCRE2 确认器 | 按 idx 升序的 idx、字节码长度；当前统一为 0，表示懒编译 |
| Go 必要条件 sidecar | 带长度的 `NEC1`，保存完整 merged 必要条件，以及非零 assert 必要条件的 idx 和内容 |
| trailer | u32 CRC32 |

flags 的 bit0 是定位，bit1 是已构建双向锚定；bit4/bit5 只描述存储方式，不改变缓存身份。
v1 拒绝 bit2/bit3 的 anchorMerged/assertMerged 及其他未知位。
route 的 bit0..6 依次是 hasLiterals、windowable、batchable、anchorable、biAnchorable、gate、re2Loc。
verifierKind 0 为 RE2，1 为 VillanCh/go-pcre2-lite 的 PCRE2 确认器。
specialized 直接使用保存的 kind，不重新识别表达式。

NEC1 中每个必要条件包含 i32 minRunLen、u8 runClass/firstByte/lastByte/hasFactor，
以及按字节值升序的非零 requiredBytes：u32 数量，随后每项是 u8 字节值和 i32 原 int16 计数。
先写 merged 条件，再写 u32 assert 数量及每条的 i32 idx 和必要条件。
这是初版格式对完整 Go 扫描状态的补充，不能省略。

精简模式的具体规则：

- 表达式及字面量共用字符串表。token 0 后跟变长字节长度和新字符串；其他 token 为首次出现顺序的索引加 1。
  全局字面量索引直接引用规则中已经保存的字符串，列表元素数仍为 u32。
- MVP1 是魔数、u32 恢复长度、u8 是否存在 merged unit、u32 位集行字典数、u32 整数数组字典数、
  每个 pattern 是否有 unit 的位图，随后按原顺序保存 unit。
  单独的反向 MPU1 是魔数、u32 恢复长度、两个 u32 字典数及一个 unit。偏移、长度表和 slot→unit 序号在加载时恢复。
- unit 保存变长 npos、nsym、flags，nword 由 npos 推导；表及扩展字段顺序保持 MVS v3。
  cuts 恢复后生成 128 项 asciiSym，不存重复的映射表。DFA 状态数、guard 数量及必要条件整数也用变长编码。
- 位集按行去掉尾部零字，全零行只写 token 0；token 1 定义新行，其他 token 引用先前行的索引加 2。
  新行在「置位位置差分」「非零字索引差分 + u64」「连续原始字」三种表示中选择最短者。
  MVP1 中的所有 unit 共享行字典，相同行在不同宽度的表中也可通过补零复用。
- cuts、posPat、条件后继位置和 DFA 转移表共用整数数组字典，token 0 表示空数组、1 定义新数组，其他 token 为索引加 2。
  新数组按实际长度选择 RLE、i32 差分或逐值 zigzag，长度来自 unit 字段，加载时验证引用的形状。

共享按完整内容判等；哈希表只负责查找，不允许哈希碰撞替代完整比较。字典始终按首次出现分配索引，
没有 map 遍历带来的不稳定输出。精简 blob 恢复上限为 256 MiB；超过此大小的编译结果导出时使用稠密格式。
加载后再次导出保留原字节；调用方修改输入或导出缓冲不会改变数据库。
解码字典只保存目标缓冲中的偏移和长度，不为每一条共享行另分配一份数组；字典本身也无需 GC 扫描指针。
旧稠密缓存可继续命中，不在读取时自动重写；需要缩小时可移除旧缓存，让下一次编译生成新格式。

加载首先验证外层和 unit 的完整边界、symbol/position/DFA 索引和位集 padding，之后才调用 C。
Go 位集从 unit 解码，标量/LimEx 派生表通过现有逻辑初始化，预过滤器从字面量建立。
整个加载过程不调用 syntax.Parse、regexp.Compile、tryCompileNFA、字面量提取或正则 NFA/DFA 构造。
RE2 和 PCRE2 确认器只在扫描确实需要确认时，通过 sync.Once 初始化。
RE2 定位确认器使用 Longest，与 MVS 编译路径保持一致。
因此 LoadImage 本身保证零正则编译；存在性 NFA 已经权威命中的规则也不编译确认器。
需要 gate、无 NFA 兜底或 RE2 断言定位确认的规则，在首次候选命中时仍可能编译确认器。
v1 没有保存标准库 regexp 字节码，也没有保证任意规则集的第一次扫描完全免编译。

镜像字节与操作系统和 CPU 字节序无关，内嵌示例检查确定性输出。
正则确认复用仓库现有的 VillanCh/go-pcre2-lite，不引入 dlclark/regexp2 或其他新依赖。
当前包需要启用 cgo，并提供该 PCRE2 绑定所需的 C 工具链；不承诺 CGO_ENABLED=0 可以编译。
纯 Go NFA/AC 是扫描实现，不能替代 PCRE2 确认器的构建依赖。
改变编译语义、路由或持久数据契约时必须升级镜像版本并重新生成内置文件。

## 实测与优化

测试环境：2026-10-18，Windows/amd64，Go 1.22.12，i5-13490F，NVMe SSD，启用 cgo。
存在性模式，分别测试 256 条合成规则、87 条可编译 MITM 规则，以及仓库默认 GetRequest 的 4941 条规则。
GetRequest 包含 nfp.gz 的 4940 条和随包附加的 HTTP softmatch，保留规则顺序。
测量命令和数据口径如下；实际数值由同文件中的 benchmark 输出复现：

```sh
go test ./common/minirehs -run '^$' -bench BenchmarkCompilationImage \
  -benchmem -benchtime=500ms -count=3
go test ./common/minirehs -run '^TestImageGetRequest$' -v -count=1
go test ./common/minirehs -run '^$' -bench BenchmarkTeddyGetRequestConfirmation \
  -benchmem -benchtime=1s -count=3
```

下表取每项三轮的中位数。编译/加载包含数据库 Close；读文件包含打开、读取和关闭，磁盘数据为系统页缓存已热的读，
不代表机械硬盘冷读。写入项包含文件 Sync 和替换。

| 数据集 | 默认镜像文件 | Compile | LoadImage | 加载加速 | 磁盘缓存命中 |
| --- | --- | --- | --- | --- | --- |
| 合成 256 条 | 5.31 KB | 15.61 ms | 1.50 ms | 10.4× | 1.47 ms |
| MITM 87 条 | 26.22 KB | 12.66 ms | 2.45 ms | 5.2× | 2.59 ms |
| GetRequest 4941 条 | 1.37 MB | 369.29 ms | 68.83 ms | 5.4× | 68.35 ms |

GetRequest 单独读文件约 0.20 ms，导出约 75.74 ms，其中不做 zstd 的结构精简导出约 52.20 ms。
同步写盘/替换中位数约 2.34 ms，三轮范围约 2–11 ms。完整的冷缓存编译并保存约 452.98 ms，
比只编译增加约 84 ms（23%），以后每次命中节省约 301 ms。一次后续命中即可摊回首次保存的成本。
加载并执行第一次扫描约 74.91 ms；这项包含确实需要的懒确认器编译，预热扫描不包含它。
独立项目的中位数会受 GC 和 I/O 波动影响，不能精确相加。

同二进制加载旧稠密镜像约 42.67 ms，加载精简但未压缩镜像约 63.89 ms，
最终默认文件约 68.83 ms。结构恢复和解压确实消耗 CPU；本机 NVMe 热缓存场景下，稠密格式加载更快。
原始大文件先前测得的热读约 8.65 ms、同步写盘约 35 ms；新格式大幅减少了文件 I/O。
若冷存储带宽假设为 100 MB/s，30.86 MB 的传输约需 309 ms，1.37 MB 约需 14 ms；
这是带宽推算，不是本机测得的机械硬盘数据。

GetRequest 的每次加载 Go 分配约 135.2 MB，缓存命中约 136.5 MB，编译约 498.6 MB。
缓存比内嵌额外分配读取整份文件的缓冲；库内仍需保留 blob、Go NFA，并在 cgo 构建中复制 C 数组。
字典解码直接写目标缓冲，已将初版精简加载的约 159.9 MB 分配降低约 25 MB。

GetRequest 尺寸的三个阶段：

| 表示 | 字节数 | 相比稠密镜像 |
| --- | ---: | ---: |
| 原始稠密布局 | 30,860,935 | 100% |
| 结构精简，未做 zstd | 3,953,835 | 12.8% |
| 结构精简后再 zstd | 1,370,674 | 4.4% |

原始 MVS blob 28,982,285 字节精简为 2,488,563 字节；路由、表达式等其余内容也从约 1.88 MB
降至约 1.47 MB。最终相较原始镜像减少 95.6%，约 277 字节/规则。
直接对稠密镜像做 gzip BestSpeed 约 2.80 MB、zstd SpeedDefault 约 2.23 MB；
结构精简后再压缩比单做 zstd 再小约 39%。示例六条最终 871 字节。
实际尺寸取决于位置数、字母表、重复展开及反向 NFA，不能仅按规则条数线性外推。
go:embed 的资源段保存最终小文件；加载后仍需恢复稠密运行表，文件变小不代表运行内存减少相同比例。

本次已落实的其他性能改进：

1. Compile 的普通 RE2 路径不再通过 YakRegexpUtils 重复编译同一表达式。
2. Teddy 从字面量直接初始化，跳过不会使用的稠密 AC 表；含单字节字面量时直接建立 Go AC，
   也不再先建立一份随后释放的 C AC。GetRequest 首轮实测的加载临时 Go 分配
   从约 1.80 GB 降为当前默认镜像约 135 MB，加载时间从约 429 ms 降至几十毫秒。Go B/op 包含临时分配，
   不等于存活堆、进程 RSS，也不包含 C malloc。
3. 大于 256 个字面量时，Teddy 确认增加最长 16 字节的前缀哈希索引，完整字面量比较仍是最终判据，
   哈希碰撞只增加候选。同二进制、同规则/记录、预热后的 A/B 中，GetRequest 的 912 字节 HTTP 样本
   从 4.285 ms/次降至 0.523 ms/次，约快 8.2 倍；小字面量集合继续使用原来的桶确认路径。

镜像提高的是启动/重载速度；热扫描由同一套自动机执行。上述扫描提升来自 Teddy 确认算法改进，
不能把它归因于序列化。GetRequest 样本会触发大量共同 HTTP 前缀，当前约 1.7–2 MB/s；
相同 912 字节样本在合成和 MITM 规则集分别是几十 MB/s。吞吐高度依赖规则和输入，
这组数字不代表网络扫描的目标数/秒，也不包含网络、TLS、fp 捕获组确认等开销。

进一步的优先方向是：减少 Go NFA 恢复时的派生表分配，降低恢复共享表的 CPU 成本，
降低纯 Go 稠密 AC 构建内存，以及减少规则大量共同前缀时的候选确认与 PCRE2 临时分配。
现阶段没有证据支持默认打开全局 anchorMerged/assertMerged；它们仍然保持原来的关闭状态。

## 验证范围

测试覆盖确定性与再次导出、存在性/定位命中、窗口、锚定/反向 NFA、gate、反向引用、UTF-8、
0x80 非法 UTF-8 字节、单字/多字 NFA、specialized、ScanBatch、回调停止、Info 和并发懒确认。
真实 MITM 流量及 GetRequest 规则集对照 Compile 与 LoadImage；钩子检查 LoadImage 没有编译或语法分析。
磁盘测试包含身份变化、损坏替换、线程/多进程单次编译、清理和并发完整文件替换。
另有稠密/精简格式兼容、原生 blob 逐字节还原、压缩损坏/长度伪造、规范变长整数、
稀疏及稠密行、跨 unit 去重、CRC 正确的损坏字段测试、截断遍历及 FuzzLoadImage。
PCRE2 专用的 match-start reset、branch reset 及 DEFINE 子程序也检查镜像往返后的命中/不命中语义。

CI 仅在 Essential Tests 现有 Utils Infra / Parser / Net / Misc 的普通测试列表中加入
`./common/minirehs/...`，使用统一测试执行器，不添加平台判断、专用步骤或独立工作流。
常规测试运行整包完整参数，包含内嵌文件新鲜度；本机另行验证 native amalgamation、race 和 fuzz。
race 专门运行 Image、Teddy 和 MVSAmalgamation 测试，包含并发懒确认和缓存竞争。
整包 race 的已有标准库 oracle 差分会超过五分钟，本机及第一轮 Linux CI 均观察到此情况，
因此不将整包 race 记录为通过。
不执行无 cgo 测试或无 cgo 交叉编译；arm64 的运行与性能需要在启用 cgo 的原生机器上验证。
