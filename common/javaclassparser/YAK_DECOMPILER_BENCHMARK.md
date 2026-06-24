# YAK DECOMPILER REPORT BENCHMARK

A full-spectrum evaluation of the Yaklang Java decompiler (`java.Decompile` /
`javaclassparser.Decompile`) across four axes: **syntax coverage**,
**correctness**, **test hygiene**, and **performance**. Every number below is
produced by a reproducible test or benchmark in this repository (no synthetic or
hand-waved figures) and can be regenerated with the commands shown in each
section.

- Decompiler entry points: `javaclassparser.Decompile([]byte) (string, error)`
  and the Yaklang library wrapper `java.Decompile`.
- Host used for the figures: darwin/arm64, Go 1.22.12, OpenJDK `javac` present.
- Reproduce everything fast (no network, no local Maven cache required):
  `go test ./common/javaclassparser/...`

---

## 1. Executive summary

| Axis | Result | How it is measured |
|------|--------|--------------------|
| Coverage (parse-or-degrade) | 23/23 corpus groups produce **valid Java**; 0 syntax errors, 0 hard errors, 0 panics | `TestSyntaxCoverageMatrix` |
| Coverage (full reconstruction) | 20/23 groups fully reconstructed; 3 groups isolate exactly **2 root-cause gaps** | `TestSyntaxCoverageMatrix` |
| Correctness (javac round-trip) | 4/13 classic single-class corpora recompile cleanly; failures pinpoint concrete gaps | `TestRecompileRoundtrip` |
| Determinism | byte-identical output across repeated decompiles | `TestCorpusDeterminism` |
| Test suite | green & fast: `./...` ≈ 22s vs >150s before (8x), no machine-specific dependencies | `go test ./common/javaclassparser/...` |
| Performance | core 281 ms / 216 MB per 106-class jar; validation safety net ≈ +24% CPU / +23% allocs | `BenchmarkDecompileJar` |

The decompiler's **safety guarantee holds**: for every input in the corpus it
either fully reconstructs a method or degrades it to a tagged, still-parseable
stub (`yak-decompiler:` marker), never emitting un-parseable Java and never
panicking out of `Decompile`.

---

## 2. Coverage benchmark

Reproducible because the corpus is **Java source** compiled by `javac` at test
time (under `tests/corpus/{classic,modern}`), so the bytecode is regenerated on
the host instead of being checked in.

```
go test -run TestSyntaxCoverageMatrix -v ./common/javaclassparser/tests/
```

Outcome classes per group: `OK` (fully reconstructed + valid), `STUB` (some
member degraded to a stub but class still valid), `SYNTAX` (invalid Java emitted
— a real defect), `ERROR` (decompile returned an error), `PANIC`.

### Classic corpus (Java 8 bytecode) — 18 groups
```
ok=17  stub=1  syntax=0  error=0  panic=0
```
- The single `STUB` is **Exceptions** → `tryCatchFinally(int[],int)` fails with
  `ParseBytesCode failed: multiple next`.

### Modern corpus (Java 17 bytecode) — 5 groups
```
ok=3  stub=2  syntax=0  error=0  panic=0
```
- `STUB` groups **Records** and **SealedVar** fail only on the compiler-synthesized
  `toString()/hashCode()/equals()` with
  `ParseBytesCode failed: call bootstrap method error` (the `invokedynamic`
  `ObjectMethods` bootstrap).

### Coverage conclusion
The two remaining gaps are precisely isolated and orthogonal:
1. **`try/catch/finally` CFG reconstruction** ("multiple next") — a control-flow
   structuring limitation when a region has multiple successors.
2. **Record / sealed `invokedynamic ObjectMethods` bootstrap** — the auto-generated
   value-type methods are not yet synthesized.

Everything else (operators, literals, control flow, loops, switches,
try-with-resources, arrays, generics, inheritance, inner classes, enums, lambdas,
strings, annotations, initializers, concurrency, casts/instanceof, pattern
matching, switch expressions, text blocks) reconstructs to valid Java.

---

## 3. Correctness benchmark (decompile → recompile round-trip)

The strictest oracle: take known-good source, compile it, decompile the
`.class`, then feed the decompiled Java **back through `javac`**. This is far
stronger than the ANTLR syntax net — it catches type errors, precedence errors,
unreachable-code and bad-operand errors that still parse.

```
go test -run TestRecompileRoundtrip -v ./common/javaclassparser/tests/
```
`javac` is pinned to the English locale (`-J-Duser.language=en -J-Duser.country=US`,
`-nowarn -Xlint:none`) so diagnostics are stable across machines.

### Classic single-class corpora
```
recompile-ok:  4   (CastsInstanceof, ControlFlow, Strings, Switches)
recompile-fail: 9
stub:          1   (Exceptions)
dec-err:       0
multiclass:    4   (skipped: multi-type compilation units)
```

The 9 recompile failures are the actionable correctness frontier (representative
first error per category):

| Category | javac error class | Likely root cause |
|----------|-------------------|-------------------|
| Literals | integer number too large | long-literal `L` suffix not emitted |
| Loops | unreachable statement | all loops lowered to `do{...}while(true)` |
| Operators | bad operand types | compound-assignment / promotion modeling |
| Arrays | incompatible types | array element typing |
| Generics | cannot find symbol | erased/elided type arguments |
| Initializers | incompatible types | field initializer typing |
| Lambdas | variable already defined | synthetic captured-var naming |
| Concurrency | cannot find symbol | synthesized helper references |
| TryWithResources | variable ... | resource desugaring |

These are intentionally **not** masked: passing categories are pinned by
`recompileGateBaseline` so a regression that breaks `CastsInstanceof`,
`ControlFlow`, `Strings`, or `Switches` fails CI, while the rest are tracked as
the improvement backlog.

### Correctness fixes already landed in this evaluation
- **Cast precedence**: `OP_CHECKCAST` now renders as `((Type)(x)).m()` instead of
  `(Type)(x.m())`, fixing member-access on cast receivers
  (`code_analyser.go`; golden `VarFold` refreshed).
- **Absolute nested-archive paths**: `normalizeArchivePath` preserves the leading
  slash so `/abs/app.war/.../foo.jar/Foo.class` opens from the host filesystem.

---

## 4. Test-hygiene benchmark

Goal: a stable, fast, portable core suite with no machine-specific or
time-wasting tests, while keeping (and increasing) real coverage.

```
go test ./common/javaclassparser/...      # green, ~22s total
```

Actions taken:
- **Gated machine-specific diagnostics** behind env vars (`BENCH_JAR`, `JDSC_DIR`,
  `M2_DETERMINISM`) so the default run no longer scans `~/.m2` or `/tmp/...`.
  Default suite time dropped from **>150s to ~22s (≈8x)**.
- **Deleted `decompiler_test.go`**: four debug tests hardcoded to
  `/Users/z3/Downloads/...` with no assertions; one nil-panicked in
  `filepath.Walk`, aborting the package binary and hiding every later failure.
- **Repaired the failures that the crash had been hiding** (all pre-existing):
  - `fs_test`: assert the current graceful per-method stub behavior instead of an
    obsolete whole-dump-failure marker.
  - `access_flags_verbose_test`: enums render as `public enum` (implicit
    final/abstract are illegal to write).
  - jar tests: off-by-one root count, stale trailing-slash expectation, and
    **real bytecode** for nested-jar fixtures (they had stored Java source under a
    `.class` name, which only "decompiled" by echoing input).
  - `loop_test`: corrected a swapped then/else golden (true branch belongs in the
    then-block).
- **Added portable replacements** for the gated diagnostics: `TestCorpusDeterminism`
  verifies byte-identical output without needing a local Maven cache.

---

## 5. Performance benchmark

```
# core decompiler in isolation (validation safety net off)
BENCH_NO_VALIDATE=1 BENCH_JAR=<jar> go test -run xxx -bench 'BenchmarkDecompileJar$' -benchmem ./common/javaclassparser/tests/
# full pipeline (validation on, default)
BENCH_JAR=<jar> go test -run xxx -bench 'BenchmarkDecompileJar$' -benchmem ./common/javaclassparser/tests/
```

Target: `commons-codec-1.15.jar` (106 classes), `-benchtime=8x`.

| Configuration | ns/op | B/op | allocs/op |
|---------------|------:|-----:|----------:|
| Full pipeline (validation on) | ~378 M | 281.6 MB | 4.64 M |
| Core only (validation off) | ~281 M | 216.3 MB | 3.41 M |
| **Validation safety net share** | **≈ 24%** | **≈ 23%** | **≈ 26%** |

### Profile attribution
CPU is **GC-bound** (`gcDrain`/`scanobject`/`greyobject` dominate), driven by
allocations. On the validation path ~70% of allocations are ANTLR ATN-simulation
objects (`NewBaseATNConfig`, `BaseATNConfigSet.Add`, prediction-context merges) —
inherent to re-parsing each class to guarantee parse-ability. In the core
decompiler the largest allocators are `ParseOpcode`, the dominator-tree build, and
the stack-simulation/type-inference closures.

### Optimizations landed
- **`ParseOpcode` pre-sizing**: the opcode slice and both offset maps are now sized
  from the bytecode length, removing repeated grow/rehash garbage (the single
  largest core allocator). Behavior is identical (verified by goldens +
  `TestCorpusDeterminism`).
- **Validation timer hygiene**: the syntax-validation budget uses a stoppable
  `time.NewTimer` instead of `time.After`, so each per-class/member timer (and the
  source string it retains) is freed the moment validation returns rather than
  lingering for the full budget. This prevents thousands of simultaneous pending
  timers/goroutines on large-jar batch scans.

### Why the big lever (cross-parse ANTLR cache) was deliberately not pulled
The pinned ANTLR Go runtime (`v4.0.0-20220911`) has no locking on its DFA /
`JStore` structures, and decompilation runs in parallel (the jdsc self-check uses
100 goroutines). A process-wide shared validation DFA would data-race; the
existing per-worker cache + `DetachParserATNSimulatorCaches` design is the safe
choice. Pursuing this further would require an ANTLR upgrade (out of scope) and is
recorded as future work.

---

## 6. Backlog (prioritized by impact, from the data above)

1. **`try/catch/finally` "multiple next" CFG** — the only classic-corpus stub and
   the most common stub cause observed in real jars.
2. **Record / sealed `invokedynamic ObjectMethods` bootstrap** — unblocks modern
   (Java 17+) value types end-to-end.
3. **Recompile-frontier fixes**, highest-leverage first: long-literal suffix
   (Literals), loop idiom recovery vs `do/while(true)` (Loops), operator
   promotion (Operators).
4. **Allocation reduction** in `ParseOpcode` / stack-simulation if an ANTLR upgrade
   later enables a shared validation cache.

---

## 7. Reproduction quick reference

```
# Coverage matrix (javac-compiled corpus)
go test -run TestSyntaxCoverageMatrix -v ./common/javaclassparser/tests/

# Correctness round-trip (decompile -> javac)
go test -run TestRecompileRoundtrip -v ./common/javaclassparser/tests/

# Determinism (portable, no Maven cache)
go test -run TestCorpusDeterminism -v ./common/javaclassparser/tests/

# Full fast suite
go test ./common/javaclassparser/...

# Performance (set BENCH_JAR to any local jar)
BENCH_JAR=<jar> go test -run xxx -bench 'BenchmarkDecompileJar$' -benchmem ./common/javaclassparser/tests/
BENCH_NO_VALIDATE=1 BENCH_JAR=<jar> go test -run xxx -bench 'BenchmarkDecompileJar$' -benchmem ./common/javaclassparser/tests/
```
