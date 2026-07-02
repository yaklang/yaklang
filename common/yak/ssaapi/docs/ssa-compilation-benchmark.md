# SSA Compilation Benchmark (large projects, 1G+)

`yak ssa-compile` on large projects, branch
`refactor/ssa/compile_step_shrink_ast` @ `bfc2ffd0e` (per-unit flush
consolidation + aggressive-clear removal). This branch is compile-split only;
the cross-process / scan-log / offset-map fix is isolated on
`enhance/syntaxflow/support_large_project_coress_process` (separate benchmark
doc there).

## Environment

- **Branch / commit**: `refactor/ssa/compile_step_shrink_ast` @ `bfc2ffd0e`
- **yak**: `dev`, go1.22.12 linux/amd64
- **Machine**: AMD Ryzen 5 7600 (12 threads), 23 GiB RAM, WSL2
- **Invocation**: `YAKIT_HOME=<worktree-local .db> yak ssa-compile -l <lang> -t <path>`
  (fresh `.db` per project; `/usr/bin/time -v` for wall + Max RSS).
- **`-l` is required for mixed-language trees** — see "Auto-detect caveat" below.

## Results — compilation completes on 1G+ projects

| Project | Lang | Disk | Java/PHP files | Wall time | Peak RSS | Exit | Crash | ERRO | WARN |
|---------|------|-----:|---------------:|----------:|---------:|-----:|------:|-----:|-----:|
| javacms/core (dotCMS) | Java | 1.8G | 7,476 | 31m09s | 12.7 GiB | 0 | 0 | 380 | 4,820 |
| spring-project/spring-framework | Java | 385M | 8,316 | 12m04s | 5.5 GiB | 0 | 0 | 150 | 7,513 |

**Both 1G-class Java projects compile to completion, exit 0, no crash / panic /
concurrent-map.** The compile-split flush path (one GC per unit, dbcache
`ResidentFlushCache`) held peak RSS to 12.7 GiB (dotCMS, 1.8G tree) and
5.5 GiB (spring-framework, 385M tree) on a 23 GiB box. Compile is the part
this branch owns; it is stable on large Java projects.

`spring-framework`: 4765 of ~8316 `.java` compiled (the rest are
test/skip-excluded modules and the `spring-asm-*` / `ci` config files that fail
pre-handler parse). ERRO 150 is all Java visitor (`assign variable is nil`,
`ClassBluePrint is nil`, member-call nil) — recovered, non-fatal.

### javacms/core error breakdown (380 ERRO, all non-fatal)

| Count | Message (normalized) | Notes |
|------:|----------------------|-------|
| 172 | `BUG: ClassBluePrint is nil` | Java class-resolution limitation on dynamic/loading classes |
| 128 | `assign variable is nil` | Java visitor, per-file, recovered |
| 52 | `BUG: readMemberCallVariableEx ... nil` (key/value) | Java member-call on unresolved type |
| 7 | `file size N exceeds max limit` / `skip file ... documentation.json` | 12.7MB JSON over the 5MB cap — expected skip |
| 6 | `pre-handler parse [test-jmeter/helm-chart/...]` | k8s YAML templates parsed as Java by the JMeter test module — pre-existing |
| 3 | `'\N': unquote error invalid syntax` | string-literal edge case |

All errors are caught; none abort the compile. `files_compiled=6612` (of ~7476
`.java`; the rest skipped via the size cap or excluded test/chart modules).

## Auto-detect caveat (javacms/core mis-detects as Python)

**Symptom**: `yak ssa-compile -t ~/Target/javacms/core` (no `-l`) detects the
project as **Python** and compiles only 7 `.py` files (preHandler 523 / build 7)
instead of 7476 `.java`. With `-l java` it correctly compiles 6612 files.

**Root cause** (in `common/coreplugin/base-yak-plugin/SSA 项目探测.yak`): the
project-detect yak script walks the tree and, on the **first** marker file hit,
calls `setLanguage(...)` which is first-wins and sticky. javacms/core is mostly
Java (7476 `.java`, root `pom.xml`) but also ships a stray
`.claude/skills/cicd-diagnostics/requirements.txt`; because directory readdir
order is non-deterministic, that `requirements.txt` (a Python marker) can be
seen before `pom.xml` (a Java marker), so the walk locks `language=python` and
then the extension-score path (`addLanguageScore`) is short-circuited.

**Why it wasn't caught**: extension scoring (`scoreMap`) already counts `.java`
correctly, but the marker short-circuit (`setLanguage` in `onFileStat`) bypasses
it. The two mechanisms don't agree when markers appear out of order.

**Mitigation (no code change on this branch — compile-split is unaffected)**:
pass `-l java` for mixed trees. A proper fix belongs in the project-detect yak
script (markers should *score*, not *short-circuit*, so a clear extension
majority wins) — tracked separately, not part of the compile-split refactor.
The old benchmark doc's config (`maxFiles=100`, batch-level GC,
`AggressiveClearMemory`) is superseded on this branch.

## Notes

- The old benchmark table (maxFiles=100 / batch-level GC / AggressiveClearMemory)
  is superseded: `AggressiveClearMemory` is removed and program-level release is
  folded into `ReleaseCompletedUnitMemory`; GC is one-per-unit at flush.
- `yak` builds from `./common/yak/cmd/` (CLAUDE.md's `cmds/yak.go` path is stale).
- Runtime/scan-phase benchmarks (code-scan rule execution) are out of scope for
  this compile-split branch; they live on
  `enhance/syntaxflow/support_large_project_coress_process`.