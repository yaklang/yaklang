#!/bin/sh

#需要antlr4.11.0及以上版本

rm ./parser/*.tokens
rm ./parser/*.interp
// antlr4 -Dlanguage=Go ./JavaScriptLexer.g4 ./JavaScriptParser.g4 -o parser -no-listener -visitor
antlr -Dlanguage=Go ./JavaScriptLexer.g4 ./JavaScriptParser.g4 -o parser -package JS -no-listener -visitor



# re-gen build and test
# antlr -Dlanguage=Go ./JavaScriptLexer.g4 ./JavaScriptParser.g4 -o parser -package JS -no-listener -visitor && go test -count 1 -memprofile /tmp/a.out && go tool pprof -pdf ./antlr4JS.test binary /tmp/a.out > /tmp/a.pdf && open /tmp/a.pdf
# rm parser/JavaScriptLexer.tokens parser/JavaScriptParser.tokens && rm parser/JavaScriptLexer.interp parser/JavaScriptParser.interp && antlr -Dlanguage=Go ./JavaScriptLexer.g4 ./JavaScriptParser.g4 -o parser -package JS -no-listener -visitor && go test -run TestBasicSyntax