# YAK JAVA DECOMPILER - STATUS & KNOWN CORE DEFECTS

> Language: **English** | [简体中文](./YAK_DECOMPILER_BENCHMARK.zh-CN.md)
>
> This document records the **current state** of the Java decompiler and the
> **core defects** that bound it. It is a status record, not a change log: the
> deep fixes for the core defects are tracked and attacked separately.

Entry points: `javaclassparser.Decompile([]byte) (string, error)` and the
Yaklang wrapper `java.Decompile`. Host for the figures: darwin/arm64, Go 1.22.12.

## 1. Current state

| Axis | Result | Status |
|------|--------|--------|
| Synthetic corpus (syntax) | 31/31 groups, 0 stub / 0 syntax error / 0 panic | GA |
| Synthetic round-trip (decompile -> javac) | 26/26 eligible groups recompile, 0 fail | GA |
| Hard-case families (switch / ternary / try-catch / inner-class) | all PASS | GA |
| Determinism | byte-identical output across repeated decompiles | GA |
| Safety contract | never panics out of `Decompile`, never hangs; degrades to a tagged `yak-decompiler:` stub | GA |
| Real-jar partials (`.m2`, last snapshot) | 120 jar / 12000 classes: ok=11965, partial=35, syntax=0, err=0, panic=0; full `~/.m2` ~0.4% partial | not GA |

The synthetic corpus is complete. The residual gap is entirely real-world
control flow, on which a small fraction of classes still degrade to a tagged,
still-parseable stub. Degradation is always explicit (`yak-decompiler:` marker),
never silently-wrong code.

Reproduce the green subset (no `~/.m2`):

```
go test -run 'TestSyntaxCoverageMatrix|TestRecompileRoundtrip|TestDecompileSyntaxRegression|TestSwitchHardCasesNoCorruption|TestTernaryHardCasesNoCorruption|TestTryCatchHardCasesNoCorruption|TestGAPanicFreeBoundary|TestDecompileCyclicStatementTreeNoCrash|TestCorpusDeterminism' \
  -count=1 ./common/javaclassparser/tests/
```

## 2. Core defect A: stack simulation has no CFG dataflow merge

This is the single root cause behind most residual real-jar partials. It is **not**
a per-class quirk; the per-class symptoms in section 4 are all downstream of it.

**Where.** `(*Decompiler).CalcOpcodeStackInfo` in
`common/javaclassparser/decompiler/core/code_analyser.go`.

**What.** Operand-stack and local-variable simulation walks the opcode CFG with a
single-pass DFS (`WalkGraph`, `core/utils.go`). A DFS does not visit every
predecessor of a join before the join, and it never **merges** the per-predecessor
variable tables / operand stacks at a control-flow join. So at a join (or any node
reached before one of its predecessors is simulated) the simulation state is
incomplete.

**Concrete failure chain** (fastjson2 `TypeUtils.doubleValue`,
`panic_nilref_typeutils.class`):

1. A `*load N` reads a local slot whose definition reached this point only via an
   un-simulated predecessor edge, so `GetVar(N)` returns nil.
2. `loadVarBySlot` registers that nil `*JavaRef` as a `varUserMap` key.
3. The variable-fold pass aborts with `nil ref key in varUserMap`; `parser.go`
   suppresses the error and continues on the unfolded (broken) graph.
4. Downstream rewriters (`ScanCoreInfo`, ...) then dereference nil branch nodes
   and the whole method degrades to a stub.

**Why the current code only masks it.** The landed guards (empty-if-merge-source
tolerance in `CalcOpcodeStackInfo`, nil-key suppression in `parser.go`, the
per-consumer nil guards in `statement_wrap.go`) all paper over a symptom of this
one gap. A direct local repair (synthesizing the provably-live local at the
uninitialized-slot load) does remove this class's stub, but it mutates shared
simulation state and **regresses** other classes (switch operand-stack rebuild:
`loopSwitchTail`, `doubleToBigInt`) - confirming the real fix must be at the
dataflow level, not at any single consumer.

**Real fix (tracked elsewhere).** Turn `CalcOpcodeStackInfo` into a proper
dataflow pass: process nodes in reverse-postorder with a worklist and, at every
join, **merge** the predecessor var-tables / operand stacks to a fixpoint (back
edges handled explicitly). This is a decompiler-core change that affects the
byte-for-byte output of every class, so it must be validated against the full
`javaclassparser` corpus before landing.

## 3. Core defect B: structuring can emit cyclic / shared containers

**Where.** The rewriter structuring stage (`decompiler/rewriter`, loop/if
structuring), surfacing in `AssertStatementsAcyclic` (`rewrite_var.go`).

**What.** On some real classes the structuring stage emits a self-referential
container (an `IfStatement` whose own body transitively contains itself) or
double-attaches a post-loop container both at the loop tail and inside a nested
`do-while` body. A self-cycle would drive every recursive tree walker
(`FoldAssertionGuards`, `rewriteVar`, `Statement.String`, ...) into Go's
**unrecoverable** `fatal error: stack overflow`.

**Current containment.** `AssertStatementsAcyclic` runs iteratively (its own
explicit stack) before any recursive pass and distinguishes a true cycle (a node
on the current DFS path -> panic -> clean stub) from a finite shared DAG (a node
visited off-path -> safe to skip). A true cycle therefore degrades to a tagged
stub instead of crashing the process (zxing Aztec `Encoder.encode`,
`cyclic_if_tree.class`).

**Real fix (tracked elsewhere).** Loop/if-structuring surgery so it never
double-attaches a node and keeps post-loop code out of the loop body, eliminating
the cyclic/shared container at the source instead of degrading to a stub.

## 4. Residual real-jar partial families (all downstream of section 2/3)

1. **variable-fold nil ref key** - section 2 (fastjson2 `TypeUtils.doubleValue`).
2. **cyclic / shared container** - section 3 (druid `TDDLHint`, jackson
   `UTF8DataInputJsonParser`, zxing `Encoder`).
3. **multiple next** - a node keeps two `Next` edges after structuring, tied to
   `break`/`continue` at different loop levels (fastjson2 `seekLine`).
4. **post-decompile syntax** - residual `ConditionStatement`s inside a structured
   `IfStatement` body not recursed into, plus loop-exit mis-attribution.

All four reduce to incomplete CFG dataflow (section 2) and loop / value-merge
structuring (section 3) - the decompiler's hardest structural problem. Each is
currently caught by a safety net, so the safety contract holds.

## 5. Safety fixes already landed on this branch

These remove two hard failures (both strictly worse than a stub) exposed once
real classes reached the structuring stage. They do not by themselves clear
partials:

- **`mergeIf` convergence** (`statement_wrap.go`): the nil-branch guard set the
  fixpoint's `result=true` before returning without mutating the graph, so
  `MergeIf()` re-discovered the same unmergeable pair forever (infinite loop /
  hang). `result` is now set only after a real merge. Fixes the
  `panic_nilref_typeutils` hang.
- **`AssertStatementsAcyclic` check order** (`rewrite_var.go`): `visited` was
  checked before `ancestors`, so a true self-cycle was misclassified as a shared
  DAG and slipped through to a fatal stack overflow. `ancestors` is now checked
  first. Fixes the `cyclic_if_tree` crash.

## 6. Performance profile (current characterization)

- **GC-bound.** Core decompile ~215 ms / ~161 MB cumulative heap per 106-class
  jar; the post-decompile ANTLR re-parse (the syntax safety net) adds ~+60%
  runtime / ~+42% bytes.
- **Tail-bound.** On byte-buddy (2845 classes) one 43 KB class is 26% of a cold
  pass and the top 1% of classes are 61%; the high-value target is the
  pathological tail, not the average case.
- **Parallel scaling.** Near-linear to ~4 workers, peaks ~8 (3.6x), then
  GC-regresses; raising the ceiling needs allocation reduction.
- **Cross-parse ANTLR cache** is deliberately not shared: the pinned ANTLR Go
  runtime has no locking on DFA/`JStore`, and decompilation runs parallel, so a
  shared validation DFA would data-race (needs an ANTLR upgrade).

## 7. Reproduction commands

```
# Synthetic coverage + round-trip (no ~/.m2)
go test -run 'TestSyntaxCoverageMatrix|TestRecompileRoundtrip' -v ./common/javaclassparser/tests/

# Panic / hang / crash boundary (the section 2/3 classes, must degrade cleanly)
go test -run 'TestGAPanicFreeBoundary|TestDecompileCyclicStatementTreeNoCrash' -count=1 ./common/javaclassparser/tests/

# Determinism (portable, no Maven cache)
go test -run TestCorpusDeterminism -v ./common/javaclassparser/tests/

# Real-jar stub-reason attribution (needs ~/.m2)
STUB_REASONS=1 M2_MAX_JARS=120 M2_MAX_CLASSES=12000 go test -run TestM2StubReasons -v ./common/javaclassparser/tests/
```
