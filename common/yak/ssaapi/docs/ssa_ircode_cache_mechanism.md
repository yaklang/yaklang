# SSA Compile IR Cache Mechanism

## Purpose

When SSA compilation runs in database mode, IR objects do not need to stay in
memory until the entire compile finishes.

The compile IR cache exists to reduce peak memory usage while keeping these
properties stable:

- recently used IR can still be read from memory quickly
- spilled IR can be reloaded lazily from the SSA database
- IR is only removed from memory after the corresponding database write is
  acknowledged
- delete and writeback semantics stay consistent during both runtime eviction
  and final close

## Ownership

The mechanism is split across four layers:

- `ssaconfig.SSACompileConfig`
  - owns compile-time cache settings
- `ssa.ProgramCache`
  - decides which backend is used and which IR should remain hot
- `dbcache.Cache`
  - manages resident entries, eviction, async persistence, and save
    acknowledgement
- `ssadb`
  - stores `IrCode`, `IrSource`, `IrIndex`, `IrOffset`, and reload data

`ssaapi.Config` should not keep a second copy of compile IR cache state. Runtime
code reads the effective values from `ssaconfig.Config`.

## Configuration

The user-visible compile IR cache knobs are:

- `CompileIrCacheTTL`
- `CompileIrCacheMax`

Their meaning is:

- `ttl > 0`
  - resident IR can expire by time
- `max > 0`
  - resident IR can expire by capacity
- `ttl = 0 && max = 0`
  - runtime eviction is disabled and IR stays resident until close

### Adaptive defaults

The cache policy can be adjusted automatically from the size of the compile
input. The runtime input used for this decision is
`SSACompileConfig.CompileProjectBytes`.

This value is:

- calculated from the total source bytes of files that enter the compile stage
- used only to tune cache defaults for small and large projects
- not serialized into JSON
- not part of long-term project metadata

## Runtime Model

In database mode the instruction cache has three observable states:

1. resident
   - the IR object is still available in memory
2. pending persist
   - eviction has started, but the object is still retained until async
     persistence finishes
3. persisted and removed
   - the database write succeeded, and the resident copy can be dropped

This prevents a bad window where memory has already been cleared but the
database write has not completed yet.

### Save acknowledgement

Eviction does not remove resident IR immediately.

The flow is:

1. mark the resident object as pending
2. marshal it into database form
3. batch-save it
4. remove the resident copy only after save succeeds

If a pending object is read again before save finishes, the runtime can keep
serving the in-memory copy instead of forcing an early reload from the database.

## Function-Finish Gating

Compile-time IR is not treated uniformly during the whole build.

Instructions that belong to an unfinished function should not be evicted just
because a TTL or capacity threshold has been reached. The function still owns
live build state such as parameters, free values, parameter members, blocks, and
other function-scoped IR relationships.

The current policy is:

- unfinished-function IR stays protected from runtime eviction
- when `Function.Finish()` runs, function-scoped IR is re-armed for eviction
  tracking
- hot instructions can still remain resident after finish

This keeps the build path stable while allowing finished parts of the program to
participate in memory reduction.

## Hot Instructions

Some instructions are intentionally kept hotter than ordinary IR because they
are frequently revisited or still carry non-reloadable runtime state.

The hot-instruction set is defined in `common/yak/ssa/database_cache_utils.go`.
At the time of writing it includes:

- `Function`
- `BasicBlock`
- `ConstInst`
- `Undefined`
- `Make`

This list is a maintenance point. If reload behavior changes, the hot set should
be reviewed together with the reload guarantees.

## Source Persistence

`IrCode` rows can reference `IrSource`, so source persistence needs the same
acknowledgement discipline as instruction persistence.

The source-hash flow is:

1. reserve a source hash as pending
2. enqueue the `IrSource` save
3. move the hash to the persisted set only after the save succeeds
4. if the save fails, clear the pending reservation so later retries are still
   allowed

This avoids treating a source file as already persisted when the database write
did not actually succeed.

## Close Semantics

Close should reuse the same persistence path as runtime eviction.

The close flow is:

- mark resident entries for persistence with delete-style eviction reason
- let the normal marshal and batch-save pipeline run
- wait for save acknowledgement
- clear resident entries only after acknowledgement

This avoids maintaining a separate save path with different semantics at the end
of compile.

## Observability

Debug logs are available to inspect cache behavior. Typical log families are:

- reload
- save
- save skip
- writeback
- saver summary
- cache summary

These logs are operational aids. They should not be required to understand the
public API.

## Maintenance Notes

When modifying this mechanism, check these invariants:

- compile-time cache settings come from `ssaconfig`
- unfinished-function IR is not evicted prematurely
- resident objects are only removed after save acknowledgement
- failed `IrSource` saves do not poison later retries
- hot-instruction assumptions still match reload guarantees

## Tests

The main regression coverage lives in:

- `common/utils/dbcache/cache_test.go`
- `common/yak/ssa/database_cache_test.go`
- `common/yak/ssa/database_search_test.go`

When the mechanism changes, add or update tests around:

- finish-triggered eviction
- lazy reload
- dirty writeback
- source-hash acknowledgement and retry
- hot-instruction residency
