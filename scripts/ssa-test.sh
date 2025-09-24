#!/bin/sh -e

echo "Start to Test  AST to SSA"

echo "Start to Test Yak SSA"
# yak ssa
go test -timeout 20s ./common/yak/yak2ssa/test/...
go test -timeout 1m ./common/yak/ssaapi/test/yak
echo "Start to Test JS"
# Test js
# go test -timeout 60s ./common/yak/JS2ssa/...
go test -timeout 1m ./common/yak/ssaapi/test/javascript

echo "Start to Test Java"
go test -timeout 5m ./common/yak/java/...
go test -timeout 3m ./common/yak/ssaapi/test/java

echo "Start to Test PHP"
go test -timeout 2m ./common/yak/php/...
go test -timeout 2m ./common/yak/ssaapi/test/php

echo "Start to Test Go"
go test -timeout 30s ./common/yak/go2ssa/...
go test -timeout 30s ./common/yak/antlr4go/...
go test -timeout 2m ./common/yak/ssaapi/test/golang

echo "Start to Test C" 
go test -timeout 30s ./common/yak/c2ssa/...
go test -timeout 2m ./common/yak/ssaapi/test/c


echo "Start to Test SSAAPI"
# SSA
go test -timeout 20s ./common/yak/ssa/...
# SSAAPI
go test -timeout 1m ./common/yak/ssaapi
go test -timeout 1m ./common/yak/ssaapi/ssareducer
go test -timeout 1m ./common/yak/ssaapi/test/ssatest


echo "Start to Test Syntaxflow rule"
go test -count=1 -timeout 60s -skip "TestBuildInRule_Verify_DEBUG" ./common/syntaxflow/...

echo "SSA URL"
go test -count=1 -timeout 1m -run ^TestSFURl$ github.com/yaklang/yaklang/common/yak/yakurl

echo "Start to Test SSA-Analyze"

# SSA plugin rule/option
go test -timeout 20s ./common/yak/static_analyzer/test/...
# StaticAnalyze
go test -timeout 20s -run TestAnalyzeMustPASS* ./common/coreplugin
# BuildIn SyntaxFlow Rule
go test -timeout 2m ./common/syntaxflow/sfbuildin/...
go test -timeout 1m ./common/yak/ssaapi/test/syntaxflow

echo "syntaxflow and language grpc"
go test -count=1 -timeout 5m -run TestGRPCMUSTPASS_SyntaxFlow* ./common/yakgrpc/...
go test -count=1 -timeout 5m -run TestGRPCMUSTPASS_LANGUAGE* ./common/yakgrpc/...


echo "Start to Test SFWEB"
go test -timeout 1m ./common/sfweb/...