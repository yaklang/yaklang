# SCA

本内核为后续独立的依赖清除功能提供组件身份、依赖关系和来源证据；当前只做分析，不修改项目清单、锁文件或删除依赖。

SCA 从调用方提供的只读文件系统快照提取组件、出现位置、依赖声明和诊断。`common/sca/...` 的生产及测试闭包仅使用标准库和仓库内部叶子包，不需要第三方模块或 CGO。

`ScanReport(ctx, fs.FS, opts...)` 返回结构化报告；`ScanFilesystem` 保留旧包列表接口。20 类分析器及固定语法边界见 [function-contracts.json](function-contracts.json)。Docker、镜像、Git 获取入口已移除，调用方须先提供快照；POM 只访问快照中的父级、模块和显式仓库材料，JAR 不查询远端坐标。

组件按生态、名称、版本、来源等完整身份归并；文件路径及原生实例分别保留。范围、markers、extras、校验和与未解析引用作为证据保留，不推断为确定版本或已安装组件。`go.sum` 只提供精确模块校验和，不生成组件清单。材料声明的 SRI/checksum 进入 `Verification` 和 SBOM hashes，冲突摘要不会无声合并。超限截取在稳定身份顺序下进行。缺失材料、语法错误、超限和取消返回诊断及不完整状态；`Diagnostic.Code` 可 `errors.Is/As`。SBOM 使用自有 CycloneDX 1.5 DTO。

格式限制：pnpm 接受冻结 v5/v6 和 v9.0；Cargo.lock 接受缺省或 v1/v2/v3/v4；Poetry 支持旧布局及 lock-version 2.1；静态 gemspec 与 pip 声明不执行脚本或安装器；不支持任意 YAML 对象、TOML 1.1、pip includes 或 SQLite WAL 恢复。POM 父级坐标必须匹配，不读取宿主缓存和环境补全未知属性。

各格式保留字段由 `TestFourFormatFullFieldContract` 对照冻结旧 `ScanFilesystem` 转储和输入原文。名称版本投影相等不是字段等价，也不是 performance PASS。授权语义差单列，不是未声明的功能损失。

- Cargo/Yarn/pnpm/Poetry/Bundler：原文 range/specifier 与锁定版本分开；未匹配不按扫描顺序唯一 Resolved。
- gemspec：只接受 quoted / `%q` 字面量；动态表达式是 `unsupported_syntax` / `malformed_input` 且 Incomplete。
- packaging：Home-page 进入 Source；Requires-Dist 保留 extras/markers；Platform 不是 CPU Architecture。
- POM：空 scope 保持为空；`(,1.0]` 等不确定版本不升格；缺失 parent 是 `evidence_insufficient`。旧 dump 把 `Apache 2.0` 规范成 `Apache-2.0` 是授权语义差。
- Gradle：Observation.Scope 是锁行 scopes，无声明边。
- JAR：快照内 `pom.properties` 或 MANIFEST；保留外层文件并用 `!/` 限定内部证据位置，不计算 ZIP 摘要、不读远端 POM。
- Go binary：只读 `debug/buildinfo` 的 path/version/sum 与 replace；无 license/行号。
- Conan：完整 `ref` 留在 Source/Condition；缺失或空 ref 的 Target 为空并标 Incomplete；root node `0` 是合法容器。
- dpkg：已安装记录是 Component；Depends AND/OR 与 Provides 不是虚拟组件。
- RPM：已安装记录是 Component；Require/Provide 带 flags 与版本约束；path/soname 不是组件。全部 129 个包及 2535 条去重后的依赖，与冻结 SQLite 原始 header 独立对照；覆盖版本、架构、epoch/release、摘要、许可证、Require/Provide flags 和 SBOM 关联。
- pip：精确 `==` 才有 Version；范围/未钉死是声明。
- Pipfile.lock：只读 default 组的 index/hash/version；`_meta.sources` URL 与 develop 不扩张。
- composer.lock 保留 source URL#ref 与原文 require；composer.json 声明是单独身份；空 dist shasum 与 autoload 不适用。

资源默认值见 [resource-policy.json](resource-policy.json)。`MaxResultBytes` 是受控分配的逻辑预算：缓冲区扩容、固定字段转换、可执行文件/归档表及报告归一化均先预留再分配；0 用默认 256MiB，负值拒绝。它不等同于 Go 堆/RSS 硬顶，Go 运行时、固定启动/错误对象，以及调用方提供的文件系统与可信回调分配不包含在内。`MergePackages` 预算耗尽时保留原切片；需要错误信息时使用 `MergePackagesBudget`。

802 项候选已逐项核对迁移或排除依据，范围包括 22 项显式 TOML 语法裁剪；相关字段、正例、拒绝行为和错误分类由迁移测试约束。macOS 的开发验证器会先校准文件、网络和进程拦截，再检查代表性扫描；其范围是当前 Go/Darwin libc 调用路径，不宣称覆盖任意原始内核系统调用。

旧新差异按输入原文及冻结旧结果核对：dpkg/RPM 的虚拟范围组件改为声明、APK/RPM 的猜测边保留为候选；原始许可证文本不再强制 SPDX 归一化。Cargo/Poetry/Yarn/pnpm/npm/go.mod/Go binary 补全原文摘要；gemspec 保留各个 license 数组元素，新增声明边。Pipfile.lock 修复旧入口零识别；pip 去除旧扫描顺序造成的漏报；composer.json 根声明单列。Gradle/JAR/packaging/Conan 与 Bundler 的旧字段保留；新增字段和解析边由输入原文测试约束。范围裁剪仍以 `function-contracts.json` 为准，不通过更新旧 golden 隐藏差异。

## 新版本锁文件

- **Cargo.lock v4**：保留经过 URL 编码的 source 原文，依赖按完整 name/version/source 绑定；`%2B` 与 `+` 不做解码合并。未来版本继续显式拒绝。
- **pnpm lockfile v9.0**：将 `packages` 元数据与 `snapshots` 图关联，保留 scoped name、peer/patch 后缀、别名、workspace importer、dev/optional 声明和完整性摘要。快照缺少对应元数据时报材料不足，不生成虚假包；本地 `link:` 引用保留声明，不读取外部目录。不把包管理器版本号与 lockfile 版本号混用。
- **Poetry lock-version 2.1**：保留全部已锁定分组（含 dev），`Scope` 使用 `groups:[...]`，组条件映射在 `Condition` 中以 `markers:{...}` 保留；字符串 marker 原样保留。多条件依赖展开为分别带 constraint/marker/extras 的声明；同名多版本不凭扫描顺序绑定。旧格式的既有 runtime/category 行为保留。

上述清单描述锁文件中的材料，不表示每个包在当前宿主环境必然安装。组/marker 不在扫描器中求值；CycloneDX 的 `sca:observations`、`sca:requirements` 属性保留条件，使用方不能把跨环境图当作某个部署环境的安装证明。真实上游样例固定到 Cargo 仓库 tag `0.84.0`、pnpm `v9.15.9`、Poetry `2.1.1` 对应提交，另有编码身份、peer 隔离、组条件和破坏输入的自建反例；测试离线读取新 ZIP，不运行包管理器。历史旧新性能数字不覆盖新增格式。

作为完整 SCA 产品，后续仍需单独建设：uv/Bun、NuGet/.NET、Swift 等生态覆盖；指定部署环境的条件求值与跨材料关系核实；更广的真实仓库/异常输入语料；SPDX 及更新 CycloneDX 版本的导出；漏洞情报匹配、VEX/可达性和许可证策略。依赖清除还需上层结合构建与使用证据；仅凭锁文件没有直接引用不能安全删除依赖。这些能力不属于当前只读分析内核已完成的承诺。

## 算法边界

记 B 为实际处理的输入字节，N 为记录数，E 为显式边及候选数，D 为限制后的语法/引用深度。下述空间包括受控工作集，不把 Go 运行时 RSS 当作可精确预留的内存。

| 阶段 | 输入、输出与不变量 | 成本与限制 |
|---|---|---|
| 快照发现与调度 | 只读 fs.FS → 有序候选；路径不能越出材料根；文本分析器不打开无关文件 | 遍历 O(文件数)，目录排序 O(F log F)，固定 worker 数；累计文件、字节和入队 backing 先计量 |
| 文本/固定记录 | JSON、有限 TOML/YAML、XML、行记录 → 各格式 Library/Requirement；不执行表达式或反射解码框架 | 词法/遍历随 B 和节点数增长，原生索引通常 O(N+E)，排序另计；深度 D、字段、节点与转换工作集先预留 |
| POM/原生引用 | 材料内的父级、模块、锁定 ID → 显式引用与未决证据；绝不按同名随意绑定 | 缓存已访问材料；循环检测、引用深度、解析步数和边数上限，缺失引用标不完整 |
| RPM | BDB/NDB/SQLite 页与 RPM header → 固定包字段及 Require/Provide；无 SQL 引擎 | 按实际页访问、累计读取和 header 元素计量；重复页/溢出链有访问与循环限制；分配前验证 count/offset/type |
| JAR/EGG/Go binary | ZIP / 可执行文件 → 固定元数据；不启动子进程 | 中央目录、解压量、层数及可执行表项先检查；符号名索引 O(字符串表字节+符号数)，避免逐符号扫描完整表 |
| 身份与图 | 完整组件键、出现位置、声明 → 去重及确定性排序；范围不升格为安装身份，不跨项目/快照合并 | 哈希索引期望 O(N+E)，排序 O(N log N+E log E)，另计键字节；密集候选按实际 E 收费 |
| SBOM | 已归一化报告 → 自有 CycloneDX 1.5；引用必须存在、声明与确定边分开 | 随输出字段及 E 增长，循环不递归遍历依赖图；输出容量与字段受预算约束 |

语法错误、未知格式、材料不足、取消及超限使用不同诊断；共享预算失败丢弃并发图，避免把调度相关的任意前缀当作完整结果。资源最大值、默认值及 0/负值行为见 `resource-policy.json`。

## 冻结样例测量

Go 1.22.12、macOS arm64、M1 Max；旧基线 `d33a21b6` 与 `b12940b6cd`，21 轮独立进程，旧新顺序交替，取中位数。新侧计时包括 `ScanReport`，完整字段与 SBOM 在计时后导出并另行验证。15 类可比较样例的时间为旧版 0.288–0.810 倍、进程峰值 RSS 为 0.256–0.468 倍，均低于 1.10 门槛；这不是生产规模的普遍性能保证。

其余 5 类（dpkg、RPM、pip、Pipfile.lock、Composer）因已安装/声明清单、旧漏报或旧结果不确定而不作同语义比值验收。它们仍必须通过完整新契约及原始字段验证；尤其不能用旧 Pipfile.lock 零识别的耗时要求新实现“同工作量”。规模阶梯另测 100/1,000/10,000/100,000 条记录与实际边数，覆盖重复、来源分离、稠密图、环及范围。测试命令见下文；不提交机器日志。

## 接入与回滚

调用方负责获取不可变材料，再传入 `ScanReport(ctx, snapshot)` 或 `ScanFilesystem(snapshot)`；Git、Docker endpoint 和镜像/容器入口没有本地 CLI 回退。历史 Git 调用方改为显式本地快照，见 `git_to_sca.yak` 接入测试。普通内存快照无需临时文件。Go 二进制扫描沿用 ELF/PE 自动识别；底层固定解析器另保留薄 Mach-O，fat Mach-O、XCOFF 和 Plan9 不在冻结契约内。

`CreateCycloneDXSBOMByDXPackages` 返回项目自有 `*dxtypes.BOM`，使用 `MarshalCycloneDXBomToJSON` 序列化固定 CycloneDX 1.5 JSON；接入方不再依赖 `*cyclonedx.BOM` 或通用 XML/多版本 SDK。回滚须整体回退 SCA API、SSA/Yak 接入及模块文件，不在新内核内启用旧解析器兜底。

零第三方模块指 SCA 生产与测试闭包，全仓仍有真实使用者：`common/urfavecli/altsrc/toml_file_loader.go` 使用 TOML，`common/openapi/openapiyaml/yaml.go` 使用 YAML，`common/utils/yakgit` 使用 go-git，`common/consts/database.go` 使用 SQLite（根模块保留其 replace）。这些模块不能因 SCA 脱离而直接删除。

## 验证

Essential Tests 已包含 `./common/sca/...`，不增加独立 workflow。以下命令可在仓库根目录复现，输出无需提交：

```sh
GOWORK=off CGO_ENABLED=0 go test ./common/sca/... -count=1
GOWORK=off CGO_ENABLED=0 go vet ./common/sca/...
GOWORK=off CGO_ENABLED=1 go test -race ./common/sca/... -count=1
python3 scripts/sca/dependency_audit.py
python3 scripts/sca/dependency_audit.py --test
go run scripts/sca/source_audit.go
go run scripts/sca/source_audit.go common/utils/filesys/filesys_interface
python3 scripts/sca/verify_artifacts.py
go test ./common/sca ./common/sca/analyzer ./common/sca/model -run '^$' -bench 'Benchmark(SparseDiscovery|ExactIdentity|Normalize|NormalizeTopology|FormatScan)$' -benchmem
```

macOS 隔离启动器禁止网络、写盘、以及除**当前测试二进制**以外的进程执行。`TestReview5149_CyclicBOM` 与 `TestReview5149_BOMDeepAndBudgetChild` 会再执行同一测试二进制作为子进程，隔离配置显式允许该路径；这两项循环回归必须留在常规套件中，不要 skip。不要使用拒绝该二进制 exec 的配置，也不要把 `./common/sca/...` 整包隔离理解成“零进程执行”。

```sh
GOWORK=off CGO_ENABLED=0 go test ./common/sca/... -count=1 -exec "$PWD/scripts/sca/isolated-test-exec.sh"
```

固定样例存储在 44 个版本化 ZIP 中，共 852 个逻辑文件（含随附许可与独立 oracle）；原有 830 个文件的内容和原路径保持不变。ZIP 仅由 `fixtures*_test.go` 的 `go:embed` 引入；测试按需在内存解压，支持目录遍历、Seek/ReadAt 和独立可变读取副本，不解压到工作目录。生产包没有 embed 声明，`yak.go` 不导入测试加载器。

命名使用 `<格式及支持版本>_corpus_v<语料修订>.zip`，例如 `cargo_lock_le_v3_corpus_v1.zip`（包括 v3，故不写 lt_v3）、`pnpm_lock_v5_v6_corpus_v1.zip`、`toml_v1_0_and_rejected_v1_1_corpus_v1.zip`。格式版本与语料版本分开；未来新格式版本使用新归档，保留既有语料。ZIP 成员仍是该 `testdata` 下的相对路径，新增归档不能重复定义同一逻辑路径。

增加样本时：选择版本化归档与独立逻辑路径，在 manifest 中补充 `archive/member/path/sha256/bytes/mode/source`，保留原有记录；从包含这些原路径的外部材料树用 `fixture_store.py --source-root <expanded-common-sca>` 打包，并更新对应 archive 的摘要/大小及测试专用 embed 列表。生成器固定排序、时间、权限和压缩级别，拒绝与逐文件摘要不符的输入。不要把展开目录提交到仓库。

```sh
python3 scripts/sca/fixture_store.py --check-reproducible
python3 scripts/sca/verify_test_assets.py
# 可额外检查实际 yak.go 构建产物：
python3 scripts/sca/verify_test_assets.py --binary /path/to/yak
```

固定样例及其期望是测试输入，来源与摘要见 [testdata/manifest.json](testdata/manifest.json)。上游许可见 [SCA_THIRD_PARTY_NOTICES.md](SCA_THIRD_PARTY_NOTICES.md)，源码与测试映射分别见 [source-extraction-map.json](source-extraction-map.json) 和 [test-migration-map.json](test-migration-map.json)。映射校验检查摘要、目标文件，以及 ADAPTED/SEMANTIC_REPLACEMENT/MIGRATED_AS_IS 必须有非空 `local_tests`；不代替语义测试或独立审查。

附加开发验收工具均写入调用方指定的输出目录，不修改 workflow、不下载依赖：

```sh
python3 scripts/sca/mutation_check.py --out /path/to/mutation-output
OLD=/path/to/frozen-old-checkout OUT=/path/to/matrix COUNT=15 SCA_MATRIX_FULL_FIELDS=1 scripts/sca/run_format_scan_matrix.sh
# 在独立开发环境安装的 jsonschema 中运行，schema 来自固定参考包：
python scripts/sca/validate_sbom_matrix.py /path/to/matrix/matrix.json /path/to/pinned-schemas
python3 scripts/sca/observe_darwin.py --go /path/to/go --out /path/to/attempt-output
```

性能矩阵必须对各次 `old_runs/new_runs` 求中位数；最后一次结果不是中位数。规模实验覆盖 100/1,000/10,000/100,000 个记录，以及重复身份、同名不同版本/来源、稠密边、环和范围声明；记录数、实际边数、分配和逻辑预留量一起解释。
