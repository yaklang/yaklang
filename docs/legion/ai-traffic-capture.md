# AI HTTP traffic capture protocol v1

Legion owns access control, durable traffic evidence, session quotas, masked
views and export/analysis authorization. Yaklang owns the actual instrumented
HTTP attempts and their local upload queue. The two repositories exchange the
versioned messages in `scannode/proto/legion/ai/v1/ai.proto`; they do not import
each other's implementations.

## Capture boundary

An opted-in session receives `BindAISessionCommand.traffic_capture`. The tool
caller temporarily associates its immutable call ID with the owning engine's
observer. HTTP/1, HTTP/2 and HTTP/3 `lowhttp` transports notify that observer for
each transport attempt, including status retries, redirects, authentication
retries, protocol fallback, connection failures and cancellation. HTTP/2's
internal stream retries are separate observations. A saved HTTPFlow is never
used to infer the attempt count. Existing HTTPFlow query behavior is unchanged.

The captured packets are the HTTP representation at the transport boundary,
not TLS records or HTTP/2/HTTP/3 binary frames. An attempt can fail before any
request byte reaches the target; its lifecycle still records the attempted
request and failure. External processes, browsers and uninstrumented HTTP
clients are explicitly unsupported in the `ai_traffic_capability` event.
Model-provider calls are excluded because only tool runtime IDs are bound.

The start record fixes session, turn, tool, agent, node, node-session and bind
epoch. Start and terminal uploads have the same random flow ID and independent
phase keys. Metadata carries only method, scheme, host, status, timestamps,
lengths, hashes, truncation and fixed error codes. It never contains URL queries,
headers, bodies, response error text, credentials or local packet file paths.

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
The runtime persists this receipt before removing the upload. A process restart
with the same binding identity reuses queued bytes and receipts; ambiguous acknowledgement never deletes data.
Upload failure retains the queue and retries in subsequent worker cycles.

## Drain and failure boundary

The existing node command lane accepts `ai.session.traffic.drain` with
`DrainAITrafficCommand`. It checks the bind epoch and node-session identity,
stops accepting new observations and waits at most 30 seconds for active
attempts and queued receipts. `ai_traffic_drain` contains
`AITrafficDrainResult`, including command ID, pending count and completion.
The dedicated acknowledgement is published even if the engine has retired.
Normal runtime Close/Cancel closes the engine then attempts the same bounded
drain. Unacknowledged local data is retained. Forced process/container loss
cannot manufacture a receipt; the platform must retain an incomplete state.

## Read-only evidence analysis

The `ai.traffic.analysis.readonly.v1` capability is advertised only by the
stateless runtime. A separate analysis session binds server-owned
`traffic_analysis` with source session and flow IDs. Every later input must carry
server-assembled read-only evidence for those references. Hotpatches, sync and
interactive overrides, executable Focus/Forge, MCP, managed workspaces,
attachments and credentials are rejected. Engine setup disables tools, search,
planning, perception and automatic skills. An immutable config flag additionally
rejects the tool invocation entry point and propagates to child agents. User
prompts and mutable runtime flags cannot clear it. Evidence text is bounded and
untrusted; findings are instructed to cite exact `[flow:<flow_id>]` references.

## Focused verification

Run compilation and tests on the authorized remote WSL host using the exact
worktree commit. These tests do not claim deployment/provider acceptance:

```sh
go test ./common/utils/lowhttp -run '^TestHTTPAttemptObserver' -count=1
go test ./common/ai/aid/aicommon -run '^TestReadOnlyEvidence' -count=1
go test ./scannode -run '^(TestResilienceAITraffic|TestAITraffic|TestProductNodeManifestCapabilitiesMatchCompiledSurface)' -count=1
```

Real acceptance additionally exercises the platform ingest route, durable
storage, cancellation/drain, node loss, masked evidence analysis and UI state
against the same paired Legion/Yaklang source commits.
