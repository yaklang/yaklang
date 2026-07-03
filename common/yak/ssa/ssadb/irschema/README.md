# irschema — SSA IR DB schema governance

`irschema` is the single source of truth for the SSA IR DB DDL. It governs the
schema version that compile/scan nodes read against, centralizes DDL into one
migrator binary, and provides a CI-enforced drift gate so GORM struct changes
can never silently diverge from production DDL.

See `docs/design-docs/ssa-ir-db-migration-governance.md` for the full RFC.

## Roles

| Role | Component | What it does | DDL rights on IR DB |
|------|-----------|--------------|---------------------|
| Schema source of truth | `irschema` package | Embeds migration SQL + version + checksum; exposes `Check` (read-only) and `Migrate` (DDL). | n/a (library) |
| DDL authority | `cmd/yak-ir-migrator` | The ONLY thing that runs DDL on the IR DB. Standalone binary; no yak engine import. | `ir_ddl_owner` |
| Version gate | scannode | Calls `irschema.Check` per compile/scan task; fail-fast on incompatible. | none (`ir_dml_user`, DML only) |
| Orchestrator | `legion-control ir-migrate` | Shells out to `yak-ir-migrator`, then records the version in business PG. | none (delegates) |
| Reader | `legion-scheduler` | Reads the version row from business PG; injects `expected_ir_schema_version` into task inputs. | none |

The control plane (legion) never imports the yaklang engine tree — it shells
out to the migrator binary and reads the resulting version from business PG.

## Adding a schema change

When you edit a GORM struct tag in `common/yak/ssa/ssadb/` (e.g. add a column,
change a type, add an index):

1. **Bump the version**: edit `version.go` and increment `CurrentIRSchemaVersion` (e.g. 1 → 2).
2. **Add a migration file**: create `migrations/0002_<short_description>.up.sql` with the additive DDL. The filename MUST be `NNNN_*.up.sql` (zero-padded, contiguous versions starting at 1).
3. **Regenerate the baseline snapshot JSON** if you changed the baseline (only for v1 changes — see "Snapshot regeneration" below). For v2+ this is unnecessary; the snapshot is only the v1 baseline.
4. **Run the drift test locally** to confirm the migration SQL produces the same schema as the new GORM structs:
   ```bash
   # start a Postgres 16 (docker run -d --name pg -p 5435:5432 -e POSTGRES_PASSWORD=x -e POSTGRES_USER=palm-user -e POSTGRES_DB=palm postgres:16)
   PALM_POSTGRES_HOST=127.0.0.1 PALM_POSTGRES_PORT=5435 PALM_POSTGRES_USER=palm-user PALM_POSTGRES_PASSWORD=x PALM_POSTGRES_DB=palm TEST_POSTGRES=1 \
     go test ./common/yak/ssa/ssadb/irschema/ -run TestIRSchemaDrift -count=1 -v
   ```
   The test fails with a concrete diff if the migration SQL does not match what GORM AutoMigrate produces.
5. **CI enforces it**: `.github/workflows/ir-schema-drift.yml` runs the same test on every PR with a `postgres:16` service container. A struct change without a matching migration file fails the build.

## Operator runbook

### Fresh IR DB (first deploy)

```bash
yak-ir-migrator --dsn 'postgres://ir_ddl_owner:***@host:5436/ir_db?sslmode=disable'
# → {"status":"migrated","version":1}, exit 0
legion-control ir-migrate --config configs/legion-control.yaml --dsn 'postgres://ir_ddl_owner:***@host:5436/ir_db?sslmode=disable' --migrator-binary /usr/local/bin/yak-ir-migrator
```

### Existing AutoMigrated IR DB (adopt)

The migrator detects an existing IR DB with no `ir_schema_migrations` table
and runs a baseline adoption:

- **Matches baseline**: the DB is stamped at v1 with NO DDL (idempotent).
- **Drifts from baseline**: the migrator refuses with exit code 2 and prints a
  structured diff. The operator must either:
  - reconcile the DB manually (ALTER it to match the baseline), then re-run, or
  - pass `--force-adopt` (DESTRUCTIVE: stamps v1 without verifying the schema;
    use only after manual review and only when the drift is intentional/known).

### Rolling upgrade (v1 → v2)

```
T0  IR DB at v1, all nodes on yaklang v1
T1  deploy yaklang v2 binaries to staging
T2  legion-control ir-migrate --dsn <ir-db>     # applies 0002_*.up.sql, stamps v2
T3  rollout new scannode (v2 binary) to one node
    → Check sees DB at v2, expects v2 → Compatible → proceeds
T4  old scannode (v1 binary) still running
    → Check sees DB at v2, expects v1 → NOT Compatible (too-new)
    → emits ScanResult_IRSchemaIncompatible, fails fast
    → scheduler stops dispatching to v1 nodes
T5  drain + replace remaining v1 nodes with v2
T6  all nodes on v2, schema at v2
```

No v1 node ever touches the v2 DB. Two migrator instances serialize on
`pg_advisory_xact_lock`.

## Snapshot regeneration (baseline only)

`migrations/0001_baseline.snapshot.json` is the in-binary structural mirror of
`0001_baseline.up.sql`, used by the adoption logic to decide whether a
pre-governance DB matches the baseline. To regenerate after a baseline edit
(rare; prefer additive v2+ migrations instead):

```bash
# 1. Apply the (edited) baseline SQL to a fresh Postgres 16.
docker run -d --name pg16 -p 5435:5432 -e POSTGRES_PASSWORD=x -e POSTGRES_USER=palm-user -e POSTGRES_DB=palm postgres:16
createdb -h 127.0.0.1 -p 5435 -U palm-user irbase
psql -h 127.0.0.1 -p 5435 -U palm-user irbase < common/yak/ssa/ssadb/irschema/migrations/0001_baseline.up.sql
# 2. Snapshot it (use the Snapshot API via a tiny throwaway main, or yak-ir-migrator --dump-snapshot if added).
# 3. Write the JSON to migrations/0001_baseline.snapshot.json.
# 4. Run TestIRSchemaDrift to confirm it matches the freshly-applied SQL.
```

## Phase 2: DDL/DML privilege split

Grant scripts live in `legion/packaging/ir-db-roles/` (legion repo). The
migrator uses the `ir_ddl_owner` DSN; scannode receives only the `ir_dml_user`
DSN (SELECT/INSERT/UPDATE/DELETE, no CREATE/ALTER). `ALTER DEFAULT PRIVILEGES`
on `ir_ddl_owner` ensures `ir_dml_user` auto-gets DML on future migrator-created
tables.

## Phase 3: forward compatibility

`Check` accepts a DB at version `N` or `N-1` (`MinSupportedIRSchemaVersion`).
A DB newer than `CurrentIRSchemaVersion` is rejected (too-new), so an old node
never silently misinterprets columns a newer binary introduced. Bump
`MinSupportedIRSchemaVersion` only when the rollout cadence requires a wider
window.