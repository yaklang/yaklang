# YakVM boundary hardening after #5098

Baseline: `855dd03e309d3463dee4d546c9d1c8455994248b` (merged #5098).
This change implements the report's immediate execution-safety and operand-stack
work, together with adjacent reproducible compiler, decoder, FFI and iteration
fixes. Existing tests are retained without changing their assertions or skips.

## Implementation and compatibility

| Report item | Implementation |
| --- | --- |
| C01 | Native and Yak workers use a common task runner. Each root execution has a bounded error collection shared by its children, including cross-VM sandbox calls. Workers never write the parent's coroutine panic slot. Argument validation precedes task registration. Native callback context is captured before asynchronous execution. |
| C02 | The send instruction uses a frame-aware `reflect.Select` over send and context cancellation, after direction/type validation. No blocked helper sender is created. |
| C03 | `Len` locks; scope inspection copies bindings under lock and resolves names after unlocking. Inner scope bindings still win. |
| C04, first stage | Checked wire reads, host-integer checks, aggregate object/byte/depth budgets, unique table IDs, root/reachability/parent-child checks, opcode/count/control-target/operand validation, failure-state cleanup, and bounded decoder fuzzing. |
| C05–C08 | Variadic tails use actually consumed positional arguments; missing fixed parameters retain the configured arity policy. Diagnostics use the actual variadic element type. Nil-first literals infer an interface element type. Index and slice normalization are separate; omitted bounds are represented explicitly. Arrays slice to copied slices and convert to arrays with checked lengths. |
| C10 | Nonempty interfaces require assignability; nil pointers cannot be dereferenced into structs. Reflection conversions prove convertibility. Named string/byte/function conversions preserve the destination type. Nonidentical container conversions retain their copy behavior. |
| C11, first stage | Unknown visitor panics become compiler errors; failed compilation publishes no codes. Reuse resets diagnostics and temporary control state. `defer` requires a call expression. `go` supports asynchronous expressions and implicitly calls a direct function definition once. |
| C12 | A FuzzTag partial-result-plus-error produces exactly one empty result. |
| C13 | Map display keys are formatted once for sorting; deleted keys are skipped. Set iterators close on exhaustion, break, return, panic and cancellation. Closing is optional on the public iterator interface for compatibility. |
| C14, first stage | `PeekN` returns nil outside `[0, Len)`. The generic linked stack and its full shadow restore behavior remain supported. |
| O01/O02 | Only frame operands use a typed contiguous stack; pop clears references. Binary and assignment paths use ordered local operands. Type instructions construct reflection types directly; make validates counts/length/capacity before allocation and avoids formatting newly allocated containers. |

### Public async behavior

`VirtualMachine.AsyncWait()` retains its signature and waiting behavior.
`VirtualMachine.AsyncWaitError()` waits and drains a VM-wide aggregate of failures.
`Frame.WaitAsync()` waits and reports only the submitting execution's failures.
Each collection retains at most 64 errors plus an omitted-error count. Finish
scheduling before waiting; do not wait on a group from one of its own workers.
Trace/Inline retains one execution group along with its retained frame.

As before, pre-cancelled Yak async calls do not launch a worker or panic at their
call site. They are now observable through the error collections. Native
`stopRecover` explicitly keeps its uncontained debugging behavior; Yak workers
retain their historical outer panic boundary even if the interpreter's inner
recovery is disabled. Private synchronous mode remains opt-in and now rejects
script async calls in the actual execution VM. Its host must still guarantee that
arbitrary native code does not invoke callbacks from another goroutine.

### Semantics fixed by regression tests

See [yak_semantics.md](yak_semantics.md). Intentional corrections include empty
and end-position slices, explicit zero versus omitted negative-step bounds,
well-typed array conversion, cancellable channel sends, and rejection of invalid
`defer` expressions. `go expr` evaluates a non-call expression in an implicit
zero-argument worker function. A direct function definition (including arrow
functions and parenthesized definitions) is called once; a returned closure or
function-valued variable/container element is not implicitly called. Existing
`go f(args)` keeps call-site argument evaluation. Slice results remain copies;
successful programs do not
gain Python-style out-of-bounds clipping or Go-style shared slice views.

### Limits of this batch

The decoder defaults to 64 MiB of input, 1,048,576 counted objects and 128 nested
code bodies. `UnmarshalWithLimits` lets trusted callers set different budgets.
Its structural validation is **not a complete control-flow stack/scope verifier**
and decoded code is not an untrusted execution sandbox. Instruction fuel,
call/operand depth, native-call cancellation and total runtime allocations still
need an execution-budget design. Oversized operand backing storage is discarded
on abnormal outer execution exit; there is no new default runtime stack limit.

Failed compilation reserves symbol IDs monotonically; it does not transactionally
roll back a shared symbol-table graph. Whole-graph compile transactions, the
remaining mutable Value metadata audit (C14), and numeric value-domain policy
(C09) remain separate work. This PR does not claim to normalize large unsigned
integers or to finish a general numeric type-join specification.

O03–O10's binding plans, local slots/capture cells, native call plans, hook tables,
full execution ownership/fuel, large-string indexing, value representation and
compact/fused instructions are not enabled here. The report places these behind
additional semantic characterization and profiles. Defer evaluation timing and
loop capture contracts retain the existing implementation and regression suite.

## Validation

Local toolchain: Go 1.22.12, Windows amd64, CGO enabled with the existing
w64devkit GCC. Baseline and candidate have independent worktrees.

- Complete `go test ./common/yak/antlr4yak/... -count=1 -timeout=15m` passes,
  including the engine, VM, compiler, DAP and LSP suites.
- Full VM/compiler `-race` passes. Incremental engine `TestCore*` race coverage
  passes, including native-to-Yak callbacks and cross-VM async ownership.
- LL-only `YAK_ANTLR_SLL_FIRST=0` runs the same new semantic/boundary corpus in a
  separate process and passes; default SLL-first coverage also passes.
- Decoder fuzzing ran for 30 seconds with two workers: 141,456 executions,
  no panic or failed input. Truncation, malformed table graphs, invalid counts,
  depth budgets, marshaller reuse and valid execution round trips are regressions.
- Async host panics and cold/saturated method-cache checks run in subprocesses.
  Set producer shutdown is checked by reacquiring the set's write lock after exit.
- `go` expression/function-definition cases cover original source, formatted source
  and serialized bytecode. Worker-only evaluation, lexical capture after the
  submitting function returns, short-circuit behavior, explicit calls and returned
  closures are checked without changing existing language tests. Yak SSA follows
  the same rules, with regressions for worker placement and no extra closure call;
  the existing incomplete-editor-input tests remain enabled.
- The broader Lua/NASL runs encounter existing `Test4` (left-value assignment)
  and `TestCode` (Yak function assertion) failures. Both reproduce on the clean
  baseline with the same errors. Their tests are unchanged. No claim is made that
  the complete cross-language or entire engine race suite is green.

The new `YakVM hardening` workflow adds VM/compiler race, incremental engine race,
LL-only and decoder fuzz jobs alongside complete Yak regression coverage. It
does not replace or relax existing workflows.

## Performance evidence

Intel Core i5-13490F, Go 1.22.12 windows/amd64, GOMAXPROCS=4. Identical new benchmark
files are installed in both worktrees. Separately compiled binaries run six
alternating base/head and head/base rounds, 400 ms per benchmark. Compilation is
excluded; one engine operation is a complete script with 1,000 iterations.
These are sample medians, not benchstat significance claims or universal speedups.

| Benchmark | Base ns/op | Candidate ns/op | Change | Base → candidate B/op | Base → candidate allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: |
| Arithmetic | 1,900,782 | 1,559,715 | -17.9% | 1,698,711 → 1,434,544 | 43,975 → 31,968 |
| Yak functions | 3,392,058 | 2,981,292 | -12.1% | 2,921,652 → 2,505,355 | 68,739 → 50,727 |
| Native calls | 2,809,825 | 2,429,618 | -13.5% | 2,384,757 → 2,048,600 | 56,722 → 41,715 |
| Typed containers/slices | 5,567,350 | 3,867,307 | -30.5% | 4,112,723 → 3,008,651 | 123,995 → 72,995 |
| Warm operand Push+Pop | 22.055 | 1.249 | -94.3% | 24 → 0 | 1 → 0 |

The last row measures a stack operation group, not a whole interpreter speedup.
Normal mode is used throughout; private synchronous mode is not used to inflate
the result. Raw samples are in `testdata/hardening-base-windows.txt` and
`testdata/hardening-head-windows.txt`. No Linux/ARM64 performance claim is made.
