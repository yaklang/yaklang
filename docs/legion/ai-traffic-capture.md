# AI HTTP traffic capture protocol v1

Legion owns access control, durable traffic evidence, session quotas, masked
views and export/analysis authorization. Yaklang owns the actual instrumented
HTTP attempts and their local upload queue. The two repositories exchange the
versioned messages in `scannode/proto/legion/ai/v1/ai.proto`; they do not import
each other's implementations.

## Capture boundary

An opted-in session receives `BindAISessionCommand.traffic_capture`. Its execution
context carries the HTTP observer through existing tool and script context
propagation. Capture does not bind tool IDs or change tool `runtime_id` parameters.
HTTP/1, HTTP/2 and HTTP/3 `lowhttp` transports notify that observer for
each transport attempt, including status retries, redirects, authentication
retries, protocol fallback, connection failures and cancellation. HTTP/2's
internal stream retries are separate observations. A saved HTTPFlow is never
used to infer the attempt count. Existing HTTPFlow query behavior is unchanged.

The captured packets are the HTTP representation at the transport boundary,
not TLS records or HTTP/2/HTTP/3 binary frames. An attempt can fail before any
request byte reaches the target; its lifecycle still records the attempted
request and failure. External processes, browsers and uninstrumented HTTP
clients are explicitly unsupported in the `ai_traffic_capability` event.
Model-provider network contexts explicitly suppress the observer while retaining
cancellation, deadlines and other context values.

The start record fixes session, turn, node, node-session and bind epoch.
Protocol field 5 and its former `tool_call_id` name are reserved. Start and terminal
uploads have the same random flow ID and independent phase keys. Metadata carries method, scheme, host, status, timestamps, lengths,
hashes, truncation, fixed error codes and the optional protocol-v1
`AITrafficRecord.redacted_url` (field 25). The absolute URL is captured at request
start and retained in terminal records even when packet headers are unavailable
or local/platform byte quotas drop raw evidence. It is limited to 16 KiB and must
match the record's scheme and host. Userinfo and fragments are removed. Sensitive
query values and path segments following sensitive keys are replaced with
`[REDACTED]`, using the same key rules as Legion URL redaction. Invalid or oversized
URLs are omitted rather than copied into metadata. Legion validates origin and
size again and applies its own redaction before persistence. Older producers that
omit the field retain the legacy scheme/host fallback until packet metadata is
available. Metadata never contains raw sensitive URL values, headers, bodies,
response error text, credentials or local packet file paths.

## Durable queue and receipts

The runtime writes each upload to a private `legion-ai-traffic/<identity-hash>`
directory under `YAKIT_HOME`, using temporary file, fsync, rename and directory
fsync. Packet defaults are 10 MiB per combined request/response and 1 GiB admitted
bytes per session generation. Server configuration may raise these values up
to hard limits of 64 MiB per combined packet and 64 GiB per session. Both HTTP
headers take priority over bodies within the combined budget. Remaining bytes
are shared between bodies, reallocating unused space to the larger side. If
the two headers alone exceed the limit, bounded prefixes are retained with
`packet_headers_limit`; ordinary body truncation uses `packet_limit`. The runtime bounds its admission; Legion is the final
quota authority. Oversize local header/body files are read as actual bytes,
never uploaded as local path placeholders. Size and truncation remain explicit.

The collector accepts no directory override. Its root comes from local operator
configuration; remote session and node-session identifiers are hashed before
selecting the per-generation directory. Atomic writes require an absolute
directory and a plain direct-child basename, reject traversal and absolute file
names, and verify the final relative path before creating or replacing a file.
Temporary-file and metadata-acknowledgement cleanup failures are logged. A
metadata marker cleanup failure does not invalidate an already durable HTTP
receipt or claim missing packet evidence.

The queue sends sanitized `AITrafficBatch` metadata through existing
`AISessionEvent` messages (`event_type=ai_traffic_batch`, protobuf JSON with
original field names). Packet upload is `POST
<traffic_capture.upload_base_url>/<session_id>/records`, with
`application/x-protobuf` body `AITrafficUpload`. Authentication uses the existing
node session secret as Bearer token and `X-Node-Session-ID`. The endpoint must
share the authenticated platform origin; redirects are refused.

Legion returns `AITrafficReceipt` only after the admitted metadata and bytes are
durable. The receipt must match protocol version, flow ID, phase and SHA-256 of
the exact uploaded protobuf bytes, and have `durable=true`. A quota rejection
uses `storage_status=quota_dropped`, preserving metadata and ending retries.
`storage_status=metadata_only` confirms durable metadata without packet bytes;
it also ends retries without claiming that a packet object exists.
The runtime persists this receipt before removing the upload. A process restart
with the same binding identity reuses queued bytes and receipts; ambiguous acknowledgement never deletes data.
Metadata publication uses the runtime's confirmed event publisher. A successful
acknowledgement is fsynced beside the queued upload, bound to its SHA-256, so
packet upload retries and restarts do not publish the same metadata again.
An absent or ambiguous acknowledgement is replayed while the upload remains
queued. Each broker acknowledgement wait is bounded to one second and the
calling drain deadline, and also observes runtime cancellation. HTTP ingest
receipts confirm the authoritative durable metadata as well as admitted bytes.
After that receipt the queue and matching broker marker are removed; the node
does not continue retrying redundant broker events.
Upload failure retains the queue and retries in subsequent worker cycles.

## Drain and failure boundary

The existing node command lane accepts `ai.session.traffic.drain` with
`DrainAITrafficCommand`. It checks the bind epoch and node-session identity,
stops accepting new observations and waits at most 30 seconds for active
attempts and queued receipts. `ai_traffic_drain` contains
`AITrafficDrainResult`, including command ID, pending count and completion.
The dedicated acknowledgement is published even if the engine has retired.
Normal runtime Close/Cancel closes the engine then attempts the same bounded
drain. Automatic completion and failure drain before publishing the terminal
event; manager cleanup also drains before removing the runtime or workspace.
The automatic drain stops its background worker after the bounded attempt.
A subsequent platform drain retries remaining uploads within its own deadline,
even when that worker has stopped. Broker metadata failure does not block
HTTP recovery. It never waits for an unrelated uploader's
lock past the command deadline. The drain is idempotent and does not close the
engine from its own callback.
Unacknowledged local data is retained. Forced process/container loss
cannot manufacture a receipt; the platform must retain an incomplete state.
Capture errors and terminal metadata-only/quota receipts persist an incomplete
coverage marker. Even after restart, an empty queue then returns `complete=false`
immediately; uploading metadata does not repair unavailable packet evidence.

## Read-only evidence analysis

The `ai.traffic.analysis.readonly.v1` capability is advertised only by the
stateless runtime. A separate analysis session binds server-owned
`traffic_analysis` with source session and flow IDs. Every later input must carry
server-assembled read-only evidence for those references. Hotpatches, sync and
interactive overrides, executable Focus/Forge/Skill context or runtime options, MCP, working directories, managed workspaces,
attachments and credentials are rejected. Engine setup disables tools, search,
planning, perception and automatic skills. The final engine options enforce the
existing `DisableToolUse` setting after provider and message options. The tool
invocation entry point checks that setting, and child configurations inherit it.
Analysis inputs cannot supply hotpatches to reopen tool execution. Evidence text is bounded to
1 MiB of UTF-8 bytes; the platform also reserves command transport overhead. It is
untrusted; findings are instructed to cite exact `[flow:<flow_id>]` references.

## Focused verification

Run compilation and tests on the authorized remote WSL host using the exact
worktree commit. These tests do not claim deployment/provider acceptance:

```sh
go test ./common/utils/lowhttp -run '^TestHTTPAttemptObserver' -count=1
go test ./common/ai/aid/aicommon -run '^(TestReadOnlyEvidence|TestToolCallerHTTPAttemptObserver)' -count=1
go test ./common/ai/openai -run '^TestGatewayExcludesSessionHTTPAttemptObserver$' -count=1
go test ./common/ai/aid/aitool/buildinaitools/yakscripttools/test -run '^TestDoHTTPRequestCapturesSessionContextWithoutToolIdentity$' -count=1
go test ./scannode -run '^(TestResilienceAITraffic|TestAITraffic|TestProductNodeManifestCapabilitiesMatchCompiledSurface)' -count=1
```

Real acceptance additionally exercises the platform ingest route, durable
storage, cancellation/drain, node loss, masked evidence analysis and UI state
against the same paired Legion/Yaklang source commits.
