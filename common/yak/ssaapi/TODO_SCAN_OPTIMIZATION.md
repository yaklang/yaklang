# SSA Code Scan — Remaining Optimization TODO

Derived from large-project scan analysis (23 projects, PHP/Python/Go).
See `build/scan-optimization-summary.md` for the full report.

## Performance

### CheckUntil per-node sub-query — partially addressed

`commit 69397ce25` added a per-node `matchCache` (sync.Map keyed by SSA
node ID) to `checkItem`. Repeated nodes across descent paths now skip
the full SF sub-query. However, the first visit of each node still
runs a complete `QuerySyntaxflow` → `SFFrame.Feed` → `exec` loop. For
rules with unique-node-heavy descents (moodle: 11134 sources), the
cache hit rate may be low. Further options:
- Downgrade simple `until` conditions (`*?{have: ".Sprintf"}`,
  `*?{opcode:add}`) to lightweight opcode/type checks without full SF
  execution.
- Pre-compile `until` patterns into a fast-match predicate.

**Files:** `common/yak/ssaapi/sf_config.go`, `sf_dataflow.go`

### ssa.NewConst cascade allocation — reverted, needs different approach

`commit` attempted to cache `ConstInst` for common values ("", 0, 1)
but was reverted: `ConstInst` embeds `*anValue` which is mutable
(program-specific id/fun/block state), so sharing across tests caused
test isolation failures. The `*Const` value is already cached via
`ConstMap`, but `ConstInst` + `anValue` + `anInstruction` + type are
allocated fresh each call (17.61% cum heap on GoBlog scan).

Possible approaches:
- Pool `anValue` + `anInstruction` with reset (sync.Pool + explicit
  field zeroing before reuse).
- Separate the immutable `*Const` from the mutable wrapper so the
  wrapper can be pooled without sharing mutable state.

**Files:** `common/yak/ssa/const.go:68-90`, `ssa_predefined.go`

### Go SQL rule fanout — rule-side change

`*.QueryRow as $func; *.Query as $func;` matches all methods named
Query/QueryRow across the entire program (156 matches on GoBlog).
Each match triggers `GetTopDefs` with `until` sub-queries.

Fix: narrow `*.Query` to `$sink.Query` (only match database/sql sink)
in the rule .sf files.

**Files:** `common/syntaxflow/sfbuildin/buildin/golang/cwe-89-sql-injection/`

### PHP compile ANTLR4 GC pressure

pfsense compile: 69% GC CPU, 56% heap in ANTLR `ParserATNSimulator`.
`NewBaseATNConfig` alone is 29% of heap. Inherent to ANTLR4 LL(*)
adaptive prediction.

Fix: ATNConfig object pool or prediction context cache in the ANTLR
runtime. High risk — touches external dependency.

**Files:** ANTLR runtime (go/antlr)

### gorm v1 DB read overhead

`Scope.InstanceGet` (7.50% alloc objects) + `Scope.Fields` (3.48%)
from gorm v1's reflect-heavy ORM layer during ssadb reads.

Fix: migrate to gorm v2 or use direct SQL queries with batch caching.

**Files:** `common/yak/ssa/ssadb/`

## Correctness

### Go GetCurrentRange nil-fallback (rare)

`commit de5d5f907` demoted the WARN (fallback token provides correct
range) to Debug. The ERROR case (`fallback is nil` → dummy 1:1,1000:1
range) still occurs ~11 times per Go project. These produce wrong
source locations in vulnerability reports.

Fix: ensure `CurrentRange` is always set before instruction creation
in the Go frontend. Investigate which code paths emit instructions
without a range context (import handling, deferred emission).

**Files:** `common/yak/ssa/position_front.go:100`, `InstructionEmit.go`, `builder.go:207`

### Moodle scan failure — PHP rule fanout

PHP info-leak rules use `include=* & $source` / `include=* & $output`
that match 11134 values on moodle (1.8M lines). Each triggers full
`GetTopDefs` descent → combinatorial explosion → scan killed.

Fix: narrow `include` patterns in PHP rule .sf files. The
`--rule-timeout 10m` in `scan-project.sh` will bail, but the rules
should be more targeted for large projects.

**Files:** `common/syntaxflow/sfbuildin/buildin/php/`

## Coverage

### Python rule coverage — only 10 rules succeed

Only 9 `.sf` rules have `lang: python` + 29 general = 38 max, but only
10 succeed on Python projects. 28 general rules fail (pattern mismatch
or SSA feature gap).

Fix: add more Python-specific detection rules; investigate why general
rules fail on Python (may be SSA frontend gaps for Python constructs).

### PHP rule coverage — only 35 rules succeed

16 PHP-specific + 19 general = 35 out of 269. 234 skipped via
language filter.

Fix: add more PHP-specific rules for common vulnerability types
(command injection, SSRF, deserialization, etc.).
