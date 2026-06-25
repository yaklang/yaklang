# Java 反编译长尾清零工作流 (HARNESS_WORKFLOW)

> 目标: 把 `.m2` 真实 jar 语料上的 `partial` / `syntax` / `err` / `panic` 逐个清零。
> 配套基准: [`YAK_DECOMPILER_BENCHMARK.md`](./YAK_DECOMPILER_BENCHMARK.md)
>
> 适用任意承载这项工作的分支；本文不绑定具体分支名。

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

### 1b. 全量分桶 + 计数（阶段性盘点，分钟级）

需要看全部失败按 reason 如何分布、或前后对比 partial 是否下降时，跑一次完整扫描（**不要**在迭代中频繁用，太慢）：

```bash
# 在仓库根目录执行
STUB_REASONS=1 \
M2_MAX_JARS=120 M2_MAX_CLASSES=24491 \
PROBLEM_DIR=/tmp/jdec-problems \
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
   - 在 [`YAK_DECOMPILER_BENCHMARK.md`](./YAK_DECOMPILER_BENCHMARK.md) 追加一节，记录本轮根因、修复点、同配置 before/after 的 `partial`/`stubs`/`panic` 计数（用真实数据，禁止编造）。

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
| **迭代首选：秒级定位第一个失败类** | `STUB_REASONS=1 STOP_ON_FIRST=1 M2_MAX_JARS=120 M2_MAX_CLASSES=24491 PROBLEM_DIR=/tmp/jdec-problems PROGRESS_EVERY=0 go test -run TestM2StubReasons -v ./common/javaclassparser/tests/` |
| 大扫描 + 失败类落盘分桶（阶段性盘点） | `STUB_REASONS=1 M2_MAX_JARS=120 M2_MAX_CLASSES=24491 PROBLEM_DIR=/tmp/jdec-problems go test -run TestM2StubReasons -v ./common/javaclassparser/tests/` |
| 计数 + 每类指纹（前后对比） | `M2_OUT=/tmp/m2.txt M2_MAX_JARS=120 M2_MAX_CLASSES=12000 go test -run TestM2RegressionHarness -v ./common/javaclassparser/tests/` |
| 单类复现（文件） | `DIAG_FILE=<path>.class go test -run TestDiagDecompileClass -v ./common/javaclassparser/tests/` |
| 单类复现（jar+子串） | `DIAG_JAR=<jar> DIAG_CLASS=<substr> go test -run TestDiagDecompileClass -v ./common/javaclassparser/tests/` |
| 覆盖全语料（含 spring 等） | 上述命令追加 `M2_INDUSTRY=1`（每 jar 上限 `M2_MAX_PER_JAR`，默认 200） |
| 本地快回归（≤30s，主闸门） | `go test ./common/javaclassparser/...` |
| 回归数据集目录 | `common/javaclassparser/tests/testdata/regression/*.class` |
| 回归用例落点 | `tests/regression_test.go`（语法/完整）、`tests/ga_panic_free_test.go`（panic） |

## 红线

- 以认真查阅为荣：动核心代码前，必须先用 `DIAG_FILE` 复现并定位到具体方法/字段。
- 以复用现有为荣：复用上表的 harness 与回归机制，不要新造平行的测试入口。
- 以主动测试为荣：每轮必须新增回归用例并跑过 30s 快回归，否则本轮不算完成。
- 以遵循规范为荣：保持安全契约（不出无法解析的 Java、不 panic、退化必带 `yak-decompiler:` 标记）。
- 以诚实数据为荣：基准文档里的 before/after 计数必须由命令真实跑出，禁止估算或编造。
