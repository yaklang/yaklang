#!/bin/bash -e

clear
clear

set -e
set -o pipefail

export GITHUB_ACTIONS=true

# 禁用并行测试以避免数据库锁定问题
# 所有 AI 测试共享同一个全局数据库实例，必须串行执行
echo "Running AI tests sequentially to avoid database lock conflicts..."

# 添加 -p=1 参数强制串行执行，避免数据库并发访问冲突
go test -p=1 -v -timeout 3m ./common/ai/aid/... 2>&1 | tee /tmp/ai_aid_test.log | { grep -E -A10 -B10 "(FAIL|--- FAIL|panic:|test timed out)" || grep -E "(PASS|RUN|=== RUN|--- PASS|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|testing\..*panic|recovered)" /tmp/ai_aid_test.log; }
go test -p=1 -v -timeout 60s ./common/ai/tests/... 2>&1 | tee /tmp/ai_tests_test.log | { grep -E -A10 -B10 "(FAIL|--- FAIL|panic:|test timed out)" || grep -E "(PASS|RUN|=== RUN|--- PASS|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|testing\..*panic|recovered)" /tmp/ai_tests_test.log; }
go test -p=1 -v -timeout 60s ./common/ai/rag/pq/... 2>&1 | tee /tmp/ai_rag_pq_test.log | { grep -E -A10 -B10 "(FAIL|--- FAIL|panic:|test timed out)" || grep -E "(PASS|RUN|=== RUN|--- PASS|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|testing\..*panic|recovered)" /tmp/ai_rag_pq_test.log; }
go test -p=1 -v -timeout 60s ./common/ai/rag/hnsw/... 2>&1 | tee /tmp/ai_rag_hnsw_test.log | { grep -E -A10 -B10 "(FAIL|--- FAIL|panic:|test timed out)" || grep -E "(PASS|RUN|=== RUN|--- PASS|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|testing\..*panic|recovered)" /tmp/ai_rag_hnsw_test.log; }
go test -p=1 -v -timeout 60s ./common/ai/aispec/... 2>&1 | tee /tmp/ai_aispec_test.log | { grep -E -A10 -B10 "(FAIL|--- FAIL|panic:|test timed out)" || grep -E "(PASS|RUN|=== RUN|--- PASS|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|testing\..*panic|recovered)" /tmp/ai_aispec_test.log; }
go test -p=1 -v -timeout 60s ./common/aireducer/... 2>&1 | tee /tmp/aireducer_test.log | { grep -E -A10 -B10 "(FAIL|--- FAIL|panic:|test timed out)" || grep -E "(PASS|RUN|=== RUN|--- PASS|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|testing\..*panic|recovered)" /tmp/aireducer_test.log; }
go test -p=1 -v -timeout 40s ./common/aiforge/aibp/forge_builder_test.go 2>&1 | tee /tmp/aiforge_aibp_test.log | { grep -E -A10 -B10 "(FAIL|--- FAIL|panic:|test timed out)" || grep -E "(PASS|RUN|=== RUN|--- PASS|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|testing\..*panic|recovered)" /tmp/aiforge_aibp_test.log; }
go test -p=1 -v -timeout 3m ./common/aiforge 2>&1 | tee /tmp/aiforge_test.log | { grep -E -A10 -B10 "(FAIL|--- FAIL|panic:|test timed out)" || grep -E "(PASS|RUN|=== RUN|--- PASS|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|testing\..*panic|recovered)" /tmp/aiforge_test.log; }
go test -p=1 -v -timeout 60s ./common/ai/rag/entityrepos/... 2>&1 | tee /tmp/ai_rag_entityrepos_test.log | { grep -E -A10 -B10 "(FAIL|--- FAIL|panic:|test timed out)" || grep -E "(PASS|RUN|=== RUN|--- PASS|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|testing\..*panic|recovered)" /tmp/ai_rag_entityrepos_test.log; }
go test -p=1 -v -timeout 1m -run TestMUSTPASS ./common/ai/rag 2>&1 | tee /tmp/ai_rag_mustpass_test.log | { grep -E -A10 -B10 "(FAIL|--- FAIL|panic:|test timed out)" || grep -E "(PASS|RUN|=== RUN|--- PASS|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|testing\..*panic|recovered)" /tmp/ai_rag_mustpass_test.log; }
go test -p=1 -v -timeout 1m -run TestMUSTPASS ./common/ai/rag/plugins_rag/... 2>&1 | tee /tmp/ai_rag_plugins_test.log | { grep -E -A10 -B10 "(FAIL|--- FAIL|panic:|test timed out)" || grep -E "(PASS|RUN|=== RUN|--- PASS|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|testing\..*panic|recovered)" /tmp/ai_rag_plugins_test.log; }
