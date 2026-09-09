# MCP stdio process isolation

`yak mcp` and the standalone MCP executable run a supervisor for stdio mode.
The supervisor re-executes the same binary and arguments with an internal worker
environment. SSE and HTTP continue to run directly.

The implementation follows Memfit's parent/worker lifecycle, with a separate
protocol connection instead of prefix-filtering worker stdout:

```text
client stdin/stdout <-> supervisor <-> authenticated loopback TCP <-> worker
client stderr      <---------------- inherited stdout/stderr ----------+
```

The supervisor owns client stdout. Worker stdout and stderr are diagnostics from
process creation onward, including package initialization, cached file handles,
tool subprocess output and panic stacks. `DialWorker` opens the protocol connection
after server initialization and clears internal worker credentials from the
environment before tools execute. The listener binds only IPv4 loopback on an
ephemeral port and authenticates the worker using a random per-launch token.

The CLI retains its early stderr logging setup and supervisor stdout reservation.
These protect output in the supervisor itself; worker protocol isolation does not
depend on fd duplication. Yak's stdio supervisor skips database initialization.
Other package initializers still run in each process, so new initializers must not
write directly to client stdout.

## Failure and shutdown behavior

- The worker starts on the first client frame. Failure to launch or initialize can
  therefore be returned against the client's `initialize` request ID.
- Initialization has a 60-second deadline. The worker connects only after its
  databases, tool registry and local client are ready.
- Complete worker responses are drained before outstanding requests receive a
  JSON-RPC `-32603` error. IDs are preserved, including strings and large numbers;
  notifications and server-initiated requests do not complete client requests.
- Errors include the failure reason and, when available, the worker exit code.
  Raw tool logs and panic stacks go to stderr, not into JSON-RPC error data.
- Client EOF half-closes the worker connection and allows five seconds to finish
  queued calls and exit. This is a shutdown grace period, not a tool-call timeout
  while the client remains connected. Cancellation also stops the worker.
- A protocol frame has at most five seconds to finish writing to client stdout.
  After cancellation, a write gets at most 250 ms to finish (or send a small final
  error). If writing cannot finish, the private output handle is closed and the
  write goroutine is joined before cleanup; no further frame is appended to a
  partial response. These are output delivery limits, not model/tool timeouts.
  Incoming client and worker frames have a 64 MiB scanner limit.
- Worker diagnostics inherit the client's stderr descriptor directly. There are
  no supervisor log-copy goroutines for `cmd.Wait` to wait on. A full stderr pipe
  can block the worker's logging, but does not prevent killing/reaping the worker.
  The CLI exits with a nonzero code after a session error without another fatal
  log or panic write to the possibly blocked stderr.
- Workers are waited on and reaped. On Unix, cleanup kills remaining members of
  the worker process group, including tool subprocesses that stayed in that
  group. Other platforms kill the direct worker.
- Failed calls are not replayed and workers are not automatically restarted;
  clients must establish a new session after an abnormal exit.

## Validation

`go test -race ./common/mcp/stdio` exercises real subprocesses: initialization and
tool output, panic/exit/kill, truncated or invalid frames, request correlation,
draining final responses, cancellation, EOF and process cleanup. Backpressure tests
cover unread stdout/stderr, cancellable inherited pipe descriptors, and output
write deadlines. Non-file writers supplied to `Run` must implement `Close` that
interrupts `Write`; arbitrary blocking `io.Writer` implementations are not accepted.

`go test ./common/mcp -run '^TestMUSTPASS_MCPCommandStdioKeepsStdoutJSONRPCOnly$'`
builds both CLI entry points and verifies actual Yak script output and killing
the executing worker while the client's input remains open, including an unread,
full stderr pipe through the CLI exit path.
