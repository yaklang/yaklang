# Resident Legion platform assistant

Build `go build ./cmd/legion-platform-assistant` and run the executable with
`--legion-url https://legion.example --listen 127.0.0.1:8094`.
Supply a random secret of at least 32 bytes through
`LEGION_ASSISTANT_SERVICE_SECRET`; configure the same service secret in Legion.
Use an authenticated internal TLS channel when deploying across hosts.

`GET /healthz` reports process availability. `POST /v1/turns` accepts protojson
`AssistantTurnRequest` under a service-secret bearer and returns NDJSON
`AssistantRuntimeEvent` records. Each invocation creates and closes a fresh
AIEngine. Legion reconstructs history, chooses the exact allowed platform tools,
and provides a distinct short-lived, user-scoped delegation token. The runtime
never adds transport identity or tokens to model prompts. Model and tool calls
use that delegation token against the fixed configured Legion origin:

- `/v1/ai/assistant/runtime/model`
- `/v1/ai/assistant/runtime/tools`

Callbacks reject redirects and non-200 responses. Service and delegation
credentials have separate purposes. The process does not resolve global model
credentials or process-wide AI preset prompts. Each model subrequest honors
both its own cancellation and the parent turn cancellation. The profile bypasses general AIEngine options and hard-restricts
tool lookup and action dispatch; no builtin tools, shell, code, skills, MCP,
Forge, planning or persistent engine/checkpoint database is enabled. Perception
is disabled, and helper coordinator construction is refused. Direct answers
and tool parameters use the same restricted parent configuration. Endpoint,
guardian and channel resources stop with the effective configured context.

Defaults are 16 concurrent turns, two turns per user, three minutes per turn,
12 engine iterations, 256 KiB input and 1 MiB callback/output bounds. Active turn
IDs cannot be reused. Saturation returns 429 and active duplicates return 409.
Request cancellation cancels model/tool HTTP requests and releases admission.
Termination signals cancel active requests before bounded HTTP shutdown.
Legion owns durable turn state, confirmation, and action execution; prepare
capabilities only create proposals. A truncated stream must be treated as failed
by the caller unless a terminal event was received.

Focused verification:

```sh
GOCACHE=/tmp/legion-platform-assistant/go-cache go test ./common/ai/platformassistant ./common/aiengine -run '^TestPlatform' -count=1
```

The CLI accepts `--listen`, `--legion-url`, `--max-concurrent`, `--max-per-user`, and `--timeout`.
`--legion-url` is required; the secret is accepted only through the environment.
A container recipe is at `cmd/legion-platform-assistant/Dockerfile`. Place a
prebuilt Linux `legion-platform-assistant` executable in its Docker build
context, then pass `--listen 0.0.0.0:8094 --legion-url <Legion origin>` at startup.
The recipe copies that exact artifact and runs as UID 65532. It does not build
source or alter product release manifests. Build platform compatibility and
release artifact publication remain the release owner's responsibility.

The platform profile treats a verified final answer as the end of the current
exchange when there is no effective TODO delta or open TODO work. It does not
ask the model for an additional finish decision after delivering that answer.
General agent completion semantics and tool authorization remain unchanged.

Malformed action responses can be corrected within three transaction attempts.
Model callback failures remain terminal to this loop: Legion owns provider retries,
so the runtime does not multiply them. Exhausted format failures emit a stable
error category without forwarding raw engine prompts, causes or HTTP dumps.
