# SSA persistence dirty tracking — problem statement and plan (uncommitted draft)

Status: analysis only, no code change attached. Written while reviewing
`common/yak/ssa/instruction_dirty.go` (`MarkDirty` / `markPersistenceDirty`).

## 1. The review point

`markPersistenceDirty` is bound once to a concrete instruction and then pushes
that instruction into the resident cache:

```go
func (i *anInstruction) markPersistenceDirty() {
    if i == nil || i.cache == nil {
        return
    }
    i.cache.markInstructionDirty(i)
}
```

The complaint is correct in the narrow sense:

* It only runs where a builder happens to call it (`defer b.markPersistenceDirty()`
  around emit/assign/finish helpers). Any mutation that reaches an instruction
  through a plain field write or through a pointer alias — `inst.Users = append(...)`,
  `block.Preds = ...`, `func.FormalArgs = ...`, a slice element edit behind an
  `Int64Slice` — never touches `MarkDirty`.
* Because IR is object-graph-shaped, "who is allowed to mutate this instruction"
  is not enforceable at compile time. A conservative marker that fires on every
  pointer access would serialize the whole compile.
* Result: the dirty set is a *lower bound*. It is safe (nothing persisted is
  stale in the sense of being attempted-but-skipped) but it is also *not
  sufficient*: correctness currently depends on the cache also persisting
  everything at Close/Flush boundaries, which is why the delta test
  (`instruction_persistence_delta_test.go`) can edit a slice in place with no
  `Set`/`MarkDirty` and still observe the new value after a full flush.

## 2. What already exists

`common/yak/ssa/database_cache_instruction.go` already carries the machinery for
a value-based check:

| Piece | Role |
| --- | --- |
| `irCodePersistenceFingerprint(code *ssadb.IrCode)` | hashes the serialized snapshot into a comparable fingerprint |
| `residentFingerprints` | last fingerprint stored for a resident instruction |
| `unchangedSkipped` | counter of writes skipped because the fingerprint matched |
| `instruction_persistence_delta_test.go` | asserts a no-op flush does not hit the DB (SQL trigger rejects unchanged updates) |

So the *decision* mechanism exists; what is missing is a single place that
recomputes the fingerprint after serialization and decides insert/update/skip
there, instead of believing the builder's `MarkDirty` calls.

## 3. Why "mark on mutation" cannot be made correct cheaply

1. **Aliasing.** Instructions are reachable from many places (values, users,
   blocks, function tables, the resident cache). Marking requires knowing all
   writers; there is no write barrier on Go structs, and field writes bypass
   methods.
2. **Granularity.** A single `Users` append changes one field of one IR row; a
   `Finish()` pass can rewrite hundreds of rows via helpers that never see the
   instruction. Per-field dirty bits would need a rewrite of every write site.
3. **Concurrency.** The parallel compile path mutates IR from several unit
   builders. A chained dirty marker would need the same locking the cache
   already has, adding contention to the hottest path (emit/assign).
4. **Cost of being wrong.** Missing a marker is silent data loss risk; the
   current fallback (persist on boundaries) hides it, but any future "skip on
   clean" optimization would turn the lower bound into a correctness bug.

## 4. Proposed direction: decide after serialization

Serialization already happens for every persisted instruction. Compute the
fingerprint there and use it as the authority:

```
flush prepare (per instruction):
    code := marshalInstructionSnapshot(inst)      // must be deterministic
    fp   := irCodePersistenceFingerprint(code)
    if residentFingerprints[key] == fp && !force {
        unchangedSkipped++
        skip DB write
    } else {
        write (upsert via gorm SaveInBatches)
        residentFingerprints[key] = fp
    }
```

Properties:

* **Correct by construction.** The decision reads the value that would be
  written, so a mutation through any alias is visible; `MarkDirty` becomes a
  scheduling hint (whether to consider the row at all) rather than the source of
  truth.
* **Cost.** One extra hash per considered instruction per flush. Serialization
  is already paid on the persistence path, so the added work is the hash only.
  It is *not* free for the fast path: once every resident instruction is
  fingerprinted, a no-op flush still walks and hashes the resident set.
* **Concurrency.** The fingerprint map needs the same protection the resident
  map has today (the cache already guards it); no new invariant.
* **Determinism requirement.** The Marshal* output must be stable across runs
  (map iteration order, timestamps, ids). `Instruction.Marshal` already exists;
  it must be audited for ordering (ObjectMember maps are the suspect) before the
  fingerprint can gate a skip.

## 5. Options and trade-offs

| Option | Change | Correctness | Cost |
| --- | --- | --- | --- |
| A. Status quo | keep `MarkDirty` as-is | safe but no skipping guarantee; tests must always force-write | lowest |
| B. Fingerprint after serialization, skip when equal | move authority to the fingerprint; `MarkDirty` = hint | correct if marshal is deterministic | +1 hash per considered row; must audit marshal determinism |
| C. Fingerprint only on eviction/Close, force-write on unit boundaries | narrower variant of B | correct for the flush that matters; boundary writes stay unconditional | smaller blast radius, less saving |
| D. Per-field dirty bits | rewrite write sites | correct only if every writer is captured | highest, rejected |

## 6. Suggested plan (do not implement half of this)

1. **Audit Marshal determinism** for `IrCode` (map ordering, slice ordering,
   time/`UpdatedAt`, `ID`/`DeletedAt` normalization). Add a test that marshals
   the same instruction twice, including an `ObjectMemberPairs` map, and
   compares the fingerprint.
2. **Add a fingerprint cache with an explicit invalidation path** (keyed by
   instruction id, cleared on eviction and on `CloseWithoutSave`) and a metric
   for `unchangedSkipped`.
3. **Wire it at the single write funnel.** Do not bolt it onto `MarkDirty` call
   sites; the funnel is `instructionStore`'s save path, so every caller
   (boundary flush, eviction, Close) inherits the behaviour.
4. **Keep the forced-write escape hatch** for the first flush after a full
   rebuild and for tests, so the delta test can keep rejecting unchanged
   updates.
5. **Only then** consider downgrading `markPersistenceDirty` to a scheduling
   hint (or deleting the per-call-site noise), with the flush tests proving both
   directions: changed → written, unchanged → skipped.

Acceptance evidence for the change would be: the existing SQL-trigger test
(unchanged rows must not be written) plus a new test that mutates a field
through a pointer alias with no `MarkDirty` and still sees the new value
persisted.
