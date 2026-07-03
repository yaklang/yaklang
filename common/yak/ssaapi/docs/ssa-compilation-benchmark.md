# SSA code-scan Benchmark (cross-process / large-project branch)

Runtime benchmark for `yak code-scan` on
`enhance/syntaxflow/support_large_project_coress_process`.

This branch layers the **cross-process / scan-log / offset-map fix**
(commit `e670828b4`) on top of the compile-split refactor
(`refactor/ssa/compile_step_shrink_ast` @ `bfc2ffd0e`). It is isolated here
because the deeper dataflow it enables is a SyntaxFlow/large-project concern,
not a compile-split concern.

## Environment

- **Branch**: `enhance/syntaxflow/support_large_project_coress_process`
  (= `bfc2ffd0e` compile-split + `e670828b4` cross-process/scan-log/offset fix)
- **yak**: `dev`, go1.22.12 linux/amd64
- **Machine**: AMD Ryzen 5 7600 (12 threads), 23 GiB RAM, WSL2
- **Invocation**: `YAKIT_HOME=<worktree-local .db> yak code-scan -t <path>`
  (default config, info log level, no pprof / no perf logs). Fresh `.db` per
  project. Wall time from `/usr/bin/time -v`; peak memory = Max RSS.

## Results

| Project | Lang | Files | Wall time | Peak RSS | Exit | Risks | ERRO | WARN | Status |
|---------|------|------:|----------:|---------:|-----:|------:|-----:|-----:|--------|
| grav | PHP | 522 | 4m34s | 1.72 GiB | 0 | 63 | 41 | 1046 | ✅ completes all 269 rules |
| moodle | PHP | 7733 | 138min+ (killed) | n/a (killed) | 137 | 0 | 1590 | 3589 | ❌ hangs on a heavy rule |
| javacms (Java) | Java | TBD | TBD | TBD | TBD | TBD | TBD | TBD | not run yet |

## grav (small-medium PHP) — works

`yak code-scan -t ~/Target/grav`: 522 PHP files, 4m34s wall, 1.72 GiB peak RSS,
exit 0, scans all 269 rules, finds 63 risks. Scan-log ERRO dropped from 69
(pre-fix) to 41 after the cross-process fix; the residual ERRO is pre-existing
PHP visitor limitations (`unhandled expression` for `match()`,
`weakLanguage call … not found`).

**With the per-rule-timeout fix** (solidified DB, `code-scan -p grav`, default
5m `--rule-timeout`): all 269 rules finish (`Finished=269/269`, `Failed=0`),
still **63 risks**, no rule hit the budget — confirms the 5m default does not
bail fast rules on a normal project (no coverage regression).

## moodle (large PHP) — hung on a heavy rule pre-fix; fix = per-rule budget (see below)

`yak code-scan -t ~/Target/moodle`: 7733 PHP files. Compile phase (~95min,
7398 files) completes, then the scan-rules phase **hangs**:

- Several heavy rules (`检测PHP不安全的文件上传漏洞`, `检测PHP信息泄露风险`,
  `检测PHP FTP信息泄露漏洞`, …) all stall at the same state:
  `get topdef: 11134 values, {include=* & $xxx [sf]}` /
  `status=native$call include=[{...php-tp-all-extern-variable-param-source...}]`.
- These rules use the SyntaxFlow `dataflow(include=...)` native call
  (`nativeCallDataFlow`, `sf_dataflow.go:441`). On moodle the `include` pattern
  matches **11134 external-variable param sources**; `nativeCallDataFlow`
  collects all 11134 into `vs` and runs the recursive `getTopDefs` dataflow on
  each.

### Root cause

The cross-process fix (`rollbackCrossProcess` restoring the `emptyStackHash`
sentinel) is **correct** — it stops the dataflow from aborting early, so grav
finds more risks. But it removes an **implicit per-source depth cap** that
existed before (the `BUG:The cross process table is empty` early-abort, which
fired 40× on grav pre-fix). With the sentinel restored, each of moodle's 11134
sources now traverses deeply.

Existing caps do not bound the **total** work:

- `dataflowValueLimit = 100` (`analyze_context.go:28`): caps only a single
  `getTopDefs` call's breadth (bails >100). Does not cap the 11134-source outer
  set or total recursion.
- `errRecursiveDepth` "recursive call is over 10000" (`analyze_context.go:31`):
  caps per-branch **depth** (fired 538× on moodle). Does not cap
  11134 sources × branching × depth.
- `MaxDepth` default 500 (`exclusive_config.go:89`): per-branch depth; fired
  only 2×.

So: **11134 sources × (now-deep, uncapped-total traversal) = exponential**; each
heavy rule runs 20+ min and several run concurrently → the scan cannot finish
in 138min (killed, `Finished=230/269`).

### Fix (this branch) — per-rule wall-clock budget

The chosen bound is a **per-rule wall-clock budget**, implemented as a
`context.WithTimeout` around each rule query in the scan runner
(`syntaxflow_scan/runtime.go` `Query`), wired through a new
`ssaconfig.WithScanRuleTimeout` option + `--rule-timeout` CLI flag on
`code-scan` (default **5m**, `0` disables).

The deadline propagates via the existing context plumbing:
`QueryWithContext(ruleCtx)` → `queryConfig.ctx` → `OperationConfig.ctx`
(`exclusive_config.go` `WithExclusiveContext`) → `AnalyzeContext.getContext()`
(`analyze_context.go:207`), which is checked with `select { case <-ctx.Done():
… }` at **every recursive dataflow step** (`analyze_context.go:195`). So when
the budget fires, the recursive `getTopDefs` unwinds and the rule is bailed
(partial results) instead of hanging. The scan runner detects the bail via
`ruleCtx.Err()` (works whether the query surfaced the ctx error or returned
partial results with nil err) and logs `rule … hit per-rule budget (…), bailed`.

Why a time budget over a source-set cap: a source-set cap (`truncate vs to N`)
loses coverage on every large project, even rules that would finish in time.
The time budget only bails rules that genuinely exceed it, so normal projects
keep full coverage (verified: a tiny PHP smoke scan finds its XSS risk with
`--rule-timeout 30s`, budget never fires) while moodle's 20+min heavy rules
are bailed at 5m. A source-set cap remains a possible future opt-in backstop
but is not enabled by default.

Unit guard: `syntaxflow_scan/rule_timeout_test.go`
`TestStartScan_RuleTimeout_BailsHeavyRule` — a synthetic PHP program with
~2000 entry functions × a 15-deep call chain into a sink exercises the same
breadth × depth `dataflow(include=...)` workload. It asserts (a) with no
budget the heavy rule does real measurable work, and (b) with a 100ms budget
the rule is bailed (`hit per-rule budget` error callback) and the scan
finishes fast. PASS.

### Verification status

- Smoke: `ssa-compile -t <tiny> -p smokephp` then `code-scan -p smokephp
  --rule-timeout 30s` → 269 rules, 1 risk, exit 0, no hang. The solidified-DB
  scan path (`-p`, no recompile) works with the fix.
- grav / moodle / javacms end-to-end with the fix: **pending** — re-run
  `code-scan -p <name> --rule-timeout 5m` on the solidified DBs and fill the
  table above (expect moodle to finish instead of hanging, with the heavy
  rules bailed at 5m).

## Note on the isolated commit

`e670828b4` is intentionally on this branch, not on
`refactor/ssa/compile_step_shrink_ast`. The TODO(scan-log) comments it carries
trace where split-compile flush + lazy reload surface nil/missing data; the
OffsetMap mutex guards the scan-time race that the deeper dataflow exposed.