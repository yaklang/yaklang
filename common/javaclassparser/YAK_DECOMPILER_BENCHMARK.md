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
| Real-jar correctness (.m2 corpus) | 120 jars / 12000 classes: **ok=11965, partial=35, syntax=0, err=0, panic=0**; a per-class sha256 fingerprint diff verifies byte-identical output across runs | `TestM2RegressionHarness` |
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
cache. `TestM2RegressionHarness` runs over 120 jars / 12000 classes and writes a
per-class sha256 fingerprint:

```
ok=11965  partial=35  syntax=0  err=0  panic=0
```

`syntax=0`, `err=0` and `panic=0` mean no class produces un-parseable Java and no
decompile returns an error or lets a panic escape; `partial` counts classes where
at least one member degraded to a tagged stub. Pre-Java-6 `try/finally` subroutines (`jsr`/`ret`)
are inlined by `core/jsr_inline.go`: the finally body is duplicated at each `jsr`
call site, `ret` becomes a `goto`, jsr back-edges are redirected, and try/catch
exception entries nested inside the finally are cloned per call site. The pass
validates the whole shape **before** any mutation and conservatively leaves any
non-canonical form (`jsr_w`/`goto_w`/`switch` wide targets, exception entries
straddling a subroutine boundary, 16-bit offset overflow, etc.) untouched —
degrading to a stub rather than emitting wrong code — and is a no-op for methods
without `jsr`/`ret`. A `JSR_INLINE_OFF` kill-switch reverts to the old behavior.
The remaining 40 partials are the real-jar reduction frontier tracked in the
backlog.

### 3.2 Catch-handler structuring fix (real-jar partial reduction)

A first round of diagnosis and fixing landed against backlog #1 (real-jar partial
convergence). `TestM2StubReasons` (`STUB_REASONS=1 M2_MAX_JARS=120
M2_MAX_CLASSES=12000`) attributes each residual stub to a CFG family; the largest
bucket was **"try-region structuring failed: try without catch handler"** (61
stubs, ~39% of all stubs at the time).

Root cause: `rewriter.TryRewriter` distinguished the try body from catch handlers
by the **position** in the try node's successor list (assuming `node.Next[0]` is
the try body and `Next[1..]` are catch handlers). But later CFG passes
(`RemoveGotoStatement`, loop/if structuring, node-id regeneration) can reorder
the successor list. When the order was reversed (catch handler before the try
body), the real try body was fed into the catch slot, where its first statement
was not the caught-exception store and so got dropped, leaving a try with zero
catch handlers — flagged as a corrupted body and degraded to a stub.

Fix: identify catch handlers **by content** rather than position. Every catch
handler's structured body opens with the synthetic caught-exception store (`<var>
= <exception placeholder>`, the `Flag=="exception"` CustomValue pushed at the
handler PC by the stack simulation). The single non-handler successor is the try
body; the rest are catches. The new classification only activates when there is
exactly one try-body candidate, otherwise it falls back to the original
positional scheme to minimize regression risk.

Same-config before/after (120 jars / 12000 classes; `STUB_REASONS` and
`TestM2RegressionHarness` count `partial` identically):

```
before: classes=12000  partial=127  stubs=157
after:  classes=12000  partial=74   stubs=91
```

The "try without catch handler" bucket dropped **61 → 0**, and
"post-decompile syntax validation failed" dropped 22 → 18 as a side effect. The
synthetic corpora remain 0-stub / 0 round-trip failures (full suite green), and
the regression case `ternary_in_try.class` — previously degraded to a stub by
this exact defect — now decompiles fully and correctly (its regression guard was
updated to lock in the correct behavior).

### 3.3 Generic field-type rendering fix (syntax → 0)

After the first round `TestM2RegressionHarness` (120 jars / 12000 classes) still
reported `syntax=5` — a few classes emitting **non-stub but syntactically invalid**
Java (worse than a stub, since it produces un-parseable code). Root cause:
`DumpFields` rendered the field type with `fieldType.String(c.FuncCtx)` (which
already registers imports and performs short-name / FQN disambiguation through
`ShortTypeName` internally) and then **redundantly** re-ran `Import` /
`ShortTypeName` on that whole rendered string. When a type argument is forced to
FQN disambiguation (e.g. field `Set<java.util.logging.Logger>`, with `Logger`
kept fully qualified to avoid clashing with an imported `ch.qos...Logger`), the
rendered string contains dots: `SplitPackageClassName` split it on `.` into a
bogus package `Set<java.util.logging` + class `Logger>`, emitting
`import Set<...Logger>;` and collapsing the field type to `Logger>` —
un-parseable, so the field was dropped and the bad import leaked.

Fix: for non-array fields use the `fieldType.String(c.FuncCtx)` result directly,
dropping the redundant `Import`/`ShortTypeName` second pass. Result:

```
syntax: 5 → 0
```

Guarded by the `generic_field_type.class` regression case (field
`Map<java.util.Date, java.sql.Date>`, second `Date` forced to FQN).

### 3.4 Crash hardening for incomplete stack simulation

The panic bucket (`ParseBytesCode panic: nil pointer`, ~8) came from incomplete
stack simulation producing **nil-typed** values that were dereferenced at several
sites: `StackSimulationImpl.AssignVar` (comparing `ref.Type()` vs `val.Type()`),
`SlotValue.ResetValue` (`val.Type().ResetTypeRef`), and `AssignStatement.String`
declaration rendering (`declType.String`). Nil guards were added at each: a
missing type falls back to the other side, and if still missing the member
degrades cleanly through the safety net instead of crashing the whole method.
This removes the panics (more robust, concurrency-safe) but methods that are
genuinely under-simulated (e.g. `matchPath`) still degrade to a stub (the panic
bucket merges into the empty-slot bucket), so the total partial count does not
drop from this change alone.

### 3.5 Principled merge-value reconstruction rewrite (empty-slot bucket → 0)

A second round targeted the then-largest residual bucket, **"incomplete stack
simulation (empty slot)" (36)**, with a principled rewrite.

Root cause: in javac bytecode, a value that survives on the operand stack across
a control-flow merge ⇔ a ternary `?:` or short-circuit `&&`/`||` (and nested
binary trees thereof) in source. The old implementation, in a single `WalkGraph`
pass, first `Push`ed a placeholder `SlotValue` at nodes judged to be if-merges,
then tried to back-fill via two fragile back-end paths (a structural probe + a
legacy chain walk); if neither filled the slot, the empty slot leaked as an
`EmptySlotValuePlaceholder`, was detected by the dumper, and degraded the whole
method to a stub. Its "push placeholder first, try to reconstruct, leak on
failure" shape lacked the "only build a tree when it is reconstructible"
invariant.

Fix: a new `buildSharedLeafTernary` recursively builds the ternary tree rooted at
the merge node via the dominator relation ("both branches ultimately reach the
merge"). `firstReconverge` (bidirectional BFS minimizing summed distance) locates
the nearest reconvergence of a condition's two arms, and `isInnerValueTernary`
(reconvergence point is itself a `valueMergeSet` member) distinguishes an *inner
value ternary* from a *true outer condition*, so inner sub-expressions are not
mistaken for independent conditions. Boolean-valued ternary trees are then
rewritten by `boolReduce` into idiomatic `&&`/`||`/`!`: algebraic simplification
of literal arms (`c ? true : false ⇒ c`, `c ? true : B ⇒ c || B`, …) and
shared-leaf factoring (`c ? (A || S) : S ⇒ (c && A) || S`). `boolReduce` uses
structural literal detection plus pointer-identity comparison (not `String()`
comparison) for linear complexity.

To stop the new builder from duplicating a large shared *value* subtree into
ternary arms on complex type-dispatch chains (e.g. `deepEquals`'s big `instanceof`
dispatch) — which would explode output size and trip post-syntax — the builder
**adopts only boolean-literal shared leaves** (`iconst_0`/`iconst_1`); a
non-literal shared leaf is declined and falls back to the legacy path, which keeps
it as control flow. The legacy implementation is retained behind
`EnableLegacyMergeReconstruction` (default false) for one-switch rollback.

Same-config (120 jars / 12000 classes) before/after:

```
before: partial=74  (empty-slot 36 + multiple-next 28 + post-syntax 18 + panic, etc.)
after:  partial=40  (empty-slot 0  + multiple-next 29 + post-syntax 18 + panic 6 + other 3)
```

The "empty slot value" bucket goes **36 → 0**, `syntax`/`err` stay 0, `ok` rises
to 11960, the full suite and `recompile_roundtrip` stay green, and performance
returns to baseline (~160s for the full 120-jar config). Locked by the
`empty_slot_stub.class` regression test (asserting the reconstructed boolean
short-circuit expressions and the absence of stub markers).

### 3.6 Panic bucket → 0 (stack-simulation nil-type / stack-underflow hardening)

A third round hardened the **panic bucket** (stack simulation emitting a
value with a nil type that is dereferenced during rendering/construction, or an
operand-stack underflow) into a contract across the whole corpus. Each real panic
site was located with an env-gated native-stack capture (`DEC_PANIC_STACK`, off by
default) and given a nil/underflow guard:

- `FunctionCallExpression.String` argument-cast logic: a nil `arg.Type()` skips the
  cast (comma-ok assertions are nil-safe), rendering the argument as-is (ant
  `SelectorUtils.matchPath`).
- `NewBinaryExpression` / `NewUnaryExpression`: a nil result type falls back via
  `nonNilType` to an operand type then int; `ResetType` runs only on non-nil types
  via `resetTypeSafe` (ant `CBZip2InputStream`).
- `NewConditionStatement` boolean-compare folding guarded by `isBoolPrimer` (ant
  `FileUtils`).
- `MergeTypes` drops nil arm types instead of calling `String()` on a nil
  `JavaType` (bndlib `HeaderReader`).
- `NewJavaArrayMember` / `JavaArrayMember.Type()`: a nil base type degrades to a
  plain member access (ant `CBZip2OutputStream`).
- `StackSimulationImpl.Peek/Pop`: an underflow returns an empty-slot `SlotValue`
  placeholder (cleanly degraded by the safety net) instead of `panic("Stack is
  empty")` (Groovy-exotic bytecode such as logback `NestingType.$INIT`).

Same-config (120 jars / 12000 classes) before/after:

```
before: ok=11960  partial=40  (multiple-next 29 + post-syntax 18 + panic 6 + other)
after:  ok=11965  partial=35  (multiple-next 28 + post-syntax 18 + other; panic 0)
```

The panic bucket goes **6 → 0**: for every input in the corpus, `Decompile`
neither returns an error nor lets a panic escape (more robust, more concurrency-
safe) — one of the GA safety floors. Five previously-panicking classes now fully
decompile (`ok` +5); one (`$INIT`) becomes a clean empty-slot stub. Locked in CI by
`TestGAPanicFreeBoundary` (six embedded real boundary classes asserting no panic,
valid syntax, and no stub for the fixed ones).

> Current state (120 jars / 12000 classes): **`ok=11965`, `partial=35`,
> `syntax=0`, `err=0`, `panic=0`**. The remaining partials are dominated by
> ParseBytesCode "multiple next" (28) and post-decompile syntax validation (18,
> mostly unstructured branches leaking a bare `ConditionStatement` — same family as
> multiple-next). Both are deeper **CFG-structuring completeness** problems whose
> canonical shapes are now diagnosed: a try with multiple catches inside a loop
> body where the catches break out of the loop (logback `SocketNode.run`), and
> nested conditions sharing merge targets (ant `Exec.run`). They form the work
> surface for a unified, pattern-independent structuring engine.

### What "partial" / "stub" does **not** mean
A stubbed member is still surrounded by **structurally decompiled, readable,
syntax-parseable Java** for the rest of the class, and the stub itself is
explicitly tagged (`yak-decompiler:` marker) so downstream tools can detect it.
A degraded member is never silently replaced with plausible-but-wrong code:
for a security tool, a clearly-marked stub is strictly better than a
compilable-but-incorrect reconstruction.

### 3.7 Variable-fold nil-deref panic + early-return "multiple next" (real-jar partial reduction)

A fourth round removed two residual defect families surfaced by
`TestM2StubReasons` on a 120-jar / 24491-class `.m2` slice (validation off, so
method-dump failures are attributed directly rather than collapsed into a
post-syntax bucket):

1. **Variable-fold nil-pointer panic.** A typed-nil `*JavaRef` entered
   `varUserMap` as a key when `loadVarBySlot` loaded an uninitialized local slot
   (`GetVar` returns nil); the variable-fold walker then dereferenced
   `ref.VarUid`/`ref.Val` and panicked, crashing the whole decompile through the
   recover net. The fold entry now surfaces this as an ordinary error so the
   method degrades to a tagged stub instead of a Go panic, and the INVOKESPECIAL
   `<init>` arm-store path is hardened against the same typed-nil ref plus a nil
   `VarFoldRule`. Hits beetl `FloatingIOWriter.<init>` and fastjson2
   `TypeUtils.doubleValue`.

2. **Early-return "multiple next".** `IfRewriter` wired both an arm's early
   `return`/`throw` exit *and* the genuine fall-through continuation as `Next` of
   the structured if node, giving it two `Next` edges; the linear statement
   collector then aborted with `"multiple next"` and degraded the whole method.
   The rewriter now drops a method-exit terminator (return/throw — never
   break/continue, which are loop-control jumps with a real successor) from the
   if's endNodes when a real fall-through remains, so the if keeps a single
   linear successor. Reconstructs Jackson's `PropertyDeserializer.deserializeAndSet`
   (a core deserialization path) across all five `jackson-databind` versions, plus
   druid `OracleStatementParser`.

Same-config (120 jars / 24491 classes, validation off) before/after:

```
before: partial=42  stubs=30  (multiple-next 18 + invalid-stack-size 6 + empty-slot 2 + panic 2 + other 4)
after:  partial=36  stubs=24  (multiple-next 12 + invalid-stack-size 6 + empty-slot 2 + variable-fold(clean err) 2 + other 4)
```

The **panic bucket goes 2 → 0** on this slice (those two classes now degrade to
clean tagged stubs via the ordinary-error path instead of a Go panic), the
**multiple-next bucket goes 18 → 12** (`-6`), `syntax`/`err` stay 0, and the full
suite + `recompile_roundtrip` stay green. Locked in CI by `TestGAPanicFreeBoundary`
(`panic_nilref_*`) and `TestDecompileSyntaxRegression`
(`multiple_next_early_return`).

The residual `invalid-stack-size` family (6, all in fastjson2 `TypeUtils`/
`JSONPathSingleName` JSON-parser switch loops where reconverging arms carry
different operand-stack depths) is a deeper stack-simulation completeness problem;
it degrades cleanly to a tagged stub and is tracked in the backlog.

### 3.8 javac `assert`-guard corruption fold (post-syntax partial reduction)

A fifth round fixed the `assert`-heavy class family (e.g. backport-util-concurrent
`ArrayDeque.checkInvariants`, with three `assert`s). javac lowers `assert <c>;`
into a `$assertionsDisabled` guard + `throw new AssertionError()`. A single assert
reconstructs fine, but when several asserts share/overlap the same throw target
the value-merge structuring can leave an **orphaned `ConditionStatement(mentions
$assertionsDisabled)` immediately followed by its `throw new AssertionError()`**,
which renders as the fatal `if (<cond>);` (a bare condition as a top-level
statement) and stubs the whole method via post-decompile syntax validation.

`FoldAssertionGuards` (`rewriter/assert_fold.go`, run after the acyclic check in
`ParseBytesCode`) detects that exact orphaned pair — a `ConditionStatement` whose
condition renders with `$assertionsDisabled`, followed by a `throw AssertionError`
— and folds the throw into a real `if (<cond>) { throw ... }` body. It only acts
on that corrupted shape (it requires both the `$assertionsDisabled` mention and the
AssertionError throw), so already-correctly-structured asserts and ordinary code
are untouched. It runs *after* `AssertStatementsAcyclic` so a pathologically
deep/cyclic tree still degrades cleanly instead of blowing the fold's recursive
walk. Kill-switch: `ASSERT_FOLD_OFF=1`.

Effect (STOP_ON_FIRST probe of the 120-jar a-c slice): the first failing class
moves from `ArrayDeque` (class #239, `post-decompile syntax validation failed`)
past it to `logback NestingType` (class #1194, a different family), i.e. the
assert-corrupted classes in the leading range now fully reconstruct. Locked in CI
by `TestDecompileSyntaxRegression` (`assert_guard_cyclic`).

### 3.9 Switch-case operand-stack rebuild from the switch StackEntry (post-syntax partial → 0)

A sixth round cleared the next failing family: a switch whose case/default bodies
build on top of an operand stack that is **not** empty after the selector pop.
(Groovy-compiled enums do this routinely — `$INIT` lowers to
`selectConstructorAndTransformArguments` and threads a freshly-allocated object +
an args array through every switch arm via `dup_x1`/`dup2_x1` on top of
`[objarr, newexpr]`.) The bytecode simulator shared a **single** post-switch
operand-stack snapshot across all arms via `preRuntimeStackSimulation`, which a
single shared variable updated after *every* node. As soon as an earlier arm ended
with an empty stack (an `athrow`/`return`), that snapshot was clobbered, so later
case bodies started from a stale/empty stack, underflowed on the dup ops, and
leaked `empty slot value` placeholders that degraded the whole method to a stub.

`calcOpcodeStackInfo` (`code_analyser.go`) now rebuilds each case/default body's
operand-stack simulation from the switch instruction's **post-selector** `StackEntry`
(`code.Source[0].StackEntry`) instead of the shared variable. That snapshot is exactly
the state after the selector `Pop`, so it is correct for every arm independently and
immune to earlier arms clobbering it. The shared variable is removed. A
`DEBUG_EMPTYSLOT` env hook (mirrors `DEBUG_TRYNOCATCH`) prints the corrupted body
instead of stubbing, for faster triage.

Effect (STOP_ON_FIRST probe of the 120-jar a-c slice): the first failing class moves
from `logback NestingType` (class #1194, `incomplete stack simulation: empty stack
slot leaked into method body`) past it to `druid SchemaResolveVisitorFactory`
(class #3923, a different family), i.e. the Groovy switch family in the leading
range now fully reconstructs. Locked in CI by `TestDecompileSyntaxRegression`
(`groovy_constructor_switch`).

### 3.10 Open long-tail structural families (full-`.m2` sweep investigation)

A full-`~/.m2` sweep (1107 jars / ~760k classes, `M2_INDUSTRY=1`, `-timeout 2h`) shows the residual
partial rate around 0.4% (e.g. ~368 partials in 94000 scanned classes). The failures cluster into four
structural families, all rooted in loop / value-merge structuring rather than per-class quirks. The
investigation (upstream-source diffing, synthetic MVPs, in-decompiler probes) localized each so the next
iteration has a precise target instead of re-deriving it:

- **Cyclic / shared container statement** (the "unknown" bucket — reason slugs to empty). Caused by
  `AssertStatementsAcyclic` rejecting a container reachable from two parents. Confirmed shape: a
  `for(;;)` parser loop with `break`/`continue` in nested `else if` arms, followed by post-loop code
  (e.g. druid `TDDLHint.<init>`'s `if (functions.size() > 0) type = Function;`). The post-loop
  `IfStatement` object is attached BOTH at the switch/loop-tail level AND inside a nested `do-while`
  body (seen twice in the acyclic DFS at the same pointer), so it is a loop-exit mis-attribution that
  double-collects one node. The double-attachment originates in the loop/switch body builders, NOT the
  `copyIfBody` path (marking those nodes visited did not clear it). Repros: druid TDDLHint, jackson
  `UTF8DataInputJsonParser` (4 versions).
- **variable-fold: nil ref key in varUserMap** (fastjson2 `TypeUtils.doubleValue`, `getYear`,
  `findBestMethod`). A `loadVarBySlot` of an uninitialized slot produces a nil `*JavaRef` that becomes
  a varUserMap key; the fold walker derefs it. Naive fixes all regress: materializing a fresh var,
  skipping registration, or deleting/skipping the nil key in the fold all break `loopSwitchTail` and
  similar (a change at the fold site flips an "invalid if merge node" error two phases earlier — the
  method's varUserMap is per-method, so the cross-method coupling points at the shared `funcCtx` /
  param-init path, the real but not-yet-cracked root cause).
- **multiple next** (fastjson2 `seekLine`, jackson readers). A node retains two `Next` edges after
  structuring; tied to `break`/`continue` targeting different loop levels. Synthetic MVPs of the
  upstream source did NOT reproduce it, so it depends on the exact `javac` CFG topology, not the source
  shape alone.
- **post-decompile syntax validation failed** (druid `SchemaResolveVisitorFactory.resolve`, fastjson2
  writers). Residual `ConditionStatement`s (rendered as the bracket-less `if cond;`) nested inside a
  structured `IfStatement` body that `IfRewriter` did not recurse into, combined with loop-exit
  mis-attribution that pulls post-loop `if/else if` into the loop.

All four reduce to **loop-structuring mis-handles post-loop / cross-loop code when the loop body has
`break`/`continue`**, the decompiler's single hardest structural problem. Each is currently caught by a
safety net (panic-free recover + tagged stub), so the safety contract holds; clearing them needs
loop-rewriter surgery to (a) never double-attach a node and (b) keep post-loop code out of the loop
body — the prioritized GA work.

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
1. **Real-jar partial reduction** — drive the remaining `.m2` partials toward
   zero by diagnosing the per-class stub reasons that survive on real-world
   bytecode (the synthetic corpus is already at 0 stubs / 0 round-trip failures).
   Landed so far: round 1 catch-handler classification by content (§3.2, partials
   127 → 74, "try without catch handler" 61 → 0); round 2 principled merge-value
   reconstruction rewrite (§3.5, partials 74 → 40, "empty slot" 36 → 0); round 3
   panic-bucket contract hardening (§3.6, partials 40 → 35, panic 6 → 0, ok 11960 →
   11965, locked by `TestGAPanicFreeBoundary`); round 4 variable-fold nil-deref
   panic + early-return multiple-next (§3.7, validation-off slice partials 42 → 36,
   stubs 30 → 24, panic 2 → 0, multiple-next 18 → 12). Remaining frontier:
   multiple next (12), invalid stack size (6, JSON-parser switch-loop operand-stack
   reconvergence), post-decompile syntax (same unstructured-branch family as
   multiple-next) — CFG-structuring completeness problems awaiting a unified
   pattern-independent structuring engine. A separate
   **generic field-type rendering** defect (field types like `Set<...>` emitted as
   `import` statements) breaks a few classes' syntax and needs its own fix.
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
