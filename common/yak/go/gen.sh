#!/bin/sh

rm ./parser/*.tokens
rm ./parser/*.interp
java -jar ../antlr4thirdparty/antlr-4.11.1-complete.jar -Dlanguage=Go -package goparser ./GoLexer.g4 ./GoParser.g4 -o parser -no-listener -visitor