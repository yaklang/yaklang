# 3+4 组合对齐与最终验证报告（3p4-ALIGN-REPORT）

日期：2026-08-13（Asia/Shanghai）
仓库：`/home/wlz/Developer/yaklang-workspace/test-scan-large_projects`（3 号 worktree）
分支：`test/scan/large_projects`
对齐目标：#4 worktree `enhance-ssa-scan_perf2`（分支 `enhance/ssa/scan_perf2`）最终 HEAD `d846dea34`

## 0. 结论摘要

- 3 号已安全对齐到 4 号最终 HEAD `d846dea34d613e529af1e5f5baff778d188ca395`，两 worktree 的 HEAD 与源码树完全一致（对齐后 `git diff d846dea34` 为空、`git status --short` 仅剩未跟踪任务文档）。
- 对齐前完成逐提交审计：3 号全部 24 个提交在 4 号中均有 `=`（identical）对应，4 号额外包含 13 个提交（dbcache 修复/测试、gorm pin、FunctionType 修复、CI 超时等）。
- 3+4 组合 yak 重新构建成功：`go build` 66.1s，二进制 296,727,240 字节，SHA256 `94512a46…`，对应 commit `d846dea34`。
- 测试矩阵：`scripts/ssa-test.sh` 14m09s，除 1 个 grpc 语言补全断言失败外全部通过；`-race` 补充验证发现 ssadb source-filesystem 测试存在 ANTLR Java parser 首次初始化 DATA RACE（非本分支改动，见 §6）。
- 性能验证全部开启 `--debug`（pprof HTTP `127.0.0.1:18080`，CPU/heap/goroutine 快照完整保留）：
  - small（jeepay）Risk=284、medium（tamguo）Risk=493、Hadoop DB-only Risk=8764、Hadoop 完整 compile+scan Risk=9026。
  - Hadoop DB-only 扫描窗口 355s、累计 alloc 188.1GB，相对 main 基线 debug-scan2（679s / 313.3GB）分别为 **-47.7% / -40.0%**。
- 未 push；所有最终 artifacts 保留在 `build/3.1-closure/`；中间实验已按清单移入 `~/.local/share/Trash/files/`（非永久删除）。

## 1. 对齐审计（3 → 4）

### 1.1 对齐前状态

| 项 | 3 号 | 4 号 |
|---|---|---|
| HEAD | `ef67b36a7`（A3 native-SQL ConstType） | `d846dea34`（docs 3+4 audit） |
| 分支 | `test/scan/large_projects` | `enhance/ssa/scan_perf2` |
| merge-base vs origin/main | `f53369df5` | `367e86277`（更新的 main） |

### 1.2 range-diff 结果

`git range-diff origin/main..ef67b36a7 origin/main..d846dea34`：

- 3 号 24 个提交全部 `=` 映射到 4 号对应提交（A1/A2/B/C/D/F×3/O1/O1-revert/O2/O3/A2round/A3round/6 个基础 perf+fix/docs）。
- 4 号额外 13 个提交：
  - `df1fe835b` build(gorm) pin gorm hash
  - `1bf717d5d` fix(ssa) named FunctionType String()
  - `02fb0b8f1` fix(ssaapi) clear old IR rows on recompile
  - `3b2b7b2af` fix(ssaapi/test) sync embedded rules
  - `fcad1921a` / `f2ca24127` / `e0f62eb6d` fix(dbcache) 持久化结算语义
  - `c88eac502` / `4a88ec424` / `cb7598064` / `cbcaed383` 测试/CI
  - `cf4709b2e` / `d846dea34` docs
- 逐提交 patch 对比：23/24 对 patch 完全一致（hash 相同）；`fix(consts) SQLite WAL` 一对改动内容一致（13 文件、+358/-47），仅因父基线不同导致 diff index/行号偏移。
- 关键提交对应（内容一致）：
  - A3：`ef67b36a7` = `a10b96fe6`
  - A2round：`ab09c0f62` = `46e7dfa82`
  - O3：`737e32bc9` = `8c06e0951`
  - O2：`091b4cdb5` = `106e7d29e`
  - O1-revert（含 alias 回归测试）：`d0d2424b7` = `8b9005724`
  - O1：`16acb1b78` = `cc1815a1d`

### 1.3 对齐动作

```sh
git reset --hard d846dea34d613e529af1e5f5baff778d188ca395
```

对齐后验证：`git rev-parse HEAD` == `d846dea34…`；`git status --short` 仅剩未跟踪 `.codex-*` 任务文档；`git diff --check` 通过。

证据文件：
- `build/3.1-closure/alignment-evidence/3-history-before-align-d846dea34.txt`
- `build/3.1-closure/alignment-evidence/range-diff-3-to-4.txt`

## 2. 编译效果（3+4 组合 binary）

构建命令：

```sh
go build -o build/3.1-closure/3p4-build/yak-3p4 ./common/yak/cmd/yak.go
```

| 指标 | 值 |
|---|---:|
| go build wall | 66.068s |
| go build user / sys | 557.455s / 56.264s |
| 二进制大小 | 296,727,240 B（283.0 MiB） |
| SHA256 | `94512a46b13e8e93ed84a371dd7503ed8c62080885aebbc0aa2849bed0962328` |
| binary mtime | 2026-08-13 12:16:16 +0800 |
| HEAD | `d846dea34d613e529af1e5f5baff778d188ca395` |
| HEAD commit time | 2026-08-13 11:28:36 +0800 |
| Go | go1.26.5-X:nodwarf5 linux/amd64 |
| GOCACHE | `/home/wlz/.cache/go-build`（系统默认，未自定义） |

`yak version --json`：`{"BuildTime":"2026-08-13 12:16:50…","GitHash":"-","GoVersion":"go1.26.5-X:nodwarf5","Version":"dev"}`（未注入 ldflags，故 GitHash 为 `-`；以构建记录 + mtime + HEAD 为准）。

构建日志保留：`build/3.1-closure/3p4-build/build.log`、`build2.time`、`sha256sum.txt`、`binary-stat.txt`、`binary-commit.txt`。

## 3. 测试矩阵

### 3.1 `scripts/ssa-test.sh`（项目约定测试入口）

运行方式：`scripts/ssa-test.sh`（内部 `go run ./common/yak/cmd sync-rule` + 各语言/SSA 包 `go test`）。

结果：**wall=848.7s（14m09s），user=4118.1s，sys=344.9s，exit=1**

- SSA 相关全通过：`dbcache`、`ssa`、`ssa/ssadb`、`ssaapi`、`ssaapi/ssareducer`、`ssaapi/test/ssatest`、`ssa_compile`、`syntaxflow/...`、`yakgrpc`（除下述 1 项）。
- 此前记录的两个 ssaapi `TestOffsetFix` 既有失败本次通过。
- 唯一失败（与 SSA/dbcache 改动无关）：
  - `TestGRPCMUSTPASS_LANGUAGE_SuggestionCompletion_Callback/hijackHTTPResponse-rsp-bytes`
  - 断言 `rsp` 应包含 `[]byte` 内置方法，实际返回 sample 方法集合（builtin 方法列表不匹配），属于 grpc 语言补全/内置文档数据问题。

日志：`build/3.1-closure/3p4-test-matrix-20260813-132108.log`

### 3.2 `-race` 补充验证（用户此前要求的并发验证）

```sh
go test -race -timeout 5m ./common/utils/dbcache/... ./common/yak/ssa/...
```

- `dbcache` 全部通过。
- `ssa/ssadb` 的 5 个 source-filesystem 测试报 DATA RACE（`TestSourceFilesysLocal`、`TestSourceFilesys`、`TestProgram_ListAndDelete`、`TestSourceFilesystem_YakURL`、`TestIrSourceFS_File_URL`）。
- 竞态点：并发 compile worker 首次调用 `GetJavaParserSerializedATN` → `javaparserParserInit` 写共享 ATN 字节，另一 worker 的 `ATNDeserializer.readInt` 并发读（`common/yak/java/parser/java_parser.go`）。该文件在 `origin/main..d846dea34` 范围内无任何提交改动，属既有 parser 初始化线程安全问题，非本分支性能改动引入；本轮不修（收尾模式，禁止新改动）。

日志：`build/3.1-closure/3p4-race-20260813-133551.log`

## 4. Debug+pprof 性能验证

统一参数模板（与保留的 main 基线 debug-scan2 同 target/rules/flags）：

```sh
yak code-scan (--db <db> -p hadoop | -t <target>) --debug <run>/debug --log-level info --format sarif -o <run>/report.sarif --rule-timeout 4h --file-perf-log --rule-perf-log
```

说明：
- `--debug <dir>` 为 d846dea34 真实支持的 Debug 选项，自动生成 `<dir>/{log,cmd.txt,ssadb.db,cpu-pprof,memory-pprof,goroutine-pprof}`，pprof HTTP 服务固定 `127.0.0.1:18080`，30s 后首快照、每 5m02s 一轮、退出时 final 快照；heap ≥10GB 时 CPU profile 时长 5 分钟（导致进程退出前有数分钟收尾采样，见各 run 的 wall 与扫描窗口区分）。
- `--file-perf-log`/`--rule-perf-log` 按基线命令保留传入，但 d846dea34 的 `code-scan --help` 与源码中均未定义（无对应 flag 解析），属兼容性 no-op；per-rule 耗时仍完整记录在 stdout（`[RUNNING]/[DONE] duration=…`），per-file 编译耗时在 debug/log 的 `f1_units` 行。
- `time -v` 不可用（系统未装 GNU time），改用 bash `TIMEFORMAT` 记录 wall/user/sys + 每秒 `/proc` RSS 采样记录峰值。

### 4.1 small：jeepay（compile+scan）

- run：`build/3.1-closure/3p4-small-20260813-121815`
- exit=0，进程 wall=104s
- Risk=284（基线 284 不变）、SARIF ruleId=284
- RSS 峰值 1,197,348 KB（1.14 GiB）
- 最终 heap inuse 456.6MB；CPU 热点：GC `gcBgMarkWorker` cum 71.8%（`scanObjectsSmall` flat 22.5%），整体 GC-bound
- pprof：initial + final CPU/heap/goroutine 各 2 份

### 4.2 medium：探果 tamguo（compile+scan）

- run：`build/3.1-closure/3p4-medium-20260813-122029`
- exit=0，进程 wall=118s
- Risk=493（旧基线区间 489-494，编译非确定性范围内）、SARIF ruleId=493
- RSS 峰值 1,218,240 KB（1.16 GiB）
- pprof：initial + final CPU/heap/goroutine 各 2 份

### 4.3 Hadoop DB-only（与 main 基线 debug-scan2 同 DB/rules）

- run：`build/3.1-closure/3p4-hadoop-dbonly-20260813-122350`
- DB：`build/hadoop-run19/ssa.db` 的 fresh copy（同基线输入）
- exit=0；进程 wall=649s（含收尾 5 分钟 CPU pprof 采样）
- 扫描窗口：scan stage 12:24:31 → 最后 Risk 12:30:26 = **355s**
- Risk=8764（不变）、SARIF ruleId=8764、Failed=0/Skipped=142/Success=127/Total=269
- RSS 峰值 14,737,688 KB（14.05 GiB）
- 累计 alloc（final alloc_space）：**188,077 MB**
- pprof：CPU/heap/goroutine 各 3 份（initial/12:29:37/final）

#### 与 main 基线对比（debug-scan2，2026-08-11）

| 指标 | main 基线 debug-scan2 | 3+4 DB-only | 变化 |
|---|---:|---:|---|
| 扫描窗口 | 679s（00:30:06→00:41:25） | 355s（12:24:31→12:30:26） | **-47.7%** |
| 累计 alloc_space | 313,343 MB | 188,077 MB | **-40.0%** |
| Risk | 8764 | 8764 | 不变 |
| Failed/Skipped/Success/Total | 0/142/127/270 | 0/142/127/269 | 一致 |

与 3 号 A3 run（`a3-hadoop-20260812-174953`，扫描窗口 400s、alloc 188.9GB）相比：本次 355s / 188.1GB，均略优。

规则预算说明：`检测Java 日志伪造攻击` 命中 work-limit（auto-scaled 200000→1000000，visited=1000021）duration=355.1s，返回 partial results；A3 同规则同样命中（visited=1000011，duration=379.1s），行为一致。

### 4.4 Hadoop 完整 compile+scan（编译内存证据）

- run：`build/3.1-closure/3p4-hadoop-compile-20260813-123653`
- target：`/home/wlz/Target/apache/hadoop`（fresh DB）
- exit=0，进程 wall=2586s（43m06s）
- 编译窗口：compile stage 12:37:23 → `f4_finish` 13:06:46 = **1763s（29m23s）**；末批 persist 13:07:09
- 扫描窗口：13:07:09 → 最后 Risk 13:19:24 = **735s**（含规则装载；work-limit 日志 13:11:42 → 13:19:24 = 462s）
- Risk=9026、SARIF ruleId=9026、Failed=0/Skipped=142/Success=127/Total=269
- RSS 峰值 26,791,052 KB（25.55 GiB）
- 累计 alloc（final alloc_space）：**1,047,890 MB**
- pprof：CPU/heap/goroutine 各 9 份（initial + 7 周期 + final）

对比参考：main run19 完整 compile+scan（`build/hadoop-run19/debug`，2026-08-10）编译窗口约 1669s（23:01:46→23:29:35），最后一个 memory snapshot alloc 1,003,629 MB（23:37:31；run 最终 report.sarif 为 0 字节，无最终 Risk，不能作为完整闭环基线）。本报告将 run19 完整 run 仅作参考，不据此宣布 compile 收益。

### 4.5 热点与差异（pprof 实测）

Hadoop DB-only（alloc_space，final）：

| 热点 | 3+4 数值 | 备注 |
|---|---:|---|
| `BitVector.Or` | 33,009 MB | O1 回退后遗留（clone-detach），C 档所有权修复另开分支 |
| `ssaapi.init.func23`（valuePool） | 21,576 MB | 已知评估拒绝项 |
| `TakeSymbolSnapshot.func1` | 9,330 MB | 基线 42.3GB → **-78%** |
| `BitVector.ensure` | 5,930 MB | COW 相关 |
| GORM `search.clone`+`DB.clone` | 8,820 MB | 基线 ~45GB → **-80%** |
| `nativeGetIrCodesByIds` | 9,553 MB cum | native 批量读路径 |
| sqlite `GoStringN` | 5,873 MB | 编译期读取 |

Hadoop compile+scan：

- 编译期 CPU（60s profile @12:52）：GC 扫描占优（`spanClass.sizeclass` flat 15.6%、`scanObjectsSmall` 13.5%、`cgocall` 13.5%），编译期小对象分配密集。
- 扫描期 CPU（300s profile @13:12）：GC 后台标记 cum ~56%（`scanSpan`/`tryDeferToSpanScan`），`ssaapi.(*Value).getInstruction` cum 340s（4.6%），仍是 GC-bound。
- alloc 大头：ANTLR `NewATNConfig` flat 143.7GB（编译期解析），其次 GC/JSON/GORM。

## 5. 保留/清理清单

### 保留（最终证据）

| 路径 | 说明 |
|---|---|
| `build/hadoop-run19/` | main 基线（debug-scan2 pprof/日志/report + 6.2GB ssa.db + run19 compile 日志） |
| `build/3.1-closure/FINAL-REPORT.md` | 3.1 最终报告 |
| `build/3.1-closure/round-a2-report.md` / `round-a3-report.md` | A2/A3 轮次报告 |
| `build/3.1-closure/PLAN-CLOSURE-AUDIT.md` / `next-optimization-report.md` | 收尾审计与后续建议 |
| `build/3.1-closure/alignment-evidence/` | 对齐前历史 + range-diff |
| `build/3.1-closure/3p4-build/` | 3+4 binary + 构建日志/SHA |
| `build/3.1-closure/3p4-small-…121815` / `3p4-medium-…122029` | small/medium 最终验证 |
| `build/3.1-closure/3p4-hadoop-dbonly-…122350` / `3p4-hadoop-compile-…123653` | Hadoop DB-only / compile+scan 最终验证 |
| `build/3.1-closure/3p4-test-matrix-*.log` / `3p4-race-*.log` | 测试矩阵/race 日志 |
| `build/3.1-closure/3p4-ALIGN-REPORT.md` | 本报告 |

### 移入 Trash（`~/.local/share/Trash/files/`，非永久删除）

| 路径 | 大小 | 理由 |
|---|---:|---|
| `build/3.1/` | 6.9G | 3.1 中间实验（REPORT/run 目录/旧 binary） |
| `build/core-run/` | 4.0G | 旧 core 实验 |
| `build/grav-run/` | 247M | 旧 grav 实验 |
| `build/DVWA/` | 65M | 旧 DVWA 分析产物 |
| `build/go-tmp/` | 4K | 空/临时 |
| `build/3.1-closure/a2-*`（small/medium/hadoop） | 6.8G | A2 轮次中间 run |
| `build/3.1-closure/a3-*`（small/medium/hadoop） | 6.7G | A3 轮次中间 run |
| `build/3.1-closure/hadoop-20260812-055920` | 6.4G | closure 中间 run |
| `build/3.1-closure/medium-20260812-045125` | 296M | 阻塞/中间 run |
| `build/3.1-closure/medium2-20260812-055237` | 174M | 中间 run |
| `build/3.1-closure/small-20260812-044900` | 145M | 中间 run |
| `build/3.1-closure/yak` / `yak-a2` / `yak-a3` | 850M | 旧 binary（3p4-build/yak-3p4 替代） |
| `build/3.1-closure/3p4-small-20260813-121742` | 12K | 首次包装脚本失败的残留空目录 |

未跟踪任务文档 `.codex-*.md`、`build-trash-list.txt` 按用户此前要求保留，不删除。

## 6. 已知问题与限制

1. **ssadb source-filesystem 测试 -race 失败（ANTLR parser 首次初始化竞态）**：`common/yak/java/parser/java_parser.go` 在本分支提交范围内零改动，判定为既有问题；收尾模式不修。
2. **grpc 语言补全断言失败 1 项**：builtin 方法列表不匹配，与 SSA/dbcache 改动无关。
3. **compile+scan 与 DB-only Risk 不同（9026 vs 8764）**：compile+scan 用新 binary 重新编译出 fresh IR，规则执行路径更完整（Skipped 相同但部分规则多产出）；DB-only 与 main 基线 debug-scan2 同输入同口径，为官方对比口径。
4. **run19 完整 compile+scan 基线不可闭环**：其 report.sarif 为 0 字节、日志无最终 Risk，故 compile 对比仅作参考。
5. **`--file-perf-log`/`--rule-perf-log` 在当前 binary 为 no-op**：源码未定义对应 flag；per-rule/per-file 数据以 stdout/debug log 为准。
6. **进程 wall 与扫描窗口差异**：heap ≥10GB 时 pprof 收尾会跑 5 分钟 CPU 采样，DB-only 进程 wall 649s 中含约 294s 收尾采样；报告统一用“扫描窗口”作性能对比。

## 7. 收尾状态

- `git diff --check`：通过。
- `gofmt`：本次未修改任何 Go 源码，无需格式化。
- `git status`：提交本报告前仅有未跟踪任务文档；提交后 HEAD 将包含 1 个 docs 提交（本报告），源码树与 #4 保持一致。
- 不 push。
