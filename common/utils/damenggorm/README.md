# DM8 SSA IR storage

The SSA store accepts a native DM URL through `SSA_DATABASE_RAW`:

```text
dm://SSA_USER:URL_ENCODED_PASSWORD@dm.example:5236?schema=SSA_IR
```

Provision a dedicated, uppercase schema before opening the store. The connector
requires an explicit schema and verifies the actual current schema before engine
migration or writes. Use an application account with access to that schema; the
examples and tests do not require a specific administrator account.

Normal compiler initialization runs the registered SSA migrations and index
patches. `SSA_DB_SKIP_MIGRATE=1` retains the existing scan-only behavior: the
schema must already have been initialized by a writer. Node-local profile
databases retain their existing SQLite configuration. PostgreSQL and SQLite
remain available through their existing DSNs.

## Storage behavior

- DM identifiers are quoted in uppercase; returned column labels are adapted
  for GORM's field mapping. SQL literals, comments, and bound values are preserved.
- Unbounded text and JSON payloads use CLOBs bound as text. Indexed names use
  bounded VARCHAR columns, defaulting to `VARCHAR(1024)` when no size is declared.
  Identity columns are read back after each insert. Bulk IR writes use a
  transaction without assuming consecutive generated IDs.
- Code/type replacement includes deletion and insertion in the same DM
  transaction. A failed code batch preserves the previously stored rows.
- Equivalent ordered-column indexes are reused. Existing duplicate data causes
  uniqueness installation to fail; it is not silently deleted. Migration errors
  fail initialization. This is not a general historical-schema conversion tool.
- The native driver uses separate connectors and serialized handshakes to avoid
  shared connection-negotiation state. Cancellation closes the underlying socket
  rather than concurrently mutating the driver's active statements.

## Verification

Unit and SQLite regression checks:

```sh
GOWORK=off go test -mod=readonly -race ./common/utils/damenggorm
GOWORK=off go test -mod=readonly ./common/schema ./common/yak/ssa/ssadb \
  -run 'Test(SQLTextBytes|LargeBatchChunks|UniqueConstraint|SaveIrTypeBatch|RetryProgramDelete|DeleteProgramChecked|DeleteProgramIrCode)' \
  -count=1
GOWORK=off go vet ./common/utils/damenggorm ./common/yak/ssa/ssadb
```

For native tests, supply mode-0600 regular URL files selecting a dedicated DM
schema prefixed with `LEGION_SSA_IR_` and a dedicated PostgreSQL database prefixed
with `legion_ssa_ir_fixture_`. These tests write and delete their own programs;
never select a production IR library. Credentials are read from files and
inherited by child processes through file paths.

```sh
YAK_SSA_DM_ALLOW_FIXTURE=1 \
YAK_SSA_DM_URL_FILE=/private/dm-ir.url \
YAK_SSA_PG_URL_FILE=/private/pg-ir.url \
YAKIT_HOME=/private/isolated-engine-state \
GOWORK=off go test -mod=readonly ./common/yak/ssaapi \
  -run '^Test((DM|Postgres)IRCompileAndCrossProcessReload|DMIRBatchFailurePreservesPreviousRows)$' \
  -count=1 -timeout=6m
```

Both backends compile Java with a long Chinese string, reload it in a fresh
process, execute SyntaxFlow, persist/reload an incremental layer, and delete the
programs. The DM failure case checks rollback after a uniqueness conflict.
On Linux hosts where the bundled libpcap archive rejects PIE linking, the local
test executable can be built with
`-ldflags '-linkmode=external -extldflags=-static'`; this test-only option is not a
release build recipe.

These checks passed on the tested DM8 instance. Production-scale performance,
concurrent compiler workloads, existing PostgreSQL IR conversion, schema
upgrades, and backup/restore require separate validation. Source-hash warnings
were observed in both the DM and PostgreSQL controls; Unicode source content and
the downstream source API were checked separately. The broader `common/schema`
vet check also reports the pre-existing malformed tag in
`syntaxflow_scan_task.go:40`; it is not counted as passing.
