#!/bin/bash
# antlr -Dlanguage=Go ./SyntaxFlow.g4 -o sf -package sf -no-listener -visitor
java -jar ../yak/antlr4thirdparty/antlr-4.13.2-complete.jar -Dlanguage=Go ./SyntaxFlowLexer.g4 ./SyntaxFlowParser.g4 -o sf -package sf -no-listener -visitor
