# YAK JAVA DECOMPILER ENGINEERING BENCHMARK

> Language: **English** | [简体中文](./YAK_DECOMPILER_BENCHMARK.zh-CN.md)

Reproducible evaluation of the Yaklang Java decompiler (`java.Decompile` /
`javaclassparser.Decompile`) across **syntax safety**, **reconstruction
coverage**, **javac round-trip correctness**, **determinism**, **test hygiene**,
and **allocation cost**. Every number below is produced by a reproducible test or
benchmark in this repository (no synthetic or hand-waved figures) and can be
regenerated with the commands shown in each section.

- Decompiler entry points: `javaclassparser.Decompile([]byte) (string, error)`
  and the Yaklang library wrapper `java.Decompile`.
- Host used for the figures: darwin/arm64, Go 1.22.12, OpenJDK `javac` present.
- Reproduce everything fast (no network, no local Maven cache required):
  `go test ./common/javaclassparser/...`

> **Scope note.** "No stub" is **not** the same as "correct reconstruction", and
> parsing under ANTLR is **not** the same as recompiling under `javac`. This
> report therefore separates three distinct claims: (1) the output is
> syntax-parseable, (2) the output is produced without a degraded stub, and (3)
> the output recompiles under `javac`. Only (3) is evidence of semantic fidelity.

---

## 1. Executive summary

This report evaluates the Yaklang Java decompiler across syntax safety,
reconstruction coverage, `javac` round-trip correctness, determinism, test
portability, and allocation cost. The implementation is a best-effort,
partially fault-tolerant source-reconstruction component suitable for
interactive inspection and security-analysis workflows. It is **not a
source-equivalent Java decompiler** and should not be treated as the sole
authority for automated semantic decisions.

| Axis | Result | How it is measured |
|------|--------|--------------------|
| Syntax safety (parse-or-degrade) | 31/31 corpus groups produce **syntax-parseable Java**; 0 syntax errors, 0 hard errors, 0 panics | `TestSyntaxCoverageMatrix` |
| Reconstruction coverage (no stub) | 31/31 groups emit **non-degraded output** (zero stubs across classic and modern corpora) | `TestSyntaxCoverageMatrix` |
| Correctness (javac round-trip) | **26/26** eligible corpora recompile cleanly; 0 fail, 0 stub, 0 decompile error | `TestRecompileRoundtrip` |
| Real-jar correctness (.m2 corpus) | over 80 jars / ~12000 classes: **ok=11830, partial=170, syntax=0, err=0**; a per-class sha256 fingerprint diff verifies byte-identical output across runs | `TestM2RegressionHarness` |
| Determinism | byte-identical output across repeated decompiles; performance changes are guarded by per-class sha256 fingerprints | `TestCorpusDeterminism`, `TestDumpJarFingerprint` |
| Test suite | green & fast: `./...` ≈ 22s, no machine-specific dependencies | `go test ./common/javaclassparser/...` |
| Allocation cost | core **≈215 ms** and **≈161 MB cumulative heap allocation** per 106-class jar; the post-decompile ANTLR re-parse adds ≈ +60% runtime and ≈ +42% bytes relative to core-only | `BenchmarkDecompileJar` |
| Scalability | near-linear to ~8 workers (3.6×), then **GC-bound regression** | `BenchmarkDecompileJarParallel` |

The decompiler's **safety guarantee holds**: for every input in the corpus it
either reconstructs a method or degrades it to a tagged, still-parseable stub
(`yak-decompiler:` marker), never emitting un-parseable Java and never panicking
out of `Decompile`.

### Round-trip correctness detail

All 26 corpus groups eligible for strict `javac` round-trip validation (22
single-class groups plus 4 multi-class inner/nested-class groups) recompile
successfully: Annotations, Arrays, Boundary, CastsInstanceof, ComplexExpressions,
ComplexMisc, Concurrency, ControlFlow, ControlFlowEdge, Enums, Exceptions,
ExceptionsComplex, FieldsAndArrays, Generics, Inheritance, Initializers,
InnerClasses, Lambdas, Literals, Loops, NestedControlFlow, NumericEdge,
Operators, Strings, Switches, TryWithResources. There are **0 recompile
failures, 0 stubs, and 0 decompile errors** in this set.

The four multi-class groups recompile end to end, exercising inner-class
reconstruction: synthetic `access$NNN` bridges, `this$0` outer references, `val$`
capture fields, interface `default` methods, `@interface` annotation types, and
enum synthetic suppression with explicit constant arguments.

### Readiness assessment

The decompiler meets the bar of an **engineering beta** for best-effort code
presentation, provided that: degraded methods remain explicitly tagged;
downstream analysis does not assume semantic equivalence from syntax-valid
output; and resource limits plus untrusted-input fuzzing are added before
exposure to hostile inputs. General-availability readiness requires further
improvement in real-world jar coverage (the remaining real-jar partials),
malformed-input resilience, and peak-resource characterization.

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

### Classic corpus (Java 8 bytecode) — 26 groups
```
ok=26  stub=0  syntax=0  error=0  panic=0
```

### Modern corpus (Java 17 bytecode) — 5 groups
```
ok=5  stub=0  syntax=0  error=0  panic=0
```

### Coverage conclusion
Both corpora emit **zero stubs** — every member of every group reconstructs to
real Java rather than degrading. Operators, literals, control flow, loops,
switches, try-with-resources, arrays, generics, inheritance, inner classes,
enums, lambdas, strings, annotations, initializers, concurrency,
casts/instanceof, pattern matching, switch expressions, text blocks, records and
sealed types all produce **syntax-parseable** source for the tested corpus.
Syntax-parseable is a weaker claim than `javac`-recompilable; see §3 for the
round-trip results that measure semantic fidelity.

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
`-nowarn -Xlint:none`) so diagnostics are stable across machines. Run with
`RC_VERBOSE=1` to dump the decompiled source plus every `javac` error per
category.

### Corpus round-trip results
The oracle decompiles **every** class of a group (including inner, nested,
anonymous and local classes) and recompiles the units together, so inner-class
reconstruction is exercised end to end rather than skipped.
```
recompile-ok:  26  (Annotations, Arrays, Boundary, CastsInstanceof, ComplexExpressions,
                    ComplexMisc, Concurrency, ControlFlow, ControlFlowEdge, Enums,
                    Exceptions, ExceptionsComplex, FieldsAndArrays, Generics, Inheritance,
                    Initializers, InnerClasses, Lambdas, Literals, Loops, NestedControlFlow,
                    NumericEdge, Operators, Strings, Switches, TryWithResources)
recompile-fail: 0
stub:           0
dec-err:        0
multiclass:     0   (compiled together, not skipped)
```

Every passing category is pinned by `recompileGateBaseline`, so any regression
that breaks a green category fails CI.

### What the round-trip covers
Each corpus group exercises a distinct construct family and is validated end to
end:

- **Control flow**: `if/else` chains, `switch` (fall-through, `String` switch,
  sparse lookup vs dense table, default-in-the-middle), nested loops with
  `break`/`continue`, labeled `break`/`continue` across multiple levels,
  `while(true)`+break, do/while, three-level loop nesting.
- **Expressions and operators**: mixed `int/long/float/double` promotion,
  full-width bit manipulation on `long` (`& | ^ << >> >>> ~`), boolean vs
  integer disambiguation for `&`/`|`/`^`, short-circuit `&&`/`||` both as
  conditions and as returned/stored boolean values, deep right-leaning chained
  ternaries, `instanceof`+cast dispatch chains, cast precedence on member access.
- **Numeric edges**: integer overflow wrap-around, shift counts at/beyond the
  type width (`<<32`, `>>>33`), compound assignment with implicit narrowing,
  hex/binary/octal/underscore literals, `char` arithmetic, `float`/`double`
  special values (`NaN`, `±Infinity`), and numeric-literal suffixes
  (`9223372036854775807L`, `3.14F`, `2.718281828D`).
- **Fields and arrays**: instance/static fields, compound assignment and
  pre/post increment on field array elements (`this.buf[i] *= 2`),
  multi-dimensional and jagged arrays, array initializers, correct
  `multianewarray` rank, array-typed field rendering, blank `final` fields.
- **Exceptions**: `try/catch/finally`, nested try/catch/finally, single- and
  multi-resource try-with-resources, multi-catch (`A | B`), rethrow,
  `finally` after `return`. The `finally` body is reconstructed in its faithful
  desugared form (duplicated on each exit path plus a `catch (Throwable)`
  rethrow), exactly as the bytecode executes.
- **Types and members**: generics with null-initialized slot type widening,
  inheritance, interface `default` methods, `@interface` annotation types, full
  enum reconstruction (synthetic `values()/valueOf()/$VALUES` suppression,
  constructor synthetic-prefix stripping, explicit constant arguments), inner /
  nested / anonymous / local classes, lambdas with isolated parameter scope and
  generic-signature recovery, concurrency (`synchronized` on `this`/fields).
- **Pre-Java-6 bytecode**: `try/finally` compiled with `jsr`/`ret` subroutines
  is inlined to the modern duplicated-finally form before structuring, so legacy
  jars decompile instead of degrading (see §3.1).

> **Known semantic limitation (not a recompile failure).** `Loops.labeled`
> recompiles cleanly, but a `continue <label>` whose target is an outer `for`
> loop's *increment* can be dropped when that increment node is shared with the
> loop's natural exit edge: the `do{...}while(true)` lowering can place the
> shared increment statement (`i++`) on only one successor path, so the other
> path (the `continue outer` branch) renders as an empty `if` body. This compiles
> but can diverge at runtime for that specific labeled-continue idiom. It is
> tracked under "loop idiom recovery" in the backlog; the loop-semantics
> round-trip battery (`TestLoopSemanticsRoundTrip`, which executes and compares
> fingerprints) covers every non-labeled shape and passes.

### 3.1 Real-jar validation (.m2 corpus)
Beyond the synthetic corpus, the decompiler is validated against a real Maven
cache. `TestM2RegressionHarness` runs over 80 jars / ~12000 classes and writes a
per-class sha256 fingerprint:

```
ok=11830  partial=170  syntax=0  err=0
```

`syntax=0` and `err=0` mean no class produces un-parseable Java and no decompile
returns an error or panics; `partial` counts classes where at least one member
degraded to a tagged stub. Pre-Java-6 `try/finally` subroutines (`jsr`/`ret`)
are inlined by `core/jsr_inline.go`: the finally body is duplicated at each `jsr`
call site, `ret` becomes a `goto`, jsr back-edges are redirected, and try/catch
exception entries nested inside the finally are cloned per call site. The pass
validates the whole shape **before** any mutation and conservatively leaves any
non-canonical form (`jsr_w`/`goto_w`/`switch` wide targets, exception entries
straddling a subroutine boundary, 16-bit offset overflow, etc.) untouched —
degrading to a stub rather than emitting wrong code — and is a no-op for methods
without `jsr`/`ret`. A `JSR_INLINE_OFF` kill-switch reverts to the old behavior.
The remaining 170 partials are the real-jar reduction frontier tracked in the
backlog.

### What "partial" / "stub" does **not** mean
A stubbed member is still surrounded by **structurally decompiled, readable,
syntax-parseable Java** for the rest of the class, and the stub itself is
explicitly tagged (`yak-decompiler:` marker) so downstream tools can detect it.
A degraded member is never silently replaced with plausible-but-wrong code:
for a security tool, a clearly-marked stub is strictly better than a
compilable-but-incorrect reconstruction.

---

## 4. Test-hygiene benchmark

Goal: a stable, fast, portable core suite with no machine-specific or
time-wasting tests, while keeping real coverage.

```
go test ./common/javaclassparser/...      # green, ~22s total
```

Properties of the suite:
- **Machine-specific diagnostics are gated** behind env vars (`BENCH_JAR`,
  `JDSC_DIR`, `M2_DETERMINISM`), so the default run never scans `~/.m2` or
  `/tmp/...` and completes in ~22s with no external dependencies.
- **Portable determinism check**: `TestCorpusDeterminism` verifies byte-identical
  output without needing a local Maven cache.
- **Corpus is source, not bytecode**: `tests/corpus/{classic,modern}` are `.java`
  files compiled by `javac` at test time, so fixtures are regenerated on the host
  and stay in sync with the running JDK.

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

`BENCH_NO_VALIDATE=1` turns off the post-decompile ANTLR re-parse and isolates
the **decompiler core** from the **safety net**:

| Configuration | ns/op | B/op | allocs/op |
|---------------|------:|-----:|----------:|
| Full pipeline (validation on) | ~343 M | 229 MB | 3.54 M |
| Core only (validation off) | **215 M** | **161 MB** | 2.28 M |
| **Validation safety-net share** | **≈ 37%** time | **≈ 30%** bytes | **≈ 36%** allocs |

The safety net is not free, but it is the contract that guarantees no un-parseable
Java ever leaves `Decompile`; ~36% wall-time is the price of that guarantee (it is an
ANTLR re-parse of the whole class, whose ATN-simulation allocations dominate that
share and are intrinsic to the third-party runtime).

### 5.2 The profile is GC-bound — allocations are the real currency

A CPU profile of the core (`go tool pprof -top`) is dominated by the garbage
collector, not by decompiler logic:

```
runtime.gcDrain        47.9% cum
runtime.scanobject     40.7% cum
runtime.mallocgc       19.2% cum
runtime.greyobject     13.3% cum
```

So **reducing allocations directly buys CPU**. The core is built to keep
allocations low and is proven output-equivalent by `TestDumpJarFingerprint`
(per-class `sha256(status+output)` diffs clean across changes). Allocation-aware
design choices currently in the core:

- `WalkGraph` uses a plain `map[T]struct{}` visited set (no interface boxing, no
  mutex — the walk is single-goroutine) and a slice-backed DFS stack.
- `GenerateDominatorTree` reuses one scratch bitset across fixed-point sweeps and
  builds each idom's child slice at its exact final capacity in a two-pass
  count-then-fill (no incremental `append` growth, no per-idom sort closure).
- `CalcMergeOpcode`, `ScanJmp` and `DropUnreachableOpcode` use plain maps and
  reuse buffers instead of mutex-guarded `Set[*OpCode]`.
- `CalcOpcodeStackInfo` pre-sizes `opcodeToSim` and `nodeToVarScope` to
  `len(d.opCodes)` (exactly one entry per opcode).
- `fixJavaStringEscapes` uses package-level precompiled regexes; pure-ASCII
  string literals skip MIME sniffing entirely.
- `ParseOpcode` pre-sizes the opcode slice and both offset maps from bytecode
  length; `DumpClass.assemble` uses `strings.Builder` (O(n), not O(n²)).

On the validation path the bulk of allocations are ANTLR ATN-simulation objects
(`NewBaseATNConfig`, `BaseATNConfigSet.Add`, prediction-context merges) —
inherent to re-parsing each class and not addressable without an ANTLR runtime
change.

### 5.3 The workload is heavily tail-bound

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

### 5.4 Cold-start vs warm steady state

The same `InstrumentedType$Default` costs **7.9 s** in a cold one-shot pass but only
**~127 ms** warm and repeated (≈62×). The gap is one-time process initialization
(ANTLR ATN deserialization, regex compilation, `sync.Once` setup) that the first
complex class absorbs. For **batch/jar** decompilation this amortizes to nothing;
for **single-class CLI** invocations it is a real latency floor worth pre-warming.

### 5.5 Parallel scalability

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
contend on the shared collector. Further allocation reductions are the path to a
higher multi-core ceiling.

### 5.6 Why the big lever (cross-parse ANTLR cache) is deliberately not pulled
The pinned ANTLR Go runtime (`v4.0.0-20220911`) has no locking on its DFA /
`JStore` structures, and decompilation runs in parallel (the jdsc self-check uses
100 goroutines). A process-wide shared validation DFA would data-race; the
existing per-worker cache + `DetachParserATNSimulatorCaches` design is the safe
choice. Pursuing this further would require an ANTLR upgrade (out of scope) and is
recorded as future work.

---

## 6. Backlog (prioritized by impact, from the data above)

**Correctness (semantic fidelity):**
1. **Real-jar partial reduction** — drive the 170 remaining `.m2` partials toward
   zero by diagnosing the per-class stub reasons that survive on real-world
   bytecode (the synthetic corpus is already at 0 stubs / 0 round-trip failures).
2. **Loop idiom recovery** — reconstruct `for`/`while` instead of the universal
   `do{...}while(true)` lowering. This would fix the `labeled`
   `continue <outer-increment>` semantic limitation (a shared increment node the
   do-while model can place on only one successor) and improve readability.
3. **Idiomatic `finally` folding** — the `try/catch/finally` round-trip is
   correct today via the faithful desugared form (duplicated finally body plus a
   `catch (Throwable)` rethrow, exactly as the bytecode runs). A future pass can
   collapse this into a single idiomatic `finally {}` block for readability.
4. **Untrusted-input hardening** — resource limits and malformed-input fuzzing
   before exposure to hostile inputs.

**Performance (all in service of the GC-bound profile in §5.2):**
5. **Further allocation reduction** in the structuring and stack-simulation
   phases to raise the parallel ceiling (§5.5).
6. **Tail-class structuring complexity** (§5.3) — profile and reduce the
   superlinear cost on the pathological 1% of classes.
7. **Single-class cold-start pre-warm** (§5.4) — warm ANTLR/regex once for CLI use.
8. **Shared validation DFA** — only after an ANTLR runtime upgrade makes it
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
