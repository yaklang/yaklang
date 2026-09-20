# 本地验收记录

分支 `codex/sca-minimal-dependency-core`，基线 `d33a21b6a30945df93a663a18f3cce087b0f3718`。**核心依赖收缩通过；任务书完整结项尚未通过。** 机器可读状态和证据位置见 `audit/acceptance.json`，对应代码摘要见 `audit/implementation-fingerprint.json`。

## 已执行并通过

- 生产闭包从 704 包 / 91 个第三方模块收缩至 134 包 / 0 个第三方模块；含测试闭包 205 包 / 0 模块。唯一批准的内部叶子已递归审计。
- 20 类分析器保留；881 项测试、子测试及固定 fuzz seed 通过，0 失败，0 跳过测试用例。没有测试文件的包不算跳过用例。
- 无 CGO 的空模块缓存离线测试、race、go vet、五个平台交叉构建通过。交叉构建不冒充五个平台原生运行。
- macOS 禁止网络、写文件和子进程的隔离执行通过；这与 AST 能力检查共同提供证据，尚没有独立 syscall 尝试计数。
- 16 组 RPM BDB/NDB/SQLite 固定材料、上游 reader 全量期望和独立 rpm-qa 期望通过；11 个 fuzz 入口完成短时运行，3 个刻意破坏的不变量被测试捕获。
- Java、PHP、TypeScript 的定向 SSA 及真实 Yak 虚拟文件系统回归通过；两种 CycloneDX 输出通过固定 1.5 schema 的独立离线验证。
- 同名多版本归并与稀疏目录发现完成规模阶梯、时间/分配/峰值 RSS 比较。详见 `SCA_BENCHMARKS.md`，不把内核微基准外推为全部格式性能结论。
- 88 个生产源码、809 个静态样例的摘要和 802 条候选映射的本地目标已校验；许可、上游固定提交和删除清单保留。

## 尚未满足的严格门槛

1. 候选处置清单已经存在，但通用解析器测试与冻结语法的逐 case 等价性尚未全部独立复核，不能宣称相关用例迁移率已经获证 100%。
2. 读入、归档、语法展开及最终输出有界；部分格式 DTO 在输出计数检查前，仍按字节/语法预算分配。语法与解析步数部分按解析器计数，独立的全局结果内存估算预算尚未实现，因此严格的全局分配前预算门槛未通过。
3. 全格式、密集边、循环、多来源的完整旧新性能矩阵，以及逐格式临时文件和被拒绝系统调用尝试的计数尚未完成。

以下证据记录于创建 PR 之前；远端 CI 和提交状态以 PR 为准。以上开放项是实际未完成工作，不是已经执行但未附日志，也不以“零依赖”豁免。

## 复核

```sh
python3 scripts/sca/dependency_audit.py
python3 scripts/sca/dependency_audit.py --test
go run scripts/sca/source_audit.go
go run scripts/sca/source_audit.go common/utils/filesys/filesys_interface
python3 scripts/sca/verify_artifacts.py
GOWORK=off CGO_ENABLED=0 go test ./common/sca/... -count=1
GOWORK=off CGO_ENABLED=0 go vet ./common/sca/...
GOWORK=off CGO_ENABLED=1 go test -race ./common/sca/... -count=1
GOWORK=off CGO_ENABLED=0 go test ./common/sca/... -count=1 -exec "$PWD/scripts/sca/isolated-test-exec.sh"
```

最后一条仅适用于 macOS。来源再生成需要已锁定且位于产品模块外的参考 checkout 与旧基线归档；普通构建、测试及摘要检查不需要这些 checkout，也不下载材料。
