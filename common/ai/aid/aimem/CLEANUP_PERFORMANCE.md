# AI memory cleanup: correctness, resource bounds, and measurements

Measured on 2026-09-13, Apple M1 Max (10 cores, 64 GiB RAM), macOS arm64,
Go 1.22.12. Baseline: `03a8fc3fe`; candidate: this PR. Databases were isolated
SQLite fixtures or consistent backups; live user databases were not cleaned.

## Why cleanup repeated and produced excessive work

- The 30-minute cooldown existed only in process memory, so restarting allowed
  another immediate scan. Small batches under 20 were never physically cleaned.
- Over-count selection loaded every complete memory entity and sorted in Go,
  even though the coordinator ultimately deleted only 100 IDs. Low-value
  selection could miss eligible rows behind its first, higher-value candidates.
- RAG deleted the batch's SQL rows, then removed graph nodes one at a time.
  Neighborhood repair and each full-graph save read other already-deleted lazy
  vectors. Those failures were not negatively cached and produced repeated SQL
  queries and warnings. The graph persistence callback ran once per ID.
- Empty memory graph export failed, leaving the old binary stored. Independently
  loaded memory backends could subsequently save their pre-cleanup graph again.
- Automatic graph saves launched a goroutine for every change, allowing queued
  saves to grow while SQLite was contended.

The fix removes all targets before repairing survivors, saves the graph once,
transacts RAG rows with their graph, transacts memory rows with their graph and
cleanup generation, and returns persistence errors. Stale memory writers rebase
against that generation and retain only still-live local additions. Empty graphs
are persisted explicitly. New collections receive a fresh generation so a
drop/recreate cannot reset the stale-writer guard. Save requests share one worker
per memory backend.

## Measurements

Microbenchmarks use identical synthetic fixtures, three iterations per run and
three runs; the table reports medians. These are local measurements, not an SLA.
The graph microbenchmark uses a fixed 16-neighbor ring to isolate deletion work;
the regression tests separately exercise constructed HNSW graphs and lazy SQLite
vectors, including survivor search and transaction rollback.

| Workload | Baseline | Candidate |
|---|---:|---:|
| Select cleanup candidates, 10,000 rows × 16 KiB content | 193.28 ms/op | 40.65 ms/op |
| Allocations for that selection | 224,010,482 B/op | 366,285 B/op |
| Remove 100 nodes from a 10,000-node graph | 69.76 ms/op | 6.52 ms/op |
| Change callbacks for that graph batch | 100 | 1 |
| Allocations for that graph batch | 2,470,528 B/op | 29,648 B/op |
| Delete 100 memories in the same local database backup | 5.619 s | 0.752 s |
| Missing-vector reads in that backup replay | 56,998 | 0 |
| Warning/error lines in that backup replay | 109,307 | 0 |

The local project database was approximately 29 GB, but its default memory
session contained **286 active memories**, not 29 GB of memory entities. Both
versions reduced this to 186; a second cleanup selected zero IDs. In a separate
128-document regression deleting 100 documents, the original implementation
made 169,901 missing-vector reads; the candidate made zero.

Two dirty-database stress fixtures deliberately included invalid JSON in vectors
and question metadata. Candidate selection returned only 100 IDs without
parsing either field:

| Fixture | Content payload | Maximum process RSS |
|---|---:|---:|
| 100,000 rows × 16 KiB | 1,562 MiB | 90,636,288 B |
| 1,000,000 rows × 1 KiB | 976 MiB | 90,456,064 B |

RSS was measured with `/usr/bin/time -l` around the compiled test executable;
it includes fixture construction, updates and scanning. These stress tests
measure candidate selection, not deletion of a million-node vector graph.

## Limits and recovery behavior

- Default selection is capped at 100 IDs per session; configured batches are
  capped at 1,000, and entity deletion/query chunks at 100. Selection reads IDs
  only and ranks in SQLite. SQLite still evaluates scores across eligible rows.
- A pass visits at most 32 sessions using a persisted keyset cursor and checks a
  10-second context budget between stages. This is cooperative cancellation,
  not a hard wall-clock bound on every SQLite query or graph operation.
- The cooldown survives process restarts and is stored per database. Remaining
  backlog can legitimately require later passes; successful deletions must not
  return as live rows or be restored by an old in-process memory backend.
- Existing valid graphs still require graph-sized topology/load/export work.
  Maintenance does not rebuild or delete an entire corrupt RAG collection as a
  side effect of deleting a memory batch. On such an error it retains memory
  rows and document mappings for recovery and later retry.
- HNSW/PQ binary length fields are validated against remaining bytes before
  allocations, including the standalone PQ codebook. This prevents tiny corrupt
  headers from requesting allocations based on forged counts.

## Reproduction and verification

Use an isolated `YAKIT_HOME` for these commands:

```sh
export YAKIT_HOME="$(mktemp -d)"
go test -short ./common/ai/aid/aimem ./common/ai/rag/vectorstore ./common/ai/rag/hnsw ./common/yakgrpc/aisessioncleanup -skip '^TestRequestLocalEmbedding$' -count=1 -timeout=5m
go test -race ./common/ai/aid/aimem ./common/ai/rag/vectorstore ./common/ai/rag/hnsw -run 'TestMUSTPASS_(Cleanup|DeleteBatch|LoadBinary|ImportCodebook)' -count=3
go test ./common/ai/rag/hnsw -run '^$' -fuzz '^FuzzLoadBinaryBounded$' -fuzztime=30s -parallel=2
go test ./common/ai/aid/aimem ./common/ai/rag/vectorstore -run '^$' -bench 'Benchmark(CleanupMemoryScan|GraphWrapperDeleteBatch)$' -benchtime=3x -count=3
YAK_AIMEM_STRESS_ROWS=100000 go test ./common/ai/aid/aimem -run '^TestMUSTPASS_CleanupLargeDirtyDatabase$' -v -count=1
YAK_AIMEM_STRESS_ROWS=1000000 YAK_AIMEM_STRESS_BODY_BYTES=1024 go test ./common/ai/aid/aimem -run '^TestMUSTPASS_CleanupLargeDirtyDatabase$' -v -count=1
```

`TestRequestLocalEmbedding` needs an independently running embedding service at
`127.0.0.1:11435`; it failed with connection refused on the measurement host and
was excluded from the offline package run. It was not replaced with a fake
service or weakened. The optional `TestCleanupLocalReplay` accepts a disposable
backup via `YAK_AIMEM_REPLAY_DATABASE`; its filename must contain `cleanup-replay`.

Bare `go run common/yak/cmd/yak.go` with the database backup did not enter AI
memory cleanup in the inspected revision. The baseline executable exited in
2.59 seconds. The cleanup timings above come from invoking the actual cleanup
chain against that same backup, which reproduced the supplied warning pattern;
they are not claims about total CLI startup time or Go compilation time.

A separate warm `go run` process sample took about 10.1 seconds and observed
approximately 3.41 GB RSS in Go's `link` process versus 63 MB in the running
`yak` process (200-ms sampling). The first changed-source invocation took 42.9
seconds including compilation/linking. These observations distinguish build
resource use from the runtime cleanup defect; this PR changes the latter.
