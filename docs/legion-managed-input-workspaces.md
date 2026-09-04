# Legion managed input workspaces

Legion authorizes attachments and sends a versioned `InputManifest` in the typed
AI Bind command. Yaklang resolves the resources before constructing an Agent.
The resolver does not inspect task names, product task versions, Focus names or
release versions. `source.*` is a compatibility adapter for existing Focus
releases; generic file tools use the same managed workspace implementation.

## Contract and admission

- Schema: `legion.input-manifest/v1`; capability:
  `ai.input.managed_attachment.v1`.
- Resource kind: `managed_attachment`, with explicit `required=true` and
  `read_only=true`. Unknown schemas, kinds and policies fail closed.
- The manifest digest is SHA-256 over deterministic protobuf with only
  `manifest_id` cleared. It includes owner, product, Task Run, Session, attempt,
  workspace, initial attempt command ID and every resource's metadata.
- Compare owner and Session to Bind, attempt to `result_context.job.attempt_id`,
  and Task Run to `ai_task_run_id`. `session.run_id` is the Job/Focus Run ID and
  must not be confused with the attempt. All attachment refs must match exactly.
- The initial `attempt_command_id` is immutable across transport rebinds.
  Current Bind metadata and `bind_epoch` can change without changing it.
- The opaque runtime JSON contains `input_manifest_id`, not a second manifest.
  Turn context must repeat that ID and the authorized target. Managed inputs
  currently require a server-authorized immutable Focus in `single_run` mode.
  AI Chat turn attachments and persistent source projects are separate paths.
- Only Linux stateless nodes advertise the capability. Other platforms fail
  closed until equivalent descriptor-relative confinement is implemented.

## Download and storage

`scannode/inputresolver` reconstructs
`/v1/ai/attachments/{resource_id}/download` from the configured Legion API origin
and authenticates with the Node session. Command URLs are ignored. Redirects
are rejected, and no object locator, URL or credential is passed to the model.
Downloads use a 64 KiB buffer, incremental SHA-256, exact size checks, a total
deadline and a progress-reset idle deadline. A private `.part-*` file is renamed
only after integrity and disk synchronization succeed. Every required resource
must be ready before the runtime driver is invoked.

The default root is `$YAKIT_HOME/ai-input-workspaces`. The dedicated root and
workspaces have owner-only permissions. Every preparation receives a fresh
directory, including a transport rebind of the same attempt. Inputs are chmod
read-only after preparation, and outputs are separate. Root permissions and
host paths alone do not grant isolation: model operations use a finite tool set
and descriptor-relative `openat` traversal with `O_NOFOLLOW` for every ancestor
and final component. Inputs are also checked against the manifest allowlist.

The resolver's trusted `Config` owns these limits; they are not task- or
model-controlled runtime options:

| Limit | Default |
| --- | --- |
| One resource | 8 GiB |
| One workspace's inputs | 16 GiB |
| Reserved storage under one root | 32 GiB |
| Writable outputs per workspace | 64 MiB |
| Free disk headroom | 64 MiB |
| Concurrent downloads per resolver | 2 |
| Workspaces per root | 128 |
| Download idle / total deadline | 30 seconds / 2 hours |
| Read page / search result count | 256 KiB / 200 |
| Orphan grace / reclaim batch | 5 minutes / 32 workspaces |

Reservation decisions hold a root allocator lock and account conservatively for
active reservations as well as free disk. A lease records the immutable
manifest and reservation on disk before downloading starts. Allocation writes
the complete lease in a private `.staging/` directory and atomically publishes
the directory into the active namespace. Partial staging directories cannot
poison active reservation accounting; only recognized complete stale staging
leases are reclaimed. A process-held
`flock` protects each active lease. Resolver startup and every new preparation
scan a bounded set of expired leases and reclaim only recognized, unlocked
workspaces. Unknown metadata is retained and causes allocation to fail closed;
operators should inspect it rather than deleting shared directories. Normal
cancel/close/completion cleans only the workspace instance owned by that Bind.

## Agent access and lifecycle

`list_files`, `read_file`, `search_file` and `write_output` are scoped adapters.
They expose logical paths, bounded content and immutable file metadata.
`read_file` supplies `next_offset` for paging; search streams long lines without
loading the whole file. Output writes accept a new flat filename in `outputs/`
and cannot modify an input or overwrite an existing output.

The finite tool manager also rejects guessed tool names and database/MCP
fallbacks. Managed input sessions disable dynamic capability/Forge/skill/child
agent actions, hotpatches, sync commands and extra per-message host file refs.
Server-signed Focus actions and ordinary tool calls remain available; those
tool calls resolve only to the finite scoped tool objects. This is an Agent
capability boundary, not an OS sandbox for arbitrary trusted Yak scripts.

Pending Bind reservations carry cancellation and identity. A higher epoch
cancels an older preparation; duplicate pending deliveries retry instead of
constructing a second engine. Old-epoch cancellation cannot affect a newer
reservation or runtime. Access operations observe cancellation, and cleanup
waits for active reads before removing owned files. A delayed cleanup never
reuses a newer attempt's directory. Managed-input Job event IDs include the
actual transport Bind command namespace, keeping same-Bind retries idempotent
while allowing a newer Bind of the same Attempt to publish accepted results.

Preparation emits `input.workspace.preparing`, `.progress`, `.ready`, `.failed`
and `.cleaned`. Actual reads/searches emit `input.file.access` with resource,
logical path and actual byte/line ranges. Trusted events and report
`structured_summary.input_identity` retain Task Run, Session, attempt, workspace
and manifest IDs. Model-visible workspace info omits private runtime roots and
server actor identifiers. Successful preparation, a file access and a final
report are distinct evidence; none alone proves exhaustive analysis.

## Verification boundary

Focused tests exercise typed manifest admission, log and unrelated synthetic
tasks, failure before runtime construction, bounded reads/searches, output and
symlink denial, download integrity/cancellation/stalls, disk/concurrency limits,
duplicate/replacement fencing, and crash-lease reclamation. They establish T1.
Real product T2 still requires exact committed Legion/Yaklang sources, a
configured provider, a real Task entry, persisted preparation/access/result
events, and a streamed input of at least 1 GiB with measured RSS and late-file
evidence. No unit fixture substitutes for that runtime evidence.
