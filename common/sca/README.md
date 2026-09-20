# SCA

SCA 从调用方提供的只读文件系统快照提取组件、出现位置、依赖声明和诊断。`common/sca/...` 的生产及测试闭包仅使用标准库和仓库内部叶子包，不需要第三方模块或 CGO。

`ScanReport(ctx, fs.FS, opts...)` 返回结构化报告；`ScanFilesystem` 保留旧包列表接口。20 类分析器及固定语法边界见 [function-contracts.json](function-contracts.json)。Docker、镜像、Git 获取入口已移除，调用方须先提供快照；POM 只访问快照中的父级、模块和显式仓库材料，JAR 不查询远端坐标。

组件按生态、名称、版本、来源等完整身份归并；文件路径及原生实例分别保留。范围、markers、extras、校验和与未解析引用作为证据保留，不推断为确定版本或已安装组件。`go.sum` 只提供精确模块校验和，不生成组件清单。npm 锁文件根记录的原始约束会进入关系；材料声明的 SRI/checksum 进入 `Verification` 和 SBOM hashes，冲突摘要不会无声合并。超限截取在稳定身份顺序下进行。POM 未知版本保留声明边；BOM import 按图访问展开，环、深度和步数超限返回结构化诊断。pnpm 只接受冻结的 v5/v6 标识，Cargo.lock 只接受缺省或 v1/v2/v3。缺失材料、语法错误、超限和取消返回诊断及不完整状态；`Diagnostic.Code` 在发现、读取、解析和输出阶段使用同一组可 `errors.Is/As` 识别的分类。调用方可以使用已确认的部分结果。SBOM 使用自有 CycloneDX 1.5 DTO，替代第三方 SDK 类型。

格式限制包括 pnpm v5/v6、只读 Cargo/Poetry TOML、静态 gemspec 和 pip 声明；不执行脚本或安装器，不支持任意 YAML 对象、TOML 1.1、pip includes 或 SQLite WAL 恢复。POM 父级坐标必须匹配，不读取宿主缓存和环境补全未知属性。具体正例、负例和兼容断言随各解析器测试保留。

资源默认值见 [resource-policy.json](resource-policy.json)。共用预算含文件/候选/读写字节、归档展开、语法节点、组件/观察/边，以及 **MaxResultBytes 逻辑结果内存估算**（不是 Go 堆或 RSS；0 用默认 256MiB，负值拒绝）。JSON/TOML/YAML/XML/文本工作副本、语法节点、材料缓存、入队、RPM/JAR/APK/DPKG 发出、归并插入和报告 append 在分配或插入前记账；快照材料与 POM 分析结果按 id 只计一次。encoding/json 与 encoding/xml 解码器内部缓冲、RPM 页缓冲（计入 MaxReadBytes）、`sort.Slice` 临时索引和 `MergePackages` 兼容适配器仍可能超出该估算，见 resource-policy.json 的 `result_memory.unbounded_runtime`。`TestFormatFieldMatrix` 与 `BenchmarkFormatScan` 覆盖冻结夹具上的字段语义和格式扫描；它们不是 802 条候选的独立审查，也不是与旧实现逐格式对照的完整性能证明。系统调用尝试须用隔离执行或平台追踪取证，零依赖导入审计不能代替。

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

macOS 可额外执行禁止网络、文件写入及子进程的隔离测试：

```sh
GOWORK=off CGO_ENABLED=0 go test ./common/sca/... -count=1 -exec "$PWD/scripts/sca/isolated-test-exec.sh"
```

固定样例及其期望是测试输入，来源与摘要见 [testdata/manifest.json](testdata/manifest.json)。上游许可见 [SCA_THIRD_PARTY_NOTICES.md](SCA_THIRD_PARTY_NOTICES.md)，源码与测试映射分别见 [source-extraction-map.json](source-extraction-map.json) 和 [test-migration-map.json](test-migration-map.json)。映射校验只检查摘要和目标存在性，不代替语义测试或独立审查。
