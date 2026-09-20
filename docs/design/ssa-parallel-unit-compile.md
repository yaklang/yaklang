# Parallel compile-unit execution

The compiler partitions files into language-owned units and condenses their
dependency graph into strongly connected components (SCCs). Memory limits may
combine several SCCs in an execution batch, or split an oversized SCC across
batches. A memory batch is therefore not itself proof that its units are
independent.

`compileUnitBuildWaves` derives dependency-ready waves inside each batch. Each
wave runs independent SCC groups concurrently; units inside one SCC stay serial.
Dependencies in earlier batches have already completed. Splitting an SCC does
not permit its fragments to overlap across the batch lifecycle barrier.

## Ownership and lifecycle

1. One bounded AST parser pool serves all files in a batch. Small single-file
   packages can use multiple workers together. A bounded input-order queue
   preserves unit/declaration order even when later ASTs finish first. Admission
   slots are released by the consumer; a slow head cannot retain the whole batch
   or starve behind later files. Prefetching for the next batch can overlap the
   previous batch's semantic scan and persistence.
2. Declaration registration and deferred helpers remain serial: they use
   application-wide import, editor and declaration state.
3. Frontends opt into library body parallelism through `ConcurrentUnitBuilder`.
   Java supplies package dependencies and private execution builder views.
   Application-owned units execute exclusively. Each actual Program owner is
   drained once per batch, without recursively draining its upstream libraries;
   the scheduler controls when those library bodies run. Frontends that have not
   opted in retain the original single serial batch drain.
4. All active body builders join before semantic scanning, persistence and
   release. Cancellation also joins active builders before returning and does
   not start queued work.
5. The completed batch is semantically scanned, persisted and released before
   the next batch mutates shared IR. IR compilation does not yet overlap a
   semantic scan that enumerates the same live instruction cache.

Shared dependency values can acquire users and member relations even when the
consumer packages are independent. Their ID lists use per-value locks. Published
slice prefixes are immutable: removals and replacements allocate a new slice,
and readers resolve IDs after releasing the lock. Resident indexes use atomic
read/append/write operations to avoid lost entries.

Private builder scopes commit their resulting basic block back to the owning
declaration after execution. This preserves control flow between consecutive
field initializers while keeping editors and lexical state local. Tasks owned
by that declaration remain serial. Shared type-name updates use per-type locks;
default constructor lookup and registration use a short critical section, with
constructor body execution outside it to permit recursive construction.

Blueprint inheritance edges are registered as IR metadata without assignments
in either declaration scope. Reverse edges must not start a dependency's lazy
body. For frontends opted into concurrent units, missing dependency members
are synthesized in the consuming library;
the dependency retains the member relation but its block and scope remain
unchanged. Type method maps publish immutable snapshots after lazy initialization.
Type formatting uses a per-call recursion path instead of temporarily renaming
shared types; completed root function strings retain an atomic cache. A missing
memory-mode relation never falls back to a same-named persisted program.

## Validation

Use `scripts/ssa-test.sh` for all tests. Focused coverage includes dependency
diamonds and SCCs, real library overlap, cancellation, shared static fields,
snapshot stability, a shared bounded AST window, and database reload with
cross-package dataflow and source ranges. Run these tests with `-race` as well as
the Java, PHP, SSA and scan lifecycle regression suites.

For project measurements, use `yak code-scan -t TARGET -l LANGUAGE --debug DIR`.
Start with small projects in `--memory` mode, then validate persistence before a
large DB-backed run. Retain the source commit, binary hash, arguments, risk
results, phase timing, process CPU/RSS/swap samples and profiles together.

Each build-wave log records `groups`, requested `concurrency`, `peak_active` and
elapsed `cost`. `peak_active` measures overlapping group workers, not occupied
CPU cores. Compare it with process CPU samples. A large SCC or serial declaration
phase can still limit utilization; increasing the worker limit cannot remove
these dependencies.
