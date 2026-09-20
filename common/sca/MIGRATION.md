# SCA 输入与 API 迁移

本次在 `codex/sca-minimal-dependency-core` 上替换 SCA 运行链路。兼容基线为 `d33a21b6a30945df93a663a18f3cce087b0f3718`。原始本地验收快照先于 PR 创建；提交与远端 CI 状态以 PR 为准。

## 输入

首选 `ScanReport(ctx, fs.FS, options...)`。调用方负责取得不可变、只读、经过授权的材料快照；`ScanFilesystem` 保留旧 Package 返回值，并同时返回不完整扫描错误。零值预算映射到 `resource-policy.json` 的有限默认值。

删除 `ScanDockerImageFromContext`、`ScanDockerContainerFromContext`、`ScanDockerImageFromFile`、`ScanGitRepo` 和 Docker `endpoint` 选项。镜像解包、Git 检出及历史版本枚举属于调用方工作：每次把准备好的文件树作为单独快照交给 SCA，使用 `WithSnapshotID` 保留外部快照标识。核心不再连接 Docker、不调用 Git、没有 CLI 或旧解析器 fallback。

普通文件和虚拟文件系统均不需要临时文件。默认不跟随符号链接，拒绝非普通文件与越界逻辑路径。`os.DirFS` 兼容入口能检测部分文件替换，但不是抗并发恶意修改的宿主文件系统沙箱；需要强隔离的调用方应先提供不可变快照。

POM 只读取快照内的相对父级、模块及明确提供的 `repository/<group>/<artifact>/<version>/<artifact>-<version>.pom`。不读取 HOME、`.m2/settings.xml`、本地 Maven 缓存、环境属性或网络仓库。

RPM SQLite 输入必须是提交完成、无活动 WAL/journal 的快照。非空旁文件会产生错误；不恢复数据库，不运行 SQL。仅传入裸数据库 ReaderAt 时，快照一致性由调用方保证。

## 数据与类型

新 Report 分离 ComponentKey、Observation、Requirement 和 Diagnostic。空版本表示未知；约束保留在 Requirement，不能由范围猜测安装版本。相同组件在不同项目、路径及 peer 上下文中保留不同观察记录。错误不等于空成功，调用方必须同时检查 `error` 和 `Report.Complete`。

`dxtypes.Package` 保留既有字段；新增生态、来源、架构、实例、证据类别、原始许可证等字段放在可选 `PackageDetails` 中。旧构造方式不分配扩展证据；读取用 `Details()` 取得零值兼容视图，写入前调用 `EnsureDetails()`，或在构造时显式提供 `PackageDetails`。`MergePackages` 和 `CanMerge` 只允许精确身份去重，不再执行同名版本猜配、AND/OR 字符串合并。SSA 调用方继续消费兼容结构，但遇到错误仍保留已经取得的部分证据。

CycloneDX 接口现在返回本项目自有 `dxtypes.BOM`，不再返回 SDK 类型。旧 JSON 辅助函数仍可序列化该 DTO；新调用方优先使用 `CreateCycloneDXSBOMFromReport`。输出固定为 1.5，依赖关系放在 `dependencies`，不伪装成包含树。原始未解析需求和完整性作为 properties 保留。

## 回退与核查

回退应整体恢复基线 SCA、三处 SSA 适配、文件系统 regular-file mode 修复、生成清单和 go.mod/go.sum，不能只恢复某个解析器并混用两套身份规则。生产没有并行旧实现开关。基线 Git 对象与隔离审计归档保留旧测试和期望。

执行 `python3 scripts/sca/dependency_audit.py`、`go run scripts/sca/source_audit.go` 和 `CGO_ENABLED=0 GOWORK=off go test ./common/sca/...` 复核。`audit/acceptance.json` 区分已执行门禁与尚未证明的任务书要求；零依赖审计不是完整验收证书。
