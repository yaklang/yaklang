# ARTEX query/readback and lookup-index follow-up

This is the small, format-compatible follow-up to the 2026-10-16 ARTEX
`bb79c7f41f` baseline. It does not implement the separate range/type storage
migration discussed in `ssa-storage-sharing-followup.md`.

## Changes and semantic boundaries

1. A name pool that has no matching IDs returns an empty result channel without
   issuing `WHERE 1 = 0`. Exact/glob/regexp matching and include/exclude handling
   are otherwise unchanged.
2. A successful native instruction read returning no row does not retry the same
   lookup with GORM. Actual native schema/driver errors still use the legacy
   fallback; cancellation/deadline errors do not restart an unbounded query.
   There is no negative cache, so a later INSERT is immediately visible.
3. Variable-offset reads use parameterized native SQL with the same program,
   variable/value and soft-delete predicates as the old reader. An empty program
   retains the existing cross-program lookup behavior. Every call owns its row
   objects; aliases, multiple use ranges, unnamed bindings and metadata remain
   distinct. Legacy schemas that cannot satisfy the native column list retain
   the GORM reader. This deliberately does not introduce a cross-rule cache or
   preload unfinished compile-time offsets.
4. New databases use the existing canonical UNIQUE indexes on
   `(program_name, code_id)` and `(program_name, type_id)` without also creating
   non-unique indexes for precisely those same column lists. Existing legacy
   indexes are left untouched. If the canonical unique index is unavailable,
   the migration still creates the legacy non-unique lookup index and the
   existing batch-write fallback remains usable. Other indexes, including
   opcode-only, field, owner, range and soft-delete indexes, are not removed.

## Storage version boundary

No persisted table, column, scalar ID, row identity, binding or payload changes.
The current `ir_indices_v1`, `ir_name_pool_v1`, `ir_offsets`, `ir_codes` and
`ir_types` formats retain their existing versions. Removing duplicate *creation*
work for a new database is not an incompatible schema version; old readers use
these existing canonical lookup keys too. No DROP or destructive old-database
migration is performed.

Any later range/relationship split or type payload/ref split must use explicitly
versioned tables/files (for example `_v2` or `-v2`) and separate reader/writer
selection. Do not pack multiple values into the existing scalar `value_id` or
merge mutable Type identities merely because their display strings match.

## Validation

Regression tests cover empty-query counts, real native failures, a miss followed
by INSERT, all offset rows and metadata against the GORM reference, program and
alias isolation, mutable result independence, legacy schemas/indexes and stable
row identity across batch upsert. Run through `scripts/ssa-test.sh`, including
`-race` on `common/yak/ssa/ssadb`; build yak before committing.

The historical successful ARTEX baseline has 389,089 IR codes and a saved program;
its debug database is 497.32 MiB. The broader optimization inventory and new-run
acceptance evidence are recorded separately under
`/mnt/data/scan-outputs/artex-optimization-20261016/`. Baseline SQL timing totals
are concurrent aggregates, not wall time, and sampling snapshots are not a
process high-water RSS measurement. Do not claim speed/space gains before the
new `-t` / same-database `-p` run has completed.
