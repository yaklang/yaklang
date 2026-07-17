#!/bin/sh

rm -f ./parser/*.tokens
rm -f ./parser/*.interp
../antlr4util/antlr4 -Dlanguage=Go ./NaslLexer.g4 ./NaslParser.g4 -o parser -no-listener -visitor
