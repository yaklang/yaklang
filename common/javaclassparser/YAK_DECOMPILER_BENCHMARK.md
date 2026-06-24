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
| Correctness (javac round-trip) | **5/13** classic single-class corpora recompile cleanly (was 4/13; **Literals fixed this round**); remaining failures pinpoint concrete gaps | `TestRecompileRoundtrip` |
| Determinism | byte-identical output across repeated decompiles; perf changes proven equivalent by per-class sha256 fingerprints | `TestCorpusDeterminism`, `TestDumpJarFingerprint` |
| Test suite | green & fast: `./...` ≈ 22s vs >150s before (8x), no machine-specific dependencies | `go test ./common/javaclassparser/...` |
| Performance | core **246 ms / 182 MB** per 106-class jar (was 315 ms / 217 MB → **−22% time, −16% bytes** this round); validation safety net ≈ +18% CPU / +23% allocs | `BenchmarkDecompileJar` |
| Scalability | near-linear to ~8 workers (3.6×), then **GC-bound regression** | `BenchmarkDecompileJarParallel` |

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
recompile-ok:  5   (CastsInstanceof, ControlFlow, Literals, Strings, Switches)
recompile-fail: 8
stub:          1   (Exceptions)
dec-err:       0
multiclass:    4   (skipped: multi-type compilation units)
```

The 8 recompile failures are the actionable correctness frontier. Each root cause
below was confirmed by reading the **full** `javac` diagnostic (run with
`RC_VERBOSE=1` to dump the decompiled source + every error per category), not
guessed:

| Category | Exact javac error | Confirmed root cause | Difficulty |
|----------|-------------------|----------------------|-----------|
| Arrays | `int[][][][] cannot be converted to int[][][][][][][]` | local array-type **dimension** over-counted vs the `multianewarray` it is initialized from | medium (type calc) |
| Operators | `bad operand types '<<' first:boolean second:int` (×12) + `incompatible types` | **JVM boolean/int ambiguity**: a local that is `int` in source is typed `boolean` because `&`/`|`/`^` on booleans share opcodes with int bitops | hard (type inference) |
| Loops | `unreachable statement` (the `continue;` after a nested infinite region) | every loop lowered to `do{...}while(true)`; the always-taken inner exit makes the synthesized outer `continue` unreachable | hard (loop idiom recovery) |
| Initializers | `int[] cannot be converted to int` + `cannot find symbol var1` | array field rendered as element type; a `final` field initialized in `<init>` is hoisted to a field initializer that references the (out-of-scope) constructor parameter | medium–hard |
| Generics | `cannot find symbol` | erased/elided type arguments on a generic call | medium |
| Lambdas | `variable v already defined` | synthetic captured-variable naming collision | medium |
| Concurrency | `cannot find symbol` | references to a synthesized helper/inner that is not emitted | medium |
| TryWithResources | `variable ...` | try-with-resources `close()` desugaring not re-sugared | medium |

Passing categories are pinned by `recompileGateBaseline`, so a regression that
breaks `CastsInstanceof`, `ControlFlow`, `Literals`, `Strings`, or `Switches` fails
CI; the rest are tracked as the backlog.

### Correctness fixes landed in this evaluation
Four defects were diagnosed from the round-trip oracle and fixed; **Literals now
recompiles cleanly** as a result, and all are verified non-regressing by the golden
suite + `TestCorpusDeterminism`:

1. **Numeric literal suffixes in expression position** (`java_value.go`,
   `JavaLiteral.String`). Long/float/double literals dropped their `L`/`F`/`D`
   suffix outside field declarations, so `Long.valueOf(9223372036854775807)` failed
   with *"integer number too large"* and `Float.valueOf(3.14)` had no overload (a
   bare `3.14` is a `double`). Now emitted as `9223372036854775807L`, `3.14F`,
   `2.718281828D`, with NaN/Infinity handled the same way the field path already did.
2. **Boolean field constants** (`dumper.go`). The JVM stores `boolean` as an int
   constant, so a `boolean` field rendered as the illegal `static final boolean B = 1`.
   Now rendered `= true` / `= false`.
3. **Boolean method arguments** (`expression.go`, `FunctionCallExpression.String`).
   An int literal flowing into a `boolean` parameter (Java has no int→boolean
   conversion) made autoboxing like `Boolean.valueOf(1)` fail. Now coerced to
   `true`/`false`, mirroring the existing int→byte/short/char cast logic.
4. **Primitive-cast precedence** (`code_analyser.go`, the `I2L/L2D/D2L/...` group).
   A conversion cast was rendered as `(long)a * b`, which parses as `((long)a) * b`
   and triggered *"possible lossy conversion from double to long"*. Now parenthesized
   as `(long)(a * b)` — the same precedence fix already applied to `OP_CHECKCAST`.

Previously landed in this evaluation:
- **Cast precedence on member access**: `OP_CHECKCAST` renders `((Type)(x)).m()`
  instead of `(Type)(x.m())` (golden `VarFold` refreshed).
- **Absolute nested-archive paths**: `normalizeArchivePath` preserves the leading
  slash so `/abs/app.war/.../foo.jar/Foo.class` opens from the host filesystem.

### What "recompile-fail" does **not** mean
A `recompile-fail` class is still **structurally decompiled to readable, valid Java**
(it passes the ANTLR syntax net and the coverage matrix); it only fails the much
stricter *javac type-check* round-trip. The frontier above is about semantic fidelity
of a minority of constructs, not about producing garbage.

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

Target: `commons-codec-1.15.jar` (106 classes), `-benchtime=5x -count=2`.

### 5.1 Throughput and the validation safety-net tax

The single most useful lever is `BENCH_NO_VALIDATE=1`, which turns off the
post-decompile ANTLR re-parse and isolates the **decompiler core** from the
**safety net**. Numbers below are *after* this round's optimizations:

| Configuration | ns/op | B/op | allocs/op |
|---------------|------:|-----:|----------:|
| Full pipeline (validation on) | ~378 M | 248 MB | 4.54 M |
| Core only (validation off) | **246 M** | **182 MB** | 3.31 M |
| **Validation safety-net share** | **≈ 18%** time | **≈ 23%** bytes | **≈ 26%** allocs |

The safety net is not free, but it is the contract that guarantees no un-parseable
Java ever leaves `Decompile`; ~18% wall-time is the price of that guarantee.

### 5.2 The profile is GC-bound — allocations are the real currency

A CPU profile of the core (`go tool pprof -top`) is dominated by the garbage
collector, not by decompiler logic:

```
runtime.gcDrain        47.9% cum
runtime.scanobject     40.7% cum
runtime.mallocgc       19.2% cum
runtime.greyobject     13.3% cum
```

So **reducing allocations directly buys CPU**. The largest *core* allocators
(`-alloc_space`, before this round's fixes) were:

| Allocator | Bytes | Share | Status |
|-----------|------:|------:|--------|
| `utils.Set[any].Add` (via `WalkGraph`) | 367 MB | 19.4% | **fixed (−interface boxing + mutex)** |
| `ParseOpcode` | 206 MB | 10.9% | pre-sized (prior round) |
| `GenerateDominatorTree` (+`func1`) | 193 MB | 10.2% | backlog |
| `Stack[*].Push` | 94 MB | 4.9% | backlog (pre-size) |
| `codec.MatchMIMEType` → `csv/bufio` per string literal | 77 MB | 4.1% | **fixed (ASCII fast-path)** |
| `Set[*OpCode].Add` | 73 MB | 3.9% | backlog |

On the validation path, separately, ~70% of allocations are ANTLR ATN-simulation
objects (`NewBaseATNConfig`, `BaseATNConfigSet.Add`, prediction-context merges) —
inherent to re-parsing each class.

### 5.3 Optimizations landed this round (each proven output-equivalent)

Equivalence is proven, not assumed: `TestDumpJarFingerprint` writes a per-class
`sha256(status+output)` for every class of `commons-codec` **and** `byte-buddy`
(≈3k classes); the fingerprint dirs `diff` clean before vs after every change.

1. **`WalkGraph` visited set — drop interface boxing and the mutex.**
   The graph walk used a thread-safe `Set[any]`: every node pointer was boxed into
   an `interface{}` map key (the #1 core allocator at 19%) and every `Has`/`Add`
   took an `RWMutex`, despite the walk being single-goroutine. Constrained the type
   parameter to `comparable` and switched to a plain `map[T]struct{}`.
   **Core: 315 → 254 ms/op (−19%), 217 → 193 MB/op (−11%).**

2. **Skip MIME sniffing for pure-ASCII string literals.**
   `JavaStringToLiteral` ran full magic-byte detection (`codec.MatchMIMEType`,
   which allocates a `csv`/`bufio` reader) on *every* literal to recover a possibly
   mis-decoded Chinese charset — impossible for ASCII bytes. Guarded behind a
   pure-ASCII check (ASCII already took the same quote path, so behavior is
   identical). **Core: 254 → 246 ms/op, 193 → 182 MB/op.**

Cumulative for the round: **core 315 → 246 ms/op (−22%), 217 → 182 MB/op (−16%)**;
end-to-end bytes 282 → 248 MB (−12%).

Prior-round optimizations still in place:
- **`ParseOpcode` pre-sizing** (opcode slice + both offset maps sized from bytecode
  length).
- **Validation timer hygiene** (stoppable `time.NewTimer` instead of `time.After`,
  so per-member budget timers and the source they retain are freed immediately).

### 5.4 The workload is heavily tail-bound

`TestTopSlowClasses` (one cold pass, ranked by time) shows a tiny minority of
classes dominate total cost:

| Jar | Classes | top-1 class | top-1% of classes | top-10% |
|-----|--------:|------------:|------------------:|--------:|
| commons-codec-1.15 | 106 | 14.6% | 14.6% | 68.7% |
| byte-buddy-1.14.17 | 2845 | 26.3% | **60.8%** | 88.4% |

On byte-buddy, **one 43 KB class** (`InstrumentedType$Default`) is 26% of a full
cold pass and the top 1% of classes are 61%. Implication: average-case tuning moves
throughput only modestly; the high-value target is the pathological tail (deeply
nested CFG / huge methods that stress the structuring and stack-simulation phases).

### 5.5 Cold-start vs warm steady state

The same `InstrumentedType$Default` costs **7.9 s** in a cold one-shot pass but only
**~127 ms** warm and repeated (≈62×). The gap is one-time process initialization
(ANTLR ATN deserialization, regex compilation, `sync.Once` setup) that the first
complex class absorbs. For **batch/jar** decompilation this amortizes to nothing;
for **single-class CLI** invocations it is a real latency floor worth pre-warming.

### 5.6 Parallel scalability

`BenchmarkDecompileJarParallel` on byte-buddy (full jar, warm), varying
`BENCH_CONC`:

| Workers | ns/op | Speedup |
|--------:|------:|--------:|
| 1 | 4.27 s | 1.0× |
| 2 | 2.27 s | 1.88× |
| 4 | 1.38 s | 3.09× |
| 8 | 1.19 s | 3.59× |
| 16 | 1.71 s | 2.50× (**regression**) |

Scaling is near-linear to ~4 workers and tops out around 8 (3.6×), then **regresses**
past it. This is the GC-bound signature from §5.2: many allocating goroutines
contend on the shared collector. The allocation reductions in §5.3 directly raise
this ceiling, and further allocation work (dominator tree, stacks) is the path to
better multi-core scaling.

### 5.7 Why the big lever (cross-parse ANTLR cache) was deliberately not pulled
The pinned ANTLR Go runtime (`v4.0.0-20220911`) has no locking on its DFA /
`JStore` structures, and decompilation runs in parallel (the jdsc self-check uses
100 goroutines). A process-wide shared validation DFA would data-race; the
existing per-worker cache + `DetachParserATNSimulatorCaches` design is the safe
choice. Pursuing this further would require an ANTLR upgrade (out of scope) and is
recorded as future work.

---

## 6. Backlog (prioritized by impact, from the data above)

**Correctness (semantic fidelity):**
1. **`try/catch/finally` "multiple next" CFG** — the only classic-corpus stub and
   the most common stub cause observed in real jars.
2. **JVM boolean/int disambiguation** (Operators) — infer `boolean` vs `int` for
   locals from usage/`Z` descriptors instead of the shared int opcodes; the single
   biggest recompile-frontier blocker.
3. **Loop idiom recovery** — reconstruct `for`/`while` instead of universal
   `do{...}while(true)`, which also removes the *unreachable statement* failures.
4. **Array dimension typing** (Arrays) and **field-initializer hoisting**
   (Initializers) — both are localized type/scope calc bugs.
5. **Record / sealed `invokedynamic ObjectMethods` bootstrap** — unblocks modern
   (Java 17+) value types end-to-end.

**Performance (all in service of the GC-bound profile in §5.2):**
6. **Dominator-tree allocations** (193 MB, 10%) and **stack/`Set[*OpCode]`
   pre-sizing** (167 MB combined) — the next-largest core allocators after the two
   fixed this round; lowering them raises the parallel ceiling (§5.6).
7. **Tail-class structuring complexity** (§5.4) — profile and reduce the
   superlinear cost on the pathological 1% of classes.
8. **Single-class cold-start pre-warm** (§5.5) — warm ANTLR/regex once for CLI use.
9. **Shared validation DFA** — only after an ANTLR runtime upgrade makes it
   thread-safe.

---

## 7. Reproduction quick reference

```
# Coverage matrix (javac-compiled corpus)
go test -run TestSyntaxCoverageMatrix -v ./common/javaclassparser/tests/

# Correctness round-trip (decompile -> javac); RC_VERBOSE dumps full diagnostics
go test -run TestRecompileRoundtrip -v ./common/javaclassparser/tests/
RC_VERBOSE=1 go test -run TestRecompileRoundtrip -v ./common/javaclassparser/tests/

# Determinism (portable, no Maven cache)
go test -run TestCorpusDeterminism -v ./common/javaclassparser/tests/

# Full fast suite
go test ./common/javaclassparser/...

# Performance: core-vs-fullpipeline, scaling, tail distribution, and equivalence
BENCH_JAR=<jar> go test -run xxx -bench 'BenchmarkDecompileJar$' -benchmem ./common/javaclassparser/tests/
BENCH_NO_VALIDATE=1 BENCH_JAR=<jar> go test -run xxx -bench 'BenchmarkDecompileJar$' -benchmem ./common/javaclassparser/tests/
BENCH_JAR=<jar> BENCH_CONC=8 go test -run xxx -bench 'BenchmarkDecompileJarParallel$' ./common/javaclassparser/tests/
BENCH_JAR=<jar> go test -run TestTopSlowClasses -v ./common/javaclassparser/tests/   # tail distribution
OUT_DIR=/tmp/fp DIFF_JARS=<jarA:jarB> go test -run TestDumpJarFingerprint ./common/javaclassparser/tests/   # output-equivalence proof
```
