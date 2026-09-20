# SCA

SCA 从调用方提供的只读文件系统快照提取组件、出现位置、依赖声明和诊断。`common/sca/...` 的生产及测试闭包仅使用标准库和仓库内部叶子包，不需要第三方模块或 CGO。

`ScanReport(ctx, fs.FS, opts...)` 返回结构化报告；`ScanFilesystem` 保留旧包列表接口。20 类分析器及固定语法边界见 [function-contracts.json](function-contracts.json)。Docker、镜像、Git 获取入口已移除，调用方须先提供快照；POM 只访问快照中的父级、模块和显式仓库材料，JAR 不查询远端坐标。

组件按生态、名称、版本、来源等完整身份归并；文件路径及原生实例分别保留。范围、markers、extras、校验和与未解析引用作为证据保留，不推断为确定版本或已安装组件。`go.sum` 只提供精确模块校验和，不生成组件清单。缺失材料、语法错误、超限和取消返回诊断及不完整状态，调用方可以使用已确认的部分结果。SBOM 使用自有 CycloneDX 1.5 DTO，替代第三方 SDK 类型。

格式限制包括 pnpm v5/v6、只读 Cargo/Poetry TOML、静态 gemspec 和 pip 声明；不执行脚本或安装器，不支持任意 YAML 对象、TOML 1.1、pip includes 或 SQLite WAL 恢复。POM 父级坐标必须匹配，不读取宿主缓存和环境补全未知属性。具体正例、负例和兼容断言随各解析器测试保留。

资源默认值见 [resource-policy.json](resource-policy.json)。读入、归档及语法展开有界，但部分格式 DTO 在输出计数检查前仍按字节/语法预算分配，尚不满足严格的全局分配前内存预算。逐候选语义迁移的完整独立复核、全格式旧新性能矩阵及系统调用尝试计数也尚未完成；零依赖不代表这些门槛已经通过。

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
go test ./common/sca ./common/sca/analyzer ./common/sca/model -run '^$' -bench 'Benchmark(SparseDiscovery|ExactIdentity|Normalize)$' -benchmem
```

macOS 可额外执行禁止网络、文件写入及子进程的隔离测试：

```sh
GOWORK=off CGO_ENABLED=0 go test ./common/sca/... -count=1 -exec "$PWD/scripts/sca/isolated-test-exec.sh"
```

固定样例及其期望是测试输入，来源与摘要见 [testdata/manifest.json](testdata/manifest.json)。上游许可见 [SCA_THIRD_PARTY_NOTICES.md](SCA_THIRD_PARTY_NOTICES.md)，源码与测试映射分别见 [source-extraction-map.json](source-extraction-map.json) 和 [test-migration-map.json](test-migration-map.json)。映射校验只检查摘要和目标存在性，不代替语义测试或独立审查。
