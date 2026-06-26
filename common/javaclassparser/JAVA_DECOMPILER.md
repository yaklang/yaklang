# Yak Java Decompiler

> Language: **English** | [简体中文](./JAVA_DECOMPILER.zh-CN.md)
>
> Status: **GA (General Availability)**. Snapshot: 2026-06-26, darwin/arm64, Go 1.22.12.

The Yak Java decompiler reconstructs readable Java source from `.class` and
`.jar` bytecode. It is a from-scratch bytecode-to-source engine (no dependency on
an external decompiler such as CFR / Procyon / Fernflower), designed to plug into
the Yaklang SSA pipeline while remaining usable as a standalone source recovery
tool.

- Go entry point: `javaclassparser.Decompile(classBytes []byte) (string, error)`
- Yaklang entry point: `java.Decompile(sourcePath, destDir)`

---

## 1. Why it is GA

A decompiler is GA-ready when, on a broad real-world corpus, it (a) never crashes
the host process, (b) never emits invalid Java as a success, and (c) fully
reconstructs the overwhelming majority of classes. The live-measured evidence:

| Axis | Measurement (2026-06-26) | Verdict |
|------|--------------------------|---------|
| Industry corpus sweep | 60,000 classes across 546 of 1,107 local `.m2` jars | **ok=60000 partial=0 syntax=0 err=0** | GA |
| Mainstream libraries | guava(2007) commons-lang3(385) jackson-databind(756) fastjson(179) spring-core(1105) | all `ok`, 0 partial / 0 fail | GA |
| Portable syntax corpus | 31 groups (26 classic + 5 modern Java), recompiled + round-tripped | 0 stub, 0 syntax error | GA |
| Real-bytecode regressions | 77 focused `.class` fixtures pinned from the corpus | all parse cleanly | GA |
| `javac` recompile oracle | decompile -> `javac --release 8` for every eligible group | all recompile | GA |
| Determinism | repeated decompile of the same class | byte-identical output | GA |
| Safety contract | no panic escapes `Decompile`; no stack overflow; failures degrade to tagged stubs | green on the panic-free boundary suite | GA |

How those numbers were produced (reproducible, no corpus magic):

```bash
# Full-corpus success-rate sweep (the 60,000-class row above)
M2_OUT=bench.txt M2_INDUSTRY=1 M2_MAX_CLASSES=60000 M2_MAX_PER_JAR=400 \
  go test -run TestM2RegressionHarness -count=1 -timeout 30m \
  -v ./common/javaclassparser/tests/

# Per-library timing + success probe (the mainstream-libraries row)
BENCH_JAR=<path-to-guava.jar> \
  go test -run TestDecompileJarTiming -count=1 ./common/javaclassparser/tests/
```

The industry sweep samples every jar in `~/.m2` (capped at `M2_MAX_PER_JAR`
classes per jar so a few giant jars cannot dominate), rather than only the
alphabetically-first ones, so it covers Spring, Tomcat, Netty, Jackson, Guava and
the rest. A Maven cache is a moving target — new dependencies can introduce new
bytecode shapes — but at this snapshot the sampled population is clean.

---

## 2. Using it

### Go

```go
import "github.com/yaklang/yaklang/common/javaclassparser"

// source is readable Java; err is non-nil only on malformed bytecode
source, err := javaclassparser.Decompile(classBytes)
```

### Yaklang

```javascript
// sourcePath: a .class or .jar (also .war / nested archives); destDir: output folder
java.Decompile(sourcePath, destDir)
```

After the call, `destDir` contains one `.java` per decompiled class, preserving
the package directory layout. Nested archives (jar-in-jar, jar-in-war) are
unfolded transparently.

### Partial output and the stub contract

When a single method body cannot be faithfully reconstructed (an exotic CFG
shape the structural analysis has not generalized), the decompiler does **not**
drop it silently and does **not** invent likely-wrong source. Instead it emits an
explicit, tagged stub:

```java
static { /* yak-decompiler: undecompilable <clinit>: <reason> */ }
```

`javaclassparser.DecompileStubMarker` is the `"yak-decompiler:"` sentinel; test
whether the output contains it to distinguish a full decompile from a partial one.
`EnableDecompileSyntaxValidation` (default `true`) gates the post-decompile syntax
safety net that re-renders or degrades malformed members.

---

## 3. How it works

The pipeline has four stages, each hardened by its own test layer:

1. **Classfile parsing** — `ClassParser` turns raw bytes into the constant pool,
   fields, methods and full code attribute (instructions, exception table,
   stack-map frames).
2. **Operand-stack simulation** — each method's bytecode is replayed to rebuild a
   typed expression tree and recover local-variable slots. This is where most
   "hard" real-world bytecode lives: slot reuse across array values, loop
   counters and catch variables; DUP/swap families; switch-case operand stacks;
   ternary value stores and tail-duplicated returns.
3. **Structural analysis** — the instruction graph is lifted into a statement tree
   (loops via a standard natural-loop algorithm, if/else merge, try/catch/finally
   region reconstruction, synchronized blocks). Shared-DAG containers are told
   apart from true cycles so the statement tree stays acyclic.
4. **Emission** — the statement tree renders to Java source, then a Java syntax
   frontend re-parses it. Anything that fails to parse is re-rendered or
   downgraded to a tagged stub, so a successful return is always valid Java.

---

## 4. Coverage by construct

| Construct | Status |
|-----------|--------|
| Control flow: if/else, loops, labelled break/continue | GA |
| `switch` (statement & expression), string switch | GA |
| try/catch/finally, try-with-resources, multi-catch | GA |
| Lambdas & method references (`invokedynamic`) | GA |
| Inner / nested / anonymous / local classes | GA |
| Generics, enums, annotations | GA |
| Interface & annotation `<clinit>` field hoisting | GA |
| `synchronized` blocks, assertions | GA |
| Modern Java (records, sealed, pattern matching, switch expr, text blocks) | syntax corpus covered; fidelity tracked continuously |

---

## 5. Verifying and reproducing

```bash
# Syntax coverage + javac round trip
go test -run 'TestSyntaxCoverageMatrix|TestRecompileRoundtrip' -v \
  ./common/javaclassparser/tests/

# Real-bytecode syntax regressions (the 77 fixtures)
go test -run TestDecompileSyntaxRegression -v ./common/javaclassparser/tests/

# Panic / hang / crash boundary
go test -run 'TestGAPanicFreeBoundary|TestDecompileCyclicStatementTreeNoCrash' \
  -count=1 ./common/javaclassparser/tests/

# Determinism
go test -run TestCorpusDeterminism -v ./common/javaclassparser/tests/

# Whole package
go test ./common/javaclassparser/...
```

These run in CI (the `javaclassparser/tests` budget is 5 minutes); the large
`.m2` sweeps are opt-in environment-gated tests that do not run in normal CI.

---

## 6. Known limits and the path to "perfect"

GA does not mean every method on Earth is perfect. The honest caveats:

- **Partial stubs can still appear** on rare, deeply irregular CFGs. The contract
  is that they are *explicit and safe*, never silent or invalid. Each one is a
  concrete, reproducible target.
- **Source-level fidelity** (variable names, formatting, comments) is not
  byte-for-byte identical to the original source — names come from the debug
  attribute when present and are synthesized (`varN`) when absent. That is
  inherent to decompilation.
- The corpus is Java 8-21 bytecode from a real Maven cache; entirely novel
  bytecode shapes from future toolchains are exercised as they appear.

The recommended way to drive residual partials to zero is the iterative
stop-on-first workflow: scan the corpus, capture the first failing class, fix the
root cause, add a regression `.class`, re-run the portable suite, and resume. It
is deliberately a one-class-at-a-time loop so fixes never mask each other.
