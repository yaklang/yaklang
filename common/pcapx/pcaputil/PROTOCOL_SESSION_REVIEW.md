# #5099 session correctness review

Reviewed the M0/M1 implementation merged into #5013 at `e452c839cd`.
Fixes use the existing capture `binFlow` path. Concurrent MQTT 5.0 work from
#5100 is retained; the rule archive is regenerated from both sets of sources.

## Corrected behavior

- `ProtocolSession` applies zero defaults per field, enforces structural/frame
  budgets, returns typed errors, releases returned events, and makes Close
  idempotent. Feed after Close consumes no bytes. Calls on one session must be
  serialized, as must the ordered TCP input they represent.
- WebSocket fragmentation is independent in each direction. Orphan continuation,
  interrupted fragments, wrong masking direction, post-Close frames and
  oversized aggregate messages fail explicitly. Buffered capacity is reserved
  before growth; unfinished messages are reported on capture close.
- HTTP upgrade tracking follows each queued request. A rejected earlier upgrade
  cannot authorize an unrelated 101 response. Header tokens are parsed rather
  than matched as substrings; negotiated extensions are explicitly unsupported.
- gRPC requires an observed gRPC content type in that direction, rather than
  treating arbitrary HTTP/2 DATA as a message. Each stream/direction has its own
  bounded partial message. Final DATA is processed even when HTTP/2 retires the
  stream, and invalid compressed flags, excessive declared sizes and truncated
  messages at END_STREAM fail instead of being marked decoded.
- Redis admission needs a validated prefix, not an entire bulk body inside the
  probe budget. Length arithmetic, terminators, scalar syntax, aggregate counts
  and nesting are bounded. RESP arrays accept arbitrary RESP elements; existing
  bulk-first command fields remain available. Array shape does not establish a
  sender role: metadata reports per-direction arrivals, not inferred outstanding
  requests. RESP3 push values do not claim request/response correlation.
- LDAP request tracking checks MessageID, operation and direction. StartTLS
  switches only after a successful matching ExtendedResponse, whose optional
  responseName need not repeat the OID. Abandon removes its target MessageID.
  BER bounds and outstanding requests honor the structural budget. The prior
  positive write fixture now uses distinct IDs for concurrent operations.
- PostgreSQL short typed messages can be admitted, but ambiguous C/D/E/S layouts
  require observed direction. The parser no longer guesses ErrorResponse from
  an uppercase Execute portal name. MySQL's partial greeting retains precedence
  when its first five bytes resemble a PostgreSQL header.
- Existing HTTP/2/MySQL capture status for connection-state budget exhaustion
  remains `context-required`; the new session API also carries the precise
  `ResourceExceeded` kind.

## Validation

`protocol_session_review_test.go` reproduces the original failures through
Probe/Feed/Close, including duplex WebSocket/gRPC traffic, ordinary HTTP/2 DATA,
Redis overflow/truncation, LDAP StartTLS/Abandon, closed sessions and budgets.

```sh
go test ./common/bin-parser/... -count=1 -timeout=10m
go test ./common/pcapx/pcaputil ./common/pcapx/cmd/pcap-inspect -count=1 -timeout=5m
go test -race ./common/pcapx/pcaputil -count=1 -timeout=5m
GOMAXPROCS=4 go test ./common/pcapx/pcaputil -run '^$' \
  -fuzz '^FuzzProtocolSessionReview$' -fuzztime=45s -parallel=4
```

The 45-second fuzz run exercised 155,157 cases and checked no recovered flow
panic, bounded peak buffered bytes, and zero retained buffer bytes after Close.
Full regression caught the short-header MySQL collision and legacy budget
status change; both were corrected without weakening the existing assertions.

## Boundaries

This is passive, bounded session observation, not a claim of complete protocol
conformance. WebSocket handshake authentication, negotiated extensions, TLS
decryption, gRPC decompression/protobuf schemas, Redis streamed aggregates and
RESP3 attributes remain outside this implementation. Unsupported wire forms
must not be presented as complete decoded application messages. Catalog status
is not promoted to `done` by these fixes.

Protocol references: [WebSocket RFC 6455](https://www.rfc-editor.org/rfc/rfc6455),
[LDAP RFC 4511](https://www.rfc-editor.org/rfc/rfc4511),
[Redis RESP specification](https://redis.io/docs/latest/develop/reference/protocol-spec/),
[gRPC HTTP/2 protocol](https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-HTTP2.md),
[PostgreSQL message formats](https://www.postgresql.org/docs/current/protocol-message-formats.html).
