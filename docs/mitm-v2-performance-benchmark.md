# MITM V2 Performance Benchmark

This benchmark establishes a repeatable before/after contract for the MITM V2
performance work. It uses a loopback upstream server and temporary SQLite files;
it does not send traffic to real targets or modify the active Yakit project.

## Quick Start

Capture a low-resource baseline from the backend worktree:

```bash
GOMAXPROCS=4 GOMEMLIMIT=4GiB go run ./cmd/yak-mitm-perf capture \
  -profile smoke \
  -frontend-dir /path/to/yakit-plugin-history \
  -out reports/mitm-perf/before/report.json
```

After an optimization, run the same command with a different output path. Do
not change profile, concurrency, body size, seed rows, Go version, or machine:

```bash
GOMAXPROCS=4 GOMEMLIMIT=4GiB go run ./cmd/yak-mitm-perf capture \
  -profile smoke \
  -frontend-dir /path/to/yakit-plugin-history \
  -out reports/mitm-perf/after/report.json
```

Compare the reports and fail when a same-name metric regresses by more than 15%:

```bash
go run ./cmd/yak-mitm-perf compare \
  -baseline reports/mitm-perf/before/report.json \
  -candidate reports/mitm-perf/after/report.json \
  -max-regression 15
```

The comparison also fails if profile inputs, Go/runtime limits, metric coverage,
or exercised correctness checks differ. A skipped race/frontend check cannot
silently replace one that ran in the baseline.

Repeated benchmarks use the percentage threshold directly. One-shot
`mitm.*` integration samples also use explicit absolute noise floors so values
near zero do not produce false failures. `standard`/`stress` use query P95
10 ms and goroutine peak/delta 5; single-repetition `smoke` uses query P95
25 ms, request P95 10 ms, and goroutine peak/delta 10. All profiles use
shutdown 15 ms, post-shutdown CPU 0.01 core, and queue peak 10 items. The table
and JSON retain the raw change and mark it `noise`; a change beyond both the
percentage threshold and its absolute floor is still a regression.

`reports/mitm-perf/` is ignored by Git. Keep reports as CI artifacts or attach
them to the performance issue/PR; do not commit machine-specific numbers.

## Profiles And Resource Limits

| Profile | Requests/scenario | Concurrency | Body | Seeded rows | Repetitions | Use |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| `smoke` | 40 | 4 | 32 KiB | 1,000 | 1 | Local iteration |
| `standard` | 200 | 8 | 64 KiB | 20,000 | 3 | PR before/after evidence |
| `stress` | 1,000 | 16 | 64 KiB | 100,000 | 3 | Explicit scheduled run |

The runner defaults to `GOMAXPROCS=4` and a 4 GiB Go memory soft limit. The
stress profile is never selected implicitly. Any override must be identical in
the before and after captures. On Linux/WSL, CPU metrics use process
`getrusage` user+system time; average cores therefore maps approximately to the
CPU percentage shown by system monitors (`4.0 cores` is roughly `400%`).

## Coverage For The Seven Work Items

| Work item | Before/after metrics | Non-performance gate |
| --- | --- | --- |
| 1. Controlled A/B | `mitm.baseline.*`, `mitm.trafficguard_off.*`, `mitm.realtime_off.*`, including average CPU cores and CPU milliseconds/flow | Every scenario must complete and persist all flows; report inputs must match |
| 2. TrafficGuard worker/budget | TrafficGuard `ns/op`, `MB/s`, baseline request/persist throughput, goroutine peak | Findings behavior remains covered by existing TrafficGuard tests |
| 3. DB batching/backpressure | Persist throughput, queue peak, enqueue→SQL P95, SQL P95, post-stop drain time | Queue must drain and every accepted flow must persist |
| 4. Query/index/session changes | Fresh/seeded client P95, backend SQL/COUNT P95, protobuf conversion P95, active-query P95/in-flight | Focused race gate must pass after shared Gorm mutation is fixed |
| 5. Frontend refresh/manual list | Request/response/persist→direct-probe P95, database watcher and Duplex delivery P95, query overlap, Vitest manual-list mean/P99/ops-per-second | Every realtime repetition must deliver an HTTPFlow push and a bounded timing payload; manual list transition unit tests must pass |
| 6. Stream/Flow race fixes | Goroutine peak plus optional focused `-race` capture | No `WARNING: DATA RACE`; concurrent stream writes need a dedicated correctness test when fixed |
| 7. Shutdown protocol | Stream shutdown, queue-at-stop, post-stop drain, post-shutdown CPU, goroutine delta | Shutdown completes within 10 seconds and accepted flows follow the chosen flush policy |

Run the focused race gate as a separate, more expensive capture:

```bash
GOMAXPROCS=4 GOMEMLIMIT=4GiB go run ./cmd/yak-mitm-perf capture \
  -profile smoke \
  -include-race \
  -frontend-dir /path/to/yakit-plugin-history \
  -out reports/mitm-perf/race/report.json
```

The `race.mitmv2_query_write` check concurrently queries and writes the project
database, retaining the command output as a report artifact. The focused test
has a 90-second execution timeout; its outer command allows up to 20 minutes so
a cold race-instrumented dependency build is not misreported as a data race.

## Interpretation Rules

- Compare reports from the same machine. CPU frequency policy and concurrent WSL
  workloads can move short benchmarks substantially.
- Use `smoke` to catch large regressions, not to claim a small improvement.
- Use `standard` with at least three repetitions for performance claims.
- A throughput improvement is invalid if request, persistence, race, or shutdown
  checks fail.
- Queue depth and goroutine count are boundedness metrics. Lower is normally
  better, but a zero push count can also mean notifications are broken; retain
  functional tests for delivery semantics.
- Each integration scenario performs one measured warm-up request and waits for
  it to persist before the steady-state sample. This preserves lazy-start cost
  as `warmup_request_ms` without biasing the TrafficGuard on/off throughput A/B.
- Each realtime query requests bounded `SystemTiming` and correlates unique
  scenario rows by token/ID. The report separates MITM flow construction,
  async-write queue wait, SQL insert, database watcher, backend query/conversion,
  and Duplex delivery instead of treating one client duration as the cause.
- The loopback probe covers the backend and the gRPC boundary that Electron
  consumes. Metrics named `*_to_probe_receive_*` explicitly exclude Electron
  structured clone, Renderer scheduling, React commit, and Chromium paint.
  Those stages are available in the in-app observability snapshot. The Yakit
  worktree now provides a real-engine WDIO MITM V2 HTTP performance scene that
  starts this Yak worktree, drives the UI, captures Renderer/React timing and
  verifies project DB/table correctness. It remains a separate report until a
  parent reporter merges both benchmark formats.

Run the Electron-side `standard` scene from the Yakit worktree after building
its Renderer test artifacts:

```bash
yarn test:e2e:build
YAKIT_E2E_MITM_PROFILE=standard yarn test:e2e:electron:mitm-performance
```

The local bounded build is tagged `production-unminified`; compare it only to
the same build mode. Fixed release workers should use the explicit minified E2E
build and at least three repetitions. See the frontend `e2e/README.md` for the
idle gate, resource limits, report contract and comparison command.

Electron harness version 4 records request and response body sizes separately,
plus every live cycle's trigger source, cursor, high-water mark, rows and packet
bytes. Scheduler idle gaps, Long Task count and Long Task blocking ratio are
diagnostic. The ratio uses the load/drain observation window as its denominator,
so a faster candidate can report a larger ratio simply because it drains sooner.
Absolute Long Task total duration, p95 and maximum remain regression gates.

The body matrix now covers small, request-heavy, response-heavy and
bidirectional traffic. Request-body cases send real POST bodies, the loopback
target checks the exact received byte count, and the MITM list query must contain
neither raw Request nor raw Response. After the performance observation window,
`GetHTTPFlowById` checks that both complete packet bodies are still available;
moving this detail lookup outside the window prevents the correctness assertion
from contaminating renderer timing.

Use the matrix runner's `--repeat 3` option or
`yarn test:e2e:electron:mitm-matrix:repeat` for strictly sequential stability
samples. Each repetition uses a new project database and Electron user-data
directory. The aggregate preserves scalar measurements as the median and also
reports min/P50/P95/max, mean, population standard deviation, median absolute
deviation, coefficient of variation and relative range. Baseline and candidate
must use the same repeat count; a repeated candidate does not turn an older
single baseline into a formal statistical comparison.

The final bounded WSLg A/B used the same `production-unminified` Renderer, Yak
`GOMAXPROCS=2` / `GOMEMLIMIT=2GiB`, one WDIO worker and 120 requests at
concurrency 8:

| Body case | Database drain | Renderer drain | Request → React p95 | Query round-trip p95 | Long Task total / p95 | Comparator |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| Request 64 KiB / Response 4 KiB | `2723 → 98 ms` | `2748 → 363 ms` | `3893 → 1156 ms` | `2125 → 116 ms` | `455 → 407 ms` / `100 → 76 ms` | passed |
| Request 64 KiB / Response 256 KiB | `2054 → 190 ms` | `2078 → 460 ms` | `5656 → 1514 ms` | `3362 → 167 ms` | `374 → 395 ms` / `86 → 72 ms` | passed |

In the request-heavy case, list conversion p95 changed from `1070.469 ms` to
`0.256 ms`; list packet bytes were zero while the detail check still observed
exactly 65,536 request-body bytes and 4,096 response-body bytes. The
bidirectional detail check observed exactly 65,536 and 262,144 bytes. These are
single-run engineering samples for hotspot validation, not release claims;
fixed-hardware gates still require at least three identical repetitions.

## Bounded Yak CPU Profiling

The Electron harness can collect a bounded Yak CPU profile during the exact
MITM load window. Run it from the Yakit worktree; the convenience script uses
the request-heavy body case and a five-second capture:

```bash
yarn test:e2e:electron:mitm-cpu-profile
```

For the bidirectional large-body case:

```bash
node scripts/run-electron-mitm-body-matrix.mjs \
  --case bidirectional-64k-256k \
  --cpu-profile-seconds 5
```

The duration is explicitly bounded to 1 through 60 seconds. Diagnostic runs
build a separate symbol-preserving Yak binary, start pprof with
`--pprof --pprof-listen 127.0.0.1:0 --pprof-block-rate 0`, wait for the
versioned `yak grpc pprof ready` event, and cap download size and timeout. They
write `yak-cpu.pprof`, flat/cumulative top reports and
`yak-cpu-summary.json`. Reports are marked `diagnosticOnly`, and the Electron
comparator rejects either side when CPU profiling was enabled. Always repeat a
candidate without profiling before using it as a gate.

The first profiles found two backend costs that were independent of list page
size:

- Linux process attribution performed a full `/proc` walk for bursty
  connections. A short-lived PID hint cache now revalidates the socket inode
  and executable on every hit, serializes only same-stripe fallback searches,
  and retains the exact full-scan fallback. Its request-heavy cumulative share
  changed from `1080 ms / 16.51%` to `50 ms / 0.90%`.
- Teddy's nibble fingerprint produced a candidate at almost every position in
  low-entropy bodies. An exact two-byte prefix bitmap now rejects impossible
  prefixes before confirm without changing the match set. In the bidirectional
  profile, TrafficGuard changed from `3240 ms / 27.76%` to
  `100 ms / 1.27%`; the minirehs C scan itself changed from `3120` to `30 ms`.

The second change is also covered by a stable microbenchmark: a repeated-byte
256 KiB JSON response changed from roughly `24 ms/op` to `0.57 ms/op`, while a
normal JSON/HTML payload changed from roughly `3.5` to `1.56 ms/op`. Random
SIMD/scalar/Go-AC differential tests, real TrafficGuard rules and race tests
guard the no-false-negative contract.

The CPU gains exposed the next bottleneck instead of completing the overall
gate. The first unprofiled 64 KiB request / 256 KiB response candidate improved
throughput from `17.68` to `31.75 req/s` and request-to-React P95 from `5656` to
`1463 ms`, but Yak peak RSS changed from `854` to `1071 MB` and Renderer Long
Task total from `374` to `436 ms`. The comparator therefore correctly remained
failed and the RSS threshold was not relaxed.

## Bounded Yak Heap/Allocation Profiling

Heap diagnostics use the same symbol-preserving, loopback-only pprof fixture as
CPU diagnostics, but the two modes are mutually exclusive. The harness forces
GC and captures one heap profile after the idle gate, then another after load,
database/Renderer drain and CPU recovery:

```bash
yarn test:e2e:electron:mitm-heap-profile
```

It subtracts the baseline profile from the post profile for `alloc_space` and
`inuse_space`, and also reports the absolute post-run live heap. Raw snapshots,
flat/cumulative top reports and `yak-heap-summary.json` are bounded to 64 MiB.
The forced GCs make the whole run diagnostic; the comparator rejects it.

For 120 requests at concurrency 8 with a 64 KiB request and 256 KiB response,
the first profile measured `5,395,723,287 B` of cumulative allocation for only
37.5 MiB of raw bidirectional bodies. `io.ReadAll` accounted for
`3,810,906,733 B / 70.63%`, and `bytes.growSlice` accounted for
`644,438,541 B / 11.94%`. The baseline-to-post live-heap delta was only about
14.8 MiB and the absolute post-GC heap was about 271.3 MiB, identifying a
short-lived copy/allocation storm rather than equivalent retained growth.

`SplitHTTPPacketEx` previously passed its already-complete `[]byte` through
`bufio.Reader` and `io.ReadAll`, geometrically growing a second body buffer. It
now calculates the exact unread length and performs one allocation/copy while
preserving the existing independent-body ownership contract. A 256 KiB
microbenchmark changed as follows (five 500 ms samples; medians shown):

| Metric | Before | After | Change |
| --- | ---: | ---: | ---: |
| Time | `207.2 µs/op` | `67.8 µs/op` | about `3.05x` faster |
| Allocated bytes | `1,191,665 B/op` | `267,750 B/op` | `-77.5%` |
| Allocations | `52 allocs/op` | `32 allocs/op` | `-38.5%` |

Repeating the exact heap scenario reduced total allocation to
`2,999,089,540 B`, a reduction of `2,396,633,747 B / 44.4%`; absolute post-GC
live heap remained effectively flat at about 269.9 MiB. Full `common/utils/lowhttp`
tests, explicit 4 KiB reader-boundary/ownership tests and targeted race tests
passed.

The final unprofiled candidate passed against the immediately preceding
candidate: throughput `31.75 → 38.38 req/s`, Renderer drain `853 → 432 ms`,
request-to-React P95 `1463 → 1092 ms`, Query round-trip P95 `192.8 → 122.1 ms`,
and Yak peak working set `1071 → 915 MB`. Against the original baseline, Yak
working set is now within the 15% threshold (`854 → 915 MB`), but Renderer Long
Task total is still a regression (`374 → 443 ms / +18.4%`). The overall gate
therefore remains failed; the next attribution target is CDP/React work, not a
looser backend memory threshold.

### Read-only HTTPFlow body inspection

The next allocation pass preserved the copying APIs and added an explicitly
read-only packet-body view for callers that inspect a complete packet only
within the current call stack. Only the HTTPFlow metadata, truncation-decision
and large-request-spill paths were migrated. Actual truncation still allocates
an independent packet, and tests assert that the input packet is unchanged.

`BenchmarkCreateHTTPFlowBodyMatrix64K256K` reduced per-flow allocation from
about `6.10 MiB/op` and `768 allocs/op` to `4.79 MiB/op` and `758-759
allocs/op`, or about `21.5%` fewer allocated bytes. Median wall time was noisy
and effectively flat (`4.99 → 5.07 ms/op`). In the focused 256 KiB split
benchmark, the legacy copying API used `267,750 B/op`, while the view used
`5,603 B/op` (`-97.9%`).

The repeated heap diagnostic is
`reports/e2e-electron/2026-07-23T14-14-57-009Z` in the Yakit worktree. Against
`2026-07-23T02-54-58-894Z`, total window allocation changed from
`2,999,089,540` to `2,764,771,622 B` (`-7.8%`), split flat allocation from
`672,897,530` to `449,617,316 B` (`-33.2%`), and `CreateHTTPFlow` cumulative
allocation from `812,470,067` to `573,600,442 B` (`-29.4%`). Absolute post-GC
live heap changed from `283,032,394` to `273,422,821 B`, so the saving did not
turn into retained heap. `io.ReadAll` remained flat at roughly 776 MB and is
the next measured backend target.

The unprofiled three-run matrix is
`matrices/body-2026-07-23T14-22-29-748Z`; its machine comparison against
`body-2026-07-23T13-47-17-325Z` is
`comparisons/httpflow-body-view-2026-07-23`. All runs completed 120/120 flows
with exact bodies, stream ordering and cleanup. Throughput and end-to-end
latency improved in the medians, CPU/RSS were flat, while persistence and Long
Task samples regressed. The deterministic allocation result is accepted; the
short WSL run is not described as an across-the-board latency win.

### Bounded Content-Length response reading

The next isolated candidate targets response construction before HTTPFlow
creation. For Content-Length values up to 1 MiB, the reader performs one exact
allocation instead of geometrically growing `io.ReadAll`; larger declared
lengths retain progressive reading so an untrusted peer cannot force an
unbounded up-front allocation. The newly owned body slice backs a read-only
`rsp.Body` directly, while the mutable httpctx bare packet remains an
independent clone. Tests lock input/body/bare ownership and the historical
newline-padding behavior for short bodies.

Three 100-iteration samples produced these medians:

| Path | Before | After | Change |
| --- | ---: | ---: | ---: |
| Network Content-Length reader | `421.8 µs`, `1,992,237 B`, 84 allocs | `192.3 µs`, `806,019 B`, 60 allocs | bytes `-59.5%` |
| Existing-byte reparsing | `368.5 µs`, `1,721,486 B`, 78 allocs | `123.3 µs`, `535,308 B`, 55 allocs | bytes `-68.9%` |

The matching heap report is `2026-07-23T14-47-27-344Z`. Against
`2026-07-23T14-14-57-009Z`, total allocation changed from `2,764,771,622` to
`2,339,609,176 B` (`-15.4%`), `io.ReadAll` from `776,202,307` to
`358,823,143 B` (`-53.8%`), and response-parser cumulative allocation from
`739,646,392` to `328,783,745 B` (`-55.5%`). Absolute post-GC live heap fell
from `273,422,821` to `265,137,425 B`.

The unprofiled comparison is `comparisons/http-response-body-read-2026-07-23`,
using baseline matrix `body-2026-07-23T14-22-29-748Z` and candidate
`body-2026-07-23T14-53-28-951Z`. All three runs completed 120/120 flows with
exact bodies, stream ordering and cleanup. Throughput improved 5.7%, Yak peak
working set fell 3.0%, and CPU was effectively flat; request/response-to-React
medians regressed 2.1%/8.5% with overlapping ranges. The allocation change is
accepted without claiming an across-the-board UI latency improvement.

### DumpHTTPResponse body restoration

The following candidate is deliberately separate from response reading.
`DumpHTTPResponse` may consume a parser-owned immutable body through an
internal remaining-byte view, but still advances the original reader to EOF,
copies the bytes into the returned packet, and restores `rsp.Body` to the
previously unread content. Third-party bodies keep the original `io.ReadAll`
fallback. Tests cover partial reads, external bodies, chunked responses and
mutation independence between the dump and restored body.

The 256 KiB microbenchmark changed from a median `298.0 µs`, `1,465,820 B/op`
and 38 allocations to `64.4 µs`, `274,910 B/op` and 16 allocations (`-81.2%`
allocated bytes). The heap report `2026-07-23T15-10-24-871Z` changed total
allocation from `2,339,609,176` to `2,157,080,041 B` (`-7.8%`),
`DumpHTTPResponse` cumulative allocation from `216,439,900` to `69,928,653 B`
(`-67.7%`), and `io.ReadAll` from `358,823,143` to `198,994,567 B` (`-44.5%`).
Absolute post-GC live heap moved from `265,137,425` to `278,683,416 B`; this
single-run variation remains below an earlier 283 MB sample and is not claimed
as a live-heap improvement.

The unprofiled comparison is `comparisons/http-response-dump-body-2026-07-23`,
using baseline `body-2026-07-23T14-53-28-951Z` and candidate
`body-2026-07-23T15-15-43-104Z`. All three runs passed. Yak peak working set,
request/response-to-React and Long Task medians improved; throughput regressed
2.7% while network request P95 was flat. The allocation candidate is retained
with that throughput risk explicitly recorded.

### Rejected color-match body view

An isolated experiment used a body view only inside synchronous
`prepareColorMatch`, while public `SplitPacket` and `MatchPacket` kept copying.
The 256 KiB microbenchmark allocated 19.1% fewer bytes, and the heap profile
reduced the targeted color path by 8.3%, but total profile allocation increased
0.4%. The three-run candidate `body-2026-07-23T15-37-29-530Z` then regressed
throughput 5.3%, request/response-to-React 8.2%/7.3%, and Long Task total 21.6%
against `body-2026-07-23T15-15-43-104Z`. The code and dedicated benchmark were
removed; reports remain as rejection evidence. This prevents a local
allocation result from being promoted without end-to-end support.

### Read-only header helpers

Caller attribution showed that several compatibility split calls only inspect
headers or the response status while still cloning a large body. Seven
header/cookie/content-type/status helpers now use the explicit read-only body
view; public split and body getter APIs retain their independent-copy contract.
`BenchmarkHeaderOnlyHelpersLargeBody` compares the old copy path with the new
path on a 256 KiB body. Header extraction changed from roughly `52 us` and
`269,715 B/op` to `7.1 us` and `7,568 B/op` (97.2% fewer allocated bytes);
status extraction has the same order of improvement. Equivalence, existing
helper tests, targeted race, and the complete `common/utils/lowhttp` suite pass.

The configuration-matched canary heap report is
`2026-07-23T16-17-33-098Z`, compared with dumper baseline
`2026-07-23T15-10-24-871Z`. Total allocation changed from `2,157,080,041` to
`2,017,235,444 B` (-6.5%); `splitHTTPPacketEx` flat allocation changed from
`445,604,334` to `291,504,901 B` (-34.6%), and cumulative allocation changed
from `476,085,908` to `329,841,601 B` (-30.7%). Absolute post-GC live heap was
effectively flat (`278,683,416 → 277,031,840 B`). Total allocation is now 62.6%
below the first large-body profile.

The unprofiled three-repeat candidate is
`body-2026-07-23T16-11-29-679Z`; its comparison with
`body-2026-07-23T15-15-43-104Z` is stored under
`http-header-readonly-view-2026-07-23`. Correctness and cleanup pass in all
samples. Median throughput improved 1.3%, Yak peak working set fell 3.7%, and
Yak CPU and Long Task time were flat. Request/response-to-React regressed
5.0%/7.3%, and persistence-write P95 changed from 36 to 43 ms, so the result is
retained as an allocation improvement rather than an end-to-end latency claim.
An accidentally shadow-mode diagnostic group was rejected by configuration
identity checks and is not used in this A/B; the product default remains
`shadow`.

### FixHTTPPacketCRLF read-only body

`FixHTTPPacketCRLF` reads its input body for length, chunked and multipart
normalization, then writes an independent result. The internal implementation
keeps a `cloneBody` oracle so tests can compare the historical copy path with
the production read-only view path byte-for-byte. Large Content-Length,
no-fix-length, chunked plus pipeline rest, multipart, input immutability and
output independence are covered. Existing CRLF tests, targeted race and the
complete lowhttp suite pass.

On a 256 KiB body the old path used about `131 us / 538,745 B/op / 49 allocs`;
the view path used about `70 us / 276,576 B/op / 48 allocs` (48.7% fewer
allocated bytes). The matched canary heap report
`2026-07-23T16-27-08-323Z` compared with
`2026-07-23T16-17-33-098Z` reduced `FixHTTPPacketCRLF` cumulative allocation
from 95.13 to 52.61 MiB (-44.7%) and total allocation from `2,017,235,444` to
`1,962,799,243 B` (-2.7%). Absolute post-GC live heap changed from
`277,031,840` to `273,042,814 B`; total allocation is now 63.6% below the first
large-body profile.

The first historical comparison showed contradictory Renderer/network
regressions, so it was not accepted directly. A source-hash-isolated,
contemporaneous A/B toggled only the internal oracle: copy baseline
`body-2026-07-23T16-38-23-835Z` and view candidate
`body-2026-07-23T16-45-53-696Z`, with the report under
`http-fix-crlf-body-view-paired-2026-07-23`. All six samples passed correctness
and cleanup. The candidate improved throughput 11.9%, network request P95 22.3%,
Long Task total 21.1%, and Yak peak working set 2.4%; Yak CPU was flat.
Request/response-to-React regressed 4.3%/4.0%, and Query P95 remained noisy, so
those are recorded risks. The final source uses the view; the temporary copy
build exists only in the isolated reports.

### Rejected automatic-unzip body view

The automatic decoder checks every packet, so the historical path cloned a
large body even when it found no encoding and returned the original packet.
An internal copy/view oracle reduced the unencoded 256 KiB microbenchmark from
about `45 us / 268,105 B/op` to `4.2 us / 5,958 B/op` (97.8% fewer allocated
bytes). No-encoding, gzip, chunked, conservative/non-conservative failure,
input immutability, output independence, race and the full lowhttp suite passed.

The matched heap report `2026-07-24T01-40-41-012Z` reduced the targeted decoder
copy from 37.69 to 1.50 MiB, but total allocation changed from `1,962,799,243`
to `1,966,690,729 B` (+0.2%) and post-GC live heap moved in the wrong direction
in that sample. A contemporaneous copy/view 3+3 used
`body-2026-07-24T01-45-22-529Z` and
`body-2026-07-24T01-51-58-570Z`; its report is under
`http-auto-unzip-body-view-paired-2026-07-24`. The view candidate had flat
throughput (-1.3%) but regressed Long Task total 25.3%, request-to-React 11.9%,
and persistence-write P95 63.3%; Yak CPU/RSS improved only 1.9%/0.9%.
Consequently the implementation and dedicated oracle/benchmark were removed.
The artifacts remain as rejection evidence, and the valid source remains the
preceding `FixHTTPPacketCRLF` candidate.

## Fixed-rate, Linux ownership, and large-body flow construction

Harness version 7 adds a fixed-rate producer whose schedule is independent of
response completion. Counters are reset atomically immediately before the load
window, and the committed-shadow initial snapshot must be omitted. This keeps
pre-load/reconnect events out of backlog and delivery measurements and makes a
producer scheduling miss distinguishable from proxy saturation. Fixed-rate
runs remain sequential; at this stage the product default remained `shadow`.

On Linux, client-process attribution now prefers an exact source/destination
4-tuple INET_DIAG request and keeps a bounded pool of 16 netlink connections.
Unsupported kernels and unusual `net.Conn` values retain the source-only
fallback. `/proc/<pid>/fd` is read in batches of 32 names and inspected with
`readlinkat`, so a match does not allocate every directory entry and full path.
Median source-only/exact microbenchmarks were approximately `488/134 us`,
`11,048/5,752 B/op`, and `75/43 allocs`; found/missing FD scans reduced bytes
from about `8,301/14,836` to `1,369/2,586 B/op`. Exact-netlink and FD-scan 3+3
comparisons are stored under matrices
`body-2026-07-24T05-00-04-439Z` and
`body-2026-07-24T05-17-09-686Z` respectively.

Three further large-body candidates were retained only after compatibility,
race/package, allocation-profile, CPU-profile, and repeated Electron evidence:

| Candidate | Deterministic evidence | Matched 3+3 evidence |
| --- | --- | --- |
| No-enabled-rule `HookColor` fast path | 64 KiB: `36-39 us / 214,452 B` to `76 ns / 16 B`; heap `1.956 → 1.815 GB` | `body-2026-07-24T06-00-23-198Z` → `body-2026-07-24T06-08-31-743Z`; throughput `+4.4%`, RSS `-4.1%`, request P95 about `+5.2%` risk |
| Streaming legacy-compatible `HTTPFlow.CalcHash` | 64 KiB: `83.7 → 48.3 us`, `222,030 → 96 B/op`; heap `1.815 → 1.648 GB` | `body-2026-07-24T06-08-31-743Z` → `body-2026-07-24T06-26-54-407Z`; throughput `+17.5%`, request P95 `-22.3%` |
| Single-read `GetPostCommonParams` | 64 KiB: `1.37 → 0.451 ms`, `1,182,900 → 656,290 B/op`; target CPU `290 → 150 ms` | `body-2026-07-24T06-26-54-407Z` → `body-2026-07-24T06-44-07-916Z`; throughput P50 `-2.9%`, request P95 `+19.6%` risk |
| Exact request-body read plus owned-buffer handoff | 64 KiB: `97.1 → 53.5 us`, `500,369 → 215,485 B/op`; input/body/bare remain independent | Exact read: `body-2026-07-24T06-44-07-916Z` → `body-2026-07-24T07-17-03-886Z`; handoff: `body-2026-07-24T07-17-03-886Z` → `body-2026-07-24T07-33-13-446Z` |
| Owned response bare-packet handoff | 256 KiB: `166.3 → 131.6 us`, `806,037 → 535,669 B/op`; caller input/body/bare remain independent | `body-2026-07-24T07-33-13-446Z` → `body-2026-07-24T07-52-15-391Z`; throughput `+7.4%`, Yak CPU P95 `-0.2%`, Yak RSS `+1.5%` risk |

The hash implementation has a 256-case byte-for-byte differential oracle. The
POST implementation is compared with the historical JSON/XML/form control flow
and preserves repeated body reads and printable octet-stream behavior. The
third candidate is retained for its repeatable allocation/CPU/GC reduction, not
reported as an end-to-end latency win. Its heap and CPU diagnostic runs are
`2026-07-24T06-39-51-437Z` and `2026-07-24T06-50-28-648Z`.

For the exact request-body read, allocation profile
`2026-07-24T07-09-42-831Z` reduced total allocation 4.0%, `io.ReadAll` 58.9%,
and request-parser cumulative allocation 28.6%. Its 3+3 comparison improved
throughput P50 2.7% and request P95 13.9%, while Query/Renderer drain remained
noisy. The owned-buffer handoff profile `2026-07-24T07-25-55-429Z` reduced a
further 4.4% total allocation, 18.4% request-parser cumulative allocation, and
9.4% `bytes.growSlice`. Its CPU diagnostic changed parser cumulative time from
210 to 140 ms, but the formal 3+3 also recorded throughput P50 -8.1%,
request-to-React +5.8%, and Long Task total P50 +95.7%. The handoff is retained
because it removes one measured owned-buffer copy with matching micro/heap/CPU
evidence; the contradictory WSL UI metrics remain explicit risk rather than an
end-to-end improvement claim.

The current allocation leaders are `bytes.growSlice`, `io.ReadAll`,
`splitHTTPPacketEx`, `bytes.Clone`, and response-body storage. Any next change
must first specify independence and mutation contracts among the exact wire
packet, parser body, httpctx bare packet, and persisted fields. A lower local
allocation count is not sufficient evidence for sharing those buffers.

For response bare-packet handoff, the parser-created `rawPacket` is transferred
only after all writes have finished. The public clone setter remains the default
for shared or external input, while the explicit owned setter is limited to this
parser boundary. Heap report `2026-07-24T07-45-15-744Z` reduced `bytes.Clone`
from 162.1 to 122.7 MB and response-parser cumulative allocation from 325.8 to
291.5 MB; total allocation moved `1.423 → 1.437 GB`, so only the targeted
reduction is attributed to the change. CPU diagnostics also moved in the wrong
direction (`4.16 → 4.59 s` total samples and `210 → 320 ms` parser cumulative),
and are retained as contradictory evidence.

The formal 3+3 comparison is
`body-2026-07-24T07-52-15-391Z/comparison-vs-before-response-bare-handoff`.
Configuration and correctness coverage match the baseline. Throughput improved
7.4%, request-to-React was flat at -0.7%, Renderer drain improved 14.5%, and Yak
CPU P95 was flat at -0.2%; Electron CPU P95 regressed 19.2% and Yak RSS 1.5%.
Backend Query P95 regressed 20.4%, but its baseline/candidate coefficients of
variation were 82%/98%. The change is retained for deterministic allocation
evidence without claiming a general CPU or UI-latency win.

## Fixed-rate SQLite And Direct Live-list Experiments

The fixed-rate scene schedules 1,000 requests at 200 requests/second with
concurrency 16, an empty request body, and a 4 KiB response body. It separates
producer schedule lag, proxy throughput, persistence backlog, visible-ID
backlog, and post-load drain. Runs remain strictly sequential and compare the
same Yak and Renderer source fingerprint.

Increasing the SQLite project writer pool from one connection to two was
rejected. The writer1 baseline is matrix
`body-2026-07-24T08-32-22-099Z`; writer2 is
`body-2026-07-24T08-38-12-845Z`, with
`comparison-vs-writer1.{json,md}` in the candidate directory. Query round-trip
changed from 117.8 to 213.2 ms, persistence wait from 11 to 26 ms,
request-to-React from 954 to 1,295 ms, schedule lag from 22.38 to 554.49 ms,
and Long Task time from 1,064 to 1,836 ms. A separate read-only query-pool
candidate was also rejected. The supported default remains writer1/read0.

`SubscribeHTTPFlows` now carries a body-free list summary plus optional scalar
timestamps for request hijack, response mirror, flow construction, persistence
enqueue, and persistence start. No request or response packet is added to the
stream. In a compatible top-of-list canary, the Renderer maps these summaries
directly into the virtual list; a Gap, stream failure, project/filter/scroll
change, incompatible cursor, or active recovery query cancels pending direct
rows and returns to `QueryHTTPFlows`.

The first direct candidate flushed every 100 ms. Its shadow/canary matrices are
`body-2026-07-24T09-26-19-594Z` and
`body-2026-07-24T09-32-31-361Z`. Request-to-React improved from 991 to 213 ms
and maximum visible backlog from 165 to 44, but Long Task time changed from 525
to 3,278 ms (+524.4%) and Electron CPU P50 increased about 171%. That scheduler
was removed.

The retained canary makes the first row after idle immediately visible, then
uses a 250 ms sparse interval and a 500 ms sustained interval once at least
eight rows are pending. A batch is capped at 256 rows and the pending queue at
2,048 rows. Because the application still uses a legacy React root, timer-driven
state updates are wrapped in `unstable_batchedUpdates`; this removed the repeated
synchronous render fan-out seen in the rejected candidate.

The final same-fingerprint 3+3 comparison uses shadow baseline
`body-2026-07-24T09-58-01-917Z` and direct canary
`body-2026-07-24T10-03-34-990Z`; the candidate contains
`comparison-vs-shadow-direct-batched.{json,md}`. All six samples completed and
persisted 1,000/1,000 unique flows with no body, Gap, sequence, duplicate,
out-of-order, or cleanup failure. Every canary inserted 1,000 direct rows with
zero fallback rows and zero live-list queries in 11--12 React batches.

| Metric | Shadow/query | Direct canary | Change |
| --- | ---: | ---: | ---: |
| Request-to-React P95 | `990 ms` | `490 ms` | `-50.5%` |
| Persist-to-React P95 | `987 ms` | `485 ms` | `-50.9%` |
| First visible | `137 ms` | `44 ms` | `-67.9%` |
| Maximum visible-ID backlog | `193` | `39` | `-79.8%` |
| Renderer drain | `486 ms` | `368 ms` | `-24.3%` |
| Long Task total | `517 ms` | `0 ms` | candidate range `0--169 ms` |
| Electron CPU P95 | `8.67%` | `6.81%` | `-21.5%` |
| Electron CPU P50 | `2.49%` | `3.09%` | `+24.2%` risk |
| Yak CPU P50 | `116.6%` | `100.8%` | `-13.5%` |
| Throughput | `200.09 req/s` | `199.32 req/s` | `-0.4%` |

The formal direct group originally left 1,000 legacy `FlowCommitted` shadow
entries pending because that observer only understood Query-returned rows. The
follow-up does not clear by a coarse maximum ID: it reconciles the legacy event
and a committed direct-list event by exact database identity, project
generation, and ID in either arrival order. Reports now expose direct matches,
direct rows without a shadow event, and commit/shadow-to-direct latency.

The single real-engine validation is matrix
`body-2026-07-24T10-37-01-191Z`, report
`2026-07-24T10-37-01-314Z`. It completed and persisted 1,000/1,000 unique flows,
matched all 1,000 direct rows to shadow events, ended with zero pending and zero
direct rows without an event, and reduced sampled peak shadow pending from
1,000 to 48. It used 11 batches with zero Query, fallback, Gap, or Long Task.
This one run validates reconciliation correctness, not a new performance claim.

At the time of this experiment the default remained `shadow`; large-body,
sustained/burst, slow-consumer, reconnect/replay, project/filter/scroll,
100-site, and real Chromium/nuclei matrices remained release gates. The later
Phase 36 fixed-rate attribution and recovery validation supersede this default
decision without changing the historical result.

## Slow-Consumer Recovery Gate

The frontend worktree now has a fixed-rate, real-Electron recovery matrix:

```bash
yarn test:e2e:electron:mitm-slow-consumer
```

The producer continues while WDIO scrolls the MITM table away from the top at
25% progress and restores it at 75%. This is a correctness gate first: every
legacy `FlowCommitted` event must reconcile by database identity, project
generation, and ID with exactly one direct or Query-visible row. Final maximum
ID equality is insufficient because it can hide a hole in the middle.

That exact check caught a real ownership race in report
`2026-07-24T10-55-49-429Z`: the database and dedicated stream completed 800/800,
but only 291 direct plus 485 Query rows reconciled, leaving 24 pending. Newer
direct rows had advanced the table cursor before the Query fallback recovered
the older interval. The retained recovery gate closes direct insertion on the
first fallback. It reopens only after an exhausted Query covers the fallback
high water and current stream cursor, the result is React-visible, and no newer
stream event invalidated the candidate.

The passing 0/4 KiB run is report `2026-07-24T11-22-46-115Z`: 800 requests at
120.07 req/s, `232 direct + 568 Query`, zero pending/unmatched rows, 1 recovery
entry/completion, 120 ms database catch-up, 487 ms Renderer drain, and 1,856 ms
from returning to the top to Renderer drain. The passing 64/256 KiB run is
`2026-07-24T11-27-17-125Z`: 240 requests at 29.97 req/s, `76 direct + 164 Query`,
zero pending/unmatched rows, 2 recovery entries/completions, 165 ms database
catch-up, 314 ms Renderer drain, and 2,095 ms from return to drain. Both have
zero Gap, sequence, duplicate, out-of-order, unavailable, body, or cleanup
errors. Request-to-React measurements include the intentional off-top interval
and are therefore not top-follow latency comparisons.

This closes the sustained-producer/scroll-away recovery item only. At that
point the product default remained `shadow`; reconnect/replay, project/filter
transitions, long-duration and 100-site bursts, and Chromium/nuclei production
remained gates. Phase 36 later repeated the recovery cases on the current
backend before promoting the compatible MITM top-view path.

## MITMV2 Plain-Response Cache Ownership

The large-body allocation profile found a second full-response clone in
`handleHijackResponse`. For an unmodified response, `getPlainResponseBytes`
already decoded and cached an independent copy. Calling
`SetPlainResponseBytes` again immediately afterward provided no additional
ownership boundary. The retained helper now refreshes that cache only when the
response was already marked modified. The modified path still clones, so later
changes to hijacked bytes cannot mutate the cached plain response.

The focused checks are resource-bounded and can be run independently:

```bash
GOMAXPROCS=2 GOMEMLIMIT=2GiB go test -p=1 ./common/yakgrpc \
  -run '^TestCacheModifiedPlainResponseBytes$' -count=1
GOMAXPROCS=2 GOMEMLIMIT=2GiB go test -race -p=1 ./common/yakgrpc \
  -run '^TestCacheModifiedPlainResponseBytes$' -count=1
GOMAXPROCS=2 GOMEMLIMIT=2GiB go test -p=1 ./common/yakgrpc \
  -run '^$' -bench '^BenchmarkCacheModifiedPlainResponseBytes256K$' \
  -benchmem -count=5
```

At 256 KiB, the legacy unconditional setter is approximately
`38-40 us/op, 262,233 B/op, 4 allocs`; the unmodified cached path is
approximately `19 ns/op, 0 B/op, 0 allocs`. The modified branch remains
`38-43 us/op, 262,233 B/op, 4 allocs`, which is the required ownership cost.

The single heap candidate is report `2026-07-24T11-42-26-163Z`, compared with
`2026-07-24T07-45-15-744Z`. Total allocation changed from `1,436,882,660` to
`1,395,650,410 B (-2.9%)`; `bytes.Clone` changed from `122,690,190` to
`90,925,766 B (-25.9%, -31.8 MB)`. Post-GC live heap was effectively flat at
`272.1 -> 274.3 MB`. A separate single CPU profile moved in the wrong direction
(`4.59 -> 5.24 s` total samples), so it is explicitly not CPU-improvement
evidence.

The adjacent product A/B used the same 120-request, concurrency-8, 64/256 KiB,
shadow/shadow configuration. The legacy matrix is
`body-2026-07-24T11-48-49-833Z`; the candidate is
`body-2026-07-24T11-57-17-023Z`, with
`comparison-vs-unconditional-plain-response-clone.{json,md}` in the candidate
directory. All six runs completed 120/120 with body, database, stream, and
cleanup checks passing; the comparator reports no case-config or diagnostic
differences. Candidate medians were throughput `+21.8%`, request latency p95
`-25.0%`, first visible `-32.0%`, Renderer drain `-53.5%`, Long Task `-59.1%`,
and Yak CPU p95 `-0.2%`. Yak RSS was `+2.1%`; request/response/persist-to-React
were `+6.5%/+17.6%/+15.3%`, and persist-write p95 was `+52.0%`. These mixed WSL
timings are retained as risks rather than a broad UI claim. The change is kept
for the deterministic allocation removal and unchanged modified-response
ownership contract.

## HTTPFlow Title Extraction From Bytes

Line-level allocation attribution showed approximately 35.2 MB at
`ExtractHTTPFlowHTMLTitle(string(rspRaw))` in the 120-flow, 256 KiB response
scene. The persisted title is at most 128 runes, but the conversion materialized
the entire response before the existing bounded regular-expression scan.

The retained path adds byte-slice entry points while preserving the existing
case-insensitive expression, invalid-UTF-8 escaping, 128-rune result limit, and
512 KiB scan limit. Only new HTTPFlow construction uses the byte path. The
string API, `SetResponse`, legacy-row fallback, and database representation are
unchanged. Differential coverage includes missing and mixed-case titles,
historically unsupported title attributes, long titles, invalid UTF-8, title
positions before/after/across the scan boundary, and input immutability.

The focused benchmark command is:

```bash
GOMAXPROCS=2 GOMEMLIMIT=2GiB go test -p=1 ./common/schema \
  -run '^TestExtractHTTPFlowHTMLTitleBytesMatchesString$' \
  -bench '^BenchmarkExtractHTTPFlowHTMLTitle256K$' -benchmem -count=5
```

For a 256 KiB response, the full-string path measured approximately
`44-50 us/op, 262,309-262,316 B/op, 2 allocs`; the byte path measured
`1.68-1.74 us/op, 64 B/op, 1 alloc`. Focused race, full `common/utils` and
`common/schema`, and Yakit persisted/legacy-title tests pass.

Heap report `2026-07-24T12-15-05-134Z` is compared with
`2026-07-24T11-42-26-163Z`. The target line disappears from the profile;
`CreateHTTPFlow` flat allocation changes from `70.4` to `42.5 MB (-39.7%)`,
cumulative allocation from `397.0` to `358.6 MB (-9.7%)`, and total allocation
from `1,395,650,410` to `1,374,974,896 B (-1.5%)`. Post-GC live heap changes
from `274.3` to `282.1 MB (+2.9%)`, so no live-heap improvement is claimed.

The strict product baseline is `body-2026-07-24T11-57-17-023Z`; the byte-path
candidate is `body-2026-07-24T12-19-42-548Z`, with
`comparison-vs-string-html-title.{json,md}` in the candidate directory. All six
runs complete 120/120 and pass body, database, stream, and cleanup checks with
no configuration or diagnostic differences. Candidate medians are Yak RSS
`-1.1%`, Yak CPU p95 `+0.8%`, throughput `-2.8%`, request/response-to-React
`-1.6%/-6.8%`, Renderer drain `-19.2%`, and persistence-write p95 `-47.4%`.
Long Task time changes from `113` to `161 ms (+42.5%)`, with candidate values
from `112` to `215 ms`; this remains an explicit fixed-hardware retest risk.
The byte path is retained for deterministic allocation evidence without
claiming a general Renderer or UI improvement.

## MITMV2 Plain-Request Cache Body View

After decoding a bare request, `getPlainRequestBytes` only needs the body length
to decide whether the packet is small enough for the plain-request context
cache. The legacy split API cloned the complete body for that check, after
which `SetPlainRequestBytes` correctly cloned the complete packet again for
independent context ownership. The retained helper uses the explicit read-only
view API for the length check. The context setter, wire packet, parser body and
bare/plain packet ownership contracts are unchanged. MITMV2 and legacy MITM use
the same helper at their equivalent cache entry points.

The focused checks are:

```bash
GOMAXPROCS=2 GOMEMLIMIT=2GiB go test -p=1 ./common/yakgrpc \
  -run '^TestCachePlainRequestBytesIfStorable$' -count=1
GOMAXPROCS=2 GOMEMLIMIT=2GiB go test -race -p=1 ./common/yakgrpc \
  -run '^TestCachePlainRequestBytesIfStorable$' -count=1
GOMAXPROCS=2 GOMEMLIMIT=2GiB go test -p=1 ./common/yakgrpc \
  -run '^$' -bench '^BenchmarkCachePlainRequestBytesIfStorable$' \
  -benchmem -count=5
```

For a cacheable 128 KiB body, the five-sample medians changed from
`45.351 us/op, 270,903 B/op, 19 allocs` to
`24.427 us/op, 139,828 B/op, 18 allocs` (`-46.1%` time and `-48.4%` bytes).
For a 256 KiB body above the cache threshold, they changed from
`42.648 us/op, 262,619 B/op, 15 allocs` to
`0.717 us/op, 472 B/op, 14 allocs` (`-98.3%` time and `-99.8%` bytes).
Boundary coverage verifies that exactly 200 KiB is cached, 200 KiB plus one
byte is not, and mutating the source packet cannot mutate the cached packet.
Focused race and MITMV2 gzip/manual-hijack regressions pass. The legacy 300 KiB
wire-forward test reached the upstream but hit its fixed five-second database
query deadline once in a combined cold run; its isolated repeat passed in
3.6 seconds, so that remains an automation-flake observation rather than
performance evidence.

Heap report `2026-07-24T13-25-18-259Z` is compared with
`2026-07-24T12-15-05-134Z`. The `MITMV2.func6` request path changes from
`27.18` to `15.45 MB cumulative (-43.2%)`; its body-split length check changes
from `9.57` to `0.50 MB (-94.8%)`. The remaining `6.97 MB` is the required
context-owned packet clone. Total allocation is effectively flat at
`1,374,974,896 -> 1,377,352,386 B (+0.17%)`, while absolute post-GC live heap
changes from `282.1` to `259.6 MB (-8.0%)`. Only the caller-attributed removal
is claimed because unrelated allocation samples moved in both directions.

The strict shadow/shadow product comparison uses baseline
`body-2026-07-24T12-19-42-548Z` and candidate
`body-2026-07-24T13-30-36-835Z`, with
`comparison-vs-cloned-request-body.{json,md}` in the candidate directory. All
six runs complete 120/120 and pass body, database, stream and cleanup checks;
case configuration and diagnostics are identical. Candidate medians are
throughput `+2.1%`, request latency p95 `-1.7%`, request/response-to-React
`-6.2%/-5.2%`, and Yak CPU p95 effectively flat. Risks are Yak RSS `+3.4%`,
first visible `+24.0%`, Electron CPU p95 `+21.1%`, Long Task time
`161 -> 226 ms (+40.4%)`, and persistence-write p95 `+40.0%`. The change is
retained for deterministic allocation removal and unchanged ownership, without
claiming a general UI improvement.

## Response-Fix Packet-Only And Body-Length Views

The latest heap profile showed two ownership mismatches. The max-content-length
branch only measured a response body but used the compatibility API that clones
it. `HTTPWithoutRetry` and `resolveHTTPFlowStoredResponse` also discarded the
body returned by `FixHTTPResponse`, even though that API must preserve its
independent-body contract. The retained change leaves that public contract
untouched and adds `FixHTTPResponsePacket`, which only returns an independently
rebuilt packet. Its normal body input is a read-only view. Malformed chunked
bodies are defensively cloned because the legacy chunk decoder may rewrite its
error preview. Actual truncation still rebuilds a new packet. Differential
coverage includes plain, gzip, valid and malformed chunked, 100-Continue and
error inputs, input immutability, output ownership and legacy body ownership.
The complete lowhttp package, focused Yakit tests and both focused race suites
pass.

Across five 256 KiB samples, response fixing changes from a median
`723.359 us/op, 556,261 B/op, 86 allocs` to
`680.101 us/op, 294,125 B/op, 85 allocs` (`-6.0%` time and `-47.1%` bytes).
The exact body-length operation changes from
`45.214 us/op, 262,777 B/op, 17 allocs` to
`0.894 us/op, 618 B/op, 16 allocs` (`-98.0%` time and `-99.8%` bytes).

Heap report `2026-07-24T14-09-50-940Z` is compared with
`2026-07-24T13-25-18-259Z`. Allocation attributed to the targeted packet split
changes from `95.67 MiB` to about `1 MiB`; the `33.04 MiB`
`GetHTTPPacketBody` caller disappears, and the complete response-fix focus
changes from `198.07` to `121.40 MiB cumulative (-38.7%)`. Total window
allocation changes from `1,377,352,386` to `1,293,933,355 B (-6.1%)`.
Post-GC live heap changes from `259.6` to `270.4 MB (+4.1%)`, so no live-heap
improvement is claimed.

The strict product baseline is `body-2026-07-24T13-30-36-835Z`; the candidate
is `body-2026-07-24T14-15-45-537Z`, with
`comparison-vs-response-body-clones.{json,md}` in the candidate directory. All
six runs complete 120/120 and pass body, database, stream and cleanup checks;
configuration and diagnostics match, and the actual Renderer input fingerprint
is identical. Candidate medians are Yak CPU p95 `-0.7%`, Yak RSS `-4.4%`, first
visible `-22.9%` and Long Task time `-49.1%`. Risks are throughput `-8.5%`,
request latency p95 `+9.3%`, request/response-to-React `+17.9%/+6.5%`, Query p95
`3.153 -> 63.438 ms`, and Electron CPU p95 `+4.3%`. The change is retained for
the deterministic allocation and ownership evidence without claiming a UI
latency improvement. The UI and SQLite regressions require fixed-hardware
retesting.

## Response-Fix Provenance Handoff

A normal HTTP/1 response now carries `ResponsePacketFixed` only after
`FixHTTPResponsePacket` succeeds. No-fix, no-body-buffer, multi-response,
HTTP/2/3, and failed-fix paths remain unproven. After a regression test proves
that the response parser does not retain its input, minimartian transfers the
owned fixed packet into httpctx and the unmodified HTTPFlow builder takes it
once instead of fixing the same response again. Marking a response modified
releases the provenance packet and preserves the existing fix path;
`NoFixContentLength` always takes precedence. This changes no protocol, database
field, or compatibility API. Focused and full tests for the four affected
packages, including race runs, pass.

Across five 256 KiB benchmark samples, fixing again has medians of
`2.391 ms/op, 1,247,989 B/op, 319 allocs`; reusing the proven packet has medians
of `1.638 ms/op, 954,108 B/op, 236 allocs`, reductions of `31.5%`, `23.5%`, and
`26.0%`. Heap report `2026-07-24T14-49-28-440Z`, compared with
`2026-07-24T14-09-50-940Z`, no longer contains the
`resolveHTTPFlowStoredResponse` fix caller. Response-fix cumulative allocation
changes from `127,299,117` to `65,946,384 B (-48.2%)`, `CreateHTTPFlow`
cumulative allocation falls `17.5%`, `transform.Bytes` flat allocation falls
`55.1%`, and total window allocation falls `4.4%`. Post-GC live heap falls
`2.1%`, but that is one sample and is not treated as a stable live-memory claim.
CPU report `2026-07-24T15-01-46-777Z`, compared with the latest same-config
report `2026-07-24T11-45-52-112Z`, moves the target caller from
`290 ms/5.53%` below the top-list threshold; the fix chain changes from
`510` to `170 ms` and `CreateHTTPFlow` from `980` to `720 ms`. That baseline
predates both the packet-only and provenance changes, so the CPU delta is
cumulative Phase 25 plus Phase 26 evidence rather than provenance-only evidence.

The strict product comparison uses the immediately preceding candidate
`body-2026-07-24T14-15-45-537Z` as baseline and
`body-2026-07-24T14-54-42-668Z` as candidate, with
`comparison-vs-response-fix-provenance.{json,md}` in the candidate directory.
All six runs complete 120/120 and pass body, database, stream, CPU-recovery, and
cleanup gates. Configuration, diagnostics, and actual Renderer input
fingerprints match. Candidate medians are Yak CPU p95/RSS `+0.3%/+0.7%`,
throughput `+6.2%`, request-to-React `-13.4%`, Query RTT `-11.8%`, and
database/Renderer drain `-51.1%/-42.3%`. Risks are request latency p95 `+27.5%`,
first visible `+18.7%`, Long Task time `115 -> 284 ms (+147%)`, and persistence
queue-wait p95 `+32.7%`. Candidate throughput, request latency, first visible,
and Long Task coefficients of variation are `10.2%`, `16.3%`, `23.1%`, and
`47.5%`. The owned handoff is retained on deterministic benchmark, heap, CPU,
and neutral Yak CPU/RSS evidence; the high-variance UI results are recorded for
fixed-hardware retesting and are not presented as a general UI improvement.

The next allocation candidate must first split the remaining `bytes.growSlice`,
response-parser, `io.ReadAll`, and `DumpHTTPResponse` callers and prove the
lifetimes of raw, body, and dumped packets. Inputs without demonstrable unique
ownership keep their copies. The `strconv.quoteWith` persistence representation
is not changed merely because it remains visible in the profile.

## Writer-Only Response Serialization

Minimartian used `DumpHTTPResponse` for CONNECT, proxy-auth rejection, and
normal HTTP responses even though all three callers immediately discarded the
returned packet. The old path therefore serialized to the client writer while
also retaining a complete cache buffer. `WriteHTTPResponse` now shares the
existing protocol, header, body-read, and body-restoration implementation but
does not create that cache. The compatibility `DumpHTTPResponse` API and its
ownership contract are unchanged. Only those three discard-only callers use
the new API. Byte-for-byte wire equivalence, restored response bodies, nil
writer handling, focused/full package tests, and race tests pass.

Across five 256 KiB samples, dump-and-discard has medians of `63.358 us/op`,
`274,939 B/op`, and `16 allocs`; writer-only has `2.276 us/op`, `4,272 B/op`,
and `9 allocs`, reductions of `96.4%`, `98.4%`, and `43.8%`. Heap report
`2026-07-24T15-24-38-358Z`, compared with `2026-07-24T14-49-28-440Z`, changes
total window allocation from `1,237,457,392` to `1,170,515,389 B (-5.4%)`.
The old `DumpHTTPResponse` cumulative `69,909,517 B` is replaced by
`WriteHTTPResponse` cumulative `31,419,800 B`, about `38.5 MB (-55.1%)` less,
and the dumper cache's `bytes.Buffer.Grow` caller disappears. Roughly 30 MiB
remains because those bytes must actually be written to the client. Post-GC
live heap and positive live delta regress `3.5%` and `8.9%` in this one sample,
so no stable live-memory improvement is claimed. CPU report
`2026-07-24T15-33-54-677Z`, compared with `2026-07-24T15-01-46-777Z`, moves the
old dumper from `330 ms/7.93%` below the candidate's `11.5 ms` top threshold.
Because this CPU case has only a 4 KiB response and includes about 500 ms of
random RSA generation in the candidate, only target disappearance and no CPU
regression are attributed to this change.

The formal strict comparison is `body-2026-07-24T14-54-42-668Z` to
`body-2026-07-24T15-48-39-764Z`; the candidate contains
`comparison-vs-discarded-response-packet.{json,md}`. All six runs complete
120/120 with exact 64 KiB request and 256 KiB response bodies and pass database,
stream, CPU-recovery, and cleanup gates. The comparator status is `passed`, with
no configuration or diagnostic differences and identical actual Renderer input
fingerprints. The three candidate backend fingerprints are all
`a08ab85d69e9e44a583a`, with build-cache states `false/true/true`. Candidate
medians are Yak CPU p95 `-0.1%`, peak RSS `-2.5%`, throughput `+3.4%`, request
p95 `-23.1%`, first visible `-8.7%`, Renderer drain `-30.9%`, and Long Task
`-42.3%`. Risks are Query p95 `5.179 -> 11.226 ms (+116.8%)`, database change
detection p95 `32 -> 119 ms (+271.9%)`, Electron drain CPU p95 `+18.1%`, and
producer-stop visible backlog `28 -> 40`. Query CV is `52.8%/79.7%` and change
detection CV is `87.3%/71.8%`; final drain and all correctness gates still pass.
The narrow writer-only change is retained on deterministic allocation, heap,
CPU-caller, and neutral Yak CPU/RSS evidence, without claiming the WSL product
improvements as fixed-hardware results.

An earlier candidate matrix was invalidated before comparison because its build
identity varied for the same source. The fixture waited for ChildProcess
`exit` rather than `close` and decoded output chunk-by-chunk; a roughly 907 KiB
git diff containing Chinese text could split UTF-8 code points differently and
produce replacement characters in the hash input. It now waits for `close`,
concatenates Buffer chunks before decoding, and has a large tracked-Unicode
regression test. Nine fixture tests and all 48 preflight tests pass, and the
real dirty backend identity is stable across five reads. The next candidate is
the response parser raw-packet result discarded by its caller (about 29.44 MB
in the current profile). Its body and httpctx ownership must be proven before
removing storage; mutable buffers will not be aliased merely to reduce copying.

## Requestless Response Raw-Packet Capture

`ParseBytesToHTTPResponse` calls `ReadHTTPResponseFromBytes(..., nil)` after
lowhttp already owns the complete response packet. The parser nevertheless
rebuilt headers and body into a `bytes.Buffer` that no request/httpctx consumer
could receive, while `rsp.Body` separately owned the parsed body. The parser now
creates that raw-packet buffer only when `req != nil`. Requestless parsing still
copies the body into its owned reader, so mutating the input cannot change the
response. Request-backed parsing still transfers an independent bare packet to
httpctx. A new bytes-plus-request test proves caller input, bare packet, and body
ownership independently; the existing no-input-retention and short
Content-Length tests, focused race, and full `common/utils`, lowhttp, and
minimartian suites pass. Public APIs, wire behavior, and database fields do not
change.

Across five before/after 256 KiB parser samples, medians change from
`109.345` to `53.000 us/op (-51.5%)`, `535,315` to `264,766 B/op (-50.5%)`, or
`-270,549 B/op`, and `55` to `52 allocs`. Heap report
`2026-07-24T16-10-40-261Z`, compared with `2026-07-24T15-24-38-358Z`, changes
total window allocation from `1,170,515,389` to `1,050,260,546 B (-10.3%)`.
`ParseBytesToHTTPResponse` cumulative allocation falls from `63,909,353` to
`26,649,496 B (-58.3%)`, `ReadHTTPResponseFromBytes` falls from `125,942,769`
to `50,492,117 B (-59.9%)`, and `bytes.growSlice` falls from `381,323,072` to
`312,605,303 B (-18.0%)`. The requestless raw-packet body-write line disappears;
about 47.65 MiB remains for the independently owned response bodies. Post-GC
live heap regresses `2.7%`, while positive live delta improves `8.2%`; both are
single samples, so no stable live-memory claim is made.

Standard CPU report `2026-07-24T16-15-24-318Z`, compared with
`2026-07-24T15-33-54-677Z`, changes total samples from `2.30` to `2.31 s` and
throughput from `101.39` to `102.40 req/s`, showing no total CPU regression.
That case has only a 4 KiB response, however, so the target caller is below the
sampling threshold and its single-sample request latency and Yak CPU/RSS move
in the adverse direction. It is not evidence of target CPU improvement.

The formal strict comparison is `body-2026-07-24T15-48-39-764Z` to
`body-2026-07-24T16-17-57-516Z`, with
`comparison-vs-requestless-response-raw-packet.{json,md}` in the candidate.
All six runs complete 120/120 with exact 64 KiB request and 256 KiB response
bodies and pass database, stream, CPU-recovery, and cleanup gates. Comparator
status is `passed`; configuration and diagnostics match, and actual Renderer
input fingerprint is `fa377df501505467d37539e9407e550f7931d1ebf5607b0d6cc4ce176fe3328a`
throughout. Candidate backend fingerprint is `542887dfcc4c0913032f` in all three
runs, with cache states `false/true/true`. Candidate medians are Yak CPU p95/RSS
`+0.2%/+0.8%`, throughput `+5.3%`, request p95 `-12.8%`, first visible
`-20.1%`, Query p95 `-28.4%`, and Query RTT `-56.1%`. Risks are
request-to-React `+4.9%`, Renderer drain `+9.5%`, persistence queue-wait p95
`52 -> 81 ms (+55.8%)`, maximum persistence backlog `5 -> 8 (+60%)`, and
Electron CPU p50 `+28.2%`; Electron CPU p95 improves `0.2%`, candidate Renderer
drain and queue-wait CV are `29.8%` and `24.0%`, and final drain still passes.
The narrow allocation removal is retained on ownership, benchmark, heap, and
neutral product CPU/RSS evidence without claiming general UI improvement. The
next profile split covers `readHTTPResponseBodyWithLimit`, `bytes.Clone`, and
remaining `bytes.growSlice` callers. Mutable bodies will not alias fixed packets,
and persistence representation will not change merely because
`strconv.quoteWith` remains visible.

## HTTPFlow Quote Input Copy

`CreateHTTPFlow` must preserve the existing `strconv.Quote` database encoding,
but `strconv.Quote(string(reqRaw))` and its response equivalent first allocated
full temporary strings solely to provide synchronous read-only input. The new
`quoteHTTPPacket` uses the existing unsafe bytes-to-string view, immediately
quotes it, and calls `runtime.KeepAlive` on the packet. The view is never stored;
the returned quoted string remains independently allocated. Differential tests
cover nil and empty packets, ASCII, Chinese and control characters, invalid
UTF-8, every byte from 0 through 255, and mutation of the source after return.
Output exactly matches the copied-input implementation and remains unchanged
after mutation. Focused tests and race, plus the full `common/yakgrpc/yakit`
suite, pass. Public APIs and the persisted representation do not change.

Across five samples of a 64 KiB request plus 256 KiB response flow, median
`CreateHTTPFlow` cost changes from `3.187` to `3.151 ms/op (-1.1%)`, from
`2,734,899` to `2,390,940 B/op (-12.6%, -343,959 B/op)`, and from `432` to
`430 allocs`. The isolated 256 KiB quote benchmark changes from `1.490` to
`1.434 ms/op (-3.8%)`, `942,083` to `671,746 B/op (-28.7%, -270,337 B/op)`,
and `3` to `2 allocs`.

Heap report `2026-07-24T16-33-49-322Z`, compared with
`2026-07-24T16-10-40-261Z`, no longer attributes allocations to the two
temporary input-string source lines. `CreateHTTPFlow` cumulative allocation
falls from `295,995,370` to `258,208,379 B (-12.8%, -37.8 MB)`. Total window
allocation regresses `1.7%`, post-GC live heap regresses `3.2%`, and positive
live delta regresses `2.0%`; retained quote-output allocation also varies from
`91.6` to `109.2 MB`. These single-sample values do not support an overall heap
claim, so only disappearance of the target input copies is attributed. CPU
report `2026-07-24T16-38-03-642Z`, compared with
`2026-07-24T16-15-24-318Z`, changes `CreateHTTPFlow` cumulative CPU from `550`
to `330 ms`, the quote chain from `150` to `80 ms`, and total samples from
`2.31` to `1.85 s`. It is diagnostic corroboration, not a standalone release
claim.

The formal strict comparison is `body-2026-07-24T16-17-57-516Z` to
`body-2026-07-24T16-46-15-707Z`, with
`comparison-vs-httpflow-quote-input-copy.{json,md}` in the candidate. All three
candidate repetitions complete 120/120 with exact bodies and pass database,
shadow-stream, detail hydration, CPU-recovery, and cleanup gates. Comparator
status is `passed`; configuration and diagnostics match. Actual Renderer input
fingerprint remains `fa377df501505467d37539e9407e550f7931d1ebf5607b0d6cc4ce176fe3328a`,
and candidate backend fingerprint is `9b3aae4c5c388db88e54`. Candidate medians
change Yak CPU p95 by `+0.4%`, Yak RSS by `-3.0%`, Electron CPU p95 by `-3.5%`,
throughput by `+3.6%`, request-to-React by `+2.2%`, and first-visible by `+3.3%`.
Adverse values are request p95 `+11.7%`, Renderer drain `+44.4%`, Query RTT
`+156.5%`, and backend Query p95 `+571.1%`; candidate backend Query CV is still
`66.5%`, final drain passes, and persistence queue p95/backlog improve
`35.8%/37.5%`. The change is retained for deterministic allocation removal and
neutral product CPU/RSS evidence, without a general UI-latency claim.

The first formal attempt, `body-2026-07-24T16-40-23-283Z`, remains a recorded
failure. The container reached `scrollTop=1120` and the virtual list reached
`marginTop=1008`, but the driver sampled the old first-row identity after only
two animation frames plus 20 ms. The table's scroll path is throttled at 200 ms,
so a busy Renderer can update virtual geometry before React row content. The
gate now performs a bounded 1.5-second state wait, re-resolves current DOM nodes,
and additionally requires the original first row after restoring to the top;
timeouts still fail. All 48 preflight tests pass. In the successful formal runs,
row movement settled in `111.9..118.7 ms` and restoration in `0.7..0.9 ms`.
The next profile split targets `bytes.growSlice`, `io.ReadAll`, `bytes.Clone`,
and response-body callers without changing packet ownership or persistence
format.

## Conn-Pool Lazy Recovery Packet

The HTTP/1 connection-pool read loop previously copied every successfully
parsed response from its capture buffer into a second `bytes.Buffer`. Normal
responses never consumed that recovery copy; only parser-error or
`Connection: close` handling can need it when `ReadUntilStable` discovers
additional bytes. The retained change returns the original captured packet on
success and allocates one exact combined packet only when recovery actually has
non-empty trailing bytes. It preserves timeout, EOF/connection retirement,
forced bare-response replacement, and packet ownership behavior.

Focused identity and ownership tests cover empty and non-empty recovery data.
Connection-pool timeout, body-stream, chunked, no-body-buffer and HTTP/1 idle
tests pass, as do the focused race run and the complete
`common/utils/lowhttp` package. Across five 256 KiB benchmark samples, the
removed eager buffer has a median of approximately `38.067 µs/op`,
`262,146 B/op`, and `1 alloc/op`; the successful lazy path has a median near
`0.492 ns/op`, `0 B/op`, and `0 alloc/op`. This benchmark intentionally isolates
only the eliminated success-path work.

Heap report `2026-07-24T17-04-36-726Z`, compared with
`2026-07-24T16-33-49-322Z`, no longer contains the old `conn_pool.go:865`
allocation, which accounted for about `28.80 MiB` in the baseline.
`persistConn.readLoop` cumulative allocation changes from `188,813,567` to
`172,880,394 B (-8.4%)`; total window allocation changes from `1,068,583,191`
to `1,027,154,904 B (-3.9%)`, and `bytes.growSlice` falls `5.9%`. Post-GC live
heap falls `2.8%`, while positive live delta regresses `10.2%`, so no retained
heap claim is made. CPU report `2026-07-24T17-09-48-900Z` uses only a 4 KiB
response: the target copy is below sampling visibility while total samples and
throughput regress. It remains adverse diagnostic evidence, not a CPU gain.

The formal strict comparison is `body-2026-07-24T16-46-15-707Z` to
`body-2026-07-24T17-12-11-181Z`, with
`comparison-vs-eager-conn-pool-recovery-copy.{json,md}` in the candidate. All
six runs complete 120/120 requests with exact 64 KiB request and 256 KiB
response bodies and pass database, shadow-stream, detail, virtual-scroll,
CPU-recovery and cleanup gates. Configuration and diagnostic differences are
empty. Candidate backend fingerprint is `b0f9f67fbbc613466aaf` with cache states
`false/true/true`; actual Renderer input fingerprint remains
`fa377df501505467d37539e9407e550f7931d1ebf5607b0d6cc4ce176fe3328a`.
Candidate medians change Yak CPU p95/RSS by `-0.5%/-0.1%` and request p95 by
`-18.2%`, but throughput by `-8.5%`, Electron CPU p95 by `+3.2%`, first-visible
by `+6.7%`, Long Task total from `63` to `152 ms`, and database catch-up/drain
by `+83.8%/+61.7%`. Several candidate ranges are wide and overlap the
baseline. The change is retained on deterministic allocation, ownership and
heap-caller evidence; the matrix does not support a product-wide throughput or
UI-latency improvement claim.

## MITM Metadata-Only Intermediate Response Body

The minimartian HTTP/1 upstream path already captures the complete response in
`responseRaw` or `respBuffer`, then reparses `LowhttpResponse.RawPacket` to
construct the final response. The transport parser nevertheless retained a
separate temporary `http.Response.Body` that this successful MITM path did not
consume. The retained change adds an explicit, default-off lowhttp option used
only by minimartian. It discards that intermediate body only for ordinary,
bounded `Content-Length` responses of at most 1 MiB. HEAD/TRACE/CONNECT,
chunked or CL+TE, header callbacks, `NoBodyBuffer`, too-large responses,
content-length fixing, and larger bodies continue through the previous parser.
Public parser entry points and default lowhttp behavior are unchanged.

The parser still owns a distinct final bare packet, including the historical
rule that a skipped informational response is not part of the stored final
bare response; the outer capture continues to retain all wire bytes. A first
implementation that drained without reserving both existing capture buffers
increased the 256 KiB allocation from roughly 806 KB to 917 KB/op and was
rejected. Replacing the parser-owned final packet with the outer capture was
also rejected because it would include preceding 1xx responses and change
packet semantics. Tests cover byte identity, response-size accounting, short
Content-Length, `100 Continue`, independent capture/context ownership,
chunked/callback/over-limit fallback, and both pooled and non-pooled real HTTP.
Focused and complete tests for `common/utils`, `common/utils/lowhttp`, and
`common/minimartian`, plus the relevant race runs, pass.

Across five 256 KiB benchmark samples, retaining the intermediate body has a
median of `198.073 µs/op`, `806,357 B/op`, and `65 allocs/op`; metadata-only
parsing has a median of `131.973 µs/op`, `544,555 B/op`, and `64 allocs/op`.
That is approximately `-33.4%` time, `-32.5%` bytes, and one allocation. Heap
report `2026-07-26T04-39-01-213Z`, compared with
`2026-07-24T17-04-36-726Z`, changes connection-pool read-loop/parser cumulative
allocation from `172,880,394` to `68,004,055 B (-60.7%)`, response-body-reader
flat allocation from `105,265,512` to `65,291,266 B (-38.0%)`, and parser
cumulative allocation from `244,025,856` to `134,868,305 B (-44.7%)`. Total
window allocation falls `8.8%`, `bytes.growSlice` falls `25.4%`, post-GC live
heap falls `3.4%`, and positive live delta falls `11.6%`. The remaining body
reader allocation belongs to parser callers outside this narrow MITM option.

The 4 KiB CPU diagnostic `2026-07-26T04-44-08-160Z`, against
`2026-07-24T17-09-48-900Z`, is adverse overall: samples change from `2.22` to
`2.40 s`, throughput from `103.89` to `96.87 req/s`, and request p95 from
`164.76` to `248.01 ms`. The target copy is only about 0.5 MiB across that
window and remains below sampling resolution while GC stacks dominate, so this
run supports no CPU-gain claim.

The formal strict comparison is `body-2026-07-24T17-12-11-181Z` to
`body-2026-07-26T04-48-05-606Z`, with
`comparison-vs-retained-intermediate-response-body.{json,md}` in the candidate.
All six runs complete 120/120 requests with exact 64 KiB request and 256 KiB
response bodies and pass database, shadow-stream, detail, virtual-scroll,
CPU-recovery, and cleanup gates. Comparator status is `passed`; configuration
and diagnostic differences are empty. Candidate backend fingerprint is
`a3ca1f78d46d2c0dd0f6` with cache states `false/true/true`, and the actual
Renderer input fingerprint remains
`fa377df501505467d37539e9407e550f7931d1ebf5607b0d6cc4ce176fe3328a`.
Candidate medians change Yak CPU p95 by approximately `0.0%`, Yak peak RSS by
`-1.5%`, throughput by `+8.5%`, database catch-up/drain by `-59.5%/-47.7%`,
and persist-to-React p95 by `-15.4%`. Adverse medians are request p95 `+16.3%`,
duplex-delivery p95 from `82` to `288 ms`, and Yak drain CPU `+42.4%`.
Duplex ranges are `75..557` and `90..319 ms`, with a slightly lower candidate
mean and maximum, and request-p95 ranges overlap. The change is retained for
deterministic allocation, ownership, correctness, and heap-stack evidence; the
mixed WSL matrix does not establish a product-wide UI-latency gain.

## Parser-Owned Request Body Dump Handoff

The request parser already allocates a body independently from both caller
input and the httpctx bare packet. `DumpHTTPRequest` nevertheless copied that
owned body through `io.ReadAll` before writing the final dump buffer and
restoring the request body. The retained change introduces a package-private
owned request body that preserves `io.ReadCloser`, `io.WriterTo`, close, and
remaining-body semantics. The dumper may consume a read-only view only for
that private type, moves the original reader to EOF, and restores a new owned
reader afterward. The serialized output remains an independent copy. External,
plugin-replaced, and otherwise unknown bodies retain the old `io.ReadAll`
fallback. Public APIs, wire bytes, content-length/chunked handling, and
input/bare/body ownership do not change.

Tests cover a partially consumed parser body, consumption of the original
reader, restoration of only the remaining bytes, output/body independence,
and external-body fallback. Existing concurrent parser and 64 KiB ownership
tests continue to pass. Focused race, complete `common/utils`,
`common/mutate`, `common/crep`, `common/minimartian`, and
`common/utils/lowhttp` tests, plus the lowhttp focused race run, pass; all 48
frontend preflight tests pass. Across five 64 KiB parsed-request dump samples,
the previous median is `67.959 µs/op`, `359,369 B/op`, and `34 allocs/op`; the
owned-view median is `16.000 µs/op`, `74,430 B/op`, and `18 allocs/op`, or
approximately `-76.5%/-79.3%/-47.1%`.

Heap report `2026-07-26T05-35-58-885Z`, compared with
`2026-07-26T04-39-01-213Z`, changes `DumpHTTPRequest` cumulative allocation
from `81,358,855` to `20,194,869 B (-75.2%)`; its `io.ReadAll` caller disappears
entirely. Global `io.ReadAll` falls from `104,305,301` to `37,514,854 B
(-64.0%)`, total window allocation from `936,710,334` to `857,827,388 B
(-8.4%)`, and post-GC live heap by `1.9%`. `bytes.growSlice +4.1%`,
`bytes.Clone +2.8%`, and positive live delta `+5.9%` are adverse single-sample
values, so no retained-heap claim is made.

CPU report `2026-07-26T05-41-58-226Z`, against
`2026-07-26T04-44-08-160Z`, uses the relevant 64 KiB request and 4 KiB
response. Total samples fall from `2.40` to `2.28 s (-5.0%)`; baseline
`DumpHTTPRequest 190 ms` and `io.ReadAll 240 ms` fall below the candidate top
threshold, and request-parser cumulative time falls from `280` to `130 ms
(-53.6%)`. Single-run throughput improves `5.9%` and request p95 improves
`45.3%`, while Yak RSS regresses `1.6%` and Electron CPU p95 regresses `6.8%`;
all remain diagnostic rather than release-wide claims.

The formal strict comparison is `body-2026-07-26T04-48-05-606Z` to
`body-2026-07-26T05-50-09-512Z`, with
`comparison-vs-request-body-readback.{json,md}` in the candidate. All six runs
complete 120/120 requests with exact 64 KiB request and 256 KiB response bodies
and pass database, shadow-stream, detail, virtual-scroll, CPU-recovery, and
cleanup gates. Comparator status is `passed`; configuration and diagnostic
differences are empty. Candidate backend fingerprint is
`e89c39ecbfba4c444c9d` with cache states `false/true/true`; actual Renderer
input fingerprint remains
`fa377df501505467d37539e9407e550f7931d1ebf5607b0d6cc4ce176fe3328a`.
Candidate medians change Yak CPU p95/RSS by `+0.1%/-1.3%`, throughput by
`+1.7%`, request p95 by `-1.1%`, and Long Task total by `+1.2%`. Adverse values
are Electron CPU p95 `+11.1%`, first-visible `+17.6%`, Renderer drain `+25.5%`,
database catch-up/drain `+52.9%/+32.8%`, and backend Query p95 `+226.1%`.
Throughput is less variable, while first-visible and some SQLite metrics are
consistently adverse. The change is retained for deterministic microbenchmark,
heap/CPU caller, correctness, and ownership evidence, without a UI-latency
claim.

## Unencoded Auto-Unzip Packet Body View

The MITM V2 response mirror calls `DeletePacketEncoding` to produce the plain
response exposed to plugins. The previous implementation used public
`SplitHTTPPacket`, which copied the complete body even when a response had no
content encoding and was not chunked, then returned the original packet after
detecting that no transformation was required. A normal 256 KiB response thus
created and discarded a 256 KiB temporary allocation.

The retained change is private to `_unzipPacketEncodingInternal`. Detection and
decoding now use `splitHTTPPacketEx(..., copyBody=false)` to obtain a read-only
body view. No-op and conservative-failure paths still return the original
packet and backing slice. Successful gzip, zlib, deflate, brotli, zstd, or
chunked transformations continue to build an independently owned packet via
`ReplaceHTTPPacketBody`. Decoder review found that `codec.HTTPChunkedDecode`
can truncate its diagnostic argument for some malformed packets, so the
chunked branch explicitly retains `bytes.Clone(body)`. The cross-goroutine,
plugin-visible clone stored by `SetPlainResponseBytes` is also deliberately
unchanged.

Tests assert same-pointer return and unchanged input for an unencoded packet,
independent output for gzip, original-packet fallback for invalid gzip, and no
input mutation for malformed chunked bodies. Existing gzip, chunked, zlib, and
deflate tests, the focused lowhttp race run, the complete
`common/utils/lowhttp` suite (`187.53 s`), and focused MITM V2 gRPC
auto-unzip/manual-hijack tests pass. All 48 frontend preflight tests also pass.
Across five 256 KiB unencoded-response samples, the prior median is
`52.909 µs/op`, `263,137 B/op`, and `26 allocs/op`; the body-view median is
`1.412 µs/op`, `974 B/op`, and `25 allocs/op`, or approximately
`-97.3%/-99.6%/-1 allocation`.

Heap report `2026-07-26T06-20-44-227Z`, compared with
`2026-07-26T05-35-58-885Z`, removes the previous `36,980,867 B` cumulative
`DeletePacketEncoding` entry. `MITMV2.func7` falls from `63,394,592` to
`36,906,146 B (-41.8%)`; the remainder aligns with the intentionally retained
plain-response clone. Global `SplitHTTPPacket` cumulative allocation changes
from `63,488,542` to `31,979,396 B (-49.6%)`, while `splitHTTPPacketEx` flat
allocation changes from `69,523,697` to `41,460,950 B (-40.4%)`. Total window
allocation falls from `857,827,388` to `815,443,386 B (-4.9%)`,
`bytes.growSlice` falls `6.5%`, and `bytes.Clone` falls `7.4%`. Post-GC live
heap and positive live delta regress `1.6%` and `6.0%`, respectively, so this
supports a removed temporary-allocation claim, not a retained-heap claim.

The 64 KiB request/4 KiB response CPU diagnostic
`2026-07-26T06-28-11-506Z`, against `2026-07-26T05-41-58-226Z`, changes total
samples from `2.28` to `1.97 s (-13.6%)`, GC flat samples from `810` to
`570 ms`, and Yak peak RSS by `-1.9%`. The optimized copy is response-size
sensitive and is below sampling resolution in this 4 KiB response case in both
runs. Single-run throughput changes `+15.0%`, request p95 `+2.5%`, Electron CPU
p95 `+64.7%`, and first-visible `+16.5%`; these mixed values are diagnostic and
are not attributed to this change.

The formal strict comparison is `body-2026-07-26T05-50-09-512Z` to
`body-2026-07-26T06-33-07-606Z`, with
`comparison-vs-unencoded-unzip-body-copy.{json,md}` in the candidate. All six
runs complete 120/120 requests with exact 64 KiB request and 256 KiB response
bodies and pass database, shadow-stream, detail, virtual-scroll, resource
recovery, and cleanup gates. Comparator status is `passed`; configuration and
diagnostic differences are empty. Candidate backend fingerprint is
`a37abefdd1026d87ba27`. Earlier diagnostics prewarmed the build, so the formal
cache states are `true/true/true`; actual Renderer input fingerprint remains
`fa377df501505467d37539e9407e550f7931d1ebf5607b0d6cc4ce176fe3328a`.
Candidate medians change Yak CPU p95/RSS by `+0.1%/-0.9%`, throughput by
`+0.8%`, request p95 by `-10.8%`, first-visible by `-7.7%`, Renderer drain by
`-7.1%`, and database catch-up/drain by `-22.6%/-15.9%`. Adverse signals are
Electron CPU p95 `+7.0%`, Query RTT `+64.8%`, database change detection
`+77.6%`, and several Query timing segments. Query and database segments remain
highly variable: backend-count CV changes from `137%` to `73%`, and Query RTT
CV from `55%` to `30%`; all final-drain correctness gates pass. The change is
retained for deterministic ownership, microbenchmark, and heap-caller evidence,
without a product-wide UI-latency claim. Chunked and cross-stage plain/bare
copies remain until their ownership can be redesigned and proven separately.

## Mirror-Response Synchronous Filter Body View

The remaining `SplitHTTPPacketFast` body copy in the Phase 33 heap was traced
to `handleMirrorResponse`. In the default path, the returned body is only read
synchronously by the bundled/static-JavaScript filter. The previous code still
copied the complete response body on every flow. The body crosses an
asynchronous goroutine boundary only when a `MirrorHTTPFlow` plugin hook is
actually installed.

The retained change uses `SplitHTTPHeadersAndBodyFromPacketView` for the
synchronous filter. If an asynchronous mirror hook exists, it takes a
`bytes.Clone` snapshot before starting the goroutine, preserving the historical
independent body and avoiding a read/mutation race. Oversized responses retain
the same header-only plain-response behavior while plugins still receive the
full independent body. Public split APIs, filter decisions, stored packets, and
cross-stage plain/bare snapshots are unchanged.

A product-level test checks byte-identical headers and bodies against the old
split, verifies that the synchronous body is a view, and verifies two-way
independence of the asynchronous hook snapshot. Static-JavaScript filtering,
auto-unzip/manual-hijack focused tests, the focused race run, and all 62
`TestGRPCMUSTPASS_MITMV2*` tests (`211.44 s`) pass. Across five 256 KiB samples,
the existing clone/view benchmark medians are `55.616` versus `0.897 µs/op
(-98.4%)`, `262,778` versus `618 B/op (-99.8%)`, and `17` versus `16
allocs/op`.

Heap report `2026-07-26T07-10-20-462Z`, compared with
`2026-07-26T06-20-44-227Z`, removes the former `31,979,396 B` cumulative
`SplitHTTPPacketFast`/`SplitHTTPPacket` path. `splitHTTPPacketEx` flat allocation
falls from `41,460,950` to `7,084,035 B (-82.9%)`, cumulative allocation from
`48,277,346` to `11,278,907 B (-76.6%)`, and `MITMV2.func25` by `6.1%`. Total
window allocation falls only `0.1%` because sampled `bytes.growSlice` and
`bytes.Clone` regress approximately `9.1%` and `7.1%`. Post-GC live heap falls
`5.7%`, while positive live delta regresses `5.0%`; only the removed target
caller is treated as deterministic evidence.

The standard 4 KiB response CPU diagnostic `2026-07-26T07-14-13-863Z`, against
`2026-07-26T06-28-11-506Z`, changes total samples from `1.97` to `2.06 s
(+4.6%)` and GC flat samples from `570` to `870 ms`. The response-size-sensitive
target remains below sampling resolution. Yak CPU p95 is effectively unchanged,
RSS changes `-1.6%`, throughput `+2.2%`, request p95 `-0.6%`, and first-visible
`+11.7%`; the run is a mixed regression diagnostic, not a CPU-gain claim.

The first formal candidate matrix, `body-2026-07-26T07-16-32-198Z`, remains as
failed evidence. Its third repetition received `Promise was collected` from the
Electron CDP bridge before load began; both application windows and Yak stayed
alive and cleanup succeeded. The frontend now retries exactly once only around
the idempotent scenario-observer installation and only for that exact transport
error. Application/assertion failures are never retried, and a second transport
failure is surfaced. Four new tests and all 52 preflight tests pass. No samples
from the failed matrix were reused.

The valid formal strict comparison is `body-2026-07-26T06-33-07-606Z` to
`body-2026-07-26T07-26-19-918Z`, with
`comparison-vs-mirror-response-body-copy.{json,md}` in the candidate. All six
runs complete 120/120 requests and every body, database, shadow-stream, detail,
virtual-scroll, recovery, and cleanup gate. Comparator status is `passed`; its
configuration and diagnostic differences are empty. Candidate backend
fingerprint is `578160e9bb081343ca73`, cache states are `true/true/true`, and
actual Renderer input fingerprint remains
`fa377df501505467d37539e9407e550f7931d1ebf5607b0d6cc4ce176fe3328a`.
Candidate medians change Yak CPU p95/RSS by `-0.1%/+1.2%`, throughput by
`+15.3%`, request p95 by `-14.6%`, first-visible by `-22.6%`, and Long Task
total by `-32.5%`. Adverse values are Electron CPU p95 `+6.0%`, Renderer drain
`+41.9%`, database catch-up/drain `+75.7%/+47.7%`, persistence queue wait p95
`+118.2%`, and duplex delivery `+623.5%`. Candidate throughput range
`71.3..79.4 req/s` exceeds the baseline `61.9..73.8`, and request-p95 CV falls
from `14.9%` to `3.3%`. Under an unbounded producer, faster ingestion can expose
more downstream backlog and drain time; these mixed results do not establish a
product-wide UI gain. The change is retained for ownership, microbenchmark,
heap-caller, and neutral Yak CPU/RSS evidence.

## Eager MITM Request Fix And Reparse

The MITMV2 request-hijack callback already owns minimartian's parsed
`originReqIns`, including the request-scoped httpctx state. The previous hot
path nevertheless called `FixHTTPRequest` on the original packet and then
called `ParseBytesToHttpRequest`, which internally fixes CRLF again. The parsed
result was unused by normal traffic. Its only apparent consumer was a
manual-drop flow option, but `createHTTPFlowFromHTTP` subsequently overwrote
that option with `originReqIns`, so the work was dead there as well.

The retained change removes both eager operations and the ineffective drop-flow
option. It does not change the packet, httpctx, plugin input, persisted schema,
or public API. The manual-drop and mirror-response focused tests, all 62
`TestGRPCMUSTPASS_MITMV2*` tests (`211.532 s`), and a focused race run (`11.586
s`) pass. Across five 256 KiB request samples, reusing `originReqIns` is about
`0.49 ns/op / 0 B/op / 0 allocs`; the removed eager fix/reparse median is
approximately `237.984 us/op / 1,347,495 B/op / 99 allocs`.

Heap report `2026-07-26T08-03-27-479Z`, compared with Phase 34 report
`2026-07-26T07-10-20-462Z`, changes total window allocation from `815,443,386`
to `748,489,570 B (-8.2%)`. The MITMV2 request-handler cumulative stack falls
from `54.93` to `7.40 MiB`; the two removed source lines, previously about
`11.76 + 33.60 MiB`, disappear. Global `ParseBytesToHttpRequest` allocation
falls from `113,133,768` to `71,051,477 B (-37.2%)`, and
`FixHTTPPacketCRLF` from `59,347,977` to `35,181,026 B (-40.7%)`. The diagnostic
binary fingerprint is `146cbf678e5c0e211f75`, and all 120 flow/body/database/
stream/cleanup gates pass.

The formal strict shadow comparison is Phase 34 matrix
`body-2026-07-26T07-26-19-918Z` to candidate
`body-2026-07-26T09-01-25-649Z`, with
`comparison-vs-eager-request-fix-parse.{json,md}` in the candidate. All six runs
complete 120/120 and the comparator passes with no configuration or diagnostic
difference. The actual Renderer input fingerprint remains
`fa377df501505467d37539e9407e550f7931d1ebf5607b0d6cc4ce176fe3328a`;
candidate backend fingerprint is `6fe8eab244e61640d25e` with cache states
`true/true/true`. Medians change Yak CPU p95/RSS by `-0.1%/-9.2%`, Electron CPU
p95 by `-29.5%`, request/response-to-React by `-2.2%/-4.1%`, and Renderer drain
by `-9.4%`. Adverse values are throughput `-5.2%`, request p95 `+9.1%`, first
visible `+13.2%`, Long Task `+47.3%`, persistence-write p95 `+66.7%`, and Yak
drain CPU `+188.9%`. The change is retained for deterministic dead-work,
microbenchmark, heap-caller, and RSS evidence, without claiming a universal
end-to-end speedup from a short max-rate sample.

## Fixed-Rate Direct Live Default Promotion

To separate faster backend production from database and Renderer effects, the
Phase 35 source was held constant while the 1,000-request, 200 requests/second
fixed-rate case ran three times in each UI mode. The shadow matrix is
`body-2026-07-26T08-06-39-849Z`. Persistence queue and write p95 are both `1
ms`, and database-change detection p95 is `20 ms`, while trigger-to-Query p95 is
`996.2 ms`, persist-to-React p95 is `958 ms`, and maximum visible backlog is
`193`. This attributes the dominant top-follow delay to the Query visibility
path, not SQLite writes. Earlier writer2 and separate read-pool experiments had
already regressed the same fixed-rate case, so no database concurrency setting
was changed.

The direct body-free stream candidate is matrix
`body-2026-07-26T08-12-43-176Z`; its
`comparison-vs-shadow-phase35.{json,md}` allows only the declared
`httpFlowLiveStreamMode: shadow -> canary` difference and passes. Every run
commits, stores, and renders exactly 1,000/1,000 unique rows using 11 direct
batches, with zero Query, fallback, Gap, sequence gap, duplicate, out-of-order,
or unavailable event.

| Metric | Shadow/query | Direct canary | Change |
| --- | ---: | ---: | ---: |
| Request-to-React p95 | `970 ms` | `490 ms` | `-49.5%` |
| Persist-to-React p95 | `958 ms` | `486 ms` | `-49.3%` |
| First visible | `117 ms` | `42 ms` | `-64.1%` |
| Maximum visible-ID backlog | `193` | `36` | `-81.3%` |
| Renderer drain | `474 ms` | `434 ms` | `-8.4%` |
| Yak peak working set | `613.3 MiB` | `558.2 MiB` | `-9.0%` |
| Electron CPU p95 | `8.4%` | `6.3%` | `-24.9%` |
| Electron CPU p50 | `2.5%` | `3.1%` | `+25.3%` risk |
| Yak CPU p95 | `145.0%` | `151.2%` | `+4.3%` risk |
| Long Task total | `569 ms` | `0 ms` | candidate maximum `105 ms` |
| Throughput | `200.1 req/s` | `200.1 req/s` | flat |

The current-backend slow-consumer matrix is
`body-2026-07-26T08-23-13-914Z`. Its 800-flow small-body case reconciles `234
direct + 566 Query = 800` rows (540 live-stream fallback rows), with one recovery
entry/completion, and drains 1,921 ms after returning to the top. Its 240-flow
64/256 KiB case reconciles `89 direct + 151 Query = 240` rows (142 fallback
rows), also with one entry/completion, and drains in 2,182 ms. Both end with zero
missing ID, pending match, Gap, sequence error, or duplicate.

This evidence promotes the dedicated body-free stream to the product default
for a compatible MITM top view. Query remains the compatibility and correctness
fallback for old-engine `UNIMPLEMENTED`, Gap/disconnect, project/filter change,
off-top scrolling, recovery, and incompatible cursors; there is no protocol,
database, or public-API migration. The rebuilt-default smoke matrix
`body-2026-07-26T09-14-17-341Z` passes 1,000/1,000 with 11 direct batches, zero
Query/fallback/protocol error, request-to-React `492 ms`, first-visible `42 ms`,
and maximum visible backlog `43`. Reconnect/replay, project/filter transitions,
long-duration/burst, 100-site, and real Chromium/nuclei tests remain expansion
gates.

## Controlled Response Packet Body Views

The next heap-driven candidate targets two response-parser Body copies without
weakening the ownership boundary. Minimartian already owns the complete
`LowhttpResponse.RawPacket` for the lifetime of the final `http.Response`, but
the previous final parse allocated a second Body. Interactive response hijack
also copied the selected packet into an independent snapshot and then made a
second Body allocation while parsing that snapshot.

`ReadHTTPResponseFromBytesWithBodyView` is an explicit opt-in parser for an
immutable, caller-owned complete packet. Its returned Body aliases that packet,
and the API contract requires the packet to remain unchanged until the response
is no longer used. The existing `ReadHTTPResponseFromBytes` API retains its
historical independent-Body semantics. Minimartian opts in only for its owned
raw packet. The hijack path still performs exactly one required `bytes.Clone`
snapshot before parsing and makes Body a view of that owned snapshot. This is an
additive API with no protocol, persistence-schema, or existing-caller migration.

Tests cover regular and short Content-Length responses, an informational
response followed by the final response, chunked input, input immutability,
documented aliasing, the minimartian caller, and independence of the hijack
snapshot from its source. Focused and full tests for `common/utils`,
`common/minimartian`, and `common/crep`, a focused race run, and every
`TestGRPCMUSTPASS_MITMV2*` test (`210.885 s`) pass. Across five 256 KiB samples,
the owned-Body parser median is approximately `66.299 us/op / 264,974 B/op / 54
allocs`; the controlled view median is `5.005 us/op / 2,824 B/op / 53 allocs`,
or about `-92.5%` time and `-98.9%` allocated bytes for the isolated operation.

Heap report `2026-07-26T10-24-19-483Z`, compared with Phase 35 report
`2026-07-26T08-03-27-479Z`, changes total window allocation from `748,489,570`
to `692,794,542 B (-7.4%)`. The two old
`readHTTPResponseBodyWithLimit` call paths, totaling about `67.35 MiB`, and the
corresponding `ParseBytesToHTTPResponse` path disappear. The approximately
`36.24 MB` `bytes.Clone` below `cloneAndParseHijackedResponse` is the retained
hijack snapshot made explicit; the old code allocated the same ownership
boundary with `make/copy`. All 120 workload and correctness gates pass. The
diagnostic backend fingerprint is `53d8b443935d4979830f`. Single-sample adverse
movement in global `bytes.growSlice` and other unrelated callers is retained as
risk and is not used to claim lower steady-state memory.

The formal strict shadow comparison is
`body-2026-07-26T09-01-25-649Z` to
`body-2026-07-26T10-41-13-373Z`, with
`comparison-vs-copied-response-body.{json,md}` in the candidate. All six runs
complete 120/120, and the comparator passes with empty configuration and
diagnostic differences. Actual Renderer input remains
`fa377df501505467d37539e9407e550f7931d1ebf5607b0d6cc4ce176fe3328a`;
candidate backend fingerprint is `10ec43a2a0d6c40ecb0b` with cache states
`false/true/true`. Medians change throughput by `+3.7%`,
request/response-to-React by `-4.5%/-2.6%`, persistence queue/write p95 by
`-22.4%/-43.3%`, Long Task total by `-25.5%`, and backend conversion per flow
by `-49.3%`; Yak CPU p50/p95 is approximately `-0.2%/+0.2%`. Adverse medians
are Yak RSS `+5.6%`, Renderer drain `+12.0%`, Electron CPU p95 `+12.9%`, request
latency p95 `+8.3%`, first-visible `+6.3%`, and noisy database-change detection
`+149.3%`. The candidate is retained for deterministic allocation and ownership
evidence, not as a universal endpoint-speed claim.

After restoring the product-default canary Renderer fingerprint
`9cb881b6298183534bb3d574bb9403af8264269dd20645d6aa01d7fdfcd96f6f`, the
fixed-rate smoke matrix `body-2026-07-26T10-51-59-171Z` passes exactly
1,000/1,000 at `199.86 req/s`, using 11 direct batches and zero Query, fallback,
Gap, sequence, duplicate, or unavailable errors. Request-to-React is `489 ms`,
first-visible is `44 ms`, and maximum visible-ID backlog is `39`.

## Parser-Owned Bare Request Handoff

The Phase 37 heap separates the remaining global `bytes.Clone` allocation into
ownership-specific callers. The request parser built a complete bare packet in
its own `bytes.Buffer`, then passed it to `SetBareRequestBytes`, whose public
compatibility contract cloned externally mutable input. The parser never used
its buffer after that call, so this was a duplicate ownership boundary rather
than required wire storage.

`SetBareRequestBytesOwned` is a narrow transfer API used only by the parser. The
existing `SetBareRequestBytes` and every plain/hijacked request or response
setter keep cloning. Caller input, the httpctx bare packet, and `req.Body`
remain independently mutable, while the parser-owned reconstruction becomes
the context-owned packet. There is no wire, database, protobuf, or existing-API
behavior change.

Tests verify that the compatibility setter still clones, the owned setter
retains the transferred backing array, a 64 KiB caller packet cannot mutate the
bare packet or Body, bare mutation cannot affect Body, and 128 concurrent
parsers remain independent. Full `common/utils` (`28.449 s`), full
`common/utils/lowhttp` (`183.745 s`), httpctx, and focused race tests pass. One
first all-MITMV2 combined run hit the fixed four-second database-query deadline
in `InvalidUTF8RequestDetail`; the test then passed three isolated repetitions
(`2.72/1.66/1.55 s`) and the complete suite passed on rerun (`195.780 s`). This
is retained as a deadline-flake record rather than hidden or attributed to the
ownership change.

Across five 64 KiB request-parser samples, the median changes from `54.235` to
`30.083 us/op (-44.5%)`, from `215,453` to `141,724 B/op (-34.2%)`, and from
`50` to `49 allocs`. Heap report `2026-07-26T11-15-02-333Z`, compared with
Phase 37 report `2026-07-26T10-24-19-483Z`, removes the approximately `21.98
MiB` parser branch through `SetBareRequestBytes -> bytes.Clone`. Global
`bytes.Clone` falls from `116,717,473` to `88,642,234 B (-24.1%)`, request
parser cumulative allocation from `128,500,199` to `94,476,908 B (-26.5%)`,
and total window allocation from `692,794,542` to `647,763,815 B (-6.5%)`.
External hijack bare-request, plain request/response, and response-snapshot
clones remain. `bytes.growSlice +1.3%`, post-live heap about `+10.0%`, and
positive live delta about `+60.4%` are adverse single-sample values, so this is
not steady-state-memory evidence. All 120 gates pass; diagnostic backend
fingerprint is `c8547a51c57cfa36f129`.

The formal strict shadow comparison is
`body-2026-07-26T10-41-13-373Z` to
`body-2026-07-26T11-51-53-723Z`, with
`comparison-vs-cloned-request-bare-packet.{json,md}` in the candidate. All six
runs complete 120/120, and the comparator passes with empty configuration and
diagnostic differences. Renderer input is unchanged at
`fa377df501505467d37539e9407e550f7931d1ebf5607b0d6cc4ce176fe3328a`;
candidate backend fingerprint is `ae8561a7c4869b2905a6` with cache states
`false/true/true`. Request p95 improves `11.9%`, first-visible `15.3%`, backend
conversion `20.1%`, and Renderer drain `7.9%`; Yak CPU p95 and RSS are flat.
Adverse medians are throughput `-2.1%`, request-to-React `+4.2%`, Electron CPU
p95 `+10.5%`, Long Task `+13.8%`, persistence queue/write p95 `+9.6%/+47.1%`,
and highly variable database-change detection `+181.4%`. Most ranges overlap,
so the change is retained for deterministic ownership, microbenchmark, and
heap-caller evidence rather than a universal UI-speed claim.

After restoring the product-default canary Renderer, fixed-rate smoke matrix
`body-2026-07-26T12-00-09-473Z` passes exactly 1,000/1,000 at `200.10 req/s`
using 11 direct batches, zero Query/fallback/protocol errors, request-to-React
`493 ms`, first-visible `43 ms`, maximum visible-ID backlog `38`, SQLite
queue/write p95 `1/1 ms`, and zero Long Task time.

## Bounded Content-Length Request Body Read

The Phase 38 heap attributed the remaining network-request `io.ReadAll` branch
to known `Content-Length` bodies. That path geometrically grew a temporary
buffer while the parser-owned raw packet grew independently, then copied the
body into its final request storage. For positive lengths up to 1 MiB, the
parser now reads into one exact-sized slice with `io.ReadFull`, reserves the
same bounded capacity in the raw packet, and transfers the newly read slice to
the owned request Body. The 1 MiB threshold is an internal allocation guard,
not protocol semantics. Larger, chunked, and unknown-length bodies retain their
legacy read path.

Short wire bodies retain their historical newline padding only in `req.Body`;
the bare packet preserves the actual bytes received. The ambiguous
`Content-Length + Transfer-Encoding: chunked` branch keeps its existing
Content-Length-first behavior. Caller input, httpctx bare packet, and Body are
still independently mutable. Tests cover complete and short bodies, the
ambiguous-header short-body case, over-threshold fallback, the allocation
bound, ownership, and concurrent parsing. Full `common/utils` (`28.353 s`), a
focused race run, all `common/utils/lowhttp/...` tests (`185.804 s` for the main
package), and all `TestGRPCMUSTPASS_MITMV2*` tests (`208.039 s`) pass.

Across five adjacent 64 KiB bufio-parser samples, the median changes from
`83.963` to `34.462 us/op (-59.0%)`, from `426,616` to `141,644 B/op (-66.8%)`,
and from `63` to `45 allocs/op (-28.6%)`. Heap report
`2026-07-26T14-24-26-460Z`, compared with Phase 38 report
`2026-07-26T11-15-02-333Z`, reduces cumulative allocation below
`ReadHTTPRequestFromBufioReaderOnFirstLine` from `60,929,767` to `22,806,243 B
(-62.6%)` and below `readHTTPRequestFromBufioReader` from `94,476,908` to
`51,366,390 B (-45.6%)`. The former `io.ReadAll` stack (`42,996,592 B`
cumulative) disappears; the replacement helper is `11,712,507 B` cumulative.
All 120 heap-scene correctness gates pass, and its diagnostic backend
fingerprint is `a9fd90da679f70ca70d7`.

Total allocation in the single heap windows changes from `647,763,815` to
`651,135,179 B (+0.5%)`; post-live heap changes `-4.1%` and positive live delta
`+0.08%`. Global grow/clone and SQLite samples also move in different
directions. These values do not establish a total-allocation or residency gain;
the retained evidence is the isolated benchmark and target caller reduction.

The formal strict shadow comparison is
`body-2026-07-26T11-51-53-723Z` to
`body-2026-07-26T14-40-58-936Z`, with
`comparison-vs-bounded-content-length-read.{json,md}` in the candidate. All six
runs complete 120/120, and the comparator passes with no configuration or
diagnostic differences. Renderer input remains
`fa377df501505467d37539e9407e550f7931d1ebf5607b0d6cc4ce176fe3328a`;
candidate backend fingerprint is `c0bdbe701f755f542dd7` with cache states
`false/true/true`. Medians change throughput by `+7.4%`, Electron CPU p95 by
`-16.4%`, Long Task total by `-55.7%`, and Yak CPU p95/RSS by `-0.2%/-1.2%`.
Adverse medians are request latency p95 `+16.8%`, first-visible `+13.9%`,
request-to-React `+4.3%`, and Renderer drain `+37.6%`; most ranges overlap. The
change is retained for direct parser and heap evidence, not a universal UI
latency claim.

After restoring the product-default canary Renderer fingerprint
`9cb881b6298183534bb3d574bb9403af8264269dd20645d6aa01d7fdfcd96f6f`, fixed-rate
smoke matrix `body-2026-07-26T14-49-28-551Z` passes exactly 1,000/1,000 at
`200.09 req/s`, using 11 direct batches and 1,000 direct rows with zero Query,
fallback, Gap, sequence, duplicate, out-of-order, or unavailable event.
Request-to-React is `496 ms`, first-visible is `53 ms`, maximum visible backlog
is `43`, SQLite queue/write p95 is `1/1 ms`, and the single Long Task sample is
`107 ms`. SQLite is still not the fixed-rate bottleneck, so this phase does not
change GORM, pool sizing, persistence, or Renderer consumption.

## Bounded Renderer Trace And Repeated Samples

The frontend harness now has an explicit Renderer diagnostic for the same body
matrix scene:

```bash
yarn test:e2e:electron:mitm-renderer-trace
```

It reuses the WDIO-owned main Renderer CDP session and does not change the
release application's remote-debugging policy. Capture uses a 16 MiB
`recordUntilFull` buffer, one 30-second stop/flush/read deadline and a 64 MiB
artifact ceiling. `renderer-trace-summary.json` deduplicates nested task
envelopes and attributes Renderer-main-thread tasks of at least 50 ms to
JavaScript, style/layout, paint/composite, GC and IPC. Trace, CPU profile and
heap profile are mutually exclusive, require one repetition and are rejected by
the comparator as diagnostic-only.

For 120 requests at concurrency 8 with a 64 KiB request and 256 KiB response,
the pre-change trace (`2026-07-23T03-47-54-677Z`) measured six Long Tasks totaling
`401.211 ms`; the independent Long Task Observer measured `397 ms`. Three
`Receive mojo reply` events accounted for `180.771 ms`. The remaining frame
tasks included React mouseover dispatch plus style, layout and paint. Inspection
found that a table-wide hover ID caused both virtual-cell memo comparators to
invalidate every visible cell whenever hover state changed during live inserts.

After limiting invalidation to the old and new hover rows, React
`EventDispatch` cumulative duration changed from `50.297` to `14.850 ms`, and
the corresponding function-call duration changed from `49.719` to `14.390 ms`
(about 71% lower). Trace Long Task total changed from `401.211` to `361.068 ms`;
the Observer changed from `397` to `359 ms`. IPC reply time was effectively
unchanged (`180.771 → 184.693 ms`), which both validates the narrow frontend fix
and identifies IPC reply handling as the next cross-process target.

The first post-change three-repeat candidate is recorded in the frontend matrix
`body-2026-07-23T04-00-31-083Z`. All three samples completed 120/120 requests,
had no ID gaps/duplicates or cleanup failures, omitted raw list packets and
passed exact detail-body checks. Medians and ranges were: `36.45
[31.69..39.51] req/s`, first visible `669 [492..684] ms`, query round-trip P95
`88.6 [72.7..129.3] ms`, request-to-React P95 `1209 [1109..1210] ms`, Long Task
total `357 [295..396] ms`, and Yak peak working set `882.9 [877.9..884.3] MB`.
This validates the repeat pipeline and quantifies WSL noise; it is not a formal
before/after median comparison because the pre-change side has only a single
unprofiled sample.

The next measurement boundary is protobuf/gRPC decode, Main-side object
construction, Main-to-Renderer structured clone, transferred batch bytes and
Renderer state submission. Fixed-hardware baseline/candidate runs must each use
at least three repetitions before changing the protocol or regression gates.

## IPC/Layout Exclusion Experiments And MITM Overscan

The trace summary now retains bounded native task origins, expensive nested
events, IPC interface/payload/data bytes, and layout element/object/root details.
A trace with backend `SystemTiming` disabled (`2026-07-23T04-20-47-726Z`) still
measured three IPC-native tasks totaling `171.066 ms`, the same order as
`184.693 ms` with timing enabled. Bounded backend observability is therefore not
the current IPC bottleneck and remains enabled by default.

Two plausible changes failed the real Electron trace and were removed. Explicit
MITM-only protobuf default-field compaction reduced synthetic object size but did
not reduce IPC task time; replies around 160 bytes still took 65--68 ms. No
global proto loader or gRPC response contract was changed. CSS layout containment
still laid out the document root and changed cumulative style/layout work from
roughly 121 to 143 ms. Enabling Chromium invalidation tracking caused severe
observer effect (4,334 ms of Long Tasks, maximum 1,614 ms), so that category is
also excluded from the bounded trace and its run is not performance evidence.

The retained frontend change is local to MITM: virtual-table overscan is 5
instead of the previous effective 10; other tables retain their existing
default. The candidate trace (`2026-07-23T04-55-48-958Z`) changed
`UpdateLayoutTree` element count from 2,204 to 1,864 (-15.4%), dirty layout
objects from 2,907 to 2,462 (-15.3%), and inclusive duration from 81.494 to
49.580 ms (-39.2%). After the performance window, the E2E scene records the DOM
footprint, scrolls to 1,120 px, verifies the rendered first row changes, and
restores the original row and offset. All three candidate repetitions passed
that lifecycle as `120 → 84 → 120`.

The same-machine three-repeat baseline
`body-2026-07-23T04-00-31-083Z` and candidate
`body-2026-07-23T04-59-24-854Z` had these medians:

| Metric | Overscan 10 | Overscan 5 | Change |
| --- | ---: | ---: | ---: |
| Long Task total | `357 ms` | `166 ms` | `-53.5%` |
| Long Task blocking ratio (diagnostic) | `9.89%` | `4.41%` | `-55.4%` |
| Request to React P95 | `1209 ms` | `1130 ms` | `-6.5%` |
| Response to React P95 | `1073 ms` | `1034 ms` | `-3.6%` |
| Throughput | `36.45 req/s` | `39.18 req/s` | `+7.5%` |
| First visible | `669 ms` | `765 ms` | `+14.3%` (slower; candidate CV `31.4%`) |
| Query round-trip P95 | `88.6 ms` | `112.9 ms` | `+27.4%` (slower/noisy) |

This is not an all-metrics improvement claim. The overscan change is retained
because DOM/layout work fell deterministically, Long Task reduction was stable
(`160..169 ms` in the candidate), and scroll correctness passed. First-visible
and Query P95 remain explicit risks. The query regression is dominated by the
variable backend query phase and must be investigated separately from Renderer
layout; fixed-hardware before/after runs are still required for release claims.

## Electron Real-Engine Contract

The Yak CLI emits two readiness signals after its listener is bound:

```text
yak grpc ready {"schemaVersion":1,"address":"127.0.0.1:<port>"}
yak grpc ok
```

The JSON line is the machine contract and supports `yak grpc --port 0`; the
legacy `yak grpc ok` line remains for existing scripts. Consumers must validate
the schema and loopback address, then complete an Echo RPC before declaring the
engine ready. The Yakit E2E runner builds the current backend worktree with
bounded resources, uses isolated databases, records the Git/Go identity, and
owns process-tree cleanup. See the frontend worktree's `e2e/README.md` for the
operational workflow.

When and only when `--pprof` is explicitly enabled, Yak also emits:

```text
yak grpc pprof ready {"schemaVersion":1,"address":"127.0.0.1:<port>"}
```

`--pprof-listen` defaults to the legacy `:18080`; automation must override it
with `127.0.0.1:0`. `--pprof-listen` and `--pprof-block-rate` are rejected
unless `--pprof` is present, so the normal gRPC startup surface is unchanged.

## Output

The JSON report includes Git revisions, dirty state, a SHA-256 fingerprint of
tracked changes and untracked source files, resource configuration, all
comparable metrics, correctness checks, and raw command artifacts. Metric names
and units form the comparison key. Renaming a metric intentionally makes it
non-comparable to old reports and must be called out in the PR.

## Borrowed Final Response Packet In The MITM Connection Pool

The Phase 39 heap attributed another full response-sized allocation to a narrow
connection-pool path. The pool must retain the complete wire response in its
capture buffer, while the metadata parser separately rebuilt the same bounded
`Content-Length` body in its local `rawPacket`. MITM immediately treated the
parser copy as immutable response context, so keeping both byte-for-byte copies
until flow construction was duplicate storage rather than required ownership.

The retained candidate adds an explicit internal opt-in. It is enabled only by
minimartian together with `DiscardIntermediateResponseBody`, only for the final
bounded fixed-`Content-Length` response, and only when neither a body-stream
callback nor SSE auto-detection can retain or mutate parser state. The callback
returns the exact final-response suffix of the connection-pool capture buffer,
including the case where `100` or `103` informational responses precede it.
Chunked, unknown-length, over-limit, non-pool, stream/SSE and invalid-callback
paths keep the original owned packet. The new httpctx borrowed setter documents
that the packet aliases the response context and must remain immutable; existing
public setters still clone.

Tests cover normal and short fixed-length responses, a final response after
`103 Early Hints`, invalid callback lengths, chunked fallback, actual pool
aliasing, non-pool ownership, and pointer identity. Focused tests and race,
full `common/utils` (`28.470 s`), full `common/utils/lowhttp/...` (main package
`186.651 s`), and minimartian pass. The first all-MITMV2 suite-level run ended
non-zero after about `208.8 s` without a retained individual failure in the
truncated output; the final visible hijack-filter test passed alone and a full
JSON rerun completed with zero failed actions. This is recorded as a long-suite
flake rather than hidden or attributed to the packet handoff.

Across five adjacent 256 KiB discarded-response metadata samples, the previous
path has median `110.392 us/op`, `544,440 B/op`, and `64 allocs/op`; the borrowed
path has `69.911 us/op (-36.7%)`, `273,959 B/op (-49.7%)`, and `61 allocs/op
(-4.7%)`. The ordinary owned discard branch remains about `113.187 us/op` and
`544,480 B/op`, demonstrating that callers without the explicit contract retain
their old allocation and ownership behavior.

The like-for-like shadow heap comparison is Phase 39 report
`2026-07-26T14-24-26-460Z` to candidate
`2026-07-26T16-08-37-759Z`: both run 120 requests at concurrency 8 with 64 KiB
requests and 256 KiB responses, and use the same actual Renderer input fingerprint
`9cb881b6298183534bb3d574bb9403af8264269dd20645d6aa01d7fdfcd96f6f`.
All correctness and cleanup gates pass. Total cumulative allocation changes from
`651,135,179` to `573,912,033 B (-11.86%, -77.22 MB)`. The target response parser
changes from `72,323,530` to `41,687,646 B (-42.36%, -30,635,884 B)`, matching
approximately 120 copies of a 256 KiB response; `bytes.growSlice` changes from
`205,098,692` to `159,884,440 B (-22.05%)`. Post-live heap changes `+1.24%` and
positive live delta `+3.45%`, so the result does not establish a steady-state
residency improvement.

The formal strict shadow 3+3 comparison is
`body-2026-07-26T14-40-58-936Z` to
`body-2026-07-26T16-11-14-215Z`; comparison artifacts are
`comparison-vs-phase39-response-borrow.{json,md}` in the candidate directory.
All six runs complete 120/120 and the comparator passes with empty configuration
and diagnostic differences. Medians improve request p95 by `26.0%`, first-visible
by `16.2%`, and Renderer drain by `22.6%`; request/response-to-React are flat and
Yak CPU p95 changes `+0.3%`. Adverse medians are throughput `-2.9%`, Yak peak RSS
`+1.7%`, Electron CPU p95 `+11.5%`, Long Task total `62 -> 110 ms`, and Query p95
`65 -> 90 ms`. Their ranges are noisy or overlapping, so the candidate is retained
for the direct microbenchmark and heap-caller evidence, not as a universal UI,
CPU, or memory-residency improvement.

The larger bounded slow-consumer report
`2026-07-26T16-17-54-684Z` sends 240 requests at 30 req/s with 64 KiB requests
and 256 KiB responses: 15 MiB request bodies plus 60 MiB response bodies. It
passes 240/240 with exact database and target identity, direct 98 plus Query 142,
one recovery entry/completion, no Gap/sequence gap/duplicate/out-of-order event,
and zero final backlog. Return-to-top drain is `2,117 ms`, database/Renderer drain
is `276/311 ms`, CPU recovery is `2,018 ms`, request p95 is `44.38 ms`, Query p95
is `16.1 ms`, and SQLite queue/write p95 is `9/10 ms`. This expands body volume
without increasing concurrency beyond the resource envelope.

Finally, after restoring the default canary, fixed-rate smoke
`body-2026-07-26T16-31-25-058Z` passes 1,000/1,000 at `200.10 req/s`, with 11
direct batches, 1,000 direct rows, zero Query/fallback/protocol error, maximum
visible backlog 37, and zero final persistence or visible backlog. No GORM,
database schema, protobuf, frontend consumption, or public lowhttp behavior is
changed in this phase. The next backend selection remains profile-driven:
required quote output, request post-parameter parsing, packet CRLF/body rebuilds,
and SQLite bind are candidates, but none will be changed without caller-level
allocation plus an equivalent correctness baseline.

## Allocation-Free Query Pair Framing And Unescape Fast Path

The Phase 40 heap showed `GetPostCommonParams` at `81,429,814 B` cumulative for
the 120 x 64 KiB request scene. Its form fallback passed an already immutable
string through `bytes.NewBufferString`, allocated a second query-sized
`bufio.Reader`, and allocated each `ReadString('&')` result. It then sent plain
strings with no escape marker through `%u` regular-expression replacement and
`net/url` query decoding. These allocations did not contribute to parameter
semantics.

`ParseQueryParams` now frames pairs by taking immutable string slices at each
`&`. Its existing handler still performs the same trim, first-`=` split,
`{{urlescape(...)}}` compatibility, position/options and ordered item creation.
`ForceQueryUnescape` returns its input directly when neither `%` nor `+` occurs;
all encoded and malformed inputs retain the old `%u` plus `url.QueryUnescape`
path. The form fallback also provides its owned request-body snapshot as a
read-only string view and keeps the bytes alive through parsing. Returned
strings keep that owned backing storage alive; no mutable external packet is
borrowed.

The parser is differentially tested against the previous buffered implementation
for fixed edge cases and 2,000 deterministic random byte strings under three
option sets. Coverage includes empty/repeated/trailing separators, multiple
equals, template escapes, `%u`, `+`, invalid percent escapes, CR/LF, Unicode and
invalid UTF-8. A separate oracle compares the unescape fast path with the old
composition. Existing JSON/XML/form/Base64 and repeated-body tests, focused race,
full codec, full mutate (`15.247 s`), full `common/utils/lowhttp/...` (main package
`183.474 s`) and all MITMV2 MUSTPASS tests (`203.900 s`) pass.

Across five whole 64 KiB binary-body samples, `GetPostCommonParams` changes from
median `421.846` to `247.209 us/op (-41.4%)`, from `656,162` to `65,967 B/op
(-89.9%)`, and from `23` to `10 allocs/op (-56.5%)`. The paired old/new parser
benchmark in one binary changes from `100.802` to `63.411 us/op (-37.1%)`, from
`262,371` to `152 B/op (-99.94%)`, and from `9` to `3 allocs/op`.

The like-for-like shadow heap comparison is Phase 40 report
`2026-07-26T16-08-37-759Z` to Phase 41 report
`2026-07-26T17-01-10-097Z`, both 120 requests at concurrency 8 with 64 KiB
requests and 256 KiB responses. Both use actual Renderer input fingerprint
`9cb881b6298183534bb3d574bb9403af8264269dd20645d6aa01d7fdfcd96f6f`;
heap build fingerprints differ from normal runs because pprof builds explicitly
enable diagnostic symbols. All correctness and cleanup gates pass. Total
cumulative allocation changes from `573,912,033` to `514,160,274 B (-10.4%,
-59.75 MB)`. `GetPostCommonParams` changes from `81,429,814` to `8,923,815 B
(-89.0%)`; the previous `ParseQueryParams 64,697,660 B` and its
`regexp.ReplaceAllStringFunc 40,157,169 B` stacks disappear. `CreateHTTPFlow`
changes from `218,998,084` to `142,686,031 B (-34.8%)`. Post-live heap changes
`276,482,019 -> 272,781,653 B (-1.3%)` and positive live delta changes
`23,763,880 -> 22,299,360 B (-6.2%)`; both remain single-sample diagnostics,
not residency claims.

The formal strict shadow 3+3 comparison is
`body-2026-07-26T16-11-14-215Z` to
`body-2026-07-26T17-06-42-680Z`, with
`comparison-vs-buffered-query-parser.{json,md}` in the candidate. All six runs
complete 120/120; the comparator passes with empty configuration and diagnostic
differences. Candidate normal backend fingerprint is
`446d7a62c4a379820f09` for all three samples. Medians change throughput `+12.1%`,
request p95 `-10.4%`, request/response-to-React `-8.3%/-8.0%`, Renderer drain
`-21.3%`, Electron CPU p95 `-17.4%`, Query p95 `-46.4%`, Yak CPU p95 `+0.1%`
and Yak peak RSS `-0.7%`. First-visible is `+8.9%` slower, max persistence
backlog changes `6 -> 8`, and database-change detection `6 -> 25 ms` with
candidate CV `91.8%`; candidate request/throughput/Long Task distributions also
contain one slow sample. The retained claim is deterministic parser allocation
reduction with no stable product-level regression, not that every UI metric
improved.

The current canary slow-consumer matrix is
`body-2026-07-26T17-13-20-728Z`. Its 800-request fixed-rate case passes as 184
direct plus 616 fallback rows with three recovery entries/completions. Its
240-request 64/256 KiB case transfers 75 MiB of bodies and passes as 91 direct
plus 133 fallback rows with one recovery; all Gap, sequence, duplicate,
out-of-order and cleanup gates remain zero. Final default canary smoke
`body-2026-07-26T17-17-53-978Z` completes 1,000/1,000 at `200.10 req/s` with
11 direct batches, zero Query/fallback/protocol errors, request-to-React `489 ms`,
first-visible `41 ms`, maximum visible backlog `36`, and SQLite queue/write p95
`1/1 ms`. No GORM, database pool, schema, protobuf or frontend code changes are
part of this phase.

## Owned Request Dump And Decoded Packet Handoff

The Phase 41 heap split the remaining `bytes.Clone` allocation into three
different ownership classes. The response returned by an external hijack handler
still needs its independent snapshot, and unencoded plain request/response caches
still need isolation from plugin in-place mutation. The HTTP proxy request dump,
however, is freshly allocated inside `hijackRequestHandler`, immediately stored
in the same request context, and then consumed from that context by the rest of
the chain. Cloning that packet created a second full request with no independent
owner. `dumpRequestToBareContext` now transfers the dump directly through the
narrow owned setter. The public cloning setter and parser/external-input paths
retain their previous behavior.

`DeletePacketEncodingWithOwnership` also exposes whether successful
dechunk/content decoding produced an independently owned packet. MITM plain
caches use the owned setters only in that transformed branch. A conservative or
unencoded no-op still aliases the wire packet and therefore still goes through
the cloning setter. Tests mutate the original unencoded packet to prove cache
isolation, and separately prove pointer identity when gzip decoding creates a
new packet. This optimization is relevant to normal compressed browser traffic,
while the existing uncompressed end-to-end matrix exercises the request-dump
handoff.

Across five paired 256 KiB request-dump samples, the cloned-context path has
median `92.878 us/op`, `541,207 B/op`, and `19 allocs/op`; the owned handoff has
`48.778 us/op (-47.5%)`, `270,851 B/op (-50.0%)`, and `18 allocs/op`. Across five
paired 256 KiB gzip response decode/cache samples, the cloned decoded packet has
median `343.930 us/op`, `1,769,296 B/op`, and `74 allocs/op`; owned decoded cache
handoff has `302.931 us/op (-11.9%)`, `1,498,949 B/op (-15.3%)`, and `73
allocs/op`. Focused ownership tests and race pass, as do full `common/crep`
(`21.175 s`), full `common/utils/lowhttp/...` (main package `185.371 s`), and all
MITMV2 MUSTPASS tests (`199.392 s`).

The like-for-like shadow heap reports are Phase 41
`2026-07-26T17-01-10-097Z` and Phase 42 `2026-07-26T18-07-33-609Z`, both 120
requests at concurrency 8 with 64 KiB requests and 256 KiB responses. The exact
`SetBareRequestBytes <- hijackRequestHandler` allocation changes from `12.33 MB`
to absent. Global `bytes.Clone` changes from `82.87` to `64.77 MB (-21.8%)`;
context cloning changes from `54.08` to `40.45 MB`. Total sampled allocation is
noisy in the opposite direction, `514,160,274 -> 523,742,274 B (+1.9%)`, while
post-live heap changes `272,781,653 -> 284,654,737 B (+4.4%)` and positive live
delta changes `22,299,360 -> 25,609,142 B (+14.8%)`. The retained claim is the
removed caller-level duplicate, not lower total or resident memory from a single
profile sample.

The formal strict shadow 3+3 comparison is
`body-2026-07-26T17-06-42-680Z` to
`body-2026-07-26T18-10-31-124Z`; candidate artifacts are
`comparison-vs-request-dump-owned-transfer.{json,md}`. All six runs complete
120/120 and the comparator passes with no configuration, diagnostic, body,
database, stream, virtual-scroll, or cleanup differences. Medians change
throughput `+0.9%`, request p95 `+2.2%` slower, request/response-to-React
`+0.4%/+3.6%` slower, Yak CPU p95 `-0.3%`, and Yak peak RSS `-3.0%`. Renderer
drain changes `539 -> 879 ms` and Query round-trip p95 `48.2 -> 64.3 ms`; the
former baseline spans `273..866 ms`, and neither metric has a direct caller in
this ownership-only backend change, but both remain explicit risks rather than
being hidden behind the passing comparator.

An additional canary body-scaling matrix
`body-2026-07-26T17-55-20-869Z` passes all four 120-request small, 64 KiB request,
256 KiB response, and 64/256 KiB bidirectional cases. The final fixed-rate canary
`body-2026-07-26T18-16-26-961Z` passes 1,000/1,000 at `199.66 req/s`: 11 direct
batches, 1,000 direct rows, zero Query/fallback/protocol error, maximum visible
backlog 32, zero final backlog, request-to-React `488 ms`, first-visible `55 ms`,
and SQLite queue/write p95 `1/1 ms`. No GORM, database pool, protobuf, schema, or
frontend product code is changed in this phase. The next selection remains
heap-driven; the 31--39 MB text-transform allocation is a candidate only after
an ASCII/charset semantic oracle and compressed-response end-to-end coverage.

## Validated UTF-8 Identity Handoff And Compressed E2E

Phase 43 first extends the Electron harness instead of inferring compressed
response behavior from an identity-body matrix. Harness version 9 supports an
explicit `responseContentEncoding` of `identity` or `gzip`. The loopback target
precompresses the deterministic body once, the producer verifies the encoding
header and exact wire byte count, and the post-window detail oracle verifies the
decoded body persisted by MITM. Encoding is part of matrix comparison identity,
so an identity run cannot accidentally serve as the baseline for a gzip run.
The initial 120-request smoke passes with a 262,144-byte decoded body and a
318-byte gzip body for every response.

The compressed heap baseline `2026-07-26T18-33-30-456Z` attributes
`23,984,547 B` to
`FixHTTPResponsePacket -> TryUTF8Convertor -> encoding.Decoder.Bytes ->
transform.Bytes`. `mimecharset.FromPlain` had already validated these JSON
bodies as UTF-8; decoding them through the UTF-8 identity decoder only created
a body-sized copy. The candidate returns the same bytes with the same
successful-conversion signal when the detected charset is `utf-8`/`utf8`.
Explicit charset handling, HTML/meta rewriting, GBK/GB18030 conversion, unknown
charsets and the replacement-rune guard remain on their prior paths. Tests
compare the old detector/decoder result for ASCII and non-ASCII UTF-8, verify
the original backing storage is handed off, retain explicit-UTF-8 behavior, and
prove that a body containing U+FFFD is still rejected.

Across five paired 256 KiB samples, the legacy identity decoder has median
`472.510 us/op`, `262,188 B/op`, and `3 allocs/op`; the validated UTF-8 handoff
has median `195.724 us/op (-58.6%)`, `0 B/op`, and `0 allocs/op`. Focused tests
and race pass, as do full codec, full `common/utils/lowhttp/...` (main package
`186.809 s`) and all MITMV2 MUSTPASS tests (`205.534 s`). There is no public
API, protobuf, database representation, connection-pool or GORM change.

The like-for-like gzip heap candidate is `2026-07-26T18-44-00-038Z`. Relative
to the baseline, the `23,984,547 B` transform stack disappears and total sampled
allocation changes `550,909,308 -> 515,346,459 B (-6.5%)`. Positive live delta
changes `24,520,487 -> 22,783,096 B (-7.1%)`; post-live heap changes
`274,321,676 -> 276,335,780 B (+0.7%)`, which is treated as single-sample
noise rather than a residency regression.

The formal gzip shadow 3+3 comparison is
`body-2026-07-26T18-36-06-289Z` to
`body-2026-07-26T18-48-25-177Z`, with
`comparison-vs-utf8-identity-decoder.{json,md}` in the candidate. All six runs
complete 120/120 and the comparator passes with no case-config or diagnostic
differences. Backend conversion p95 changes `0.706 -> 0.546 ms (-22.7%)`, and
per-flow conversion p95 `16.99 -> 8.00 us (-52.9%)`. Throughput is `+1.1%`,
request p95 `-1.4%`, Renderer drain `-15.6%`, Yak CPU p95 `-0.2%`, and peak Yak
working set `+1.4%`. Backend Query p95 `4.289 -> 7.398 ms (+72.5%)`, Electron CPU
p95 `+27.3%`, and Long Task `56 -> 61 ms` are retained as short-run risks;
Query has a `34.978 ms` outlier and no direct caller in this backend-only
identity-decode change, so no blanket UI-speed claim is made.

The larger bounded canary is matrix `body-2026-07-26T19-08-22-255Z`, report
`2026-07-26T19-08-22-892Z`: 400 gzip responses at 100 requests/s, concurrency
12, and 256 KiB decoded bodies (100 MiB decoded total). It completes 400/400 at
`99.48 req/s` with `99.99 req/s` dispatch, ten direct batches, 400 direct rows,
zero Query/fallback/gap/sequence-gap/duplicate/out-of-order/unavailable events,
maximum visible backlog 38, zero final backlog, and CPU recovery in `2021 ms`.
The exact 318-byte wire and 262,144-byte detail-body oracle still passes, and no
Electron or Yak process remains after cleanup.

## Bounded Gzip Trailer Size Hint

The Phase 43 compressed heap still attributed `287,915,925 B` directly to
`io.ReadAll`; `_decodeBody` accumulated `296,030,743 B`. The same 31.25 MiB of
decoded response bodies is intentionally materialized twice: once while fixing
the transport response and once for the isolated plain plugin cache. Reusing one
packet for both owners would couple stored wire/fixed data to plugin mutation,
so this phase does not take that higher-risk shortcut. Instead, gzip decoding
uses the RFC 1952 ISIZE trailer only as an initial-capacity hint.

ISIZE is untrusted until the gzip reader reaches EOF and validates the stream.
Speculative allocation is therefore capped independently at 1 MiB, while the
existing 32 MiB decoded-body limit remains enforced by a limited reader. The
hint includes one `bytes.MinRead` region so `bytes.Buffer.ReadFrom` can observe
EOF without a final geometric grow. A wrong, wrapped or last-member-only hint
can only cause bounded growth; concatenated members, checksum failure, reader
failure, exact-limit behavior and conservative return of the original compressed
packet retain their previous semantics. There is no public API, wire, protobuf,
database, ownership, GORM or frontend product change.

Across five paired 256 KiB samples, the legacy limited `io.ReadAll` path has
median `254.150 us/op`, `1,227,316 B/op` and `28 allocs/op`; the bounded hint path
has median `90.785 us/op (-64.3%)`, `311,595 B/op (-74.6%)` and `8 allocs/op
(-71.4%)`. Exact/max-minus-one limits, a malicious `0xffffffff` trailer,
concatenated members, a corrupted checksum and an injected reader error are
covered. Focused race passes, as do full `common/utils/lowhttp` (`92.900 s`) and
all MITMV2 MUSTPASS tests (`197.562 s`).

The like-for-like gzip heap reports are Phase 43
`2026-07-26T18-44-00-038Z` and Phase 44
`2026-07-26T19-36-18-312Z`, each 120 requests at concurrency 8 with 256 KiB
decoded gzip responses. Total sampled allocation changes from `515,346,459` to
`293,335,322 B (-43.1%, -222.01 MB)`. The old `io.ReadAll 287,915,925 B` node
disappears; `_decodeBody` changes `296,030,743 -> 73,673,653 B (-75.1%)`,
`ContentEncodingDecode 154,765,326 -> 38,113,828 B (-75.4%)`, and
`DeletePacketEncodingWithOwnership 177,500,542 -> 69,635,267 B (-60.8%)`.
The new `_readAllLimitedWithHint` accumulates `62,404,938 B`, approximately the
two required 120 x 256 KiB outputs. Allocation attribution moves into
`bytes.growSlice`, so that flat node grows while total allocation falls; it is
not interpreted in isolation. Positive live delta changes `22,783,096 ->
13,935,770 B (-38.8%)`, and post-live heap changes `276,335,780 -> 274,687,524
B (-0.6%)`; both remain single-sample diagnostics rather than residency claims.

The formal like-for-like shadow comparison is
`body-2026-07-26T18-48-25-177Z` to
`body-2026-07-26T19-46-59-578Z`, with
`comparison-vs-gzip-size-hint.{json,md}` in the candidate. All six runs complete
120/120 and the comparator passes with no case-config or diagnostic differences.
Candidate medians change throughput `+24.6%`, request p95 `-7.3%`, first visible
`316 -> 186 ms (-41.1%)`, Query round trip p95 `-63.8%`, Yak drain CPU p95
`-43.1%`, and peak Yak working set `-4.9%`. Database catch-up/drain are `+38.2%`
and `+26.0%` slower, maximum visible backlog is `80 -> 104 (+30.0%)`, and
Electron CPU p95 is `+4.8%`; these short-run reverse signals remain explicit,
so the retained claim is deterministic gzip allocation reduction with favorable
mixed end-to-end evidence, not universal UI improvement.

The larger current canary is matrix `body-2026-07-26T19-52-36-960Z`, report
`2026-07-26T19-52-37-603Z`. It completes 400/400 256 KiB gzip responses at
`99.69 req/s` against a 100 requests/s target, with request p95 `71.72 ms`, nine
direct batches, 400 direct rows, zero Query/fallback/gap/sequence-gap/duplicate/
out-of-order/unavailable events, maximum persistence/visible backlog `2/14`,
zero producer-stop and final backlog, and CPU recovery in `2019 ms`. Relative to
the previous single canary, request latency, backlog and Yak working set are
directionally lower while database catch-up is higher, but the matrix comparator
correctly refuses a formal A/B because that earlier baseline has only one
repetition. These values are therefore absolute canary evidence, not a statistical
performance conclusion.

## Owned Decoded Body Packet Fold

The Phase 44 gzip heap showed that each independently allocated decoded body was
then copied once more merely to prefix the rewritten HTTP headers. The decoded
slice already reserves `bytes.MinRead` slack for EOF detection, and the rewritten
header is substantially smaller than that slack. Phase 45 therefore adds a
private ownership-only packet rebuild path: when the caller has proved that the
body is independently owned and its spare capacity can hold the header, the body
is shifted right within the same allocation and the header is written into the
prefix. Insufficient-capacity and borrowed inputs retain the allocating path.
The public `ReplaceHTTPPacketBodyEx` API remains non-consuming, and the public
`FixHTTPResponse` body-isolation contract is unchanged.

Tests prove exact packet bytes, same-backing reuse for owned capacity, allocating
fallback for insufficient capacity, non-mutation and result isolation for the
public borrowed API, and preservation of the compressed wire packet for both
plain-cache and fixed-response callers. Across five paired 256 KiB gzip decode
and packet-rebuild samples, the copy path has median `164.767 us/op`, `583,286
B/op`, and `39 allocs/op`; the owned fold has `117.762 us/op (-28.5%)`, `312,691
B/op (-46.4%)`, and `36 allocs/op`. Focused race passes, as do full
`common/utils/lowhttp` (`183.237 s`) and all 62 MITMV2 MUSTPASS tests (`195.011
s`).

The race gate also exposed pre-existing MITMV2 session-lifecycle races unrelated
to the allocation candidate. Concurrent streams could overwrite the global
plugin caller/channel, causing a race and a possible close of the wrong or
already-closed channel. Registration, snapshot loading and conditional
unregistration are now protected by an RW mutex, and each session closes only
its captured notification channel. Async plugin load, dropped-flow persistence,
response-mirror flow creation and HookColor persistence now use callback-local
error variables instead of the MITMV2 outer `err`. Concurrent lifecycle tests
and the real manual-hijack race test pass.

The like-for-like gzip heap reports are Phase 44
`2026-07-26T19-36-18-312Z` and Phase 45
`2026-07-26T20-30-29-674Z`. Total sampled allocation changes from `293,335,322`
to `218,175,762 B (-25.6%, -75.16 MB)`. The old
`ReplaceHTTPPacketBodyEx 66,955,463 B` caller disappears; `bytes.growSlice`
changes `129,884,753 -> 61,440,580 B (-52.7%)`, and
`DeletePacketEncodingWithOwnership` changes `69,635,267 -> 30,071,770 B
(-56.8%)`. The required two decoded outputs remain visible under
`_readAllLimitedWithHint` at `60,391,876 B`. Post-GC live heap changes
`274,687,524 -> 269,615,985 B (-1.85%)`, while positive live delta changes
`13,935,770 -> 22,274,476 B (+59.8%)`; these forced-GC single samples do not
support a residency claim.

The formal shadow 3+3 comparison is
`body-2026-07-26T19-46-59-578Z` to
`body-2026-07-26T20-39-54-761Z`, with
`comparison-vs-owned-packet-fold.{json,md}` in the candidate. The comparator
passes with no case-config or diagnostic differences, and all six runs complete
120/120 with exact body, database, stream and cleanup gates. Candidate medians
change throughput `+14.0%`, Electron CPU p95 `-9.2%`, Yak drain CPU p95 `-8.3%`,
request-to-React `-1.9%`, and Yak peak working set approximately `-0.02%`.
Request p95 is `+3.8%` slower, first-visible `+17.7%`, database catch-up/drain
`+22.1%/+25.9%`, Renderer drain `+23.2%`, and Query round trip `+88.0%`; the
candidate distributions overlap or remain short-run noisy, so the retained
claim is the deterministic allocation removal rather than universal UI speed.

The larger canary is matrix `body-2026-07-26T20-45-36-491Z`, report
`2026-07-26T20-45-37-147Z`. It completes 400/400 256 KiB gzip responses at
`99.69 req/s` against a 100 requests/s target, with request p95 `80.51 ms`, nine
direct batches, 400 direct rows, zero Query/fallback/gap/sequence-gap/duplicate/
out-of-order/unavailable events, maximum persistence/visible backlog `6/48`,
zero final backlog, and CPU recovery in `2021 ms`. No protobuf, database schema,
GORM, connection-pool or frontend product change is part of this phase.

## Quoted HTTP Packet Output Handoff

HTTPFlow storage has historically persisted request and response packets as the
exact output of `strconv.Quote`, and readers use `strconv.Unquote`. Phase 45's
heap attributed `79,798,762 B` to `quoteHTTPPacket`. The standard implementation
first builds a byte buffer with conservative capacity and then copies the entire
quoted result into a string. Phase 46 continues to use the standard library's
`strconv.AppendQuote` encoder and the same conservative capacity, but transfers
that newly allocated, never-again-mutated byte buffer to an immutable string.
The input packet remains a read-only view and is kept alive through encoding.
This changes neither quoted bytes nor database TEXT representation.

The semantic oracle covers nil/empty inputs, ordinary HTTP, Unicode, all 256
byte values and invalid UTF-8, then mutates the input to prove result isolation.
Across five paired 256 KiB HTTP packet samples, the prior read-only-input but
copied-output path has median `1.450 ms/op`, `671,746 B/op`, and `2 allocs/op`;
the output handoff has `1.407 ms/op (-3.0%)`, `401,409 B/op (-40.2%)`, and `1
alloc/op`. Focused race and the full yakit persistence package (`49.375 s`) pass,
as do all 62 MITMV2 MUSTPASS tests (`192.017 s`).

The like-for-like gzip heap reports are Phase 45
`2026-07-26T20-30-29-674Z` and Phase 46
`2026-07-26T21-01-00-184Z`. `quoteHTTPPacket` changes from `79,798,762` to
`53,275,268 B (-33.2%)`; total sampled allocation changes `218,175,762 ->
206,004,354 B (-5.6%)`. `_readAllLimitedWithHint` varies in the opposite
direction, `60,391,876 -> 68,444,126 B (+13.3%)`, obscuring part of the direct
reduction in the single total. Positive live delta changes `22,274,476 ->
14,849,612 B (-33.3%)`; post-GC live heap changes `269,615,985 -> 271,492,940 B
(+0.7%)`. These remain forced-GC single-sample diagnostics.

The formal shadow 3+3 comparison is
`body-2026-07-26T20-39-54-761Z` to
`body-2026-07-26T21-04-57-340Z`, with
`comparison-vs-quote-output-handoff.{json,md}` in the candidate. The comparator
passes with no case-config or diagnostic differences and all six runs complete
120/120. Candidate medians change throughput `+6.6%`, request p95 `-11.9%`,
Yak drain CPU p95 `-37.0%`, Yak peak working set `-2.1%`, Query round trip
`-5.7%`, and first-visible `-1.4%`. Database catch-up/drain are `+15.1%/+7.1%`
slower, Renderer drain `+19.7%`, request-to-React `+6.8%`, and Electron CPU p95
`+2.8%`; these remain explicit short-run risks.

The larger canary is matrix `body-2026-07-26T21-10-40-565Z`, report
`2026-07-26T21-10-41-230Z`. It completes 400/400 at `100.14 req/s` against a
100 requests/s target, with request p95 `64.52 ms`, nine direct batches, 400
direct rows, zero Query/fallback/gap/sequence-gap/duplicate/out-of-order/
unavailable events, maximum persistence/visible backlog `2/48`, zero final
backlog, and CPU recovery in `2020 ms`.

The remaining approximately 30 MB SQLite bind allocation is in the external
driver's string-to-byte conversion immediately before `SQLITE_TRANSIENT` copies
the value. Passing bytes through GORM would bind them as SQLite BLOB and can
change TEXT affinity, comparison and query behavior; wrapping them in a CAST
would add dialect-specific storage semantics. Neither workaround is accepted
without a driver-level contract, and this phase does not modify or publish the
SQLite driver or GORM fork.

## Reused Gzip Reader And Flate State

Phase 46's heap still attributed about 9 MB to repeatedly constructing gzip and
flate readers. Phase 47 pools a wrapper containing both `gzip.Reader` and its
own `bytes.Reader`, and reuses the standard library reader through `Reset`.
Release closes the current stream, clears the source, and resets the gzip reader
onto the wrapper's empty source before returning it to the pool. A pooled reader
therefore cannot retain a response packet through the source interface. Decode
errors also release the wrapper, while the existing output limit, ISIZE hint,
EOF/CRC validation, multistream behavior and conservative wire-packet fallback
remain unchanged.

Tests cover reuse after invalid headers and corrupt checksums, valid decoding
after failures, and 32-way concurrent decode. Focused race, full lowhttp
(`183.936 s`) and all 62 MITMV2 MUSTPASS tests (`198.843 s`) pass. Across five
paired 256 KiB gzip samples, a fresh reader has median `98.340 us/op`, `311,595
B/op`, and `8 allocs/op`; the pooled reader has `89.620 us/op (-8.9%)`, `270,388
B/op (-13.2%)`, and `2 allocs/op`.

The like-for-like gzip heap reports are Phase 46
`2026-07-26T21-01-00-184Z` and Phase 47
`2026-07-26T21-30-52-650Z`. Total sampled allocation changes `206,004,354 ->
195,012,858 B (-5.3%)`; the approximately `9,180,204 B` gzip/flate reader
construction stack and `8,653,481 B` dictionary initialization stack disappear.
The required decoded output helper varies `68,444,126 -> 77,167,397 B` in the
opposite direction, while post-GC live heap changes `271,492,940 -> 269,973,499
B (-0.56%)`. Positive live delta changes `14,849,612 -> 21,144,836 B (+42.4%)`;
these single forced-GC samples do not support a residency claim.

The formal shadow 3+3 comparison is
`body-2026-07-26T21-04-57-340Z` to
`body-2026-07-26T21-35-03-174Z`, with
`comparison-vs-gzip-reader-pool.{json,md}` in the candidate. The comparator
passes with no case-config or diagnostic differences and all six runs complete
120/120 with exact wire/detail, database, stream and cleanup gates. Candidate
medians change database catch-up/drain `-21.0%/-18.3%`, Electron CPU p95 `-7.7%`
and Query round trip `-54.1%`. Throughput is `-3.6%`, request p95 `+9.4%`,
first-visible `+12.0%`, Renderer drain `+39.7%`, request-to-React `+1.0%`, and
Yak drain CPU p95 `+71.7%`; distributions remain noisy, so this phase claims
only the deterministic reader-state allocation reduction.

The larger canary is matrix `body-2026-07-26T21-40-43-180Z`, report
`2026-07-26T21-40-43-821Z`. It completes 400/400 256 KiB decoded gzip responses
at `99.38 req/s` against a 100 requests/s target, with request p95 `65.02 ms`,
nine direct batches, 400 direct rows, zero Query/fallback/gap/sequence-gap/
duplicate/out-of-order/unavailable events, maximum persistence/visible backlog
`6/53`, zero producer-stop and final backlog, and CPU recovery in `2022 ms`.
No frontend product, protobuf, database schema, GORM or connection-pool change is
part of this phase.

## Adaptive Quoted Packet Capacity

After the output ownership handoff, `quoteHTTPPacket` still reserved 50% extra
capacity for every packet, matching the standard library's conservative default.
Typical HTTP packets in the gzip matrix need only the two outer quotes plus a
small number of escaped header bytes, so much of that allocation was never used.
Phase 48 samples at most 4 KiB from the packet prefix and suffix and estimates
only the capacity needed by `strconv` escaping. It reserves 12.5% slack for
ordinary text and retains the former 50% slack when the sample is escape-dense
or contains invalid UTF-8. `strconv.AppendQuote` remains the sole encoder, and an
underestimate merely triggers normal slice growth; quoted bytes and database
TEXT semantics cannot change.

The semantic oracle continues to cover nil/empty, ordinary HTTP, Unicode, every
byte value, invalid UTF-8 and input mutation. Capacity tests cover printable,
Unicode, control-byte and invalid-UTF-8 inputs. In five paired 256 KiB printable
HTTP samples, conservative versus adaptive capacity has median `1.417 -> 1.432
ms/op (+1.0%)`, `401,409 -> 303,104 B/op (-24.5%)`, and one allocation in both
paths. Control-heavy input keeps six allocations and changes allocated bytes by
only `+0.18%`; the all-byte cycle keeps four allocations and changes bytes by
`+0.35%`. Their median times change about `+1.9%` and `+0.6%`, respectively.
Focused race, the full yakit persistence package (`85.610 s`) and all 62 MITMV2
MUSTPASS tests (`191.504 s`) pass.

The like-for-like gzip heap reports are Phase 47
`2026-07-26T21-30-52-650Z` and Phase 48
`2026-07-26T21-59-36-191Z`. `quoteHTTPPacket` changes `47,272,421 -> 35,898,822
B (-24.1%)`; total sampled allocation changes `195,012,858 -> 178,409,628 B
(-8.5%)`, and `bytes.growSlice` changes `77,167,397 -> 70,163,802 B (-9.1%)`.
Positive live delta changes `21,144,836 -> 18,689,472 B (-11.6%)`; post-GC live
heap changes `269,973,499 -> 275,986,585 B (+2.2%)`, so there is no residency
claim from these single forced-GC samples.

The formal shadow 3+3 comparison is
`body-2026-07-26T21-35-03-174Z` to
`body-2026-07-26T22-09-02-145Z`, with
`comparison-vs-adaptive-quote-capacity.{json,md}` in the candidate. The
comparator passes with no case-config or diagnostic differences and all six runs
complete 120/120. Candidate medians change database catch-up/drain
`-25.7%/-20.6%`, Renderer drain `-44.0%`, maximum visible backlog `-20.9%`, Yak
peak working set `-3.7%`, and request-to-React `-4.3%`. Throughput is `-4.1%`,
request p95 `+4.7%`, Query round trip `+207.6%`, Electron CPU p95 `+8.2%`, Yak
drain CPU p95 `+12.7%`, and Long Task changes from zero to `53 ms`; these mixed
short-run results restrict the claim to the deterministic quote allocation
reduction.

The larger canary is matrix `body-2026-07-26T22-14-43-186Z`, report
`2026-07-26T22-14-43-829Z`. It completes 400/400 256 KiB decoded gzip responses
at `100.12 req/s` against a 100 requests/s target, with request p95 `71.57 ms`,
nine direct batches, 400 direct rows, zero Query/fallback/gap/sequence-gap/
duplicate/out-of-order/unavailable events, maximum persistence/visible backlog
`4/50`, zero producer-stop and final backlog, and CPU recovery in `2018 ms`.
There is no frontend product, protobuf, database schema, GORM or driver change in
this phase.

## ASCII Quoted Packet Fast Path

The fixed-rate Phase 48 CPU profile
`2026-07-26T22-25-45-716Z` samples `6.15 CPU s` over five seconds. Runtime memory
and GC account for `2.07 s` flat, while `quoteHTTPPacket` is `1.16 s` cumulative
and the standard `strconv` quote implementation is `0.74 s`. The stored packet
format is byte-oriented but `strconv` must decode every rune and call Unicode
printability logic even when the entire HTTP packet is ASCII.

Phase 49 keeps `strconv.AppendQuote` as the complete fallback for any non-ASCII
packet. A pure-ASCII packet uses a byte encoder with exactly the standard
escapes for quotes, backslashes, named controls, other control bytes and DEL.
The existing prefix/suffix sample avoids a full ASCII probe for commonly placed
Unicode, and the ASCII encoder aborts to the same output buffer if it encounters
a high byte outside the sample. The full encoder oracle now explicitly includes
all 128 ASCII values in addition to all 256 bytes, Unicode, invalid UTF-8 and
input mutation. Database bytes and `strconv.Unquote` compatibility are unchanged.

Across five paired 256 KiB ASCII HTTP samples, the adaptive `strconv` path has
median `1.408 ms/op`, `303,104 B/op`, and one allocation; the ASCII path has
`216.288 us/op (-84.6%)`, `303,105 B/op`, and one allocation. The deliberately
adversarial case with a Unicode rune only at the end changes `1.413 -> 1.420
ms/op (+0.5%)`, with identical allocation count and bytes. Focused race, the full
yakit persistence package (`56.103 s`) and all 62 MITMV2 MUSTPASS tests (`199.129
s`) pass.

The like-for-like five-second fixed-rate CPU reports are Phase 48
`2026-07-26T22-25-45-716Z` and Phase 49
`2026-07-26T22-40-14-236Z`. Total samples change `6.15 -> 5.30 CPU s (-13.8%)`
and average CPU `123% -> 106%`; `quoteHTTPPacket` changes `1.16 -> 0.38 s
(-67.2%)`. The `0.74 s` standard quote stack disappears and `appendQuotedASCII`
is `0.11 s`. Runtime memory/GC flat samples change `2.07 -> 1.86 s (-10.1%)`,
while `scanobject` changes `1.57 -> 1.37 s (-12.7%)`. Gzip decode, GORM create
and SQLite bind also sample lower, but those are downstream single-profile
directions rather than separate claims. Both diagnostic runs complete 400/400
with the same fixed-rate configuration and all correctness gates.

The formal shadow 3+3 comparison is
`body-2026-07-26T22-09-02-145Z` to
`body-2026-07-26T22-44-22-972Z`, with
`comparison-vs-ascii-quote-fast-path.{json,md}` in the candidate. The comparator
passes with no case-config or diagnostic differences and all six runs complete
120/120. Candidate medians change throughput `+44.8%`, request p95 `-20.8%`,
Electron CPU p95 `-12.1%`, Yak drain CPU p95 `-30.3%`, Query round trip `-25.2%`,
first-visible `-11.2%`, and Long Task `53 -> 0 ms`. Renderer drain is `+60.7%`,
maximum visible backlog `+32.2%`, Yak CPU p95 `+2.4%`, Yak working set `+3.6%`,
and request-to-React `+5.7%`; the uncapped producer exposes more rows sooner, so
the fixed-rate canary is used to separate that downstream pressure.

The non-diagnostic fixed-rate canary is matrix
`body-2026-07-26T22-50-06-164Z`, report
`2026-07-26T22-50-06-812Z`. It completes 400/400 at `100.15 req/s` against a 100
requests/s target, with request p95 `42.55 ms`, database/Renderer drain `291/329
ms`, nine direct batches, zero Query/fallback/gap/sequence-gap/duplicate/
out-of-order/unavailable events, maximum persistence/visible backlog `4/18`,
zero final backlog, and CPU recovery in `2019 ms`. The preceding Phase 48 canary
had request p95 `71.57 ms`, database/Renderer drain `358/392 ms`, and maximum
visible backlog `50`; these are directionally consistent single canaries, not a
formal repeated comparison.

An intermediate attempt to remove the gzip output allocation size-class slack
was rejected and fully reverted. Even after pooling its EOF scratch, reversed
seven-sample order showed exact-hint reading at median `91.35 us/op` versus
`82.95 us/op (+10.1%)` for the existing buffer path, in exchange for only `3.0%`
fewer allocated bytes and no allocation-count reduction. The two independently
owned decoded outputs therefore remain unchanged. This phase has no frontend
product, protobuf, schema, GORM or SQLite driver change.

## Low-Density Quoted Packet Capacity

Phase 49 removes most quote CPU but still allocates the Phase 48 one-eighth
slack for ordinary HTTP packets. A 256 KiB response therefore occupies the
303,104-byte size class even though the encoded packet normally needs only the
packet bytes, outer quotes, and a small number of CRLF escapes.

Phase 50 divides the same bounded 4 KiB sample across the packet prefix, middle,
and suffix. Escape density at or below 1/64 uses one-sixty-fourth slack, density
above 1/8 retains one-half slack, and the middle range retains one-eighth slack.
The middle sample prevents a binary or escape-dense body center from being
classified only from text-like headers and suffixes. Capacity remains only a
hint: the ASCII encoder and `strconv.AppendQuote` fallback still produce the
same bytes, and slice growth remains correct if any sample misses an adversarial
region. Tests cover printable, light-escape, Unicode, medium-escape, control,
middle-control and invalid UTF-8 inputs, plus a non-ASCII body center.

Across five 256 KiB ASCII samples, the Phase 49 path changes from median
`214.735 us/op / 303,105 B/op / 1 alloc` to
`206.292 us/op (-3.9%) / 270,336 B/op (-10.8%) / 1 alloc`. A paired late-Unicode
check has effectively identical medians for direct standard encoding and the
sampled fallback (`1.402 ms/op` each), with one allocation. Focused race, the
full yakit persistence package (`68.069 s`, `876,944 KiB` peak RSS), and all 62
MITMV2 MUSTPASS tests (`191.994 s`, `3,286,952 KiB` peak RSS) pass without swap.

The original 120-flow heap comparison was deliberately rejected as
underpowered: the historical profile was not configuration-compatible, and a
new same-generation pair still showed sampling noise opposite to the
deterministic microbenchmark. The bounded heap A/B was therefore increased to
400 gzip responses at 100 requests/s, or 100 MiB decoded. The one-eighth control
is report `2026-07-26T23-48-02-204Z`; the one-sixty-fourth candidate is
`2026-07-26T23-45-50-519Z`. Both complete 400/400 with identical configuration
and cleanup. `quoteHTTPPacket` allocation changes
`125,645,877 -> 109,900,830 B (-12.5%)` and total allocation changes
`577,608,477 -> 560,017,411 B (-3.0%)`; `bytes.growSlice +0.3%` and SQLite bind
`-0.2%` are effectively flat. Positive-live `-3.1%` and post-live `-0.2%` remain
forced-GC diagnostics, not resident-memory claims.

The like-for-like Phase 49 and Phase 50 five-second CPU profiles are
`2026-07-26T22-40-14-236Z` and `2026-07-26T23-50-40-369Z`.
`quoteHTTPPacket` changes `380 -> 290 ms (-23.7%)` and the ASCII encoder changes
`110 -> 60 ms`; total samples change `5.30 -> 5.45 CPU s (+2.8%)`, while runtime
memory/GC changes `1.86 -> 2.20 s` and `scanobject 1.37 -> 1.63 s`. This single
profile confirms the target CPU direction but explicitly does not claim total
CPU or GC improvement.

The non-profiled fixed-rate decision gate uses an A/B/A sandwich, three strictly
sequential 400-flow runs per group. Candidate A1 is
`body-2026-07-26T23-53-13-414Z`, one-eighth control B is
`body-2026-07-26T23-58-57-106Z`, and candidate A2 is
`body-2026-07-27T00-05-18-037Z`. Both comparator outputs pass with no case or
diagnostic differences and all nine runs complete 400/400. The first candidate
comparison reports request p95 `+50.9%`, Electron CPU p95 `+18.5%`, database
drain `-14.8%`, and Renderer drain `-14.2%`; the post-control candidate reverses
those directions to `-27.7%`, `-0.2%`, `+27.2%`, and `+24.1%`. This proves those
short-run metrics follow run order rather than the capacity branch. Stable
medians are about 100 requests/s, request-to-React `-0.2%` in both candidate
groups, response-to-React `-1.4%/+1.2%`, and zero Long Task duration. The phase
therefore claims only the measured quote and total-allocation reductions and
records full end-to-end variance. There is no frontend product, protobuf,
schema, database connection, GORM, or SQLite driver change.

## Canonical HTTP Packet View Fast Path

The Phase 50 heap profile still attributed `29,363,868 B` cumulatively to
`splitHTTPPacketEx`. Even the read-only Body API parsed every header through a
pooled `bufio.Reader`, allocated a string for every line, and joined those
strings through a `strings.Builder`. Several MITM callers need only a Body view
or the request start-line callback, but still paid that reconstruction cost.

Phase 51 recognizes only the canonical CRLF form when no per-header hook is
installed. It validates the untrimmed first line, every header line, request
whitespace termination, response type and `Content-Length`, invokes the existing
request callback with the existing parser, copies the complete header once into
the returned immutable string, and returns the Body as the existing read-only
view. LF-only, prefixed, trimmed, extra-CR, whitespace-terminated request and
otherwise noncanonical packets fall back to `splitHTTPPacketEx`. APIs that clone
the Body and APIs with per-header hooks are unchanged.

The compatibility suite compares the fast path directly with the legacy parser
for requests, HTTP/RTSP responses, folded headers, mixed-case Content-Length,
blank and binary bodies, malformed line endings and prefix normalization. It
also locks header independence, Body aliasing, callback invocation/abort and
concurrent behavior. A bounded 15-second differential fuzz run executes 81,845
inputs with no mismatch. Focused race, full `common/utils/lowhttp` (`188.127 s`,
`487,104 KiB` peak RSS), full `common/yakgrpc/yakit` (`49.381 s`, `876,184 KiB`),
and all 62 MITMV2 MUSTPASS tests (`190.365 s`, `3,315,576 KiB`) pass without
swap.

Across five paired 256 KiB samples, callback-free View changes from legacy
median `809.7 ns/op / 618 B/op / 16 allocs` to
`172.6 ns/op (-78.7%) / 96 B/op (-84.5%) / 1 alloc`. The request start-line
callback form used by `CreateHTTPFlow` changes from
`731.2 -> 206.6 ns/op (-71.7%)`, `512 -> 104 B/op (-79.7%)`, and
`15 -> 2 allocs`.

The like-for-like 400-flow gzip heap reports are Phase 50
`2026-07-26T23-45-50-519Z` and Phase 51
`2026-07-27T00-33-58-280Z`. Both complete 400/400 at 100 requests/s with exact
318-byte wire and 262,144-byte decoded Body checks. Total sampled allocation
changes `560,017,411 -> 552,384,724 B (-1.36%)`. Legacy
`splitHTTPPacketEx` cumulative allocation changes
`29,363,868 -> 17,303,416 B (-41.1%)`; including the new canonical path's
`3,146,448 B`, total split implementation allocation is about
`20,449,864 B (-30.4%)`. `strings.Builder.grow` changes
`10,487,924 -> 4,719,112 B (-55.0%)` and `bufio.NewReaderSize` changes
`4,737,046 -> 2,105,353 B (-55.6%)`. Positive-live changes `-39.0%`, while
post-GC live changes `+1.9%`; both remain forced-GC diagnostics.

The same five-second CPU reports are Phase 50
`2026-07-26T23-50-40-369Z` and Phase 51
`2026-07-27T00-47-15-795Z`. Total samples change
`5.45 -> 5.52 CPU s (+1.3%)`, while `scanobject` changes
`1.63 -> 1.32 s (-19.0%)`; the split stack is too small and too variable in a
single profile to claim total CPU improvement. The candidate still completes
400/400 with zero final backlog and CPU recovery. This phase therefore claims
the deterministic parser and allocation reductions only. It changes no frontend
product code, protobuf, database schema, connection pool, GORM, or SQLite driver.

## Canonical Header Hook View Fast Path

Phase 52 extends the canonical packet View path to the per-Header hooks used by
MITM response fixing and HTTPFlow construction. The Phase 51 implementation also
exposed a compatibility edge: it could invoke a request start-line callback,
discover a noncanonical later Header, then fall back to the legacy parser and
invoke the callback again. Phase 52 validates the complete Header block before
performing any callback or hook. It then creates the immutable returned Header
string once and passes slices of that string to the hooks, so retained hook
values cannot alias a mutable input packet and no per-line byte-to-string copy is
needed. Response folding, LF-only packets and other noncanonical forms fall back
before any hook executes. Body-cloning APIs and public signatures are unchanged.

Static request/response/folded/LF-only cases compare returned Header, Body and
the complete hook sequence with the legacy parser. The request callback oracle
also verifies that a malformed later Header causes exactly one invocation. A
15-second differential fuzz run compares the no-hook, Header-hook and request
callback forms over 75,911 executions without a mismatch. Focused race passes
at `538,760 KiB` peak RSS. Full `common/utils/lowhttp` (`182.475 s`, `488,080
KiB`), full `common/yakgrpc/yakit` (`52.822 s`, `876,888 KiB`) and all 62
MITMV2 MUSTPASS tests (`195.540 s`, `3,316,704 KiB`) pass without swap.

Across five paired 256 KiB samples, the Header-hook View changes from legacy
median `792.5 ns/op / 618 B/op / 16 allocs` to
`211.0 ns/op (-73.4%) / 96 B/op (-84.5%) / 1 alloc`. The request callback form
is `730.0 -> 191.8 ns/op (-73.7%)`, `512 -> 80 B/op (-84.4%)`, and
`15 -> 1 alloc`; the callback-free form remains `789.1 -> 174.6 ns/op
(-77.9%)`, `618 -> 96 B/op`, and `16 -> 1 alloc`.

The like-for-like 400-flow gzip heap reports are Phase 51
`2026-07-27T00-33-58-280Z` and Phase 52
`2026-07-27T01-14-29-233Z`; the candidate matrix is
`body-2026-07-27T01-14-28-592Z`. Both complete 400/400 at 100 requests/s with
318-byte wire and 262,144-byte decoded response checks. Legacy
`splitHTTPPacketEx` cumulative allocation changes
`17,303,416 -> 8,913,936 B (-48.5%)`. Including the canonical path, total split
implementation allocation changes from about `20,449,864 -> 11,535,864 B
(-43.6%)`. Total sampled allocation changes
`552,384,724 -> 510,155,443 B (-7.6%)`; because `bytes.growSlice` also samples
lower by about 25.5 MB, only the directly attributed split reduction is claimed.
Positive-live changes `14,238,688 -> 18,189,280 B (+27.7%)` and post-GC live
changes `270,707,774 -> 284,827,826 B (+5.2%)`, so there is explicitly no
resident-memory improvement claim.

The Phase 52 five-second CPU report is `2026-07-27T01-20-18-423Z`, matrix
`body-2026-07-27T01-20-17-717Z`, compared with Phase 51
`2026-07-27T00-47-15-795Z`. Total samples change `5.52 -> 5.15 CPU s (-6.7%)`
and average CPU `110.4% -> 103.0%`, but the split functions remain below the
sampling threshold and `scanobject` flat samples change `1.32 -> 1.38 s`.
Consequently this phase does not claim global CPU improvement. Heap and CPU
runs both complete 400/400 with nine direct batches, no Query/fallback/gap/
sequence/duplicate/out-of-order/unavailable errors, zero final backlog and CPU
recovery. No frontend product code, protobuf, schema, database configuration,
GORM or SQLite driver is changed.

## Allocation-Bounded Response Header Classification

Phase 53 removes the repeated whole-line lowercase operation from
`fixHTTPResponse` Header classification. The previous hook lowercased every
Header once for Content-Type detection and then again for transfer/content
encoding detection. The new state parser uses ASCII case-folded prefix checks,
keeps the original Content-Type value, and lowercases only an actually matched
Transfer-Encoding or Content-Encoding line. The complete lowercase
Content-Encoding string passed to the decoder is intentionally preserved, so
there is no public API, packet, ownership, or decoding-contract change.

Static legacy equivalence covers canonical, lowercase, mixed-case, malformed
spacing, unrelated, and Unicode-bearing Header lines. End-to-end mixed-case
gzip and chunked responses are also locked. A bounded 15-second differential
fuzz run executes 29,446 inputs without a mismatch. Focused race passes in
`1.041 s` at `547,968 KiB` peak RSS. Full `common/utils/lowhttp` (`182.741 s`,
`487,912 KiB`), full `common/yakgrpc/yakit` (`64.422 s`, `877,088 KiB`), and
all 62 MITMV2 MUSTPASS tests (`202.761 s`, `3,287,024 KiB`) pass without swap.

Across five samples of six representative response Headers, the legacy
classifier median is `700.0 ns/op / 304 B/op / 12 allocs`; the candidate is
`149.1 ns/op (-78.7%) / 24 B/op (-92.1%) / 1 alloc`. In the complete 256 KiB
API benchmark, `FixHTTPResponse` changes from the pre-candidate median
`716.308 -> 735.482 us/op (+2.7%)`, with allocation count `86 -> 82`, while
packet-only fixing changes `672.045 -> 661.016 us/op (-1.6%)` and `70 -> 66
allocs`. The large Body dominates those timings, so only the isolated Header
classification result is treated as deterministic.

The like-for-like 400-flow heap reports are Phase 52
`2026-07-27T01-14-29-233Z` and Phase 53
`2026-07-27T02-49-20-863Z`; the candidate matrix is
`body-2026-07-27T02-49-20-127Z`. The `fixHTTPResponse -> strings.ToLower`
allocation sampled at about `1.0 MiB` in Phase 52 disappears from that caller.
The remaining `0.5 MiB` `strings.ToLower` sample is from the separate MITMV2
plain-response decode/cache path. Total sampled allocation changes
`510,155,443 -> 529,735,883 B (+3.8%)`, positive-live changes
`18,189,280 -> 21,252,057 B (+16.8%)`, and post-GC live changes
`284,827,826 -> 275,781,567 B (-3.2%)`; these crossed directions are retained
as forced-GC sampling noise, not a global heap claim.

The Phase 53 five-second CPU report is `2026-07-27T02-55-06-007Z`, matrix
`body-2026-07-27T02-55-05-254Z`, compared with Phase 52
`2026-07-27T01-20-18-423Z`. Total samples change `5.15 -> 5.62 CPU s (+9.1%)`,
`fixHTTPResponse` cumulative samples change `0.46 -> 0.54 s`, and `scanobject`
flat samples change `1.38 -> 1.42 s`; the target is below whole-profile
resolution, so there is no global CPU improvement claim. Heap and CPU runs
both complete 400/400 at about 100 requests/s with 400 direct rows, no Query,
gap, duplicate, out-of-order, or unavailable event, zero final persistence and
visible backlog, exact 318-byte gzip wire/262,144-byte decoded Body checks, and
CPU recovery. No frontend product code, communication protocol, protobuf,
schema, database configuration, GORM, or SQLite driver is changed.

## Reusing Existing HTTP Response Writers

Phase 54 removes a redundant 4 KiB `bufio.Writer` from response serialization.
`WriteHTTPResponse` normally receives an existing `bufio.ReadWriter`, while
`DumpHTTPResponse` writes to a `bytes.Buffer` or `io.MultiWriter`; all already
implement `io.StringWriter`. The old dumper wrapped them in another buffer and
repeatedly flushed only into the existing writer, never through it to the
socket. The candidate writes directly to an existing `io.StringWriter` and
makes those intermediate flushes no-ops. A generic writer that implements only
`io.Writer` retains the original buffered fallback and flush behavior, so
network flush ownership and the public API remain unchanged.

Direct bytes-buffer and write-only fallback tests compare the serialized packet
byte for byte and continue to lock Body restoration/ownership. Focused race
passes in `1.036 s` at `463,764 KiB` peak RSS. Full `common/utils`
(`28.353 s`, `423,356 KiB`), `common/minimartian` plus `common/crep`
(`0.014 s` and `20.536 s`, `553,668 KiB` combined peak), full
`common/yakgrpc/yakit` (`46.989 s`, `877,132 KiB`), and all 62 MITMV2 MUSTPASS
tests (`193.131 s`, `3,317,980 KiB`) pass without swap.

Across five 256 KiB samples, writer-only response serialization changes from
median `2.209 -> 1.140 us/op (-48.4%)`, `4,272 -> 176 B/op (-95.9%)`, and
`9 -> 8 allocs`. Packet-capturing Dump changes
`72.557 -> 58.233 us/op (-19.7%)`, `274,851 -> 270,755 B/op`, and
`13 -> 12 allocs`. Dump plus an external discard writer changes
`64.909 -> 61.028 us/op (-6.0%)`, `274,939 -> 270,843 B/op`, and
`16 -> 15 allocs`. Every path removes exactly 4,096 allocated bytes and one
allocation, without a deterministic latency regression.

The like-for-like 400-flow heap reports are Phase 53
`2026-07-27T02-49-20-863Z` and Phase 54
`2026-07-27T03-17-09-362Z`; the candidate matrix is
`body-2026-07-27T03-17-08-673Z`. The
`dumpHTTPResponse -> bufio.NewWriterSize` stack disappears. Total
`bufio.NewWriterSize` allocation changes `5,789,723 -> 4,210,708 B (-27.3%)`;
the `1,579,015 B` sampled reduction is close to the deterministic
`400 * 4,096 B` expectation. Remaining samples belong to actual proxy/client
and lowhttp persistent-connection writers. Total sampled allocation changes
`529,735,883 -> 539,414,975 B (+1.8%)`, positive-live changes
`21,252,057 -> 7,472,754 B (-64.8%)`, and post-GC live changes
`275,781,567 -> 267,282,124 B (-3.1%)`; only the directly attributed writer
reduction is claimed.

The Phase 54 five-second CPU report is `2026-07-27T03-21-49-203Z`, matrix
`body-2026-07-27T03-21-48-549Z`, compared with Phase 53
`2026-07-27T02-55-06-007Z`. Total samples change `5.62 -> 5.72 CPU s (+1.8%)`,
while runtime memory/GC flat samples change `2.11 -> 1.86 s (-11.8%)` and
`scanobject` changes `1.42 -> 1.31 s (-7.7%)`. Response dumping remains below
CPU sampling resolution. The CPU-run request p95 changes
`66.87 -> 122.61 ms`, while the heap-run candidate is `69.57 ms`; this is
retained as a short-run timing risk rather than evidence of either improvement
or regression.

Heap and CPU runs both complete 400/400 at about 100 requests/s with 400 direct
rows, no Query, gap, duplicate, out-of-order, or unavailable event, zero final
persistence and visible backlog, exact 318-byte gzip wire/262,144-byte decoded
Body checks, and CPU recovery. No frontend product code, communication
protocol, protobuf, schema, database configuration, GORM, or SQLite driver is
changed.

## Single-Statement SQLite Bare-Flow KV Upsert

Phase 55 attributes the remaining GORM clone cost to the bare request/response
storage caller rather than changing the ORM fork. Every gzip flow stores its
318-byte wire response under a unique `general_storages.key`, but the previous
`FirstOrCreate` path issued a SELECT before the guaranteed-new INSERT. For the
SQLite project database and only the `BARE_REQUEST`/`BARE_RESPONSE` groups, the
candidate uses the existing GORM fork's single-statement
`ON CONFLICT("key") DO UPDATE`. Other groups and non-SQLite dialects retain the
legacy `FirstOrCreate` path.

The conflict clause updates only `value`, `group`, and `updated_at`. Tests lock
first insert/read compatibility, one-row cardinality on repeated keys, and
preservation of ID, CreatedAt, ExpiredAt, ProcessEnv, and Verbose. Focused race
passes in `1.229 s` at `967,636 KiB` peak RSS. Full `common/yakgrpc/yakit`
passes in `48.804 s` at `876,360 KiB`, and all 62 MITMV2 MUSTPASS tests pass in
`198.260 s` at `3,307,816 KiB`; none use swap.

Across five transaction-scoped samples with unique bare-response keys, legacy
`FirstOrCreate` has median `75.928 us/op / 24,961 B/op / 380 allocs`; the
single-statement upsert has `34.486 us/op (-54.6%) / 12,915 B/op (-48.3%) /
189 allocs (-50.3%)`. This benchmark includes quoting, GORM callbacks, SQL
construction, driver binding, and SQLite execution; it removes one query while
retaining normal Create callbacks and the surrounding batch transaction.

The like-for-like 400-flow heap reports are Phase 54
`2026-07-27T03-17-09-362Z` and Phase 55
`2026-07-27T03-40-23-266Z`; the candidate matrix is
`body-2026-07-27T03-40-22-353Z`. `FirstOrCreate` and its query callback
disappear from the bare-KV caller. Global GORM `DB.clone` plus `search.clone`
flat allocation changes from about `4,195,456 -> 2,622,192 B (-37.5%)`, and
total sampled allocation changes `539,414,975 -> 530,453,951 B (-1.7%)`.
Positive-live changes `7,472,754 -> 24,406,354 B` and post-GC live changes
`267,282,124 -> 275,152,985 B (+2.9%)`, so there is no resident-memory claim.

In the same heap runs, database catch-up/drain changes `211/401 -> 184/291 ms`,
Renderer drain `433 -> 325 ms`, request p95 `69.57 -> 61.48 ms`, and first
visible `61 -> 47 ms`. These are favorable single-run directions, not a formal
repeated A/B. Both runs complete 400/400 at about 100 requests/s, with 400
direct rows, zero Query/gap/duplicate/out-of-order/unavailable events, zero
final backlog, exact wire/decoded Body checks, and CPU recovery.

The Phase 55 five-second CPU report is `2026-07-27T03-44-49-149Z`, matrix
`body-2026-07-27T03-44-48-470Z`, compared with Phase 54
`2026-07-27T03-21-49-203Z`. Total samples change
`5.72 -> 5.37 CPU s (-6.1%)`, average CPU `114.4% -> 107.4%`, `runtime.cgocall`
flat samples `610 -> 480 ms (-21.3%)`, SQLite bind cumulative samples
`390 -> 220 ms (-43.6%)`, SQLite statement execution `800 -> 520 ms (-35.0%)`,
and transaction commit `370 -> 210 ms (-43.2%)`. Memory/GC and `scanobject`
flat samples move in the opposite direction by `+10.8%/+4.6%`.

The CPU-run request p95 improves `122.61 -> 73.09 ms`, but first visible changes
`63 -> 171 ms`, maximum visible backlog `16 -> 42`, and producer-stop visible
backlog `0 -> 33` before Renderer drain completes. All final correctness and
recovery gates still pass. The phase therefore claims the deterministic
one-statement, allocation, clone, cgo, and SQLite stack reductions only. It
does not modify or release GORM, and changes no frontend product code,
communication protocol, protobuf, schema, connection configuration, or SQLite
driver.

## Direct SQLite Bare-Flow KV Upsert

Phase 56 keeps the Phase 55 single-statement conflict semantics but removes the
remaining GORM Create machinery from the SQLite bare-flow-only branch. The
candidate executes one fixed, parameterized statement through the fork's
`CommonDB()` transaction-aware handle. It still quotes keys and values exactly
as before, uses the same project table, and updates only `value`, `group`, and
`updated_at` on conflict. Generic KV groups and non-SQLite dialects retain
`FirstOrCreate`; no public API, schema, migration, protobuf, connection setting,
or SQLite driver changes.

The semantic tests cover first insert/read, repeat-key cardinality, created and
updated timestamps, default fields, preservation of ID, CreatedAt, DeletedAt,
ExpiredAt, ProcessEnv, and Verbose, and rollback through an enclosing GORM
transaction. Focused race passes in `1.238 s` at `941,792 KiB` peak RSS. Full
`common/yakgrpc/yakit` passes in `52.400 s` at `886,252 KiB`, and all 62 MITMV2
MUSTPASS tests pass in `189.897 s` at `3,293,816 KiB`; none use swap.

Across five transaction-scoped unique-key samples, the Phase 55 GORM upsert has
median `30.044 us/op / 12,886 B/op / 188 allocs`; direct SQLite execution has
`13.228 us/op (-56.0%) / 2,567 B/op (-80.1%) / 31 allocs (-83.5%)`. Both run
the identical parameter quoting and SQLite statement inside the same outer
transaction; the difference is SQL clause construction, reflection, callbacks,
scope cloning, and GORM's per-Create transaction wrapper.

The like-for-like 400-flow heap reports are Phase 55
`2026-07-27T03-40-23-266Z` and Phase 56
`2026-07-27T04-04-44-329Z`; the candidate matrix is
`body-2026-07-27T04-04-43-675Z`. The bare-KV caller changes
`11,541,181 -> 6,822,657 B (-40.9%)`, `OnConflictClause.String` disappears,
global GORM DB/search clone flat allocation changes
`2,622,192 -> 2,097,792 B (-20.0%)`, and total sampled allocation changes
`530,453,951 -> 522,238,654 B (-1.5%)`. Positive-live changes
`24,406,354 -> 13,209,197 B`, while post-GC live changes
`275,152,985 -> 270,303,351 B`; these forced-GC values remain diagnostic rather
than a resident-memory claim. The profiled DB/Renderer drain is slower than the
Phase 55 single sample, so it is explicitly not used as an end-to-end win.

The Phase 56 five-second CPU report is `2026-07-27T04-09-23-245Z`, matrix
`body-2026-07-27T04-09-22-580Z`, compared with Phase 55
`2026-07-27T03-44-49-149Z`. Total samples change
`5.37 -> 4.99 CPU s (-7.1%)`, average CPU `107.4% -> 99.8%`, and runtime
memory/GC flat samples `2.06 -> 1.91 s (-7.3%)`. `scanobject`, SQLite bind,
statement execution, and commit move in the opposite direction; only the
deterministic direct caller and allocation reductions are claimed.

The formal unprofiled comparison uses Phase 55 GORM control matrix
`body-2026-07-27T04-12-27-919Z` and Phase 56 direct candidate matrix
`body-2026-07-27T04-19-55-374Z`, three strictly sequential 400-flow runs per
group. The generated `comparison-vs-gorm-bare-upsert.{json,md}` passes with no
configuration, diagnostic, or correctness difference. Candidate medians change
maximum visible backlog `20 -> 16 (-20.0%)`, DB drain `366 -> 359 ms (-1.9%)`,
request-to-React p95 `508 -> 498 ms (-2.0%)`, and Yak CPU p50
`134.0% -> 129.5% (-3.3%)`. First visible `42 -> 47 ms`, Yak CPU p95
`153.1% -> 170.6%`, and Yak RSS `589.8 -> 601.9 MiB` move in the opposite
direction. All six runs complete 400/400 at about 100 requests/s, with exact
wire/decoded Body, 400 direct rows, zero Query/fallback/gap/order errors, final
backlog zero, CPU recovery, and clean shutdown. The phase is retained for its
directly attributable CPU/allocation reduction, not presented as a universal UI
latency or resource improvement. GORM itself remains unchanged and unreleased.

## In-Kernel ASCII Folding For TrafficGuard Prefiltering

Phase 57 removes the full-size lowercase copy performed before every CGO
minirehs prefilter scan. The C Teddy and Aho-Corasick kernels now fold only
ASCII `A-Z` as bytes are read; punctuation, bytes above `0x7f`, hit offsets,
literal IDs and the original packet remain unchanged. Teddy prefix rejection,
nibble lookup, exact confirmation, scalar tails, SSSE3, NEON, AC root skipping
and AC transitions all use the same fold. The pure-Go non-CGO implementation is
unchanged.

Differential coverage compares Teddy SIMD, its scalar twin, C-AC fallback and
the independent Go-AC implementation across mixed-case literals, punctuation,
ASCII boundary characters, high bytes and 4,800 seeded random corpora. Full
`common/minirehs` plus TrafficGuard passes in `40.208 s`; focused race passes in
`3.852 s` at `904,536 KiB` peak RSS. All 62 MITMV2 MUSTPASS tests pass in
`192.621 s` at `3,236,336 KiB`; no gate uses swap. The native x86 source also
passes `gcc -Wall -Wextra -Werror -fsyntax-only`. A no-CGO package attempt
remains blocked before minirehs by the repository's existing `go-pcre2-lite`
and `pcap` no-CGO build failures; no pure-Go production file is changed.

Across five 256 KiB direct-prefilter samples, the warm scratch median changes
from `189.992 -> 55.578 us/op (-70.7%)`; the cold scratch median changes from
`265.855 -> 108.444 us/op (-59.2%)`, `532,488 -> 270,339 B/op (-49.2%)`, and
`2 -> 1 alloc`. With all 59 TrafficGuard patterns, natural JSON/HTML changes
from `1.606 -> 0.762 ms/op (-52.5%)` and median reported bytes from
`473 -> 209 B/op`; the low-entropy gzip fixture changes from
`0.575 -> 0.471 ms/op (-18.1%)` and `248 -> 175 B/op`. Allocation counts in
the pooled TrafficGuard benchmark remain four.

The like-for-like 400-flow heap reports are Phase 56
`2026-07-27T04-04-44-329Z` and Phase 57
`2026-07-27T04-48-04-936Z`; the candidate matrix is
`body-2026-07-27T04-48-04-257Z`. The `asciiLowerInto` sample changes
`4,026,125 -> 0 B`, cgo `scanHitsImpl` cumulative allocation changes
`6,039,187 -> 2,013,062 B (-66.7%)`, and `MatchedIndexes` changes
`6,563,499 -> 2,537,374 B (-61.3%)`. Total sampled allocation changes
`522,238,654 -> 523,576,173 B (+0.3%)`, positive-live changes
`13,209,197 -> 13,722,408 B (+3.9%)`, and post-GC live changes
`270,303,351 -> 285,214,891 B (+5.5%)`; only the directly attributed copy and
prefilter reductions are claimed.

The Phase 57 five-second CPU report is `2026-07-27T04-52-47-809Z`, matrix
`body-2026-07-27T04-52-47-121Z`, compared with Phase 56
`2026-07-27T04-09-23-245Z`. TrafficGuard cumulative samples change
`340 -> 210 ms (-38.2%)`; the minirehs CGO scan changes from `250 ms` to below
the approximately `28 ms` top-report threshold, at least an 88% reduction.
Total samples change `4.99 -> 5.52 CPU s (+10.6%)`, runtime memory/GC flat
samples `1.91 -> 2.10 s`, and `scanobject` `1.41 -> 1.51 s`, so this is not a
global CPU improvement claim.

The formal unprofiled comparison uses Phase 56 matrix
`body-2026-07-27T04-19-55-374Z` and Phase 57 matrix
`body-2026-07-27T04-55-17-328Z`, three sequential 400-flow runs per group.
`comparison-vs-phase56-ascii-fold.{json,md}` passes with no configuration,
diagnostic or correctness difference. Candidate medians change DB catch-up
`249 -> 176 ms (-29.3%)`, database drain `359 -> 282 ms (-21.4%)`, Renderer
drain `393 -> 322 ms (-18.1%)`, Yak CPU p95 `170.6% -> 158.6% (-7.1%)`, and
Yak RSS `601.9 -> 595.1 MiB (-1.1%)`. Request-to-React remains effectively
flat at `498 -> 504 ms (+1.2%)`.

The same comparison retains adverse directions: maximum visible backlog
`16 -> 22`, persistence backlog `2 -> 3`, Long Task total `0 -> 141 ms`,
Electron drain CPU p95 `4.6% -> 7.0%`, and Yak drain CPU p95
`53.7% -> 123.5%`. All six runs still complete 400/400 at about 100 requests/s
with exact gzip wire/decoded detail, 400 direct rows, zero Query/fallback/gap/
ordering errors, eventual backlog zero, CPU recovery and clean shutdown. The
phase is retained for the deterministic scanner CPU/allocation reduction, not
presented as universal UI latency or drain-CPU improvement. It changes no
frontend product code, protocol, protobuf, schema, database setting, GORM or
SQLite driver.

## Reusing The Owned Fixed Response Across MITM Consumers

Phase 58 removes the second full gzip decode of every unmodified MITMV2
response. Lowhttp already produces an owned, decoded/fixed display packet for
the response object and transfers it into `httpctx`. The old response-hijack
handler eagerly decoded the wire packet again into the plain-response cache,
even when no response plugin inspected it. After that eager decode was made
lazy, HTTPFlow persistence still took the fixed packet and independently
decoded the wire packet whenever the plain cache was empty. The final
candidate instead uses the packet whose ownership persistence just took as
both its plain input and fixed-response provenance.

The ownership boundaries remain explicit. A response-hijack plugin gets an
independently owned decoded packet only when its lazy response closure is
actually evaluated. A modified response never borrows the fixed packet. An
asynchronous mirror hook still forces an independent plain packet, while the
common synchronous no-hook mirror path borrows the fixed packet read-only.
Persistence removes that packet from `httpctx` before reusing it and does not
publish it as a mutable plain cache entry. The same change fixes the extended
response-hook comparison to compare the replacement with the original
response rather than with the request.

Focused ownership, modified-response, async-hook snapshot, hot-patch, manual
hijack, auto-unzip and fixed-provenance tests pass. Focused race passes in
`5.597 s`; its cold test build peaks at `3,607,344 KiB` without swap. Full
`common/yakgrpc/yakit` passes in `57.079 s` at `887,584 KiB`, and all 62
MITMV2 MUSTPASS tests pass in `198.679 s` at `3,318,388 KiB`; neither uses
swap.

The like-for-like 400-flow heap reports are Phase 57
`2026-07-27T04-48-04-936Z` and Phase 58
`2026-07-27T05-44-11-701Z`; the final heap matrix is
`body-2026-07-27T05-44-10-455Z`. Total sampled allocation changes
`523,576,173 -> 432,425,521 B (-17.4%)`, and `bytes.growSlice` changes
`214,078,305 -> 120,575,176 B (-43.7%)`. The approximately `97.91 MiB`
`decodeAndCachePlainResponseBytes -> DeletePacketEncodingWithOwnership`
branch disappears; the remaining approximately `111.99 MiB` decode is the
single lowhttp response fix that creates the display packet. Post-GC live heap
changes `285,214,891 -> 253,338,738 B (-11.2%)`, while positive live delta
changes `13,722,408 -> 25,739,273 B`; the latter is retained as forced-GC
sampling risk rather than hidden.

The Phase 58 five-second CPU report is `2026-07-27T05-49-11-900Z`, matrix
`body-2026-07-27T05-49-11-214Z`, compared with Phase 57
`2026-07-27T04-52-47-809Z`. Total samples change
`5.52 -> 4.85 CPU s (-12.1%)`, average CPU `110.4% -> 97.0%`, runtime
memory/GC flat samples `2.10 -> 1.49 s (-29.0%)`, and `scanobject`
`1.51 -> 1.11 s (-26.5%)`. The duplicate decode branch changes
`390 ms -> 0`; combined decode time changes `660 -> 470 ms (-28.8%)`, and
the response handler changes `1.01 -> 0.83 s (-17.8%)`. TrafficGuard and cgo
samples move in the opposite direction in this short profile, so the phase
claims the removed decode and measured whole-window direction without treating
every CPU subtree as improved.

The formal unprofiled comparison uses the Phase 57 matrix
`body-2026-07-27T04-55-17-328Z` and Phase 58 matrix
`body-2026-07-27T05-51-28-746Z`, three sequential 400-flow runs per group.
`comparison-vs-phase57-response-reuse.{json,md}` passes with no configuration,
diagnostic or correctness difference. Candidate medians change DB catch-up
`176 -> 152 ms (-13.6%)`, database drain `282 -> 251 ms (-11.0%)`, Renderer
drain `322 -> 288 ms (-10.6%)`, request-to-React p95
`504 -> 489 ms (-3.0%)`, Yak CPU p50 `129.3% -> 114.5% (-11.5%)`, and Yak
drain CPU p95 `123.5% -> 81.2% (-34.2%)`.

The same comparison retains adverse directions: maximum visible backlog
`22 -> 43`, producer-stop visible backlog `2 -> 43`, request p95
`49.53 -> 54.34 ms`, Yak CPU p95 `158.6% -> 168.3%`, and Electron CPU p95
`6.52% -> 6.79%`. Persistence backlog improves `3 -> 1`, Long Task total
changes `141 -> 0 ms`, and all six runs complete 2400/2400 at about 100
requests/s with exact wire/decoded Body, 400 direct rows per run, zero Query,
fallback, gap or ordering error, eventual backlog zero, CPU recovery and clean
shutdown. No frontend product code, protocol, protobuf, schema, database
setting, GORM or SQLite driver changes in this phase.

## Vectorized Discord Token Candidate Gate

Phase 59 removes the byte-by-byte full-packet walk used only to gate the fixed
Discord token shape. A valid candidate must contain two dots at fixed offsets
and start with `M` or `N`. The candidate first uses the standard library's
vectorized `bytes.IndexByte` path to reject packets without a dot, then locates
only `M` and `N` starts and validates the original character shape at those
positions. Eight equal prefixes inside a 256-byte window trigger the original
linear scan, bounding adversarial data where indexed matches are dense. The
PCRE2 rule, extracted offsets, alphabet, case and findings are unchanged.

An independent copy of the former implementation is the oracle for 10,000
seeded random byte corpora, valid and invalid boundary cases, a valid token at
the end of a 256 KiB packet, every exact built-in rule and high-risk scanner
coverage. Full TrafficGuard passes in `0.301 s` at `821,712 KiB` peak RSS;
focused race passes in `1.742 s` at `899,048 KiB`; all 62 MITMV2 MUSTPASS tests
pass in `190.786 s` at `3,297,760 KiB`. None uses swap.

Across five 256 KiB samples, median gate time changes from
`123.626 -> 2.381 us/op (-98.1%)` for the low-entropy Electron fixture,
`124.931 -> 4.817 us/op (-96.1%)` for natural JSON,
`125.895 -> 5.234 us/op (-95.8%)` for dot-dense data, and
`3.691 ms -> 2.512 us/op (-99.9%)` for an `MN`-dense packet without dots. A
valid token at the packet tail changes `125.535 -> 4.777 us/op (-96.2%)`.
The combined `MN.`-dense fallback remains flat at
`372.105 -> 372.202 us/op (+0.03%)`. Every branch remains zero-allocation, so
the phase does not run a heap profile without an allocation hypothesis.

The like-for-like five-second CPU reports are Phase 58
`2026-07-27T05-49-11-900Z` and Phase 59
`2026-07-27T06-25-10-732Z`; the candidate diagnostic matrix is
`body-2026-07-27T06-25-10-068Z`. The former
`hasDiscordTokenCandidate 110 ms` leaf disappears, TrafficGuard cumulative
samples change `330 -> 230 ms (-30.3%)`, total samples change
`4.85 -> 4.55 CPU s (-6.2%)`, and average CPU changes `97% -> 91%`. The new
`IndexByte` stack accounts for `30 ms`. Runtime memory/GC flat samples move in
the opposite direction, `1.49 -> 1.67 s (+12.1%)`, and `scanobject` changes
`1.11 -> 1.27 s (+14.4%)`; those risks prevent a universal CPU claim. The run
passes 400/400, exact 318-byte gzip wire and 262,144-byte decoded detail, 400
direct rows, all stream/order/recovery/cleanup checks, at `3,865,664 KiB` peak
RSS and zero swap.

The formal unprofiled comparison uses Phase 58 matrix
`body-2026-07-27T05-51-28-746Z` and candidate matrix
`body-2026-07-27T06-28-29-151Z`, three sequential 400-flow runs per group.
`comparison-vs-phase58-discord-gate.{json,md}` passes with no configuration,
diagnostic or correctness difference. Candidate medians change Yak CPU p95
`168.252% -> 149.159% (-11.3%)`, Electron CPU p95
`6.792% -> 4.953% (-27.1%)`, maximum visible backlog
`43 -> 24 (-44.2%)`, producer-stop visible backlog
`43 -> 1 (-97.7%)`, request p95 `54.340 -> 51.497 ms (-5.2%)`, and Yak RSS
`591.941 -> 589.219 MiB (-0.5%)`.

The same comparison retains adverse directions: database catch-up/drain
`152/251 -> 230/341 ms`, Renderer drain `288 -> 376 ms`, duplex delivery p95
`33 -> 67 ms`, Yak CPU p50 `+3.7%`, request-to-React p95
`489 -> 504 ms`, and maximum persistence backlog `1 -> 4`. Long Task remains
zero. All six runs complete 2400/2400 with exact packet/detail data, 400 direct
rows per run, zero Query/fallback/gap/order errors, eventual backlog zero, CPU
recovery and clean shutdown. No frontend product code, protocol, protobuf,
schema, stored HTTPFlow representation, GORM or SQLite driver changes in this
phase.

## Avoiding SQLite String-Bind Copies For Large HTTPFlow Text

Phase 60 targets the `115,122,656 B` SQLite bind leaf in the Phase 58 heap
profile. The active SQLite driver converts every bound Go string with
`[]byte(v)`, creating another request/response-sized Go heap allocation before
the C bind. The published GORM fork `v1.9.2-yaklang.3` adds
`CreateWithColumnExpressions`, which replaces selected create-column values
without bypassing the normal create callback, hooks, default values, timestamps
or primary-key assignment.

Only SQLite HTTPFlow creates with an individual Request or Response of at least
64 KiB use the new path. Each selected value is exposed as a synchronous
read-only byte view and bound through `CAST(? AS TEXT)`. Small values and every
non-SQLite dialect retain the former `Create` path. SQLite tests assert
`typeof(request/response) == text`, `LIKE` behavior, exact byte-for-byte
readback, hash and ID assignment, and after-save behavior. There is no schema,
stored-data, protobuf or transport change.

Across five same-process benchmark samples, a 64 KiB request plus 256 KiB
response changes approximately `358 -> 32 KiB/op (-91%)`; wall time changes
approximately `3.74 -> 3.58 ms/op` and is treated as noise. An unconditional
expression path for 16 KiB fields had an approximately 5% time regression,
which is why the final adaptive threshold keeps the old path for small and
medium values. Focused functionality and race tests, the complete GORM suite,
and all 62 MITMV2 MUSTPASS tests pass; the final MUSTPASS execution takes
`210.806 s` with an isolated disposable Go cache.

The like-for-like heap reports are Phase 58
`2026-07-27T05-44-11-701Z` and Phase 60
`2026-07-27T07-46-31-959Z`. Total sampled allocation changes
`432,425,521 -> 303,064,972 B (-29.9%)`, SQLite bind changes
`115,122,656 -> 1,574,464 B (-98.6%)`, and the database-persistence category
changes `118,796,482 -> 4,724,354 B (-96.0%)`. The remaining dominant leaves
are `bytes.growSlice 107,301,148 B` and `quoteHTTPPacket 101,324,148 B`.

The five-second CPU reports are Phase 59
`2026-07-27T06-25-10-732Z` and Phase 60
`2026-07-27T07-39-15-157Z`. Total samples change
`4.55 -> 4.31 CPU s (-5.3%)`, SQLite bind `430 -> 30 ms (-93.0%)`,
`runtime.stringtoslicebyte 430 -> 0 ms`, HTTPFlow insert cumulative
`770 -> 400 ms (-48.1%)`, and `scanobject` flat
`1.27 -> 0.97 s (-23.6%)`. Quote, gzip decode and `bytes.growSlice` move in
the adverse direction in the short profile, so only the direct bind/copy and
observed whole-window directions are claimed.

The formal unprofiled comparison uses Phase 59 matrix
`body-2026-07-27T06-28-29-151Z` and Phase 60 matrix
`body-2026-07-27T07-50-48-295Z`, three sequential 400-flow runs per group.
`comparison-vs-phase59-sqlite-text-bind.{json,md}` passes with no
configuration, diagnostic or correctness difference. Candidate medians change
database catch-up/drain `230/341 -> 175/280 ms`, persistence write p95
`10 -> 4 ms`, Renderer drain `376 -> 314 ms`, and maximum
persistence/visible backlog `4/24 -> 3/22`; throughput and request/response to
React remain effectively flat.

Adverse medians remain explicit: Yak CPU p95
`149.159% -> 178.735%`, request p95 `51.497 -> 60.855 ms`, first visible
`45 -> 54 ms`, Electron CPU p95 `4.953% -> 6.206%`, and Yak RSS `+1.2%`.
All six runs still complete 2400/2400 with exact 318-byte gzip wire and
262,144-byte decoded detail, zero Query/fallback/gap/order error, eventual
backlog zero, CPU recovery and clean shutdown. The phase therefore claims the
deterministic large-text bind allocation reduction, not universal UI or CPU
improvement. The next profile-driven candidates are quote ownership/capacity
and the remaining decoded-output growth.

## Contiguous GORM Scope Field Metadata

Phase 61 first classifies the two largest Phase 60 heap leaves instead of
optimizing them speculatively. Approximately 107 MiB in `bytes.growSlice` is
the one required decoded/fixed packet output, while approximately 101 MiB in
`quoteHTTPPacket` is the final historical quoted TEXT representation. Neither
is a proven duplicate. The next avoidable caller is GORM `Scope.Fields`, which
allocated every field descriptor independently and accounted for about
6.29 MiB in the Phase 60 heap.

The published fork `v1.9.2-yaklang.4` stores all descriptors for one Scope in a
contiguous `[]Field` and keeps the existing `[]*Field` API pointing into that
stable backing allocation. Pointer stability, non-aliasing between fields,
`Set`, reflected values, blank detection and embedded-pointer initialization
are covered by compatibility tests. Across five benchmark samples, medians
change approximately `6.15 -> 5.29 us/op (-14%)`,
`2,928 -> 2,000 B/op (-31.7%)`, and
`45 -> 7 allocs/op (-84.4%)`. The complete GORM suite and focused race tests
pass. Real HTTPFlow create benchmarks change approximately
`435 -> 380 allocs/op` for small values and `456 -> 402 allocs/op` for the
large adaptive path, without a deterministic wall-time regression. All 62
MITMV2 MUSTPASS tests pass against `.4` in `211.984 s`.

The like-for-like heap reports are Phase 60
`2026-07-27T07-46-31-959Z` and Phase 61
`2026-07-27T08-32-52-494Z`. Total sampled allocation changes
`303,064,972 -> 289,174,010 B (-4.58%)`, and `Scope.Fields` changes
`6,292,552 -> 1,050,624 B (-83.3%)`. `bytes.growSlice` and quote remain about
`107.83/104.01 MiB`. Positive-live changes
`13.18 -> 28.23 MiB` while post-live changes
`307.88 -> 269.25 MiB`; these conflicting forced-GC diagnostics do not support
a resident-memory claim.

The formal unprofiled comparison uses Phase 60 matrix
`body-2026-07-27T07-50-48-295Z` and Phase 61 matrix
`body-2026-07-27T08-40-24-891Z`, three sequential 400-flow runs per group.
`comparison-vs-phase60-gorm-scope-fields.{json,md}` passes with no
configuration, diagnostic or correctness difference. Candidate medians improve
database catch-up `175 -> 167 ms`, duplex p95 `38 -> 25 ms`, first visible
`54 -> 45 ms`, maximum visible backlog `22 -> 14`, request p95
`60.855 -> 46.650 ms`, and Yak CPU p50/p95
`118.218/178.735% -> 108.982/158.853%`. Adverse directions remain explicit:
database drain `280 -> 315 ms`, Renderer drain `314 -> 369 ms`, and Yak drain
CPU p95 `59.336 -> 139.035%`. Throughput remains effectively flat. All six
runs complete 2400/2400 with exact wire/detail data, zero stream/order error,
eventual backlog zero, CPU recovery and clean shutdown.

Only the authorized GORM fork is published: commit `3b16dee`, tag
`v1.9.2-yaklang.4`. Yaklang remains an uncommitted performance branch and no
frontend product, protocol, protobuf or schema change is introduced. Recovery
from an interrupted test removes an observed 3.2 GiB disposable build cache
and an observed 3.2 GiB global Go cache before rerunning. The cold gate peaks at
approximately 3.2/3.4 GiB in its isolated build/tmp directories; both are
deleted on exit and the global cache ends at 768 KiB. Those post-cleanup values
do not revise the user-confirmed historical 290 GiB Go-cache incident.

## Caching GORM Create Bind State

Phase 62 targets repeated per-column management work rather than the required
decoded and quoted packet outputs. In the Phase 61 heap, ordinary HTTPFlow
Create still includes `Scope.InstanceGet 1,572,936 B`,
`Scope.AddToVars 1,574,144 B`, and
`createCallback 9,442,361 B` cumulative. Every bound column rebuilt an
instance-specific key and looked up `skip_bindvar`, although that setting is
normally absent and does not change during a create. The create callback also
grew its column and placeholder slices repeatedly.

The GORM candidate caches the existence semantics after the first lookup.
`InstanceSet("skip_bindvar", value)` updates the cache immediately; a false
value still means skip because historical behavior tests key existence, not
truthiness. Tests cover setting before the first bind and setting after an
initial cached miss. Create reads Fields once and preallocates its two local SQL
assembly slices. Public API, generated SQL, hooks, defaults, primary-key
assignment, schema and dialect behavior remain unchanged.

Across five samples of 64 bindings, medians change approximately
`6.24 -> 2.24 us/op (-64.1%)`, `7,602 -> 4,073 B/op (-46.4%)`, and
`143 -> 17 allocs/op (-88.1%)`. A strict published `.4` versus local-candidate
HTTPFlow benchmark uses the same build cache and five samples per side. Small
adaptive changes `2.820 -> 2.678 ms/op (-5.0%)`,
`33,134 -> 28,134 B/op (-15.1%)`, and
`380 -> 286 allocs/op (-24.7%)`; medium adaptive changes
`2.976 -> 2.732 ms/op (-8.2%)`, `60,695 -> 55,548 B/op (-8.5%)`, and
`380 -> 286 allocs/op`; large adaptive changes
`3.603 -> 3.405 ms/op (-5.5%)`, `29,653 -> 24,500 B/op (-17.4%)`, and
`402 -> 304 allocs/op (-24.4%)`.

The complete GORM suite and focused race test pass. Yaklang focused
functionality/race and all 62 MITMV2 MUSTPASS tests pass against the local
candidate; the long gate takes `193.624 s`. Phase 61 took `211.984 s`, but this
cross-run direction is not treated as a controlled 8.7% end-to-end claim. The
authorized fork publishes commit `7eadd03` as
`v1.9.2-yaklang.5`; yaklang resolves `.5` without committing or publishing its
worktree.

The like-for-like heap reports are
`2026-07-27T08-32-52-494Z` and
`2026-07-27T14-07-30-521Z`. The direct targets change as follows:
`Scope.InstanceGet 1,572,936 B -> below sampling`,
`AddToVars 1,574,144 -> 524,864 B (-66.7%)`,
`createCallback cumulative 9,442,361 -> 7,346,650 B (-22.2%)`, and
all `DB.Create` cumulative `9,967,673 -> 8,920,722 B (-10.5%)`.
Total sampled allocation nevertheless changes
`289,174,010 -> 324,228,327 B (+12.1%)`, alongside required decoded
`bytes.growSlice 107,825,500 -> 127,956,534 B (+18.7%)` and quote
`104,008,231 -> 110,047,419 B (+5.8%)`. Positive-live improves 25.1% while
post-live regresses 3.1%; no total-allocation or resident-memory claim is made.

The formal unprofiled comparison uses Phase 61 matrix
`body-2026-07-27T08-40-24-891Z` and Phase 62 matrix
`body-2026-07-27T14-14-27-605Z`, three sequential 400-flow runs per group.
`comparison-vs-phase61-gorm-create-binding.{json,md}` passes with no
configuration or diagnostic difference. Candidate medians improve database
catch-up/drain `167/315 -> 152/257 ms`, Renderer drain
`369 -> 295 ms`, first visible `45 -> 43 ms`, request-to-React
`500 -> 492 ms`, Yak drain CPU p95 `139.035% -> 88.543%`, and Yak RSS
`600.996 -> 590.980 MiB`.

Adverse medians remain explicit: maximum visible/shadow backlog
`14 -> 48`, producer-stop visible backlog `0 -> 48`, Electron CPU p95
`6.176% -> 7.432%`, duplex p95 `25 -> 27 ms`, request p95
`46.650 -> 48.443 ms`, and Long Task total `50 -> 53 ms`. Throughput and Yak
CPU p95 remain effectively flat. All six runs complete 2400/2400 with exact
wire/detail data, 400 direct rows per run, zero Query/fallback/gap/order error,
eventual backlog zero, CPU recovery and clean shutdown. No frontend product,
protocol, protobuf or schema change is introduced.

Both heap and formal-matrix fixture build/tmp directories are absent on exit,
the content-addressed Yak binary cache remains bounded at six entries and about
1.4 GiB, and the global Go cache ends at 768 KiB. These values validate the new
cleanup discipline but do not revise the historical 290 GiB incident.

## Reusing GORM Query Scan Plans

Phase 63 targets repeated query metadata work. In the Phase 62 heap,
`Scope.scan` rebuilt column-name, selected-column and reset maps for every row,
with `3,148,496 B` sampled across the 400-row query window. The GORM fork now
builds one column-to-Field-index plan after `rows.Columns()` and reuses it for
each row. Per-row work retains only the destinations required by `database/sql`
and a compact list of Fields actually reset on that row. Dedicated oracles
preserve duplicate-column order, NULL, non-pointer, pointer, `sql.Scanner`,
embedded-field and preload-join behavior.

Across five 400-row metadata samples, medians change approximately
`2.806 ms -> 49.376 us`, `2,246,449 -> 118,698 B/op`, and
`6,001 -> 408 allocs/op`. This is explicitly a metadata upper bound, not a
full-SQL claim. A controlled published `.5` versus local-candidate
`QueryHTTPFlow` benchmark uses the same SQLite seed and five groups of ten
queries. Its medians change `12.319 -> 9.423 ms/op (-23.5%)`,
`4,755,052 -> 2,755,321 B/op (-42.1%)`, and
`53,932 -> 48,739 allocs/op (-9.6%)`.

The complete GORM suite and focused race test pass, as do the complete yaklang
`common/yakgrpc/yakit` package, the focused query race test and all 62 MITMV2
MUSTPASS tests. The long gate takes `200.365 s`. The authorized GORM fork alone
is published as commit `d06871f`, tag `v1.9.2-yaklang.6`; yaklang resolves `.6`
without committing or publishing its worktree.

Like-for-like diagnostic heap reports are
`2026-07-27T14-07-30-521Z` and `2026-07-27T14-56-48-088Z`.
`Scope.scan 3,148,496 B -> below sampling`,
`QueryHTTPFlow/SelectHTTPFlowFromDB cumulative
5,248,336 -> 3,148,885 B (-40.0%)`, server `QueryHTTPFlows cumulative
7,346,028 -> 3,673,185 B (-50.0%)`, and total sampled allocation
`324,228,327 -> 281,511,879 B (-13.2%)`. These forced-GC profiles establish
caller attribution but do not establish resident-memory or UI-latency gains.

The formal unprofiled comparison uses Phase 62 matrix
`body-2026-07-27T14-14-27-605Z` and candidate matrix
`body-2026-07-27T15-03-54-384Z`, three strictly sequential 400-flow runs per
group. `comparison-vs-phase62-gorm-scan-plan.{json,md}` passes with no
configuration, historical diagnostic-coverage or experimental difference. All
2400/2400 flows complete; candidate runs each contain 400 direct rows, zero
Query/fallback/gap/order error, eventual backlog zero, CPU recovery and clean
shutdown.

Candidate medians improve maximum visible/shadow backlog
`48 -> 21 (-56.3%)`, producer-stop visible backlog `48 -> 0`, Yak drain CPU
p95 `88.543% -> 79.370% (-10.4%)`, and Yak RSS
`590.980 -> 583.617 MiB (-1.2%)`. Adverse medians remain explicit: database
catch-up/drain `152/257 -> 169/275 ms`, Renderer drain `295 -> 313 ms`, first
visible `43 -> 49 ms`, duplex p95 `27 -> 63 ms`, request-to-React
`492 -> 497 ms`, Yak CPU p50 `+6.3%`, and Long Task total `53 -> 155 ms`.
Request p95 and Yak/Electron CPU p95 are effectively flat or slightly
favorable.

This fixed-rate case uses the direct live stream throughout and reports
`queryCount == 0`, so it cannot prove that a faster fallback query improves all
real-time UI metrics. No frontend product, protocol, protobuf, schema or SQLite
driver change is introduced. Diagnostic and formal cold-build caches peak near
3.3/3.5 GiB and 2.3/2.3 GiB respectively in isolated build/tmp directories,
which are absent on exit. The Yak binary cache remains six entries/about
1.4 GiB and global Go cache about 26 MiB. These post-cleanup values do not
revise the user-confirmed historical 290 GiB Go-cache incident.

## Query-Shadow End-to-End Validation

Phase 64 removes the direct-stream blind spot from the Phase 63 matrix. It runs
the same bounded 400-request, concurrency-12, 100 requests/s, gzip 256 KiB case
with `httpflow-live-stream-mode=shadow`. The baseline temporarily resolves the
published GORM `.5`; the candidate restores `.6`. Each side runs three times
strictly sequentially, and the worktree is version-asserted back to `.6`
afterward. The Yakit runbook records the complete repeatable matrix command
without changing the root package manifest or product defaults.

The `.5` baseline matrix is `body-2026-07-27T15-18-52-745Z`; the `.6`
candidate is `body-2026-07-27T15-29-06-961Z`.
`comparison-vs-gorm5-scan-plan-shadow.{json,md}` passes with no configuration,
historical diagnostic-coverage or experimental difference. All 2400/2400 flows
complete. Every run has six queries, 400 shadow Query matches, zero direct rows,
zero row-without-event, eventual backlog zero, CPU recovery and clean shutdown.

Target medians improve materially in the real Query path: backend DataQuery p95
`37.644 -> 17.106 ms (-54.6%)`, complete backend Query p95
`37.923 -> 17.148 ms (-54.8%)`, and query round-trip p95
`55.4 -> 38.5 ms (-30.5%)`. COUNT p95 remains
`0.791 -> 0.792 ms` with the same one-in-six execution ratio. Conversion p95
changes `0.759 -> 0.943 ms`; per-flow conversion is also adverse but small in
absolute terms and highly variable, and is not the scan-plan caller.

System/UI medians are mixed. Request/response-to-React improve
`1001/983 -> 968/965 ms`, Yak drain CPU p95 improves 44.8%, and Electron RSS
improves 1.9%. First visible regresses `106 -> 185 ms`, maximum visible backlog
`24 -> 95`, producer-stop visible backlog `0 -> 87`, Long Task total
`179 -> 375 ms`, Yak RSS `584.9 -> 620.5 MiB`, and Yak CPU p95 6.5%.
Database and Renderer drain are approximately flat and throughput remains near
100 requests/s.

The result accepts `.6` as a deterministic SQL/query-scan improvement while
showing that the roughly one-second polling trigger and Renderer burst handling
still dominate shadow-mode feel. Both cold builds peak around 2.6/2.6 GiB in
isolated build/tmp directories, which are removed on exit; the Yak binary cache
remains six entries/about 1.4 GiB. The next phase should use Renderer trace to
separate query-batch arrival, React commit and virtual-table update costs before
changing frontend scheduling.

## Renderer Arrival Cadence And Direct-Merge Validation

Phase 65 first traces both communication modes. Shadow report
`2026-07-27T15-54-08-963Z` and timing-free Query report
`2026-07-27T15-59-23-758Z` retain the roughly one-second polling ceiling and
show only four 51--72 ms IPC/layout tasks each. Default-canary report
`2026-07-27T16-08-54-370Z` has zero Query but still reaches about 496 ms
request-to-React, visible/stop backlog 46, and nine roughly 50-row batches.
Its three 51--62 ms tasks are style/layout work, establishing the old 500 ms
sustained direct interval as the product-latency ceiling rather than SQLite.

The frontend changes the direct minimum and sustained intervals to 100 ms and
the MITM virtual-table overscan from five to two. Reports now record both
intervals, the eight-row sustained threshold, and overscan as case identity.
Against the retained three-run 500 ms/overscan-five matrix
`body-2026-07-27T15-03-54-384Z`, the three-run
`body-2026-07-27T16-26-57-984Z` candidate changes request/response-to-React
approximately `497/494 -> 120/110 ms`, maximum visible backlog
`21 -> 9`, and Long Task total `155 -> 0 ms`. Electron CPU changes from
approximately `3.0/7.12%` p50/p95 to `6.82/8.68%`, and RSS increases about
8.6%; the default accepts this bounded absolute resource cost for a roughly
fourfold responsiveness improvement.

Common row decoration now preserves array and row identity when no favorite,
tag or color transformation is required. Its end-to-end comparison is mixed
and evidence-only because the older side predates scheduler metadata, so it is
retained only as a deterministic allocation fast path.

The direct state updater then replaces two full deduplicate/merge passes with a
snapshot-safe path. It selects accepted rows without building a merged table,
prepends them directly when React still owns the exact snapshot, and falls back
to the authoritative full deduplicate merge if concurrent state changed. A
1000-row plus 10-row Vitest benchmark is about 1.82 times faster. The strict
same-configuration three-run comparison is
`body-2026-07-27T16-44-56-959Z -> body-2026-07-27T16-57-15-146Z`, with
`comparison-vs-double-direct-merge.{json,md}` passing without configuration or
diagnostic differences. Candidate medians improve Electron CPU p50/p95
8.1%/3.6%, request/response-to-React 7.6%/5.1%, Renderer drain 24.7%, and first
visible `137 -> 48 ms`; all three candidate runs observe zero Long Task.
Electron RSS regresses 1.5%, visible backlog changes `10 -> 11`, and noisy Yak
drain CPU remains explicitly adverse.

The higher-load validation `body-2026-07-27T17-03-45-330Z` runs three isolated
1000-request cases at 200 requests/s. Each completes 1000/1000 through exactly
51 direct batches with batch-size p95 22, zero Query/fallback/gap/order error,
and eventual backlog zero. Request-to-React p95 is 106--107 ms, maximum visible
backlog 17--21, and Electron CPU p95 8.67--9.01%. Long Tasks remain
53--104 ms, leaving style/layout as the next frontend trace target while the
backend profile again becomes the primary optimization source.

Harness hardening adds side-effect-free `-h/--help`, rejects an invalid
timing-free trace before launching resources, and terminates the complete WDIO
process tree on interruption. Report `2026-07-27T17-12-13-250Z` verifies
interrupted metadata, disposable-directory removal, and no Electron/WDIO/
chromedriver residue. Focused product tests and E2E preflight each pass 61
tests; the bounded Renderer build succeeds. Global Go cache ends near 53 MiB,
the Yak binary cache remains six entries/about 1.3 GiB, and about 840 GiB disk
is free. These post-cleanup values do not revise the user-confirmed historical
290 GiB Go-cache incident.

## Pooling MITM Downstream Connection Buffers

The 1000-request, 200 requests/s CPU report
`2026-07-27T17-17-11-550Z` records 4.33 CPU seconds in a five-second window.
Object scanning is the primary sampled cost: `runtime.scanobject` is
720 ms flat/1840 ms cumulative and `gcDrain` is 1760 ms cumulative. The paired
heap report `2026-07-27T17-24-01-284Z` attributes only about eight percent of
leaf allocation to database persistence, while every short downstream
connection allocates a fresh 4 KiB `bufio.Reader` and `bufio.Writer`.

The candidate pools the reader/writer pair for exactly the downstream
connection lifetime. It detaches and resets both sides before pooling, returns
them only after `handleLoop` completes, releases the unused first pair during a
SOCKS5 context rebuild, and makes repeated release a no-op. CONNECT/TLS reset,
flush ownership, packet bytes, public APIs, storage and protocol contracts are
unchanged.

Five microbenchmark repetitions measure fresh allocation at
1575--1652 ns/op, 8368 B/op and 5 allocations/op. The reused path measures
24.3--24.8 ns/op with zero bytes and zero allocations. The candidate heap
report `2026-07-27T17-34-38-775Z` reduces the
`CreateProxyHandleContext` cumulative caller from approximately 8.53 MiB to
1.00 MiB; the acquire path samples only about 0.50 MiB of pool warm-up.
Whole-window sampled allocation moves adversely from 178.0 to 196.8 MB because
other parser/GORM samples move in the opposite direction, so this report
supports only the targeted caller claim.

The same five-second CPU workload in `2026-07-27T17-51-24-513Z` changes total
samples from 4.33 to 3.81 CPU seconds (-12.0%). `scanobject` changes from
720 to 620 ms flat (-13.9%) and from 1840 to 1680 ms cumulative (-8.7%);
`gcDrain` changes from 1760 to 1730 ms. Startup RSA work differs by only 30 ms,
which does not explain the total reduction.

The strict decision matrix is
`body-2026-07-27T17-03-45-330Z -> body-2026-07-27T17-41-25-958Z`;
`comparison-vs-pre-proxy-buffer-pool.{json,md}` passes with no case or
diagnostic differences. All six runs complete 1000/1000 at about
200.1 requests/s, publish 1000 direct rows, observe no Query/fallback/gap/order
error, drain to zero, recover CPU, and clean up. Candidate medians improve Yak
CPU p50 by 25.4%, Yak RSS by 4.5%, and network request p95 by 31.1%; Yak CPU p95
is effectively unchanged. Database/Renderer drain, Yak drain CPU and maximum
visible backlog move adversely and remain explicit short-run risks rather than
being reported as an end-to-end UI improvement.

The full minimartian package and targeted race tests pass. Disposable 423 MiB
and 877 MiB verification caches are moved to trash after use; no E2E process or
temporary directory remains, the Yak binary cache stays at six entries/about
1.4 GiB, and the global Go cache is about 57 MiB. These values are post-cleanup
state and do not revise the historical 290 GiB incident.

## Avoiding Duplicate HTTP Header Scanner Copies

The Phase 66 candidate heap still attributes a large object count to response
parsing. `ReadLine` already returns a distinct allocation for each header line,
but `ScanHTTPHeaderWithHeaderFolding` copied every ordinary line into a second
buffer and constructed another temporary `CRLF + continuation` slice for
folded headers.

The scanner now retains the first line allocation directly and grows it only
when a continuation line is observed. The emitted backing array is never reused,
so callbacks may retain their slices exactly as before. A retained-slice oracle,
the complete utils and lowhttp suites, targeted race testing, and a ten-second
legacy differential fuzz run with 138,815 executions all pass.

Across five repetitions, canonical-header parsing changes from approximately
542--563 ns/op, 280 B/op and 11 allocations to 373--387 ns/op, 144 B/op and
6 allocations. Folded-header parsing changes from 468--481 ns/op, 208 B/op and
10 allocations to 346--363 ns/op, 144 B/op and 6 allocations.

The like-for-like heap reports are `2026-07-27T17-34-38-775Z` and
`2026-07-27T18-16-19-882Z`. The duplicate-copy node changes from
2,097,236 sampled bytes and 57,344 sampled objects to zero. Scanner cumulative
objects change from about 250,413 to 232,238 (-7.3%), response-parser
cumulative bytes change from 20.48 to 16.80 MB (-18.0%), and whole-window
sampled allocation changes from 196.85 to 194.45 MB (-1.2%). The latter is
small enough to remain a directional observation rather than a global-memory
claim.

The five-second CPU reports are `2026-07-27T17-51-24-513Z` and
`2026-07-27T18-23-31-611Z`. Total samples change from 3.81 to 3.66 CPU seconds
(-3.9%) and average CPU from 76.2% to 73.2%. The scanner is below stable CPU
sample resolution, so this establishes the absence of an obvious CPU tradeoff,
not a caller-specific CPU win.

The strict unprofiled matrix is
`body-2026-07-27T17-41-25-958Z -> body-2026-07-27T18-25-48-136Z`.
`comparison-vs-phase66-header-scan.{json,md}` passes with three sequential
1000-request runs per side and no case, diagnostic, or metric-coverage
difference. Every candidate run completes and persists 1000 unique flows,
publishes all 1000 direct rows, reports no fallback/gap/order error, drains to
zero, recovers CPU, and shuts down cleanly. Median Yak CPU p50 improves 7.8%
and Renderer drain 9.3%; short-window duplex, first-visible and RSS movements
remain explicit noise risks rather than product claims.

No frontend product, protocol, protobuf, schema, database, GORM, or driver
change is introduced. The 1.3 GiB disposable test cache is permanently removed,
E2E build/tmp directories are absent on exit, the content-addressed Yak cache
remains capped at six entries/about 1.4 GiB, and the global Go cache is about
57 MiB. These are post-cleanup values and do not revise the user-confirmed
historical 290 GiB Go-cache incident.

## Allocation-Free ASCII HTTP Header Classification

The Phase 67 allocation-object tree shows `strings.Builder.grow` accounting for
about 8.5% of sampled objects. `splitHTTPPacketEx` and `fixHTTPPacketCRLF`
lowercased every header key and value merely to recognize Content-Length,
Content-Type, Transfer-Encoding, multipart/form-data, and chunked.

The candidate performs allocation-free ASCII case folding for normal HTTP
headers. If any input byte is non-ASCII, it executes the original
`strings.ToLower` comparison. Differential tests therefore retain Unicode,
invalid UTF-8, empty-prefix/needle, and malformed-header behavior instead of
assuming all traffic is conforming. A ten-second fuzz run executes 99,943
inputs without a mismatch; focused parser tests, targeted race testing, and the
complete lowhttp suite (180.800 seconds) pass.

Five same-binary paired repetitions change header classification from a median
of about 923 to 439 ns/op (-52.4%), from 176 to zero bytes/op, and from 12 to
zero allocations/op.

The controlled heap reports are `2026-07-27T18-16-19-882Z` and
`2026-07-27T18-53-48-984Z`. Whole-window sampled allocation changes from
194.45 to 183.82 MB (-5.5%) and sampled objects from approximately 2.10 to
1.91 million (-8.9%). `strings.Builder.grow` objects change from 179,381 to
88,950 (-50.4%), `splitHTTPPacketEx.func1` cumulative objects from 178,313 to
83,558 (-53.1%), and the `fixHTTPPacketCRLF.func3` allocation node disappears.
The complete fix and split cumulative object counts improve by approximately
45.1% and 38.7%.

The five-second CPU reports are `2026-07-27T18-23-31-611Z` and
`2026-07-27T19-00-25-967Z`. Total samples are effectively flat at 3.66 and
3.68 CPU seconds; the target lowercase/builder/split nodes fall below the
sampling threshold, while `scanobject` moves adversely. The result supports
the allocation claim but deliberately does not claim a global CPU improvement.

The strict unprofiled matrix is
`body-2026-07-27T18-25-48-136Z -> body-2026-07-27T19-02-35-441Z`.
`comparison-vs-phase67-header-classification.{json,md}` passes with three
sequential 1000-request runs per side and no configuration, diagnostic, or
metric-coverage difference. Every candidate run persists 1000 unique flows,
publishes 1000 direct rows, reports no fallback/gap/order error, drains to zero,
recovers CPU, and shuts down cleanly. Database catch-up, duplex delivery,
Renderer drain, Yak RSS, and Long Task medians are favorable; request p95,
Yak CPU p50, and drain CPU move adversely and remain short-window risks.

No frontend product, protocol, protobuf, schema, database, GORM, or driver
change is introduced. The 1.3 GiB disposable verification cache is permanently
removed rather than left in trash; E2E build/tmp directories are absent on
exit, the Yak cache remains capped at six entries/about 1.4 GiB, and the global
Go cache is about 64 MiB. These are post-cleanup values and do not revise the
user-confirmed historical 290 GiB Go-cache incident.

## Reducing Common-Dialect Identifier Quote Allocations

The Phase 68 allocation-object profile identifies
`github.com/yaklang/gorm.commonDialect.Quote` with 65,537 flat and 81,921
cumulative sampled objects. The GORM fork replaces
`fmt.Sprintf("\"%s\"", key)` with equivalent string concatenation. Identifier
quoting, SQL text, parameter ordering, and dialect APIs are unchanged.

Exact cases, a legacy differential fuzz oracle, and paired benchmarks cover the
change. A ten-second fuzz run executes 301,458 inputs without a mismatch; the
complete GORM suite and targeted race test pass. For the common `request`
identifier, the paired median changes from about 83.9 to 27.0 ns/op (-67.8%),
32 to 16 bytes/op, and two to one allocation/op.

A real `HTTPFlow` Create A/B/A places candidate wall time between the two
`.6` baselines, so it establishes no latency claim. It does reduce allocation
count from about 286 to 241 allocations/op (-15.7%) and bytes by about 2%.
The authorized GORM-only publication is commit `40342e7` and lightweight tag
`v1.9.2-yaklang.7`; yaklang resolves that version. The complete
`common/yakgrpc/yakit` suite and the targeted SQLite TEXT race test pass.

The controlled heap reports are `2026-07-27T18-53-48-984Z` and
`2026-07-27T19-31-43-403Z`. Whole-window sampled allocation changes from
183.82 to 182.73 MB (-0.59%) and sampled objects from approximately 1.914 to
1.835 million (-4.12%). The baseline Quote node has 65,537 flat and 81,921
cumulative objects; it falls below the candidate report threshold. This
supports the direct object claim without assigning all whole-window movement
to one formatting expression.

The five-second CPU reports are `2026-07-27T19-00-25-967Z` and
`2026-07-27T19-38-13-641Z`. Total samples change from 3.68 to 3.57 CPU seconds
(-3.0%) and average CPU from 73.6% to 71.4%. Create and Quote remain below the
sampling threshold, so the run excludes an obvious CPU tradeoff but is not a
caller-specific CPU claim.

The strict unprofiled matrix is
`body-2026-07-27T19-02-35-441Z -> body-2026-07-27T19-40-24-435Z`.
`comparison-vs-phase68-gorm-quote.{json,md}` passes with three sequential
1000-request runs per side and no configuration, diagnostic, or metric-coverage
difference. Every candidate run persists 1000 unique flows, publishes 1000
direct rows, reports no fallback/gap/order error, drains to zero, recovers CPU,
and shuts down cleanly. Duplex p95, request p95, first-visible, and Electron CPU
p95 are favorable; database catch-up, Renderer drain, Yak CPU p50, and Yak RSS
move adversely and remain explicit WSL short-window risks.

No frontend product, protocol, protobuf, schema, or database-configuration
change is introduced. GORM and yaklang disposable verification caches peak at
about 2 GiB and are permanently removed; E2E temporary directories and
processes are absent on exit. The global Go cache is about 72 MiB and disk free
space about 839 GiB. These are post-cleanup values and do not revise the
user-confirmed historical 290 GiB Go-cache incident.

## Precompiling Static MITM Glob Filters

The Phase 69 heap profile identifies
`MITMFilter.IsPassed -> YakMatcher -> gobwas/glob.Compile` as a repeated-work
hotspot. The same default hostname and method patterns are compiled for every
flow, accounting for 150,188 cumulative sampled objects, or 8.18% of the
window's object count.

The candidate compiles unencoded literal glob groups when an MITM filter is
updated, before the matcher is published to concurrent readers. The resulting
map is read-only during matching. Invalid patterns, encoded groups, and patterns
added by mutating `Group` after preparation retain the legacy
compile-on-execute path, preserving result and error behavior.

Five same-binary paired repetitions change the median from approximately 6159
to 2448 ns/op (-60.3%), 4000 to 600 bytes/op (-85.0%), and 123 to 35
allocations/op (-71.5%). Differential fuzzing executes 9,026 inputs without a
mismatch. The complete httptpl package, MITM filter-manager regression tests,
and a targeted concurrent race test pass.

The controlled heap reports are `2026-07-27T19-31-43-403Z` and
`2026-07-27T20-25-50-802Z`. Cumulative `gobwas/glob` sampled objects change
from 150,188 to 8,192 (-94.5%). All remaining candidate samples are below the
response MIME-check path; the static request hostname/method compilation path
falls below the report threshold. Whole-window sampled objects move adversely
from 1.835 to 1.920 million (+4.6%), and sampled allocation from 174.27 to
177.56 MB (+1.9%), so the evidence supports the target caller only.

The five-second CPU reports are `2026-07-27T19-38-13-641Z` and
`2026-07-27T20-32-36-941Z`. The target request-filter path changes from about
60 to 20 cumulative milliseconds (-66.7%), while total samples move adversely
from 3.57 to 4.03 CPU seconds (+12.9%). This is not recorded as a global CPU
improvement.

The strict unprofiled matrix is
`body-2026-07-27T19-40-24-435Z -> body-2026-07-27T20-34-45-131Z`.
`comparison-vs-phase69-static-glob.{json,md}` passes with three sequential
1000-request runs per side and no configuration, diagnostic, or metric-coverage
difference. Every candidate run persists 1000 unique flows, publishes 1000
direct rows, reports no fallback/gap/order error, drains to zero, recovers CPU,
and shuts down cleanly. Duplex p95, Yak CPU p50, first-visible, and Electron RSS
are favorable; database catch-up, Electron CPU p95, request p95, and a 50 ms
Long Task move adversely and remain explicit WSL short-window risks.

No frontend product, protocol, protobuf, schema, database, or GORM change is
introduced. The shared test/fuzz/race cache peaks at 7.1 GiB and is permanently
removed. E2E build/tmp directories and processes are absent on exit, the Yak
binary cache remains capped at six entries/about 1.4 GiB, and the global Go
cache is about 73 MiB. These are post-cleanup values and do not revise the
user-confirmed historical 290 GiB Go-cache incident.

## Borrowing Header Strings from Parser-Owned Lines

The Phase 70 allocation-object profile identifies the request and response
parsers as the next repeated small-object source. The Header scanner already
provides one independently owned allocation per logical Header line and does
not mutate it after the callback. The candidate lets Header-map key and value
strings borrow that allocation instead of copying both substrings. Exact
canonical common names are replaced with process-lifetime strings, with their
lowercase classification cached once. Mixed-case, non-ASCII, invalid UTF-8,
missing-colon, and malformed-input behavior remain compatible with the legacy
parser.

Five same-binary paired benchmark repetitions change the median from about
98.02 to 43.36 ns/op (-55.8%), 42 to 4 bytes/op (-90.5%), and three to zero
allocations/op. Differential fuzzing executes 74,917 inputs with two workers
and no mismatch. Request and response lifetime tests overwrite the caller's
source packet after parsing, force GC, and then validate Host and ordinary
Headers. Focused parser tests, the complete `common/utils` package, and a
targeted race run pass.

The controlled heap reports are `2026-07-27T20-25-50-802Z` and
`2026-07-27T20-59-52-643Z`. Request-parser cumulative sampled objects change
from 234,835 to 90,120 (-61.6%), with its Header callback changing from 97,708
to 40,513 (-58.5%). Response-parser cumulative objects change from 332,238 to
155,295 (-53.3%), with its callback changing from 68,266 to 33,142 (-51.5%).
Whole-window sampled objects change from 1.920 to 1.394 million (-27.4%) and
sampled allocation from 186,181,770 to 171,375,677 bytes (-8.0%). Response
cumulative bytes move adversely because of large-allocation sampling, so the
object counts and paired microbenchmark are the primary causal evidence.

The five-second CPU reports are `2026-07-27T20-32-36-941Z` and
`2026-07-27T21-06-48-931Z`. Total samples change from 4.03 to 3.55 CPU seconds
(-11.9%), response-parser cumulative CPU from 220 to 170 ms (-22.7%), and the
folding scanner from 180 to 100 ms (-44.4%). The request parser remains below
stable CPU report resolution; the run establishes a favorable target direction
without replacing the unprofiled decision matrix.

The strict unprofiled matrix is
`body-2026-07-27T20-34-45-131Z -> body-2026-07-27T21-08-49-299Z`.
`comparison-vs-phase70-owned-header-strings.{json,md}` passes with three
sequential 1000-request runs per side and no configuration, diagnostic, or
metric-coverage difference. Every candidate run persists 1000 unique flows,
publishes 1000 direct rows, reports no fallback/gap/order error, drains to
zero, recovers CPU, and shuts down cleanly. Database catch-up, Yak CPU p50,
Yak RSS, Electron CPU p95, and request/response-to-React medians are favorable;
first-visible, request p95, duplex p95, Renderer drain, and Electron drain CPU
move adversely and remain explicit WSL short-window risks.

No frontend product, protocol, protobuf, schema, database, GORM, or driver
change is introduced. The 1.2 GiB disposable verification cache is permanently
removed; E2E build/tmp directories, test homes, and processes are absent on
exit. The Yak cache remains capped at six entries/about 1.4 GiB, the global Go
cache is about 88 MiB, and disk free space about 839 GiB. These are post-cleanup
values and do not revise the user-confirmed historical 290 GiB Go-cache
incident.

## Fast Path for Normalized Header-Value Lookup

The Phase 71 heap profile shows `getHeaderValueList` canonicalizing lowercase
`content-length`, `host`, and `transfer-encoding` on every HTTP dump. It also
allocates a result slice and deduplication map when the Header contains only
one ordinary canonical value.

The candidate reuses static canonical strings for known lowercase common keys.
When exactly one of the lowercase/canonical storage forms exists, and it has at
most eight non-empty unique values, the function returns the existing slice.
Mixed storage, empty or duplicate values, unknown casing, and larger lists
retain the legacy merge/deduplication path. The eight-value bound prevents the
allocation-free duplicate check from becoming an adversarial quadratic path.

Five same-binary paired benchmark repetitions change the median from about
139.4 to 54.07 ns/op (-61.2%), 40 to 8 bytes/op (-80.0%), and two to zero
allocations/op. Exact cases cover canonical, lowercase, mixed, empty,
duplicate, missing, and large-list fallback behavior. Differential fuzzing
executes 55,356 inputs without a mismatch. Focused dump/parser tests, the
complete `common/utils` package, and a targeted race run pass.

The controlled heap reports are `2026-07-27T20-59-52-643Z` and
`2026-07-27T21-33-20-051Z`. The baseline `getHeaderValueList` accounts for
32,768 flat and 54,613 cumulative sampled objects, with about 0.5/1.0 MiB of
sampled allocation. `canonicalMIMEHeaderKey` adds about 21,845 objects and
0.5 MiB. Both target nodes fall below the candidate report threshold.
Whole-window sampled allocation changes from 171,375,677 to 164,300,988 bytes
(-4.1%), sampled objects from 1,394,070 to 1,363,713 (-2.2%), and positive-live
allocation improves 14.0%. Post-live moves adversely by 11.6%, so this does not
support a resident-memory claim.

The five-second CPU reports are `2026-07-27T21-06-48-931Z` and
`2026-07-27T21-39-40-932Z`. Total samples move from 3.55 to 3.65 CPU seconds
(+2.8%), while `scanobject` flat changes from 690 to 600 ms (-13.0%) and
cumulative CPU is effectively flat at 1610/1630 ms. The target lookup is below
CPU report resolution in both runs. The result supports the allocation claim,
not a CPU-speed claim.

The first unprofiled matrix, `body-2026-07-27T21-41-43-728Z`, fails after its
first report and MITM shutdown because Electron CDP returns the known transient
`Promise was collected`. Yak and the application do not panic, cleanup passes,
and the failed matrix is not included in performance samples. A complete rerun
using the cached release binary produces
`body-2026-07-27T21-08-49-299Z -> body-2026-07-27T21-48-21-509Z`.
`comparison-vs-phase71-header-value-fast-path.{json,md}` passes with three
sequential 1000-request runs per side and no configuration, diagnostic, or
metric-coverage difference.

Every candidate run persists 1000 unique flows, publishes 1000 direct rows,
reports no fallback/gap/order error, drains to zero, recovers CPU, and shuts
down cleanly. Duplex p95, request p95, visible backlog, Yak CPU p95, Electron
CPU p50/RSS, and first-visible medians are favorable. Database catch-up,
persistence backlog, Yak CPU p50, and Electron CPU p95 move adversely and
remain explicit WSL short-window risks.

No frontend product, protocol, protobuf, schema, database, GORM, or driver
change is introduced. The 1.2 GiB disposable verification cache is permanently
removed; E2E build/tmp directories, test homes, and processes are absent on
exit. The Yak cache remains capped at six entries/about 1.4 GiB, the global Go
cache is about 95 MiB, and disk free space about 839 GiB. These are post-cleanup
values and do not revise the user-confirmed historical 290 GiB Go-cache
incident.

## Borrowing Split-Parser Owned Line Strings

The Phase 72 allocation-object profile attributes 233,811 cumulative sampled
objects to `splitHTTPPacketEx`. About 52% of that subtree comes through
`fixHTTPPacketCRLF` and 35% through public `SplitHTTPPacketEx`. The first line
already has an independently owned `BufioReadLine` allocation, and each Header
line has the retained-line ownership contract established in Phase 67. The
legacy split parser nevertheless copies those bytes separately for callbacks,
Header hooks, and reconstruction.

The candidate creates one zero-copy string view of the owned first line and
reuses it for raw/request/response callbacks and reconstruction. Header hooks
and reconstruction borrow each independently owned Header line. The final
joined Header string remains independent, and body view/copy behavior is
unchanged. A lifetime test overwrites the source packet after return, forces
GC, and validates the raw first line, parsed request parts, retained hook lines,
and reconstructed Headers. The complete lowhttp suite passes in 184.144
seconds, as does targeted reader-pool/retained-line race testing.

A strict same-cache five-run A/B temporarily restores the exact legacy
conversions and then restores the candidate. The 256 KiB ordinary legacy view
median changes from about 664.5 to 589.9 ns/op (-11.2%), 514 to 426 bytes/op
(-17.1%), and 12 to 9 allocations/op (-25.0%). The request-callback view changes
from 674.4 to 591.0 ns/op (-12.4%), 482 to 386 bytes/op (-19.9%), and 13 to 9
allocations/op (-30.8%). The Header-hook view changes from 689.9 to 597.6 ns/op
(-13.4%), 514 to 426 bytes/op, and 12 to 9 allocations/op. A single line
copy/borrow benchmark changes from about 19.22 to 0.714 ns/op, 32 to zero
bytes/op, and one to zero allocations/op.

The controlled heap reports are `2026-07-27T21-33-20-051Z` and
`2026-07-27T22-07-17-076Z`. `splitHTTPPacketEx` flat sampled objects change
from 67,150 to 10,058 (-85.0%), cumulative objects from 233,811 to 95,529
(-59.1%), and the Header callback cumulative count from 105,130 to 64,171
(-39.0%). Whole-window sampled objects and allocation move adversely by
5.6%/2.0% as GORM Quote, Builder, and reflect callers vary. Positive-live also
moves adversely while post-live improves 8.9%, so the evidence supports the
target split caller rather than a global-memory claim.

The five-second CPU reports are `2026-07-27T21-39-40-932Z` and
`2026-07-27T22-13-41-550Z`. Total samples change from 3.65 to 3.58 CPU seconds
(-1.9%). The target split node is below report resolution in both runs, while
GC samples are mixed; this is not recorded as a caller-specific CPU win.

The first unprofiled matrix, `body-2026-07-27T22-15-49-617Z`, passes two runs
and then fails during third-run startup because the Electron CDP bridge is
temporarily unavailable and screenshot capture times out. It remains failed
and is excluded from comparison. The automation recognizes only the exact
bridge-unavailable transport message in addition to the existing
`Promise was collected` error. It retries idempotent window-state collection
once per poll under the existing 15/30-second hard timeout. Application
assertions, backend errors, and sustained unavailability remain failures; five
focused CDP tests pass.

The complete rerun produces
`body-2026-07-27T21-48-21-509Z -> body-2026-07-27T22-26-18-523Z`.
`comparison-vs-phase72-owned-split-lines.{json,md}` passes with three
sequential 1000-request runs per side and no configuration, diagnostic, or
metric-coverage difference. Every candidate run persists 1000 unique flows,
publishes 1000 direct rows, reports no fallback/gap/order error, drains to
zero, recovers CPU, and shuts down cleanly. Database catch-up/drain, duplex
p95, Renderer drain, first-visible, Yak CPU p50, and Electron CPU p95 are
favorable; delivery p95, visible backlog, Electron CPU/RSS, Yak RSS, and Yak
CPU p95 retain adverse short-window movements.

No frontend product consumption, protocol, protobuf, schema, database, GORM,
or driver change is introduced; the frontend change only hardens E2E CDP
transport recovery. The 873 MiB verification cache is permanently removed;
E2E build/tmp directories, test homes, and processes are absent on exit. The
Yak cache remains capped at six entries/about 1.4 GiB, the global Go cache is
about 101 MiB, and disk free space about 839 GiB. These are post-cleanup values
and do not revise the user-confirmed historical 290 GiB Go-cache incident.

## Precompiling Static MITM MIME Glob Rules

The Phase 73 allocation-object profile exposes the MIME branch left after the
Phase 70 ordinary-glob optimization. The default `ExcludeMIME` path runs
`MITMFilter.IsMIMEPassed -> YakMatcher -> MIMEGlobRuleCheck` for every response
and recompiles patterns such as `image/*`, `audio/*`, `video/*`, and `*zip`.
The baseline attributes 65,537 cumulative sampled objects to
`MIMEGlobRuleCheck`, including about 32,768 objects under
`glob.Compile/parserMain`.

The candidate compiles unencoded static MIME rules before the filter is
published to concurrent readers and performs immutable lookups while matching.
It preserves the legacy branch-specific behavior for slash-separated MIME
components, bare wildcards, case-insensitive bare contains, invalid globs,
encoded groups, and patterns added after preparation. No filter storage or RPC
format changes.

Five same-binary paired benchmark repetitions change the median from about
4151 to 2061 ns/op (-50.3%), 2808 to 841 bytes/op (-70.0%), and 77 to 22
allocations/op (-71.4%). Fixed semantic cases exercise every legacy branch.
Differential fuzzing executes 7,897 inputs with two workers without a mismatch.
The complete `common/yak/httptpl` package, concurrent matcher race coverage,
and the existing MITM V2 Content-Type gRPC filter test pass.

The controlled heap reports are `2026-07-27T22-07-17-076Z` and
`2026-07-28T02-54-28-837Z`. `MIMEGlobRuleCheck`, `glob.Compile`, and
`parserMain` all fall below the candidate delta-report threshold. The complete
YakMatcher cumulative sampled-object count changes from about 132,780 to
10,923 (-91.8%). Unrelated Builder, GORM, and large-packet sample movements do
not have a direct caller relationship, so this is a target-path claim rather
than a whole-heap or resident-memory claim.

The five-second CPU reports are `2026-07-27T22-13-41-550Z` and
`2026-07-28T03-01-05-758Z`. YakMatcher cumulative samples change from about
40 to 10 ms and `IsMIMEPassed` from about 30 to 10 ms. Total samples change
from 3.58 to 1.99 CPU seconds, but GC and GORM Create move substantially at
the same time. The total is therefore not attributed to this MIME change; CPU
is only corroborating evidence for the benchmark and target heap result.

The complete unprofiled comparison is
`body-2026-07-27T22-26-18-523Z -> body-2026-07-28T03-03-30-690Z`.
`comparison-vs-phase73-static-mime-precompile.{json,md}` passes with three
sequential 1000-request runs per side and no configuration, diagnostic, or
metric-coverage difference. Every candidate run persists 1000 unique flows,
publishes 1000 direct rows, reports no Query/fallback/gap/order/unavailable
condition, drains to zero, recovers CPU, and shuts down cleanly.

Product metrics remain mixed. Renderer drain, Yak RSS, and Yak drain CPU p95
are favorable. Database catch-up, duplex p95, request-to-React p95, visible
backlog, Yak load CPU p50/p95, and Electron load CPU p50/p95 move adversely.
The static MIME matcher improvement is accepted, while the formal matrix does
not establish a whole-process CPU or interaction-latency improvement.

No frontend product, protocol, protobuf, schema, database, GORM, or driver
change is introduced. The disposable fuzz/race cache and tmp peak at about
5.7/1.9 GiB and are permanently removed. E2E build/tmp directories and related
processes are absent on exit. The Yak binary cache is about 1.4 GiB, the global
Go cache about 102 MiB, and disk free space about 839 GiB. These are
post-cleanup values and do not revise the user-confirmed historical 290 GiB
Go-cache incident.

## Bypassing Raw Matcher Material Hashing and Caching

After the static MIME compilation work, every sampled YakMatcher object in the
Phase 74 heap belongs to generic material preparation. Empty or explicit `raw`
scope already returns the current packet, but the legacy path still hashes the
complete request/response/scope tuple, hex-encodes the digest, and accesses a
one-minute TTL cache. The
`YakMatcher -> cacheHash -> CalcSha1 -> hex.EncodeToString` chain accounts for
about 10,923 sampled objects.

The candidate returns a read-only byte-to-string view only for empty and
explicit `raw` scope. Matcher functions consume it synchronously during the
call and do not retain it. Expressions keep an owned string; header, body,
request, interactsh, and unknown scopes retain the legacy parsed/cache path.
Unencoded groups are read directly instead of being copied to a temporary
slice, while encoded groups still decode into an independently allocated,
pre-sized slice. A binary-matcher regression case verifies that its forced hex
encoding remains intact.

In a same-cache five-run before/after comparison, the complete precompiled
default MIME matcher changes from about 2176 to 263 ns/op (-87.9%), 841 to zero
bytes/op, and 22 to zero allocations/op. An in-binary unknown-scope oracle
isolates the old raw hash/cache path: it changes from about 1647 to 38.36
ns/op (-97.7%), 425 to zero bytes/op, and 18 to zero allocations/op. Coverage
includes default/explicit raw, word, suffix, regexp, glob, MIME, and/negative,
hex/binary, packet mutation between calls, NUL, invalid UTF-8, and Unicode.
Differential fuzzing executes 8,933 inputs without a mismatch. The complete
httptpl package, targeted race checks, and the existing MITM V2 Content-Type
gRPC filter test pass.

The controlled heap reports are `2026-07-28T02-54-28-837Z` and
`2026-07-28T03-31-16-037Z`. YakMatcher, `cacheHash`, `CalcSha1`, hex, and MIME
have no match in the candidate delta report. Whole-window sampled objects move
from 115,130 to 214,431 as process lookup, SQLite Query, Header canonicalization,
and pprof samples move adversely. The result is therefore a target-caller
claim, not a whole-heap or resident-memory claim.

The five-second CPU reports are `2026-07-28T03-01-05-758Z` and
`2026-07-28T03-37-46-431Z`. Baseline YakMatcher, cacheHash, and IsMIMEPassed
each have about 10 ms; all fall below the candidate's 10 ms resolution. Total
samples change from 1.99 to 1.61 CPU seconds (-19.1%) and cumulative
`scanobject` from 1.55 to 1.16 seconds (-25.2%). This corroborates the
allocation direction but is not attributed wholesale to the matcher change.

The complete unprofiled comparison is
`body-2026-07-28T03-03-30-690Z -> body-2026-07-28T03-39-59-596Z`.
`comparison-vs-phase74-raw-material-fast-path.{json,md}` passes with three
sequential 1000-request runs per side and no configuration, diagnostic, or
metric-coverage difference. Every candidate run persists 1000 unique flows,
publishes 1000 direct rows, reports no Query/fallback/gap/order/unavailable
condition, drains to zero, recovers CPU, and shuts down cleanly.

Request/response-to-React p95, committed delivery p95, visible backlog, Yak CPU
p95, and Electron CPU are favorable. Yak CPU p50, request p95, first visible,
database catch-up/drain, Renderer drain, and Yak RSS move adversely. The raw
matcher work is accepted for its deterministic local evidence; the matrix does
not establish a stable whole-process or first-visible improvement.

No frontend product, protocol, protobuf, schema, database, GORM, or driver
change is introduced. The 7.1 GiB disposable fuzz/race cache is permanently
removed. E2E build/tmp peak at about 3.1/3.1 GiB during the bounded cold build
and are absent on exit, as are related processes. The Yak binary cache remains
six entries/about 1.4 GiB, the global Go cache about 104 MiB, and disk free
space about 839 GiB. These are post-cleanup values and do not revise the
user-confirmed historical 290 GiB Go-cache incident.

## Skipping Unmodified Response Snapshot and Reparse

The Phase 75 large-body heap showed a conservative ownership boundary in the
generic crep response hijacker. MITM V2 returned the original response packet
when no plugin, rule, or manual action changed it, but crep still cloned the
complete packet and parsed it into a second response. This is required for
unknown legacy callbacks, while MITM V2 can explicitly report every modified
path through its response context.

The candidate adds a modification-aware response-hijack option. Only a
`modified=false` result with the same packet length and starting address keeps
the already parsed response. Legacy callbacks, explicit modifications,
independent result slices, and mislabelled independent results retain the
snapshot-and-parse path. The existing option and storage/wire formats are
unchanged, so this is an additive contract rather than a breaking API update.

Five 256 KiB benchmark runs give a median of about `49.7 us/op`,
`272,682 B/op`, and `38 allocs/op` for the legacy path versus about `2.5 ns/op`,
`0 B/op`, and `0 allocs/op` for the explicit unmodified path. Focused contract
tests, the complete `common/crep` package, targeted race, and every
`TestGRPCMUSTPASS_MITMV2*` test pass; the latter takes `198.797 s`.

The matched forced-GC heap reports are `2026-07-28T03-31-16-037Z` and
`2026-07-28T04-20-06-196Z`. `cloneAndParseHijackedResponse` changes from
`39,296,860 B` to absent, global `bytes.Clone` from `51,736,565` to
`5,058,141 B (-90.2%)`, response-hijacker cumulative allocation from
`125,316,409` to `88,387,165 B (-29.5%)`, and whole-window allocation delta
from `347,607,036` to `297,616,058 B (-14.4%)`. The last value is supporting
evidence; only the target callers have direct attribution. The old CPU profile
puts the clone/reparse target below its 10 ms resolution, so no low-value CPU
profile was added.

The unprofiled gate is
`body-2026-07-28T03-39-59-596Z -> body-2026-07-28T04-27-08-051Z`.
`comparison-vs-phase75-unmodified-response-fast-path.{json,md}` passes with no
configuration, diagnostic, or metric-coverage difference. All three candidate
runs complete 1000 producer, target, database, unique, shadow-direct, and
live-direct rows, with no fallback, gap, sequence, duplicate, order, or
unavailable condition; final backlogs are zero and cleanup succeeds.

Database catch-up/drain, duplex delivery, first visible, request latency,
Renderer drain, and Yak CPU p50 are favorable. Visible backlog,
request/response-to-React, Yak CPU p95/RSS, and Electron CPU move adversely.
The change is retained for deterministic ownership, allocation, and correctness
evidence, not as a claim of stable whole-product improvement.

The 3.7 GiB disposable Go cache is permanently removed. E2E build/tmp and
related processes are absent after the run; the bounded Yak binary cache is six
entries/about 1.4 GiB, global Go cache about 151 MiB, and free disk about
839 GiB. These are post-user-cleanup values and do not revise the confirmed
historical 290 GiB Go-cache incident.

## Skipping Unmodified Request Reparse

The Phase 76 large-body heap showed the corresponding conservative boundary on
the request side. MITM V2 returned the original request packet when no filter,
rule, plugin, or manual action modified it, but the generic crep layer still
called `ParseBytesToHttpRequest` and replaced the already parsed request. That
behavior remains necessary for unknown legacy callbacks, while MITM V2 records
every modified path in request context.

The candidate adds an additive modification-aware request-hijack option. Only
`modified=false` together with the same packet length and starting address
retains the current `http.Request`. Legacy callbacks, explicit modifications,
independent result slices, mislabelled independent results, and drops retain
the conservative path. The two request options replace each other in
last-option-wins order. In-place modifiers must explicitly report true.

Five 256 KiB runs give a median of about `152.7 us/op`, `807,492 B/op`, and
`67 allocs/op` for the legacy parse path versus about `2.917 ns/op`, `0 B/op`,
and `0 allocs/op` for the explicit unmodified path. Contract tests cover option
replacement, legacy semantics, same-packet retention, explicit modification,
and an independent packet incorrectly labelled unmodified. The complete
`common/crep` package, targeted race, and all
`TestGRPCMUSTPASS_MITMV2*` tests pass; the latter takes `197.079 s`.

The matched forced-GC reports are `2026-07-28T04-20-06-196Z` and
`2026-07-28T04-57-46-431Z`. Request-hijacker cumulative allocation changes
from `43,526,824` to `21,617,855 B (-50.3%)`,
`ParseBytesToHttpRequest` from `49,237,490` to
`26,837,176 B (-45.5%)`, `FixHTTPPacketCRLF` from `34,807,277` to
`20,194,861 B (-42.0%)`, and `ReadHTTPRequestFromBytes` from
`31,290,684` to `18,444,644 B (-41.1%)`. The lower-level request reader changes
from `53,597,303` to `44,672,541 B (-16.7%)` because the initial wire parse and
owned bare-request dump are still required.

Whole-window sampled allocation changes only from `297,616,058` to
`289,589,540 B (-2.7%)`; positive live delta and post-GC live heap change
`22,840,318 -> 12,803,780 B` and
`278,685,936 -> 269,169,217 B`. Those forced-GC values are diagnostic
corroboration, not whole-process or resident-memory claims. The target is below
the useful resolution of the previous CPU profile, so no attribution-free CPU
profile was added.

The unprofiled gate is
`body-2026-07-28T04-27-08-051Z -> body-2026-07-28T05-05-07-631Z`.
`comparison-vs-phase76-unmodified-request-fast-path.{json,md}` passes with
three sequential runs per side and no case-configuration, diagnostic, or
metric-coverage difference. Every candidate run completes 1000 producer,
target, database, unique, shadow-direct, and live-direct rows, with no Query,
fallback, gap, sequence, duplicate, order, or unavailable condition. Final
backlogs are zero, CPU recovers, and cleanup succeeds.

Yak CPU p50, request latency, request/response-to-React, persist-to-React,
Electron CPU, and Long Task total duration are favorable. Visible backlog,
Duplex delivery, database catch-up/drain, Renderer drain, and Yak drain CPU
move adversely; first-visible and RSS remain near neutral. The change is
retained for deterministic semantics and allocation evidence, not as a claim
that every whole-product metric improved.

No frontend product, protocol, protobuf, schema, database, GORM, or driver
change is introduced. The 3.7 GiB validation cache and about 2.9 GiB temporary
build tree are permanently removed. Profiled and unprofiled E2E build/tmp
directories peak near 2.0/2.0 GiB and 1.7/1.7 GiB respectively and are absent
on exit, as are Electron, Yak, WDIO, and ChromeDriver processes. The bounded
Yak binary cache remains six entries/about 1.4 GiB, global Go cache about
158 MiB, and disk free space about 839 GiB. These are post-cleanup values and
do not revise the user-confirmed historical 290 GiB Go-cache incident.

## Replacing Per-I/O Context Copies with Connection-Bound Cancellation

The Phase 77 large-response heap exposed a transport cost below the HTTP
packet builder. `CreateProxyHandleContext` wrapped the accepted connection in
`ctxio.NewReader` and `ctxio.NewWriter`. Each individual read or write created
an equal-sized transfer buffer, a channel, and a goroutine so that context
cancellation could race the underlying operation. A 256 KiB response therefore
paid another packet-sized copy even though the connection and its cancellation
scope already live for the complete proxy session.

The candidate binds cancellation once per minimartian connection. Reads and
writes now pass the caller's packet directly to `net.Conn`; one watcher uses a
connection deadline to interrupt an outstanding operation when the session
context is cancelled, and closes only connections that do not support
deadlines. Releasing a proxy context stops and joins the watcher before the
buffer pair can be reused during the SOCKS/TLS context rebuild. Generic
`ctxio` remains unchanged for callers whose lifetime is not connection-bound.

Five same-binary benchmark runs change a 64 KiB read from a median of about
`14.107 us/op`, `65,720 B/op`, and `4 allocs/op` to about `1.019 us/op`,
`0 B/op`, and `0 allocs/op`. A 256 KiB write to an allocation-free sink changes
from about `40.461 us/op`, `262,330 B/op`, and `4 allocs/op` to about
`7.614 ns/op`, `0 B/op`, and `0 allocs/op`. The sink benchmark isolates wrapper
copy/allocation overhead and is not a network-throughput claim. Tests verify
direct packet identity on read and write, cancellation of a blocked
`net.Pipe` read, release-before-cancel behavior, and the pooled context
lifecycle. The complete `common/minimartian` package, targeted race checks, and
all `TestGRPCMUSTPASS_MITMV2*` tests pass; the latter takes `198.890 s`.

The matched forced-GC heap reports are `2026-07-28T04-57-46-431Z` and
`2026-07-28T05-33-50-436Z`. The former
`ctxio.(*ctxReader).Read` allocation (`11,649,708 B`) and the downstream
`bufio.(*Writer).Write` allocation (`39,974,245 flat / 41,026,922 cumulative
B`) are absent in the candidate. `Proxy.handleRequest` cumulative allocation
changes from `220,528,058` to `162,204,973 B (-26.5%)`, and whole-window
sampled allocation from `289,589,540` to `230,513,763 B (-20.4%)`.
`bytes.growSlice` remains essentially flat at about `132.1 -> 133.7 MB`,
confirming that the required final HTTP packet still exists. Post-GC live heap
is slightly favorable while positive-live delta is adverse, so neither is used
as a resident-memory claim.

The five-second CPU diagnostic is `2026-07-28T05-40-29-047Z`. The removed
`ctxio` target is below the candidate profile resolution and GC scan work moves
favorably, but total samples and `Proxy.handleRequest` move adversely against
the latest compatible earlier CPU profile. This is not a controlled adjacent
CPU A/B, so no whole-process CPU improvement is claimed.

The unprofiled product gate is
`body-2026-07-28T05-05-07-631Z -> body-2026-07-28T05-44-17-656Z`.
`comparison-vs-phase77-connection-bound-direct-io.{json,md}` passes with three
sequential 1000-request runs per side and no case-configuration, diagnostic,
or metric-coverage difference. Every candidate run completes 1000 producer,
target, database, unique, shadow-direct, and live-direct rows, with no Query,
fallback, gap, sequence, duplicate, order, or unavailable condition. Final
backlogs are zero, CPU recovers, and cleanup succeeds.

Candidate medians are favorable for visible backlog (`40 -> 19`), duplex
delivery p95 (`76 -> 45 ms`), request-to-React p95 (`116 -> 107 ms`),
request latency p95 (`6.148 -> 5.519 ms`), Renderer drain (`397 -> 375 ms`),
and Electron CPU. Database catch-up changes adversely by `3.8%` and Yak drain
CPU p95 by `14.5%`; the latter has high three-run dispersion. Yak load CPU and
both process peak working sets are near neutral. The target allocation removal
is accepted without claiming that every product metric improves.

No frontend product, protocol, protobuf, schema, database, GORM, or driver
change is introduced. The bounded cold build peaks near `3.3/3.3 GiB` for the
dedicated build/tmp directories; both are absent after the run. Electron, Yak,
WDIO, and ChromeDriver processes and temporary test homes are also absent. The
Yak binary cache remains capped at six entries/about `1.4 GiB`, the global Go
cache is about `165 MiB`, and disk free space about `839 GiB`. These are
post-user-cleanup values and do not revise the confirmed historical
`/home/go0p/.cache/go-build = 290G` incident.

## Reusing Parsed Requests for HTTPFlow Parameter Counts

The Phase 78 large-body heap showed that `CreateHTTPFlow` dumped an already
parsed request and immediately reparsed that dump through
`NewFuzzHTTPRequest`. The resulting fuzz request was used only to take the
length of the public GET, POST, and Cookie parameter lists. This repeated the
request parser, header/body ownership work, and dump allocation for every
persisted flow.

The candidate adds `mutate.CountHTTPRequestParams(*http.Request)` and reuses
the request instance already owned by the MITM pipeline. The raw-packet
fallback still parses once when a caller does not provide that instance.
Counting deliberately delegates to the existing fuzz parameter
materialization so JSON, base64-encoded JSON, form, XML, Cookie, duplicate, and
empty-body behavior remains identical. It consumes and restores `Body` through
the existing POST implementation and does not retain the request.

Five same-binary benchmark runs change the parameter-count path from a median
of about `18.147 us/op`, `10,071 B/op`, and `225 allocs/op` to about
`9.863 us/op`, `5,081 B/op`, and `155 allocs/op`: approximately `-45.6%`
time, `-49.5%` bytes, and `-31.1%` allocations. Differential tests compare
legacy materialized counts with the parsed-request helper for query/Cookie
JSON and base64, form, JSON body, XML, and empty requests. Product tests also
verify that the parsed and raw fallback paths produce the same non-zero
HTTPFlow counts and leave the body readable. The complete `common/mutate` and
`common/yakgrpc/yakit` packages, targeted race checks, and all
`TestGRPCMUSTPASS_MITMV2*` tests pass; the latter takes `193.596 s`.

The matched forced-GC heap reports are `2026-07-28T05-33-50-436Z` and
`2026-07-28T06-23-26-506Z`. `CreateHTTPFlow` cumulative sampled allocation
changes from `28.89` to `10.11 MB (-65.0%)`; the old
`NewFuzzHTTPRequest -> DumpHTTPRequest` and
`GetGetQueryParams -> ParseBytesToHttpRequest` stacks disappear. The remaining
target allocation is the existing 64 KiB POST parameter materialization.
Whole-window `bytes.growSlice` changes from `133.67` to `126.05 MB (-5.7%)`,
the required database-compatible `quoteHTTPPacket` output remains essentially
flat (`40.98 -> 40.75 MB`), and total sampled allocation changes from
`230.51` to `216.76 MB (-6.0%)`. Positive-live delta is favorable while
post-GC live heap is adverse, so neither is treated as a resident-memory claim.

The five-second CPU diagnostic is `2026-07-28T06-30-52-200Z`. The baseline
parameter materialization and reparse nodes fall below the candidate's 10 ms
reporting resolution, but total samples, GC scan, and allocation CPU move
adversely. The profile therefore confirms removal of the target stack without
supporting a whole-process CPU claim.

The first unprofiled gate is
`body-2026-07-28T05-44-17-656Z -> body-2026-07-28T06-33-18-310Z`;
`comparison-vs-phase78-reuse-parsed-request-param-counts.{json,md}` passes
correctness but reports broadly adverse product medians. An unchanged second
three-run matrix, `body-2026-07-28T06-44-50-029Z`, demonstrates substantial
A/A drift: duplex p95 changes `87 -> 53 ms`, request-to-React p95
`128 -> 109 ms`, Yak CPU p95 `163.49% -> 148.37%`, and Long Task duration
`290 -> 103 ms` without a code change. Comparing that repeat to Phase 78 puts
first-visible at `45 -> 46 ms`, request/response-to-React at
`107/107 -> 109/108 ms`, request latency at `5.519 -> 5.495 ms`, and Yak CPU
p50 at `49.459% -> 49.654%`; database and Renderer drain remain mixed.

Across both candidate matrices, all six runs complete 1000 producer, target,
database, unique, shadow-direct, and live-direct rows. Query, fallback, gap,
sequence, duplicate, order, replay, recovery, and unavailable counts are zero;
final backlogs are zero, CPU recovers, and cleanup succeeds. The change is
retained for deterministic semantics, microbenchmark, and target-heap evidence.
The formal product evidence is treated as neutral/noisy rather than a stable
whole-product improvement or regression.

No frontend product, protocol, protobuf, schema, database, GORM, or driver
change is introduced. Validation and E2E dedicated build/tmp directories are
absent after cleanup, as are Electron, Yak, WDIO, and ChromeDriver processes
and temporary test homes. The Yak binary cache remains capped at six
entries/about `1.4 GiB`, the global Go cache is about `174 MiB`, and disk free
space about `839 GiB`. These are post-user-cleanup values and do not revise the
confirmed historical `/home/go0p/.cache/go-build = 290G` incident. A later
phase may replace list materialization with an exact count-only visitor, but
only with the same differential semantic oracle.

## Counting HTTPFlow Parameters without Materializing Fuzz Objects

The Phase 79 helper removed request dump/reparse, but still built every
`FuzzHTTPRequestParam`, JSON/GJSON path string, and XML XPath only to return
three lengths. The candidate keeps all public fuzz-list APIs unchanged and
implements the HTTPFlow-only count through matching GET/form, JSON, base64,
Cookie whitelist, and XML traversal rules. POST body inspection retains the
same consume-and-restore behavior.

Deterministic tests cover duplicate and invisible query keys, keyless items,
ignored Cookies, JSON and base64 JSON, nested arrays, form fallback, XML
declarations/comments, SOAP filtering, empty bodies, and repeated body reads.
A bounded two-worker differential fuzz run executes `48,956` inputs without a
count mismatch against the materialized public lists. The complete
`common/mutate` and `common/yakgrpc/yakit` packages, targeted race checks, and
all `TestGRPCMUSTPASS_MITMV2*` tests pass; the latter takes `192.823 s`.

Five same-binary runs compare the Phase 79 parsed-list path with count-only.
Median time changes from about `10.205` to `3.164 us/op (-69.0%)`, allocation
from `5,134` to `1,252 B/op (-75.6%)`, and allocation count from `155` to
`34 allocs/op (-78.1%)`. The older dump/reparse control remains about
`19.661 us/op`, `10,203 B/op`, and `225 allocs/op`.

The matched forced-GC reports are `2026-07-28T06-23-26-506Z` and
`2026-07-28T07-17-18-286Z`. `CountHTTPRequestParams` cumulative sampled
allocation changes from `10,597,031` to `8,923,815 B (-15.8%)`; parameter
object/path construction falls below the report threshold. The remaining
target allocation is entirely `httpRequestReadBody` copying the 64 KiB parser-
owned request body. `CreateHTTPFlow` changes from `51,869,704` to
`48,095,014 B (-7.3%)`, whole-window sampled allocation from `216,763,884` to
`199,569,595 B (-7.9%)`, and post-GC live heap is directionally favorable.
`bytes.growSlice` and the required packet quote are also favorable, but those
sampled global nodes are supporting evidence rather than direct attribution.

The five-second CPU reports are `2026-07-28T06-30-52-200Z` and
`2026-07-28T07-25-10-025Z`. Total samples change from `1.92` to
`1.64 CPU s (-14.6%)` and `scanobject` cumulative from `1.23` to
`0.94 s (-23.6%)`, while `CreateHTTPFlow` changes adversely from `260` to
`320 ms`; count-only itself accounts for about `90 ms`. This diagnostic is
mixed and does not support a whole-process CPU claim.

The unprofiled product gate is
`body-2026-07-28T06-44-50-029Z -> body-2026-07-28T07-28-10-244Z`.
`comparison-vs-phase79-count-only-param-totals.{json,md}` passes with three
sequential runs per side and no diagnostic difference. All candidate runs
complete 1000 producer, target, database, unique, shadow-direct, and
live-direct rows. Query, fallback, gap, sequence, duplicate, order, replay,
recovery, and unavailable counts are zero; final backlogs are zero, CPU
recovers, and cleanup succeeds.

Candidate medians are favorable for duplex delivery p95 (`53 -> 43 ms`),
first-visible (`46 -> 42 ms`), request-to-React (`109 -> 107 ms`), visible
backlog (`20 -> 19`), and Yak CPU p50 (`49.65% -> 44.86%`). Request latency,
Yak CPU p95/RSS, Long Task duration, and Electron CPU are near neutral.
Database catch-up/drain and Renderer drain change adversely by
`7.1%/8.4%/7.2%`. The change is retained for exact semantic, fuzz,
microbenchmark, heap, and end-to-end correctness evidence, without claiming
that every product metric improved.

No frontend product, protocol, protobuf, schema, database, GORM, or driver
change is introduced. The validation Go cache/tmp peak at about
`4.6/2.7 GiB` and are permanently removed; E2E build/tmp directories,
temporary homes, and Electron, Yak, WDIO, and ChromeDriver processes are
absent after cleanup. The bounded Yak cache remains six entries/about
`1.4 GiB`, the global Go cache about `174 MiB`, and disk free space about
`839 GiB`. These post-cleanup values do not revise the confirmed historical
`/home/go0p/.cache/go-build = 290G` incident. The next profile-backed candidate
is a parser-owned request-body view that could remove the remaining 8.9 MB
snapshot under a separately tested ownership contract.

## Borrowing Parser-Owned Request Bodies for HTTPFlow Counts

Phase 80 left one fully attributed allocation in
`CountHTTPRequestParams`: `httpRequestReadBody` copied the unread body into a
new `bytes.Buffer`, then replaced `req.Body` with that copy. MITM requests
created by the low-level parser already own an immutable body slice, so this
copy was unnecessary for the synchronous count-only visitor.

The candidate adds `ReadOwnedHTTPRequestBodyView`. It is intentionally narrow:
it succeeds only for the parser-owned body type, returns the unread view,
resets the same body to the beginning of that view, and documents the result
as synchronous and read-only. Foreign/custom `io.ReadCloser` bodies continue
through the previous copy-and-restore path. Tests cover partial reads, foreign
body non-consumption, repeated reads, forced GC, and count equivalence. The
bounded differential fuzz oracle executes `34,644` inputs without a mismatch.
Complete `common/utils`, `common/mutate`, and `common/yakgrpc/yakit` tests,
targeted race runs, and all `TestGRPCMUSTPASS_MITMV2*` tests pass; the MITM
group takes `201.469 s`.

Five isolated runs change a 64 KiB copy-and-restore from about
`12.407 us/op`, `65,600 B/op`, and `3 allocs/op` to about `5.596 ns/op`,
`0 B/op`, and `0 allocs/op`. This benchmark measures only the body access
contract, not network throughput.

The matched forced-GC reports are `2026-07-28T07-17-18-286Z` and
`2026-07-28T08-01-35-167Z`. The previous
`CountHTTPRequestParams -> countPostParams -> httpRequestReadBody ->
bytes.Reader.WriteTo` stack (`8,923,815 B`) disappears. `CreateHTTPFlow`
cumulative sampled allocation changes from `48,095,014` to `38,622,299 B
(-19.7%)`, and whole-window sampled allocation from `199,569,595` to
`177,333,317 B (-11.1%)`. `bytes.growSlice -15.8%` and the required packet
quote being slightly favorable are supporting, not directly attributed,
evidence. Post-GC live heap is only slightly favorable and is not a resident
memory claim.

The five-second CPU reports are `2026-07-28T07-25-10-025Z` and
`2026-07-28T08-08-07-756Z`. The count/countPost target falls below the 10 ms
report threshold and `CreateHTTPFlow` changes from `320` to `220 ms (-31.3%)`.
Whole-window CPU changes adversely from `1.64` to `1.74 s`, with GC scan also
adverse, so only the target caller is accepted.

The first unprofiled candidate matrices,
`body-2026-07-28T08-10-11-405Z` and
`body-2026-07-28T08-20-30-981Z`, were internally correct but both showed
duplex, request-to-React, and Yak CPU medians above the earlier Phase 80
window. A new E2E control can select only an existing executable in the
managed 20-hex Yak build cache, rejects missing or diagnostic combinations,
and records the selected and current-source fingerprints plus whether they
match. Its 13 fixture tests pass.

That control enables the same-window comparison
`body-2026-07-28T08-32-58-073Z -> body-2026-07-28T08-38-43-195Z`, using the
exact Phase 80 binary `536fe35700419c447fcc` and the Phase 81 binary
`cd3f035a183b867b86e2`. The old binary itself now measures duplex p95
`81 ms`, request-to-React `117 ms`, and Yak CPU p50 `49.56%`, versus its
earlier `43 ms`, `107 ms`, and `44.86%`. This reproduces the apparent
regression without the Phase 81 code and confirms temporal/environment drift.
In the adjacent old/new comparison, Phase 81 is neutral for Yak CPU p50
(`49.56% -> 49.70%`) and favorable for duplex (`81 -> 72 ms`),
request-to-React (`117 -> 113 ms`), database drain (`434 -> 320 ms`), and
Renderer drain (`473 -> 381 ms`). Request latency and Long Tasks move
adversely and remain noisy, so these product medians are not presented as a
uniform speedup.

A second same-window body-bearing comparison is
`body-2026-07-28T08-45-35-379Z -> body-2026-07-28T08-50-45-504Z`, with three
120-request, 64 KiB request-body runs per binary. Throughput is near neutral
(`+1.1%`), Yak CPU p50 is directionally favorable (`-4.8%`), and database and
Renderer drain improve `13.8%/12.0%`; request latency, request/response-to-
React, and Yak peak working set move adversely. The max-rate 120-request
distributions are wide, so the run is a correctness/no-obvious-tradeoff gate,
not a stable product-latency claim.

Across the two old/new comparisons, all twelve runs complete exact producer,
target, database, unique, and live-direct counts. The 64 KiB runs receive
exactly `7,864,320` request-body bytes each. Fallback, gap, sequence gap,
duplicate, out-of-order, replay, recovery, unavailable, and cleanup counts are
zero. The candidate is retained for the deterministic ownership,
microbenchmark, heap, CPU-target, and full-chain correctness evidence.

No frontend product, protocol, protobuf, schema, database, or GORM behavior is
changed. The same-window runs reuse cached binaries and perform no Go build.
E2E build/tmp and temporary homes are absent afterward, with no Electron, Yak,
WDIO, or ChromeDriver process left. The bounded Yak cache remains about
`1.4 GiB`, the global Go cache about `181 MiB`, and disk free space about
`838 GiB`. These are post-user-cleanup values and do not revise the confirmed
historical `/home/go0p/.cache/go-build = 290G` incident.

## Borrowing Request-Owned Plain Request Cache Bytes

The Phase 81 allocation profile shows that an unencoded request first stores
its request-context-owned bare packet and then clones the same full packet
again through `decodeAndCachePlainRequestBytesIfStorable` and
`SetPlainRequestBytes`. The Phase 82 candidate adds an exact-alias-only
borrowed setter. It accepts the value only when its starting address and
length exactly match the context bare packet. Equal external slices,
sub-slices, encoded buffers, and other foreign values retain the cloning
fallback; independently decoded buffers transfer their existing ownership.

The five-run medians for a 64 KiB request change from `13,993` to `1,468
ns/op (-89.5%)`, from `74,507` to `776 B/op (-99.0%)`, and from `19` to `18
allocs/op`. The 128 KiB case changes from `23,905` to `1,467 ns/op (-93.9%)`,
from `140,045` to `776 B/op (-99.4%)`, and from `19` to `18 allocs/op`.
External mutation, forced-GC lifetime, exact alias rejection, and the previous
clone semantics are covered by tests. The full `httpctx` tests, focused plain
request tests and race tests, and all `TestGRPCMUSTPASS_MITMV2*` tests
(`197.454 s`) pass. The complete `common/yakgrpc` package exceeded its
existing ten-minute package timeout while running unrelated long AI/facade
tests; there was no candidate assertion failure, and the run is not reported
as passing.

The forced-GC allocation profiles are
`2026-07-28T08-01-35-167Z -> 2026-07-28T09-39-45-988Z`. The baseline
`bytes.Clone -> SetPlainRequestBytes ->
decodeAndCachePlainRequestBytesIfStorable` stack accounts for `10,116,282 B`
and is absent from the candidate profile. A separate
`reserveHTTPRequestPacketBody` node with the same sampled size remains; it is
the request parser body reservation and no longer descends through the plain
cache clone. Whole-window sampled allocation moves adversely from
`177,333,317` to `188,729,189 B` as unrelated `bytes.growSlice` and packet
quoting samples increase, so no global-allocation improvement is claimed.
Post-live heap moves from `267,815,622` to `261,270,100 B`.

The baseline five-second CPU profile contains `bytes.Clone` at `60 ms
(3.45%)` and the full cache helper at `70 ms (4.02%)`. Neither the clone nor
the plain-cache helper appears in the candidate's otherwise similarly sized
`1.75 CPU s` profile. This is target-path evidence rather than a whole-process
CPU claim.

The first three-by-three same-window comparison at 120 requests initially
shows throughput `-11.2%`. A trailing three-run Phase 81-binary control then
shows that the old binary itself has slowed by `9.6%` compared with its
leading group. Relative to that adjacent old control, Phase 82 is `-1.7%` in
throughput, `-3.8%` in request p95, `-3.0%` in Yak CPU p50, and `-4.6%` in
Yak peak working set. This prevents the grouped time-order drift from being
misclassified as a code regression.

To increase signal without threatening WSL resources, the checked-in body
matrix now includes `request-64k-medium`: 600 requests, concurrency 12, 64
KiB request bodies, and 4 KiB responses. The formal comparison is
`body-2026-07-28T10-14-58-858Z -> body-2026-07-28T10-20-06-695Z`, with
`comparison-vs-phase81-cached-request-64k-medium-same-window.{json,md}`.
Candidate medians improve throughput by `14.3%`, request p95 by `10.9%`,
database and Renderer drain by `4.6%/5.6%`, request/response-to-React by
`10.9%/19.4%`, and Long Task total by `81.8%`. Yak CPU p50 and peak working
set are approximately neutral at `-1.0%/-0.2%`.

First-visible time (`64 -> 156 ms`), duplex p95 (`65 -> 127 ms`), and maximum
instantaneous visible backlog (`21 -> 69`) move adversely. Faster production
and the 700 ms scheduling phase can amplify those instantaneous values, while
final database and Renderer drain improve, but the adverse observations
remain explicitly open rather than being described as a uniform end-to-end
speedup.

All six medium runs complete exact producer, target, database, and unique
counts of `600`, with exactly `39,321,600` request-body bytes per run.
Fallback, gap, sequence gap, duplicate, out-of-order, replay, recovery,
unavailable, and cleanup counts are zero. No frontend product behavior,
protocol, protobuf, schema, database, GORM, or driver behavior changes.
Runs remain single-instance and sequential. The managed Yak cache is about
`1.4 GiB`, global Go cache about `183 MiB`, disk free space about `839 GiB`,
and no Electron, Yak, or ChromeDriver process remains. These values follow the
user's cleanup and do not revise the historical 290 GiB Go cache incident.

## Bounding the TrafficGuard Prefilter's Initial Pair Buffer

The Phase 82 heap retains several large allocations with necessary output or
independence contracts: the quoted packet database representation,
`bytes.growSlice` packet construction, and independent request body/bare packet
storage. The next directly attributable candidate is the TrafficGuard CGO
prefilter. Its scratch pair buffer was initially sized as `len(data)/8+64`,
allocating about 256 KiB for every cold 256 KiB clean input even when the scan
reported no literal hits.

Phase 83 caps only the initial buffer at 8,192 `(end, literalID)` pairs, or 64
KiB. The C kernel already returns the untruncated hit count. Inputs exceeding
the cap therefore allocate the exact required buffer and rescan without losing
hits. A previously expanded scratch reuses its backing array during later
rescans instead of allocating the same exact buffer again.

An initial 2,048-pair cap was rejected before formal testing: a 256 KiB input
with 4,000 hits required a second scan and regressed median CPU time by about
43%, despite lower allocation. With the final 8,192 cap, the three-run clean
input median changes from `102.658` to `67.553 us/op (-34.2%)`, from `270,339`
to `65,537 B/op (-75.8%)`, and remains at one allocation. The 4,000-hit median
changes from `200.869` to `152.681 us/op (-24.0%)`, from `398,590` to `193,787
B/op (-51.4%)`, and remains at 17 allocations.

Tests cover a clean 256 KiB capacity bound, exact 3,000-hit expansion and
rescan, and reuse of the exact-size backing buffer on a second dense scan. The
complete `common/minirehs` suite (`42.612 s`), TrafficGuard, focused race
suite, and all `TestGRPCMUSTPASS_MITMV2*` tests (`197.776 s`) pass.

The forced-GC heap comparison is
`2026-07-28T09-39-45-988Z -> 2026-07-28T10-49-57-181Z`.
`scanHitsImpl 3,917,119 B` and its `MatchedIndexes 4,441,419 B` cumulative
caller both fall below the candidate report threshold. Whole-window sampled
allocation changes from `188,729,189` to `184,998,744 B (-2.0%)`, and
post-live heap changes from `261,270,100` to `259,080,023 B (-0.8%)`.
Positive-live delta moves adversely, so no resident-memory claim is made.

The five-second CPU samples change from `1.75` to `2.38 CPU s`.
Prefilter cumulative samples change from `60` to `80 ms`, while their share is
approximately neutral at `3.43% -> 3.36%`. The whole-process profile therefore
does not establish a CPU improvement; the paired direct benchmarks remain the
CPU evidence.

The formal unprofiled comparison is
`body-2026-07-28T11-05-00-470Z -> body-2026-07-28T11-10-17-929Z`, with
`comparison-vs-phase82-prefilter-pair-cap.{json,md}`. Candidate medians improve
throughput by `6.4%`, response-to-React p95 by `25.2%`, Long Task total by
`49.5%`, and Yak drain CPU p95 by `22.8%`. Yak steady CPU and peak working set
are near neutral. Database and Renderer drain move adversely by `26.5%/22.7%`,
and duplex p95 moves adversely by `68.7%`; the distributions are wide and no
uniform product speedup is claimed.

All six runs complete exact producer, target, database, and unique counts of
120, receive exactly `7,864,320` request-body bytes, and pass the post-window
64 KiB request/256 KiB response detail checks. Fallback, gap, sequence gap,
duplicate, out-of-order, replay, recovery, unavailable, and cleanup counts are
zero. No frontend product behavior, protocol, protobuf, schema, database,
GORM, or driver behavior changes. Isolated test and E2E build caches are
removed afterward. The managed Yak cache remains about `1.4 GiB`, global Go
cache about `183 MiB`, and disk free space about `839 GiB`, with no Electron,
Yak, or ChromeDriver process remaining. These post-cleanup values do not revise
the historical 290 GiB Go cache incident.
