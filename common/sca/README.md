# SCA

SCA 从调用方提供的只读文件系统快照提取组件、出现位置、依赖声明和诊断。`common/sca/...` 的生产及测试闭包仅使用标准库和仓库内部叶子包，不需要第三方模块或 CGO。

`ScanReport(ctx, fs.FS, opts...)` 返回结构化报告；`ScanFilesystem` 保留旧包列表接口。20 类分析器及固定语法边界见 [function-contracts.json](function-contracts.json)。Docker、镜像、Git 获取入口已移除，调用方须先提供快照；POM 只访问快照中的父级、模块和显式仓库材料，JAR 不查询远端坐标。

组件按生态、名称、版本、来源等完整身份归并；文件路径及原生实例分别保留。范围、markers、extras、校验和与未解析引用作为证据保留，不推断为确定版本或已安装组件。`go.sum` 只提供精确模块校验和，不生成组件清单。npm 锁文件根记录的原始约束会进入关系；材料声明的 SRI/checksum 进入 `Verification` 和 SBOM hashes，冲突摘要不会无声合并。超限截取在稳定身份顺序下进行。POM 未知版本保留声明边；BOM import 按图访问展开，环、深度和步数超限返回结构化诊断。pnpm 只接受冻结的 v5/v6 标识，Cargo.lock 只接受缺省或 v1/v2/v3。缺失材料、语法错误、超限和取消返回诊断及不完整状态；`Diagnostic.Code` 在发现、读取、解析和输出阶段使用同一组可 `errors.Is/As` 识别的分类。调用方可以使用已确认的部分结果。SBOM 使用自有 CycloneDX 1.5 DTO，替代第三方 SDK 类型。

格式限制包括 pnpm v5/v6、只读 Cargo/Poetry TOML、静态 gemspec 和 pip 声明；不执行脚本或安装器，不支持任意 YAML 对象、TOML 1.1、pip includes 或 SQLite WAL 恢复。POM 父级坐标必须匹配，不读取宿主缓存和环境补全未知属性。具体正例、负例和兼容断言随各解析器测试保留。

资源默认值见 [resource-policy.json](resource-policy.json)。共用预算含文件/候选/读写字节、归档展开、语法节点、组件/观察/边，以及 **MaxResultBytes 逻辑结果内存估算**（不是 Go 堆、RSS 或严格分配前硬顶；0 用默认 256MiB，负值拒绝）。JSON/TOML/YAML/XML/文本工作副本、语法节点、材料缓存、入队、RPM/JAR/APK/DPKG 发出、归并插入和报告 append 在分配或插入前记账；解码器 4KiB scratch、RPM 页峰值、32KiB 读缓冲和 sort 索引有保守加价。`MergePackages` 对外无 error 签名保留：预算耗尽时返回**原切片**，不把 nil 伪装成空集合；该失败路径不改 license/边，避免回退后重复合并。同一指针只贡献一次证据。`MergePackagesBudget` 在变更输入前对可控的归并工作集（映射、额外证据拷贝、边、输出索引）记账。需要错误值时用该接口。GC/栈/map 桶增长仍可能超出估算，见 `result_memory.unbounded_runtime`。`TestFormatScanContract`/`BenchmarkFormatScan` 覆盖 20 个保留分析器的冻结夹具，不是 802 条候选审查，也不是单独的旧新完整矩阵。系统调用尝试须用隔离执行或平台追踪取证。

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
go test ./common/sca ./common/sca/analyzer ./common/sca/model -run '^$' -bench 'Benchmark(SparseDiscovery|ExactIdentity|Normalize|FormatScan)$' -benchmem
```

macOS 隔离启动器禁止网络、写盘、以及除**当前测试二进制**以外的进程执行。`TestReview5149_CyclicBOM` 与 `TestReview5149_BOMDeepAndBudgetChild` 会再执行同一测试二进制作为子进程，隔离配置显式允许该路径；这两项循环回归必须留在常规套件中，不要 skip。不要使用拒绝该二进制 exec 的配置，也不要把 `./common/sca/...` 整包隔离理解成“零进程执行”。

```sh
GOWORK=off CGO_ENABLED=0 go test ./common/sca/... -count=1 -exec "$PWD/scripts/sca/isolated-test-exec.sh"
```

固定样例及其期望是测试输入，来源与摘要见 [testdata/manifest.json](testdata/manifest.json)。上游许可见 [SCA_THIRD_PARTY_NOTICES.md](SCA_THIRD_PARTY_NOTICES.md)，源码与测试映射分别见 [source-extraction-map.json](source-extraction-map.json) 和 [test-migration-map.json](test-migration-map.json)。映射校验检查摘要、目标文件，以及 ADAPTED/SEMANTIC_REPLACEMENT/MIGRATED_AS_IS 必须有非空 `local_tests`；不代替语义测试或独立审查。
