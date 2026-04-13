#!/bin/sh -e

SSA_TEST_TEMP_HOME=0
if [ -z "${YAKIT_HOME:-}" ]; then
  YAKIT_HOME="$(mktemp -d /tmp/yakit-home.XXXXXX)"
  export YAKIT_HOME
  SSA_TEST_TEMP_HOME=1
fi

cleanup_ssa_test_home() {
  status=$?
  if [ "${SSA_TEST_TEMP_HOME:-0}" = "1" ] && [ -n "${YAKIT_HOME:-}" ] && [ -d "${YAKIT_HOME}" ]; then
    rm -rf "${YAKIT_HOME}"
  fi
  trap - EXIT INT TERM
  exit "$status"
}

trap cleanup_ssa_test_home EXIT INT TERM

echo "Use isolated YAKIT_HOME: ${YAKIT_HOME}"
go run ./common/yak/cmd sync-rule

echo "Start to Test AST to SSA"
echo "Start to Test Yak SSA"
go test -timeout 20s ./common/yak/yak2ssa/test/...
go test -timeout 1m ./common/yak/ssaapi/test/yak

echo "Start to Test Python"
go test -timeout 2m ./common/yak/python/...
go test -timeout 1m ./common/yak/ssaapi/test/python

echo "Start to Test JS"
go test -timeout 1m ./common/yak/ssaapi/test/javascript
go test -timeout 2m ./common/yak/typescript/...

echo "Start to Test Java"
go test -timeout 5m ./common/yak/java/...
go test -timeout 3m ./common/yak/ssaapi/test/java

echo "Start to Test PHP"
go test -timeout 5m ./common/yak/php/...
go test -timeout 2m ./common/yak/ssaapi/test/php

echo "Start to Test Go"
go test -timeout 30s ./common/yak/go2ssa/...
go test -timeout 30s ./common/yak/antlr4go/...
go test -timeout 2m ./common/yak/ssaapi/test/golang

echo "Start to Test C"
go test -timeout 30s ./common/yak/c2ssa/...
go test -timeout 30s ./common/yak/antlr4c/...
go test -timeout 2m ./common/yak/ssaapi/test/c

echo "Start to Test SSAAPI"
go test -timeout 20s ./common/yak/ssa/...
go test -timeout 1m ./common/yak/ssaapi
go test -timeout 1m ./common/yak/ssaapi/ssareducer
go test -timeout 1m ./common/yak/ssaapi/test/ssatest

echo "Start to Test Syntaxflow rule"
go test -p 1 -count=1 -timeout 5m -skip "TestBuildInRule_Verify_DEBUG" ./common/syntaxflow/...

echo "SSA URL"
go test -count=1 -timeout 1m -run ^TestSFURl$ github.com/yaklang/yaklang/common/yak/yakurl

echo "Start to Test SSA-Analyze"
go test -timeout 20s ./common/yak/static_analyzer/test/...
go test -timeout 20s -run TestAnalyzeMustPASS* ./common/coreplugin
go test -timeout 5m ./common/syntaxflow/sfbuildin/...
go test -timeout 1m ./common/yak/ssaapi/test/syntaxflow
go test -v -timeout 20s ./common/syntaxflow/sfanalysis/...

echo "syntaxflow and language grpc"
go test -count=1 -timeout 5m -run TestGRPCMUSTPASS_SyntaxFlow* ./common/yakgrpc/...
go test -count=1 -timeout 5m -run TestGRPCMUSTPASS_LANGUAGE* ./common/yakgrpc/...
go test -v -timeout 5m -run TestGRPCMUSTPASS_SSA* ./common/yakgrpc/...

echo "Start to Test SFWEB"
go test -timeout 1m ./common/sfweb/...
go test -v -timeout 4m ./common/sfweb/...

echo "SyntaxFlow Core"
go test -v -timeout 20s ./common/syntaxflow/tests
go test -v -timeout 20s ./common/syntaxflow/sfdb/...
go test -v -timeout 20s ./common/syntaxflow/sfvm/...

echo "SyntaxFlow Scan"
go test -v -timeout 1m ./common/yak/syntaxflow_scan/...
