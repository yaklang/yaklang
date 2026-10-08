# Company SSA IR schema operations

Company SSA IR databases are dedicated PostgreSQL databases. Provision the
owning operations role and restricted runtime roles through the Legion
operations tooling before dispatching SSA work. A node cannot initialize or
migrate a company database.

The `yaklang-node` binary and the full `yak` binary expose the same operations
entrypoint. Supply the operations DSN through an environment variable, then run:

```sh
yaklang-node ssa-ir-schema --action init --company-id company_example --dsn-env LEGION_SSA_IR_OPS_DSN
yaklang-node ssa-ir-schema --action check --company-id company_example --dsn-env LEGION_SSA_IR_OPS_DSN
yaklang-node ssa-ir-schema --action migrate --company-id company_example --dsn-env LEGION_SSA_IR_OPS_DSN
```

`check` is the default action and performs only a binding/version read. `init`
uses the engine's registered SSA models and patches inside one PostgreSQL
transaction, then records schema version 1 in:

```sql
CREATE TABLE legion_company_ir_binding (
    singleton boolean PRIMARY KEY CHECK (singleton),
    company_id text NOT NULL,
    schema_version integer NOT NULL
);
```

The table contains exactly one row with `singleton = true`. The operations role
owns the marker; both runtime roles receive only `SELECT` on it. The compiler
role may perform required DML on SSA tables. The reader role has read access.
Neither runtime role may own tables, create schema objects, or update the marker.
Legion validates these roles when opening the registered company connection.

`migrate` requires an existing binding. Neither action can rebind a database to a
different company or downgrade a newer version. Existing PostgreSQL duplicate
rows or incompatible legacy unique indexes cause a migration failure; the
operations transaction rolls back without silently deleting those rows. Repair
such conflicts explicitly before retrying. Back up an existing database before
running a schema upgrade.

SSA tasks receive only their assigned company DSN in an isolated `distyak`
process. The trusted entrypoint consumes and removes the connection environment,
requires disabled automatic migration, and checks the marker before script
execution. Generic scripts use isolated local databases. Long-lived node and AI
processes reject node-wide SSA DSNs. Company IR deletion runs through the Legion
company repository; the legacy node deletion command refuses company sessions.
