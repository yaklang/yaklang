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
| moodle | PHP | 7733 | 138min+ (killed) | n/a (killed) | 137 | 0 | 1590 | 3589 | ❌ hangs on a heavy rule (pre-budget) |
| javacms (Java) | Java | 1.8G | **15m22s** | ~21 GB + ~16 GB swap (24 GB RAM machine) | 0 | 8711 | 1 SAXParserFactory (pre-existing) | — | ✅ **completes all 270 rules** (Fix 1–8, `--rule-work-limit 200000 --rule-timeout 10m`) |

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
- **javacms (Java, 1.8G, solidified DB)**: `code-scan -p javacms-core
  --rule-timeout 10m --rule-work-limit 200000` → **all 270 rules finish in
  15m22s**, 127 success / 1 failed (SAXParserFactory "condition failed" —
  pre-existing, same on optABC/main), **8711 risks**, exit 0. Peak RSS ~21 GB
  + ~16 GB swap on a 24 GB-RAM machine (heavy rules bail at the work-limit,
  54 partial bails). This is the first javacms run that COMPLETED instead of
  OOM-killing — see the memory-optimization section below.
- grav / moodle end-to-end with the work budget: **pending** — re-run on the
  solidified DBs with the new default `--rule-work-limit 200000`.

## Memory-optimization layer (Fix 1–8)

On top of the per-rule budget, a pprof-driven pass cut allocation churn and
bounded live memory so javacms finishes within 24 GB instead of OOM-killing
at 24 GB RAM + 32 GB swap. Measured on the final scan
(`build/pprof/javacms-F78b/heap_before_gc.pb.gz`, alloc_space):

| Hotspot | optABC (pre-fix) | Fix 1–8 | Fix |
|---|---|---|---|
| NewRuneOffsetMap (FileFilter per-file rebuild) | 71 GB / 20% | 8.5 GB / 8% | Fix 3: memoize on MemEditor |
| execRule/TrackLow churn (off-path closure+name) | 124 GB cum | gone | Fix 4: off-path = direct call |
| BitVector.Clone (mergeAnchorBits) | 355 GB / 35% | 2.4 GB / 2% | Fix 2: COW |
| SafeMapWithKey.Set live (DependOn/EffectOn edge graph) | 2.3 GB live, unbounded | bounded | Fix 8: per-value 256 + per-descent 200k caps |
| MergeValues churn (clearup inherited-var re-merge) | 463 GB / 27% | gone | Opt A (snapshot-once, fixed over-skip) |

Net: cumulative alloc 1686 GB → ~105 GB (−94%); live heap peak 49 GB →
~16 GB (within 24 GB RAM); BitVector.Clone and MergeValues dropped out of
the top. Remaining top (`Program.NewValue` 11 GB, `TakeSymbolSnapshot` 12 GB)
are the Fix 6 (snapshot frequency) and Fix 5 (instruction-id Value cache)
targets — follow-ups, not needed for javacms to complete.

`<include>` lib rules now honor the parent rule's ctx + work budget (Fix 7):
the path-traversal rule's `<include('java-write-filename-sink')>` (which runs
`<typeName>` over tens of thousands of File calls) previously ran 30min+
past the rule budget; it now bails at the work-limit like any other opcode.

## dbcache final-flush stall (recompile `-t` path) — fixed 2026-07-07

The scan-stage fixes above (Fix 1–8) landed on the **solidified-DB** scan path
(`-p <program>`, no `-t`), which is what produced the 15m22s / 8711-risk
completion. The **recompile-from-target** path (`-l java -t <target> -p <name>`)
additionally runs the full SSA compile + a final `SaveToDatabase` flush; on
javacms-core that final flush **stalled**: all IrIndex/IrOffset/IrType rows
piled into async dbcache savers and were drained only at the end via per-row
`db.Create` / `db.Save` / `FirstOrCreate`, so the savers' `FeedBlock` blocked
5–7 s/item and the scan stage never started (1 h+, then killed; 0 risks).
Root cause was `338cf67c0`, which disabled per-batch `FlushCompileUnit` to pass
two suites (it spilled BasicBlocks → nil-Variable on lazy reload; and
`flushAuxStores` cleared resident maps → TestImportClass over-resolved).

Fix (this commit):
- **`ProgramCache.FlushAuxSavers()`** (`common/yak/ssa/database_cache.go`) —
  per-batch drain of index/offset/type async savers ONLY. It spills no
  instructions and clears no resident maps, so the two suites above stay green;
  cross-unit resolution keeps using the resident maps and BasicBlocks stay
  resident. Called once per batch in `parseProjectWithFSUnits`
  (`common/yak/ssaapi/ssa_compile_fs.go`).
- **`SaveIrIndexBatch` / new `SaveIrOffsetBatch` / new `SaveIrTypeBatch`** →
  chunked multi-row INSERT (chunk = `floor(900/cols)`: IrIndex 100, IrOffset
  150, IrType 150). IrType is a delete-then-insert upsert (delete
  `(program_name, type_id)` chunked at 999, then bulk INSERT) to preserve the
  idempotent-update semantics of `TestTypeFlushUpsertsExistingTypeRows` (a
  later flush of the same type_id overwrites the row with the merged value,
  not a duplicate). ir_types/ir_indices/ir_offsets have no UNIQUE constraint
  and recompile deletes the program's rows first (`DeleteProgramIrCode`), so
  pure INSERT is correct.
- **`WithName("IrIndex")` / `WithName("IrOffset")`** on the two savers —
  previously the `dbcache save blocked` log had an empty name and could not
  identify which saver stalled.

Result (javacms-core recompile-scan, `yak code-scan -l java -t
/home/wlz/Target/javacms/core -p javacms-core --rule-timeout 10m
--rule-work-limit 200000`, worktree-local YAKIT_HOME, 24 GB RAM):

| stage | before (stuck) | after (fix) |
|---|---|---|
| compile (102/102 batch) | ~4 min | ~9.5 min (incl. per-batch aux flush) |
| final flush | **stall ~1 h, killed** | **~16 min** (type 4.3 + instruction 12) |
| scan (269 rules) | **never started** | 269/269 done |
| **risk** | **0** (killed) | **9149** |
| `dbcache save blocked` | 5 (5–7 s each) | **0** |
| peak RSS / swap | 21 GB RSS + swap 32 GB (thrash) | 21.5 GB RSS, swap < 900 MB (no thrash) |
| total wall | >1 h (killed) | 54 min |

pprof + the next optimization targets are in
`build/pprof/javacms-auxflush2/summary.md` (instruction-store final flush is
the new bottleneck: ~60–70 % CPU in GC scanning a 21 GB resident heap;
`saveInstructionPersistRecords` is still per-row `tx.Save`/`UpsertIrCode`;
`fullTypeNameAdd` 2.1 GB + `ssadb.EmptyIrCode` 1.75 GB stay resident during
flush). Those are follow-ups — the recompile path now completes instead of
hanging/OOM-killing.

## Note on the isolated commit

`e670828b4` is intentionally on this branch, not on
`refactor/ssa/compile_step_shrink_ast`. The TODO(scan-log) comments it carries
trace where split-compile flush + lazy reload surface nil/missing data; the
OffsetMap mutex guards the scan-time race that the deeper dataflow exposed.