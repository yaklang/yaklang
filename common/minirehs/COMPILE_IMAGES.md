# MVS 编译镜像、磁盘缓存与性能评估

`Database.MarshalImage` 将已经完成的 MVS 编译结果保存为 `.rehs`；`LoadImage` 恢复可扫描的数据库，
跳过正则解析、Glushkov 构造、字面量提取、自动机合并和 DFA 构造。
`LoadOrCompile` 在磁盘缓存缺失、规则变化或文件损坏时重新编译并替换缓存。

镜像保存正向 NFA、反向 NFA、合并 NFA、PatternID、字面量索引、路由和窗口参数，
也保存完整的 Go 必要条件过滤器。后者不能只靠 C blob 中的精简必要条件恢复，
否则纯 Go 的 merged 扫描会丢掉已有的预检查优化。

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

缓存保存的是原始镜像，不保存源代码运行时对象或指针。没有 scratch 池化、mmap 或后台写盘。
一次冷缓存需要额外导出和同步写盘；以后每次命中只增加读文件成本。

## v1 格式

所有整数小端。字符串使用 u32 字节长度，数组使用 u32 元素数；i32 的 `-1` 保留哨兵语义。
文件最后的 IEEE CRC32 覆盖其之前的全部字节。CRC 用于发现损坏，不能认证文件来源。

| 顺序 | 内容 |
| --- | --- |
| 32 字节头部 | `REHS`、imageVersion=1、mvsBlobVersion=3、flags、minLiteralLen、patternHash、nPatterns |
| 按 pattern idx | ID、原始 Flags、verifierKind、route、specialized、winW、gateHead、winHead、winTail、最终 expr、literals |
| 字面量索引 | 全局 literals；每个字面量的 patIdx、litHead、litBiHead、litBiTail |
| 路由列表 | mergedCount、maxAnchorNword、assertAlwaysOn、assertAlwaysOnC/Go、otherAlwaysOn、specializedAlwaysOn |
| 反向 unit | count；每条的 idx 和带长度的 encodeUnit 字节 |
| C blob | 带长度的原始 MVS1 v3 blob，导出时不重新构建 |
| regexp2 确认器 | 按 idx 升序的 idx、字节码长度；当前统一为 0，表示懒编译 |
| Go 必要条件 sidecar | 带长度的 `NEC1`，保存完整 merged 必要条件，以及非零 assert 必要条件的 idx 和内容 |
| trailer | u32 CRC32 |

flags 的 bit0 是定位，bit1 是已构建双向锚定；v1 拒绝 bit2/bit3 的 anchorMerged/assertMerged 及未知位。
route 的 bit0..6 依次是 hasLiterals、windowable、batchable、anchorable、biAnchorable、gate、re2Loc。
verifierKind 0 为 RE2，1 为 regexp2。specialized 直接使用保存的 kind，不重新识别表达式。

NEC1 中每个必要条件包含 i32 minRunLen、u8 runClass/firstByte/lastByte/hasFactor，
以及按字节值升序的非零 requiredBytes：u32 数量，随后每项是 u8 字节值和 i32 原 int16 计数。
先写 merged 条件，再写 u32 assert 数量及每条的 i32 idx 和必要条件。
这是初版格式对完整 Go 扫描状态的补充，不能省略。

加载首先验证外层和 unit 的完整边界、symbol/position/DFA 索引和位集 padding，之后才调用 C。
Go 位集从 unit 解码，标量/LimEx 派生表通过现有逻辑初始化，预过滤器从字面量建立。
整个加载过程不调用 syntax.Parse、regexp.Compile、tryCompileNFA、字面量提取或自动机构建。
RE2 和 regexp2 确认器只在扫描确实需要确认时，通过 sync.Once 初始化。
RE2 定位确认器使用 Longest，与 MVS 编译路径保持一致。

镜像字节与操作系统和 CPU 字节序无关，内嵌示例在 cgo/非 cgo 构建中检查相同字节。
无 cgo 时使用纯 Go NFA、Aho-Corasick 和 regexp2 确认器。
PCRE2 独有扩展仍然需要 cgo；跨构建分发包含 regexp2-only 的规则时，应使用两种确认器共有的语法及语义。
镜像格式不会把 PCRE2 扩展变为纯 Go regexp2 的新能力。
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
```

GetRequest 原始镜像约 30.86 MB（29.43 MiB），其中 MVS blob 约 28.98 MB，
其他路由和表达式约 1.88 MB。平均约 6.25 KB/规则，尺寸取决于 NFA 位置数、字母表、重复展开和反向 NFA，
不能仅按规则条数线性外推。256 条合成规则约 426 KB，MITM 约 306 KB，示例六条约 14 KB。
gzip BestSpeed 的 GetRequest 文件约 2.80 MB；v1 本身不压缩，压缩内嵌需要调用方先解压再 LoadImage，
会增加解压时间和临时内存。直接 go:embed 原始镜像大致增加同等大小的二进制资源段。

本次已落实三项性能改进：

1. Compile 的普通 RE2 路径不再通过 YakRegexpUtils 重复编译同一表达式。
2. Teddy 从字面量直接初始化，跳过不会使用的稠密 AC 表。GetRequest 首轮实测的加载临时 Go 分配
   从约 1.80 GB 降为约 126 MB，加载时间从约 429 ms 降至几十毫秒。Go B/op 包含临时分配，
   不等于存活堆、进程 RSS，也不包含 C malloc。
3. 大于 256 个字面量时，Teddy 确认增加最长 16 字节的前缀哈希索引，完整字面量比较仍是最终判据，
   哈希碰撞只增加候选。GetRequest 的 912 字节 HTTP 样本从约 4.4 ms/次降至约 0.46 ms/次，
   约快 9 倍；小字面量集合继续使用原来的桶确认路径。

镜像提高的是启动/重载速度；热扫描由同一套自动机执行。上述扫描提升来自 Teddy 确认算法改进，
不能把它归因于序列化。GetRequest 样本会触发大量共同 HTTP 前缀，当前约 2 MB/s；
相同 912 字节样本在合成和 MITM 规则集分别是几十 MB/s。吞吐高度依赖规则和输入，
这组数字不代表网络扫描的目标数/秒，也不包含网络、TLS、fp 捕获组确认等开销。

进一步的优先方向是：减少 Go NFA 恢复时的派生表分配，压缩持久位集及重复 unit，
降低纯 Go 稠密 AC 构建内存，以及减少规则大量共同前缀时的候选确认与 regexp2 临时分配。
现阶段没有证据支持默认打开全局 anchorMerged/assertMerged；它们仍然保持原来的关闭状态。

## 验证范围

测试覆盖确定性与再次导出、存在性/定位命中、窗口、锚定/反向 NFA、gate、反向引用、UTF-8、
0x80 非法 UTF-8 字节、单字/多字 NFA、specialized、ScanBatch、回调停止、Info 和并发懒确认。
真实 MITM 流量及 GetRequest 规则集对照 Compile 与 LoadImage；钩子检查 LoadImage 没有编译或语法分析。
磁盘测试包含身份变化、损坏替换、线程/多进程单次编译、清理和并发完整文件替换。
另有 CRC 正确的损坏字段测试、截断遍历及 FuzzLoadImage。

CI 覆盖 Linux cgo/纯 Go、Windows 纯 Go、内嵌文件新鲜度、native amalgamation、race 和 arm64 交叉编译。
arm64 交叉编译只验证可构建性，不代替 arm64 机器上的运行与性能测量。
