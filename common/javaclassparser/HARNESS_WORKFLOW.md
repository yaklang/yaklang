# Java 反编译长尾清零工作流 (HARNESS_WORKFLOW)

> 目标: 把 `.m2` 真实 jar 语料上的 `partial` / `syntax` / `err` / `panic` 逐个清零。
> 配套基准（中/英）: [`JAVA_DECOMPILER.zh-CN.md`](./JAVA_DECOMPILER.zh-CN.md) / [`JAVA_DECOMPILER.md`](./JAVA_DECOMPILER.md)
>
> 适用任意承载这项工作的分支；本文不绑定具体分支名。
>
> **终极目标 = 整个 `~/.m2` 全部清零**（不止前 120 个 jar / a-c 前缀）。默认 `M2_MAX_JARS=120`
> 只扫字典序前 120 个 jar（约 a–flexmark 前缀），覆盖 spring/tomcat/netty/... 等需用全量扫描
> （见 §1 全量命令）。每轮推进记下当前最前失败点（`class N @ <jar>`），下一轮从同一处继续。

本文件约束后续 harness（无论人工还是自动 agent）的工作方式。**这是清零长尾问题的唯一推荐路径：一次只盯一个 class，修一个、锁一个、再扫下一个。** 严禁批量乱改、严禁跳过验证、严禁在没有定位到具体失败 class 前先动核心代码。

---

## 0. 总原则：遇到第一个问题 jar 立即停手

运行大型 `.m2` 扫描时，**不要一口气扫完所有 jar**。harness 是按 jar 名字典序确定性扫描的，所以"第一个出问题的 jar / class"是稳定可复现的。

- 一旦扫描中出现第一个 `partial` / `syntax` / `err` / `panic`，**立即停止扫描，转入修复**。
- 不要积攒一堆问题再批量改 —— 每个长尾缺陷的根因往往不同，批量改会互相掩盖、引入回归。
- 修复的最小闭环是一个 **class**，不是一个 jar、更不是一个 reason 桶。
- 单次迭代严格遵守 §1 → §2 → §3 → §4 的顺序，禁止跳步。

每轮迭代只解决一个 class，并以"本地回归 30s 内全绿"作为收尾闸门。

---

## 1. 定位：找到可疑 jar，定位无法编译的 class

### 1a. 快速定位第一个失败 class（迭代首选，秒级）

逐个清零时，**绝大多数迭代用这一条**：扫到第一个失败 class 就立即停手并落盘，并打印一条可直接复制的 `DIAG_FILE` 复现命令。这把每轮从"分钟级全量扫"压成"秒级定位"。

```bash
STUB_REASONS=1 STOP_ON_FIRST=1 \
M2_MAX_JARS=120 M2_MAX_CLASSES=24491 \
PROBLEM_DIR=/tmp/jdec-problems PROGRESS_EVERY=0 \
M2_PROGRESS_FILE=/tmp/jdec-progress/state.env \
go test -run TestM2StubReasons -v ./common/javaclassparser/tests/
# stderr 末尾会给出:
#   [stub-reasons] STOP_ON_FIRST: aborted after first failure at class <N>
#   [stub-reasons]   class: <jar>!<class>
#   [stub-reasons]   bucket dir: /tmp/jdec-problems/<bucket>
#   [stub-reasons]   reproduce: DIAG_FILE=<bucket>/*.class go test -run TestDiagDecompileClass ...
```

- `STOP_ON_FIRST=1`：遇到第一个 `partial`/`err`/`panic` 立即保存并退出（按 §0 原则）。语料全清时会扫完整段范围并打印 `no failure found`，所以它也是"是否清零"的探针。
- 语料已清零想确认时：重跑同一条，看到 `no failure found in scanned range` 即代表本范围 0 失败。
- 想覆盖 spring/tomcat/netty 等非 a-c 前缀 jar：追加 `M2_INDUSTRY=1`（每 jar 上限 `M2_MAX_PER_JAR`，默认 200）。
- `M2_PROGRESS_FILE=/tmp/jdec-progress/state.env`：每轮自动记录当前失败点、bucket、单类复现命令，以及续跑命令。优先使用 jar 级命令：`rerun_from_failure_jar` 从失败 jar 开头复核；`continue_after_locked_jar` 直接跳过已处理 jar 继续扫。class 级命令 `rerun_from_failure` / `continue_after_locked` 只在需要同 jar 内精确复查时使用。
- 手动续跑优先用 `M2_START_JAR=<jar>` 从某个 jar 开始（包含该 jar），或 `M2_RESUME_AFTER_JAR=<jar>` 直接跳过某个 jar（不再打开前面的 jar，速度最快）。也可以用 `M2_START_JAR_INDEX=<N>` + `M2_START_JAR_END=<M>` 限定 jar 字典序下标闭区间（1 基，包含两端），便于多进程分片扫描。只有需要定位到同一个 jar 内某个 class 后面时，才用 `M2_RESUME_AFTER_CLASS=<N>` 或 `M2_RESUME_AFTER=<jar>!<class>`。

### 1b. 全量分桶 + 计数（阶段性盘点，分钟级）

需要看全部失败按 reason 如何分布、或前后对比 partial 是否下降时，跑一次完整扫描（**不要**在迭代中频繁用，太慢）：

```bash
# 在仓库根目录执行
STUB_REASONS=1 \
M2_MAX_JARS=120 M2_MAX_CLASSES=24491 \
PROBLEM_DIR=/tmp/jdec-problems \
M2_PROGRESS_FILE=/tmp/jdec-progress/state.env \
go test -run TestM2StubReasons -v ./common/javaclassparser/tests/
```

- 想覆盖 spring/tomcat/netty 等非 a-c 前缀的 jar，加 `M2_INDUSTRY=1`（每 jar 上限 `M2_MAX_PER_JAR`，默认 200）。
- 想要纯计数 + 每 class 指纹（用于前后对比 partial 数量是否下降），用：

```bash
M2_OUT=/tmp/m2-before.txt M2_MAX_JARS=120 M2_MAX_CLASSES=12000 \
go test -run TestM2RegressionHarness -v ./common/javaclassparser/tests/
# 文件首行: # jars=.. classes=.. ok=.. partial=.. syntax=.. err=..
```

**选定本轮要修的那一个 class：**

- 优先级: `panic` > `err` > `syntax` > `partial`（越靠前危害越大）。
- 同优先级内，从 `/tmp/jdec-problems/<bucket>/` 里挑**字节数最小**的 `.class`（复现最快、根因最干净）。
- bucket 目录名形如 `partial__<reason_slug>` / `err__<reason_slug>`；目录内同时有 `.class`（原始字节码）和 `.java` / `.err.txt`（当前反编译产物，含 `/* yak-decompiler: <reason> */` 标记，直接指明哪个方法/字段退化了）。

确认是哪个 class、哪个方法无法重建后，**单类复现**（这是后续调试与回归的锚点）：

```bash
DIAG_FILE=/tmp/jdec-problems/<bucket>/<name>.class \
go test -run TestDiagDecompileClass -v ./common/javaclassparser/tests/
```

也可直接从 jar 里按子串挑类复现：

```bash
DIAG_JAR=<相对 ~/.m2 或绝对路径>.jar DIAG_CLASS=<类名子串> \
go test -run TestDiagDecompileClass -v ./common/javaclassparser/tests/
```

---

## 2. 修复：改 class 对应的根因，再跑这个 jar 的测试

1. 在 `common/javaclassparser/` 的核心反编译代码里**针对根因**修复（结构化 / 栈模拟 / 渲染等），不要为了过单个用例打特例补丁。
2. 修复时坚持反编译器的安全契约：
   - 永远不输出无法解析的 Java；宁可退化成带标记的 stub，也不能输出"看起来对但其实错"的代码。
   - `Decompile` 不许 panic、不许返回 error 逃逸；无法重建的成员退化为 `yak-decompiler:` stub。
   - 复杂改动保留可回退开关（参照已有的 `JSR_INLINE_OFF` / `EnableLegacyMergeReconstruction` 等 kill-switch 风格）。
3. 改完先用 §1 的 `DIAG_FILE` 单类复现，确认该 class 现在**完整反编译且无 stub 标记、能过 frontend**。
4. 再把这个 class 所在的 **jar** 重新跑一遍，确认本次修复没有把同 jar 内其他 class 改坏：

```bash
# 把扫描范围缩到目标 jar 所在的小窗口（字典序），快速验证整个 jar
STUB_REASONS=1 M2_INDUSTRY=1 M2_MAX_PER_JAR=100000 \
DIAG_JAR=<目标 jar> \
go test -run TestM2StubReasons -v ./common/javaclassparser/tests/
```

> 说明: harness 没有"只测单 jar"的专用入口；实践中用 `DIAG_JAR` 单类逐个验证该 jar 内此前失败的类，或用 `M2_MAX_JARS` 把窗口压到刚好覆盖目标 jar。重点是确认目标 class 修好、同 jar 邻居没退化。

---

## 3. 锁定：把修过的 class 挪进本地回归集，跑 30s 内的快回归

把刚修好的那个 `.class` 固化成**永久回归用例**，这样 CI（无 `~/.m2`）也能守住这个修复永不回归。

1. 复制原始字节码进回归数据集（用能说明问题的语义化文件名）：

```bash
cp /tmp/jdec-problems/<bucket>/<name>.class \
   common/javaclassparser/tests/testdata/regression/<语义化名>.class
```

2. 在对应的回归测试里加一条用例（三者都从 `//go:embed testdata/regression/*.class` 的 `regressionFS` 读取）：
   - 语法 / 完整重建类问题 → `tests/regression_test.go` 的 `TestDecompileSyntaxRegression`（填 `mustContain` / `mustNotContain`，至少断言不含 `yak-decompiler`）。
   - 之前会 panic 的边界类 → `tests/ga_panic_free_test.go` 的 `TestGAPanicFreeBoundary`（`wantFull` 标记是否要求无 stub）。
   - 需要断言具体语义结构（如开关 case 顺序、字面量符号、循环极性等）→ 仿照 `regression_test.go` 里写一个独立 `Test...` 函数。

3. 跑**本地快回归**，必须 30s 内全绿（最快反馈）：

```bash
# 整个包约 22s，确定性、无外部依赖、无网络、无需 ~/.m2
go test ./common/javaclassparser/...
```

如需更快的针对性子集：

```bash
go test -run 'TestDecompileSyntaxRegression|TestGAPanicFreeBoundary|TestSyntaxCoverageMatrix|TestRecompileRoundtrip|TestDecompile' \
  ./common/javaclassparser/tests/
```

4. 收尾闸门（全部满足才算本轮完成）：
   - 新增回归用例通过；
   - `TestSyntaxCoverageMatrix` / `TestRecompileRoundtrip` 合成语料仍 0 stub / 0 round-trip 失败；
   - 整包 `go test ./common/javaclassparser/...` 全绿且 ≤ 30s；
   - 在 [`JAVA_DECOMPILER.md`](./JAVA_DECOMPILER.md) 追加一节，记录本轮根因、修复点、同配置 before/after 的 `partial`/`stubs`/`panic` 计数（用真实数据，禁止编造）。

---

## 4. 循环：扫下一个 jar，进入下一轮

回到 §1 重新跑 `.m2` 扫描（可对比 §1 落盘的 `M2_OUT` 计数确认 `partial` 在下降、`syntax`/`err`/`panic` 没反弹），找到**下一个**失败 class，重复 §1 → §2 → §3。

- 每轮只清一个 class，稳步把长尾压向 0。
- 若某 reason 桶里多个 class 根因相同，先修最小的一个并加回归，再扫验证整桶是否随之归零（通常会），不要为每个同根因 class 各打一个特例补丁。
- 计数对比命令：

```bash
M2_OUT=/tmp/m2-after.txt M2_MAX_JARS=120 M2_MAX_CLASSES=12000 \
go test -run TestM2RegressionHarness -v ./common/javaclassparser/tests/
diff <(head -1 /tmp/m2-before.txt) <(head -1 /tmp/m2-after.txt)
```

---

## 速查表

| 目的 | 命令 |
|------|------|
| **迭代首选：秒级定位第一个失败类** | `STUB_REASONS=1 STOP_ON_FIRST=1 M2_MAX_JARS=120 M2_MAX_CLASSES=24491 PROBLEM_DIR=/tmp/jdec-problems PROGRESS_EVERY=0 M2_PROGRESS_FILE=/tmp/jdec-progress/state.env go test -run TestM2StubReasons -v ./common/javaclassparser/tests/` |
| 续跑到下一个未处理 class | 使用 `/tmp/jdec-progress/state.env` 中的 `continue_after_locked`，或手动加 `M2_RESUME_AFTER_CLASS=<N>` / `M2_RESUME_AFTER=<jar>!<class>` |
| 续跑到下一个未处理 jar（首选，最快） | 使用 `/tmp/jdec-progress/state.env` 中的 `continue_after_locked_jar`，或手动加 `M2_RESUME_AFTER_JAR=<jar>` |
| 从某个 jar 开始复核（包含该 jar） | `M2_START_JAR=<jar> STUB_REASONS=1 STOP_ON_FIRST=1 ... go test -run TestM2StubReasons -v ./common/javaclassparser/tests/` |
| 扫描 jar 下标闭区间（多进程分片） | `M2_START_JAR_INDEX=50 M2_START_JAR_END=200 STUB_REASONS=1 STOP_ON_FIRST=1 M2_INDUSTRY=1 PROBLEM_DIR=/tmp/jdec-50-200 M2_PROGRESS_FILE=/tmp/jdec-progress/50-200.env go test -run TestM2StubReasons -v ./common/javaclassparser/tests/` |
| 调整单进程 jar 并行数 | 默认 `M2_CONCURRENT_JARS=4`；需要降噪/排查时设 `M2_CONCURRENT_JARS=1`，CPU 余量足时可试 `M2_CONCURRENT_JARS=8` |
| 大扫描 + 失败类落盘分桶（阶段性盘点） | `STUB_REASONS=1 M2_MAX_JARS=120 M2_MAX_CLASSES=24491 PROBLEM_DIR=/tmp/jdec-problems M2_PROGRESS_FILE=/tmp/jdec-progress/state.env go test -run TestM2StubReasons -v ./common/javaclassparser/tests/` |
| 计数 + 每类指纹（前后对比） | `M2_OUT=/tmp/m2.txt M2_MAX_JARS=120 M2_MAX_CLASSES=12000 go test -run TestM2RegressionHarness -v ./common/javaclassparser/tests/` |
| 单类复现（文件） | `DIAG_FILE=<path>.class go test -run TestDiagDecompileClass -v ./common/javaclassparser/tests/` |
| 单类复现（jar+子串） | `DIAG_JAR=<jar> DIAG_CLASS=<substr> go test -run TestDiagDecompileClass -v ./common/javaclassparser/tests/` |
| 覆盖全语料（含 spring 等） | 上述命令追加 `M2_INDUSTRY=1`（每 jar 上限 `M2_MAX_PER_JAR`，默认 200） |
| **全 `~/.m2` 清零扫描（所有 jar）** | `STUB_REASONS=1 M2_INDUSTRY=1 M2_MAX_CLASSES=1000000 M2_MAX_PER_JAR=1000000 PROBLEM_DIR=/tmp/jdec-all PROGRESS_EVERY=1000 go test -run TestM2StubReasons -v ./common/javaclassparser/tests/`（`M2_INDUSTRY=1` 不再截断到前 120 jar；放开两个上限扫全部 ~76 万 class） |
| 本地快回归（≤30s，主闸门） | `go test ./common/javaclassparser/...` |
| 回归数据集目录 | `common/javaclassparser/tests/testdata/regression/*.class` |
| 回归用例落点 | `tests/regression_test.go`（语法/完整）、`tests/ga_panic_free_test.go`（panic） |

## 后台扫描 + 前台并行修复（加速长尾清零）

全量 `.m2` 扫描是分钟级任务，等它结束再修会大量浪费时间。推荐的工作方式是**后台跑全量收集、前台并行逐个修**，互不阻塞：

- **后台启动全量收集**（一次性把所有失败 class 落盘分桶，前台立刻继续干活）：
  ```bash
  nohup env STUB_REASONS=1 M2_MAX_JARS=120 M2_MAX_CLASSES=24491 PROBLEM_DIR=/tmp/jdec-all-problems PROGRESS_EVERY=1000 \
    go test -run TestM2StubReasons -v ./common/javaclassparser/tests/ > /tmp/scan-all.log 2>&1 &
  # 随时查看进度：  tail -f /tmp/scan-all.log
  # 看已落盘的失败类：  ls /tmp/jdec-all-problems/
  ```
  - 后台扫描把每个失败 class 原始 `.class` + 当前 `.java`/`.err.txt` 写进 `/tmp/jdec-all-problems/<bucket>/`，**前台直接拿这些已落盘的 class 逐个复现修复，不必等扫描跑完**。
  - 不要因为扫描慢就卡住：扫描只是"仓库"，修复节奏完全由前台驱动。

- **记进度、快速复定位**：harness 按 jar 名字典序确定性扫描，"第一个出问题的 class 编号 / jar"是稳定可复现的。设置 `M2_PROGRESS_FILE=/tmp/jdec-progress/state.env` 后，harness 会自动写入当前最前失败点（如 `first_failure_class_index=3923`）、bucket、`DIAG_FILE` 复现命令，以及 `rerun_from_failure` / `continue_after_locked` 两条续跑命令。修好并锁定一个 case 后，使用 `continue_after_locked` 从下一个未处理 class 继续扫；下一个失败点应当**向后**推进（编号变大），可据此判断是否真有进展。

- **优先用 jar 级续跑**：class offset 会先打开 jar、遍历 class，跳过大前缀时仍然慢。进度文件里的 `rerun_from_failure_jar` / `continue_after_locked_jar` 使用 `M2_START_JAR` / `M2_RESUME_AFTER_JAR`，能在打开 zip 前跳过整个 jar，后续长跑默认优先用这两个命令。只有要在同一个 jar 内从某个 class 之后精确继续，才退回 class 级游标。

- **单进程 jar 并行**：`TestM2StubReasons` 默认 `M2_CONCURRENT_JARS=4`，即同一个 `go test` 进程内最多同时扫描 4 个 jar；worker 只做 jar/class 反编译，主 goroutine 仍按 jar 字典序聚合计数、落盘和 `STOP_ON_FIRST`，所以“第一个失败”仍按扫描顺序决定。调参时在本机 `100-160` 窗口、400 个 class 上对比过 `1/2/4/8`，`4` 略优且不会过度抢 CPU；需要排查并发相关问题时设置 `M2_CONCURRENT_JARS=1` 可回到串行。

- **a-z 分片多进程扫描**：需要更快摸底时，可以继续改用多个独立 `go test` 进程按 jar basename 前缀分片。每个进程设置不同 `M2_JAR_PREFIXES`、`PROBLEM_DIR` 和 `M2_PROGRESS_FILE`，互不共享状态。例如：
  ```bash
  M2_JAR_PREFIXES=a-f STUB_REASONS=1 STOP_ON_FIRST=1 M2_INDUSTRY=1 PROBLEM_DIR=/tmp/jdec-af M2_PROGRESS_FILE=/tmp/jdec-progress/af.env go test -run TestM2StubReasons -v ./common/javaclassparser/tests/
  M2_JAR_PREFIXES=g-l STUB_REASONS=1 STOP_ON_FIRST=1 M2_INDUSTRY=1 PROBLEM_DIR=/tmp/jdec-gl M2_PROGRESS_FILE=/tmp/jdec-progress/gl.env go test -run TestM2StubReasons -v ./common/javaclassparser/tests/
  M2_JAR_PREFIXES=m-r STUB_REASONS=1 STOP_ON_FIRST=1 M2_INDUSTRY=1 PROBLEM_DIR=/tmp/jdec-mr M2_PROGRESS_FILE=/tmp/jdec-progress/mr.env go test -run TestM2StubReasons -v ./common/javaclassparser/tests/
  M2_JAR_PREFIXES=s-z STUB_REASONS=1 STOP_ON_FIRST=1 M2_INDUSTRY=1 PROBLEM_DIR=/tmp/jdec-sz M2_PROGRESS_FILE=/tmp/jdec-progress/sz.env go test -run TestM2StubReasons -v ./common/javaclassparser/tests/
  ```
  为了公平地覆盖字母段，不要总从 `a` 开始。每轮可以随机选一个前缀段先跑，例如 `python3 -c 'import random; print(random.choice(["a-f","g-l","m-r","s-z"]))'` 选出本轮首段；修完首段失败后，再扫下一个随机段。

- **jar 下标窗口分片**：当字母前缀分布不均、或想精确切连续范围时，使用 `M2_START_JAR_INDEX` / `M2_START_JAR_END`。下标基于排序后的 jar 列表，先应用 `M2_JAR_PREFIXES` 过滤，再按 1 基闭区间扫描；显式设置窗口后不会再受默认 `M2_MAX_JARS=120` 截断影响。多个进程要使用不同的 `PROBLEM_DIR` 和 `M2_PROGRESS_FILE`；每个进程内部仍默认最多 4 个 jar 并行：
  ```bash
  M2_START_JAR_INDEX=10 M2_START_JAR_END=20 STUB_REASONS=1 STOP_ON_FIRST=1 M2_INDUSTRY=1 PROBLEM_DIR=/tmp/jdec-10-20 M2_PROGRESS_FILE=/tmp/jdec-progress/10-20.env go test -run TestM2StubReasons -v ./common/javaclassparser/tests/
  M2_START_JAR_INDEX=100 M2_START_JAR_END=110 STUB_REASONS=1 STOP_ON_FIRST=1 M2_INDUSTRY=1 PROBLEM_DIR=/tmp/jdec-100-110 M2_PROGRESS_FILE=/tmp/jdec-progress/100-110.env go test -run TestM2StubReasons -v ./common/javaclassparser/tests/
  ```
  如果同时启动多个独立窗口，建议把单进程内部并发降到 `M2_CONCURRENT_JARS=1`，并按机器余量设置 `GOMAXPROCS=2` 或 `GOMAXPROCS=3`。原因是 `Decompile` 的语法安全网会调用 ANTLR 解析，超时后也只能放弃等待，底层解析 goroutine 仍会继续跑完；外层多进程再叠加内部 jar 并发，容易出现 CPU/内存过度抢占，日志还会因为有序聚合等待窗口首个慢 jar 而看起来“不动”。遇到 Saxon 这类大 jar 时，优先把它拆成单 jar 小窗口，例如：
  ```bash
  GOMAXPROCS=2 M2_START_JAR_INDEX=876 M2_START_JAR_END=876 M2_CONCURRENT_JARS=1 STUB_REASONS=1 STOP_ON_FIRST=1 M2_INDUSTRY=1 PROBLEM_DIR=/tmp/jdec-876 M2_PROGRESS_FILE=/tmp/jdec-progress/876.env go test -run TestM2StubReasons -v ./common/javaclassparser/tests/
  GOMAXPROCS=2 M2_START_JAR_INDEX=877 M2_START_JAR_END=1200 M2_CONCURRENT_JARS=1 STUB_REASONS=1 STOP_ON_FIRST=1 M2_INDUSTRY=1 PROBLEM_DIR=/tmp/jdec-877-1200 M2_PROGRESS_FILE=/tmp/jdec-progress/877-1200.env go test -run TestM2StubReasons -v ./common/javaclassparser/tests/
  ```

- **单 jar 单独测**（确认本 jar 内修复没把邻居改坏、或专攻某个 jar）：
  ```bash
  STUB_REASONS=1 M2_INDUSTRY=1 M2_MAX_PER_JAR=100000 DIAG_JAR=<相对 ~/.m2 或绝对路径>.jar \
    go test -run TestM2StubReasons -v ./common/javaclassparser/tests/
  ```

## 进度信息不在这里保存

## 遇到难 case 的通用解题法

长尾里有些 class 是反编译器最难的结构化问题（do-while(true)+continue、switch 里跨分支共享操作数栈、值合并的极端形状等）。碰到"看上去无路可走"的 case 时，不要死磕单条路径，综合用下面这些方法：

- **找上游源码对照（先拿到"正确答案"）**：失败 class 通常来自知名开源库（druid、logback、spring、guava…）。从失败 class 的 jar 名 + 类全限定名，去该库的 GitHub 仓库（优先用对应版本 tag，如 `alibaba/druid` 的 `druid-1.2.23`；找不到精确 tag 就退 `master`/`main`）拉原始 `.java` 源，定位到出错方法的**原始写法**。有了"教科书正确输出"，再对照反编译器当前产物，一眼就能看出是哪个结构（for+break、if-else-if、instanceof 链、值-merge）没结构化对——比对着乱码猜根因可靠一个数量级。拉取可直接 `curl https://raw.githubusercontent.com/<owner>/<repo>/<tag>/<path>`，或用 node/fetch。
- **核心重构回归时反查测试 oracle**：改 `CalcOpcodeStackInfo`、slot/varTable 合流、`rewriteVar`、variable-fold 这类核心算法后，出现回归不能立刻认定"新算法一定错"。旧测试可能只是锁住了历史错误输出，或断言过窄、不符合真实 Java 语义。此时必须求真：先用 `javap -c -v` 看字节码真实控制流和局部变量槽；本机若有 CFR / FernFlower / Vineflower 等成熟反编译器，就对同一个 `.class` 生成第三方 oracle；没有这些工具时，用上游源码 oracle 和最小合成样本补足。只有确认新输出偏离字节码语义、源码 oracle 或成熟反编译器共识时，才回滚核心改动；如果是测试不科学，应修正测试，让测试表达语义不变量，而不是继续固定错误文本。
- **第三方反编译器必须实际跑一遍**：难 case 不只看我们自己的输出。对落盘的 `DIAG_FILE`，至少用 CFR 和 Vineflower 各跑一次，观察成熟反编译器如何处理同一段控制流、slot 复用、switch/try/loop 结构：
  ```bash
  cfr /tmp/jdec-problems/<bucket>/<case>.class --outputdir /tmp/jdec-oracle/cfr
  vineflower /tmp/jdec-problems/<bucket>/<case>.class /tmp/jdec-oracle/vineflower
  ```
  若第三方也退化或输出异常，优先看 `javap -c -v` 和上游源码；若第三方稳定重建，优先对照它的控制结构找我们的 CFG/栈模拟偏差。
- **合成数据构造**：从失败 class 的字节码里提取出最小的失败模式（一段 `dup_x1/dup2_x1/swap`、一个带 `continue` 的 `do-while(true)`、一个跨 if-merge 的值），手写一个等价的最小 Java 源，`javac` 编译成 `.class` 当回归种子。最小可复现样本能把"500 字节码大方法"压缩成几十字节，根因一眼可见，回归也更快。
- **搜索论文与各类知识**：操作数栈合并 / 值-merge 三元树 / switch 分发结构化 / `continue`-`break` 反循环展开，都有成熟研究（CFE/ASTRÉ、Procyon、CFR、Vine、Soot 的 `Body` 重构）。先弄清楚这类模式的"教科书正确输出"长什么样，再对照反编译器当前产物找偏差，比盲改可靠得多。
- **构建 MVP**：对拿不准的结构化改动，先在一个最小合成样本上验证"这样改能不能产出语法正确、语义贴近的结果"，确认无误再往核心代码里落。MVP 能把高风险重构的爆炸半径锁死在一个文件里。
- **诚实取舍**：某些编译器合成的"反人类"模式（Groovy 的 `selectConstructorAndTransformArguments`、Kotlin 的协程状态机）确实极难干净结构化；如果根因证实是这类、且安全契约（不 panic、不出无法解析的 Java、退化必带 `yak-decompiler:` 标记）已经满足，则一次干净 stub 是可接受交付，记录根因后跳到下一个 case，不要无限堆砌补丁。

## 红线

- 以认真查阅为荣：动核心代码前，必须先用 `DIAG_FILE` 复现并定位到具体方法/字段。
- 以复用现有为荣：复用上表的 harness 与回归机制，不要新造平行的测试入口。
- 以主动测试为荣：每轮必须新增回归用例并跑过 30s 快回归，否则本轮不算完成。
- 以遵循规范为荣：保持安全契约（不出无法解析的 Java、不 panic、退化必带 `yak-decompiler:` 标记）。
- 以诚实数据为荣：基准文档里的 before/after 计数必须由命令真实跑出，禁止估算或编造。
