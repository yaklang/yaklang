# RAG startup latency and gRPC authentication diagnostics

Branch: `codex/rag-startup-latency`, based on `ab520b4aac60b907285872d304fb9563e68e9db2`.

This change modifies only Yaklang. Yakit startup sources were inspected read-only to identify the authentication probe. Measurements did not replace the live engine, change its configuration, or migrate its databases.

## Findings and limits

The original 08:44:48–08:45:07 log records 18.217915s in GetCollection. Graph recovery had already begun at 08:44:48. The 18.241618s memory warning includes this collection load; it is not a second 18-second embedding request.

The current database did **not** reproduce the historical 18 seconds. A read-only replay of `yakit-projects/default-yakit.db` found 545 live memory documents, a 105786-byte persisted graph, and 717 layer-node occurrences. Binary decoding took 0.30ms. The old restore path issued 717 queries and took 25.15ms. The database has the document lookup indexes. A private, collection-only snapshot with the newer lookup indexes removed took 51.32ms, still nowhere near 18 seconds. These observations do not establish whether the original delay involved cold storage, contention, a different binary/database state, or another stage of loading.

The newer 09:16 excerpt shows listener readiness and a rejected authentication request, but no measured RAG delay. The three seconds before SetProjectDB are not evidence of memory initialization blocking.

## Changes

- Read a collection's document identifiers in one query when restoring a lazy HNSW graph. Previously each node on every layer issued its own lookup. Embeddings, document contents, metadata, and PQ codes are not eagerly loaded.
- Resolve legacy byte UIDs and string document IDs, preserve lowest-row-ID selection for duplicate identifiers, and exclude soft-deleted rows. Missing referenced documents still fail restoration so existing controlled recovery remains available.
- Lazy embedding/PQ lookups use the resolved primary key and collection ID. This also prevents a document with the same logical ID in another collection supplying the wrong vector.
- Share the two duplicate binary restore implementations.
- Split memory timing into embedding availability and RAG loading. Slow collection loads report metadata, embedding-client setup, graph, document-count, and dimension stages. Graph restoration reports decoding, identifier query/iteration, and graph construction; concurrent cache-load waits have a separate warning.
- Replace `secret schema[bearer] missed` with a warning identifying the rejected RPC and missing/malformed Bearer authorization. Incorrect secrets return `Unauthenticated`, retaining the historical `secret verify failed` message. Credentials are never logged; authentication remains enforced for unary and streaming interceptors.

## Authentication explanation

The local Yakit startup implementation at `app/main/handlers/utils/engineStartup.js`, function `authenticatedProbe`, first checks an authenticated Echo, then deliberately repeats Echo without a password. It requires status 16 (`Unauthenticated`) to prove the endpoint enforces authentication. That second request triggers the old misleading ERROR log. The original log lacks the RPC method, so an individual historical line cannot independently identify its caller; the observed startup timing matches this explicit probe. No Yakit modification is needed to clarify the engine diagnostic.

## Measurements

macOS arm64, Go 1.22.12. These are graph-restore measurements, not end-to-end AI first-response latency.

| Dataset | Before | After | Notes |
| --- | ---: | ---: | --- |
| Existing memory, 545 documents | 25.15ms / 717 queries | 3.15–3.63ms / 1 query | After: three runs; vectors stay lazy |
| Synthetic 128 documents | 4.02ms | 0.76ms | Median of three benchmark runs |
| Synthetic 1024 documents | 32.55ms | 6.02ms | Median of three benchmark runs |
| Synthetic 1024 allocations | 313820/op | 31816/op | Median; allocated bytes about 22.9MB to 7.38MB |

Complete collection load with embedding initialization disabled took 3.68–5.36ms on the existing 545-document database. A second read-only replay against `yakit-projects-2/default-yakit.db` also completed in milliseconds; this database was being updated by the active application, so it is not used for a fixed-size before/after comparison.

The read-only profiler never migrates or repairs databases, calls embedding services, or persists graphs. GORM's SQL logger counts queries, but its Rows duration excludes row iteration; the comparison uses complete wall-clock restore time.

## Validation

All current-change checks passed:

```sh
go test ./common/ai/rag/vectorstore -run '^TestMUSTPASS_' -count=1 -timeout 180s
go test -race ./common/ai/rag/vectorstore -run '^TestMUSTPASS_(GraphStartup|DeleteBatchConcurrentReaders|GraphCacheIsolatedByDatabase|DeleteCollectionEvictsAndClosesGraph)' -count=3 -timeout 120s
go test ./common/ai/aid/aimem -run '^TestAsyncMemory' -count=3 -timeout 60s
go test ./common/yak/cmd -run '^TestGRPCSecretAuth$' -count=3 -timeout 60s
go build -o /tmp/yak-rag-startup-6339 ./common/yak/cmd
YAK_STARTUP_TEST_BINARY=/tmp/yak-rag-startup-6339 go test ./common/yak/cmd -run '^Test(GRPCSecretAuth|CLIStartupIPCRequiresAuthAndPreservesLiveEndpoint|CLIStartupLegacyTCPAuthentication)$' -count=1 -v -timeout 180s
go test ./common/ai/rag/vectorstore -run '^$' -bench '^BenchmarkGraphStartup$' -benchmem -benchtime=200ms -count=3 -timeout 120s
```

The CLI checks launch isolated child engines and temporary databases. They cover authenticated success, anonymous/wrong-password rejection, Unix endpoint ownership, TCP `--secret`, and TCP `--local-password`.

New graph tests cover string/legacy byte identifiers × ordinary/PQ lazy payloads, cross-collection collisions, missing/soft-deleted documents, and duplicate identifiers. Existing export/KeyAsUID, corruption recovery/deletion, cancellation, cache isolation, and concurrent deletion tests also pass. Re-running the new query-count regression against the original `utils.go` through a Go overlay fails all four branches: 170 queries instead of one, proving the test detects the old behavior.

Read-only profiling can be repeated with an explicit path:

```sh
YAK_RAG_STARTUP_PROFILE_DB=/absolute/path/to/project.db \
  go test ./common/ai/rag/vectorstore -run '^TestProfilePersistedRAGStartup$' -count=3 -v -timeout 120s
```

Logs retained under `/tmp/yak-rag-*.log`, including `startup-before`, `startup-after`, `bench-before`, `bench-after`, `all-mustpass`, `race`, and `cli-acceptance`. Earlier Schema-stream race failures are separate and have not been fixed by this branch's RAG changes.
