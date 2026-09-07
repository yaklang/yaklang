# HTTP/2 client validation

The pooled client used by `HTTPWithoutRedirect` preserves HPACK and connection
flow-control state across stream cancellation. Its read loop must continue
processing late HEADERS/CONTINUATION and DATA for discarded streams; otherwise
one canceled request can corrupt responses or stall unrelated requests.

The regression suite also covers:

- Initial 65,535-byte send windows, partial/exact credit, zero-window cancellation,
  request timeouts during upload, SETTINGS changes, and window overflow scope.
- Early final responses during upload, informational responses, trailers,
  content-length mismatches, invalid fields/status, invalid server prefaces,
  unopened stream IDs, and stream-ID exhaustion.
- GOAWAY draining, REFUSED_STREAM, ambiguous POST failures, complete responses
  racing transport shutdown, and sensitive request headers never being indexed.
- Incremental HPACK decoding, compressed/decoded header limits and continuation
  limits. The fuzz target splits arbitrary HPACK blocks across cancellation.
- Stream recycle, silent connection cancellation, stalled writes, bounded body
  queues, early callback return, response body limits, and slow-consumer isolation.
- More sequential requests than the concurrent-stream limit, coalesced cold
  dials, independent waiting-request cancellation, Clear during dialing, late
  transport disposal, and bounded idle connections across origins.

## Reproduce

```sh
go test ./common/utils/lowhttp/... -count=1 -timeout=15m
go test -race ./common/utils/lowhttp -run 'Test.*(H2|HTTP2|BodyStream|SSE)' -count=3 -timeout=10m
go test -race ./common/utils/lowhttp -run 'TestH2Pool(ClearCancels|DialWaiter)' -count=10 -timeout=3m
go test -race ./common/crep -run 'TestMITMH2_|TestH2Fix_' -count=3 -timeout=10m
go test ./common/minimartian/... -count=1 -timeout=5m
go test ./common/utils/lowhttp -run '^$' -fuzz '^FuzzH2ResponseHeaderBlock$' -fuzztime=30s -parallel=2
go test ./common/utils/lowhttp -run '^$' -bench '^BenchmarkH2ClientRoundTrip$' -benchmem -benchtime=2000x -cpu=4
```

Local validation used Go 1.22.12 on darwin/arm64. The full lowhttp suite,
three-pass H2/SSE race suite, ten-pass dial cancellation suite, three-pass MITM
race suite and minimartian suite passed. An additional five-pass `TestH2` race
run passed after aligning the HPACK fixture with the production 4,096-byte
initial table. That fixture completed 81,601 fuzz executions in 15 seconds;
prior 30-second runs completed 95,171 and 165,151 executions without failure.

`go vet ./common/utils/lowhttp/...` still reports existing findings in
`curl.go` (self-assignment), `exec_test.go` (copying a TLS config mutex), and
`http_response_fix_badcase_test.go` (unreachable code).

## Performance comparison

The benchmark uses a local TLS HTTP/2 server, a warmed pool, repeated request
headers and a two-byte response. Old (`af595e3c76`) and hardened binaries were
run alternately three times, with 2,000 requests per case and GOMAXPROCS=4.
Medians from one local run:

| Case | Before ns/op | After ns/op | Before B/op | After B/op | Before allocs/op | After allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Sequential | 110,341 | 96,363 | 19,320 | 18,716 | 308 | 299 |
| Parallel | 52,345 | 36,421 | 20,146 | 18,312 | 318 | 289 |

Persistent request HPACK state, grouped frame writes, batched WINDOW_UPDATE,
connection reuse and coalesced dials reduce overhead in this workload. These
numbers are local measurements, not latency guarantees: earlier runs while
other tests were active showed substantial timing variation.

## Scope and limits

The unread streaming queue is limited to the advertised 1 MiB stream window.
Normal buffered responses retain their body by API design; callers that need a
body-size limit should set `WithMaxContentLength`, or use `WithNoBodyBuffer`
and a streaming handler. A handler must eventually return; Go cannot forcibly
terminate arbitrary caller code, although closing the body unblocks its reads.

This change preserves the existing empty-DATA request framing for compatibility.
It does not add browser-identical H2 fingerprints, server push, extended CONNECT,
or rewrite the legacy standalone `HTTPRequestToHTTP2` conversion helper. It is
not a claim of complete HTTP/2 conformance or proof that no future defect exists.
