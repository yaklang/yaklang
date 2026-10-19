# GORM batch save audit — follow-up list (uncommitted draft)

Status: notes only. Describes `SaveInBatches` in the local gorm fork
(`~/Developer/work/gorm`, pushed to `yaklang/gorm` branch
`enhance/perf/create-in-batches`) and the SSA write path that uses it. Nothing
here is committed; `go.mod` carries a local-only `replace` for the fork.

## 1. Why this exists

The SSA review flagged `SaveIrCodeBatch` in `common/yak/ssa/ssadb/code.go`:
the "batch update" was implemented as DELETE-then-INSERT (`bulkUpsertIrCode`),
which

* destroys and rebuilds the row's secondary-index entries on every flush,
* assigns new primary keys, so a row's identity is not stable across flushes,
* removes the previous snapshot before the replacement is known to be valid
  (a rejected statement leaves no row at all), and
* needs a per-batch `IN (?)` delete list that grows with batch size.

The fix is a real batch upsert. gorm v1 has `CreateInBatches` but nothing that
updates, so the fork gained `SaveInBatches`.

## 2. API: what gorm v2 does and what we chose

gorm v2 (v1.31.2, 2026-06-25) has no dedicated upsert verb. An upsert is
`Create` plus a conflict clause:

```go
db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&users)
```

and `Save(slice)` is exactly that shortcut (`finisher_api.go`: for a slice it
adds `OnConflict{UpdateAll: true}` and runs the create callbacks with
`gorm:update_track_time`). `UpdateAll` rewrites every column except the primary
key, columns whose default comes from an SQL function, and `created_at`, and it
stamps `updated_at`. v1 has no clause API, so the fork spells the same thing as

```go
db.SaveInBatches(rows, batchSize, "program_name", "code_id")
// SQLite/Postgres: INSERT ... ON CONFLICT("program_name","code_id") DO UPDATE SET col=excluded.col, ...
// MySQL:           INSERT ... ON DUPLICATE KEY UPDATE col=VALUES(col), ...
```

The name follows v2's `Save`. `UpdateInBatches`, the first version of this API,
is gone: it implied a plain `UPDATE`, and it was never published beyond this
branch.

Rules (all covered by `batch_save_test.go`):

* The conflict target must be backed by a UNIQUE index; the caller guarantees
  that. With no columns the target is the primary key, like `Save`.
* The SET list leaves out the conflict columns, the primary key and
  `created_at`, and stamps `updated_at`. Leaving out the conflict columns is
  also a measured win: on a table with seven indexes a 20k-row flush went from
  about 1.14s to 0.95s, because assigning an indexed column rewrites its index
  entries even when the value does not change.
* When the target is not the primary key, the primary key is never inserted.
  A stale id on a struct therefore cannot collide with another row, and an
  existing row keeps its id. This is what made the caller-side reset of
  `ID/CreatedAt/UpdatedAt/DeletedAt` unnecessary.
* Primary keys are not written back into the structs: after an upsert the last
  insert id does not say which rows were inserted.
* `nil` elements are skipped (`CreateInBatches` still rejects them).
* The conflict clause is built per statement from the columns it inserts and is
  passed as an argument, not stored on the DB handle. `OnConflictDoUpdate` and
  `SetOnConflict` store it with `InstantSet`, which leaks the clause into the
  receiver; that is left as is, they are older APIs.
* A caller-supplied `gorm:insert_option` is refused rather than overridden.
* The struct is the snapshot: a `deleted_at` it carries is written, a nil one
  restores a soft-deleted row (same as v2).

Not done, deliberately: the v2 `Clauses(OnConflict{...})` surface, `DoNothing`
and per-column `DoUpdates`. Nothing in this tree needs them; `OnConflictDoNothing`
already covers the CVE loader.

Limits to remember: one chunk must not hold the same conflict key twice
(PostgreSQL rejects it, SQLite lets the later row win); MySQL has no conflict
target syntax, so any unique key triggers the update there.

Fixed in the same series: `sqlite3.HasIndex` matched `INDEX <name> ON` inside
the stored `CREATE` text, which never matches a quoted name. It now compares
`sqlite_master.name`, so SSA asks the dialect (`hasNamedIndex`) instead of
keeping three hand-written per-database probes.

`sqlite3_extended` needs no special case in gorm: `common/consts` registers the
alias with the sqlite3 dialect object, whose `GetName()` is `"sqlite3"`. The
`sqlite3_extended` branches added with the first version were dead and have been
removed.

## 3. SSA sites

| Site | Before | After |
| --- | --- | --- |
| `ssadb.SaveIrCodeBatch` | DELETE + `CreateInBatches`, then caller-reset + `OnConflictUpdateAll` | `SaveInBatches(items, irCodeBatchChunk, "program_name", "code_id")` when `ux_ir_codes_program_code` exists; no copy of the slice, no field reset |
| `ssadb.SaveIrTypeBatch` | DELETE + `CreateInBatches` | `SaveInBatches(items, irTypeBatchChunk, "program_name", "type_id")` once `ensureUniqueIrTypesProgramTypeIndex` created `ux_ir_types_program_type` (older duplicates removed, oldest id kept) |
| `ssadb.UpsertIrCode` | single-row wrapper | unchanged (delegates) |
| `ssadb.bulkUpsertIrCode`, `bulkUpsertIrType` | only path | fallback for databases without the unique index; they read fields of every element, so they get `withoutNil(items)` |
| `ssadb.SaveIrIndexBatch`, `SaveIrOffsetBatch` | each hand-copied the slice to drop nils | share `withoutNil`; still plain inserts |
| `ssa.database_cache_instruction.go` upsert path | comment said "delete+bulk insert" | comment corrected; still calls `SaveIrCodeBatch` |

Why the reset existed: commit `225b83b79b` introduced it for the delete-then-
insert path ("clear primary key / soft-delete state so `CreateInBatches`
allocates a new row instead of colliding on id"). `fdfd0d6614` moved the upsert
to `ON CONFLICT` and kept it, because that statement took the id from the struct
and rewrote the conflict columns. Both reasons are gone with `SaveInBatches`.
Every struct that reaches these savers comes zeroed from `acquireIrCode` /
`EmptyIrType`, so the clearing never changed a value in production; it only cost
four writes per row and hid the real hazard (a struct carrying another row's id).

Measured on the real `IrCode` schema (20,000 rows, file SQLite, seven indexes,
alternating runs on a loaded machine): old statement shape 1.08–1.21s, new
0.95–1.17s per flush. The allocation count dropped about 2%. The gain is
smaller here than in the isolated gorm benchmark because the wide row's
serialization dominates.

## 4. SSA sites deliberately left alone

| Site | Why |
| --- | --- |
| `ssadb.SaveIrIndexBatch`, `SaveIrOffset`, `SaveIrSource` | insert-only; the callers filter already-persisted rows before writing, so an upsert would only add conflict-check cost. |
| `ssa.database_cache_instruction.go` insert path | pure inserts of new IR; `CreateInBatches` is already the right call. |
| `ensureUnique*` / `deleteDuplicate*` window-function SQL in `ssadb/database.go` | one-time legacy migrations, not hot paths and not expressible with a gorm API. |
| `native_read.go` | read path with hand-built SELECTs. |

## 5. Non-SSA findings (not changed)

No other delete-then-insert batch upsert exists in the tree. The remaining
patterns that could later use `SaveInBatches` are per-row upserts in loops:

* `common/cve/cveresources/*.go`, `common/cve/cvequeryops/Loader.go` — CVE loader
  already batches inserts and uses `OnConflictDoNothing("product")` for products;
  the `FirstOrCreate`-style per-row paths in `cve_descriptor.go` could batch.
* `common/yakgrpc/yakit/*.go` — `yak_script.go` (4), `risk.go` (3),
  `project_kv.go` (3), `kv.go` (3), `fingerprint_general_rule_group.go` (3),
  `risk_exports.go` (2) all do per-row `FirstOrCreate`/`Save`. These are
  interactive, low-row-count paths; batching is a throughput nicety, not a
  correctness fix.
* `common/yak/ssaapi/sfreport/convert_json.go` — the largest `FirstOrCreate`
  count (7) in one file; report conversion writes row-by-row.
* `common/syntaxflow/sfdb/rule_op.go`, `rule_group_op.go` — rule/group upserts
  are per-row; rule sync is batched upstream so the volume is moderate.

None of these were touched: converting CRUD helpers to upserts without a UNIQUE
index or a stable natural key would change semantics (for example losing the
"create once, never overwrite" behavior of `FirstOrCreate`).
