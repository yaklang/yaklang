#!/bin/sh

rm -f ./parser/*.tokens
rm -f ./parser/*.interp
java -jar ../antlr4thirdparty/antlr-4.13.2-complete.jar -Dlanguage=Go ./NaslLexer.g4 ./NaslParser.g4 -o parser -no-listener -visitor
