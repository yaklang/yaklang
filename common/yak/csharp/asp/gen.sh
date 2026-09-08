#!/bin/sh

rm -f ./parser/*.tokens
rm -f ./parser/*.interp
../../antlr4util/antlr4 -Dlanguage=Go -package aspparser ./ASPLexer.g4 ./ASPParser.g4 -o parser -no-listener -visitor
