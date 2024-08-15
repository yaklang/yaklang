#!/bin/bash
# antlr -Dlanguage=Go ./SyntaxFlow.g4 -o sf -package sf -no-listener -visitor
antlr -Dlanguage=Go ./SyntaxFlowLexer.g4 ./SyntaxFlowParser.g4 -o sf -package sf -no-listener -visitor
