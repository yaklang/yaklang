# AI session startup diagnostics

The node entrypoint writes `component=legion-node event=startup` markers to
stderr. AI session stages emit `status=started`, then `status=completed` or
`status=failed` with monotonic `elapsed_ms`. New diagnostic records contain
fixed stage names, status and timing only; they do not include configuration,
credentials, prompts or raw errors. Existing application logs are separate.

| Stage | Measured work |
| --- | --- |
| `main` | Entry marker after Go package initialization, before argument routing |
| `capabilities_sync` | Total built-in capability synchronization and verification |
| `builtin_tools_sync` | Tool synchronization, including any lazy database initialization |
| `builtin_tool_output_options` | Tool output-option initialization |
| `profile_database_open` | Explicit profile database accessor; the database may already be open |
| `builtin_tools_verify` | Tool-store count and non-empty validation |
| `builtin_forges_sync` | Built-in blueprint synchronization |
| `builtin_forges_verify` | Required blueprint lookup |
| `node_construct` | Scan Node construction |
| `node_run` | Entry marker before running the node and bootstrapping its session |
| `sessionmgr_register` | Post-bootstrap registration callback, including the HTTP request |

`capabilities_sync` contains its child stages: do not add its duration to theirs.
A started stage without a terminal marker identifies the last entered step,
but does not distinguish a blocked operation from process termination or missing
logs. Completion of a void-returning initializer means that it returned; use
the subsequent verification stages to check the resulting capability store.

For a slow new session, correlate container `State.StartedAt` and timestamped
container logs. Time before `main` is outside these application timers. For
that gap, set `GODEBUG=inittrace=1` **before starting an isolated diagnostic
container** using the same image and CPU/memory limits. Go emits package-init
timings to stderr; the interval may also contain loader/runtime work. These
markers do not prove that package initialization accounts for the entire gap.

Keep the normal registration/ready and model-call logs when assembling the
timeline. `sessionmgr_register` completion is not command-consumer readiness,
model first byte, or browser-visible output. Compare fresh and already
initialized data directories without reusing a live session's writable data.
No profiler listener, extra network request or automatic inittrace is enabled
by these diagnostics.
