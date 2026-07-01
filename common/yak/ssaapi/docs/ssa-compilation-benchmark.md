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

## moodle (large PHP) — hangs on a heavy rule (WIP, see TODO)

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

### TODO (this branch)

Add an explicit total-work bound so the deeper (correct) dataflow cannot hang
on large projects. Candidates:

- Cap the `vs` source set in `nativeCallDataFlow` (`sf_dataflow.go:471`):
  process at most N sources, warn+truncate beyond that.
- Per-rule wall-clock budget in the scan runner (`ssacli.go`): bail a rule
  after T seconds.
- A tighter effective per-source depth/cost cap (e.g. lower `MaxDepth` for
  `dataflow()` native calls) so deep traversal is bounded.

Recommended: source-set cap + per-rule time budget. Then re-run moodle + a Java
medium-large target and fill the javacms row above.

## Note on the isolated commit

`e670828b4` is intentionally on this branch, not on
`refactor/ssa/compile_step_shrink_ast`. The TODO(scan-log) comments it carries
trace where split-compile flush + lazy reload surface nil/missing data; the
OffsetMap mutex guards the scan-time race that the deeper dataflow exposed.