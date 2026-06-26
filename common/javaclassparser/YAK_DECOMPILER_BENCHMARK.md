# YAK Java Decompiler Benchmark

> Language: **English** | [简体中文](./YAK_DECOMPILER_BENCHMARK.zh-CN.md)
>
> Snapshot date: 2026-06-26. Host: darwin/arm64, Go 1.22.12.

This document records the release benchmark for the Yak Java decompiler entry
points:

- `javaclassparser.Decompile([]byte) (string, error)`
- Yaklang wrapper `java.Decompile`

The current branch has moved from a "known structural defects" report to a GA
readiness report: the portable regression suite is green, the safety contract is
green, and the actively repaired `.m2` scan windows are clean.

## 1. Current Status

| Axis | Result | Status |
|------|--------|--------|
| Synthetic syntax corpus | 31/31 groups, 0 stub, 0 syntax error, 0 panic | GA |
| Synthetic round trip (`decompile -> javac`) | 26/26 eligible groups recompile | GA |
| Embedded real-bytecode regressions | 46 focused `.class` regressions parse successfully | GA |
| Hard-case families | switch, ternary, try/catch, lambda, inner class, interface `<clinit>` all covered | GA |
| Determinism | repeated decompile output is byte-identical | GA |
| Safety contract | no panic escapes `Decompile`; failures degrade to explicit `yak-decompiler:` stubs | GA |
| Active `.m2` repair windows | repaired windows listed below run with `partial=0 err=0 stubs=0` | GA for validated windows |

Full local Maven caches are naturally moving targets: new dependencies can
introduce new bytecode shapes. The rule for this branch is strict: any `WARNING`
for a dropped field/method or any generated stub in an actively scanned window is
treated as a defect, fixed with a regression class, and then rescanned.

## 2. Latest Validation

Portable tests:

```bash
go test -run TestDecompileSyntaxRegression -v ./common/javaclassparser/tests/
go test ./common/javaclassparser/...
```

Both commands passed on this snapshot.

Latest `.m2` shard results:

| Range | Result |
|-------|--------|
| `943-1200` | `classes=31189 ok=31189 partial=0 err=0 stubs=0` |
| `1461-1600` | `classes=61150 ok=61150 partial=0 err=0 stubs=0` |
| `1756-1926` | progressed through the Elasticsearch fixes to Liquibase with `60103` classes ok before the next defect was found and fixed |
| `1926-2000` | `classes=23262 ok=23262 partial=0 err=0 stubs=0` |

The last two rows are intentionally split because the Liquibase slot-0 issue was
found at jar index `1926`; after fixing it, the `1926-2000` suffix was rerun and
completed cleanly.

## 3. Fixed Edge-Case Families

The following real-world bytecode families are now locked by regression tests:

- Multi-use local slots across array values, loop counters, and catch variables.
  Example: XMLBeans `QNameHelper.hexsafe`.
- Generic and primitive parameter slot collisions, including empty
  `param_placeholder` argument rendering. Example:
  Elasticsearch `CopyOnWriteHashMap$InnerNode.put`.
- Interface `static final` fields whose `<clinit>` initializer cannot be
  source-hoisted. Example: Elasticsearch `Client.CLIENT_TYPE_SETTING_S`.
- Instance methods whose bytecode writes to local slot 0 after the initial
  receiver load. Example: Liquibase `co.at(ax)`.
- Multi-dimensional primitive arrays and category-2 stack handling. Example:
  SparseBitSet `long[][][]`.
- Try/finally around loop containers that previously produced a statement graph
  cycle. Example: Commons Collections `ExtendedProperties.load`.
- Interface and annotation `<clinit>` hoisting for final static fields. Examples:
  ECJ `TypeConstants` and `JavadocTagConstants`.
- Large boolean ternary trees, boolean constructor arguments, and nil-safe return
  type resetting. Examples: JTidy, OpenRewrite, Saxon, ECJ.

These are no longer tracked as known residual defects. If they regress, the
regression suite should fail before `.m2` scanning is needed.

## 4. Safety Contract

The decompiler must prefer explicit degradation over invalid Java:

- no Go panic may escape `Decompile`;
- no recursive statement walker may stack-overflow the process;
- no invalid source should be returned as a successful decompile;
- a method body that cannot be represented must be replaced with a tagged
  `yak-decompiler:` stub;
- field or method drops are not acceptable in active scan work and must be fixed
  when discovered.

The current implementation validates generated Java through the Java syntax
frontend and then re-renders or degrades malformed members. The `.m2` harness
also records stub reasons and exact jar/class locations so new failures can be
turned into regression fixtures.

## 5. Scan Workflow

Use bounded shard scans for local Maven cache validation:

```bash
GOMAXPROCS=2 \
STUB_REASONS=1 \
STOP_ON_FIRST=1 \
M2_INDUSTRY=1 \
M2_START_JAR_INDEX=1926 \
M2_START_JAR_END=2000 \
M2_CONCURRENT_JARS=1 \
M2_MAX_CLASSES=1000000 \
M2_MAX_PER_JAR=1000000 \
PROBLEM_DIR=/tmp/jdec-shard-1926-2000 \
PROGRESS_EVERY=100 \
M2_PROGRESS_FILE=/tmp/jdec-progress/1926-2000.env \
go test -timeout 30m -run TestM2StubReasons -v ./common/javaclassparser/tests/
```

Recommended process:

1. Run shards with `STOP_ON_FIRST=1`.
2. Treat any warning, partial, panic, syntax failure, or stub as a defect.
3. Reproduce the single class with `TestDiagDecompileClass`.
4. Cross-check with bytecode (`javap -c -v`) and an external decompiler when
   available.
5. Fix the decompiler, add a regression `.class`, run the portable tests, and
   resume the shard from the failing jar.

## 6. Portable Reproduction Commands

```bash
# Synthetic coverage and javac round trip
go test -run 'TestSyntaxCoverageMatrix|TestRecompileRoundtrip' -v ./common/javaclassparser/tests/

# Real-bytecode syntax regressions
go test -run TestDecompileSyntaxRegression -v ./common/javaclassparser/tests/

# Panic/hang/crash boundary
go test -run 'TestGAPanicFreeBoundary|TestDecompileCyclicStatementTreeNoCrash' -count=1 ./common/javaclassparser/tests/

# Determinism
go test -run TestCorpusDeterminism -v ./common/javaclassparser/tests/

# Full javaclassparser package
go test ./common/javaclassparser/...
```
