#!/bin/sh

rm -f ./parser/*.tokens
rm -f ./parser/*.interp
../antlr4util/antlr4 -Dlanguage=Go -package csharpparser ./CSharpLexer.g4 ./CSharpParser.g4 -o parser -no-listener -visitor
