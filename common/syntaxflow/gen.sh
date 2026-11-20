#!/bin/bash
# antlr -Dlanguage=Go ./SyntaxFlow.g4 -o sf -package sf -no-listener -no-visitor
../yak/antlr4util/antlr4 -Dlanguage=Go ./SyntaxFlowLexer.g4 ./SyntaxFlowParser.g4 -o sf -package sf -no-listener -no-visitor
