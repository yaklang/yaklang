#!/bin/sh

#需要antlr4.13.0及以上版本

rm ./parser/*.tokens
rm ./parser/*.interp
antlr4 -Dlanguage=Go ./JavaScriptLexer.g4 ./JavaScriptParser.g4 -o parser -no-listener -visitor