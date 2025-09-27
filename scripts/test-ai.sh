#!/bin/bash -e

go test -v -timeout 3m ./common/ai/aid/...  | grep -E "(PASS|FAIL|RUN|=== RUN|--- PASS|--- FAIL|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|test timed out|testing\..*panic|recovered)"
go test -v -timeout 60s ./common/ai/tests/...  | grep -E "(PASS|FAIL|RUN|=== RUN|--- PASS|--- FAIL|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|test timed out|testing\..*panic|recovered)"
go test -v -timeout 60s ./common/ai/rag/pq/... | grep -E "(PASS|FAIL|RUN|=== RUN|--- PASS|--- FAIL|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|test timed out|testing\..*panic|recovered)"
go test -v -timeout 60s ./common/ai/rag/hnsw/... | grep -E "(PASS|FAIL|RUN|=== RUN|--- PASS|--- FAIL|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|test timed out|testing\..*panic|recovered)"
go test -v -timeout 60s ./common/ai/aispec/...   | grep -E "(PASS|FAIL|RUN|=== RUN|--- PASS|--- FAIL|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|test timed out|testing\..*panic|recovered)"
go test -v -timeout 60s ./common/aireducer/...   | grep -E "(PASS|FAIL|RUN|=== RUN|--- PASS|--- FAIL|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|test timed out|testing\..*panic|recovered)"
go test -v -timeout 40s ./common/aiforge/aibp/forge_builder_test.go   | grep -E "(PASS|FAIL|RUN|=== RUN|--- PASS|--- FAIL|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|test timed out|testing\..*panic|recovered)"
go test -v -timeout 3m ./common/aiforge   | grep -E "(PASS|FAIL|RUN|=== RUN|--- PASS|--- FAIL|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|test timed out|testing\..*panic|recovered)"
go test -v -timeout 60s ./common/ai/rag/entityrepos/... | grep -E "(PASS|FAIL|RUN|=== RUN|--- PASS|--- FAIL|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|test timed out|testing\..*panic|recovered)"
go test -v -timeout 1m -run TestMUSTPASS ./common/ai/rag   | grep -E "(PASS|FAIL|RUN|=== RUN|--- PASS|--- FAIL|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|test timed out|testing\..*panic|recovered)"
go test -v -timeout 1m -run TestMUSTPASS ./common/ai/rag/plugins_rag/...   | grep -E "(PASS|FAIL|RUN|=== RUN|--- PASS|--- FAIL|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|test timed out|testing\..*panic|recovered)"
