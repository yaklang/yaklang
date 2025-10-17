#!/bin/sh

#rm ./parser/*.tokens
#rm ./parser/*.interp

java -jar ../antlr4thirdparty/antlr-4.11.1-complete.jar -Dlanguage=Go ./CLexer.g4 ./CParser.g4 -o parser -package c -no-listener -visitor